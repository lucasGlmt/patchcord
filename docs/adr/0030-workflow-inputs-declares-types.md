# ADR-0030 — Inputs de workflow déclarés et typés

## Statut
Accepté

## Contexte

Un workflow référence ses entrées uniquement via des expressions
`${{ workflow.inputs.<key> }}` (`internal/workflow/expr.go`) : rien dans
`workflow.Definition` (`internal/workflow/definition.go`) ne déclarait
jusqu'ici quelles entrées un workflow attend, leur type, ni si elles sont
obligatoires. Deux conséquences concrètes :

- `GET /v1/workflows/{id}` (ADR-0028) ne pouvait exposer aucune information
  permettant à un client de construire un formulaire de lancement. Le
  dashboard de référence (`apps/examples/dashboard`) retombait sur un
  textarea JSON brut pour les inputs — inutilisable pour un workflow comme
  `greet_twice`, où l'utilisateur attend simplement un champ texte `name`.
- Une entrée manquante ou mal orthographiée n'échouait qu'au moment de la
  résolution d'une expression, en pleine exécution d'un step (`workflow
  input "name" was not provided`) — contrairement à toutes les autres
  classes de typo que `workflow.Validate` détecte déjà à l'installation
  (action inconnue, référence de step en avant, connecteur non-expression).

## Décision

**Un bloc `inputs:` optionnel dans le format YAML des workflows**, une
liste de `{name, type, required, description, default, enum}`
(`workflow.InputDef`). `type` vaut `string` (par défaut), `number`,
`boolean` ou `enum`. `workflow.Validate` appelle `validateInputDefs`
(`internal/workflow/inputs.go`) : noms uniques et non vides, type connu,
`enum` exige une liste `enum` non vide (et rejetée pour tout autre type),
`required` et `default` sont mutuellement exclusifs (un défaut satisferait
silencieusement "required", rendant le champ incohérent), et `default`
doit correspondre au type déclaré.

**Un workflow qui ne déclare pas `inputs` garde exactement le comportement
actuel** : toute clé passée à `${{ workflow.inputs.<key> }}` est acceptée
sans validation. Aucune migration n'est requise pour les workflows déjà
installés.

**`workflow.PrepareInputs`** applique le schéma déclaré à chaque
démarrage de run (`runs.Start`, appelé aussi bien par `patchcord workflow
run` que par `POST /v1/workflows/{id}/run`) : une clé fournie mais non
déclarée est rejetée (même logique de détection de typo que le reste de
`Validate`), une entrée manquante retombe sur `default` sinon échoue si
`required`, et chaque valeur est coercée vers son type déclaré — la CLI
(`--input key=value`, `StringToStringVar`) ne fournit que des chaînes,
alors qu'un corps JSON HTTP porte déjà des valeurs typées ; les deux
chemins passent par la même coercion. `runs.Start` retourne désormais ce
map préparé (defaults appliqués, valeurs coercées) en plus de la
définition et du run, et c'est ce map — pas les entrées brutes de
l'appelant — qui doit être transmis à `runs.Continue` pour que la
résolution des expressions voie les mêmes valeurs que celles persistées
comme `run.Inputs`.

**`GET /v1/workflows/{id}` expose désormais `inputs`** (nouveau type
`workflowInputDetail`, `internal/api/workflows.go`), régénérant le contrat
OpenAPI (`make swagger`) — cohérent avec la façon dont ADR-0028 a exposé
`steps`. **`POST /v1/workflows/{id}/run` répond `400`** (pas `500`) quand
les inputs fournis ne satisfont pas le schéma déclaré
(`errors.Is(err, workflow.ErrInvalidInputs)`), distinguant une erreur
cliente d'une panne interne.

**Le dashboard React construit un vrai formulaire** à partir de
`workflow.inputs` (`RunDialog.tsx`) — un champ texte pour `string`, un
champ numérique pour `number`, un switch pour `boolean`, un select pour
`enum` — et ne retombe sur le textarea JSON que pour un workflow qui ne
déclare aucun schéma.

## Conséquences positives

- Un client (dashboard, ou tout futur consommateur du SDK TypeScript) peut
  construire un formulaire de lancement typé sans deviner la forme des
  inputs à partir du YAML brut.
- Une entrée manquante ou mal orthographiée échoue à l'installation
  (`workflow.Validate`, pour un schéma incohérent) ou immédiatement au
  lancement d'un run (`PrepareInputs`), jamais silencieusement en pleine
  exécution d'un step.
- Rétrocompatible à 100 % : tout workflow déjà installé, n'ayant jamais
  déclaré `inputs`, continue de fonctionner sans aucune modification.

## Conséquences négatives

- Nouvelle surface publique à maintenir en compatibilité ascendante :
  toute évolution du schéma d'input (un nouveau `type`, par exemple) devra
  être répercutée à la fois dans `internal/workflow`, `workflowDetail`
  (`internal/api`) et le SDK TypeScript (`WorkflowInputDetail`), comme
  ADR-0028 l'avait déjà noté pour `steps`.
- Le jeu de types (`string`/`number`/`boolean`/`enum`) est volontairement
  restreint ; un besoin de type personnalisé porté par un greffon (évoqué
  par Lucas, explicitement écarté pour l'instant) demanderait une nouvelle
  décision d'architecture, pas une extension mécanique de celui-ci.
