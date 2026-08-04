# ADR-0044 — Registre de packages et mise à jour de bundle

## Statut
Accepté

## Contexte

La phase 7 (écosystème, CLAUDE.md §9) reprend ses deux dernières tâches de
roadmap : « registre » et « mise à jour ». Les tâches précédentes de la
phase les ont explicitement différées :

- ADR-0042 (formats de package) : l'installation automatique des
  dépendances greffons manquantes d'un bundle « dépend du registre, tâche
  ultérieure » ;
- ADR-0043 (signature et vérification) : la distribution des clés
  publiques par un tiers de confiance est « tâche « registre » de la phase
  7, plus tard ».

Cette décision ferme le premier de ces deux points (résolution d'un
package par identifiant) et livre la « mise à jour » pour les bundles — la
distribution de clés via le registre reste hors scope (voir plus bas).

Contrainte de conception, rappelée par CLAUDE.md §1.9 et les non-objectifs
de section 1 (pas de marketplace, pas de SaaS multi-tenant) : le cloud ne
doit jamais être requis pour démarrer ou utiliser l'agent. Un « registre »
ne peut donc être qu'un mécanisme entièrement optionnel et utilisable
hors-ligne — pas un serveur applicatif Patchcord.

Un bug concret bloquait toute mise à jour de bundle avant cette décision :
`internal/bundles/package.go`'s `installEmbeddedApp` appelait `apps.Install`
(strict — échoue avec `apps.ErrAlreadyExists` si l'id de l'app est déjà
enregistré), alors que `internal/bundles/bundles.go`'s `record()` upsert
déjà la ligne de provenance du bundle à chaque installation (son
commentaire : « Re-installing the same bundle id ... replaces it »).
Réinstaller un bundle déjà installé et embarquant une app échouait donc
systématiquement, en contradiction avec l'intention déjà documentée du
package. `apps.InstallOrUpdate` existait déjà (`internal/apps/apps.go`),
utilisé jusqu'ici uniquement par `app dev`.

Autre observation : `grep -rn "net/http" internal/` ne montre, avant cette
décision, que des usages entrants (serveur HTTP/SSE/WebSocket) — résoudre
un package depuis un registre http(s) est donc la première capacité
réseau *sortante* du core. Cela ne contredit pas le non-négociable §1.3
(le core ne connaît aucun service métier concret) : l'emplacement du
registre est une adresse générique configurée par l'utilisateur, exactement
sur le modèle de confiance de `trust add`.

## Décision

**Nouveau paquet `internal/registry`**, sur le modèle d'`internal/trust` :
une table `registries` (migration `0010_registries.sql`) enregistre des
registres configurés par nom, chacun pointant soit vers un répertoire
local, soit vers une URL `http(s)://` servant un `index.json` statique
plus les fichiers de package eux-mêmes — aucun serveur de registre
applicatif, aucune authentification, aucune commerce. `Add`/`List`/`Remove`
reproduisent l'upsert-on-conflict de `trust.Add`.

`index.json` déclare, par id de package, un `kind` (`plugin`/`app`/
`workflow`/`bundle`, vocabulaire déjà fixé par CLAUDE.md §3 et le document
de vision §9.3), une version `latest` explicite, et une table
`version -> chemin relatif`. Aucune comparaison sémantique de version
(pas de dépendance semver) : c'est cohérent avec le reste du dépôt, où
`bundles.splitPluginDependency` compare déjà les versions par égalité de
chaîne exacte — `latest` est une déclaration de l'auteur de l'index,
jamais recalculée par le client.

`registry.Resolve(ctx, db, id, version)` interroge les registres configurés
dans leur ordre d'ajout ; le premier registre dont l'index liste `id`
l'emporte. Trois cas sont distingués explicitement (documentés sur
`Resolve`) :

- un registre dont l'index est illisible échoue immédiatement, en nommant
  ce registre — jamais ignoré silencieusement au profit d'un registre
  suivant qui fonctionnerait, ce qui masquerait une vraie erreur de
  configuration ;
- un registre lu avec succès mais qui ne liste pas `id` n'est pas une
  erreur — `Resolve` passe au registre suivant ;
- une fois `id` trouvé dans un registre, une version demandée absente de
  cet index échoue immédiatement (`ErrUnknownVersion`), sans chercher cette
  version ailleurs — jamais de mélange de sources pour un même id.

`registry.Fetch` télécharge/copie le package résolu dans un fichier
temporaire ; le cas répertoire local réutilise
`internal/packaging.SafeJoin` (aucune logique anti-traversée dupliquée) ;
le cas http(s) ajoute une vérification équivalente sur le chemin relatif
de l'URL (sémantique `path`, jamais `filepath`).

