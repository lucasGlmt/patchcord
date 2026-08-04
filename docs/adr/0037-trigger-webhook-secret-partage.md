# ADR-0037 — Trigger `webhook` et vérification par secret partagé

## Statut
Accepté

## Contexte
Après le trigger `schedule` ([ADR-0035](0035-trigger-schedule-scheduler-persistant.md)) et l'authentification admin ([ADR-0036](0036-authentification-admin-jetons-opt-in.md)), Lucas a choisi les webhooks comme troisième chantier de la Phase 6 — précisément parce que l'authentification admin qu'on venait de construire débloquait ce qui était identifié comme le vrai obstacle : exposer un endpoint entrant sans rien pour le protéger. Le document de vision mentionne les trois triggers côte à côte dès la section 2 ("l'agent est capable d'exécuter les mêmes workflows manuellement, par cron ou par webhook"), confirmant que `webhook` est un trigger de premier ordre attendu, pas une extension improvisée.

Un webhook diffère structurellement de `schedule` sur un point central : `schedule` n'a **aucun** appelant (le scheduler se réveille tout seul), alors qu'un webhook **a** un appelant réel — le service tiers qui poste la requête HTTP. Ce point a directement influencé trois décisions prises avec Lucas avant l'implémentation :

1. **Vérification** : un webhook ne peut pas être protégé par le jeton admin de l'ADR-0036 — l'expéditeur externe (GitHub, Stripe, un script, Zapier...) n'en détiendra jamais. Deux options ont été comparées : une signature HMAC du corps (plus robuste, mais suppose que l'expéditeur sait signer — pertinent seulement pour des fournisseurs sophistiqués avec leur propre schéma, ce qui reviendrait à coder une logique métier concrète dans le core, interdite par le non-négociable #3) contre un secret partagé simple comparé en en-tête (universellement compatible, quel que soit l'expéditeur). Lucas a choisi le secret partagé.
2. **Mapping des inputs** : contrairement à `POST /workflows/{id}/run` qui exige l'enveloppe `{"inputs": ..., "bindings": ...}`, aucun expéditeur de webhook réel n'enverra jamais son payload enveloppé de cette façon. Lucas a choisi que le corps JSON brut devienne directement la map `workflow.inputs`.
3. **Inputs requis sans défaut** : interdits pour `schedule` (aucun appelant), Lucas a choisi de les **autoriser** pour `webhook` — le payload entrant joue exactement le rôle du corps d'un run manuel. Seuls les connecteurs liés (`connector:`) restent interdits, faute de map `bindings` fournie par un expéditeur anonyme.

C'est une décision d'architecture au sens de CLAUDE.md section 6 : elle étend le format public des workflows (`trigger.secret_ref`) et la frontière HTTP publique (`POST /v1/webhooks/{id}`, une route qui échappe délibérément à l'authentification admin de l'ADR-0036).

## Décision

**Format.** `workflow.Trigger` (`internal/workflow/definition.go`) gagne `secret_ref` (`secrets.Reference` — même type que les références de secret des connecteurs), significatif uniquement quand `type: webhook`. `workflow.Validate` (`internal/workflow/compile.go`) :
- exige `secret_ref.key` non vide et `secret_ref.type` reconnu (`secrets.ValidateType`, la même validation déjà utilisée par les connecteurs) ;
- rejette `cron`/`on_missed` sur un trigger `"webhook"`, et `secret_ref` sur `"manual"`/`"schedule"` — pas de champ silencieusement ignoré, même discipline que l'ADR-0035 ;
- rejette toute étape avec un `connector:` non vide (`validateNoConnectorBoundStep`, factorisée depuis la règle déjà écrite pour `"schedule"`) ;
- **n'** applique **pas** la règle "input requis sans défaut interdit" de `"schedule"` — un webhook a un vrai appelant.

**Endpoint.** `POST /v1/webhooks/{id}` (`internal/api/webhooks.go`, `handleWebhookTrigger`) :
1. Résout la dernière version installée du workflow (`runs.LatestWorkflow`) ; absente ou trigger différent de `"webhook"` → `404` (même réponse dans les deux cas, pour ne pas donner d'indice sur l'existence du workflow à un appelant sans le secret).
2. Résout `secret_ref` via `secrets.EnvStore` et le compare à l'en-tête `X-Patchcord-Webhook-Token`, en temps constant (`crypto/subtle.ConstantTimeCompare`) — absent ou incorrect → `401`.
3. Décode le corps JSON directement en `map[string]any` — pas un objet JSON → `400`.
4. Démarre le run via `runs.Start`/`runs.Continue`, exactement le même chemin que `handleRunWorkflow`, extrait dans un petit utilitaire partagé `startRunAndRespond` (`internal/api/workflows.go`) pour ne pas dupliquer la logique de démarrage asynchrone + réponse `202` — un run déclenché par webhook n'est pas un type de run différent, juste déclenché différemment, exactement le principe déjà posé par l'ADR-0035 pour `schedule`.

**Jamais admin-gated.** `internal/api/router.go` enregistre `POST /v1/webhooks/{id}` en dehors de `withAdminAuth` — la seule route, avec `withRunAuth`, à ne pas suivre ce mécanisme par défaut, mais pour une raison différente : `withRunAuth` accepte optionnellement un jeton admin en plus d'une session d'app ; `POST /v1/webhooks/{id}` n'accepte **jamais** de jeton admin, uniquement le `secret_ref` propre à ce workflow. Confirmé par un test dédié (`internal/api/webhooks_test.go`, "works even once an admin token exists").

**Aucun nouvel état persisté.** Contrairement à `schedule` (table `schedules`, `next_run_at`), un trigger `webhook` ne nécessite aucune nouvelle table : sa configuration vit entièrement dans la définition du workflow déjà installée, et la vérification se fait entièrement au moment de la requête — pas de synchronisation à l'installation (pas d'équivalent de `scheduler.Sync`).

## Conséquences positives
- Aucune logique spécifique à un fournisseur tiers dans le core (pas de vérification `X-Hub-Signature-256` de GitHub ni équivalent) — respecte strictement le non-négociable #3.
- Compatible avec n'importe quel expéditeur webhook existant sans passerelle intermédiaire : un simple `curl -d '{...}'` avec le bon en-tête suffit.
- Réutilise entièrement l'infrastructure de secrets déjà construite pour les connecteurs (`secrets.Reference`, `secrets.ValidateType`, `secrets.EnvStore`) — aucune nouvelle abstraction de gestion de secret introduite.
- Un run déclenché par webhook est indiscernable d'un run manuel ou planifié dans `patchcord workflow runs`/`GET /v1/runs` — même moteur, même persistance, même modèle d'observabilité.

## Conséquences négatives
- Le secret partagé est un jeton statique, jamais tourné automatiquement, transmis en clair dans l'en-tête à chaque requête (protégé par TLS en déploiement serveur, hors scope de cette session — voir Phase 6 "TLS via reverse proxy") — moins robuste qu'une signature HMAC qui ne fait jamais voyager le secret lui-même.
- Aucune vérification d'intégrité du corps (une signature HMAC couvrirait aussi une altération en transit, pas seulement l'authentification) — accepté comme compromis pour rester compatible avec n'importe quel expéditeur simple.
- Le corps JSON brut devenant directement les inputs, un workflow webhook est structurellement couplé à la forme exacte du payload de son expéditeur — changer de fournisseur ou son format de payload casse le workflow, sans couche de transformation intermédiaire.
- Comme pour `schedule`, un workflow avec une étape liée à un connecteur ne peut toujours pas être déclenché par webhook — reste `"manual"` ou restructuré si un connecteur est requis.
