# ADR-0006 — Le projet commence dans un monorepo

## Statut
Accepté

## Contexte
Le protocole de greffons, les contrats d'API et les SDK évoluent rapidement en phase initiale. Répartir immédiatement `cmd/`, `internal/`, `api/`, `sdk/go-plugin`, `sdk/typescript`, `plugins/examples` et `apps/examples` dans des dépôts séparés multiplierait la coordination nécessaire pour chaque changement de contrat, alors même que ces contrats ne sont pas encore stabilisés.

## Décision
Le projet démarre en monorepo. Les frontières internes (`cmd/`, `internal/`, `api/`, `sdk/`, `plugins/examples/`, `apps/examples/`, `docs/`, `migrations/`) sont néanmoins conçues et respectées comme si chaque composant vivait déjà dans un dépôt séparé : `internal/` n'est jamais importable depuis `plugins/`, `sdk/`, ou `apps/`.

## Conséquences positives
- Les changements touchant à la fois un contrat public et son implémentation (core, SDK, greffon d'exemple) peuvent être faits atomiquement, dans une seule revue.
- La CI et l'outillage de développement restent simples pendant la phase de prototypage rapide.
- L'extraction future vers des dépôts séparés (`patchcord-agent`, `patchcord-protocol`, `patchcord-sdk-go`, `patchcord-sdk-typescript`, greffons individuels) reste possible sans réécriture, car les frontières sont déjà respectées.

## Conséquences négatives
- Sans discipline constante, le monorepo facilite les raccourcis (un greffon d'exemple qui importerait `internal/` "temporairement") — cette règle doit être surveillée activement, y compris via revue de code.
- Le monorepo grandira avec des composants dont les cycles de release naturels diffèrent (core vs SDK vs greffons d'exemple), ce qui devra être géré au moment de l'extraction en phase stable.
