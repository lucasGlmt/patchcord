# ADR-0039 — Image Docker : build multi-stage, base distroless, config par défaut embarquée

## Statut
Accepté

## Contexte
Après le trigger `schedule` ([ADR-0035](0035-trigger-schedule-scheduler-persistant.md)), l'authentification admin ([ADR-0036](0036-authentification-admin-jetons-opt-in.md)), le trigger `webhook` ([ADR-0037](0037-trigger-webhook-secret-partage.md)) et la configuration serveur ([ADR-0038](0038-configuration-serveur-fichier-yaml-precedence.md), construite précisément pour que Docker puisse suivre l'exemple `docker-compose` du document de vision), Docker est le chantier suivant de la Phase 6, choisi par Lucas.

Deux vérifications techniques ont conditionné les choix ci-dessous plutôt que des préférences arbitraires :
- `modernc.org/sqlite` (le driver SQLite du projet) est du Go pur — un build `CGO_ENABLED=0` réussit sans rien perdre, vérifié directement (`CGO_ENABLED=0 go build ./cmd/patchcord`). Ça ouvre la voie à une image finale sans libc du tout.
- L'exemple `docker-compose` du document de vision (section 13.3) monte `./plugins:/plugins` — mais dans **ce** dépôt, `./plugins` est déjà le code source de `plugins/examples/` (des paquets Go, pas des binaires). Monter ce répertoire tel quel exposerait des sources, pas des exécutables prêts à tourner dans un conteneur sans toolchain Go ni shell.

Aucun de ces deux points n'a nécessité d'arbitrage avec Lucas — ce sont des contraintes découvertes en vérifiant, pas des choix de conception. Le reste (image de base minimale, rien de métier embarqué dans l'image, configuration par défaut adaptée au conteneur) découle directement des non-négociables déjà établis (section 1 de CLAUDE.md) et du travail de l'ADR-0038.

## Décision

**Build multi-stage** (`Dockerfile`, racine du dépôt) : une étape `golang:1.25` compile `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"`, une étape finale `gcr.io/distroless/static-debian12` ne copie que le binaire résultant plus un fichier de config par défaut. Aucune toolchain Go, aucun gestionnaire de paquets, aucun shell dans l'image livrée — vérifié : l'image construite pèse **19,3 Mo**.

**Rien de métier embarqué.** Aucun greffon n'est construit ni copié dans l'image (non-négociable #3 : le core ne connaît aucune intégration concrète). Un greffon se construit séparément pour l'OS/l'architecture du conteneur et se monte via un volume, exactement comme en local : `patchcord plugin install` reste la seule voie d'installation, jamais un mécanisme spécifique à Docker.

**`./bin/plugins`, pas `./plugins`, dans `docker-compose.yml`.** Pour éviter la collision constatée avec le code source de ce dépôt, l'exemple de composition monte `./bin/plugins` — déjà le répertoire de sortie de `make build-plugins`, déjà dans `.gitignore`. Un déploiement dans un répertoire vierge (pas ce dépôt) n'aurait pas ce problème, mais documenter le vrai comportement dans le vrai dépôt prime sur reproduire l'exemple du document de vision au mot près.

**Configuration par défaut embarquée** (`docker/config.yaml`, copié dans l'image à `/etc/patchcord/config.yaml`) : `listen: 0.0.0.0:7331`, `data_dir: /data`. Ce n'est **pas** un changement du défaut de la CLI elle-même (`serve` continue de démarrer sur `127.0.0.1:7331` en dehors de Docker, sans branchement conditionnel — non-négociable #2 respecté) ; c'est le choix d'un artefact d'emballage distinct, la même logique qu'une image officielle Postgres ou Nginx dont la configuration par défaut diffère de celle du projet amont. Sans ce fichier embarqué, le premier démarrage échouerait immédiatement (pas de `config.yaml` pré-existant sur le volume monté) ou serait injoignable depuis l'hôte (le défaut `127.0.0.1` de la CLI n'écoute que dans le conteneur). Cette config embarquée reste substituable : elle occupe le rang le plus bas de la précédence posée par l'ADR-0038, donc `PATCHCORD_LISTEN`, `PATCHCORD_DATA_DIR`, ou un montage écrasant `/etc/patchcord/config.yaml` par le fichier propre à l'opérateur (retrouvant alors l'exemple exact du document de vision, `--config=/data/config.yaml`) l'emportent toujours.

**`docker-compose.yml`** (racine) : `build: .` plutôt qu'une image publiée — aucune image `patchcord/agent` n'existe encore sur un registre, ce fichier construit depuis ce dépôt. À remplacer par `image: patchcord/agent:<tag>` le jour où une image est publiée (repoussé, hors scope).

## Vérifié en conditions réelles
Build et exécution réels via le démon Docker local (pas seulement une lecture du Dockerfile) : image construite (19,3 Mo), conteneur démarré, migrations appliquées, `GET /v1/system/health` répond `200` via le port publié. Greffon `text` cross-compilé (`GOOS=linux`), installé via `docker exec ... plugin install /plugins/example-text`, repris après un `docker restart`. Workflow installé et déclenché via l'API HTTP publiée sur l'hôte, run `succeeded`, sortie `"HELLO FROM DOCKER"`. Base SQLite confirmée persistée sur le montage bind hôte (`./data/patchcord.db`), appartenant à l'utilisateur hôte — aucun problème de permission constaté (image non `nonroot`, choix délibéré pour rester sans friction sur un premier montage bind, cf. Conséquences négatives).

## Conséquences positives
- Image minimale (19,3 Mo, sans shell) — surface d'attaque réduite, rien à corriger dans l'image elle-même en cas de CVE d'une distribution Linux classique.
- `docker compose up --build` fonctionne du premier coup, sans préparation de fichier de config sur l'hôte — la friction "premier démarrage" reste nulle, cohérent avec l'esprit local-first du projet.
- La précédence de l'ADR-0038 rend la configuration embarquée strictement optionnelle à substituer : rien de nouveau à apprendre pour un opérateur qui préfère l'exemple exact du document de vision.
- Aucune duplication de logique d'installation de greffon : `patchcord plugin install` reste l'unique chemin, en local comme en conteneur.

## Conséquences négatives
- L'image tourne en `root` (pas de variante `:nonroot` de distroless) — choix délibéré pour éviter les problèmes de permissions sur les montages bind lors d'un premier essai, mais un déploiement de production voudra probablement durcir ça (utilisateur non-root + `chown` préalable du volume hôte), non traité ici.
- Aucune image publiée sur un registre — `docker-compose.yml` construit toujours localement depuis le code source ; distribuer une image versionnée reste un chantier séparé.
- Le fichier `docker/config.yaml` embarqué introduit un deuxième "défaut" à connaître (celui de la CLI nue, et celui de l'image) — documenté explicitement pour éviter la confusion, mais c'est un concept de plus par rapport à un unique défaut partout.
