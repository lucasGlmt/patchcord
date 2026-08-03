# ADR-0031 — Étapes conditionnelles via un attribut `if` sur le step, pas une action dédiée

## Statut
Accepté

## Contexte
Lucas voulait ajouter un mécanisme de condition (et, plus tard, de boucle `foreach`) aux workflows, et hésitait entre deux formes :

- une action dédiée (`uses: cond.if@1`) qui recevrait la condition en entrée ;
- un attribut au niveau du step, orthogonal à `uses`/`with`, qui gate l'exécution de l'action déjà déclarée par le step (`uses: text.uppercase@1` + `if: ...`).

La première forme pose un problème de fond : une action, au sens du vocabulaire du projet (CLAUDE.md section 3), est une "opération atomique exécutable par un workflow", avec entrées/sorties/capacités/timeout propres, exécutée par un greffon. Une condition n'est pas une capacité métier exécutée par un greffon — c'est du contrôle de flux interne au moteur de workflows. En faire une action forcerait soit un greffon "noop" fictif dont la seule raison d'être serait de satisfaire le contrat `Uses`, soit un cas spécial dans `internal/runs` pour reconnaître `cond.if@1` et le traiter différemment de toute autre action — les deux cassent la frontière "le core ne connaît aucune capacité métier concrète" (non-négociable #3) ou la cohérence du modèle d'action.

C'est une décision d'architecture au sens de CLAUDE.md section 6 : elle touche le format public des workflows (`api/workflow` au sens large — le schéma YAML est une frontière contractuelle), et un choix structurant qui serait coûteux à défaire une fois des workflows publiés en dépendant (ADR-0008, immutabilité).

## Décision
`if` est un champ de `workflow.Step` (`internal/workflow/definition.go`), au même niveau que `uses`, `with` et `connector` — jamais une action. Sa valeur est soit un booléen littéral, soit une expression `${{ ... }}` qui doit elle-même se résoudre en booléen (les trois formes déjà supportées : `workflow.inputs.<key>`, `steps.<id>.outputs.<key>`, `bindings.<name>`) ; `workflow.Validate` rejette toute autre forme littérale (chaîne, nombre...) à l'installation, avant qu'un run ne démarre.

`workflow.ResolveIf` évalue `if` à l'exécution, avant la résolution de `with`/`connector` et avant tout appel à l'action. Un step sans `if` s'exécute toujours (valeur par défaut : vrai). Un step dont `if` se résout à `false` est enregistré `skipped` sans jamais invoquer son action — mais, contrairement à un step sauté parce qu'un step précédent a échoué, il **n'arrête pas le run** : `internal/runs/runner.go` (`Continue`) passe au step suivant normalement. Un `if` qui échoue à se résoudre (référence invalide, type non booléen) échoue le run entier, exactement comme une expression `with`/`connector` malformée aujourd'hui.

Un step `skipped` ne produit aucune sortie enregistrée (`stepOutputs`) — référencer `${{ steps.<id>.outputs.<key> }}` pour un step sauté échoue le run, au même titre qu'un step qui n'a jamais tourné.

## Conséquences positives
- Le modèle d'action reste cohérent : toute action déclarée par `uses` est une vraie capacité de greffon, jamais un artefact du moteur.
- `if` se comporte exactement comme `connector` — un attribut de step résolu par le même langage d'expression, la même validation à la compilation — donc aucune nouvelle grammaire à apprendre ni à maintenir dans `expr.go`.
- Un `foreach` futur suivrait le même patron (attribut de step plutôt qu'action), gardant le format de workflow cohérent.

## Conséquences négatives
- `internal/runs/runner.go` gagne un cas de sortie supplémentaire dans sa boucle principale (skip sans échec du run) — la machine à états de `Step` autorisait déjà `Pending -> Skipped` (utilisée pour les steps jamais atteints après un échec), mais ce chemin l'exerce désormais aussi pour un skip volontaire, ce qui mérite une lecture attentive de `Continue` pour qui modifie cette fonction plus tard.
- Un step qui dépend, via `${{ steps.<id>.outputs.<key> }}`, d'un step potentiellement sauté par `if` échoue le run à l'exécution plutôt qu'à l'installation — le moteur ne fait pas d'analyse statique de "ce step peut-il être sauté" avant de valider une telle référence.
