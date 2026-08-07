# ADR-0071 — Permission d'application `connectors.use`, appliquée

## Statut
Accepté

## Contexte

ADR-0026 a délibérément limité le premier modèle de permissions d'application à
`workflows.run` seul, en reportant `connectors.use` et `capabilities` : *"n'ont
aujourd'hui aucun point d'application dans l'agent... les modéliser maintenant serait
de la validation sans rien à valider"*. `api/app/v1/manifest.schema.json` documentait
explicitement ce report et anticipait une extension additive "le jour où
`connectors.use`... [aura] un point d'application réel".

Ce jour est arrivé : `internal/runs.Continue` résout déjà, pour chaque step lié, un id
de connecteur avant d'appeler `connectors.Resolve` (`internal/runs/runner.go`,
`resolveStepConnector`), et `withRunAuth` (`internal/api/adminauth.go`) a déjà, pour
tout appel à `POST /v1/workflows/{id}/run`, une `auth.Session` validée en main avant de
laisser passer la requête. Il ne manquait qu'un fil entre les deux — aucun mécanisme
nouveau à inventer, contrairement à `capabilities` qui reste hors périmètre (voir
"Conséquences négatives").

Une expression de connecteur (`internal/workflow/expr.go`) peut prendre quatre formes :
`each`, `workflow.inputs.*`, `steps.*.outputs.*`, `bindings.*`. Les deux premières ne
sont connues qu'en cours de run (après qu'un step précédent s'est exécuté, ou pendant
une itération `foreach`) — impossible de connaître à l'avance, au moment de
`POST /v1/workflows/{id}/run`, tous les connecteurs qu'un run va toucher. Un rejet
HTTP immédiat (403 avant `runs.Start`) n'est donc pas généralisable aux quatre formes.

## Décision

**`AppPermissions` gagne `ConnectorsUse []string`**, au même patron que
`WorkflowsRun` (`internal/apps/manifest.go`) : nesting YAML
`permissions.connectors.use`, même validation "pas de chaîne vide" que `workflows.run`.
Contrat publié dans un nouveau `api/app/v2/manifest.schema.json` — `v1` reste
inchangé, octet pour octet, comme trace historique de ce qui a été livré (`api/app/embed.go`
documente cette règle depuis ADR-0026). Aucune migration SQL : `apps.permissions` est
déjà un blob JSON, un nouveau champ Go y transite sans y toucher.

**Vérification uniforme, au moment de la résolution, pas au démarrage du run.**
`runs.ExecuteOptions` gagne `AllowedConnectors *[]string` — `nil` signifie non
restreint (jeton admin, CLI, scheduler, déclencheur webhook : comportement inchangé
pour tout appelant qui a déjà accès complet aujourd'hui). `resolveStepConnector`
vérifie l'id de connecteur résolu contre cette liste blanche, juste après
`workflow.ResolveConnector` et avant `connectors.Resolve` — un seul point de contrôle
qui couvre les quatre formes d'expression identiquement. Un connecteur refusé échoue le
step (`runs.ErrConnectorNotPermitted`), pas la requête HTTP : cohérent avec le fait
qu'un run resté sans connecteur résolvable échouait déjà de la même façon avant cette
décision.

**Le fil entre la session HTTP et `ExecuteOptions` reste dans `internal/api`, jamais
dans `internal/runs`.** `internal/runs` ne doit pas dépendre d'`internal/auth` : la
session résolue par `appSessionAllowsRun` est stockée dans le contexte de la requête
(`internal/api/context.go`, nouveau) par `withRunAuth`, puis relue par
`startRunAndRespond` (`internal/api/workflows.go`) qui construit
`AllowedConnectors: &session.Permissions.ConnectorsUse` avant d'appeler
`runs.Continue`. `handleWebhookTrigger` ne passe jamais par `withRunAuth` — il reste
donc toujours non restreint, sans conséquence puisque
`internal/workflow/compile.go`'s `validateNoConnectorBoundStep` interdit déjà tout step
lié à un connecteur sur trigger `webhook`/`schedule`.

**Pas de nouveau package `internal/permissions`.** La vérification est un
`slices.Contains` sur une liste d'ids arbitraires fournie par la session — rien de
commun avec le vocabulaire fermé de permissions de greffon (ADR-0072) qui justifierait
une abstraction partagée. `internal/permissions/` reste un `.gitkeep` réservé, non
rempli, comme depuis le commit initial.

## Conséquences positives

- Ferme exactement le trou qu'ADR-0026 anticipait, sans introduire de mécanisme
  nouveau : réutilise le point d'application déjà existant pour `workflows.run` et le
  point de résolution déjà existant pour un connecteur.
- Extension additive et non-cassante du contrat public (`api/app/v2`), conforme au
  non-négociable #5.
- Aucune violation de frontière de package : `internal/runs` reste ignorant
  d'`internal/auth`.

## Conséquences négatives

- **Changement de comportement à la mise à jour.** Une application déjà installée dont
  le manifeste ne déclare pas `permissions.connectors.use` verra désormais tout step
  lié à un connecteur échouer dans les workflows qu'elle a par ailleurs le droit de
  lancer — alors que rien n'était vérifié avant cette décision. C'est le même défaut
  que `workflows.run` (rien déclaré = rien autorisé), et un choix explicitement
  confirmé plutôt qu'une régression accidentelle : l'opérateur d'une telle application
  doit mettre à jour son manifeste après la mise à jour de l'agent.
- La vérification n'a lieu qu'au moment de la résolution du connecteur, en cours de
  run — un run peut donc démarrer (`202 Accepted`) puis échouer plus tard sur un step
  précis, jamais en amont via un 403 explicite au lancement.
- `permissions.capabilities` (§15.4) reste hors périmètre : aucun point d'application
  n'existe pour elle, et lui en inventer un serait une décision distincte, plus lourde
  (un vrai capability broker, §15.6).
