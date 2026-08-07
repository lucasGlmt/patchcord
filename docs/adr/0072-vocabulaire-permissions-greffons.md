# ADR-0072 — Vocabulaire de permissions de greffon, validé

## Statut
Accepté

## Contexte

Le champ `Permissions []string` existe à trois endroits depuis les débuts du protocole
de greffons — `PackageManifest` (`internal/plugins/manifest.go`), le handshake live
(`internal/plugins/handshake.go`), le SDK (`sdk/go-plugin/plugin.go`) — sans jamais
être vérifié. Le SDK le dit explicitement avant cette décision : *"Declarative only in
this version: the agent does not yet enforce them."* ADR-0013 flaguait déjà ce manque
comme conséquence négative acceptée ; ADR-0042 scope le champ à un usage d'affichage
("`permissions` y est déclaré pour être affiché avant le lancement du process").

Dans la pratique, une seule convention est réellement utilisée : la chaîne littérale
`"network.outbound"`, déclarée à l'identique par les 4 greffons d'exemple qui touchent
le réseau (`http`, `postgresql`, `mysql`, `openai`). Le document de vision (§9.1)
esquisse un style plus riche et namespacé (`network.outbound`, `secrets.read:postgresql`)
mais uniquement en prose illustrative, jamais codifié.

Cette décision ne construit pas de sandboxing réel : aucun mécanisme n'existe pour
empêcher un greffon déclarant `network.outbound` de lire aussi le système de fichiers —
c'est le rôle du futur "capability broker" (§15.6), une décision séparée et plus lourde,
hors périmètre ici.

## Décision

**Vocabulaire dans `api/plugin/v1`, pas dans `internal/plugins` ni dans
`sdk/go-plugin` seul.** Nouveau fichier `api/plugin/v1/permissions.go` (écrit à la
main, pas généré par `buf`) : `type Permission string`, constante
`PermissionNetworkOutbound`, préfixe paramétré `PermissionSecretsReadPrefix =
"secrets.read"`, fonction `ValidatePermission(s string) error`. `api/plugin` est déjà
le seul package importé indépendamment par le core (`internal/plugins/handshake.go`) et
par `sdk/go-plugin` (module Go séparé, ADR-0066) — le placer ailleurs violerait soit le
non-négociable #4 (un greffon ne dépend que du protocole public et du SDK, jamais
d'`internal/`), soit forcerait le core à dépendre du SDK.

**Type Go, pas enum proto.** Un nouveau scope reconnu ne doit pas exiger un bump de
`CurrentProtocolVersion` : ça ne change rien à la forme du message sur le fil
(`HandshakeResponse.permissions` reste `repeated string`), seulement ce que
`ValidatePermission` accepte.

**Trois points de vérification, rejet immédiat, pas d'avertissement.**
`internal/plugins.ParsePackageManifest` (le `manifest.json` packagé),
`internal/plugins.Handshake` (la réponse RPC live — nécessaire séparément du premier
point : `catalog.go` persiste `Permissions` depuis le handshake, pas depuis
`PackageManifest`, les deux peuvent diverger), et `sdk/go-plugin`'s `newServer` (retour
immédiat au développeur du greffon, avant même l'ouverture du listener gRPC). Un échec
à l'un de ces points isole uniquement le greffon concerné —
`internal/plugins/supervisor.go`'s `launchAndHandshake` traite déjà tout échec de
handshake ainsi (id manquant, version de protocole incompatible...), cohérent avec le
non-négociable #7.

**Pas de revalidation du catalogue déjà stocké.** Cohérent avec ADR-0068 : un greffon
déjà installé dont les permissions ont été persistées avant cette décision continue de
s'afficher sans problème. Seul un nouveau handshake — réinstallation, ou relance au
redémarrage de l'agent — applique la validation.

## Conséquences positives

- Ferme l'écart entre ce qu'un `manifest.json` déclare et ce qui est réellement
  persisté : les deux points de vérification (paquet + handshake live) couvrent le cas
  où un process de greffon renvoie des permissions différentes de celles déclarées
  statiquement.
- Un développeur de greffon reçoit un signal immédiat et local (`Serve` échoue au
  démarrage), avant même de tenter d'installer le greffon dans l'agent.
- `GET /v1/plugins` et `plugin list` exposent désormais les permissions au même titre
  que `plugin inspect`/`plugin install` et l'outil MCP `list_plugins` — plus de
  divergence entre surfaces (non-négociable #8).

## Conséquences négatives

- **Risque de relance.** Un greffon tiers déjà installé, dont le handshake live
  renvoie une permission non reconnue par ce vocabulaire, échouera désormais à
  (re)démarrer — à la réinstallation ou au redémarrage de l'agent — alors qu'il
  fonctionnait avant cette décision. Comportement voulu (politique "pas de trappe
  silencieuse" déjà appliquée aux autres champs du handshake), pas une régression
  accidentelle : confirmé explicitement plutôt qu'un simple avertissement.
- Reste de la validation de forme, pas de l'enforcement : rien n'empêche un greffon de
  faire autre chose que ce qu'il déclare. Le capability broker (§15.6) qui apporterait
  un vrai contrôle reste une décision distincte, non prise ici.
- Le vocabulaire initial (`network.outbound`, `secrets.read:*`) est volontairement
  minimal — basé uniquement sur ce que les greffons d'exemple utilisent aujourd'hui et
  ce que le document de vision liste explicitement. Toute extension future doit rester
  additive dans `api/plugin/v1/permissions.go`.
