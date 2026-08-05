# ADR-0059 — Greffons de référence embarqués et auto-installés

## Statut
Accepté

## Contexte

`patchcord plugin install ./text` enregistrait le chemin de l'exécutable
tel quel dans le catalogue (`internal/plugins/catalog.go`), relatif au
répertoire courant au moment de l'`install`. Le Plugin Supervisor relance
chaque greffon du catalogue à chaque `patchcord serve`/`workflow run`
(`internal/plugins/supervisor.go`), depuis le répertoire courant de *cette*
commande — presque jamais celui de l'`install` d'origine. Un greffon
installé, par exemple, depuis un checkout de `patchcord_core` puis exécuté
depuis un répertoire de bundle applicatif (`patchcord_bundles/apptest`)
échouait donc à se relancer, silencieusement : `launchAndHandshake`
consigne l'échec et continue plutôt que de faire tomber l'agent (non-négociable
#7), si bien que `plugin list` continuait d'afficher le greffon comme
installé pendant que `ExecuteAction` échouait avec « action ... is not
currently available ». Ce bug de chemin relatif est corrigé séparément
(`internal/plugins/catalog.go` résout désormais un chemin absolu à
l'installation) — voir le commit correspondant pour ce correctif isolé.

En creusant la cause racine avec Lucas, la vraie friction est apparue
en amont du bug lui-même : un agent fraîchement installé n'a *aucun*
greffon, donc aucune action, tant que l'utilisateur n'a pas manuellement
localisé et installé des exécutables de greffons — ce que même la
tranche verticale de référence du document de vision (section 20,
`text.uppercase@1`) suppose déjà fait. Lucas veut que les greffons « de
base » soient utilisables sans étape d'installation manuelle.

Deux lectures possibles de cette demande, aux conséquences très
différentes sur les non-négociables du projet (`CLAUDE.md`, section 1) :

1. **Lier les greffons statiquement dans le binaire du core** (imports Go
   directs, plus de processus séparé). Rejeté d'emblée : contredit #2 (le
   core doit fonctionner sans logique métier concrète en son sein), #3
   (« pas de plugin natif Go `.so` » — définition même d'un greffon,
   section 3), et #7 (un crash de greffon ne doit jamais faire tomber
   l'agent — impossible à garantir sans isolation de processus).
2. **Continuer d'exécuter les greffons comme des processus RPC
   indépendants et supervisés**, mais supprimer l'étape manuelle de
   localisation + `plugin install`.

Seule la seconde lecture est compatible avec l'architecture. Reste à
choisir *quels* greffons rendre disponibles par défaut : `plugins/examples/`
en contient huit, dont trois (`mysql`, `postgresql`, `openai`) sont des
intégrations à un service métier concret. En embarquer l'exécutable dans
le binaire de l'agent, même comme processus séparé, revient à faire du
core quelque chose qui « connaît » MySQL/PostgreSQL/OpenAI par défaut à
l'installation — en tension avec l'esprit du non-négociable #3, même si la
frontière RPC elle-même n'est pas franchie. Les cinq autres
(`text`, `json`, `encoding`, `http`, `time`) sont des briques génériques
sans service métier concret derrière — leur embarquement ne pose pas ce
problème.

## Décision

1. Patchcord embarque, via `go:embed`, les exécutables précompilés de cinq
   greffons de référence — `text`, `json`, `encoding`, `http`, `time`
   (`internal/plugins/embedded/`) — un jeu par plateforme cible
   (`linux`/`darwin`/`windows` × `amd64`/`arm64`), sélectionné à la
   compilation via des fichiers à contrainte de build
   (`platform_<goos>_<goarch>.go`). `mysql`, `postgresql` et `openai`
   restent à installer explicitement via `plugin install`, comme
   aujourd'hui.

2. Ces exécutables ne sont **jamais liés en mémoire du core** : ce sont des
   binaires autonomes, octet pour octet identiques à ceux que
   `plugin install` installerait manuellement, toujours lancés et
   supervisés comme des processus séparés parlant le protocole RPC des
   greffons (`internal/plugins.Supervisor`). Rien ne change à ce que
   *sont* les greffons — seule l'étape de « les trouver et les installer »
   disparaît.

3. `internal/plugins.SeedEmbedded(ctx, db, dataDir, logger)` installe ces
   exécutables embarqués dans le catalogue **une seule fois** par
   répertoire de données (`--data-dir`) : un indicateur dans une nouvelle
   table `agent_meta` (clé/valeur générique, `migrations/0011_agent_meta.sql`)
   marque le répertoire comme « déjà amorcé ». Un greffon embarqué que
   l'utilisateur a désinstallé (`plugin uninstall`) reste désinstallé —
   `SeedEmbedded` ne le réinstalle jamais de force, et n'effectue non plus
   aucune mise à jour automatique d'un greffon déjà présent (évolution
   possible, non traitée ici).

