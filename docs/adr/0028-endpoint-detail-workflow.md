# ADR-0028 — `GET /v1/workflows/{id}` : détail d'une version de workflow

## Statut
Accepté

## Contexte

`GET /v1/workflows` (liste) ne retourne que `id`, `version`, `installed_at`
— assez pour lister les versions installées, pas pour afficher la
structure d'un workflow (ses steps, l'action de chacun, ses entrées). Côté
CLI, `patchcord workflow export` couvre ce besoin en imprimant le YAML
brut d'une version, mais rien d'équivalent n'existait côté API publique —
un client qui veut afficher « les détails » d'un workflow (un dashboard,
typiquement) n'avait aucune route à appeler.

Cette absence est devenue bloquante en construisant le dashboard React de
référence (`apps/examples/dashboard`) : afficher la liste des steps d'un
workflow avant de le lancer suppose de connaître sa définition, pas
seulement son id et sa version.

## Décision

**Nouvelle route `GET /v1/workflows/{id}`, avec un `?version=` optionnel**
(défaut : la dernière version installée, même convention que
`runs.WorkflowSource` et `patchcord workflow export --version`).
`handleGetWorkflow` (`internal/api/workflows.go`) réutilise
`runs.WorkflowSource` pour charger le YAML puis `workflow.Parse` pour
obtenir la définition — aucune nouvelle logique de lecture, uniquement un
nouveau point d'entrée HTTP sur des fonctions déjà éprouvées par le CLI.

**Une forme JSON structurée, pas juste le YAML brut en `text/plain`.** Le
type de réponse `workflowDetail` expose `id`, `version`, `schema_version`,
`trigger_type` et `steps` (chacun avec `id`, `uses`, `with`, `connector`)
en plus de `source` (le YAML brut, pour un éventuel affichage « voir la
source »). Un client (le dashboard, ou tout futur consommateur JS) peut
ainsi rendre une table de steps sans embarquer un parseur YAML côté
client — la seule source de vérité pour la syntaxe d'un workflow reste
`internal/workflow.Parse`, cohérent avec le non-négociable #8 (CLI et API
passent par la même couche applicative). Reparser le YAML aurait dupliqué
cette logique en TypeScript, avec un risque de divergence.

**Un type de réponse dédié, pas `workflow.Definition` sérialisé
directement.** Comme `runSummary`/`appSummary` avant lui, `workflowDetail`
est un type propre à `internal/api`, avec des tags JSON explicites
(`workflow.Definition` ne porte que des tags `yaml`) — cohérent avec le
reste du package, qui n'expose jamais un type interne tel quel sur le fil.

## Conséquences positives

- Débloque l'affichage des détails d'un workflow dans le dashboard React
  sans dupliquer le parseur YAML côté client.
- Aucune nouvelle logique métier : la route ne fait que composer
  `runs.WorkflowSource` et `workflow.Parse`, déjà testés indépendamment.
- Spec OpenAPI régénérée (`make swagger`) — le contrat public reste la
  source de vérité versionnée (non-négociable #5).

## Conséquences négatives

- Un nouveau champ de surface publique à maintenir en compatibilité
  ascendante : toute évolution du schéma de workflow (nouveaux champs de
  step, par exemple) devra être répercutée à la fois dans `workflowDetail`
  et dans le SDK TypeScript (`WorkflowDetail`), pas seulement dans
  `internal/workflow`.
