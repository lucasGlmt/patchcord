# ADR-0001 — Patchcord Agent est le produit fondamental

## Statut
Accepté

## Contexte
Le projet est né sous le nom GLMT Compagnon comme une application desktop Flutter embarquant un agent Dart local : l'interface graphique portait la vision, l'agent n'était qu'un processus enfant à son service. Cette architecture a validé plusieurs idées (workflows déclaratifs versionnés, actions atomiques, connecteurs encapsulant secrets et configuration), mais elle limite structurellement le produit à un seul mode de déploiement et fait de l'interface le centre de gravité du système.

## Décision
Patchcord Agent devient le produit fondamental. Il doit pouvoir fonctionner intégralement sans interface graphique, piloté uniquement par sa CLI et son API. Les interfaces graphiques, dashboards et applications desktop — y compris une future application desktop officielle — sont des clients du runtime parmi d'autres, jamais son centre.

## Conséquences positives
- L'agent est déployable en local, en service système, en conteneur ou sur serveur sans dépendre d'une couche UI.
- La CLI devient une interface de référence complète, ce qui force à exposer toutes les fonctionnalités via des services internes bien définis.
- Aucune logique métier ne peut se cacher dans une interface graphique, puisque celle-ci n'est jamais indispensable.

## Conséquences négatives
- Aucune expérience graphique "clé en main" n'est livrée par défaut avec le core.
- La CLI doit rester une interface de première classe et non un sous-produit, ce qui demande un effort de conception soutenu dès la phase 1.
- La transition depuis l'ancienne architecture (agent Dart embarqué) impose une réécriture complète du noyau plutôt qu'une simple migration incrémentale.
