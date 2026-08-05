# ADR-0062 — Descripteurs de schéma pour les actions et les connecteurs

## Statut
Accepté

## Contexte

Le doc de vision décrit depuis le début un contenu plus riche que ce que le
protocole de greffons transporte réellement :

- §7.4 : *« Une action déclare : son identifiant ; sa version ; ses
  entrées ; ses sorties ; les capacités requises ; les connecteurs
  compatibles ; ses erreurs connues ; son comportement en mode test ; son
  timeout par défaut. »*
- §7.3 attend d'un connecteur qu'il expose *« sa compatibilité avec
  certaines actions »*, ce qui suppose de pouvoir décrire la forme de sa
  configuration.

`Contributions` (`api/plugin/v1/plugin.proto`) ne porte aujourd'hui que des
identifiants nus : `repeated string connectors`, `repeated string actions`.
`sdk/go-plugin.Action` n'a que `ID()` et `Run()` — aucune forme d'entrée/
sortie, aucune description. ADR-0020 puis ADR-0022 ont chacun explicitement
différé le typage de la configuration d'un connecteur au-delà d'une string,
faute à l'époque de vrai besoin ; ADR-0022 a validé l'existence des types
contre le catalogue, mais jamais leur forme.

Ce trou a deux conséquences concrètes, discutées avec Lucas le 2026-08-05 :

1. **Développement humain** : une erreur de champ dans l'`input:` d'un step
   de workflow n'est détectée qu'à l'exécution (`workflow.Validate` ne
   vérifie que l'existence de l'id d'action), et ni la CLI ni le dashboard
   ne peuvent générer d'aide contextuelle — même limite que celle qu'ADR-0030
   avait déjà résolue côté inputs de workflow, jamais côté actions.
2. **Développement agentique** (cf. mémoire `agent_driven_bundle_dev_mcp` :
   objectif de faire construire des bundles/apps par des agents de code
   comme Claude Code ou Codex) : sans schéma ni description exposés, un
   agent ne peut découvrir la forme d'une action qu'en lisant le code source
   du greffon — aucun mécanisme de documentation structurée à interroger,
   MCP ou pas.

Les deux besoins pointent vers le même mécanisme manquant ; ce n'est pas une
extension du protocole, c'est l'implémentation d'une partie du protocole
déjà spécifiée par le doc de vision et jamais construite.

## Décision

**`Contributions` gagne des descripteurs riches, en rupture nette de
protocole (`protocol_version` 1 → 2).**

```protobuf
message Contributions {
  reserved 1, 2;
  reserved "connectors", "actions";

  repeated ConnectorDescriptor connectors = 3;
  repeated ActionDescriptor actions = 4;
}

message ActionDescriptor {
  string id = 1;                             // "postgresql.query@1"
  string description = 2;                    // une phrase, humaine
  google.protobuf.Struct input_schema = 3;    // JSON Schema
  google.protobuf.Struct output_schema = 4;   // JSON Schema
  uint32 default_timeout_seconds = 5;         // 0 = valeur par défaut de l'agent
}

message ConnectorDescriptor {
  string type = 1;                            // "postgresql.connection@1"
  string description = 2;
  google.protobuf.Struct config_schema = 3;   // JSON Schema, jamais les secrets (ADR-0009)
}
```

Les anciens numéros de champ 1/2 sont réservés plutôt que réutilisés : un
greffon compilé contre le protocole v1 doit produire une erreur de
négociation claire (`agent requires protocol v2, plugin speaks v1 —
recompile against sdk/go-plugin v2`) au moment du handshake, pas un décodage
silencieux ou corrompu. C'est un changement de la forme même d'un champ, pas
un ajout — aucune coexistence gracieuse n'est possible au niveau du wire
format ; `protocol_version` sert ici à échouer proprement, pas à faire
cohabiter les deux générations.

**JSON Schema, transporté par `google.protobuf.Struct`** — même choix que
`api/workflow/schema.json` (précédent déjà établi par §5.6 du doc de
vision : Protobuf / JSON Schema / OpenAPI sont les trois formats de contrat
public) et même encodage que celui déjà utilisé pour les payloads
d'exécution (`ExecuteActionRequest.input`). Pas de nouveau format introduit.

