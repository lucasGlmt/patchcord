# ADR-0043 — Signature et vérification des packages

## Statut
Accepté

## Contexte

ADR-0042 (phase 7, tâche 1) a livré les formats `.patchcord-plugin`,
`.patchcord-workflow` et `.patchcord-bundle`, avec un paquet interne partagé
`internal/packaging` réutilisé par `internal/apps`, `internal/plugins` et
`internal/bundles`. Le document de vision (§9.1) prévoit `checksums.json` et
`signature.json` dans la structure du package et liste « vérifier les
checksums »/« vérifier la signature » comme étapes 2 et 3 de l'installation
(§9.2) — jamais implémenté jusqu'ici. ADR-0027 avait explicitement différé
ce point à la phase 7.

Trois décisions structurantes ont été tranchées avec Lucas avant
l'implémentation :

1. **Schéma cryptographique** : Ed25519, pas de PKI/certificats. Cohérent
   avec le local-first (ADR-0007) — il n'y a pas de registre ni d'autorité
   de certification à qui faire confiance, une simple paire de clés suffit.
2. **Modèle de confiance** : trust store explicite. Une clé publique doit
   être approuvée (`patchcord trust add <id> <pubkey>`) avant d'être
   considérée fiable pour un id de package donné — pas de confiance
   implicite, pas de trust-on-first-install.
3. **Application** : avertir par défaut sur un package non signé ou signé
   par une clé non approuvée ; `--require-signature` sur `install` force le
   rejet. Une intégrité invalide (checksums qui ne correspondent pas,
   signature cryptographiquement invalide) reste un rejet
   **inconditionnel**, jamais un simple avertissement, quelle que soit la
   politique appelante.

## Décision

**`internal/packaging` gagne les primitives de checksums/signature**
(`sign.go`), au-dessus d'`Archive`/`Extract` existants (ADR-0042) :

- `SignedArchive(sourceDir, key ed25519.PrivateKey, w io.Writer)` calcule un
  `checksums.json` (sha256 hex de chaque fichier, clé = chemin relatif —
  `encoding/json.Marshal` sur une map trie déjà les clés, sortie
  déterministe gratuite) et, si `key != nil`, signe **les octets bruts de
  `checksums.json`** pour produire `signature.json`
  (`{"algorithm":"ed25519","publicKey":...,"signature":...}`). `key == nil`
  produit un package avec intégrité mais sans provenance — le comportement
  par défaut de `pack` sans `--sign-key`.
- `Verify(dir string) (VerificationOutcome, error)`, sur un répertoire déjà
  extrait : `checksums.json` absent → non-erreur (package ancien style,
  rétrocompatible avec les packages de la tâche 1) ; présent mais ne
  correspond pas aux fichiers réels → **erreur** `ErrChecksumMismatch`,
  toujours ; `signature.json` présent mais signature invalide → **erreur**
  `ErrInvalidSignature`, toujours. La signature est vérifiée contre les
  octets **stockés** de `checksums.json`, jamais une resérialisation —
  élimine tout risque de désaccord de déterminisme JSON entre signature et
  vérification.
- `Verify` ne connaît ni id de package ni trust store : pure intégrité +
  authenticité cryptographique, rien d'autre.

