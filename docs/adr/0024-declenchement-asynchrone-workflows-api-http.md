# ADR-0024 — Déclenchement asynchrone d'un workflow via l'API HTTP

## Statut
Accepté

## Contexte
La phase 5 ("SDK TypeScript et applications") commence. Le document de vision
(section 10.2) donne l'exemple d'usage que le SDK doit permettre :

```ts
const run = await client.workflows.run("invoice_analysis", { inputs });
for await (const event of run.events()) { console.log(event.status); }
const result = await run.result();
```

C'est-à-dire : déclencher une exécution renvoie immédiatement un identifiant
de run, que l'appelant peut ensuite observer en flux (`run.events()`,
`/v1/runs/{id}/events`, déjà livré) puis interroger pour son résultat final
(`run.result()`).

`internal/runs.Execute` (le seul point d'entrée d'exécution existant,
utilisé par `patchcord workflow run`) est entièrement **synchrone** : il crée
le run, exécute chaque étape dans le même appel, et ne retourne qu'une fois
le workflow terminé. Un handler HTTP `POST /v1/workflows/{id}/run` qui
appellerait `Execute` directement bloquerait donc pour toute la durée du
workflow — impossible à réconcilier avec l'exemple ci-dessus, où le client
doit pouvoir observer le run pendant qu'il tourne, pas seulement après coup.

Avant cette passe, l'API publique ne couvrait que `/v1/system/health` et
`/v1/runs/{id}/events` (SSE, ADR-0019). C'est la première extension de
surface HTTP depuis.

## Décision

**`internal/runs.Execute` est scindé en `Start` + `Continue`, `Execute`
devenant leur composition.** `Start(ctx, db, workflowID, inputs)` fait
exactement ce qu'`Execute` faisait avant sa boucle d'étapes (charger la
dernière version, `createRun`, transition vers `Running`) et retourne
aussitôt. `Continue(ctx, db, executor, def, run, inputs, bindings, opts)`
est la boucle d'étapes elle-même, inchangée dans son comportement, appliquée
à un run déjà créé. `Execute` appelle `Start` puis `Continue` et retourne le
run — signature, comportement et **tous les tests existants inchangés**,
preuve que la refactorisation ne modifie rien pour les appelants actuels
(CLI, tests). `go test ./internal/runs/...` passe sans aucune modification
des tests d'`Execute` déjà en place.

**`patchcord serve` appelle `Start` de façon synchrone dans le handler HTTP,
puis `Continue` dans une goroutine d'arrière-plan.** Le handler répond dès
que `Start` a persisté le run (`202 Accepted`, id + statut `running`) —
rapide et borné par le `persistTimeout` déjà existant — pendant que
`Continue` continue de tourner et d'écrire sa progression en base.
`runs.WatchRun` (ADR-0019) tolère déjà de s'abrancher sur un run en cours ou
tout juste créé (interrogation depuis une base vide) : aucune modification
nécessaire côté SSE pour que `GET /v1/runs/{id}/events` fonctionne
immédiatement après le `202`.

**Le contexte de `Continue` en tâche de fond n'est jamais celui de la requête
HTTP qui l'a déclenché.** `r.Context()` est annulé dès que la réponse est
écrite — bien avant que le workflow ait fini. `internal/runtime.Agent` crée
désormais un contexte dédié (`runCtx`, annulable), transmis via
`api.Deps.RunCtx`, et l'annule explicitement dans sa séquence d'arrêt
(`Run`), après l'arrêt HTTP mais avant `supervisor.Stop` — un run
d'arrière-plan encore actif est ainsi correctement enregistré `Cancelled`
plutôt que de continuer à appeler des greffons en cours de démontage, ou de
fuir comme une goroutine orpheline. Une défaillance dans `Continue` n'a plus
de requête HTTP à qui la remonter : elle est journalisée
(`api.Deps.Logger`), jamais perdue silencieusement.

