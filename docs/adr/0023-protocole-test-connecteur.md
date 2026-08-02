# ADR-0023 — Extension du protocole de greffons pour `patchcord connector test`

## Statut
Accepté

## Contexte
ADR-0020 différait explicitement `patchcord connector test` : *"un vrai test de
connexion doit être délégué à un greffon (non-négociable #3 — le core ne peut
pas savoir tester une connexion PostgreSQL/HTTP lui-même), et aucun greffon
connecteur n'existe encore"*. Ce n'est plus vrai — `http`, `openai`,
`postgresql` et `mysql` existent, et ADR-0022 vient de fermer le point voisin
(validation du `type` contre le catalogue). Il ne restait que celui-ci.

`connector inspect` existe déjà mais ne prouve rien sur la connexion réelle
(ADR-0020) : il vérifie seulement que les références de secrets *résolvent*,
pas que le mot de passe est le bon ou que l'hôte répond.

## Décision

**Nouvelle RPC `TestConnector` sur `PluginService`** (`api/plugin/v1/plugin.proto`),
troisième RPC du protocole après `Handshake` et `ExecuteAction` — une
extension de frontière publique au sens du non-négociable #5, donc versionnée
comme telle : ajout additif (nouvelle RPC, nouveaux messages), aucune RPC
existante modifiée, rétrocompatible avec un greffon qui ne l'implémente pas
(voir plus bas).

```protobuf
rpc TestConnector(TestConnectorRequest) returns (TestConnectorResponse);

message TestConnectorRequest { ConnectorConfig connector = 1; }
message TestConnectorResponse { bool ok = 1; string message = 2; }
```

**Distinction stricte entre échec de test et erreur RPC — même principe que
`http.request@1` distingue déjà un statut non-2xx (résultat légitime) d'une
requête qui n'a jamais abouti (vraie erreur Go).** Un mot de passe refusé ou
un hôte injoignable est `TestConnectorResponse{Ok: false, Message: "..."}`,
jamais une erreur gRPC — sinon un workflow ou un script qui interroge le
résultat n'aurait aucun moyen de distinguer "le test a tourné et a échoué" de
"le test n'a pas pu tourner du tout". Seul un vrai problème de transport, ou
`codes.Unimplemented` (voir ci-dessous), est une erreur gRPC.

**Support optionnel côté greffon : `codes.Unimplemented` si le greffon ne
gère pas les tests, distingué explicitement d'un test qui tourne et échoue.**
Le SDK (`sdk/go-plugin`) ajoute une interface `ConnectorTester` :

```go
type ConnectorTester interface {
    TestConnector(ctx context.Context, connector ConnectorConfig) error
}
```

`Plugin` gagne un champ `Tester ConnectorTester` optionnel. Un greffon qui ne
le renseigne pas répond `Unimplemented` automatiquement — **changement
additif du SDK, pas cassant** : contrairement au changement de signature de
`Action.Run` en ADR-0021 (qui touchait tout greffon existant), aucun greffon
existant n'a besoin d'être modifié pour continuer à compiler. `err == nil`
veut dire test réussi ; toute erreur devient `Ok: false, Message: err.Error()`
côté serveur SDK — le greffon écrit du Go idiomatique, jamais directement de
`TestConnectorResponse`.

**Routage par type de connecteur, symétrique au routage par action id
d'`ExecuteAction`.** `Supervisor.TestConnector(ctx, connector)` cherche, parmi
les greffons actuellement lancés, celui dont `Contributes.Connectors` contient
`connector.Type` — même boucle que `Supervisor.ExecuteAction` sur
`Contributes.Actions`, dans le même fichier (`internal/plugins/supervisor.go`).
Aucun greffon en cours d'exécution ne déclarant ce type est une erreur (pas de
connecteur "orphelin" testable).

**`patchcord connector test <id>`** (`internal/cli/connector.go`) résout le
connecteur (`connectors.Resolve`, même mécanisme que l'exécution d'action),
lance un Supervisor pour la durée de la commande — même raisonnement que
`workflow run` (ADR-0017) : tester un connecteur appelle un processus greffon
vivant, donc ce n'est pas une commande de lecture de catalogue pure — puis
affiche `OK` ou `FAILED: <message>`. **Un test qui tourne et échoue n'est pas
une erreur de commande** (code de sortie 0, comme `workflow run` qui affiche
un run `Failed` sans faire échouer la commande CLI elle-même) ; seule
l'impossibilité d'attempter le test (connecteur introuvable, aucun greffon
disponible pour ce type, greffon `Unimplemented`) fait échouer la commande.

**Greffons de référence** : `postgresql` et `mysql` implémentent
`ConnectorTester` en ouvrant une connexion et en l'`PingContext`-ant, sans
exécuter de requête — fonctionne qu'une table existe ou non. `http` et
`openai` n'implémentent pas `ConnectorTester` dans cette passe : un "test" TCP
générique pour `http.connection@1` aurait fallu inventer une sémantique
(HEAD ? GET ? quel chemin ?) sans cas d'usage concret pressant ; laissé pour
une passe séparée si le besoin se présente, plutôt que deviné maintenant.

## Explicitement hors scope
- Test de connexion pour `http`/`openai` (pas de sémantique évidente sans cas
  d'usage réel — cf. ci-dessus).
- Timeout dédié pour `connector test`, distinct de celui d'une action — la
  commande hérite du comportement par défaut du Supervisor, comme
  `workflow run` avant l'introduction des timeouts par étape (ADR-0018) ; à
  reconsidérer si un greffon met du temps à échouer proprement (ex. TCP
  connect qui traîne).

## Conséquences positives
- Ferme le dernier point différé de la checklist phase 4 sur le modèle de
  connecteur (ADR-0020 → ADR-0022 → cette ADR).
- `connector inspect` (résolution de secrets) et `connector test` (connexion
  réelle) restent deux diagnostics distincts et non ambigus, comme
  explicitement voulu par ADR-0020.
- Le SDK reste rétrocompatible : aucun greffon existant (`http`, `openai`,
  `text`) n'a eu besoin d'être modifié pour continuer à fonctionner après
  cette extension.
- Preuve de bout en bout par un vrai aller-retour gRPC entre `patchcord
  connector test` et un vrai processus greffon (`internal/plugins/testdata/fakeplugin`
  étendu, plus `postgresql`/`mysql` réels), pas seulement en mémoire.

## Conséquences négatives
- Un greffon tiers écrit avant cette passe doit être recompilé contre le SDK
  mis à jour pour bénéficier de `ConnectorTester` — mais, contrairement à
  ADR-0021, n'est pas *obligé* de changer quoi que ce soit pour continuer à
  fonctionner (additif, pas cassant).
- `http`/`openai` ne sont pas testables via `connector test` dans cette passe
  — un utilisateur de ces greffons doit encore se fier à `connector inspect`
  ou à un vrai appel d'action pour diagnostiquer une mauvaise configuration.
- Pas de timeout dédié : un greffon dont le test de connexion bloque
  longtemps (hôte qui ne répond ni n'échoue) bloque `connector test` d'autant.
