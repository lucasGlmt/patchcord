# ADR-0034 — Connecteurs et catalogue de greffons exposés en HTTP, inférence du type de connecteur pour les bindings

## Statut
Accepté

## Contexte
Lucas veut filmer des vidéos de démonstration de Patchcord et a demandé une refonte du dashboard (`apps/examples/dashboard`) pour en faire un vrai poste de pilotage : plus de modale imbriquée une fois qu'il y a beaucoup d'actions, un sélecteur visuel pour choisir un connecteur de binding plutôt que de taper du JSON, une page CRUD connecteurs, et un lifting visuel/motion pour la vidéo.

En creusant le code existant, deux manques bloquaient ces demandes côté backend :

1. **Les connecteurs n'avaient aucune surface HTTP.** `internal/connectors` et `patchcord connector ...` (create/list/inspect/test/remove) sont complets depuis l'ADR-0020/0021/0022/0023, mais `internal/api` ne les exposait pas — le commentaire de doc du SDK TypeScript le disait explicitement. Une page CRUD connecteurs dans le dashboard exigeait donc du vrai travail backend, pas seulement une nouvelle page React.
2. **Rien ne déclare le type de connecteur qu'une étape attend.** Le protocole de greffon (manifeste) attache à un greffon la liste de ses types de connecteurs et de ses actions, mais aucun lien explicite action → type de connecteur. Pour proposer un vrai `<select>` de connecteurs compatibles au lieu d'un id en texte libre, ce type doit être **inféré**.

## Décision

### Connecteurs et greffons en HTTP
`internal/api/connectors.go` ajoute `GET/POST /v1/connectors`, `GET/DELETE /v1/connectors/{id}`, `POST /v1/connectors/{id}/test`, en réutilisant tel quel `internal/connectors` (aucune logique dupliquée). Le corps JSON d'un connecteur (`connectorSummary`) ne transporte jamais une valeur de secret résolue — seulement le type/la clé de chaque référence, comme `connector inspect` en CLI (ADR-0009/0020/0021).