**Correctif** : `installEmbeddedApp` appelle désormais
`apps.InstallOrUpdate` au lieu d'`apps.Install`. Le flux standalone
`.patchcord-app` (`apps.InstallPackage`) reste volontairement strict — seul
le choix pour l'app *embarquée dans un bundle* change, pour rester cohérent
avec le choix déjà fait par `record()` au niveau du bundle lui-même. Cette
décision précise donc, sans les contredire, ADR-0042 et ADR-0043 : la
phrase d'ADR-0043 « `bundles.installEmbeddedApp` continue d'appeler
`apps.Install` » décrivait un choix qui n'était pas une garantie
d'immutabilité voulue, seulement l'état du code à ce moment — la garantie
réelle qu'ADR-0043 protège (l'app embarquée n'est jamais re-vérifiée
séparément) reste entièrement intacte.

**CLI** : nouveau groupe `patchcord registry add/list/remove <name>
<location>`, sur le modèle de `patchcord trust`. `bundle install
<path-or-ref>` accepte désormais, en plus d'un chemin de fichier existant,
une référence `id` ou `id@version` résolue contre les registres
configurés. Nouvelle commande `patchcord bundle update <id>[@version]` :
exige que `id` soit déjà installé (sinon, erreur invitant à `bundle
install`), résout sa dernière version (ou la version épinglée) via le
registre, et ne réinstalle que si la version résolue diffère de la version
installée (sinon : message « already up to date », sortie sans erreur).

## Conséquences positives

- Ferme l'exemple aspirationnel du document de vision §9.2 (`patchcord
  plugin install io.patchcord.postgresql@1.0.0`, pour l'instant seulement
  côté CLI pour les bundles — voir hors scope).
- Débloque réellement la mise à jour de bundle de bout en bout : test de
  régression dans `internal/bundles/package_test.go` et test CLI dédié
  vérifiant que l'app embarquée change effectivement de version.
- Réutilise systématiquement les mécanismes déjà éprouvés
  (`internal/trust`'s CRUD, `packaging.SafeJoin`, le patron
  `os.MkdirTemp` + `defer os.RemoveAll` de staging) plutôt que d'en
  réinventer.
- `internal/registry` est conçu génériquement (id, kind, version) pour que
  `plugin install`/`app install` puissent le réutiliser plus tard sans
  reconception.
- Entièrement utilisable hors-ligne via un registre répertoire local — pas
  de dépendance à un service distant pour tester ou utiliser la
  fonctionnalité.

## Conséquences négatives

- Première capacité réseau sortante du core — un registre http(s) mal
  configuré ou compromis peut faire échouer une résolution ou servir un
  package altéré (atténué : le package téléchargé traverse toujours
  `packaging.Verify`/`trust.CheckPolicy`, inchangés par cette décision ;
  seule la provenance du fichier local change, pas la vérification de son
  contenu).
- Pas de cache : chaque `Resolve` relit l'index en entier à chaque appel.
- `index.json` lui-même n'est ni signé ni vérifié — seul le package final
  l'est (ADR-0043). Un registre compromis peut donc rediriger vers un
  mauvais fichier, qui échouera ensuite la vérification de signature s'il
  est falsifié, mais rien n'empêche un registre de simplement ne pas
  servir la dernière version en toute discrétion.
- `latest` est une déclaration de l'auteur de l'index, jamais recalculée
  ni validée — un index mal maintenu peut annoncer une « dernière version »
  incorrecte sans qu'aucun mécanisme ne le détecte.

## Explicitement hors scope (différé, pas oublié)

- Résolution par registre pour `plugin install`/`app install` — le paquet
  `internal/registry` le permettrait déjà, seule la CLI ne le fait pas
  encore pour ces deux commandes.
- Distribution des clés de confiance via le registre (déjà nommé comme
  différé par ADR-0043) — reste ouvert.
- Cache local d'index ou de packages téléchargés.
- Comparaison sémantique de versions (semver) — les versions restent des
  chaînes opaques comparées par égalité, comme partout ailleurs dans ce
  dépôt.
- Un vrai serveur de registre applicatif Patchcord — un registre reste un
  répertoire local ou un serveur de fichiers statiques générique.
- `plugin update`/`app update`.
- Installation automatique des dépendances greffons manquantes d'un bundle
  (déjà différé par ADR-0042 — le registre existe maintenant, mais cette
  tâche spécifique n'est pas reprise ici).
