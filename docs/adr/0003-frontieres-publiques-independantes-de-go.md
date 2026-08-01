# ADR-0003 — Les frontières publiques sont indépendantes de Go

## Statut
Accepté

## Contexte
Le core est écrit en Go, et il serait naturel de définir les contrats publics (API cliente, protocole de greffons, format des packages et workflows) directement sous forme d'interfaces et de structures Go. Cette approche coudrait cependant l'écosystème entier — greffons tiers, SDK, applications — au langage d'implémentation du core, ce qui contredit l'ambition d'un protocole ouvert à plusieurs langages (Rust, TypeScript, Python, Java, .NET à terme) et rendrait toute refonte interne du core risquée pour l'écosystème.

## Décision
Les trois frontières publiques du projet (API des clients, protocole des greffons, format des packages/workflows) sont définies via des schémas indépendants du langage — Protobuf, JSON Schema, OpenAPI — et versionnées explicitement. Les interfaces Go restent utiles à l'intérieur du core, mais ne font jamais foi comme contrat public.

## Conséquences positives
- Des SDK dans d'autres langages peuvent être générés ou écrits à la main sans dépendre de Go.
- L'intérieur du core peut être refactoré librement tant que les schémas publics restent stables.
- Les contrats publics deviennent testables et vérifiables indépendamment de toute implémentation Go particulière.

## Conséquences négatives
- Une double maintenance apparaît : les schémas publics d'un côté, leur implémentation Go de l'autre, avec un risque de désynchronisation si le processus n'est pas outillé (génération de code, tests de contrat).
- Le coût de conception initiale est plus élevé qu'un simple partage de types Go entre core et greffons.
- Toute évolution des schémas publics doit suivre une discipline de versionnage stricte, y compris pendant la phase de prototypage rapide.
