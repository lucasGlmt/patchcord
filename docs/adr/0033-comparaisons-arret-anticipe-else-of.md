# ADR-0033 — Comparaisons dans les expressions, arrêt anticipé (`stop_if_false`) et chaînage `else_of`

## Statut
Accepté

## Contexte
À la suite des ADR-0031 (`if`) et ADR-0032 (`foreach`), Lucas a demandé trois extensions dans la même itération :

1. Des expressions de comparaison dans `if` (ex. `if: "${{ steps.xxx.outputs.value }} >= 2"`), pour piloter une condition sur une valeur numérique/textuelle plutôt que sur un booléen déjà calculé.
2. La possibilité d'arrêter tout le workflow (sans erreur) quand un `if` échoue, plutôt que de seulement sauter le step courant.
3. Un mécanisme d'`else` — discuté dans une session précédente, où l'option d'un bloc `steps:` imbriqué (récursif) avait été écartée par Lucas au profit d'une idée plus légère : un `if` qui "implique" le step suivant. Cette idée a été formalisée en un attribut `else_of: <step_id>` référençant un step antérieur, sans toucher au modèle de `Step` (qui reste plat, un step = une action).

**Comparaisons — tension avec un choix antérieur.** Une discussion précédente sur `switch` avait explicitement écarté l'idée d'enrichir `${{ ... }}` avec des opérateurs, au profit de pousser la comparaison dans une action de greffon (`logic.equals@1`), pour ne pas ouvrir la porte à un mini-langage de script dans le YAML (non-négociable implicite : pas d'exécution de code arbitraire dans un workflow, cf. non-objectifs du document de vision). Lucas a explicitement demandé cette fonctionnalité dans cette itération malgré cette réserve. La décision : l'implémenter, mais avec un périmètre volontairement fermé — voir ci-dessous — pour rester du côté "résolution de valeur", pas "langage d'expression".

## Décision

### Comparaisons
Le contenu d'un bloc `${{ ... }}` peut être une comparaison `<chemin> <opérateur> <littéral>` en plus d'un chemin nu (`internal/workflow/expr.go`, `parseComparison`/`compareValues`). Périmètre strictement fermé :
- 6 opérateurs : `== != > >= < <=` ;
- le membre gauche est un chemin (une des 4 formes déjà supportées : `workflow.inputs.*`, `steps.*.outputs.*`, `bindings.*`, `each`) ;
- le membre droit est un **littéral uniquement** — chaîne entre guillemets, nombre, ou `true`/`false` — jamais un autre chemin (pas de `chemin == chemin`) ;
- pas de composition booléenne (`&&`, `||`, `!`) ;
- `==`/`!=` comparent n'importe quelle paire des trois types (toujours `false` si les types diffèrent) ; les opérateurs d'ordre (`>`, `>=`, `<`, `<=`) exigent deux nombres, sinon erreur explicite plutôt qu'une comparaison lexicographique silencieusement surprenante.

L'opérateur reste **à l'intérieur** du bloc `${{ ... }}` (`"${{ steps.x.outputs.value >= 2 }}"`), pas à l'extérieur comme dans la formulation initiale de la demande (`"${{ steps.x}} >= 2"`) — la règle "toute valeur `with`/`connector`/`if`/`foreach` est soit un littéral, soit une chaîne *entièrement* faite d'une seule expression" (posée dès l'ADR d'origine du moteur) ne change pas. La grammaire de comparaison est branchée au niveau générique de `validateExpression`/`resolveExpression`, donc disponible partout où une expression l'est déjà (`with` compris), pas seulement dans `if`.

### Arrêt anticipé (`stop_if_false`)
`Step.StopIfFalse` (bool, `internal/workflow/definition.go`) modifie ce qui se passe quand `if` vaut `false` : au lieu de sauter uniquement ce step (comportement par défaut, inchangé), il saute ce step **et tous les suivants**, et le run se termine `succeeded` — pas d'erreur, une clause de garde au sens propre. `Validate` rejette `stop_if_false: true` sans `if` (rien à propos de quoi réagir). `internal/runs/runner.go` réutilise exactement le mécanisme de skip-en-cascade déjà en place pour le cas d'échec, piloté par un nouveau booléen local `stopped` distinct de `runErr` — le statut final choisit `RunSucceeded` plutôt que `RunFailed`.

### Chaînage `else_of`
`Step.ElseOf` (string, nom d'un step antérieur) fait sauter ce step si et seulement si le step référencé a "pris sa branche" — combiné en ET avec le propre `if` du step, s'il existe. Un step avec `else_of` mais sans `if` est le "else" inconditionnel de la chaîne. Chaîner `else_of` sur le maillon **immédiatement précédent** (pas toujours le premier) donne un if/elseif/else sans aucune imbrication :

```yaml
- id: case_high
  if: "${{ ... >= 8 }}"
  uses: A
- id: case_mid
  else_of: case_high
  if: "${{ ... >= 5 }}"
  uses: B
- id: case_low
  else_of: case_mid
  uses: C
```

**Bug trouvé et corrigé pendant le développement** (avant tout commit) : une première implémentation ne propageait que l'information "ce step précis a-t-il tourné" (`ranSteps[id]`), pas "sa branche, ou une branche antérieure de sa chaîne, a-t-elle été prise". Résultat observé en test de bout en bout : pour `score=9`, `case_high` tournait, `case_mid` était correctement sauté (son `else_of` pointe vers `case_high`, qui a tourné) — mais `case_low` tournait *quand même*, parce que son `else_of: case_mid` ne voyait que "`case_mid` n'a pas tourné", sans savoir que c'était parce que `case_high`, plus haut dans la chaîne, avait déjà pris la main. Correction : quand un step est sauté via `else_of`, `ranSteps[step.ID]` est mis à `true` (pas seulement quand un step tourne réellement) — propageant l'état "pris" à travers toute la chaîne, pas seulement d'un maillon à l'autre. Un test dédié à une chaîne de 3 maillons (`TestExecute_ElseOfChainOfThreeOnlyRunsOneCase`) fixe ce comportement.

## Conséquences positives
- Couvre if/elseif/else/switch sans toucher au modèle plat de `Step` ni introduire de récursion, de `schema_version` supplémentaire, ou de nouvelle table de persistance — cohérent avec la décision antérieure d'écarter les blocs imbriqués.
- Les comparaisons restent une grammaire fermée (6 opérateurs, chemin contre littéral) — pas un langage d'expression qui grossirait au fil des demandes.
- `stop_if_false` réutilise l'infrastructure de skip-en-cascade existante ; aucun nouveau statut de `Run`/`Step` n'a été nécessaire.

## Conséquences négatives
- `else_of` ne couvre qu'une action par branche — regrouper plusieurs steps sous une même branche exige de répéter la même condition (`if` ou `else_of`) sur chacun, un compromis assumé plutôt qu'une limite cachée (cf. discussion qui a précédé cette décision).
- Le membre droit d'une comparaison ne peut pas être un autre chemin (`steps.a.outputs.x == steps.b.outputs.y`) — hors périmètre pour cette itération, à rouvrir seulement si un besoin concret apparaît.
- La correction de la propagation `else_of` montre que le chaînage à N maillons est plus subtil qu'il n'y paraît ; toute évolution future de ce mécanisme doit être testée avec au moins 3 maillons, pas seulement une paire if/else.
