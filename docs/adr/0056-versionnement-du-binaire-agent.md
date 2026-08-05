# ADR-0056 — Versionnement et publication du binaire agent

## Statut
Accepté

## Contexte

Le dépôt est public sur GitHub depuis le début, mais jusqu'ici tout dev se
résume à `make build-all` en local — aucun tag git, aucun CHANGELOG, aucune
CI, aucune version embarquée dans le binaire (`patchcord version`
n'existait pas). Le mot « version » est déjà utilisé ailleurs dans le
projet avec un sens précis et différent : version du protocole de greffon
(`internal/plugins.CurrentProtocolVersion`), `schema_version` des
workflows, version des packages/bundles (ADR-0042, ADR-0043, ADR-0055).
Rien de tout ça ne couvre la question posée ici, qui est distincte : quelle
version a *le binaire agent lui-même*, et comment cette version est
produite, exposée et publiée.

Lucas veut rendre le dépôt « pro » et « open-source ready ». Ça pose trois
questions indépendantes qu'il fallait trancher avant d'implémenter :
comment le binaire connaît sa propre version, comment l'historique des
changements est documenté, et si la publication d'une release est
automatisée ou manuelle.

## Décision

**Schéma de version** : SemVer (`vX.Y.Z`), cohérent avec le reste du
dépôt (bundles, packages) et avec l'écosystème Go. Le projet étant en
phase 1 (core minimal), les tags démarrent en `0.x.y` — pas d'engagement de
stabilité avant un `v1.0.0` explicite.

**Provenance de la version** : le tag git est la source de vérité. Le
binaire embarque `Version`/`Commit`/`Date` via `-ldflags -X` dans le
package `internal/version`, calculés par `git describe --tags --always
--dirty` (Makefile), par les build args du Dockerfile, ou par le contexte
de tag de goreleaser en CI — jamais une constante éditée à la main. Un
`go build`/`make build` hors contexte de tag retombe sur `"dev"`, ce qui
est le comportement attendu en développement local, pas une erreur.

**Exposition** : `patchcord version` (nouvelle sous-commande, forme longue
avec commit + date de build) et `--version`/`-v` sur la racine (forme
courte, via `cobra.Command.Version`). `/v1/system/health` gagne un champ
`version` — utile en opération pour savoir quelle version tourne sans
lancer la CLI sur la machine hôte.

**CHANGELOG** : généré depuis les commits Conventional Commits avec
[git-cliff](https://git-cliff.org) (`make changelog`, config `cliff.toml`),
committé dans `CHANGELOG.md`. L'historique pré-ADR-0056 est irrégulier
(`greffons`, `pligin`, `worflow sse`, `test`...) — `cliff.toml` route tout
ce qui ne matche aucun type connu vers un groupe `Other` plutôt que de le
faire disparaître silencieusement. À partir de maintenant, les commits sont
attendus au format Conventional Commits (`feat:`, `fix:`, `docs:`,
`refactor:`, `test:`, `chore:`...) pour que le CHANGELOG reste exploitable.

**Publication** : automatisée via GitHub Actions. `ci.yml` fait tourner
`make check` sur chaque push/PR — il n'existait aucune CI avant cet ADR,
ajoutée dans la foulée puisqu'un repo « pro » sans CI de base n'a pas de
sens. `release.yml` se déclenche sur un push de tag `v*` et délègue à
goreleaser (`.goreleaser.yaml`) : cross-compilation
linux/darwin/windows × amd64/arm64, archives, checksums, et création de la
GitHub Release avec ses propres notes de version groupées par type de
commit. Le flux de release reste : `make changelog` → commit → `git tag
vX.Y.Z` → `git push --tags` → CI prend le relai.

**Bug de portabilité Windows découvert et corrigé en préparant cette
release** : `internal/cli/dev.go` appelait
`syscall.Setpgid`/`syscall.Kill`/`syscall.SIGTERM` (gestion du groupe de
processus pour `patchcord dev`), qui n'existent pas sous `GOOS=windows` —
le binaire n'avait donc jamais compilé pour Windows, alors que
`docs/adr/0052-defaut-data-dir-dossier-standard-du-systeme.md` et le
document de vision présupposent déjà Windows comme plateforme cible
(`%LOCALAPPDATA%`, "service Windows"). La gestion du groupe de processus
est désormais isolée dans `internal/cli/dev_unix.go` (`!windows`, comportement
inchangé : `Setpgid` + `SIGTERM` au groupe entier) et
`internal/cli/dev_windows.go` (`windows`, repli sur `Process.Kill()` du
seul processus direct — pas d'équivalent de groupe de processus posé pour
l'instant, ce qui reste une limite connue sous Windows si `npm run dev`
exécute un enfant supplémentaire comme vite).

**Hors périmètre de cet ADR** : la publication de l'image Docker
(actuellement buildable via `make docker-build`, mais pas poussée vers un
registre par la CI) et celle des greffons d'exemple. Les deux ont leur
propre cycle de version potentiel et n'ont pas été demandées — pas
d'anticipation au-delà du besoin exprimé (cf. CLAUDE.md section 9).

## Conséquences positives

- Un utilisateur ou un rapport de bug peut identifier exactement quelle
  version du binaire tourne (`patchcord version`, ou `/system/health`),
  y compris en environnement conteneurisé.
- Le dépôt gagne sa première CI : toute régression sur `go vet`/`gofmt`/
  `go test ./...` est visible sur chaque PR, pas seulement au moment où
  quelqu'un pense à lancer `make check` localement.
- Les releases sont reproductibles et traçables (tag → build CI → binaires
  signés par leur checksum → GitHub Release), sans geste manuel de
  compilation croisée.
- Le CHANGELOG documente déjà, dès ce commit, l'historique complet du
  projet — pas seulement les changements futurs.
- Le binaire compile à nouveau pour Windows (régression latente corrigée
  au passage), cohérent avec ce que ADR-0052 et le document de vision
  attendaient déjà de cette plateforme.

## Conséquences négatives

- Impose une discipline Conventional Commits à partir de maintenant ; un
  commit mal formé atterrit dans le groupe `Other`, moins lisible.
- Ajoute une dépendance d'outillage locale (`git-cliff`, comme `mdbook` et
  `swag` déjà) pour qui veut régénérer le CHANGELOG avant de tagger.
- goreleaser et les deux workflows GitHub Actions sont une nouvelle surface
  à maintenir ; une release cassée (ex. changement incompatible dans
  `.goreleaser.yaml`) ne se découvre qu'au moment de pousser un tag, pas
  avant.
- L'image Docker et les greffons d'exemple restent non versionnés/publiés
  automatiquement — incohérence assumée pour l'instant (voir « Hors
  périmètre » ci-dessus), à revisiter si un besoin concret apparaît.
- Sous Windows, `patchcord dev` ne termine que le processus direct de
  `--app-dev-cmd`, pas ses éventuels descendants (contrairement à Unix, qui
  signale tout le groupe de processus) — un vrai équivalent demanderait un
  Job Object Windows, pas posé ici faute de besoin concret exprimé.
