# ADR-0035 — Trigger `schedule` et scheduler persistant

## Statut
Accepté

## Contexte
Jusqu'ici `workflow.Trigger` n'acceptait qu'un seul type, `"manual"` : un workflow ne démarre que sur demande explicite (`patchcord workflow run`, `POST /v1/workflows/{id}/run`). Lucas veut pouvoir déclencher un workflow automatiquement, sur une expression cron.

Le document de vision range explicitement le "scheduler persistant" en Phase 6 (Déploiement serveur, section 19), aux côtés de Docker/TLS/secret providers/webhooks. Au moment où cette décision est prise, les Phases 4 (Connecteurs) et 5 (SDK TypeScript et applications) venaient tout juste d'être committées et stabilisées (`c9dafc0 feat(app): dashboard`) — donc anticiper un mécanisme de Phase 6 est en tension directe avec CLAUDE.md section 9 ("ne pas anticiper des mécanismes de phases ultérieures tant que la phase courante n'est pas stable"). Lucas a choisi explicitement, en connaissance de cause, de commencer la Phase 6 par ce chantier précis plutôt que par l'authentification distante ou les webhooks — un trigger cron ne nécessite aucune nouvelle surface réseau ni aucun secret, contrairement aux autres chantiers de la phase, ce qui en fait le point d'entrée au risque architectural le plus faible.

Trois questions de conception ont été tranchées avec Lucas avant l'implémentation :

1. **Parsing cron** : bibliothèque externe éprouvée plutôt qu'un parseur maison, pour éviter de réinventer les pièges classiques (fin de mois, DST). `github.com/robfig/cron/v3` est une dépendance générique de parsing, pas un service métier concret — elle ne viole pas le non-négociable #3 (section 1).
2. **Occurrences manquées** : si l'agent était éteint au moment d'un déclenchement prévu, le comportement doit être paramétrable plutôt que figé à un seul choix.
3. **Inputs et bindings d'un run automatique** : un run manuel reçoit ses inputs et ses bindings de connecteur à l'appel (`body.Inputs`/`body.Bindings` de `POST /v1/workflows/{id}/run`). Un run déclenché par le scheduler n'a personne pour les fournir en direct. Lucas a choisi de **restreindre** plutôt que d'étendre le schéma : un trigger `schedule` interdit tout input `required` sans `default` et toute étape avec un `connector`, plutôt que de stocker des inputs/bindings par défaut au niveau du schedule (repoussé — pas de besoin réel identifié aujourd'hui).

Périmètre de cette session, également arbitré avec Lucas : backend complet (moteur, validation, tests) **et** un affichage dashboard, mais pas un composant de saisie — le dashboard n'a aucun écran de création/édition de workflow (tout passe par `patchcord workflow install` en CLI ; le SDK TypeScript n'expose que `list`/`get`/`run`), donc un composant "remplir le cron facilement" n'a nulle part où vivre tant qu'un éditeur de workflow n'existe pas. Construire un tel composant maintenant, non câblé à une page, aurait été une implémentation à moitié terminée.

C'est une décision d'architecture au sens de CLAUDE.md section 6 : elle étend le format public des workflows (`trigger.type`), le protocole HTTP (`GET /v1/workflows/{id}`) et introduit un nouveau composant du core (`internal/scheduler`).

## Décision

**Format.** `workflow.Trigger` (`internal/workflow/definition.go`) gagne deux champs, significatifs uniquement quand `type: schedule` : `cron` (expression standard à 5 champs, requise) et `on_missed` (`"skip"` par défaut, ou `"fire_once"`). `workflow.Validate` (`internal/workflow/compile.go`) :
- valide `cron` via `cron.ParseStandard` (`robfig/cron/v3`) à l'installation, jamais seulement au premier déclenchement raté ;
- rejette `cron`/`on_missed` sur un trigger `"manual"` — pas de champ silencieusement ignoré ;
- rejette, pour un trigger `"schedule"` uniquement, tout input `required: true` sans `default` et toute étape avec un `connector:` non vide — un run automatique n'a personne pour les fournir.

**Persistance.** Une nouvelle table `schedules` (`migrations/0006_schedules.sql`), clé primaire `workflow_id` (pas `(workflow_id, version)`) : un schedule suit toujours la dernière version installée d'un workflow, comme `runs.Execute` résout toujours "la dernière version installée" ([runner.go](../../internal/runs/runner.go)). Colonnes : `cron`, `on_missed`, `next_run_at`, `updated_at`.

