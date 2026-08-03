# ADR-0032 — Étapes `foreach` : attribut de step, sémantique "map", agrégation dans une ligne unique

## Statut
Accepté

## Contexte
`workflows/examples/http_api.yaml` extrait une liste de usernames via `json.jsonpath@1` (`$[*].username`) puis tente de la passer telle quelle à `text.uppercase@1`, qui attend une chaîne — pas une liste. C'est le cas concret qui a motivé la discussion : la plupart des actions attendent un type scalaire, alors qu'une action précédente (parsing JSON, requête HTTP paginée...) produit naturellement une liste.

Comme pour `if` ([ADR-0031](0031-etapes-conditionnelles-if.md)), une option écartée d'emblée était une action dédiée (`each.map@1` ou similaire) : itérer une liste n'est pas une capacité métier exécutée par un greffon, c'est du contrôle de flux interne au moteur — même raisonnement, même conclusion.

Une contrainte concrète a pesé sur la conception : `run_steps` a `PRIMARY KEY (run_id, step_id)` ([migrations/0003_workflows.sql](../../migrations/0003_workflows.sql)) — une seule ligne par step et par run. Représenter chaque itération par sa propre ligne aurait exigé soit une migration de schéma (nouvelle table `run_step_iterations`), soit un `step_id` synthétique (`shout[0]`, `shout[1]`...), les deux ajoutant une complexité de persistance, d'événements SSE ([ADR-0019](0019-evenements-temps-reel-sse-par-scrutation.md)) et d'API non justifiée à ce stade — le moteur de workflows est encore en phase 3 (CLAUDE.md section 9).

Lucas a par ailleurs indiqué vouloir, plus tard, une policy d'erreur déclarative au niveau du workflow (stop / ignore / retry / déclenchement d'un autre workflow), explicitement reportée. En son absence, `foreach` doit se comporter en *fail-fast*, exactement comme l'échec d'un step normal aujourd'hui — pas de mécanisme de reprise ou d'ignorance ad hoc inventé localement pour ce seul cas, qui serait à défaire quand la vraie policy arrivera.

C'est une décision d'architecture au sens de CLAUDE.md section 6 : elle étend le format public des workflows, dans la continuité directe de l'ADR-0031.

## Décision
`foreach` est un champ de `workflow.Step` (`internal/workflow/definition.go`), au même niveau que `uses`, `with`, `connector` et `if` — jamais une action. Sa valeur est soit une liste littérale, soit une expression `${{ ... }}` qui doit se résoudre en liste ; `workflow.Validate` rejette toute autre forme à l'installation.

**Sémantique "map".** `internal/runs/runner.go` (`Continue`) exécute l'action du step une fois par item, séquentiellement, et agrège les sorties de chaque appel dans des listes, sous les mêmes clés que l'action retournerait normalement — `steps.<id>.outputs.<key>` devient un tableau au lieu d'un scalaire. Aucune contrainte de type n'est imposée sur l'action enveloppée : ce n'est pas un "map vers string", c'est un map générique, cohérent avec n'importe quelle action existante ou future.

**Accès à l'item courant.** À l'intérieur du `with` du step qui déclare `foreach` — nulle part ailleurs (`if`, `connector`, le `foreach` lui-même, le `with` d'un autre step) — l'expression `${{ each }}` résout vers l'item en cours d'itération (`workflow.ExprContext.Each`/`HasEach`, `workflow.ResolveForeach`). Cette restriction est vérifiée à la compilation (`Validate`), pas seulement à l'exécution : `${{ each }}` ailleurs échouerait de toute façon à chaque run avec "used outside a foreach iteration" — autant le rejeter à l'installation.

**Connecteur.** Résolu une seule fois avant la boucle, jamais par item — un connecteur ne varie pas d'une itération à l'autre.

**Persistance.** Une seule ligne `run_steps` par step foreach, sans migration de schéma : `input` enregistre la liste brute des items itérés (utile pour rejouer/déboguer), `output` les listes agrégées en cas de succès, rien en cas d'échec — exactement le même invariant "input persisté, output jamais persisté sur échec" qu'un step normal ([runner.go](../../internal/runs/runner.go), le chemin `resolveErr != nil` déjà en place avant cet ADR).

**Échec.** Le premier item qui échoue arrête tout le run (fail-fast), sans exécuter les items suivants ni les steps suivants — comportement identique à l'échec d'un step normal. L'erreur nomme l'index de l'item fautif (`item %d: %w`). Aucune policy de retry/ignore/continue-on-error n'existe : ce sera l'objet d'un futur ADR quand la policy d'erreur déclarative de Lucas sera conçue.

**Budget de temps.** Toutes les itérations partagent le même `ExecuteOptions.StepTimeout` que le step dans son ensemble — pas de budget par item. Une longue liste a besoin d'un `--step-timeout` dimensionné pour l'ensemble des appels, pas d'un mécanisme de budget par itération (non construit, pour la même raison de simplicité que ci-dessus).

**Liste vide.** Ne produit pas d'erreur : le step réussit après zéro appel, avec une sortie `{}` — impossible d'inventer les clés de sortie d'une action qui n'a jamais tourné.

**Observabilité.** Aucune ligne ni événement SSE par itération pour cette version — seulement le step dans son ensemble (`pending` → `running` → `succeeded`/`failed`). Un suivi par itération (quel item a échoué, retry ciblé) rejoint directement la policy d'erreur différée ; le construire maintenant serait prématuré par rapport à ce travail non encore engagé.

## Conséquences positives
- Résout le cas motivant (`http_api.yaml`) sans aucune migration de schéma ni nouvel événement SSE.
- Cohérence totale avec l'ADR-0031 : même famille d'attribut de step, même langage d'expression étendu plutôt que dupliqué, mêmes invariants de persistance sur échec.
- Le modèle d'action reste intact : aucune action "itération" fictive n'entre dans le catalogue de capacités des greffons.
- Le format de sortie (`{clé: [v1, v2, ...]}`) ne nécessite aucune nouvelle syntaxe d'accès en lecture — `${{ steps.<id>.outputs.<key> }}` fonctionne sans changement, qu'il s'agisse d'un step normal ou d'un step foreach.

## Conséquences négatives
- Une erreur dans un item au milieu d'une longue liste ne laisse aucune trace des items déjà réussis dans `run_steps` (l'`output` n'est jamais persisté sur échec, par cohérence avec le comportement des steps normaux) — seul le message d'erreur indique l'index fautif. Un opérateur qui veut savoir "quels items sont passés avant l'échec" doit relire les logs de l'agent, pas `run inspect`.
- Un item potentiellement sauté par un `foreach` en amont (liste vide) et référencé ensuite via `${{ steps.<id>.outputs.<key> }}` échoue le run à l'exécution plutôt qu'à l'installation — même limite déjà actée pour `if` par l'ADR-0031, le moteur ne fait pas d'analyse statique de "cette référence sera-t-elle toujours satisfaite".
- Le budget de temps partagé entre toutes les itérations peut surprendre : une liste plus longue que prévu peut faire échouer un step par timeout global là où chaque item individuel aurait eu largement le temps de s'exécuter seul.
