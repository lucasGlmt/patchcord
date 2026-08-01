# ADR-0019 — Événements temps réel des runs par SSE, via scrutation de la base

## Statut
Accepté

## Contexte
L'ADR-0018 avait volontairement laissé de côté les événements temps réel (SSE) de la checklist de la phase 3, en raison de leur ampleur : ils nécessitent une première vraie surface d'API HTTP au-delà de `/v1/system/health`. Cette passe la referme.

Le document de vision prévoit une route `/v1/events` (section 10.1) et esquisse un event log (`run.created`, `run.started`, `step.started`, ...) en section 14, en le présentant comme un mécanisme qui facilite « diffusion en temps réel » et « reconstruction de l'historique ». Il ne prescrit pas d'implémentation précise.

Le point structurant est celui posé par l'ADR-0017 et confirmé par l'ADR-0018 : `patchcord workflow run` exécute un workflow **de façon synchrone, dans son propre processus**, du début à la fin. Il n'existe donc aujourd'hui aucun bus d'événements en mémoire que le serveur HTTP de l'agent (`patchcord serve`, un processus généralement distinct) pourrait consommer directement — les deux processus ne partagent que la base SQLite.

## Décision

**Scrutation de la base plutôt que diffusion push.** `runs.WatchRun(ctx, db, runID)` interroge périodiquement (`runs.watchPollInterval`, 250 ms) le statut du run et de ses étapes via `GetRun`, et livre un `runs.Event` sur un channel Go à chaque changement de statut observé, jusqu'à ce que le run atteigne un statut terminal ou que `ctx` soit annulé — auquel cas le channel est fermé. C'est le seul canal disponible entre un processus `workflow run` qui écrit et un processus `serve` qui veut observer, sans coupler les deux au-delà du fichier SQLite partagé (WAL, déjà en place depuis la phase 1).

**Route publique `GET /v1/runs/{id}/events`**, servie par `internal/api`, encode chaque `runs.Event` en Server-Sent Event (`event: <run|step>.<statut>`, `data: <JSON>`), avec flush après chaque événement. Elle retourne 404 si le run n'existe pas, avant même d'ouvrir le flux.

**Pas d'event log persistant séparé.** Contrairement à ce que section 14 du document de vision esquisse, cette implémentation ne crée pas de table d'événements dédiée : `runs`/`run_steps` ne retiennent que le statut courant de chaque entité, pas l'historique de ses transitions. Conséquence directement testée (`TestWatchRun_ObservesEachStatusChangeInOrder`) : un client qui se connecte **pendant** l'exécution observe chaque transition au fil de l'eau ; un client qui se connecte **après coup**, sur un run déjà terminé, ne reçoit que le statut final de chaque entité, pas les statuts intermédiaires déjà disparus. C'est un compromis délibéré pour rester dans le périmètre strict de la phase 3 (diffusion temps réel) sans anticiper l'event log complet, qui reste un candidat naturel pour une phase ultérieure si l'historique détaillé s'avère nécessaire.

**Couche réutilisable, pas seulement un handler HTTP.** `runs.WatchRun` vit dans `internal/runs`, pas dans `internal/api` — cohérent avec le non-négociable #8 (CLI et API passent par les mêmes services internes). Le handler HTTP se contente de traduire les `runs.Event` en trame SSE ; une future commande CLI de suivi (`run logs --follow`, non implémentée ici, hors périmètre demandé) pourrait consommer la même fonction sans dupliquer la logique de scrutation.

## Conséquences positives
- Ferme le dernier point de la checklist phase 3 (« événements en temps réel ») sans introduire de dépendance externe (pas de message broker) ni de mécanisme IPC supplémentaire entre `serve` et `workflow run`.
- Fonctionne à l'identique quel que soit l'endroit où tourne l'agent (non-négociable #1/#2 : pas de branchement « mode local » vs « mode serveur »), puisque la scrutation ne dépend que du fichier SQLite déjà partagé.
- La fermeture du channel/flux sur statut terminal évite les connexions SSE qui pendent indéfiniment ; la fermeture sur annulation de `ctx` gère proprement la déconnexion client.
- `runs.WatchRun` est testée indépendamment du transport HTTP (`internal/runs/watch_test.go`), conformément à l'exigence de tests de CLAUDE.md ; un test de bout en bout (`internal/runtime/agent_test.go`) exerce en plus le vrai socket HTTP avec deux connexions SQLite distinctes, pour se rapprocher du scénario réel à deux processus.

## Conséquences négatives
- Latence de 250 ms au pire avant qu'un changement de statut soit observé, ce n'est pas un flux « instantané » — jugé largement suffisant pour un usage humain (suivi de run dans une UI ou un terminal), à revisiter si un cas d'usage exige du sub-100ms.
- Un run très rapide peut voir certaines de ses étapes changer de statut deux fois entre deux scrutations (ex. `running` puis `succeeded` avant le prochain tick) : seul le dernier statut observé est alors livré, l'étape intermédiaire est silencieusement absente du flux. Documenté explicitement dans le commentaire de `WatchRun`.
- Une scrutation par connexion SSE ouverte ajoute une requête SQLite toutes les 250 ms par flux actif ; avec `SetMaxOpenConns(1)` (ADR de persistance de la phase 1), un grand nombre de flux SSE simultanés dans le même processus `serve` pourrait sérialiser ces lectures derrière l'unique connexion — non problématique à l'échelle actuelle (agent local, usage mono-utilisateur), à réévaluer si le mode serveur multi-utilisateurs de la phase 6 change l'échelle attendue.
- Sans event log persistant, un client qui rate le début d'un run déjà terminé ne peut pas reconstruire son historique détaillé (section 14 du document de vision le présente justement comme un des bénéfices d'un event log) — seul `patchcord run logs`/`run inspect`, qui lisent l'état final persisté, restent la source de vérité pour l'historique après coup.
