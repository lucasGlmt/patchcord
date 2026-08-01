# ADR-0011 — Chemin du module Go public

## Statut
Accepté

## Contexte
Le `go.mod` initial déclarait `module patchcord_core`, un chemin sans hôte, valide uniquement pour des imports internes à ce dépôt. Or `sdk/go-plugin` est destiné à être importé par des greffons tiers, potentiellement dans des dépôts séparés (cf. [[0006-monorepo-phase-initiale]] et la phase d'extraction en dépôts multiples décrite dans le document de vision). Un chemin de module sans hôte ne peut pas être résolu par `go get` depuis l'extérieur du dépôt, ce qui bloquerait toute utilisation externe du SDK avant même la phase d'extraction.

## Décision
Le module Go du dépôt est nommé `github.com/lucasglmt/patchcord`. Tous les imports internes (`cmd/patchcord`, `internal/...`, futurs packages de `api/` et `sdk/go-plugin` en Go) utilisent ce préfixe.

## Conséquences positives
- Le module est immédiatement compatible avec l'outillage Go standard (`go get`, `go doc`, proxy de modules) dès qu'il sera poussé sur un dépôt GitHub à cette adresse.
- Aucun renommage de chemin d'import ne sera nécessaire au moment de l'extraction de `sdk/go-plugin` en dépôt séparé, tant que le sous-chemin est préservé ou migré consciemment à ce moment-là.
- Le chemin choisi est cohérent avec la convention Go standard `<hôte>/<organisation ou utilisateur>/<dépôt>`.

## Conséquences négatives
- Si le dépôt distant est un jour déplacé vers une autre organisation ou un domaine dédié au projet (ex. un futur `github.com/patchcord/patchcord`), tous les imports internes et externes devront être migrés — un renommage de module Go est un changement mécanique mais qui touche l'ensemble du code.
- Le chemin choisi lie temporairement l'identité du module au compte GitHub personnel de Lucas plutôt qu'à une organisation dédiée au projet ; ce choix devra être révisé avant toute publication open source large (cf. section 18 du document de vision).