**`internal/scheduler`**, nouveau package, en deux parties :
- `Sync(ctx, db, def)` — upsert ou supprime la ligne `schedules` de `def.ID` selon `def.Trigger.Type`. Installer une nouvelle version reprogramme toujours `next_run_at` depuis "maintenant", quel que soit le cron de la version précédente. Appelé explicitement par `internal/cli/workflow.go` juste après `runs.InstallWorkflow` — **pas** à l'intérieur de `runs.InstallWorkflow` lui-même, pour une raison de dépendances : `internal/scheduler` importe `internal/runs` (pour déclencher un run via `runs.Execute`), donc l'inverse (`internal/runs` important `internal/scheduler`) créerait un cycle. Le même principe vaudra pour une future route HTTP d'installation.
- `Runner` — boucle de fond, démarrée dans `internal/runtime.Agent.NewAgent` sous le même `runCtx` que les runs déclenchés par l'API HTTP (annulé à l'arrêt de l'agent, donc un run planifié en cours est enregistré `Cancelled` comme n'importe quel autre). Elle interroge `schedules` toutes les 30 secondes (`pollInterval`) — pas d'attente précise jusqu'au prochain `next_run_at` connu, parce que `patchcord workflow install` tourne dans un processus séparé et n'a aucun moyen de réveiller un `patchcord serve` déjà lancé au moment où il écrit une ligne. Chaque ligne due est déclenchée via `runs.Execute` dans sa propre goroutine, exactement le chemin qu'emprunte déjà `handleRunWorkflow` — un run planifié n'est pas un type de run différent, juste déclenché différemment.

**Occurrences manquées.** En trouvant une ligne due, `Runner` compte combien d'occurrences du cron sont passées entre l'ancien `next_run_at` et l'instant présent. Exactement une occurrence (le cas normal, l'agent tournait en continu) déclenche toujours, indépendamment d'`on_missed`. Plus d'une occurrence signifie que l'agent était hors ligne à travers au moins une période complète : `on_missed: skip` n'exécute rien, `on_missed: fire_once` exécute une seule fois pour rattraper, peu importe le nombre réel d'occurrences manquées.

**API.** `GET /v1/workflows/{id}` (`internal/api/workflows.go`) expose `trigger_cron`, `trigger_on_missed` (toujours la valeur effective — `"skip"` même quand le YAML l'a laissé implicite) et `next_run_at` (lu depuis `schedules`, donc reflète l'état vivant même en consultant une ancienne version installée).

**Dashboard.** Affichage seul sur la page de détail de workflow existante (cron + prochain déclenchement) — pas de composant de saisie cette session, faute d'écran d'édition de workflow où le brancher (voir Contexte).

## Conséquences positives
- Aucune nouvelle surface réseau ni aucun secret impliqué — le risque architectural de ce premier chantier de Phase 6 est minimal, cohérent avec le choix de Lucas de commencer par lui.
- Un run planifié emprunte exactement le même chemin (`runs.Execute`) qu'un run manuel ou déclenché par l'API — aucune logique d'exécution dupliquée (non-négociable #8).
- La restriction (pas d'input requis sans défaut, pas de binding connecteur) évite d'étendre prématurément le schéma `schedules` avec des inputs/bindings par défaut avant qu'un besoin réel ne le justifie — cohérent avec la façon dont le DAG et la policy d'erreur déclarative ont déjà été délibérément reportés.
- `on_missed` réglable évite de figer un choix produit non consensuel (rattrapage silencieux vs perte silencieuse d'exécutions) dans le comportement par défaut du moteur.

## Conséquences négatives
- Un workflow avec un input requis sans défaut ou une étape liée à un connecteur ne peut pas être planifié tel quel — il doit rester `"manual"`, ou être restructuré. Repoussé faute de besoin réel identifié ; à revisiter si un cas concret l'exige.
- Le polling à 30 secondes signifie qu'un `cron` très fin (`* * * * *`, chaque minute) peut se déclencher avec jusqu'à 30 secondes de retard par rapport à l'occurrence exacte — acceptable pour de l'automatisation, pas pour un usage nécessitant une précision à la seconde (hors de portée d'un cron à 5 champs de toute façon).
- Le dashboard ne permet toujours pas de créer ou modifier un trigger `schedule` sans passer par la CLI — un vrai éditeur de workflow reste un chantier non engagé.
