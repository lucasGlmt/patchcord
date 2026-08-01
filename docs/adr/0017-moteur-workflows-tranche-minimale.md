# ADR-0017 — Moteur de workflows : tranche verticale minimale

## Statut
Accepté

## Contexte
La checklist complète de la phase 3 (section 19 du document de vision) est large : modèles versionnés, compilation, runner séquentiel, expressions, persistance, timeouts, annulation, historique, événements temps réel. Suivant la même méthode que les phases 1 et 2, seule une tranche verticale minimale a été construite dans cette passe, avec pour objectif de reproduire de bout en bout l'exemple `hello_patchcord` de la section 20.

Deux questions structurantes se posaient : jusqu'où pousser le langage d'expression `${{ ... }}`, et comment `patchcord workflow run` accède à de vrais processus de greffons pour appeler une action — un problème similaire à celui déjà tranché par l'ADR-0015 pour les commandes `plugin`, mais pas identique : install/list/inspect de greffons ou de workflows ne font que lire/écrire un catalogue, alors qu'exécuter un workflow appelle réellement des processus vivants.

## Décision
**Découpage des paquets** : conforme au mot pour mot de CLAUDE.md section 2. `internal/workflow` est le moteur pur (parsing, compilation/validation, résolution d'expressions, machines à états `Run`/`Step`) — aucune dépendance à la persistance ni aux processus. `internal/runs` est le gestionnaire d'exécutions : persistance SQLite des versions de workflow, runs et steps, et orchestration contre une interface `ActionExecutor` (satisfaite par `internal/plugins.Supervisor`).

**Expressions** : seule la substitution de valeur entière `${{ workflow.inputs.<clé> }}` ou `${{ steps.<id>.outputs.<clé> }}` est supportée — pas d'interpolation partielle, pas d'opérateurs, pas de fonctions. La validation à la compilation (`workflow.Validate`) rejette déjà les formes d'expression malformées et les références à une étape inconnue ou définie plus tard, avant qu'un run ne démarre.

**`patchcord workflow run`** lance son propre `plugins.Supervisor` le temps du run — exactement comme `patchcord serve` au démarrage — plutôt que d'exiger un agent déjà lancé ou de construire une API HTTP `/v1/workflows/run`. Ceci étend le précédent de l'ADR-0015 (les commandes CLI sont autosuffisantes) à la seule commande qui ne peut structurellement pas se contenter de lire/écrire un catalogue, puisqu'exécuter un workflow signifie appeler de vrais processus en cours d'exécution.

**États Run/Step** : persistés selon les tables de transition explicites d'`internal/workflow/state.go`. Une étape qui échoue avant même d'appeler son action (ex. une expression non résolvable) passe tout de même par `running` avant `failed` — aucun état terminal n'est atteint sans être passé par `running`.

## Conséquences positives
- `internal/workflow` reste trivialement testable unitairement (aucune dépendance DB ni processus), exactement ce que CLAUDE.md section 5 exige pour la machine à états du moteur.
- Réutiliser exactement la frontière de paquets que le document de vision et CLAUDE.md nomment évite une restructuration ultérieure quand le scheduler, les déclencheurs webhook ou une vraie API arriveront.
- La grammaire d'expression minimale suffit à prouver le chaînage entre étapes (`steps.first.outputs.value` → entrée de `steps.second`) tout en gardant petite la surface du contrat public (le format YAML des workflows) — moins de surface à figer avant que l'immutabilité de l'ADR-0008 ne s'applique.
- `workflow run` prouve réellement que le protocole, le Supervisor et le runner s'articulent, via un vrai sous-processus de bout en bout (`internal/cli/workflow_test.go`, `TestWorkflowLifecycle_HelloPatchcordEndToEnd`), pas un mock.

## Conséquences négatives
- Chaque `patchcord workflow run` relance tous les greffons installés (pas seulement ceux dont le workflow a besoin) et les arrête ensuite — coûteux pour un catalogue de greffons important face à un petit workflow, et plus lent que réutiliser les processus déjà supervisés par un agent en cours d'exécution. À revoir quand une vraie API permettra à la CLI de soumettre un run à un `patchcord serve` déjà actif.
- Aucune vraie concurrence, aucun retry, aucun timeout par étape : une action bloquée bloque tout le run indéfiniment (borné seulement par le contexte de l'appelant, s'il en fournit un). Explicitement reporté, comme le prévoyait déjà la checklist de la section 19 ("timeouts", "annulation").
- Aucun événement temps réel (SSE) : l'appelant ne voit le résultat qu'une fois `Execute` terminé, sans visibilité sur la progression d'un run long. Également reporté.
- La simplicité de la grammaire d'expression signifie que tout workflow ayant besoin de concaténation, de conditions ou de valeurs calculées doit se découper en plusieurs étapes ou attendre une évolution future du langage — et comme les versions publiées sont immuables (ADR-0008), une évolution de la grammaire ne s'appliquera qu'aux nouvelles versions, jamais rétroactivement.
