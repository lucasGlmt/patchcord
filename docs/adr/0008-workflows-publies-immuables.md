# ADR-0008 — Les workflows publiés sont immuables

## Statut
Accepté

## Contexte
Un workflow publié peut être invoqué manuellement, par cron ou par webhook, potentiellement longtemps après sa publication. Si sa définition pouvait être modifiée en place après publication, un run en cours ou une exécution planifiée pourrait se comporter différemment selon le moment exact de son déclenchement, rendant l'historique des exécutions difficile à auditer et à reproduire.

## Décision
Une fois publiée, une version de workflow ne peut plus être modifiée. Toute évolution crée une nouvelle version numérotée (`version 1`, `version 2`, `version 3 active`, ...). Un run conserve et utilise la version du workflow qui était active au moment de son démarrage, indépendamment des versions publiées ultérieurement.

## Conséquences positives
- Chaque run est reproductible et auditable : la définition exacte utilisée reste connue et stable dans le temps.
- Plusieurs versions d'un même workflow peuvent coexister sans risque d'interférence entre une édition en cours et des exécutions en production.
- Le compilateur de workflows peut valider une version une fois pour toutes à la publication, sans revalidation à chaque run.

## Conséquences négatives
- La gestion de version doit être exposée explicitement dans la CLI et l'API (`workflow inspect`, `workflow export` par version), ce qui ajoute une surface fonctionnelle dès la phase 3.
- Le stockage croît avec chaque publication, y compris pour des changements mineurs ; une politique de rétention ou d'archivage devra être envisagée ultérieurement.
- Corriger un bug dans un workflow déjà publié nécessite de publier une nouvelle version et de migrer les déclencheurs vers celle-ci, plutôt qu'un correctif in-place.
