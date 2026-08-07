# ADR-0070 — Métriques Prometheus du core

## Statut
Accepté

## Contexte

Patchcord Core n'avait jusqu'ici aucune observabilité au-delà de
`GET /v1/system/health` (un contrôle de vivacité binaire : la base SQLite
répond ou non) et des logs structurés `slog`. Impossible de voir, sans
dépouiller les logs, le débit de runs, le taux de restart des greffons
supervisés, ou la proportion de schedules manqués — alors que l'agent est
explicitement destiné à tourner en contexte serveur (ADR-0010), où ce genre
de suivi opérationnel est un besoin courant.

Lucas a demandé un système de métriques exposé par l'agent « un peu comme
Prometheus » : à la fois interopérable avec l'écosystème Prometheus/Grafana
standard, et consommable directement par le SDK TypeScript pour construire
des tableaux de bord dans des apps Patchcord.

Note en passant : la section 6 de `CLAUDE.md` mentionne encore « le prochain
[ADR] est 0011 » — une note devenue obsolète au fil des ADR déjà écrits. Le
présent ADR porte le numéro `0070`, en continuité stricte du dernier ADR
existant (`0069`), conformément à la règle réelle de la section 6 (« en
continuité stricte du dernier numéro existant »), qui prime sur cette
parenthèse obsolète.

## Décision

1. **Nouvelle dépendance core : `github.com/prometheus/client_golang`.**
   C'est un outillage générique d'instrumentation (registre de métriques,
   compteurs/gauges/histogrammes, exposition au format texte Prometheus) —
   strictement comparable à `modernc.org/sqlite`, `google.golang.org/grpc`
   ou `robfig/cron`, déjà présents dans `internal/`. Cette dépendance ne
   connaît aucun service métier concret et ne viole donc pas le
   non-négociable #3 (le core ne référence jamais un service métier
   concret).

2. **Double exposition depuis un seul registre en mémoire
   (`internal/metrics.Registry`)** :
   - `GET /metrics` — format texte Prometheus standard
     (`prometheus/client_golang/prometheus/promhttp`), pour un serveur
     Prometheus/Grafana externe auto-hébergé. Cette route sort
     délibérément du préfixe `/v1` que suit chaque autre route JSON de
     l'API : les scrapers Prometheus et l'écosystème plus large (Grafana
     Agent, auto-découverte `kube-prometheus-stack`, etc.) s'attendent
     universellement à trouver `/metrics` à la racine — imposer un chemin
     non standard irait à contre-courant de cet écosystème sans bénéfice
     réel.
   - `GET /v1/system/metrics` — un instantané JSON du même registre, sur
     le contrat `/v1/*` existant (documenté dans le contrat OpenAPI généré
     via `make swagger`), consommé par le SDK TypeScript
     (`client.system.metrics()`) pour construire des tableaux de bord dans
     des apps Patchcord.

   Les deux routes lisent le même `internal/metrics.Registry` — une seule
   source de vérité, deux représentations.

3. **Métriques strictement en mémoire, jamais persistées.** Aucune
   migration SQLite, aucune table de compteurs. C'est le modèle standard
   Prometheus : l'historique vit dans le serveur Prometheus externe, qui
   interroge périodiquement l'agent, pas dans le processus instrumenté
   lui-même. Un redémarrage de l'agent remet tous les compteurs à zéro —
   un non-objectif délibéré, pas un oubli.

4. **Auth admin requise sur les deux routes**, exactement comme les autres
   routes exposant de l'état interne (`withAdminAuth`, ADR-0036) — pas la
   convention « ouvert, on fait confiance au réseau » que certains
   déploiements Prometheus adoptent. Un opérateur qui scrape un agent
   protégé par un token configure ce token dans le bloc `authorization` de
   son `prometheus.yml`.

5. **Périmètre de la première itération** : quatre familles de métriques,
   choisies pour correspondre aux points de persistance/décision déjà
   identifiés dans le code, sans ajouter de nouveau mécanisme de suivi
   parallèle :
   - Runs & steps (`internal/runs/store.go`) : `run_transitions_total`,
     `step_transitions_total` (par statut), durées en histogramme,
     nombre de runs actifs.
   - Superviseur de greffons (`internal/plugins/supervisor.go`) :
     `plugin_running`, `plugin_restarts_total`,
     `plugin_health_check_failures_total`, `plugin_quarantined_total`
     (par `plugin_id`).
   - Scheduler (`internal/scheduler/scheduler.go`) :
     `schedule_fires_total`, `schedule_skipped_total` (par raison),
     nombre de schedules actifs.
   - Connecteurs : `connector_test_total` (par résultat), sur le chemin
     de test à la demande existant (`Supervisor.TestConnector`) — aucun
     contrôle de santé périodique n'a été ajouté, ce n'est pas dans le
     périmètre de cette décision.

   Toutes les métriques sont enregistrées sous le namespace `patchcord_`
   (ex. `patchcord_run_transitions_total`). Les collecteurs standard de
   `client_golang` (mémoire, GC, descripteurs de fichiers du processus Go)
   sont également enregistrés — génériques, pas de lien avec un service
   métier.

6. **Convention d'injection** : `*metrics.Registry` circule par injection
   de constructeur (`NewSupervisor`, `scheduler.NewRunner`,
   `runs.ExecuteOptions`, `api.Deps`), avec repli nil-safe vers un registre
   privé non scrappé (`metrics.OrNoop`) — exactement la même convention que
   `*slog.Logger` partout ailleurs dans ce dépôt. Jamais de singleton
   global : chaque `metrics.Registry` a son propre `*prometheus.Registry`
   privé, ce qui évite toute collision d'enregistrement entre tests
   table-driven qui construisent plusieurs `Supervisor`/`Runner` dans le
   même binaire de test.

## Conséquences positives

- Un Prometheus/Grafana externe peut suivre le débit de runs, la santé et
  le taux de restart des greffons, le taux de schedules manqués, et le
  résultat des tests de connecteurs — sans exporteur personnalisé.
- Le SDK TypeScript expose `client.system.metrics()` pour construire des
  tableaux de bord dans des apps Patchcord, réutilisant les mêmes
  compteurs que l'endpoint Prometheus — une seule source de vérité.
- `client_golang` (registre, `CounterVec`/`GaugeVec`/`HistogramVec`,
  `promhttp`) évite d'écrire à la main le format d'exposition Prometheus,
  l'échappement des labels, ou les calculs d'histogramme.

## Conséquences négatives

- Nouvelle dépendance externe dans `internal/`, avec son propre arbre de
  dépendances transitives (`client_model`, `common`, `procfs`, ...) à
  suivre dans le temps.
- `GET /metrics` à la racine est une exception permanente et délibérée à
  la convention `/v1/*` — un futur outillage qui suppose « toute route vit
  sous `/v1/*` » (une règle de reverse-proxy, un script de génération de
  client) devra traiter ce cas à part.
- Les métriques sont remises à zéro à chaque redémarrage ; un opérateur
  qui veut un historique long terme doit faire tourner en continu un
  serveur Prometheus externe — Patchcord lui-même ne conserve jamais
  d'historique.
- Exiger l'auth admin sur `/metrics` signifie qu'un simple
  `curl http://host:7331/metrics` (le premier réflexe d'un opérateur)
  renvoie 401 dès qu'un token admin existe — un point de friction mineur,
  à documenter clairement (voir `docs/book/src/cli/serve.md`).