**Obligatoire sur `sdk/go-plugin.Action`, pas une interface optionnelle.**
Cohérent avec le principe déjà appliqué deux fois dans ce dépôt (ADR-0022 :
*« un knownTypes vide rejette tout, il ne désactive pas la validation »*) :
une interface optionnelle façon `ConnectorTester` aurait recréé le même
trou qu'on referme, sous une autre forme. Toute action doit déclarer
`Description()`, `InputSchema()`, `OutputSchema()` pour compiler contre le
SDK v2.

**Actions et connecteurs dans le même changement.** Même mécanisme
(`google.protobuf.Struct` portant du JSON Schema), donc une seule rupture de
protocole plutôt que deux vagues séparées.

**Propagation** : `internal/plugins.Manifest.Actions`/`Connectors` et
`plugins.CatalogEntry.Actions`/`Connectors` passent des `[]string` à des
slices de descripteurs typés côté Go (pas d'exposition directe des types
générés par protobuf, même séparation qu'aujourd'hui). Stockage inchangé —
toujours un blob JSON dans une colonne TEXT existante (`plugins.actions`,
`plugins.connectors`), donc aucune migration de schéma SQL. `KnownActions`/
`KnownConnectorTypes` gardent leur signature actuelle (`map[string]struct{}`
d'ids) : `workflow.Validate` et `connectors.Create` restent inchangés par ce
ADR — l'exploitation des schémas pour valider la *forme* d'une entrée est
un suivi, pas cette décision.

**Greffons de référence mis à jour dans le même changement** :
`text.uppercase@1` en premier (tranche verticale de référence, CLAUDE.md
§8), puis `http`, `postgresql`, `mysql`, `openai`, `time.sleep` — sans eux
rien ne compile contre le protocole v2.

## Explicitement hors scope (différé, pas oublié)

- Exploitation des schémas par `workflow.Validate` pour type-checker
  l'`input:` d'un step contre `input_schema` — ce ADR pose le mécanisme de
  déclaration, pas encore son application au moteur de workflows.
- Génération d'aide contextuelle CLI ou de formulaire dashboard à partir des
  schémas — suivi côté UI/UX, pas ce ADR.
- Un éventuel serveur MCP consommant ce catalogue pour un agent de
  développement (cf. mémoire `agent_driven_bundle_dev_mcp`) — cette
  décision est un prérequis pour cette piste, pas la piste elle-même.
- Schéma dédié pour les secrets d'un connecteur, distinct de
  `config_schema` — les secrets ne transitent toujours nulle part dans un
  contrat déclaratif (ADR-0009) ; à concevoir séparément si le besoin de les
  documenter apparaît.
- Génération automatique du JSON Schema depuis les types Go d'un greffon
  (réflexion/génération de code) — cette décision impose seulement que le
  SDK expose `InputSchema()`/`OutputSchema()`, pas comment un auteur de
  greffon les produit.

## Conséquences positives

- Ferme un écart entre le doc de vision (§7.3, §7.4) et l'implémentation,
  présent depuis les tout premiers ADR du protocole de greffons.
- Un seul mécanisme sert deux besoins distincts : validation plus tôt côté
  humain (même logique qu'ADR-0022, poussée à la forme et plus seulement à
  l'existence) et découverte structurée côté agent de développement, sans
  dupliquer l'un pour l'autre.
- Réutilise des choix déjà établis (JSON Schema comme contrat public,
  `google.protobuf.Struct` déjà en usage) plutôt que d'introduire un
  nouveau format ou une nouvelle convention.
- La réservation explicite des anciens numéros de champ transforme une
  incompatibilité de wire format en une erreur de négociation lisible au
  handshake, plutôt qu'un échec de décodage cryptique en aval.

## Conséquences négatives

- Rupture de wire protocol nette : tout greffon déjà distribué (Homebrew,
  ADR-0057) doit être recompilé contre le SDK v2 et republié — aucune
  période de coexistence entre agent v2 et greffon v1.
- Tous les greffons de référence du dépôt doivent être mis à jour dans le
  même changement avant que quoi que ce soit ne compile contre le
  protocole v2 — un changement plus large qu'une extension additive.
- Écrire un greffon trivial demande désormais plus de code qu'aujourd'hui
  (`Description()`/`InputSchema()`/`OutputSchema()` obligatoires sur chaque
  action) — compromis assumé contre le risque d'actions non documentées.
- Actions et connecteurs cassent en même temps : plus de surface à faire
  évoluer d'un coup que si les deux avaient été séquencés séparément.
