# ADR-0066 — Modules Go imbriqués pour `sdk/go-plugin` et `api/plugin`

## Statut
Accepté

## Contexte

`patchcord plugin new` (ADR-0065) génère un greffon Go autonome, publiable
dans son propre dépôt git, qui importe uniquement
`github.com/lucasglmt/patchcord/sdk/go-plugin`. Avant cette décision,
`patchcord_core` était un seul module Go (`module
github.com/lucasglmt/patchcord` à la racine) : `sdk/go-plugin` n'était
qu'un sous-paquet de ce module, sans version propre.

Lucas a soulevé la question suivante : un greffon tiers, développé hors du
monorepo, doit-il consommer le SDK Go depuis un **dépôt Git séparé** pour
que l'import fonctionne proprement ?

La réponse est non — `go get`/`go mod tidy` résolvent un sous-chemin de
paquet dans n'importe quel module public sans dépôt dédié. Mais la question
révèle un vrai problème, pas une fausse inquiétude : avec un seul
`go.mod` racine, le `go.mod` d'un greffon communautaire affiche `require
github.com/lucasglmt/patchcord vX.Y.Z` — littéralement une dépendance vers
l'agent entier — et chaque tag du core (même pour un changement purement
interne au scheduler ou à la persistence) fait apparaître une "nouvelle
version" côté greffon tiers, sans que le contrat qu'il consomme ait changé.
C'est exactement ce que le non-négociable §1.5 de CLAUDE.md/AGENTS.md
interdit : *"les frontières publiques sont contractuelles… versionnées.
L'intérieur du core peut changer librement ; ces trois frontières non."*
`sdk/go-plugin` et le protocole qu'il enveloppe (`api/plugin`) sont deux de
ces trois frontières ; elles doivent donc pouvoir évoluer et se tagger
indépendamment du reste du core.

C'est le même problème, en substance, que celui déjà tranché pour le SDK
TypeScript par l'ADR-0050 : `sdk/typescript` est resté dans le monorepo
mais publié sur npm indépendamment plutôt que consommé en `file:`. Go n'a
pas d'équivalent direct à `npm publish` — l'équivalent outillé est le
**module Go imbriqué**, taggé séparément dans le même dépôt git.

**Contrainte technique rencontrée en cours de route :** un chemin de module
Go ne peut jamais se terminer par `/v1` littéral — Go réserve les suffixes
`/vN` (N ≥ 2) à l'import versioning sémantique et rejette explicitement
`/v1` comme suffixe de chemin de module (`module.CheckPath`). Le protocole
vit sous `api/plugin/v1/` ; le module ne peut donc pas être
`.../api/plugin/v1`, seulement `.../api/plugin`, avec `v1` comme simple
sous-paquet à l'intérieur. Le chemin d'import du code (`.../api/plugin/v1`)
ne change pas — seul l'emplacement du `go.mod` remonte d'un cran. La même
contrainte s'appliquera le jour où `api/app/v1` recevra le même
traitement.

## Décision

`api/plugin` et `sdk/go-plugin` deviennent chacun un **module Go
imbriqué** du monorepo, avec leur propre `go.mod`, sous le même chemin
d'import qu'avant (`github.com/lucasglmt/patchcord/api/plugin`,
`github.com/lucasglmt/patchcord/sdk/go-plugin`). Ils seront taggés
indépendamment du module racine avec la convention Go des modules
imbriqués : `api/plugin/vX.Y.Z`, `sdk/go-plugin/vX.Y.Z`.

Un fichier `go.work` à la racine du dépôt (`use . ; use ./api/plugin ; use
./sdk/go-plugin`) fait travailler les trois modules ensemble en local :
`go build ./...`, `go vet ./...`, `go test ./...` depuis la racine
continuent de fonctionner tels quels (CLAUDE.md/AGENTS.md §10), sans qu'un
contributeur ait besoin de configuration manuelle. `go.work` et
`go.work.sum` sont committés.

Tant qu'aucun tag n'existe encore pour `api/plugin` et `sdk/go-plugin`,
leurs `go.mod` respectifs pointent vers une version placeholder
(`v0.0.0-00010101000000-000000000000`, la convention Go pour "aucune
version publiée") et le module racine, ainsi que `sdk/go-plugin`
lui-même pour sa dépendance à `api/plugin`, portent une directive
`replace` locale vers le checkout du monorepo. Une directive `replace` du
module racine ne s'applique jamais aux consommateurs externes qui
importent `api/plugin` ou `sdk/go-plugin` directement — elle n'affecte que
la construction du module qui la déclare (`go.mod`, spécification du
langage Go).

Le scaffold (`patchcord plugin new`, `internal/plugins/scaffold.go`) et ses
tests (`TestScaffold_GeneratedSourceCompiles`,
`TestPluginNewCommand_ThenPackThenInstall`) documentent/injectent
désormais deux directives `replace` (une par module imbriqué) plutôt
qu'une seule vers le module racine, pour un greffon testé contre un
checkout local avant la première publication de tag.

**Étapes de mise en production différées, pas oubliées :** le premier tag
réel doit être coupé dans l'ordre `api/plugin/vX.Y.Z` puis
`sdk/go-plugin/vX.Y.Z` (son `go.mod` doit alors pointer vers ce tag réel de
`api/plugin`, `replace` retiré) — un geste manuel de Lucas, comme le
`npm publish` de l'ADR-0050, hors du périmètre qu'un agent exécute sans
confirmation explicite.

## Conséquences positives

- Un greffon communautaire externe ne dépend plus, dans son `go.mod`, que
  de `sdk/go-plugin` (et transitivement `api/plugin`) — jamais du module
  racine `github.com/lucasglmt/patchcord` dans son ensemble.
- La version de `sdk/go-plugin`/`api/plugin` évolue indépendamment des tags
  du core : un changement interne au scheduler, à la persistence ou au
  moteur de workflows ne force plus une "nouvelle version" perçue côté
  greffon tiers.
- Concrétise le non-négociable §1.5 pour la frontière du protocole de
  greffons, dans le même esprit que l'ADR-0050 pour le SDK TypeScript.
- `go build ./...` / `go vet ./...` / `go test ./...` depuis la racine
  continuent de fonctionner sans configuration manuelle grâce à `go.work`.

## Conséquences négatives

- Trois `go.mod` (+ `go.sum`) à maintenir dans le dépôt au lieu d'un seul ;
  un contributeur qui ajoute une dépendance à `api/plugin` ou
  `sdk/go-plugin` doit le faire depuis le bon répertoire.
- Avant le premier tag réel de `api/plugin`/`sdk/go-plugin`, ces deux
  modules dépendent d'une version placeholder + `replace` locale — un état
  transitoire qui doit être nettoyé au moment de la première publication,
  sinon un consommateur externe sans `go.work` ne peut pas résoudre la
  dépendance.
- Deux commandes `go mod edit -replace` au lieu d'une dans le scaffold et
  ses tests, pour tester un greffon contre un checkout local avant tag.
- `api/plugin` ne peut pas s'appeler `api/plugin/v1` (contrainte Go sur les
  suffixes `/vN`) — une asymétrie entre le chemin du module Go et le
  dossier `v1/` qu'il contient, à anticiper si `api/app/v1` reçoit un jour
  le même traitement.