**Nouveau paquet `internal/signing`** : gestion des fichiers de clés
(génération, écriture atomique `0o600`/`0o644` sur le modèle de
`secrets.FileStore`, lecture, empreinte cosmétique sha256 tronquée pour
l'affichage CLI). Séparé de `internal/packaging`, qui ne manipule que des
clés en mémoire, jamais de fichiers.

**Nouveau paquet `internal/trust`** : trust store persistant
(`migrations/0009_trusted_keys.sql`, table `trusted_keys (id, public_key,
label, trusted_at)`, clé primaire composite `(id, public_key)`). La
confiance est liée à la **paire** (id de package, clé publique), jamais à
la clé seule — approuver une clé pour un id ne la rend pas automatiquement
fiable pour un id sans rapport. `trust.CheckPolicy` centralise la décision
« ce package doit-il être installé ? » : elle prend le résultat de `Verify`
et le drapeau `requireSignature`, consulte le trust store, et renvoie une
erreur uniquement si `requireSignature` est vrai et que le package n'est
pas à la fois signé et approuvé. Cette fonction est appelée identiquement
par les trois `InstallPackage` (apps/plugins/bundles) plutôt que dupliquée
trois fois — seul le branchement (extraire, appeler `Verify`, appeler
`CheckPolicy`, continuer ou abandonner) est répété dans chacun, pas la
logique de décision elle-même.

**`Pack`/`InstallPackage`** dans les trois formats existants gagnent
respectivement un paramètre `key ed25519.PrivateKey` et
`requireSignature bool` ; `InstallPackage` renvoie désormais aussi un
`trust.PolicyResult` (résultat de `Verify` + `Trusted bool`), pour que
l'appelant (CLI) puisse avertir même quand l'installation réussit sans
`--require-signature`. **Un bundle ne revérifie pas son app embarquée** :
`bundles.installEmbeddedApp` continue d'appeler `apps.Install`, jamais
`apps.InstallPackage` — la signature du bundle couvre déjà l'intégrité de
tout son contenu, y compris l'app embarquée.

**CLI** : deux nouveaux groupes top-level, `patchcord key generate` (aucune
connexion à la base — un outil crypto pur, comme `secret keygen`) et
`patchcord trust add/list/remove` (sur le modèle d'`auth`). `plugin
pack`/`app pack`/`bundle pack` gagnent `--sign-key <path>` ; `plugin
install`/`app install`/`bundle install` gagnent `--require-signature`, qui
échoue immédiatement (plutôt qu'un no-op silencieux) si la cible s'avère
être un exécutable brut ou un répertoire — il n'y a rien à vérifier dans ce
cas, mieux vaut le dire explicitement. Un avertissement partagé
(`internal/cli/verification.go`) s'affiche après un install réussi sans
`--require-signature` quand le package est non signé ou signé par une clé
non approuvée.

## Explicitement hors scope (différé, pas oublié)

- Registre / distribution des clés publiques par un tiers de confiance —
  tâche « registre » de la phase 7, plus tard.
- Rotation/révocation de clé au-delà de `trust remove` + `trust add` d'une
  nouvelle clé pour le même id.
- Chiffrement de la clé privée au repos (passphrase) — fichier en clair,
  `0o600`, comme un `id_ed25519` sans passphrase.
- Signature niveau fichier individuel (seule la racine `checksums.json` est
  signée).
- Installation automatique des dépendances greffons manquantes d'un bundle
  (déjà différé par ADR-0042).

## Conséquences positives

- Ferme les étapes 2 et 3 de l'installation de package (vision §9.2),
  identiquement pour les trois formats existants.
- `trust.CheckPolicy` et `packaging.Verify` sont chacun testés une seule
  fois et réutilisés trois fois — aucune des trois implémentations
  `InstallPackage` ne réinvente la décision de confiance.
- Rétrocompatible : les packages produits par la tâche 1 (sans
  `checksums.json`) continuent de s'installer par défaut, et échouent
  seulement sous `--require-signature`. Vérifié explicitement en test et en
  bout en bout manuel.
- Vérifié de bout en bout avec le vrai binaire : `key generate` → `pack
  --sign-key` → `install` (avertissement) → `trust add` → `install`
  silencieux → `install --require-signature` (échoue avant `trust add`,
  réussit après) → installation d'une archive altérée rejetée.

## Conséquences négatives

- Aucun mécanisme de distribution des clés publiques : `trust add` suppose
  que l'utilisateur a déjà obtenu le fichier `.pub` par un canal hors bande
  (à la main, comme un `known_hosts` SSH) — attendu tant que le registre
  n'existe pas.
- Une clé privée compromise ne peut pas être révoquée globalement, seulement
  retirée du trust store id par id.
