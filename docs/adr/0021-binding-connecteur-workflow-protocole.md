# ADR-0021 — Binding connecteur↔workflow et extension du protocole

## Statut
Accepté

## Contexte
ADR-0020 a livré `internal/connectors` et `internal/secrets` en CRUD autonome, mais a
explicitement différé le point qui les rend réellement utilisables : qu'une étape de
workflow puisse référencer un connecteur, et que sa configuration résolue (config +
secrets) arrive jusqu'au greffon qui exécute l'action. C'est cette passe.

Portée retenue avec Lucas : **binding minimal + démo**. Le mécanisme complet de bout
en bout (protocole, SDK, moteur de workflows, runner), prouvé par une action de
démonstration ajoutée à la lib `text` — pas de vrai greffon métier (HTTP/PostgreSQL,
reste un item séparé de la checklist phase 4) et pas de validation du `type` d'un
connecteur contre le catalogue des greffons installés (reste différé, cf. ADR-0020).

Le seul exemple du document de vision (section 7.5) montre :
```yaml
connector: "${{ bindings.ai_provider }}"
```
sans préciser comment `bindings` est peuplé. Cette passe traite `bindings` comme un
paramètre de run, symétrique à `inputs` : passé via `--binding name=connector-id` sur
`workflow run`, comme `--input k=v`.

## Décision

**`connector:` doit toujours être une expression, jamais un id littéral.** Un workflow
publié est immuable (ADR-0008) ; un id de connecteur figé dans le YAML briserait la
portabilité entre déploiements que `bindings` existe justement pour préserver.
`workflow.Validate` rejette un `step.Connector` non vide qui n'est pas entièrement une
expression `${{ ... }}`. Toute forme d'expression déjà supportée peut résoudre un
connecteur — `bindings.<name>` (le cas courant), mais aussi `workflow.inputs.<key>` ou
`steps.<id>.outputs.<key>` restent des indirections légitimes ; réutiliser
`validateExpression`/`resolveExpression` tels quels évite un validateur plus étroit
spécifique à ce champ. Seule l'absence totale d'indirection est interdite.

**Résolution du connecteur sur le contexte de l'étape (`stepCtx`), pas sur `pctx`.**
`pctx` est un budget de bookkeeping fixe (10s) partagé sur tout le run (déjà noté
comme une limite dans ADR-0018). `secrets.Store` est une interface conçue pour de
futurs adaptateurs (Vault, Keychain) qui feront de vrais appels réseau — les traiter
comme du bookkeeping serait faux dès qu'un tel adaptateur existera, et échapperait en
plus à l'annulation utilisateur (`pctx` dérive de `context.Background()`, jamais de
`ctx`). `stepCtx` (dérivé de `ctx`, borné par `opts.StepTimeout`) est créé une fois par
étape et couvre à la fois la résolution du connecteur et l'appel à l'action — mêmes
règles d'annulation/timeout que l'appel d'action lui-même.

**Résolution de l'input et du connecteur fusionnées dans le même `resolveErr`, avec
écriture unique de `StepRunning`.** La transition `StepPending→StepRunning` n'est
écrite qu'une fois par étape (une deuxième écriture `Running→Running` serait de toute
façon rejetée par `ValidateStepTransition`). Si la résolution des inputs réussit mais
celle du connecteur échoue, `resolvedInput` (déjà calculé) est tout de même persisté
dans l'écriture terminale — pas écrasé par `nil`. Même `stepFailureStatus(err)` que
pour un échec d'action (`StepCancelled` si `ctx` annulé, `StepFailed` sinon) : aucun
nouveau statut, aucune nouvelle transition.

