# ADR-0029 — Résolution récursive des expressions dans les valeurs `with` de type liste/objet

## Statut
Accepté

## Contexte
L'ADR-0017 fixait le langage d'expression `${{ ... }}` à la substitution de valeur entière : une valeur de `with` est soit un littéral, soit une chaîne entièrement composée d'une expression — pas d'interpolation partielle. En pratique, `workflow.ResolveInputs` et `workflow.Validate` n'appliquaient cette règle qu'aux valeurs *top-level* de la map `with` : ils ne descendaient jamais dans les éléments d'une liste (`[]any`) ou d'un objet (`map[string]any`).

Ceci cassait silencieusement toute action dont le schéma d'entrée attend une expression imbriquée dans une liste — le cas de référence étant `text.join@1`, dont l'entrée `values` est une liste de chaînes :

```yaml
values:
  - "Salut, "
  - "${{ steps.shout.outputs.value }}"
```

Le workflow d'exemple `greet_twice` utilise exactement cette forme. Avant ce correctif, l'expression imbriquée dans la liste n'était ni validée à la compilation, ni résolue à l'exécution : `text.join@1` recevait la chaîne `"${{ steps.shout.outputs.value }}"` littéralement et la concaténait telle quelle, au lieu de la sortie réelle de l'étape référencée.

Il s'agit d'une décision d'architecture au sens de CLAUDE.md section 6 : elle précise le contrat public du format de workflow (déjà posé par l'ADR-0017) et affecte toute action à entrée composite, présente ou future.

## Décision
`workflow.ResolveInputs` (résolution à l'exécution) et `workflow.Validate` (validation à la compilation) parcourent désormais **récursivement** les valeurs de `with` : une expression `${{ ... }}` est reconnue et résolue/validée quelle que soit sa position, y compris imbriquée dans un `[]any` ou un `map[string]any`, à n'importe quelle profondeur.

La règle de fond de l'ADR-0017 ne change pas : chaque chaîne individuelle rencontrée pendant ce parcours doit toujours être *entièrement* une expression pour être substituée — l'interpolation partielle à l'intérieur d'une même chaîne (ex. `"prefix-${{ ... }}"`) reste non supportée et continue d'être laissée telle quelle. Seul le périmètre de parcours change : de "valeurs top-level de `with`" à "toutes les valeurs de `with`, à toute profondeur".

## Conséquences positives
- `text.join@1` et toute action future à entrée composite (listes, objets) peuvent référencer des sorties d'étapes ou des entrées de workflow sans contournement.
- La validation à la compilation (`workflow.Validate`) détecte désormais les références d'étape invalides (inconnue, future, forme malformée) imbriquées dans une liste/objet, avant qu'un run ne démarre — cohérent avec la garantie déjà offerte pour les valeurs top-level.
- Aucun changement de syntaxe côté workflow YAML : les workflows qui, comme `greet_twice`, utilisaient déjà cette forme en s'attendant à ce qu'elle fonctionne se comportent maintenant correctement, sans migration.

## Conséquences négatives
- Le moteur d'expressions a une surface légèrement plus grande à maintenir (parcours récursif au lieu d'une simple boucle sur la map `with`), même si la grammaire d'expression elle-même reste inchangée.
- Une valeur `with` très profondément imbriquée coûte désormais un parcours récursif à la résolution et à la validation ; sans intérêt pratique vu la taille attendue des entrées d'action, mais à garder en tête si un schéma d'action très large apparaissait.
