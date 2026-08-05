# ADR-0063 — Validation du `with:` d'un step contre l'`input_schema` déclaré de son action

## Statut
Accepté

## Contexte

ADR-0062 a donné à chaque action un `input_schema` (JSON Schema) mais a
explicitement différé son exploitation : *« Exploitation des schémas par
`workflow.Validate` pour type-checker l'`input:` d'un step contre
`input_schema` — ce ADR pose le mécanisme de déclaration, pas encore son
application au moteur de workflows. »*

Jusqu'ici, `workflow.Validate` (`internal/workflow/compile.go`) ne
vérifiait que l'existence de `step.Uses` parmi `knownActions` — une erreur
de champ dans `with:` (type incorrect, champ requis manquant) n'était
découverte qu'à l'exécution du step, potentiellement bien après
l'installation du workflow. Cette décision ferme cet écart.

Complication propre à ce moteur : un `with:` peut contenir des expressions
`${{ ... }}` non résolues (`${{ steps.x.outputs.y }}`,
`${{ workflow.inputs.z }}`) dont la valeur/le type réel n'est connu qu'à
l'exécution (`internal/runs` les résout, jamais `internal/workflow`).
Toute vérification de type doit donc tolérer un champ dont la valeur est
une expression, sans jamais prétendre en connaître le type.

## Décision

**Deux passes indépendantes dans `validateInputSchema`
(`internal/workflow/schema.go`), pas une validation unique du document
entier :**

1. **Présence des champs requis, toujours vérifiée à la main.** Chaque nom
   de `input_schema["required"]` doit être une clé de `with`, que sa
   valeur soit un littéral ou contienne une expression n'importe où dans
   son arbre — la présence est connue au validate-time même quand le
   contenu ne l'est pas.
2. **Type des champs sans expression, délégué à une vraie lib JSON
   Schema.** Pour chaque clé de `with` présente aussi dans
   `input_schema["properties"]`, si son arbre entier ne contient aucune
   expression (vérifié récursivement, même parcours que
   `validateValueExpressions`), la valeur entière est comparée à
   `properties[clé]`. Une expression n'importe où dans l'arbre neutralise
   la vérification de **tout** le champ — pas de vérification partielle.
   Un `input_schema` absent, sans `required`/`properties`, ou une
   propriété sans `type` reconnu (cas `anySchema` d'ADR-0062) ne
   contraint rien : jamais une erreur en soi.

**`github.com/santhosh-tekuri/jsonschema/v5`**, pas un vérificateur maison,
pour la passe 2 — conforme à la norme dès maintenant plutôt que de
réimplémenter un sous-ensemble de JSON Schema au fil des besoins futurs
(`enum`, `pattern`, `oneOf`...). Contrepartie assumée : la lib attend des
valeurs à la forme `encoding/json` (nombre = `float64`), alors que
`with:` est décodé par `yaml.v3`, qui décode un entier en `int` natif —
déjà le cas géré explicitement par `coerceInputValue`
(`internal/workflow/inputs.go`) pour les inputs de workflow. Une passe de
normalisation (`normalizeNumbers`) convertit `int`/`int64` en `float64`
avant tout appel à la lib. Le schéma compilé est une copie de
`input_schema` dont `required` a été retiré : le laisser tel quel aurait
fait revalider par la lib une présence déjà vérifiée à la main (passe 1),
rejetant à tort un champ requis réellement présent mais expression-valued
— exclu de l'ensemble validé par construction.

**Échec strict**, comme le reste du moteur (`ADR-0022` :
*« un knownTypes vide rejette tout, il ne désactive pas la validation »*)
— pas de mode avertissement. Chaque workflow de référence
(`workflows/examples/*.yaml`) a été revalidé avant ce changement pour
confirmer qu'aucun ne régresse.

**`KnownAction` vit dans `internal/workflow`, pas
`plugins.ActionDescriptor` réutilisé tel quel** — même séparation
qu'ADR-0017/ADR-0022 : `internal/workflow` ne doit jamais importer
`internal/plugins`. `Validate`'s paramètre `knownActions` garde son nom et
sa position, seul le type de valeur change (`struct{}` →
`KnownAction{InputSchema map[string]any}`), gardant le diff minimal et la
sémantique « clé absente = action inconnue » inchangée.

**`plugins.KnownActions` est repurposé, pas dupliqué.** Vérifié par
recherche : cette fonction n'a aucun appelant en dehors de ceux qui
alimentent `workflow.Validate`. Elle retourne désormais
`map[string]workflow.KnownAction` au lieu de `map[string]struct{}` — tous
les appelants de production (`internal/runs`, `internal/bundles`,
`internal/cli`) construisent ce résultat puis le transmettent tel quel
avec `:=`, donc ce changement de type se propage sans aucune modification
de leur code source, seulement de leur signature.

## Explicitement hors scope (différé, pas oublié)

- Vérification profonde partielle autour d'une expression imbriquée (ne
  valider que les parties littérales d'une structure contenant une
  expression quelque part en profondeur) — la simplification v1 exclut
  tout le champ top-level dès qu'une expression y apparaît, où qu'elle
  soit.
- Génération d'aide contextuelle CLI ou de formulaire dashboard à partir
  des mêmes schémas — suivi UI, pas cette décision.
- Un schéma pour les erreurs connues d'une action ou son comportement en
  mode test (vision document §7.4) — toujours non implémenté.

## Conséquences positives

- Une erreur de champ dans `with:` (mauvais type, champ requis oublié) est
  détectée à `workflow validate`/`workflow install`, avant toute
  exécution — même levier qu'ADR-0022 a déjà appliqué à la validation du
  type d'un connecteur.
- Le mécanisme de déclaration posé par ADR-0062 a enfin un consommateur
  côté moteur de workflows, pas seulement côté catalogue.
- Réutilise une vraie norme (JSON Schema) plutôt qu'un sous-ensemble
  maison qui aurait divergé au premier schéma un peu plus riche.

## Conséquences négatives

- Nouvelle dépendance directe (`santhosh-tekuri/jsonschema/v5`) dans un
  package core, avec une conversion `int`→`float64` à maintenir en face du
  choix de `yaml.v3` — un point de bug potentiel si un futur type de
  donnée échappe à `normalizeNumbers`.
- La tolérance aux expressions reste une simplification volontairement
  grossière (tout un champ, jamais un sous-arbre partiel) — un schéma
  imbriqué profondément avec une seule expression enfouie perd toute
  vérification pour l'ensemble du champ englobant.
- Toute la chaîne d'appel `knownActions` (de `internal/plugins` à
  `internal/cli`/`internal/api`, en passant par `internal/runs` et
  `internal/bundles`) change de type en une fois — surface de diff large,
  bien que mécanique et sans changement de logique pour les appelants de
  production.
