# ADR-0068 — Le catalogue de greffons ignore une entrée illisible plutôt que d'échouer entièrement

## Statut
Accepté

## Contexte

ADR-0062 a fait passer `plugins.actions`/`plugins.connectors` d'un tableau
JSON d'identifiants nus à un tableau de descripteurs riches
(`ActionDescriptor`/`ConnectorDescriptor`), en rupture de wire format nette
(`protocol_version` 1 → 2). Cet ADR notait explicitement : *« Stockage
inchangé — toujours un blob JSON dans une colonne TEXT existante […] donc
aucune migration de schéma SQL. »*

Ce que cette phrase n'anticipait pas : sans migration de données, une
entrée de catalogue déjà installée sous protocole v1 reste stockée avec ses
`actions` comme un tableau de chaînes nues. `plugins.List` — la fonction
dont dépendent `plugin list`, `KnownActions`, `KnownConnectorTypes`,
`FindAction` et `FindConnector` — décodait chaque ligne dans le nouveau
type `[]ActionDescriptor` (une struct) et retournait une erreur dès la
première ligne non conforme :

```
list plugins: decode actions: json: cannot unmarshal string into Go value
of type plugins.ActionDescriptor
```

Constaté concrètement le 2026-08-06 : les greffons de référence embarqués
(`text`, `json`, `encoding`, `http`, `mysql`, `time`) avaient été installés
par `SeedEmbedded` (ADR-0059) avant la bascule vers le protocole v2.
ADR-0059 précise lui-même que `SeedEmbedded` *« n'upgrade jamais »* une
entrée déjà seedée. Les deux décisions sont individuellement correctes
mais se combinent en un trou : une entrée de catalogue peut rester figée
indéfiniment sur un ancien protocole, et sa présence casse alors **tout**
`plugin list` — y compris pour des greffons installés après elle et
parfaitement lisibles, comme c'était le cas ici pour
`fr.glmtsolutions.patchcord-imap-plugin`.

## Décision

**Une ligne de catalogue dont `connectors`/`actions`/`permissions` ne
décode pas dans la forme Go actuelle est journalisée puis ignorée par
`List`, pas remontée comme erreur bloquante.** Un greffon dont l'entrée
reste lisible ne doit jamais être invisible ou inutilisable à cause d'un
autre greffon dont l'entrée ne l'est plus.

Mécanisme : une erreur de décodage typée (`catalogDecodeError`, interne à
`internal/plugins`) porte le `plugin_id` et le champ en cause.
`scanCatalogEntry` la produit ; `List` la détecte via `errors.As` pour
décider de sauter la ligne (avec un `slog.Warn` nommant le greffon) plutôt
que d'interrompre l'itération. `Get` — qui cible un greffon précis par id
— continue de faire remonter l'erreur telle quelle : là, il n'y a rien
d'autre à préserver, et l'appelant a explicitement demandé cette entrée.

Le message d'erreur nomme le greffon et suggère l'action corrective
(`patchcord plugin uninstall <id>`, puis réinstallation depuis un greffon
compilé contre le protocole courant) plutôt que de laisser remonter
l'erreur `encoding/json` brute — celle-ci ne dit ni quel greffon est en
cause, ni quoi faire.

Ce que cette décision ne fait pas : elle ne migre pas les données, ne
réinstalle rien automatiquement, et ne réintroduit pas la reconciliation
qu'ADR-0059 a explicitement refusée (un greffon désinstallé par
l'utilisateur ne doit pas revenir tout seul). Elle rend seulement le point
de lecture du catalogue tolérant à une ligne qu'il ne peut plus interpréter
— l'utilisateur reste responsable de désinstaller/réinstaller le greffon
concerné, mais peut désormais le découvrir sans que ça bloque le reste.

## Conséquences positives

- Un greffon installé et fonctionnel reste utilisable même si un autre,
  installé avant une rupture de protocole (ADR-0062 aujourd'hui, une
  future rupture similaire demain), a une entrée de catalogue devenue
  illisible.
- `plugin list`, `workflow.Validate` (via `KnownActions`) et
  `connectors.Create` (via `KnownConnectorTypes`) ne peuvent plus être mis
  hors service dans leur ensemble par une seule entrée périmée.
- L'erreur nomme le greffon fautif et l'action corrective, au lieu d'un
  message `encoding/json` sans contexte — diagnostic en un coup d'œil
  plutôt qu'une plongée dans `catalog.go`.

## Conséquences négatives

- Un greffon dont l'entrée est illisible disparaît silencieusement de
  `plugin list` (visible seulement dans les logs) plutôt que d'échouer
  bruyamment — un compromis délibéré, mais qui peut surprendre un
  utilisateur qui ne consulte pas les logs.
- Ne résout pas la cause structurelle : une future rupture de protocole
  wire-incompatible laissera de nouveau des entrées gelées dans leur
  ancienne forme, faute de migration de données ou de mécanisme de
  réconciliation entre `SeedEmbedded` (ADR-0059) et un changement de
  `protocol_version`. Cette décision absorbe le symptôme (le catalogue ne
  doit pas tomber en entier), pas la cause (aucune stratégie de migration
  de catalogue au travers d'une rupture de protocole).