**Emplacement du type partagé `ResolvedConnector`** : dans `internal/connectors`, pas
`internal/runs`, pour qu'`internal/plugins` puisse l'importer sans dépendre
d'`internal/runs` — `Supervisor.ExecuteAction` satisfait `runs.ActionExecutor` par
duck-typing, sans import, et ça reste vrai. `internal/connectors` ne dépend toujours
pas du protocole des greffons : la conversion vers `pluginv1.ConnectorConfig`
(`connectorConfigToProto`) reste dans `internal/plugins/execute.go`, déjà la couche de
traduction `structpb` pour `input`/`output` — un package domaine ne doit pas connaître
le format de transport.

**Extension du protocole** (`api/plugin/v1/plugin.proto`) : `ExecuteActionRequest`
gagne un champ optionnel `ConnectorConfig connector = 3` (message additif,
présence native par pointeur en Go, rétrocompatible dans les deux sens). Le SDK
(`sdk/go-plugin`) répercute ce changement sur `Action.Run`, qui gagne un paramètre
`connector *ConnectorConfig` — **changement cassant du SDK, acceptable avant 1.0** : un
seul greffon d'exemple à mettre à jour.

**Règle générale, pas seulement pour la démo : un greffon ne doit jamais renvoyer un
secret résolu dans la sortie d'une action.** La sortie d'une étape est persistée en
clair dans `run_steps.output` (colonne déjà existante). `text.echo_connector@1`, la
nouvelle action de démonstration, renvoie `type`/`config` d'un connecteur lié mais
**jamais** `Secrets` — le faire casserait exactement la garantie qu'ADR-0009/0020
construisent (les secrets ne doivent jamais atterrir dans la base). Documenté ici
comme principe pour tout greffon consommateur de connecteur futur.

## Explicitement hors scope (toujours différé)

- Validation du `type` d'un connecteur contre les types déclarés par les greffons
  installés (ADR-0020, point toujours ouvert).
- Persistance de l'id du connecteur effectivement utilisé par une étape (pas de
  nouvelle colonne sur `run_steps`, pas de migration dans cette passe — le schéma
  SQLite reste inchangé).
- Le vrai greffon consommateur de connecteur (HTTP/PostgreSQL) — prochain item de la
  checklist phase 4.

## Conséquences positives
- Ferme le point différé d'ADR-0020 : un connecteur créé peut désormais réellement
  atteindre un greffon, prouvé par un aller-retour gRPC/Protobuf réel (pas seulement
  en mémoire) dans `internal/plugins/example_plugin_test.go`.
- L'interdiction des ids de connecteur littéraux évite dès maintenant une dette de
  portabilité qu'il aurait fallu corriger plus tard par une dépréciation, une fois des
  workflows publiés (immuables) existants.
- `internal/connectors` reste indépendant du protocole des greffons ; la traduction
  protobuf reste localisée dans la couche transport (`internal/plugins`), pas dispersée.
- Bug de fuite de contexte trouvé et corrigé pendant l'implémentation (`go vet`) : un
  chemin de retour anticipé dans la boucle du runner pouvait laisser `stepCtx` non
  annulé — corrigé avant de fusionner, exactement le genre de défaut que
  l'exigence de tests/vet de CLAUDE.md est censée intercepter.

## Conséquences négatives
- Un connecteur lié qui n'existe pas (faute de frappe dans `--binding`, ou binding non
  fourni) ne se voit pour l'instant signalé qu'à l'exécution (échec de l'étape), pas à
  l'installation du workflow — même limitation déjà acceptée pour
  `workflow.inputs.<key>`, pas une régression nouvelle, mais toujours un temps de
  détection plus tardif qu'une validation stricte au catalogue le permettrait.
- Le SDK (`Action.Run`) a changé de signature de façon cassante ; tout greffon tiers
  écrit avant cette passe doit être recompilé et mis à jour manuellement.
- Sans persistance de l'id de connecteur utilisé, `run inspect`/`run logs` ne permet
  pas de savoir après coup quel connecteur exact une étape a utilisé — seul son
  comportement observable (via la sortie de l'action, si elle le rapporte) le
  révèle.