**Nouveau endpoint `GET /v1/runs/{id}`** (`internal/api/runs.go`) : renvoie
le run et ses étapes en JSON — le pendant synchrone de `/events`, utilisé
par le SDK pour `run.result()` une fois le flux d'événements terminé (un run
atteint un statut terminal ⇒ le canal de `WatchRun` se ferme, documenté).

**CORS permissif, explicitement provisoire.** `internal/api/router.go`
enveloppe désormais le mux avec `Access-Control-Allow-Origin: *` — sans ça,
un serveur de développement Vite (origine différente de l'agent) ne peut
tout simplement pas appeler cette API depuis un navigateur, ce qui bloquerait
l'exemple d'application que cette passe doit produire. **Ce n'est pas une
frontière de sécurité** : un vrai allowlisting d'origine (`patchcord app dev
--origin ...`, doc de vision section 10.3) et les "sessions limitées" (doc de
vision section 15.4, item explicite de la phase 5) restent à construire —
même logique que `secrets.EnvStore` en ADR-0020, qualifié lui aussi
d'"adaptateur de démarrage, pas la cible finale". Une API locale liée à
`127.0.0.1` sans authentification n'introduit pas une élévation de
privilège nouvelle par rapport à la CLI, qui a déjà un accès complet et sans
authentification à la même base SQLite — un vrai modèle multi-utilisateur
distant relève de la phase 6 ("authentification distante").

## Explicitement hors scope
- `patchcord app dev/pack/install` et l'hébergement statique sous
  `/apps/<name>/` (doc de vision section 10.3) — un origin-restricted dev
  proxy et un mécanisme de packaging sont des questions de conception
  séparées, à traiter une fois cette tranche verticale validée.
- Sessions/permissions applicatives (section 15.4) — voir ci-dessus.
- Spécification OpenAPI de `/v1/*` — n'existe pas non plus pour
  `/v1/system/health` et `/v1/runs/{id}/events`, déjà en place avant cette
  passe ; mérite une passe dédiée couvrant tous les endpoints, pas
  seulement les deux ajoutés ici.
- Interruption réelle d'un run en cours d'exécution en tâche de fond : un
  `run cancel` ne fait toujours que basculer le statut en base
  (`CancelRun`, déjà documenté comme tel) — la goroutine `Continue` en vol
  n'est pas interrompue avant son prochain point de persistance ; elle
  échoue alors proprement (transition invalide détectée par
  `ValidateRunTransition`/`ValidateStepTransition`) sans corrompre l'état,
  mais sans arrêter net le travail en cours.
- Le reste de la surface `client.*` du SDK (section 10.2) :
  `plugins`/`connectors`/`actions`/`apps`/`files`/etc. — seul
  `client.workflows.run` est couvert, à l'image de l'exemple du document de
  vision.

## Conséquences positives
- L'exemple d'usage du SDK TypeScript du document de vision (section 10.2)
  est réalisable tel quel contre un vrai agent, pas seulement en théorie.
- La scission `Start`/`Continue` est non-cassante : `Execute` garde
  exactement son contrat, tous ses tests existants passent sans
  modification — une vérification de régression forte que le comportement
  est préservé.
- Le contexte d'arrière-plan annulé pendant l'arrêt de l'agent évite deux
  problèmes latents distincts : des goroutines orphelines, et des appels de
  greffons après `supervisor.Stop`.
- `internal/runs.WatchRun` n'a nécessité aucune modification : sa conception
  par scrutation depuis une base vide (ADR-0019) supportait déjà ce cas
  d'usage sans le prévoir explicitement.

## Conséquences négatives
- Le CORS permissif est une vraie surface à durcir avant tout déploiement
  au-delà d'un poste de développement local — documenté mais pas encore
  imposé par le code.
- Un `run cancel` déclenché pendant qu'un `Continue` d'arrière-plan est en
  vol ne l'arrête pas immédiatement — voir "Explicitement hors scope".
- Sans OpenAPI, les deux nouveaux endpoints ne sont documentés que par leurs
  commentaires Go et le SDK TypeScript lui-même — pas de contrat
  machine-lisible pour un client tiers non-TypeScript pour l'instant.