`POST /connectors/{id}/test` a besoin d'appeler un greffon en cours d'exécution. Plutôt que de faire comme le CLI (qui lance un superviseur éphémère pour la durée d'une commande), une nouvelle interface `api.ConnectorTester` (satisfaite par duck typing par `*plugins.Supervisor`, même pattern que `Deps.Executor` / ADR-0021) réutilise le superviseur déjà démarré et long-vivant sous `patchcord serve` (`internal/runtime/agent.go`).

`internal/api/plugins.go` ajoute `GET /v1/plugins` (id, version, types de connecteurs, ids d'action déclarés) — minimal et en lecture seule, juste ce qu'il faut pour peupler un sélecteur de type de connecteur dans le formulaire de création, pas une gestion complète des greffons par API.

`internal/connectors.Create` gagne un sentinel `ErrInvalidConnector` (même rôle que `workflow.ErrInvalidInputs`) pour que les handlers HTTP distinguent une erreur de validation (400) d'une erreur de persistance (500), sans changer le comportement ni les messages côté CLI.

### Inférence du type de connecteur pour un binding
Plutôt que d'étendre le protocole de greffon (un champ `connectorType` par action, qui toucherait une frontière publique versionnée), le type est **inféré côté serveur** : pour une étape dont `connector` vaut exactement `"${{ bindings.<nom> }}"`, `internal/api/workflows.go` retrouve le greffon installé qui contribue son `uses`, et prend son (unique) type de connecteur déclaré. `GET /workflows/{id}` expose ça en plus (`steps[].binding_name`, `steps[].connector_type`, et un nouveau tableau dédupliqué `bindings[]`, même logique que `inputs[]`) — additif, aucun changement de version de schéma. Une expression de connecteur qui n'a pas cette forme exacte (ex. `${{ workflow.inputs.x }}`) n'a pas d'identité statique à proposer avant un run — elle reste hors du sélecteur, gérée par un champ JSON avancé côté dashboard.

**Limite acceptée** : si un greffon déclare plus d'un type de connecteur, l'inférence est ambiguë et laissée vide (aucun des greffons actuels — openai/http/postgresql/mysql — n'en déclare plus d'un).

### Édition d'un connecteur dans le dashboard
Pas de commande `update` en base (ADR-0020 l'a explicitement écarté). Le bouton "Modifier" du dashboard fait `delete` puis `create` avec le même id, avec un avertissement explicite dans l'UI sur la fenêtre où le connecteur n'existe plus, et un état d'erreur qui garde les valeurs saisies si la recréation échoue après la suppression — décision produit de Lucas, pas seulement une contrainte technique.

### Dashboard : navigation par route plutôt que par modale empilée
`apps/examples/dashboard` passe d'onglets + `Dialog` imbriqués (liste → détail → run) à une vraie navigation (`react-router-dom`, en `HashRouter` — pas `BrowserRouter`, pour rester servable tel quel par `handleServeApp`'s simple `http.FileServer` une fois l'app installée, sans fallback SPA côté serveur à construire) : `/workflows`, `/workflows/:id` (page à deux colonnes : étapes + panneau "Run" avec sélecteurs de binding), `/runs` (nouveau, exploite `client.runs.list` déjà présent dans le SDK mais inutilisé jusqu'ici), `/connectors`, `/apps`. `framer-motion` habille les transitions (courtes, 150–250ms, cf. `theme.ts`/`motion.tsx`) — page fade, apparition en cascade des étapes, pulsation du statut "running".

### Bug trouvé et corrigé pendant le test manuel
Le test manuel de bout en bout (agent réel + dashboard en `npm run dev`) a révélé que `DELETE /v1/connectors/{id}` échouait silencieusement depuis le navigateur ("Failed to fetch") : `withCORS` (`internal/api/router.go`) n'autorisait que `GET, POST, OPTIONS` dans `Access-Control-Allow-Methods`, jamais mis à jour quand `DELETE` a été branché sur le mux. Invisible en `go test` (aucun test n'exerçait un vrai préflight CORS de navigateur) — corrigé, avec un test dédié (`TestRouter_CORSAllowsDelete`) qui aurait échoué avant le correctif.

## Conséquences positives
- Ferme l'écart documenté du SDK ("connectors n'a pas d'implémentation serveur") sans toucher `internal/connectors` ni son modèle de données.
- L'inférence de type de connecteur est un pur calcul côté API, réversible et sans coût — aucune frontière publique (protocole de greffon) n'a dû changer pour l'obtenir.
- Le dashboard démontre maintenant, avec de vraies données (agent + greffons + connecteur réels), le chemin complet : lister → ouvrir un workflow → choisir un connecteur par `<select>` → lancer → voir la progression en direct → gérer les connecteurs sans CLI.
- Le bug CORS trouvé en testant manuellement confirme l'utilité de la méthode de travail CLAUDE.md §8 (analyse statique d'abord, mais validation runtime avant de considérer un changement terminé) : un `go test` vert ne suffisait pas ici.

## Conséquences négatives
- L'inférence de type de connecteur est une heuristique (action → greffon propriétaire → son unique type déclaré), pas un contrat garanti par le protocole — un greffon qui déclarerait un jour plusieurs types de connecteurs redeviendra ambigu tant que le protocole n'aura pas de champ explicite par action.
- `HashRouter` laisse des URLs avec `#/...` plutôt que des chemins propres — accepté pour éviter d'ajouter un fallback SPA côté `internal/apps`/`handleServeApp`, à revisiter si une vraie navigation `BrowserRouter` devient nécessaire ailleurs.
- Le CRUD connecteurs par HTTP hérite du même manque d'authentification que le reste de l'API publique aujourd'hui (ADR-0026) — un navigateur qui atteint l'agent peut créer/supprimer/tester n'importe quel connecteur sans restriction, cohérent avec l'état actuel mais à garder en tête avant un déploiement serveur multi-utilisateurs.
