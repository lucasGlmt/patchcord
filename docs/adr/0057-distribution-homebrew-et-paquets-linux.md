# ADR-0057 — Canaux de distribution du binaire : Homebrew et paquets Linux

## Statut
Accepté

## Contexte

ADR-0056 a mis en place la publication automatisée du binaire agent
(GitHub Releases via goreleaser, sur push d'un tag `v*`), mais uniquement
sous forme d'archives brutes (`.tar.gz`/`.zip`) à télécharger et extraire
à la main. Lucas a demandé si ça valait le coup, et si c'était cohérent
avec la philosophie du projet, de publier aussi sur des gestionnaires de
paquets — Homebrew (macOS), apt (Linux), Chocolatey (Windows) ont été
évoqués nommément.

Les trois canaux n'ont pas le même coût :
- **Homebrew** (via un « tap » personnel, `lucasGlmt/homebrew-patchcord`) :
  goreleaser sait pousser automatiquement une définition Homebrew à chaque
  release. Zéro infrastructure à héberger au-delà d'un repo git.
- **apt/dnf en tant que dépôt hébergé** (pas juste des fichiers `.deb`/
  `.rpm`) : demande un serveur de dépôt signé GPG maintenu dans la durée —
  une charge d'infra permanente.
- **Chocolatey** : le dépôt communautaire modère chaque soumission
  manuellement ; le support Windows du binaire vient tout juste d'être
  réparé (ADR-0056) et n'a pas encore été éprouvé par de vrais
  utilisateurs.

Le projet est en phase 1 (core minimal, section 9 de CLAUDE.md), aucun tag
n'a encore été poussé. CLAUDE.md section 9 est explicite : ne pas
anticiper des mécanismes de phases ultérieures tant que la phase courante
n'est pas stable. La question posée ici n'est pas architecturale au sens
des non-négociables (section 1) — publier le binaire ailleurs ne crée
aucune dépendance cloud requise pour faire tourner l'agent (#9 reste
respecté) — mais c'est un choix de proportion : quelle charge de
maintenance engager, et quand.

## Décision

**Homebrew, maintenant.** `.goreleaser.yaml` gagne un bloc
`homebrew_casks` (pas l'ancien `brews`, obsolète depuis goreleaser v2.16 —
voir https://goreleaser.com/deprecations/#brews) qui pousse une définition
vers `lucasGlmt/homebrew-patchcord` à chaque release, pour macOS et Linux
(Linuxbrew). Installation utilisateur : `brew install
lucasGlmt/patchcord/patchcord`.

**`.deb`/`.rpm` en pièces jointes de la release, pas de dépôt hébergé.**
`nfpm` (déjà intégré à goreleaser) génère les deux formats à chaque
release ; ils sont attachés à la GitHub Release au même titre que les
archives `.tar.gz`/`.zip`. Installation utilisateur : télécharger puis
`dpkg -i`/`rpm -i` à la main — pas de `apt install`/`dnf install` direct,
faute de dépôt signé hébergé.

**Chocolatey : reporté.** Aucun changement pour l'instant. À reconsidérer
une fois que le support Windows (ADR-0056) aura vécu quelques releases
réelles.

**Prérequis manuels, hors du code** (pas automatisables depuis ce dépôt) :
1. Créer le repo GitHub public `lucasGlmt/homebrew-patchcord` (peut rester
   vide — goreleaser y écrit `Casks/patchcord.rb` au premier run).
2. Créer un jeton d'accès (de préférence fine-grained, limité à ce seul
   repo, permission Contents: Read & write) — le `GITHUB_TOKEN` fourni par
   défaut dans `release.yml` n'a des droits que sur `patchcord`, pas sur le
   tap.
3. L'enregistrer comme secret du repo `patchcord`, sous le nom
   `HOMEBREW_TAP_GITHUB_TOKEN` (déjà référencé dans
   `.github/workflows/release.yml`).

Sans ces trois étapes, `release.yml` échouera à l'étape de publication du
cask — les archives, `.deb` et `.rpm` de la GitHub Release, elles,
resteront publiées normalement (goreleaser ne fait pas échouer les autres
publishers si un seul échoue, sauf configuration contraire).

## Conséquences positives

- Installation en une commande sur macOS/Linuxbrew (`brew install
  lucasGlmt/patchcord/patchcord`), sans étape de compilation ni de gestion
  manuelle du `$PATH`.
- Les utilisateurs Debian/Ubuntu/Fedora/RHEL ont un paquet natif
  (dépendances, désinstallation propre via le gestionnaire de paquets du
  système) sans que le projet héberge la moindre infrastructure
  supplémentaire — même pipeline goreleaser que le reste de la release.
- Aucun engagement pris qui serait coûteux à défaire : le tap Homebrew et
  les paquets `.deb`/`.rpm` peuvent être abandonnés en supprimant
  simplement les blocs `.goreleaser.yaml` correspondants.

## Conséquences négatives

- Un secret supplémentaire (`HOMEBREW_TAP_GITHUB_TOKEN`) à provisionner et
  à faire vivre (rotation si besoin) en dehors du `GITHUB_TOKEN` par
  défaut.
- `.deb`/`.rpm` sans dépôt signé : pas de mise à jour automatique via
  `apt upgrade`/`dnf upgrade`, ni de vérification de provenance au-delà du
  HTTPS de GitHub — un utilisateur qui veut ça devra repasser par
  `dpkg -i`/`rpm -i` à chaque nouvelle version.
- Chocolatey reste absent ; un utilisateur Windows n'a que l'archive brute
  de la GitHub Release comme option pour l'instant.
