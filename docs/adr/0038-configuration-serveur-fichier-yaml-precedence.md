# ADR-0038 — Configuration serveur : fichier YAML et précédence flags > env > fichier

## Statut
Accepté

## Contexte
Après le trigger `schedule` ([ADR-0035](0035-trigger-schedule-scheduler-persistant.md)), l'authentification admin ([ADR-0036](0036-authentification-admin-jetons-opt-in.md)) et le trigger `webhook` ([ADR-0037](0037-trigger-webhook-secret-partage.md)), Lucas a proposé Docker comme prochain chantier de Phase 6. En vérifiant l'exemple `docker-compose` du document de vision (section 13.3), celui-ci utilise `patchcord serve --config=/data/config.yaml` — un flag `--config` qui n'existait pas : `serve` n'acceptait jusqu'ici que `--listen` et `--data-dir`, sans fichier ni convention de variable d'environnement (`docs/book/src/cli/configuration.md` l'affirmait explicitement : *"There is no configuration file"*).

Deux options ont été présentées à Lucas : construire un Dockerfile minimal maintenant avec les flags/variables d'environnement déjà existants, en repoussant la configuration fichier à plus tard ; ou construire la configuration serveur d'abord, pour que Docker corresponde exactement à l'exemple du document de vision. Lucas a choisi la seconde.

Le point de conception ayant nécessité un arbitrage explicite était l'ordre de précédence entre les trois sources non codées en dur (fichier, variable d'environnement, flag). Lucas a choisi la convention la plus répandue : un flag explicitement passé l'emporte toujours, puis une variable d'environnement, puis le fichier, puis une valeur par défaut intégrée — plus une source est immédiate au moment du lancement, plus elle est prioritaire.

Le reste des choix découle directement de conventions déjà établies ailleurs dans le projet, sans ambiguïté réelle nécessitant un arbitrage :
- **YAML**, cohérent avec le reste du projet (workflows, manifestes d'app) et avec le nommage `config.yaml` du document de vision.
- **`--config <path>` explicite**, pas de recherche implicite dans des emplacements par défaut (`~/.patchcord/config.yaml` ou équivalent) — cohérent avec le style du projet (rien d'implicite qui surprendrait un opérateur).
- **Échec net si le fichier pointé par `--config` n'existe pas** — jamais ignoré silencieusement.
- **Clés inconnues du fichier rejetées** (`yaml.Decoder.KnownFields(true)`) — même discipline que `workflow.Validate` applique déjà au YAML des workflows : un `liste:` mal orthographié est détecté à l'installation, pas découvert des semaines plus tard.
- **Portée limitée à `serve`** — les autres commandes (`plugin`, `connector`, `workflow`, `run`, `app`) gardent `--data-dir` en flag simple uniquement ; une invocation ponctuelle est déjà suffisamment explicite pour ne pas avoir besoin de configuration en couches.
- **Contenu minimal** — seulement `listen` et `data_dir`, les deux seuls réglages qui existaient déjà comme flags `serve`. Pas de `shutdown_timeout` ni d'autre réglage nouveau : rien n'en justifiait l'ajout dans ce chantier, cohérent avec la discipline déjà pratiquée ailleurs (secret providers, granularité des jetons admin, DAG — tous délibérément différés faute de besoin réel identifié).

C'est une décision d'architecture au sens de CLAUDE.md section 6 : elle introduit un nouveau composant du core (`internal/config`) et un mécanisme public (le format du fichier de configuration, désormais documenté), même si son contenu reste volontairement minimal pour l'instant.

## Décision

**Nouveau package `internal/config`** (`config.go`) :
- `Config{Listen, DataDir string}` — chaque champ vide signifie "cette source n'a pas d'avis", condition que `Merge` exploite.
- `Load(path string) (Config, error)` — lit et parse le fichier YAML, `KnownFields(true)` pour rejeter toute clé inconnue.
- `FromEnv() Config` — lit `PATCHCORD_LISTEN`/`PATCHCORD_DATA_DIR`.
- `Merge(base, override Config) Config` — un champ non vide de `override` remplace celui de `base`, un champ vide le laisse intact. Appelé une fois par source, de la moins prioritaire à la plus prioritaire.

**`internal/cli/serve.go`** orchestre les quatre couches, dans cet ordre croissant de précédence :
1. Défauts intégrés (`defaultListenAddr = "127.0.0.1:7331"`, `defaultDataDir = "./data"`).
2. `--config <path>`, si fourni (`config.Load`).
3. Variables d'environnement (`config.FromEnv`).
4. Flags — appliqués seulement si `cmd.Flags().Changed("listen")`/`Changed("data-dir")` est vrai, pas simplement parce que la variable Go contient une valeur (elle contient toujours quelque chose, y compris le défaut du flag lui-même) : c'est cobra qui sait distinguer "l'utilisateur a tapé ce flag" de "c'est resté à sa valeur par défaut", donc c'est `internal/cli` — pas `internal/config` — qui possède cette couche.

**Tests de précédence** (`internal/cli/serve_test.go`) : chaque scénario fixe une adresse `--listen` délibérément invalide via une seule source à la fois et vérifie, via le message d'erreur de `runtime.NewAgent` (qui répète l'adresse tentée, `"bind listen address %q"`), que c'est bien la source de plus haute précédence qui a été retenue — sans jamais laisser le serveur réellement démarrer.

## Conséquences positives
- Docker peut maintenant suivre l'exemple exact du document de vision (`--config=/data/config.yaml`), au lieu d'un Dockerfile qui s'en écarterait.
- La précédence flags > env > fichier > défauts est la convention la plus familière pour un opérateur venant d'autres outils — rien à réapprendre.
- Aucune régression : les invocations existantes (`patchcord serve --listen ... --data-dir ...`) continuent de fonctionner à l'identique, `--config` étant optionnel.
- Le point d'extension est posé pour de futurs réglages (origine CORS, niveau de log, réglages TLS-reverse-proxy) sans redécider l'architecture de précédence à chaque fois.

## Conséquences négatives
- Contenu volontairement minimal (2 réglages) — un fichier de config avec seulement `listen`/`data_dir` a une valeur limitée tant que d'autres réglages ne le rejoignent pas ; c'est un pari qu'ils arriveront (TLS, CORS, log level) plutôt qu'une solution complète dès aujourd'hui.
- `--data-dir` reste flag-only pour les commandes ponctuelles — un opérateur qui configure `PATCHCORD_DATA_DIR` pour `serve` doit quand même le repasser explicitement à `patchcord plugin list` et aux autres commandes CLI, pas de partage automatique de cette variable d'environnement entre `serve` et les commandes ponctuelles.
- Pas de rechargement à chaud du fichier de configuration : un changement dans `config.yaml` n'a d'effet qu'au prochain redémarrage de `patchcord serve`.
