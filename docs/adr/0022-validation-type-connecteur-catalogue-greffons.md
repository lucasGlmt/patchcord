# ADR-0022 — Validation du type d'un connecteur contre le catalogue des greffons installés

## Statut
Accepté

## Contexte
ADR-0020 puis ADR-0021 ont chacun explicitement différé le même point, avec la
même justification : *"aucun greffon connecteur n'existe"*, donc rien à valider
contre. Ce n'est plus vrai — les greffons `http`, `openai`, `postgresql` et
`mysql` déclarent désormais chacun un `Contributes.Connectors` dans leur
manifeste (`http.connection@1`, `openai.connection@1`,
`postgresql.connection@1`, `mysql.connection@1`), et `internal/plugins/catalog.go`
le persiste déjà (`CatalogEntry.Connectors`, alimenté par le handshake). Le
morceau qui manquait n'était pas la donnée, seulement le point de contrôle qui
la consulte.

Le doc de vision (section 7.3) liste "sa compatibilité avec certaines actions"
parmi ce qu'un connecteur gère — c'est ce point précis. `connector create`
acceptait jusqu'ici n'importe quel `--type`, y compris une faute de frappe
(`postgresql` au lieu de `postgresql.connection@1`), détectée seulement au
premier binding échoué dans un workflow, potentiellement bien après coup.

## Décision

**`connectors.Create` gagne un paramètre `knownTypes map[string]struct{}` et
rejette tout `connectorType` absent de cet ensemble.** Même répartition des
responsabilités que `workflow.Validate(def, knownActions)` (ADR-0017) : le
package domaine (`internal/connectors`) reste libre de toute dépendance à la
persistance des greffons ou au protocole — c'est l'appelant
(`internal/cli`, futur `internal/api`) qui appelle
`plugins.KnownConnectorTypes(ctx, db)` (nouvelle fonction, symétrique à
`plugins.KnownActions`) et lui passe le résultat. `internal/connectors` ne
peut de toute façon pas importer `internal/plugins` sans créer un cycle :
`internal/plugins` importe déjà `internal/connectors` pour
`ResolvedConnector` (ADR-0021).

**Sémantique stricte, pas permissive : un `knownTypes` vide rejette tout,
il ne désactive pas la validation.** Exactement le même choix que
`workflow.Validate` fait déjà pour `knownActions` — pas de branchement
"validation activée seulement si la map est non vide", qui aurait réintroduit
une trappe silencieuse. Conséquence directe et voulue : `connector create`
exige maintenant d'avoir installé le greffon qui déclare le type visé
*avant* de créer le connecteur (`patchcord plugin install` puis
`patchcord connector create --type ...`) — déjà l'ordre documenté dans les
workflows de démo (`http_httpbin_demo.yaml`, `postgresql_query_demo.yaml`,
etc.), donc aucun changement du flux utilisateur attendu, seulement une
erreur plus tôt et plus claire si l'ordre n'est pas respecté.

**`internal/cli/connector.go`** : `newConnectorCreateCommand` appelle
`plugins.KnownConnectorTypes` juste avant `connectors.Create` et lui passe le
résultat. Le texte d'aide de `--type`, qui disait explicitement "nothing
enforces this yet", est mis à jour pour refléter que c'est désormais imposé.

## Explicitement hors scope (toujours différé)
- `patchcord connector test` (vrai test de connexion délégué à un greffon) —
  reste une extension de protocole à concevoir séparément (ADR-0020).
- Revalidation rétroactive des connecteurs déjà créés si le greffon qui
  déclarait leur type est ensuite désinstallé — même précédent que
  `workflow.Validate`, qui ne revalide jamais rétroactivement une version déjà
  installée quand le catalogue change.
- Typage de `Config` au-delà de chaînes de caractères (toujours différé par
  ADR-0020).

## Conséquences positives
- Ferme le dernier point explicitement différé deux fois (ADR-0020, ADR-0021)
  de la checklist phase 4 sur le modèle de connecteur.
- Une faute de frappe dans `--type`, ou un greffon pas encore installé, est
  détectée à la création du connecteur plutôt qu'au premier binding échoué
  dans un workflow — plus tôt dans le cycle de rétroaction qu'avant.
- Aucune nouvelle dépendance ni cycle d'import : réutilise exactement le
  schéma de répartition (package domaine pur + appelant qui interroge le
  catalogue) déjà validé par `workflow.Validate`/`KnownActions`.

## Conséquences négatives
- `connector create` exige maintenant strictement d'avoir installé un greffon
  qui déclare le type au préalable — un utilisateur qui voulait pré-créer un
  connecteur avant d'avoir le greffon correspondant (par exemple pour préparer
  une configuration à l'avance) n'a plus cette latitude.
- Si un greffon est désinstallé puis qu'un autre greffon différent est
  installé sous un type similaire, les connecteurs existants ne sont jamais
  revalidés rétroactivement — un connecteur peut rester en base avec un type
  qu'aucun greffon installé ne déclare plus, sans qu'aucune commande ne le
  signale explicitement (`connector list`/`inspect` ne comparent pas au
  catalogue courant).
