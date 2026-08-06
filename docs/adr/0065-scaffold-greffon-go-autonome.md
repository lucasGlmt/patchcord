# ADR-0065 — Scaffold de greffon Go autonome

## Statut
Accepté

## Contexte

`patchcord plugin new` générait jusqu'ici un greffon minimal supposé vivre dans le monorepo Patchcord, sans `go.mod` propre, comme les greffons d'exemple sous `plugins/examples/`. Ce modèle est pratique pour les briques de référence embarquées, mais il n'est pas adapté au modèle de distribution attendu pour les greffons tiers : un dossier par greffon, publiable tel quel dans un dépôt git et installable sans dépendre d'un checkout Patchcord local.

## Décision

`patchcord plugin new <id>` génère désormais par défaut un projet Go autonome pour un seul greffon : `go.mod`, `main.go`, `manifest.json`, `README.md`, `Makefile` et `.gitignore`. Le greffon généré importe uniquement le SDK public `github.com/lucasglmt/patchcord/sdk/go-plugin`; il n'importe aucun package `internal/` du core.

Le chemin de module Go vaut l'identifiant du greffon par défaut. La commande accepte `--module` pour écrire un chemin de module différent lorsque le dépôt git cible utilise une URL d'import différente de l'identifiant Patchcord. Le manifeste de package conserve l'identifiant Patchcord comme source de vérité pour `plugin pack` et `plugin install`.

Les greffons d'exemple du monorepo restent inchangés : ils continuent de servir de références internes et de source pour les greffons embarqués.

## Conséquences positives

- Un développeur peut créer un greffon dans un dossier qui devient directement la racine d'un dépôt git.
- Le scaffold reflète la frontière publique voulue : dépendance au SDK Go et au protocole public, jamais au core interne.
- Le flux `plugin new` → `go build` → `plugin pack` → `plugin install` fonctionne hors du monorepo.
- `--module` sépare proprement l'identité Patchcord du chemin d'import Go.

## Conséquences négatives

- Le premier `go mod tidy` d'un greffon scaffoldé doit résoudre le module Patchcord publié, sauf si le développeur ajoute un `replace` local.
- Les tests du scaffold doivent injecter un `replace` local pour éviter de dépendre du réseau ou d'une publication distante.
- Les greffons d'exemple embarqués et les greffons scaffoldés n'ont plus exactement la même forme de projet.
