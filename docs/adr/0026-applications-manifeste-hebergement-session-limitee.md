# ADR-0026 — Applications : manifeste, hébergement statique et session limitée (tranche minimale)

## Statut
Accepté

## Contexte

Le SDK TypeScript (`sdk/typescript/src`) couvre déjà `workflows.run` + SSE (ADR-0024),
et `apps/examples/dashboard` est un exemple Vite déjà fonctionnel contre cette surface
— mais tout le pan "applications" du document de vision (§7.6, §9.3, §15.4) restait
vide : `internal/auth`, `internal/apps`, `api/app` ne contenaient qu'un `.gitkeep`. Le
non-négociable #8 exige que la CLI et les applications passent par les mêmes services
internes que l'API publique, et §15.4 est explicite : une application ne doit **jamais**
recevoir les pleins pouvoirs de l'agent — elle déclare ses permissions dans un manifeste
et reçoit une session limitée à celles-ci.

Portée retenue avec Lucas, dans le même esprit que ADR-0020/0021 pour les connecteurs :
**tranche verticale minimale**, pas le modèle de permissions complet de §15.4
(`workflows.run` + `connectors.use` + `capabilities`) d'un coup. `connectors.use` et
`capabilities` n'ont aujourd'hui aucun point d'application dans l'agent — exactement la
situation des permissions de greffons actuelles
(`internal/plugins.CatalogEntry.Permissions`, enregistrées mais jamais vérifiées) — les
modéliser maintenant serait de la validation sans rien à valider.

## Décision

**Manifeste minimal `patchcord-app.yaml`, un seul niveau de permission.** Schéma
contractuel versionné dans `api/app/v1/manifest.schema.json` (non-négociable #5 :
le format des packages est une frontière publique), embarqué via `api/app/embed.go`
au même patron que `api/agent/embed.go`. Seuls `id`, `version` et
`permissions.workflows.run` existent ; `internal/apps.ParseManifest` valide ces champs
à la main (pas de validateur JSON Schema à l'exécution — même choix déjà fait pour la
config des connecteurs).

**`internal/apps` au patron exact de `internal/connectors`.** `Install`/`Get`/`List`/
`Uninstall` sur `(ctx, db *sql.DB, ...)`, `ErrNotFound`/`ErrAlreadyExists`, conflit de
clé primaire détecté via `errors.As(&sqlite.Error)` + masque `0xff`. `Install` lit le
manifeste depuis un répertoire source et enregistre son chemin absolu (`static_dir`,
nouvelle table `apps`, migration `0005_apps.sql`) — aucune étape de packaging ou de
copie : pas de vrai format `.patchcord-app` dans cette passe.

**Sessions en mémoire, non persistées, non révocables (`internal/auth.Store`).** Un
token (`uuid.NewString()`, même génération que les ids de run) mappé à
`Session{AppID, Permissions, IssuedAt}` dans une map protégée par mutex.
`Session.CanRunWorkflow` est la seule vérification. C'est une dette assumée, pas un
oubli : l'agent n'a **aucune** authentification admin ailleurs dans l'API non plus
(`internal/api/router.go`, doc de `withCORS`, le note déjà explicitement) — construire
un cycle de vie de session durci protégerait quelque chose qui n'existe pas encore.

**Vérification additive, jamais par défaut.** `POST /v1/workflows/{id}/run` est
enveloppé par `withOptionalAppSession` (`internal/api/apps.go`) : sans en-tête
`Authorization`, le comportement est strictement identique à avant cette passe (tous
les tests existants de `internal/api`/`internal/cli` passent sans modification). Un
jeton présent mais invalide → 401 ; valide mais workflow hors liste → 403 ; valide et
autorisé → passe au handler existant. `Deps.Sessions *auth.Store` est nil-safe pour
tout appelant qui ne le renseigne pas et ne présente jamais de jeton.

**Hébergement statique par `http.FileServer` per-app, pas de nouveau mécanisme
générique.** `GET /apps/{id}/` (`handleServeApp`) résout l'app par id, puis
`http.StripPrefix` + `http.FileServer(http.Dir(app.StaticDir))` — premier usage d'un
`FileServer` dans le core ; jusqu'ici seul un fichier unique (`api/agent/embed.go`)
était servi. `Access-Control-Allow-Headers` gagne `Authorization` (`withCORS`), sinon
un navigateur bloquerait l'en-tête avant même l'appel réseau.

**Émission de session sans jeton administrateur préalable
(`POST /v1/apps/{id}/sessions`).** Cohérent avec l'absence d'authentification admin
documentée ci-dessus plutôt qu'une nouvelle incohérence : ajouter une protection
partielle uniquement ici, sans qu'elle existe nulle part ailleurs dans l'API, aurait
été un faux sentiment de sécurité.

**`apps/examples/dashboard` devient la première application installée réellement.**
`public/patchcord-app.yaml` (copié dans `dist/` par le build Vite, mécanisme standard
de fichiers statiques, sans script de copie ad hoc) déclare `workflows.run:
[hello_patchcord]`. `main.ts` appelle `POST /v1/apps/dashboard/sessions` avant de
lancer un run ; si l'app n'est pas installée (mode `npm run dev` classique), l'appel
échoue silencieusement (404) et le comportement reste celui d'avant cette passe —
aucune régression du chemin de développement documenté dans `index.html`.

## Explicitement hors scope (différé, pas oublié)

- `connectors.use` et `capabilities` dans le manifeste — pas de point d'application
  dans l'agent aujourd'hui, cf. Contexte.
- TTL, révocation, persistance des sessions — `internal/auth.Store` redémarre à vide
  avec l'agent.
- Vrai format de package `.patchcord-app` (`patchcord app pack`) — `Install` lit un
  répertoire tel quel.
- `patchcord app dev` (rechargement à chaud pendant le développement).
- Authentification admin globale de l'agent — dette déjà documentée avant cette passe
  (ADR-0024, `router.go`), non créée ni aggravée ici.

## Conséquences positives

- Ferme le dernier grand vide de la phase 5 (roadmap CLAUDE.md §9) : les quatre
  packages placeholder (`internal/auth`, `internal/apps`, `api/app`) ont maintenant un
  contenu réel, testé, et une application de référence tourne dessus de bout en bout
  (vérifié manuellement : install → hébergement → session → run autorisé → 403 sur un
  workflow hors liste → 401 sur un jeton invalide).
- Réutilise strictement les patrons déjà validés par les connecteurs et les greffons
  (domaine + CRUD, permissions en `[]string`, génération d'id par `uuid.NewString()`),
  aucune nouvelle abstraction introduite pour l'occasion.
- Rétrocompatible à 100 % : aucune route existante ne change de comportement en
  l'absence d'un en-tête `Authorization`.

## Conséquences négatives

- Une session perdue au redémarrage de l'agent oblige toute application ouverte à en
  redemander une — acceptable pour une première tranche, pas pour un usage serveur
  multi-utilisateurs futur.
- `POST /v1/apps/{id}/sessions` n'est protégé par rien : n'importe qui capable
  d'atteindre l'agent peut émettre une session pour n'importe quelle app installée.
  Sans gravité tant qu'aucune authentification admin n'existe ailleurs, mais devra être
  revu en même temps qu'elle.
- Le modèle de permissions à un seul niveau (`workflows.run`) devra être étendu de
  façon additive (nouveau champ, pas de rupture) le jour où `connectors.use` ou
  `capabilities` auront un point d'application réel — pas de garantie que cette
  extension soit triviale.
