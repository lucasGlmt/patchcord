# ADR-0005 — La CLI et l'API utilisent la même couche applicative

## Statut
Accepté

## Contexte
La CLI (`patchcord workflow run`, `patchcord plugin install`, etc.) et l'API publique exposent en grande partie les mêmes capacités. Il serait possible d'implémenter la CLI comme un client HTTP de l'API, ou au contraire d'implémenter chacune indépendamment pour des raisons de performance ou de simplicité locale — au risque de faire diverger leur comportement au fil du temps.

## Décision
La CLI et les handlers de l'API publique appellent les mêmes services internes (couche applicative). Aucune logique métier n'est dupliquée entre la CLI et l'API : toute règle de validation, d'autorisation ou d'orchestration vit dans un seul endroit, consommé par les deux interfaces.

## Conséquences positives
- Aucune divergence de comportement possible entre CLI, API et donc entre CLI, dashboards et applications tierces.
- Un bug corrigé dans la couche applicative est corrigé simultanément pour toutes les interfaces.
- La couche applicative devient naturellement le point d'entrée testable en priorité (tests table-driven sur les services, indépendamment du transport).

## Conséquences négatives
- La CLI ne peut pas prendre de raccourcis "pratiques" qui contourneraient la couche de services, même quand une implémentation directe serait plus simple localement.
- La conception de la couche applicative doit être posée tôt et rester stable, sous peine de devoir migrer CLI et API simultanément à chaque refonte.
