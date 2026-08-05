# ADR-0064 — Serveur MCP local : découverte du catalogue, validation de workflows, scaffold

## Statut
Accepté

## Contexte

ADR-0062 a donné à chaque action/connecteur une description et un JSON
Schema ; ADR-0063 a donné à ce schéma un premier consommateur
(`workflow.Validate`). Les deux ont été écrits en nommant explicitement la
suite : ADR-0062 concluait *« Un éventuel serveur MCP consommant ce
catalogue pour un agent de développement — cette décision est un
prérequis pour cette piste, pas la piste elle-même. »*

L'objectif reste celui posé dès le début de cette série de décisions :
qu'un agent de code (Claude Code, Codex) puisse construire un bundle/app
Patchcord de façon largement autonome, sans halluciner d'id d'action ni
de nom de champ, et sans lire le code source d'un greffon pour savoir ce
qu'il attend.

## Décision

**Un serveur MCP (Model Context Protocol) local, exposé par une nouvelle
sous-commande `patchcord mcp serve`, transport stdio uniquement.** Un
agent de code enregistre un serveur MCP local comme un sous-processus
qu'il lance et dialogue par stdin/stdout — exactement le modèle que
`patchcord mcp serve` sert. Aucun listener HTTP, aucun port : cohérent
avec le principe « le cloud n'est jamais requis » (ADR-0007), appliqué
ici à la relation inverse — ce serveur n'exige jamais qu'un cloud ou même
un port réseau existe pour qu'un agent construise un bundle.

**`github.com/modelcontextprotocol/go-sdk` (SDK officiel, maintenu avec
Google, v1.7.0) comme nouvelle dépendance directe.** Même raisonnement
que grpc/protobuf déjà acceptés pour le protocole de greffons (ADR-0013) :
réimplémenter MCP à la main pour économiser une dépendance n'aurait aucun
sens pour un protocole normalisé. Empreinte transitive mesurée
concrètement (import du seul sous-package `mcp`, pas `auth`/`oauthex`,
qui ne sont jamais utilisés) : `google/jsonschema-go`, `segmentio/asm`,
`segmentio/encoding`, `yosida95/uritemplate/v3`, `golang.org/x/oauth2`,
`golang.org/x/time` — six dépendances indirectes supplémentaires, aucune
utilisée directement.

**Nouveau package `internal/mcpserver`**, sur le modèle
`internal/runtime` (agent HTTP long-vivant) vs `internal/cli/serve.go`
(wrapper Cobra fin) : les handlers d'outils sont de la vraie logique
métier, testable directement contre une base de test, jamais dans
`internal/cli` lui-même. `internal/mcpserver` devient un troisième
consommateur de `internal/plugins`/`internal/workflow`/`internal/runs`/
`internal/apps`/`internal/bundles`, aux côtés de la CLI et de l'API HTTP
(ADR-0005) — jamais un endroit où leur logique se dédouble.

**Dix outils**, en trois familles :
- **Découverte** (`list_plugins`, `list_actions`, `describe_action`,
  `list_connectors`, `describe_connector`) — lecture seule, directement
  au-dessus du catalogue enrichi par ADR-0062.
- **Workflows** (`validate_workflow`, `list_workflows`,
  `get_workflow_source`) — `validate_workflow` réutilise tel quel
  `workflow.Parse`/`workflow.Validate` (ADR-0063) contre le catalogue
  réel, et rapporte un brouillon invalide comme un résultat normal
  (`valid`/`error` dans la sortie), jamais comme un échec d'outil : c'est
  exactement ce que cet outil sert à produire, un `IsError: true` aurait
  découragé l'agent de simplement lire le message et corriger.
- **Scaffold** (`scaffold_app`, `scaffold_bundle`) — seuls outils à effet
  de bord du serveur. Explicitement un choix plus large que la
  recommandation initiale de rester en lecture seule : la justification
  n'est pas que l'agent ne pourrait pas le faire autrement (il le peut,
  via son propre outil Bash et cette même CLI), mais qu'un scaffold
  multi-fichiers gagne davantage à être un appel structuré unique qu'une
  commande shell composée — contrairement à `plugin install`/
  `workflow install`, que l'agent lance déjà aussi bien via Bash.

**`plugins.FindAction`/`plugins.FindConnector`** (nouveaux, à côté de
`KnownActions`/`KnownConnectorTypes`) retrouvent un descripteur complet
et l'id du greffon qui le contribue, à partir d'un id d'action ou d'un
type de connecteur — lookup qui manquait, `plugins.Get` ne fonctionnant
que par id de greffon. Mêmes conventions que `ErrNotInstalled`
(`ErrActionNotFound`/`ErrConnectorNotFound`).

**Mapping erreur/résultat du SDK réutilisé tel quel.** `mcp.AddTool`
générique peuple automatiquement `StructuredContent` depuis la valeur de
sortie typée, et transforme une `error` Go retournée par un handler en
`CallToolResult{IsError: true}` — chaque handler de ce serveur se contente
donc de retourner une erreur wrappée (`fmt.Errorf("describe_action: %w",
err)`), même convention que le reste du dépôt, sans jamais construire
`CallToolResult` à la main. Seul `validate_workflow` déroge à cette
règle par nécessité (voir ci-dessus).

## Explicitement hors scope (différé, pas oublié)

- Transport HTTP/Streamable — stdio suffit au modèle d'intégration visé
  (subprocess local), pas de raison de l'ajouter sans besoin concret.
- Servir le contenu de `docs/book` comme outil ou ressource MCP — un
  agent travaillant dans ce monorepo y a déjà un accès filesystem direct.
  Un consommateur tiers sans le dépôt cloné reste un besoin réel mais
  distinct, non traité ici.
- Élargir la liste d'outils à d'autres opérations d'écriture
  (`plugin install`, `workflow install`, `bundle install`...) — seul le
  scaffold a justifié l'exception au principe « l'agent peut déjà tout
  faire en Bash », pour la raison structurelle donnée plus haut.

## Conséquences positives

- Ferme l'arc ouvert par ADR-0062/ADR-0063 : le catalogue enrichi et sa
  validation ont enfin un canal d'accès pensé pour un agent de
  développement, pas seulement pour `workflow.Validate` en interne.
- `internal/mcpserver` étant un pur consommateur de services déjà
  existants, aucune logique métier n'a dû être dupliquée ni réinventée
  pour cette fonctionnalité.
- Le mapping erreur/résultat du SDK élimine toute construction manuelle
  de `CallToolResult` dans les dix handlers, sauf l'unique exception
  documentée.

## Conséquences négatives

- Nouvelle dépendance directe et son empreinte transitive, dans un dépôt
  qui a jusqu'ici gardé sa liste de dépendances délibérément courte.
- `scaffold_app`/`scaffold_bundle` introduisent le seul effet de bord de
  ce serveur — leur risque est le même que n'importe quel outil MCP à
  écriture de fichiers (soumis au flux d'approbation propre au client MCP
  qui l'appelle), mais c'est une surface qu'un serveur strictement lecture
  seule n'aurait pas eue.
- `patchcord mcp serve` est une quatrième famille de commande longue durée
  du binaire (aux côtés de `serve`/`dev`/`run watch`), avec sa propre règle
  fragile à respecter (logger sur stderr, jamais stdout) — un refactor futur
  qui unifierait la construction des loggers CLI sans y penser casserait ce
  serveur silencieusement.