4. `internal/cli.openDataStore` — le point de passage commun à toute
   commande CLI qui touche la base de données — appelle `SeedEmbedded`
   juste après la migration. C'est le seul point d'accroche nécessaire :
   `patchcord serve`/`dev` (`internal/runtime.NewAgent`, qui n'utilise pas
   `openDataStore` et amorce donc explicitement lui aussi) tout comme
   `workflow install`, `workflow validate`, `workflow run`,
   `connector test`, `bundle install`, etc. voient alors le même
   catalogue amorcé, sans avoir à connaître individuellement l'existence
   de ce mécanisme.

5. `make build-embedded-plugins` compile les cinq greffons de référence
   pour `GOOS`/`GOARCH` (hôte par défaut) dans
   `internal/plugins/embedded/bin/<goos>_<goarch>/` — jamais commité
   (`.gitignore`), un `.gitkeep` par répertoire de plateforme suffit à
   garder la directive `go:embed` valide sur un checkout neuf (elle
   embarque alors un ensemble vide de greffons — `SeedEmbedded` traite
   « rien à amorcer » comme un cas normal, jamais une erreur). `make build`
   en dépend, donc un `make build`/`make check` local produit un binaire
   « batteries incluses » sans étape supplémentaire. La chaîne de release
   (`.goreleaser.yaml`) invoque cette même cible en `hooks.pre` par cible
   de plateforme, avec `GOOS`/`GOARCH` positionnés depuis les variables de
   template de goreleaser, pour que chaque binaire publié embarque les
   exécutables de sa propre plateforme.

## Conséquences positives

- Un agent fraîchement installé exécute `text.uppercase@1` (et les autres
  actions des cinq greffons de référence) sans aucune étape manuelle — la
  tranche verticale de référence du document de vision (section 20)
  fonctionne réellement dès l'installation.
- L'isolation de processus, la supervision et le protocole RPC restent
  entièrement intacts (non-négociables #2, #3, #7) : un greffon embarqué
  crashe exactement comme un greffon installé manuellement, sans jamais
  affecter l'agent.
- `plugin uninstall` reste la source de vérité : aucune réinstallation
  silencieuse d'un greffon que l'utilisateur a explicitement retiré.
- Le mécanisme est testable sans dépendre d'exécutables réels ni de
  process externes (`internal/plugins.listEmbeddedFiles` est substituable
  en test — cf. CLAUDE.md section 5) ; `internal/plugins/embedded.Files()`
  ne retourne jamais d'erreur en l'absence d'exécutables construits, donc
  `go build ./...`/`go test ./...` restent utilisables directement sur un
  checkout neuf sans dépendre de `make`.

## Conséquences négatives

- Le binaire `patchcord` grossit sensiblement (cinq exécutables
  embarqués, ~75 Mo supplémentaires par plateforme sur les mesures
  actuelles) — accepté comme le coût direct de « batteries incluses »
  pour un agent local-first.
- `mysql`/`postgresql`/`openai` restent à installer manuellement ; un
  futur exemple de greffon de référence "générique" ajouté à
  `plugins/examples/` n'est pas automatiquement embarqué — la liste dans
  `Makefile`/`internal/plugins/embedded` doit être étendue à la main, une
  décision au cas par cas plutôt qu'une règle générique par répertoire.
- Aucune mise à jour automatique d'un greffon déjà installé lors d'une
  mise à jour de l'agent vers une version qui embarque une version plus
  récente du même greffon de référence — évolution possible, non traitée
  par cet ADR.
- La chaîne de release dépend de `make` (déjà le cas indirectement via
  `make check` en CI, mais désormais aussi dans le `hooks.pre` de
  `.goreleaser.yaml` lui-même) — un changement de nom de cible Makefile
  casserait silencieusement l'embarquement tant que `goreleaser check`
  n'est pas relancé.
