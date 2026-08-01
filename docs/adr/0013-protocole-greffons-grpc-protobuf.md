# ADR-0013 — gRPC et Protobuf comme protocole de greffons

## Statut
Accepté

## Contexte
La section 8.2 du document de vision laisse le choix ouvert pour la première version du protocole de greffons : gRPC/Protobuf ou JSON-RPC sur socket/pipe. C'est l'une des trois frontières publiques que la section 23 identifie comme structurantes et censées rester indépendantes de Go (non-négociable #5, ADR-0003) : tout choix ici engage durablement les futurs greffons tiers, potentiellement écrits dans d'autres langages (Rust, TypeScript, Python...).

## Décision
Le protocole de greffons repose sur gRPC et Protobuf. Le contrat est défini dans `api/plugin/v1/plugin.proto` (service `PluginService`, RPC `Handshake`), versionné par répertoire (`v1`) et par le champ `protocol_version` échangé à la négociation. Les stubs Go sont générés via `buf` (`buf.yaml` / `buf.gen.yaml` à la racine) et committés dans `api/plugin/v1/`.

Le transport lui-même utilise une boucle locale TCP (`127.0.0.1:0`, port éphémère) plutôt qu'un socket Unix, pour un fonctionnement identique sur macOS, Linux et Windows sans code spécifique par plateforme. Un greffon lancé par l'agent démarre son serveur gRPC puis imprime une unique ligne JSON sur sa sortie standard (`{"address":"127.0.0.1:PORT"}`) pour que l'agent découvre où le joindre avant d'ouvrir la connexion gRPC et d'appeler `Handshake`.

## Conséquences positives
- Contrat fortement typé, générable pour de nombreux langages sans effort de conception supplémentaire côté Patchcord.
- Le streaming gRPC natif est disponible pour les besoins futurs (health checks continus, flux de logs) sans changer de protocole.
- Le motif "cœur lance un sous-processus qui expose du gRPC local, découvert via une ligne de bootstrap sur stdout" est éprouvé (Terraform, Vault, HashiCorp go-plugin) pour exactement ce scénario processus-à-processus.
- La compatibilité TCP loopback évite toute divergence de comportement entre systèmes d'exploitation, cohérent avec le non-négociable #2 ("le même binaire fonctionne en local et sur serveur").

## Conséquences négatives
- Une chaîne d'outillage protobuf (`buf`, `protoc-gen-go`, `protoc-gen-go-grpc`) est désormais nécessaire pour régénérer les stubs à chaque évolution du schéma ; sans vérification automatisée, le code généré committé peut diverger silencieusement du `.proto` si la régénération est oubliée — un contrôle CI dédié devra être ajouté.
- Le TCP loopback expose un port éphémère sur la machine locale : tout processus local capable de découvrir ce port pourrait en théorie tenter de s'y connecter. Cette version minimale du handshake ne porte encore aucune authentification ni permission (prévues section 8.4/15.5, Plugin Supervisor) — à traiter dans une phase ultérieure avant tout déploiement multi-utilisateur.
- `Close()` du processus greffon termine le processus par un signal `Kill` immédiat plutôt qu'un arrêt négocié via le protocole ; un arrêt gracieux (RPC de shutdown, délai avant kill) reste à concevoir avec la supervision complète (Plugin Supervisor), explicitement hors périmètre de cette passe.
