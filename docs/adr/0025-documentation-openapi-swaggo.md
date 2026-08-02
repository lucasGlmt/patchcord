# ADR-0025 — Documentation OpenAPI générée avec swaggo

## Statut
Accepté

## Contexte
ADR-0024 introduisait les deux premiers vrais endpoints métier de l'API
publique (`POST /v1/workflows/{id}/run`, `GET /v1/runs/{id}`) et notait
explicitement l'absence de documentation OpenAPI comme un manque hérité
(même `/v1/system/health` et `/v1/runs/{id}/events`, livrés avant, n'en
avaient pas). Le non-négociable #5 du projet exige que les frontières
publiques soient "contractuelles : Protobuf / JSON Schema / OpenAPI,
versionnées" — l'API HTTP est l'une des trois, avec le protocole de
greffons (déjà couvert par `api/plugin/v1`, Protobuf) et le format des
workflows. Lucas a demandé de fermer ce point avec swaggo, pour que la doc
reste générée depuis le code plutôt que maintenue à la main en parallèle.

## Décision

**swaggo (`swag init`) génère un spec Swagger 2.0 depuis des annotations en
commentaire au-dessus des handlers**, exactement l'esprit "automatisation"
demandé : la doc ne peut pas diverger silencieusement du code tant que
quelqu'un pense à mettre à jour les annotations en même temps que le
handler — pas de garantie absolue, mais un couplage bien plus fort qu'un
fichier `openapi.yaml` maintenu séparément.

**Annotations sur les fonctions-usine (`handleHealth`, `handleRunWorkflow`,
`handleGetRun`, `handleRunEvents`), pas sur les closures `http.HandlerFunc`
qu'elles retournent.** swag fonctionne uniquement par lecture de
commentaires Go, sans introspection réelle du routage — associer les tags
`@Router` à la fonction nommée que `router.go` appelle est le seul choix
possible avec le style "factory + closure" déjà en place dans ce package, et
suffit : swag n'a besoin de rien d'autre pour générer un spec correct.

**Sortie dans `api/agent/`, en JSON et YAML uniquement (pas de `docs.go`
généré).** `api/agent/` est l'emplacement que la structure du dépôt
réservait déjà pour les contrats publics HTTP (CLAUDE.md section 2,
symétrique à `api/plugin/v1/` pour le protocole de greffons). Le mode
"tout-en-un" de swaggo génère aussi un `docs.go` avec un `init()`
enregistrant le spec pour `swaggo/http-swagger` — délibérément omis
(`--outputTypes json,yaml`) : servir le spec ne demande qu'un
`//go:embed`, exactement le patron déjà utilisé par `migrations/embed.go`
pour les fichiers SQL. Ajouter `swaggo/http-swagger` comme dépendance
runtime pour ce seul besoin aurait été plus de poids que nécessaire ; le
composer plus tard (pour une UI Swagger interactive) reste possible sans
rien défaire de cette passe.

**Le spec généré est servi tel quel à `GET /v1/openapi.json`**
(`internal/api/router.go`, `handleOpenAPISpec`), embarqué via
`api/agent/embed.go` — un utilisateur de `@patchcord/sdk` ou un outil tiers
(Swagger Editor, Postman, un futur générateur de client) peut le récupérer
sans lire le code Go. Pas d'UI Swagger interactive dans cette passe (voir
"Hors scope").

**`api/agent/swagger.json`/`.yaml` sont committés, jamais modifiés à la
main** — même règle que `api/plugin/v1/plugin.pb.go` pour Protobuf.
Régénération via `make swagger` (nouvelle cible, symétrique à `make proto`),
qui suppose `swag` installé (`go install
github.com/swaggo/swag/cmd/swag@latest`) plutôt que vendorisé dans
`go.mod` — même choix déjà fait pour `buf`/`protoc-gen-go`, absents de
`go.mod` et simplement supposés présents sur le PATH pour `make proto`.

**`--parseInternal` est obligatoire.** swag exclut par défaut tout code
sous un dossier `internal/` (à l'image de la visibilité Go elle-même) ;
sans ce drapeau, `swag init` ne trouverait ni les annotations ni les
modèles (`runSummary`, `runStep`, `healthResponse`, `runWorkflowRequest`),
puisque tout `internal/api` vit précisément là.

## Explicitement hors scope
- UI Swagger interactive (`swaggo/http-swagger`) — servir le JSON brut
  suffit à l'objectif "automatiser la doc" ; l'ajouter plus tard ne
  nécessite aucune reprise de ce qui est fait ici.
- OpenAPI 3.x — swaggo produit du Swagger 2.0 (qui est historiquement la
  spec "OpenAPI 2.0") ; migrer vers 3.x supposerait soit un autre
  générateur, soit `swag`'s support 3.x encore jeune ; pas de besoin
  identifié qui le justifie aujourd'hui.
- Génération d'un client HTTP à partir du spec (ex. `openapi-generator`) —
  `@patchcord/sdk` reste écrit à la main pour l'instant (ADR-0024) ; le
  spec généré ici est ce qui rendrait une telle génération possible plus
  tard, mais ne la déclenche pas.
- Documentation des futurs endpoints (`/v1/workflows`, `/v1/runs` en liste,
  `/v1/plugins`, `/v1/connectors`...) — ils n'existent pas encore ; leurs
  annotations swag arriveront avec eux, au même rythme que le reste de la
  phase 5.

## Conséquences positives
- Ferme un manque explicitement noté dans ADR-0024, avant que la surface
  HTTP ne grossisse encore (plus facile à instaurer maintenant, sur quatre
  endpoints, que rétroactivement sur une douzaine).
- Aucune nouvelle dépendance runtime : `swag` est un outil de génération,
  au même titre que `buf` — le binaire de l'agent n'importe que
  `embed` (stdlib) pour servir le spec.
- Le spec vit à l'endroit que la structure du dépôt annonçait déjà
  (CLAUDE.md section 2), cohérent avec `api/plugin/v1/`.

## Conséquences négatives
- La qualité de la doc dépend de la discipline à tenir les annotations à
  jour — rien ne fait échouer `go test`/`go vet` si une annotation diverge
  du comportement réel du handler (contrairement à un changement de
  signature Protobuf, qui casse la compilation).
- Swagger 2.0 (pas OpenAPI 3.x) : certains outils tiers plus récents
  attendent nativement 3.x et nécessitent une conversion.
