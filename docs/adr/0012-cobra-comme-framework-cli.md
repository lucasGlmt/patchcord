# ADR-0012 — Cobra comme framework CLI

## Statut
Accepté

## Contexte
La section 11 du document de vision prévoit une arborescence de sous-commandes profonde et amenée à grandir tout au long des phases 1 à 7 (`patchcord serve`, `patchcord plugin list/install/inspect/enable/disable/uninstall`, `patchcord connector list/create/inspect/test/remove`, `patchcord workflow list/validate/install/inspect/run/export`, `patchcord run list/inspect/logs/cancel`, `patchcord app list/dev/pack/install/serve`). Construire et faire évoluer cette arborescence à la main avec le seul package `flag` de la bibliothèque standard demanderait de réimplémenter du code générique (aide, parsing de flags POSIX, dispatch de sous-commandes imbriquées, messages d'erreur cohérents) à chaque nouvelle commande, sans réel bénéfice pour la philosophie "core minimal" du projet.

## Décision
La CLI Patchcord est construite avec `github.com/spf13/cobra` (et sa dépendance `github.com/spf13/pflag`), encapsulée dans le package `internal/cli`. Le point d'entrée `cmd/patchcord/main.go` se limite à appeler `cli.Execute()` — aucune logique de commande n'y est définie, conformément à sa vocation de point d'entrée uniquement.

## Conséquences positives
- L'arborescence de sous-commandes prévue jusqu'en phase 7 peut être ajoutée de façon incrémentale avec un boilerplate minimal par commande.
- L'aide, l'usage et le parsing de flags (y compris flags POSIX longs/courts) sont gérés de façon cohérente et éprouvée par une bibliothèque largement utilisée dans l'écosystème Go (kubectl, docker, hugo...).
- Chaque sous-commande peut recevoir son propre `context.Context` dérivé (`cmd.Context()`), ce qui s'articule naturellement avec la propagation de contexte exigée par les conventions Go du projet (section 7 de CLAUDE.md).

## Conséquences négatives
- Trois dépendances externes (`cobra`, `pflag`, `mousetrap`) entrent dans le core, à surveiller et mettre à jour comme toute dépendance du binaire principal.
- Le code de commande devient couplé aux idiomes de Cobra (`cobra.Command`, `RunE`, `PersistentFlags`), ce qui rendrait coûteuse une migration ultérieure vers un autre framework ou vers la bibliothèque standard.
- Un tout petit surcoût de taille de binaire et de démarrage par rapport à un dispatcher `flag` minimal — négligeable pour un agent local, mais réel.
