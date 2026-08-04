# ADR-0048 — Registre ouvert multi-éditeurs (Patchbay) : ids opaques, octets hébergés par le registre

## Statut
Accepté

## Contexte

Les ADR-0044/0046/0047 ont conçu et nommé un registre « officiel », centré
sur les packages maintenus par l'équipe Patchcord sous le préfixe réservé
`io.patchcord.*`, avec une pré-confiance restreinte à ce préfixe. Lucas
élargit maintenant la portée du service qu'il va construire (Patchbay,
ADR-0047 : dépôt séparé, site de découverte, backend dès la v1) : n'importe
quel développeur tiers (exemple donné : « jean ») doit pouvoir publier son
propre greffon via un outil dédié (`bay publish`, propre au futur dépôt
Patchbay — hors périmètre de `patchcord_core`), référencé par un id du
type `github.com/jean/plugindequalite`, et installable par n'importe quel
utilisateur via `patchcord plugin install github.com/jean/plugindequalite`.

Cette décision vérifie, et fige explicitement, que ce besoin ne demande
**aucun changement** au mécanisme déjà construit côté core — pour éviter
qu'une future session ne suppose à tort que l'id d'un package doit
ressembler à `io.patchcord.*` ou qu'il faut une logique spéciale pour des
ids en forme d'URL.

## Décision

**Les ids de package restent strictement opaques.** `internal/registry`
(ADR-0044) ne leur a jamais imposé de forme — une clé de `Index.Packages`
est une chaîne quelconque. Ceci est confirmé explicitement ici : un id du
type `github.com/jean/plugindequalite` est un id valide comme un autre.
`registry.ParseRef` ne coupe que sur le premier `@` pour isoler une
version ; `/` et `.` ne sont pas des séparateurs significatifs pour le
CLI. Le CLI ne « sait » jamais qu'un id ressemble à une URL — il interroge
simplement les registres configurés, exactement comme pour un id
`io.patchcord.*`.

**Patchbay héberge les octets des packages publiés.** `bay publish`
envoie l'archive déjà packagée/signée par son auteur vers le stockage de
Patchbay ; l'entrée d'index que Patchbay sert référence un chemin relatif
à sa propre `location`, résolu par `registry.Fetch` sans aucune capacité
nouvelle. `internal/registry` n'apprend pas à suivre une URL externe
arbitraire (le dépôt GitHub d'origine de jean, une release externe) —
cette capacité n'existe pas et n'est pas ajoutée par cette décision.
Installer un package tiers ne dépend donc jamais de la disponibilité du
dépôt d'origine de son auteur, seulement de celle de Patchbay.

**`io.patchcord.*` reste inchangé et distinct.** La pré-confiance embarquée
dans le binaire (ADR-0046) continue de ne s'appliquer qu'à ce préfixe
réservé. Un package tiers (`github.com/jean/...`) suit strictement le
modèle de confiance générique déjà existant (ADR-0043) : aucune confiance
implicite, `trust add <id> <pubkey>` par id, jamais par éditeur ni par
registre dans son ensemble — approuver la clé de jean pour son id ne
rend pas automatiquement fiable un autre id publié par la même personne
ou le même registre.

**Ce n'est pas une marketplace.** Publier reste gratuit, sans commerce ni
commission — cohérent avec le non-objectif de la section 1 de CLAUDE.md.
Un registre ouvert à tout éditeur n'est pas, en soi, une marketplace :
c'est la présence de vente/commission qui en ferait une, pas la simple
ouverture à des tiers (même distinction que pub.dev/npmjs face à un
marketplace payant).

## Explicitement hors scope (différé, pas oublié — propre au futur dépôt Patchbay, pas à `patchcord_core`)

- Vérification qu'un éditeur a réellement le droit de publier sous un id
  du type `github.com/<user>/...` (preuve de propriété du compte/dépôt) —
  sans ce mécanisme, rien n'empêche aujourd'hui un usurpateur de publier
  sous l'id GitHub de quelqu'un d'autre. Nécessaire avant une ouverture
  publique réelle de `bay publish`, mais c'est un mécanisme d'auth propre
  au service Patchbay, pas au protocole de registre.
- Modération, retrait/signalement d'un package tiers, gestion de
  collisions d'id.
- Toute distinction technique « officiel vs communautaire » au niveau du
  format d'index ou du protocole — la seule distinction qui existe est la
  pré-confiance restreinte à `io.patchcord.*` (ADR-0046) ; tout le reste
  traite les ids de façon identique.

## Conséquences positives

- Zéro changement de code dans `internal/registry`/`internal/trust` :
  la conception générique de l'ADR-0044 (id opaque, confiance par id)
  absorbe ce besoin sans reconception.
- Un id `github.com/jean/...` reste lisible/traçable jusqu'à son auteur
  d'origine, même si les octets réels sont servis par Patchbay — l'id sert
  d'ancrage humain sans dépendre techniquement de GitHub à l'installation.
- Cohérent avec le modèle déjà choisi pour l'hébergement (Patchbay
  autonome, ADR-0047) : un seul endroit à interroger et à faire fonctionner
  pour que `plugin install` réussisse, y compris pour des packages tiers.

## Conséquences négatives

- Patchbay devient un point de disponibilité unique pour l'installation de
  *tout* package qu'il référence, pas seulement les packages officiels —
  une panne de Patchbay bloque aussi l'installation des packages tiers
  qu'il héberge (atténué : un utilisateur peut toujours configurer un
  autre registre ou un chemin local, `internal/registry` interroge
  plusieurs registres sans favoriser Patchbay techniquement).
- Aucune vérification de propriété de namespace n'existe tant que
  `bay publish` ne l'implémente pas — risque de typosquat/usurpation d'un
  id ressemblant à un dépôt GitHub connu, purement au niveau de Patchbay
  (le core n'a aucun moyen, ni besoin, de le détecter : il ne fait que
  résoudre l'id qu'on lui demande).
