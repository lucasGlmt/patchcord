# ADR-0042 — Formats de package `.patchcord-plugin`, `.patchcord-workflow`, `.patchcord-bundle`

## Statut
Accepté

## Contexte

La phase 6 (déploiement serveur) est terminée ; la phase 7 (écosystème,
CLAUDE.md §9) démarre par sa première tâche : les formats de package. Les
tâches suivantes de la phase (signature, vérification, registre, mise à
jour) en dépendent toutes directement.

État du dépôt avant cette décision :

- `.patchcord-app` existait déjà en entier (ADR-0027) : archive tar.gz,
  installation via un répertoire de staging + rename atomique sous
  `dataDir`, protection anti path-traversal. C'est le seul format déjà
  éprouvé en code.
- `.patchcord-plugin` n'existait pas du tout : `plugin install <path>`
  prenait un chemin vers un exécutable brut et se contentait d'un
  `Launch` + `Handshake` RPC (`internal/plugins`). Aucun manifeste
  statique, aucun format d'archive, aucun support multi-plateforme.
- `.patchcord-workflow` n'avait presque rien à construire : le document de
  vision (§9.3) le décrit comme « définition déclarative seule » — c'était
  déjà exactement le fichier YAML que `workflow install` consomme.
- `.patchcord-bundle` n'existait pas.

## Décision

**Un nouveau paquet interne `internal/packaging`** porte désormais les
primitives d'archive tar.gz génériques (`Archive`, `Extract`, `SafeJoin`),
extraites du code d'`internal/apps/package.go` qui les dupliquait
implicitement. `internal/apps`, `internal/plugins` et le nouveau
`internal/bundles` s'appuient tous les trois dessus — la dupliquer une
troisième fois aurait été la vraie erreur. `Extract` écrit systématiquement
les fichiers avec le mode `0o644`, sans jamais faire confiance au mode
déclaré dans le header tar (une archive est une entrée non fiable) ; le
seul appelant qui a besoin d'un fichier exécutable
(`plugins.InstallPackage`) fait un `os.Chmod(0o755)` explicite sur le seul
binaire sélectionné après extraction.

