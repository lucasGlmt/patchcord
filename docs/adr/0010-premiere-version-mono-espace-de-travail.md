# ADR-0010 — La première version est mono-espace de travail

## Statut
Accepté

## Contexte
Un déploiement serveur de Patchcord pourrait, à terme, servir plusieurs organisations ou équipes isolées (multi-tenant complet). Concevoir cette isolation dès la première version serveur (authentification par tenant, cloisonnement des données, quotas par organisation) ajouterait une complexité significative au modèle d'autorisation et de persistance, alors que le multi-tenant n'est pas un objectif initial du projet (cf. non-objectifs).

## Décision
La première version serveur de Patchcord reste mono-espace de travail : un agent déployé sert un seul espace de travail logique, sans isolation multi-tenant complète.

## Conséquences positives
- Le modèle d'authentification et de permissions de la phase 1/6 reste simple (jetons de session, API keys, OIDC) sans avoir à porter une notion de tenant dans chaque couche.
- Le déploiement serveur (Docker, reverse proxy, TLS) peut être livré plus rapidement, en cohérence avec la roadmap (phase 6).
- Le cas d'usage prioritaire — une entreprise ou un intégrateur déployant Patchcord pour son propre usage — est pleinement couvert sans complexité inutile.

## Conséquences négatives
- Patchcord ne peut pas, en l'état, servir une offre SaaS multi-tenant : chaque organisation nécessite son propre déploiement d'agent.
- Si un besoin multi-tenant émerge plus tard, il nécessitera une passe architecturale dédiée (isolation des espaces de travail, permissions, persistance) plutôt qu'une extension incrémentale du modèle actuel.
