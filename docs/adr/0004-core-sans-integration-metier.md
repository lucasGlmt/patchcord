# ADR-0004 — Le core ne contient aucune intégration métier

## Statut
Accepté

## Contexte
Il est tentant d'ajouter directement dans `internal/` une intégration "évidente" (par exemple un client OpenAI ou Gmail) pour livrer plus vite une première démonstration utile. Cette tentation est d'autant plus forte que le projet vient d'une architecture (GLMT Compagnon) où l'agent et ses intégrations étaient historiquement confondus. Céder à cette tentation dès la phase 1 romprait le principe fondateur "le core fournit les mécanismes, les greffons fournissent les capacités" avant même qu'il ait été éprouvé.

## Décision
Le noyau (`internal/`) ne doit jamais importer, référencer ou connaître un service métier concret — pas de client OpenAI, Gmail, Notion, Microsoft 365, PostgreSQL applicatif, CRM ou équivalent dans le core. Toute capacité de ce type est apportée exclusivement par un greffon, y compris les greffons dits "officiels".

## Conséquences positives
- Le core reste petit, générique et stable, indépendamment du nombre d'intégrations disponibles dans l'écosystème.
- La séparation core/greffons est exercée dès la première tranche verticale, ce qui garantit qu'elle fonctionne réellement plutôt que d'être une intention non vérifiée.
- Une entreprise peut auditer le core sans avoir à faire confiance à un fournisseur tiers particulier.

## Conséquences négatives
- Même une première démonstration "utile" (ex. lire des e-mails, appeler une IA) nécessite de construire un greffon complet avant de pouvoir être montrée, ce qui ralentit les premières itérations.
- Le stockage local (SQLite) et d'autres briques génériques doivent être soigneusement distingués des "services métier concrets" pour ne pas être bloqués à tort par cette règle.
- Toute proposition future d'ajouter une dépendance métier dans `internal/`, même pour un gain de simplicité apparent, doit être systématiquement remise en question.
