# ADR-0027 — `app dev` et le format de package `.patchcord-app`

## Statut
Accepté

## Contexte

ADR-0026 a posé la tranche minimale du pan « applications » (manifeste,
`internal/apps`, sessions limitées, hébergement statique) mais laissait
explicitement deux points hors scope, listés comme la dernière étape non
faite de la phase 5 (CLAUDE.md §9) :

- **`patchcord app dev`** — un cycle d'itération rapide pendant le
  développement, sans la friction de `app remove` avant chaque
  `app install` quand on retravaille la même application ;
- **un vrai format de package `.patchcord-app`** — le document de vision
  (§9.3) le décrit comme « interface web statique et manifeste de
  permissions », mais `Install` ne fait aujourd'hui que pointer
  `static_dir` vers un répertoire source qui doit rester en place
  indéfiniment.

## Décision

**`app dev` est un upsert, pas un nouveau serveur de développement.**
`handleServeApp` (`internal/api/apps.go`) sert déjà `static_dir` via
`http.FileServer` en lisant le disque à chaque requête — une application
déjà installée bénéficie donc déjà d'un rechargement à chaud gratuit dès
que son répertoire est reconstruit (`vite build --watch`, par exemple),
sans redémarrage de l'agent. Le seul obstacle réel était que `Install`
refuse un id déjà présent (`ErrAlreadyExists`, comportement volontaire
d'ADR-0026, conservé pour `app install`). `apps.InstallOrUpdate`
(`internal/apps/apps.go`) ajoute un second chemin qui fait un
`INSERT ... ON CONFLICT(id) DO UPDATE` au lieu d'échouer ; `app dev <dir>`
l'utilise. Le core ne prend pas en charge un bundler ou un serveur HMR —
ce serait une capacité métier concrète dans `internal/`, contraire au
non-négociable #3 ; un vrai rechargement à chaud du JavaScript reste le
travail du serveur de dev de Vite/React/etc., lancé séparément par le
développeur.

**Le format `.patchcord-app` est un tar.gz, produit et lu avec la
bibliothèque standard.** `apps.Pack(sourceDir, w io.Writer)`
(`internal/apps/package.go`) archive un répertoire validé par
`LoadManifest` en tar compressé gzip — fichiers réguliers et répertoires
seulement, tout autre type d'entrée (lien symbolique, etc.) fait échouer
l'empaquetage plutôt que de produire une archive silencieusement
incomplète. Aucune dépendance externe ajoutée : `archive/tar` et
`compress/gzip` suffisent, cohérent avec le choix déjà fait pour YAML
(`gopkg.in/yaml.v3`, seule dépendance non triviale d'`internal/apps`).

**`InstallPackage` extrait sous un répertoire géré par l'agent, jamais dans
un répertoire temporaire du système.** `dataDir/apps/<id>/<version>/`
(nouveau sous-répertoire de `dataDir`, jusqu'ici réservé à la base SQLite)
héberge le contenu extrait. L'extraction passe par un répertoire de
staging (`dataDir/apps/.staging-*`, `os.MkdirTemp` sur le même système de
fichiers que la cible) puis un `os.Rename` atomique — éviter un
`os.MkdirTemp` par défaut (typiquement `/tmp`) qui casserait ce rename sur
un système de fichiers différent. Contrairement à `Install` sur un
répertoire source, dont la disponibilité continue dépend de l'appelant, le
fichier `.patchcord-app` original n'a plus besoin d'exister après
`install` : Patchcord en possède désormais sa propre copie extraite.

**Une archive est une entrée non fiable : protection systématique contre
la traversée de chemin (« zip slip »).** Chaque entrée de l'archive est
résolue via `safeJoin`, qui rejette toute cible hors du répertoire de
staging avant d'écrire quoi que ce soit sur disque — cohérent avec la
consigne sécurité de CLAUDE.md (OWASP top 10 : path traversal). Testé
explicitement (`TestExtractPackage_RejectsPathTraversal`).

**`app install` accepte les deux formes, distinguées par `os.Stat`, pas
par l'extension de fichier.** Un répertoire suit le chemin `Install`
existant (inchangé, static_dir = le répertoire lui-même) ; un fichier
suit `InstallPackage`. `.patchcord-app` reste une convention de nommage
documentée (`apps.PackageExtension`, utilisée par défaut par `app pack`
pour choisir un nom de sortie), pas une vérification imposée à
l'installation — cohérent avec le reste du dépôt, où le format d'un
contenu ne se devine jamais depuis son extension (le protocole de
greffons, par exemple, ne dépend d'aucune extension de binaire).

## Explicitement hors scope (différé, pas oublié)

- Signature et vérification de package (`patchcord-plugin`/`bundle` du
  document de vision, phase 7 — écosystème).
- Mise à jour d'une application déjà installée depuis un nouveau package
  (`app install` d'un `.patchcord-app` avec un id déjà présent reste
  rejeté par `ErrAlreadyExists`, comme pour un répertoire — seul `app dev`
  fait un upsert, et uniquement pour des répertoires).
- Nettoyage des versions précédentes d'une même application sous
  `dataDir/apps/<id>/` : chaque version installée occupe son propre
  sous-répertoire, `app remove` ne supprime aujourd'hui que
  l'enregistrement en base, pas les fichiers extraits — dette assumée,
  cohérente avec l'absence de nettoyage déjà documentée pour les greffons.
- Rechargement à chaud du JavaScript côté navigateur (HMR) — reste le rôle
  du serveur de développement de l'outillage frontend choisi par
  l'application, pas de l'agent.

## Conséquences positives

- Ferme les deux derniers points de la phase 5 (CLAUDE.md §9) restés hors
  scope d'ADR-0026 ; la phase 5 du document de vision (§19) est
  maintenant complète.
- `app dev` ne duplique aucune logique : il réutilise `LoadManifest` et le
  même chemin d'écriture SQL qu'`Install`, seule la clause `ON CONFLICT`
  change.
- Le format de package n'introduit aucune dépendance externe et reste
  lisible avec des outils standards (`tar xzf`) en cas de besoin de
  débogage manuel.

## Conséquences négatives

- `dataDir/apps/` accumule une copie par version installée d'une
  application packagée, sans purge automatique — un usage prolongé avec
  des mises à jour fréquentes finira par consommer de l'espace disque
  inutilement (cf. hors scope ci-dessus).
- Le format `.patchcord-app` n'est pas signé : rien n'empêche
  aujourd'hui l'installation d'un package altéré ou malveillant au-delà
  de la protection contre la traversée de chemin — la vérification de
  provenance reste un problème de la phase 7 (écosystème).
