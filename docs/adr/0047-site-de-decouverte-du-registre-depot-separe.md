# ADR-0047 — Site de découverte du registre : dépôt séparé, backend dès la v1

## Statut
Accepté

## Contexte

L'ADR-0046 fige le mécanisme consommé par le CLI/core : `internal/registry`
résout un id de package contre des fichiers statiques (`index.json` +
packages) servis par un ou plusieurs dépôts GitHub — aucun serveur
applicatif requis pour que `plugin install`/`bundle install` fonctionnent.

Lucas souhaite en plus maintenir une **application de découverte** pour
les humains, dans l'esprit de `pub.dev` (Flutter) ou `npmjs.com` (Node) :
recherche, pages de détail par package, versions — une vitrine, pas un
mécanisme d'installation. Il a explicitement confirmé que ce sont deux
choses distinctes : les dépôts GitHub restent le stockage/canal de mise à
jour effectif pour la CLI ; le site n'en est qu'un lecteur.

Cette décision porte uniquement sur la frontière entre ce site et
`patchcord_core` — pas sur sa conception interne, qui n'appartient pas à
ce dépôt.

## Décision

**Dépôt séparé, hors de `patchcord_core`.** Cohérent avec la philosophie de
frontières du CLAUDE.md §2 (« respecte ces frontières comme si les
composants vivaient déjà dans des dépôts séparés ») : ce site n'est ni le
core, ni la CLI, ni le SDK, ni un greffon/app d'exemple — c'est un produit
distinct qui consomme un contrat public déjà stable (`index.json` de
l'ADR-0044, packages signés de l'ADR-0043). Nom de dépôt et stack technique
restent à trancher au moment de sa création (hors scope ici).

**Sens de la dépendance, non négociable.** Le site consomme le contrat
public du registre ; l'inverse est interdit : `internal/registry` ne doit
jamais appeler d'API propre au site, et aucune commande CLI/core ne doit
un jour exiger que le site soit joignable pour fonctionner. Le principe
« le cloud reste facultatif » (CLAUDE.md §1.9) s'applique ici à la relation
inverse — l'existence du site ne devient jamais une condition de
fonctionnement de l'agent.

**Backend dès la v1**, plutôt qu'un simple site statique généré au build :
synchronisation périodique des registres configurés vers un stockage
propre au site, recherche live. Ce choix est fait en connaissance de son
coût — une vraie infrastructure à opérer dès le départ, assumée par Lucas
— plutôt que la voie plus minimale (génération statique à chaque mise à
jour de registre) qui aurait été le défaut recommandé.

**Pas de marketplace.** Le site reste un catalogue de découverte, jamais
un canal de vente (non-objectif de la section 1 de CLAUDE.md) : pas de
commerce, pas de commission. Comptes utilisateurs/favoris explicitement
différés — non nécessaires pour une consultation du catalogue.

## Explicitement hors scope (différé, pas oublié)

- Nom du dépôt, stack technique, modèle de données du backend, mécanisme
  exact de synchronisation depuis les `index.json` — décisions internes au
  futur dépôt du site, pas de ce dépôt.
- Comptes utilisateurs, favoris, statistiques de téléchargement.
- Tout mécanisme de commerce (déjà exclu comme non-objectif général).
- Un éventuel ADR miroir côté dépôt du site, une fois celui-ci créé.

## Conséquences positives

- Sépare clairement « ce qui doit marcher pour que l'agent fonctionne »
  (ADR-0044/0046, zéro dépendance externe) de « ce qui aide à découvrir
  l'écosystème » (ce site) — un utilisateur hors-ligne ou sans compte
  distant n'est jamais bloqué par l'un pour l'autre.
- Le site peut évoluer (recherche, UX, éventuellement d'autres registres
  que celui de Patchcord) sans jamais toucher au protocole ni au code de
  `patchcord_core`.

## Conséquences négatives

- Un vrai backend à opérer et maintenir dès la v1 (disponibilité, coût,
  sécurité) plutôt que la voie statique plus économique — accepté comme
  compromis délibéré, pas comme dette non vue.
- Deux endroits distincts encodent une notion de « registre » (le contrat
  statique `index.json`, et le modèle de données interne du site) — un
  changement du premier doit être répercuté manuellement dans le second,
  aucune génération automatique entre les deux n'est prévue ici.