**`.patchcord-plugin`** ajoute un manifeste statique `manifest.json`
(`internal/plugins/manifest.go`), déclaré avant tout lancement de process,
distinct du manifeste RPC retourné par le handshake
(`internal/plugins/handshake.go`) qui reste la source de vérité une fois le
greffon effectivement lancé. Structure simplifiée par rapport à l'exemple
illustratif du document de vision (§9.1) : pas de champ `name` (aucun autre
manifeste du dépôt n'en a), pas de wrapper `runtime: {type: "process", ...}`
(toujours `"process"` par ADR-0002, un champ constant n'apportant rien tant
qu'un second type de runtime n'existe pas). `executables` reste à plat :
`{"darwin-arm64": "binaries/darwin-arm64/plugin", ...}`. `permissions` y est
déclaré pour être affiché avant le lancement du process (vision §9.2, étape
5). `plugin install <path>` distingue une archive d'un exécutable brut en
sniffant les deux octets magiques du gzip (`0x1f 0x8b`) plutôt que par
extension — cohérent avec le principe déjà posé par ADR-0027 (« le format
d'un contenu ne se devine jamais depuis son extension »). Le chemin actuel
(exécutable brut, utilisé par le greffon de référence `text.uppercase@1`)
reste inchangé et testé pour non-régression.

**`.patchcord-workflow`** reste une pure convention de nommage : aucune
nouvelle mécanique. `workflow install <path.yaml>` acceptait déjà n'importe
quel chemin sans regarder l'extension. `workflow.FileExtension` est ajouté
comme constante documentaire, et `workflow export` gagne un flag
`-o/--output` pour symétrie avec `app pack`/`plugin pack -o`.

**`.patchcord-bundle`**, nouveau paquet `internal/bundles`, sur le modèle
d'`internal/apps` : un `bundle.yaml` déclare un id/version, un sous-répertoire
`app` optionnel, une liste de fichiers `workflows`, et une liste
`requires_plugins` de dépendances `id@version`. `InstallPackage` orchestre,
dans l'ordre : (1) vérifie que chaque dépendance greffon est déjà installée
à la version exacte demandée — **pas** d'installation automatique, ce sera
le rôle des tâches registre/mise à jour, plus tard dans la phase ; (2)
déplace le sous-répertoire `app` extrait vers son emplacement définitif
(`dataDir/apps/<app-id>/<app-version>`) puis appelle `apps.Install` dessus,
la même chorégraphie qu'`apps.InstallPackage` mais appliquée à un
répertoire déjà extrait plutôt qu'à une archive fraîche ; (3) installe
chaque workflow embarqué via `runs.InstallWorkflow`, sans copie sur disque
— un workflow n'a besoin d'aucun domicile propre, il vit dans
`workflow_versions` (ADR-0008). Une nouvelle table `bundles` (migration
`0008_bundles.sql`) enregistre uniquement la provenance (id, version,
manifeste YAML brut) — elle ne duplique pas ce qu'`apps`/`workflow_versions`
savent déjà sur leur propre contenu, même choix « ne pas sur-normaliser »
que `workflow_versions.definition`.

## Explicitement hors scope (différé, pas oublié)

- Signature et vérification de package (checksums.json/signature.json du
  document de vision, §9.1) — prochaine tâche de la phase 7.
- Installation automatique des dépendances greffons manquantes d'un
  bundle — dépend du registre, tâche ultérieure.
- « Configuration » d'un bundle (embarquer des connecteurs, §9.3) —
  bloquée par l'absence totale de mécanisme d'export/template de connecteur
  aujourd'hui (`internal/connectors` est purement SQLite, ADR-0020) ; il n'y
  a rien qu'un bundle pourrait porter au-delà d'un id/type non secret sans
  d'abord concevoir ce mécanisme.
- Rollback multi-ressources d'un bundle partiellement installé (app
  installée mais un workflow échoue derrière, par exemple) — ce premier
  passage n'implémente pas de transaction à travers trois catalogues
  indépendants ; l'erreur nomme l'étape qui a échoué.
- Synchronisation des triggers (`scheduler.Sync`) pour un workflow embarqué
  dans un bundle — seul `runs.InstallWorkflow` est appelé, pas
  `scheduler.Sync` ; un workflow bundlé avec un trigger `schedule`/`webhook`
  n'est donc pas automatiquement programmé.
- `bundle remove`/nettoyage des versions précédentes — dette déjà assumée
  ailleurs (ADR-0027) pour les mêmes raisons.
- Compilation croisée multi-plateforme d'un greffon (remplir
  `binaries/<GOOS>-<GOARCH>/`) — reste un problème de tooling/Makefile pour
  le développeur du greffon ; `plugin pack` archive ce qui existe déjà.

## Conséquences positives

- Comble le vrai chaînon manquant de la phase 7 : `.patchcord-plugin`
  existe maintenant en code, avec support multi-plateforme et sélection du
  bon exécutable à l'installation (vision §9.2, étape 7).
- `internal/packaging` élimine une duplication qui serait devenue une
  troisième copie de la même logique de sécurité (anti zip-slip) ; un
  correctif futur de cette logique ne se fait qu'à un seul endroit.
- Chaque nouveau format réutilise au maximum les mécanismes existants
  (`apps.Install`, `runs.InstallWorkflow`, `plugins.Install`) plutôt que de
  les redéfinir — un bundle n'introduit aucune nouvelle façon d'installer
  une app ou un workflow.
- Vérifié de bout en bout avec le greffon et les workflows de référence :
  `plugin pack` → `plugin install` (package et exécutable brut), et
  `bundle pack` → `bundle install` avec une app et un workflow embarqués,
  le workflow bundlé s'exécutant réellement contre le greffon dépendance.

## Conséquences négatives

- `dataDir/plugins/` et `dataDir/bundles/` accumulent une copie par version
  installée, sans purge automatique — même dette déjà documentée pour
  `dataDir/apps/` (ADR-0027).
- Aucun de ces trois formats n'est signé ni vérifié : rien n'empêche
  aujourd'hui l'installation d'un package altéré ou malveillant au-delà de
  la protection contre la traversée de chemin.
- Un bundle qui échoue à mi-chemin (app installée, workflow en échec) laisse
  un état partiellement appliqué, sans rollback ni message unique
  récapitulatif — juste une erreur nommant l'étape fautive.
