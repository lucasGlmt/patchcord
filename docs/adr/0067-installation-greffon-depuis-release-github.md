# ADR-0067 — Installation directe d'un greffon depuis les Releases GitHub d'un dépôt

## Statut
Accepté

## Contexte

`patchcord plugin install` sait installer un greffon depuis un exécutable
brut ou une archive `.patchcord-plugin` locale (ADR-0042), mais rien
n'existe aujourd'hui pour installer directement depuis le dépôt GitHub
d'un auteur tiers — la seule façon d'essayer un greffon publié sur GitHub
est de télécharger soi-même l'archive `.patchcord-plugin` d'une Release
avant de lancer `plugin install` dessus. Lucas veut que la communauté
puisse publier et installer des greffons facilement, à la manière de
`go get` :

    patchcord plugin install github.com/lucasglmt/patchcord-imap-plugin
    patchcord plugin install github.com/lucasglmt/patchcord-imap-plugin@v1.2.0

**Cette décision révise explicitement une clause de l'ADR-0048.**
L'ADR-0048 (lignes 37-45) affirmait qu'`internal/registry` « n'apprend pas
à suivre une URL externe arbitraire (le dépôt GitHub d'origine de jean,
une release externe) — cette capacité n'existe pas et n'est pas ajoutée
par cette décision. » Cette capacité est maintenant ajoutée, mais
**volontairement en dehors d'`internal/registry`** (voir Décision) — le
reste de l'ADR-0048 (ids opaques, Patchbay héberge les octets des packages
qu'elle référence, `io.patchcord.*` reste la seule distinction de
confiance) reste intégralement valide et inchangé. L'ADR-0048 n'est donc
**pas** marquée « Remplacée par ADR-0067 » : ceci est un amendement étroit
à une seule clause, pas un remplacement de la décision.

## Décision

**Nouveau package `internal/ghrelease`, parallèle à `internal/registry`,
pas une extension de celui-ci.** `plugin install` essaie, dans l'ordre :
(1) un fichier local (comportement actuel, inchangé) ; (2) une référence
`github.com/<owner>/<repo>[@<tag>]`, résolue via l'API REST publique de
GitHub (`net/http` + `encoding/json`, aucune dépendance externe ajoutée —
non-négociable CLAUDE.md §1.3). Sans `@tag`, la dernière Release est
utilisée ; avec `@tag`, le tag est pris tel quel (pas de logique
`v`-prefix). `internal/registry` (registres configurés, Patchbay,
ADR-0044/0046/0047/0048) n'est ni modifié ni contourné : ces deux chemins
d'installation coexistent, indépendants.

**Aucune exécution de code arbitraire.** Patchcord ne clone jamais le
dépôt et ne le compile jamais. Il télécharge exactement un asset de
Release nommé `*.patchcord-plugin`, produit par l'auteur du greffon via
`plugin pack` et attaché manuellement à sa Release GitHub. Zéro asset ou
plus d'un asset `.patchcord-plugin` sur la release ciblée est une erreur
explicite (pas de désambiguïsation automatique dans cette itération).
L'asset téléchargé traverse ensuite le pipeline existant sans aucune
modification : `packaging.Verify`, `trust.CheckPolicy`,
`printVerificationStatus` — `--require-signature` s'applique à une
installation GitHub exactement comme à un package local (défaut
inchangé : `false`, décision produit confirmée explicitement pour cette
fonctionnalité : ne pas durcir la posture de confiance par défaut pour ne
pas freiner l'essai d'un greffon communautaire tout neuf).

**Dépôts publics uniquement.** `--github-token` (ou `GITHUB_TOKEN`) est
optionnel et sert uniquement à relever la limite de requêtes non
authentifiées de l'API GitHub (60/h/IP) — ce n'est pas un mécanisme
d'accès à un dépôt privé, explicitement hors périmètre de cette décision.

**Portée : `plugin install` uniquement.** `app install` et `bundle
install` ne sont pas touchés dans cette décision. `internal/ghrelease`
est conçu de façon générique (le suffixe de fichier attendu est un
paramètre d'appel, jamais codé en dur ni importé depuis `internal/plugins`)
pour que ces deux commandes puissent réutiliser le même package sans le
réécrire, le jour où cette extension sera décidée séparément.

## Conséquences positives

- Un greffon tiers publié sur GitHub s'installe en une commande, sans
  dépendre de Patchbay ni d'aucun registre configuré — utile en particulier
  avant que Patchbay (ADR-0047) n'existe réellement.
- Zéro changement à `internal/registry`, `internal/trust`, ou au pipeline
  de vérification de package : la nouvelle source alimente exactement les
  mêmes fonctions (`packaging.Verify`, `trust.CheckPolicy`,
  `plugins.InstallPackage`) qu'un package local.
- `internal/ghrelease` ne dépend d'aucun package `internal/plugins`,
  `internal/apps` ou `internal/bundles` — réutilisable par `app
  install`/`bundle install` plus tard sans reprise de conception.

## Conséquences négatives

- La syntaxe `github.com/<owner>/<repo>` est désormais réservée, pour
  `plugin install`, au chemin direct-GitHub décrit ici. L'ADR-0048 et le
  document de vision (§9.2) envisageaient qu'un id de cette forme puisse
  aussi être un id opaque résolu via un registre configuré (Patchbay).
  `plugin install` ne fait aujourd'hui aucune résolution par registre
  (contrairement à `bundle install`) donc il n'y a pas de collision
  réelle actuellement — mais le jour où `plugin install` apprendrait
  aussi à résoudre un id via un registre configuré, la même chaîne serait
  ambiguë entre les deux mécanismes. Cette décision ne tranche pas cette
  ambiguïté future ; elle réserve simplement cette syntaxe au chemin
  GitHub direct dès maintenant, à reconsidérer avant d'ajouter une
  résolution par registre à `plugin install`.
- Le CLI d'installation du core a désormais une dépendance dure à l'API
  REST de `api.github.com` pour ce seul chemin. Ceci reste conforme au
  non-négociable CLAUDE.md §1.3 (« le core ne doit jamais importer,
  référencer ou connaître un service métier concret ») parce qu'il s'agit
  d'outillage **d'installation en ligne de commande**, jamais invocable
  depuis un workflow ni exposé comme capacité métier à des connecteurs ou
  actions — mais c'est un jugement de frontière assumé explicitement ici,
  pas une évidence absolue.
- Dépôts publics uniquement : aucune installation depuis un dépôt privé
  tant que ce n'est pas explicitement conçu (authentification, risque de
  fuite de token, etc.).
- Limite de requêtes GitHub non authentifiées (60/h/IP) : installer
  plusieurs greffons GitHub en peu de temps peut échouer avec un message
  de limite de débit ; atténué par `--github-token`/`GITHUB_TOKEN`,
  optionnel.
- La posture de confiance par défaut reste permissive (identique à
  l'installation locale/package aujourd'hui) : un dépôt communautaire
  malveillant ou compromis peut servir un greffon qui s'installe et
  démarre avec un simple avertissement, sans blocage — pas pire que le
  statu quo pour un package local, mais désormais atteignable en une
  seule commande copiée depuis le README d'un greffon, avec un geste
  délibéré de l'utilisateur moindre que « télécharger ce fichier
  soi-même d'abord ».
