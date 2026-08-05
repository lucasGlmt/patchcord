# ADR-0049 — PATCHCORD_DATA_DIR étendue à toutes les commandes ponctuelles

## Statut
Accepté

## Contexte
[ADR-0038](0038-configuration-serveur-fichier-yaml-et-precedence-flags-env-fichier.md) a posé la précédence flag > variable d'environnement (`PATCHCORD_DATA_DIR`) > fichier `--config` > défaut intégré, mais explicitement limitée à `patchcord serve` — sa propre conséquence négative listée le disait déjà : « un opérateur qui configure `PATCHCORD_DATA_DIR` pour `serve` doit quand même le repasser explicitement à `patchcord plugin list` et aux autres commandes CLI ».

Lucas a buté sur exactement ce cas en développant un bundle hors du dépôt : `bundle dev --watch`, `bundle install`, `plugin list`, etc. exigeaient tous de retaper `--data-dir` à la main, alors que `serve` acceptait déjà la variable d'environnement.

Trois options ont été présentées :
1. Étendre `PATCHCORD_DATA_DIR` à toutes les commandes prenant `--data-dir` — pas de nouveau format de fichier, cohérent avec le mécanisme déjà posé par ADR-0038, laisse l'auto-chargement par dossier (ex. direnv) à un outil externe.
2. Ajouter une découverte implicite de fichier (ex. `.patchcord.yaml` dans le dossier courant) — mais contredit un choix déjà tranché dans ADR-0038 (« pas de recherche implicite dans des emplacements par défaut »).
3. Ne rien changer côté CLI, solution shell/dossier (alias, wrapper).

Lucas a choisi l'option 1.

## Décision
Élargir la portée de la variable d'environnement `PATCHCORD_DATA_DIR` (déjà définie par `internal/config`, ADR-0038) à **toutes** les commandes ponctuelles qui exposent un flag `--data-dir` — `bundle`, `plugin`, `app`, `workflow`, `run`, `auth`, `connector`, `trust`, `secret`, `registry` — pas seulement `serve`.

Précédence par commande, appliquée uniquement au champ `data_dir` (pas de fichier `--config` pour ces commandes — celui-ci reste propre à `serve`) :

1. Défaut intégré (`./data`).
2. `PATCHCORD_DATA_DIR`.
3. `--data-dir` explicitement passé.

Implémentation : `internal/cli/datadir.go` ajoute `resolveDataDir(cmd *cobra.Command, dataDir string) string`, qui réutilise `config.FromEnv()` (déjà existant, aucun nouveau package) et s'appuie sur `cmd.Flags().Changed("data-dir")` — exactement le même test que `serve.go` utilise déjà pour distinguer « l'utilisateur a tapé ce flag » de « c'est resté à sa valeur par défaut ». Chaque commande appelle `dataDir = resolveDataDir(cmd, dataDir)` comme première instruction de son `RunE`, avant tout usage de `dataDir` — un seul point d'entrée par commande, cohérent avec le fait que `openDataStore` (`internal/cli/plugin.go`) était déjà le point de passage commun pour ouvrir la base.

Le texte d'aide de chaque flag `--data-dir` mentionne désormais `(env PATCHCORD_DATA_DIR)`, comme celui de `serve` le faisait déjà.

Pas de fichier `.env` ni de découverte implicite : un opérateur qui veut le comportement « par dossier » combine `PATCHCORD_DATA_DIR` avec un outil externe comme direnv (`.envrc`) — Patchcord n'invente pas son propre mécanisme de chargement de fichier local, cohérent avec le refus déjà exprimé dans ADR-0038 de toute recherche implicite d'emplacement.

## Conséquences positives
- Résout directement la conséquence négative que ADR-0038 avait déjà anticipée sans la traiter.
- Un seul export (`export PATCHCORD_DATA_DIR=/path/to/dir`, ou via direnv dans le dossier d'un bundle en développement) suffit pour toutes les commandes CLI ponctuelles, pas seulement `serve`.
- Aucun nouveau format de fichier ni mécanisme de découverte à maintenir — réutilise `internal/config.FromEnv()` tel quel.
- Changement mécanique et localisé (`internal/cli/datadir.go` + une ligne par `RunE`) : aucune régression sur les invocations existantes avec `--data-dir` explicite (`cmd.Flags().Changed` reste vrai, `resolveDataDir` renvoie la valeur du flag inchangée).

## Conséquences négatives
- `PATCHCORD_DATA_DIR` a maintenant deux portées différentes selon la commande : pour `serve` elle passe par la couche complète fichier/env/flag d'ADR-0038 (incluant `--config`), pour les commandes ponctuelles elle n'a que env/flag — pas de fichier `--config` équivalent pour elles. Un opérateur qui chercherait `--config` sur `patchcord bundle install` ne le trouvera pas.
- N'introduit toujours aucun mécanisme de fichier local auto-découvert (`.env`, `.patchcord.yaml` de dossier) — un besoin réel pour ça referait l'arbitrage de l'option 2 écartée ici.
