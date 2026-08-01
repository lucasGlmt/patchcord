# Patchcord

> **Un runtime universel, extensible et local-first pour connecter des systèmes, automatiser des processus et construire des applications intelligentes.**

## Statut du document

- **Projet** : Patchcord
- **Nature** : document fondateur — vision produit et architecture cible
- **Statut** : proposition initiale
- **Date** : 1er août 2026
- **Ancien nom du projet** : GLMT Compagnon
- **Langage cible du noyau** : Go
- **Interfaces principales** : CLI, API HTTP, SDK TypeScript, SDK de greffons
- **Modes de déploiement** : local, service système, Docker, serveur

---

# 1. Résumé

Patchcord est un agent d’exécution autonome conçu pour fonctionner aussi bien sur une machine locale que sur un serveur disponible en permanence.

Le projet fournit un noyau minimal capable de :

- charger et superviser des greffons ;
- gérer des connecteurs ;
- exécuter des actions ;
- orchestrer des workflows ;
- planifier et déclencher des exécutions ;
- exposer une API stable ;
- servir de backend à des applications web ou desktop ;
- intégrer des capacités d’intelligence artificielle ;
- gérer les secrets, permissions, historiques et journaux d’exécution.

Patchcord n’est pas principalement un éditeur visuel d’automatisations et ne cherche pas à reproduire n8n.

Le moteur de workflows est une composante fondamentale, mais il reste une infrastructure interne au runtime. La finalité du projet est de permettre à des développeurs et intégrateurs de construire facilement des connecteurs, des automatisations, des actions communautaires et de véritables applications métier au-dessus d’un agent déployable partout.

```text
Applications
    ↓ utilisent l’API et les SDK
Patchcord Agent
    ↓ orchestre
Workflows
    ↓ exécutent
Actions
    ↓ utilisent
Connecteurs et services
```

Le principe central est le suivant :

> **Le core fournit les mécanismes. Les greffons fournissent les capacités. Les workflows fournissent l’orchestration. Les applications fournissent l’expérience.**

---

# 2. Origine et évolution du projet

Le projet est né sous le nom **GLMT Compagnon** comme une application desktop Flutter embarquant un agent Dart local.

L’architecture initiale reposait sur :

- une interface Flutter Desktop ;
- un agent Dart lancé comme processus enfant ;
- une API locale HTTP ;
- un moteur d’automatisation ;
- des actions déclaratives ;
- des connecteurs ;
- une base SQLite locale ;
- une communication sécurisée entre l’interface et l’agent.

Cette architecture a permis de démontrer plusieurs idées importantes :

- l’exécution locale d’automatisations est pertinente ;
- les workflows peuvent être déclaratifs et versionnés ;
- les actions atomiques constituent une bonne abstraction ;
- les connecteurs doivent encapsuler la configuration et les secrets ;
- l’interface ne doit pas porter la logique métier ;
- l’agent est capable d’exécuter les mêmes workflows manuellement, par cron ou par webhook.

Cependant, la nouvelle vision dépasse largement celle d’une simple application desktop.

L’agent devient désormais le produit fondamental. L’interface graphique n’est plus qu’un client parmi d’autres.

```text
Ancienne vision
Application desktop
    └── embarque un agent

Nouvelle vision
Patchcord Agent
    ├── fonctionne seul
    ├── expose une API
    ├── possède une CLI
    ├── charge des greffons
    ├── héberge des workflows
    └── sert de backend à plusieurs interfaces
```

Ce changement justifie :

- le renommage du projet ;
- la réécriture du noyau ;
- la séparation stricte entre core, greffons, SDK et applications ;
- l’abandon de toute dépendance obligatoire à une plateforme cloud propriétaire ;
- la conception d’un protocole public stable dès le départ.

---

# 3. Vision produit

Patchcord doit permettre de :

1. **connecter n’importe quoi avec n’importe quoi facilement ;**
2. **automatiser n’importe quel processus facilement ;**
3. **ajouter de l’intelligence grâce à des fournisseurs et modèles d’IA ;**
4. **construire des applications métier au-dessus du même runtime ;**
5. **choisir librement le lieu d’exécution : poste local, serveur, conteneur ou infrastructure privée.**

Patchcord peut être utilisé comme :

- backend local d’une application métier ;
- moteur d’intégration sur un serveur ;
- orchestrateur de traitements planifiés ;
- passerelle entre logiciels locaux et services distants ;
- runtime d’applications internes ;
- agent IA utilisant des outils déterministes ;
- socle de solutions verticales construites par des intégrateurs ;
- infrastructure embarquée dans une application desktop.

## 3.1 Proposition de valeur

### Pour un développeur

Le développeur peut créer :

- un greffon ajoutant des connecteurs ;
- un greffon ajoutant des actions ;
- un déclencheur spécifique ;
- une application web Vite utilisant les workflows ;
- un workflow réutilisable ;
- un bundle métier installable.

Il n’a pas besoin de modifier ou recompiler Patchcord Agent.

### Pour un intégrateur

L’intégrateur peut :

- installer Patchcord chez un client ;
- sélectionner les greffons nécessaires ;
- configurer les connecteurs ;
- importer des workflows ;
- construire une application métier dédiée ;
- déployer le tout localement ou sur un serveur.

### Pour une entreprise

L’entreprise peut :

- garder ses données sur son infrastructure ;
- éviter de dépendre d’un SaaS central ;
- exécuter des automatisations 24 heures sur 24 ;
- connecter des logiciels anciens ou internes ;
- déployer des applications métier légères ;
- utiliser ses propres fournisseurs d’IA ;
- conserver la maîtrise de ses secrets.

---

# 4. Ce que Patchcord n’est pas

Patchcord ne doit pas devenir un clone généraliste de n8n.

Le projet ne doit pas être orienté en priorité vers :

- un gigantesque canvas visuel ;
- la reproduction de centaines de nœuds officiels ;
- une expérience SaaS centralisée obligatoire ;
- un éditeur no-code monolithique ;
- l’exécution de scripts arbitraires non contrôlés ;
- une interface graphique unique imposée à tous les usages.

Le moteur de workflows est un composant du runtime, pas sa seule raison d’être.

## Non-objectifs initiaux

- canvas graphique complet ;
- plateforme SaaS multi-tenant ;
- marketplace commerciale dès la première version ;
- exécution distribuée sur plusieurs agents ;
- orchestration Kubernetes ;
- environnement de développement intégré ;
- code arbitraire directement intégré dans un workflow ;
- compatibilité immédiate avec tous les langages ;
- centaines de connecteurs officiels ;
- isolation parfaite de greffons natifs non fiables.

Ces capacités pourront évoluer ultérieurement, mais elles ne doivent pas perturber les fondations.

---

# 5. Principes fondateurs

## 5.1 L’agent est le produit fondamental

Patchcord Agent doit pouvoir fonctionner sans interface graphique.

```bash
patchcord serve
patchcord workflow run invoice-analysis
patchcord plugin install io.patchcord.postgresql
patchcord connector test accounting-db
```

Les interfaces graphiques, dashboards et applications desktop sont des clients du runtime.

## 5.2 Local-first, mais pas local-only

Patchcord fonctionne :

- sur un poste utilisateur ;
- comme service système ;
- dans Docker ;
- sur un serveur privé ;
- derrière un reverse proxy ;
- dans une infrastructure d’entreprise.

Le déploiement est une décision de l’utilisateur, pas une contrainte du produit.

## 5.3 Le core ne connaît aucun service métier concret

Le noyau ne doit pas dépendre de Gmail, OpenAI, PostgreSQL, Notion, Microsoft 365 ou d’un CRM particulier.

Ces capacités sont apportées par des greffons.

## 5.4 Les greffons sont indépendants du core

Un greffon :

- peut vivre dans un dépôt séparé ;
- possède son propre cycle de version ;
- est compilé et distribué indépendamment ;
- communique avec l’agent via un protocole public ;
- n’est jamais lié statiquement au noyau ;
- ne nécessite aucune recompilation de Patchcord Agent.

## 5.5 Les applications utilisent une API stable

Une application web ou desktop ne doit pas importer du code interne du runtime.

Elle utilise :

- l’API publique de l’agent ;
- le SDK TypeScript ;
- éventuellement un SDK spécifique à sa plateforme.

## 5.6 Les contrats publics priment sur l’implémentation

Les interfaces Go sont utiles à l’intérieur du core.

Les frontières publiques reposent sur :

- Protobuf ;
- JSON Schema ;
- OpenAPI ;
- protocoles versionnés ;
- messages indépendants du langage.

L’intérieur de Patchcord peut évoluer sans casser les greffons ni les applications.

## 5.7 Le cloud reste facultatif

Patchcord doit être pleinement utilisable sans compte distant ni serveur de licence.

Un service cloud futur peut apporter :

- registre de greffons ;
- synchronisation ;
- webhooks relayés ;
- gestion de parc ;
- sauvegarde ;
- monitoring ;
- distribution privée ;
- services IA administrés.

Mais il ne doit pas être requis pour démarrer ou utiliser l’agent.

---

# 6. Architecture générale

```text
┌───────────────────────────────────────────────────────────────┐
│                           Clients                             │
│                                                               │
│  CLI   Dashboard Vite   App métier   Desktop UI   Scripts     │
└──────────────────────────────┬────────────────────────────────┘
                               │ HTTP / WebSocket / SSE / SDK
                               ▼
┌───────────────────────────────────────────────────────────────┐
│                       Patchcord Agent                         │
│                                                               │
│  API publique                                                 │
│  Authentification et permissions                              │
│  Workflow Engine                                              │
│  Run Manager                                                  │
│  Scheduler et triggers                                        │
│  Connector Manager                                            │
│  Plugin Supervisor                                            │
│  Secret Manager                                               │
│  App Host                                                     │
│  Persistance                                                  │
│  Logs, audit et diagnostic                                    │
└──────────────────────────────┬────────────────────────────────┘
                               │ protocole de greffons
                               ▼
┌───────────────────────────────────────────────────────────────┐
│                           Greffons                            │
│                                                               │
│  PostgreSQL   Gmail   HTTP   OpenAI   Mistral   SAP   PDF     │
│  Actions      Connecteurs      Triggers      Providers        │
└───────────────────────────────────────────────────────────────┘
```

---

# 7. Les composants fondamentaux

## 7.1 Patchcord Agent

Le binaire principal contient :

- le serveur API ;
- la CLI ;
- le moteur de workflows ;
- le gestionnaire d’exécutions ;
- le scheduler ;
- le système de déclencheurs ;
- la persistance ;
- le gestionnaire de greffons ;
- le gestionnaire de connecteurs ;
- le stockage des secrets ;
- le système de permissions ;
- les journaux structurés ;
- le diagnostic ;
- l’hébergement des applications web.

Le core doit rester aussi petit et générique que possible.

## 7.2 Greffons

Un greffon est un programme indépendant lancé et supervisé par l’agent.

Il peut contribuer :

- des actions ;
- des types de connecteurs ;
- des déclencheurs ;
- des fournisseurs d’IA ;
- des mécanismes de secrets ;
- des intégrations métier.

Un greffon est distribué sous forme de package installable contenant un manifeste et un ou plusieurs exécutables.

## 7.3 Connecteurs

Un connecteur représente une configuration persistante permettant d’accéder à un système.

Exemples :

- compte Gmail ;
- tenant Microsoft 365 ;
- connexion PostgreSQL ;
- serveur SFTP ;
- espace Notion ;
- fournisseur OpenAI ;
- fournisseur Mistral ;
- API interne ;
- dossier local autorisé.

Le connecteur gère :

- sa configuration ;
- ses références de secrets ;
- son test de connexion ;
- son statut ;
- ses capacités ;
- sa compatibilité avec certaines actions.

Un connecteur n’est pas une action.

```text
Connecteur PostgreSQL
→ représente une connexion persistante

Action postgresql.query
→ utilise cette connexion pour exécuter une requête
```

## 7.4 Actions

Une action est une opération atomique exécutable par un workflow.

Exemples :

```text
postgresql.query
mail.list_messages
mail.create_draft
http.request
filesystem.read_text
pdf.extract_text
ai.generate_text
crm.create_contact
notification.send
```

Une action déclare :

- son identifiant ;
- sa version ;
- ses entrées ;
- ses sorties ;
- les capacités requises ;
- les connecteurs compatibles ;
- ses erreurs connues ;
- son comportement en mode test ;
- son timeout par défaut.

## 7.5 Workflows

Un workflow orchestre des actions.

Il est :

- déclaratif ;
- versionné ;
- validé avant exécution ;
- portable ;
- indépendant de l’interface ;
- sérialisable en YAML ou JSON ;
- exécuté par une machine à états explicite.

```yaml
schema_version: 1

id: invoice_analysis
version: 1

trigger:
  type: manual

steps:
  - id: extract_text
    uses: pdf.extract_text@1
    with:
      file: "${{ workflow.inputs.file }}"

  - id: analyse
    uses: ai.generate_structured_data@1
    connector: "${{ bindings.ai_provider }}"
    with:
      text: "${{ steps.extract_text.outputs.text }}"

  - id: export
    uses: spreadsheet.write_rows@1
    with:
      rows: "${{ steps.analyse.outputs.items }}"
```

## 7.6 Applications

Une application est une interface construite au-dessus de l’agent.

Elle peut être développée avec :

- Vite ;
- React ;
- Vue ;
- Svelte ;
- JavaScript natif ;
- Flutter ;
- Electron ;
- Tauri ;
- tout autre client capable d’utiliser l’API.

Une application peut :

- afficher des formulaires ;
- demander la sélection de fichiers ;
- lancer des workflows ;
- suivre les événements d’exécution ;
- afficher les résultats ;
- gérer ses propres préférences ;
- utiliser les connecteurs autorisés ;
- servir d’interface métier complète.

Elle ne doit jamais recevoir les privilèges administrateur complets de l’agent.

---

# 8. Architecture des greffons

## 8.1 Modèle d’exécution

Patchcord ne doit pas utiliser les plugins natifs Go chargés dans le même processus.

Les greffons fonctionnent comme des processus indépendants :

```text
Patchcord Agent
    ↓ lance
Greffon autonome
    ↕ RPC
Patchcord Agent
```

Avantages :

- aucune recompilation du core ;
- cycle de vie indépendant ;
- langage libre ;
- isolation des crashs ;
- timeout contrôlable ;
- mise à jour indépendante ;
- désactivation possible ;
- compatibilité négociée au démarrage.

## 8.2 Protocole

Le protocole de greffons doit être :

- indépendant de Go ;
- versionné ;
- documenté ;
- testable ;
- générable depuis des schémas ;
- compatible avec plusieurs SDK.

La première version peut utiliser :

- gRPC et Protobuf ;
- ou un protocole JSON-RPC sur socket ou pipes.

Le transport ne doit pas exposer les types internes du core.

## 8.3 Handshake

Au démarrage, l’agent et le greffon négocient :

- version du protocole ;
- identité du greffon ;
- version du greffon ;
- contributions disponibles ;
- permissions requises ;
- capacités prises en charge ;
- options de santé ;
- comportement d’arrêt.

Exemple conceptuel :

```json
{
  "protocol_version": 1,
  "plugin": {
    "id": "io.patchcord.postgresql",
    "version": "1.0.0"
  },
  "contributes": {
    "connectors": [
      "postgresql.connection@1"
    ],
    "actions": [
      "postgresql.query@1",
      "postgresql.execute@1"
    ]
  }
}
```

## 8.4 Supervision

Le `Plugin Supervisor` doit gérer :

- démarrage ;
- arrêt ;
- redémarrage ;
- health check ;
- limites de mémoire futures ;
- timeout ;
- journalisation ;
- détection de crash ;
- compatibilité ;
- quarantaine ;
- désactivation après échecs répétés.

## 8.5 SDK de greffons

Un SDK Go officiel doit permettre d’écrire un greffon avec très peu de code :

```go
func main() {
    patchcord.Serve(patchcord.Plugin{
        Manifest: patchcord.Manifest{
            ID:      "io.example.postgresql",
            Version: "1.0.0",
        },
        Connectors: []patchcord.Connector{
            NewPostgreSQLConnector(),
        },
        Actions: []patchcord.Action{
            NewQueryAction(),
            NewExecuteAction(),
        },
    })
}
```

Le SDK masque :

- le transport ;
- le protocole ;
- le handshake ;
- la sérialisation ;
- les erreurs ;
- les logs ;
- les annulations ;
- les timeouts ;
- l’arrêt propre.

D’autres SDK pourront être ajoutés ultérieurement :

- Rust ;
- TypeScript ;
- Python ;
- Java ;
- .NET.

---

# 9. Packaging et installation

## 9.1 Package de greffon

Format conceptuel :

```text
postgresql-1.0.0.patchcord-plugin
├── manifest.json
├── checksums.json
├── signature.json
├── LICENSE
└── binaries/
    ├── darwin-arm64/plugin
    ├── darwin-amd64/plugin
    ├── linux-amd64/plugin
    ├── linux-arm64/plugin
    └── windows-amd64/plugin.exe
```

Manifeste :

```json
{
  "schemaVersion": 1,
  "kind": "plugin",
  "id": "io.patchcord.postgresql",
  "name": "PostgreSQL",
  "version": "1.0.0",
  "protocolVersion": 1,
  "runtime": {
    "type": "process",
    "executables": {
      "darwin-arm64": "binaries/darwin-arm64/plugin",
      "linux-amd64": "binaries/linux-amd64/plugin",
      "windows-amd64": "binaries/windows-amd64/plugin.exe"
    }
  },
  "permissions": [
    "network.outbound",
    "secrets.read:postgresql"
  ]
}
```

## 9.2 Installation

```bash
patchcord plugin install ./postgresql-1.0.0.patchcord-plugin
```

Ou depuis un registre futur :

```bash
patchcord plugin install io.patchcord.postgresql@1.0.0
```

L’installation doit :

1. vérifier le manifeste ;
2. vérifier les checksums ;
3. vérifier la signature ;
4. vérifier la compatibilité ;
5. afficher les permissions ;
6. installer le package ;
7. sélectionner le bon exécutable ;
8. exécuter un handshake de validation ;
9. enregistrer les contributions ;
10. conserver un rollback possible.

## 9.3 Autres packages

Patchcord pourra gérer plusieurs types de packages :

```text
.patchcord-plugin
.patchcord-workflow
.patchcord-app
.patchcord-bundle
```

### Workflow

Définition déclarative seule.

### Application

Interface web statique et manifeste de permissions.

### Bundle

Regroupe application, workflows, configuration et dépendances.

---

# 10. API publique et SDK

## 10.1 API de l’agent

L’API publique doit couvrir :

```text
/v1/system
/v1/plugins
/v1/connectors
/v1/actions
/v1/workflows
/v1/runs
/v1/apps
/v1/secrets
/v1/events
```

Elle doit être :

- versionnée ;
- documentée avec OpenAPI ;
- indépendante des détails internes ;
- utilisable localement et à distance ;
- compatible avec HTTP, SSE ou WebSocket selon les besoins.

## 10.2 SDK TypeScript

Le SDK TypeScript permet aux applications Vite d’utiliser l’agent.

Exemple :

```ts
import { PatchcordClient } from "@patchcord/sdk";

const client = new PatchcordClient({
  baseUrl: window.__PATCHCORD_BASE_URL__,
  token: window.__PATCHCORD_SESSION_TOKEN__,
});

const run = await client.workflows.run("invoice_analysis", {
  file: selectedFileHandle,
});

for await (const event of run.events()) {
  console.log(event.status);
}

const result = await run.result();
```

Le SDK doit fournir :

```text
client.system
client.plugins
client.connectors
client.actions
client.workflows
client.runs
client.apps
client.files
client.notifications
client.storage
```

## 10.3 Développement d’applications

```bash
npm create vite@latest invoice-manager
npm install @patchcord/sdk
```

Mode développement :

```bash
npm run dev

patchcord app dev \
  --origin http://localhost:5173 \
  --manifest ./patchcord-app.yaml
```

Mode production :

```bash
npm run build
patchcord app pack ./dist
patchcord app install ./invoice-manager.patchcord-app
```

L’agent peut servir l’application :

```text
http://127.0.0.1:7331/apps/invoice-manager/
```

---

# 11. CLI comme première interface

La CLI constitue la première interface officielle et la référence fonctionnelle.

```bash
patchcord init
patchcord serve
patchcord status
patchcord doctor
```

## Greffons

```bash
patchcord plugin list
patchcord plugin install
patchcord plugin inspect
patchcord plugin enable
patchcord plugin disable
patchcord plugin uninstall
```

## Connecteurs

```bash
patchcord connector list
patchcord connector create
patchcord connector inspect
patchcord connector test
patchcord connector remove
```

## Workflows

```bash
patchcord workflow list
patchcord workflow validate
patchcord workflow install
patchcord workflow inspect
patchcord workflow run
patchcord workflow export
```

## Exécutions

```bash
patchcord run list
patchcord run inspect
patchcord run logs
patchcord run cancel
```

## Applications

```bash
patchcord app list
patchcord app dev
patchcord app pack
patchcord app install
patchcord app serve
```

Les commandes de gestion doivent appeler les mêmes services que l’API publique afin d’éviter deux comportements concurrents.

---

# 12. Moteur de workflows

## 12.1 Principes

Le moteur doit être :

- déterministe dans ses transitions ;
- persistant ;
- observable ;
- annulable ;
- compatible avec les timeouts ;
- capable de reprendre proprement après un redémarrage ;
- indépendant des implémentations des actions.

## 12.2 États

### Run

```text
queued
running
succeeded
failed
cancelled
```

États futurs possibles :

```text
paused
waiting_for_approval
retry_scheduled
```

### Step

```text
pending
running
succeeded
failed
skipped
cancelled
```

## 12.3 Concurrence et async

Go facilite l’exécution concurrente grâce aux goroutines, mais le moteur doit rester une machine à états explicite.

Chaque run reçoit un `context.Context`.

Chaque étape peut dériver :

- un timeout ;
- une annulation ;
- une limite de concurrence ;
- une politique de retry.

Les goroutines ne remplacent pas :

- la persistance ;
- l’idempotence ;
- les transitions atomiques ;
- les reprises ;
- les garanties d’exécution.

## 12.4 Versionnement

Les workflows publiés sont immuables.

```text
invoice_analysis
├── version 1
├── version 2
└── version 3 active
```

Un run conserve la version utilisée au démarrage.

## 12.5 Compilation

Une définition doit être validée et compilée avant exécution.

Le compilateur vérifie notamment :

- version de schéma ;
- unicité des étapes ;
- existence des actions ;
- compatibilité des versions ;
- connecteurs disponibles ;
- capacités requises ;
- références aux étapes ;
- types d’entrées ;
- limites ;
- permissions ;
- dépendances de greffons.

---

# 13. Déploiement

## 13.1 Mode local interactif

```bash
patchcord serve --listen 127.0.0.1:7331
```

Usage :

- automatisations personnelles ;
- accès à des fichiers locaux ;
- intégration avec des logiciels installés ;
- applications métier locales ;
- notifications desktop.

## 13.2 Service système

Patchcord peut fonctionner en arrière-plan via :

- `systemd` ;
- `launchd` ;
- service Windows.

```text
Utilisateur
    ↓ ouvre une application
Application
    ↓ appelle
Patchcord Agent déjà actif
```

## 13.3 Docker

```yaml
services:
  patchcord:
    image: patchcord/agent:1.0
    restart: unless-stopped
    volumes:
      - ./data:/data
      - ./plugins:/plugins
    ports:
      - "7331:7331"
    command:
      - serve
      - --config=/data/config.yaml
```

Usages :

- automatisations 24/7 ;
- webhooks ;
- applications internes ;
- synchronisation de systèmes ;
- traitement de tâches planifiées ;
- déploiement reproductible.

## 13.4 Serveur natif

Patchcord peut être installé derrière :

- un reverse proxy ;
- TLS ;
- OIDC ;
- un gestionnaire de secrets ;
- un pare-feu ;
- une infrastructure d’entreprise.

La première version serveur doit rester mono-espace de travail.

Le multi-tenant complet n’est pas une priorité initiale.

---

# 14. Persistance

La persistance doit être abstraite derrière des interfaces internes.

## Local

SQLite convient pour :

- workflows ;
- versions ;
- runs ;
- étapes ;
- configuration ;
- métadonnées ;
- catalogue de greffons.

## Serveur

Une évolution pourra permettre PostgreSQL, mais elle ne doit pas être nécessaire pour la première version.

## Event log

Les transitions importantes peuvent être stockées sous forme d’événements :

```text
run.created
run.started
step.started
step.succeeded
step.failed
run.succeeded
run.failed
```

Un event log facilite :

- audit ;
- diagnostic ;
- reprise ;
- diffusion en temps réel ;
- reconstruction de l’historique.

---

# 15. Secrets et sécurité

## 15.1 Authentification commerciale supprimée du core

Patchcord ne doit pas exiger :

- compte distant ;
- licence distante ;
- validation auprès d’une plateforme propriétaire.

## 15.2 Authentification technique obligatoire

L’API doit rester protégée.

Les modes d’authentification dépendent du déploiement :

### Local

- socket local ;
- jeton de session ;
- permissions par application ;
- liaison à une origine contrôlée.

### Serveur

- API keys ;
- OIDC ;
- tokens signés ;
- reverse proxy ;
- TLS.

## 15.3 Secret stores

Adaptateurs possibles :

```text
macOS Keychain
Windows Credential Manager
Linux Secret Service
fichiers Docker secrets
variables d’environnement
HashiCorp Vault
autre provider installé
```

Les workflows ne contiennent jamais les secrets.

Ils contiennent uniquement des références logiques.

## 15.4 Permissions des applications

Une application déclare ses permissions :

```yaml
permissions:
  workflows:
    run:
      - invoice_analysis

  connectors:
    use:
      - accounting_mailbox

  capabilities:
    - file.user_selected.read
    - notification.desktop
```

L’application reçoit une session limitée.

Elle ne reçoit jamais le jeton administrateur du runtime.

## 15.5 Permissions des greffons

Un greffon natif est un programme installé sur la machine.

Le manifeste de permissions doit informer l’utilisateur, mais ne constitue pas à lui seul une sandbox complète.

Première politique :

- greffons officiels signés ;
- greffons vérifiés ;
- greffons non signés uniquement en mode développeur.

Évolutions possibles :

- sandbox macOS ;
- AppContainer Windows ;
- namespaces et seccomp Linux ;
- conteneurs ;
- runtime WebAssembly ;
- capability broker.

## 15.6 Capability broker

À terme, le core doit limiter les accès directs.

Au lieu de transmettre un chemin :

```json
{
  "file_path": "/home/user/document.pdf"
}
```

le runtime peut transmettre un handle :

```json
{
  "file_handle": "file_01K..."
}
```

Le greffon demande ensuite au core de lire ce fichier, et le core vérifie les permissions.

---

# 16. Intelligence artificielle

L’IA est une capacité du runtime, pas une dépendance centrale.

Des greffons peuvent contribuer :

- fournisseurs OpenAI ;
- fournisseurs Mistral ;
- fournisseurs locaux ;
- modèles auto-hébergés ;
- embeddings ;
- génération de texte ;
- extraction structurée ;
- vision ;
- génération d’images ;
- agents spécialisés.

Les workflows peuvent combiner :

```text
Collecte de données
→ filtrage
→ transformation
→ inférence IA
→ validation
→ action déterministe
```

L’IA doit rester encapsulée derrière des actions typées et auditables.

Le moteur ne doit pas confondre :

- décision probabiliste ;
- orchestration déterministe ;
- effet externe.

---

# 17. Structure des dépôts

## Phase initiale

Un monorepo est recommandé tant que les protocoles changent rapidement.

```text
patchcord/
├── cmd/
│   └── patchcord/
│
├── internal/
│   ├── runtime/
│   ├── workflow/
│   ├── runs/
│   ├── scheduler/
│   ├── plugins/
│   ├── connectors/
│   ├── apps/
│   ├── auth/
│   ├── permissions/
│   ├── secrets/
│   ├── persistence/
│   └── api/
│
├── api/
│   ├── agent/
│   ├── plugin/
│   ├── workflow/
│   └── app/
│
├── sdk/
│   ├── go-plugin/
│   └── typescript/
│
├── plugins/
│   └── examples/
│
├── apps/
│   └── examples/
│
├── docs/
└── migrations/
```

Les frontières doivent être conçues comme si les composants vivaient déjà dans des dépôts séparés.

## Phase stable

Extraction possible :

```text
patchcord-agent
patchcord-protocol
patchcord-sdk-go
patchcord-sdk-typescript
patchcord-plugin-http
patchcord-plugin-postgresql
patchcord-plugin-openai
patchcord-desktop
```

Un greffon tiers ne doit dépendre que du protocole et du SDK public.

---

# 18. Open source et modèle économique

Patchcord peut devenir un projet open source centré sur le runtime.

## Cœur open source

- agent ;
- moteur de workflows ;
- protocole de greffons ;
- CLI ;
- SDK ;
- hébergement d’applications ;
- connecteurs fondamentaux éventuels ;
- documentation.

## Revenus possibles

### Services professionnels

- intégration ;
- développement de greffons ;
- conception de workflows ;
- applications métier ;
- déploiement ;
- maintenance ;
- formation ;
- audit ;
- support.

### Cloud facultatif

- registre officiel ;
- relay webhook ;
- monitoring ;
- sauvegarde ;
- synchronisation ;
- gestion de parc ;
- distribution privée ;
- secrets managés ;
- services IA managés.

### Marketplace

- greffons premium ;
- applications premium ;
- bundles métier ;
- commission sur les ventes ;
- signature et vérification.

### Enterprise

- SSO ;
- politiques centralisées ;
- audit consolidé ;
- déploiement de parc ;
- support prioritaire ;
- SLA ;
- versions LTS ;
- registre privé.

Le fonctionnement local et autonome du core ne doit pas dépendre de ces offres.

---

# 19. Roadmap proposée

## Phase 0 — Spécification

- définir la vision ;
- figer le vocabulaire ;
- définir les non-objectifs ;
- choisir la licence ;
- définir les contrats publics ;
- documenter les modèles de menace.

## Phase 1 — Core minimal

- binaire Go ;
- CLI ;
- configuration ;
- logs structurés ;
- API de santé ;
- SQLite ;
- gestion du cycle de vie ;
- serveur HTTP local ;
- arrêt propre.

## Phase 2 — Protocole de greffons

- Protobuf ou JSON-RPC ;
- handshake ;
- manifeste ;
- lancement de processus ;
- supervision ;
- health checks ;
- SDK Go ;
- greffon d’exemple.

Greffon de validation :

```text
text.uppercase@1
```

Objectif :

```text
développer
→ compiler
→ packager
→ installer
→ découvrir
→ exécuter
sans recompiler Patchcord
```

## Phase 3 — Moteur de workflows

- modèles versionnés ;
- compilation ;
- runner séquentiel ;
- expressions ;
- persistance ;
- timeouts ;
- annulation ;
- historique ;
- événements en temps réel.

## Phase 4 — Connecteurs

- modèle de connecteur ;
- configuration ;
- références de secrets ;
- tests ;
- binding ;
- capacités ;
- greffon HTTP ;
- greffon IA ;
- greffon PostgreSQL.

## Phase 5 — SDK TypeScript et applications

- API applications ;
- sessions limitées ;
- SDK TypeScript ;
- exemple Vite ;
- mode développement ;
- packaging ;
- hébergement statique.

## Phase 6 — Déploiement serveur

- configuration serveur ;
- Docker ;
- authentification distante ;
- TLS via reverse proxy ;
- secret providers ;
- webhooks ;
- scheduler persistant.

## Phase 7 — Écosystème

- registre ;
- signature ;
- vérification ;
- mise à jour ;
- bundles ;
- documentation développeur ;
- templates ;
- première application métier complète.

---

# 20. Première tranche verticale recommandée

La première preuve de l’architecture ne doit pas chercher à résoudre un cas métier complexe.

## Core

```bash
patchcord serve
```

## Greffon indépendant

```text
io.patchcord.example-text
└── action text.uppercase@1
```

## Installation

```bash
patchcord plugin install ./example-text.patchcord-plugin
```

## Workflow

```yaml
schema_version: 1

id: hello_patchcord
version: 1

trigger:
  type: manual

steps:
  - id: transform
    uses: text.uppercase@1
    with:
      value: "Welcome Patchcord"
```

## Exécution

```bash
patchcord workflow run hello_patchcord
```

## Résultat

```json
{
  "value": "WELCOME PATCHCORD"
}
```

Cette tranche valide :

- le protocole ;
- le packaging ;
- l’installation ;
- la découverte ;
- le registre dynamique ;
- le runner ;
- les logs ;
- les erreurs ;
- l’indépendance du greffon ;
- l’absence de recompilation du core.

---

# 21. Critères architecturaux de réussite

L’architecture est considérée comme saine lorsque :

1. Patchcord Agent peut fonctionner sans interface graphique.
2. Le même binaire fonctionne en local et sur serveur.
3. Un greffon peut être développé dans un dépôt séparé.
4. Un greffon n’importe aucun package interne du core.
5. Un greffon peut être installé sans recompilation de l’agent.
6. Une application Vite utilise uniquement le SDK public.
7. Les workflows ne contiennent aucun secret.
8. Le cloud est facultatif.
9. Le core ne connaît aucun fournisseur métier concret.
10. Les protocoles publics sont versionnés.
11. Les crashs de greffons ne font pas tomber l’agent.
12. La CLI, les applications et les dashboards utilisent les mêmes services.
13. Les workflows sont persistés et auditables.
14. Les permissions des applications sont limitées.
15. Les modes local, service et Docker partagent le même runtime.

---

# 22. Positionnement

## Formulation longue

> Patchcord est un runtime open source, extensible et local-first permettant de connecter des systèmes, d’exécuter des workflows et de construire des applications intelligentes. Il fonctionne aussi bien sur une machine locale que sur un serveur ou dans un conteneur. Ses capacités sont ajoutées par des greffons indépendants, tandis que ses SDK permettent de créer des applications complètes au-dessus du moteur.

## Formulation courte

> **The extensible runtime for integrations, workflows and intelligent apps.**

## Promesse

> **Connect anything. Automate everything. Build on top.**

## Piliers

```text
Core
→ mécanismes

Plugins
→ capacités

Workflows
→ orchestration

Applications
→ expérience

SDK
→ écosystème

Deployment
→ liberté
```

---

# 23. Décision fondatrice

Patchcord ne sera pas une application desktop à laquelle on ajoute progressivement un moteur.

Patchcord sera un **agent universel**.

Une application desktop officielle pourra exister, mais elle restera une distribution et une interface parmi d’autres.

Le projet doit donc être construit autour de trois frontières stables :

```text
API publique des clients
Protocole public des greffons
Format public des packages et workflows
```

Tout le reste pourra évoluer.

---

# 24. Conclusion

Patchcord a pour ambition de devenir une infrastructure légère et extensible permettant de construire rapidement des intégrations, des automatisations et des applications métier intelligentes.

Le projet ne cherche pas à imposer un SaaS, une interface ou un mode de déploiement.

Il fournit un runtime commun :

```text
sur un poste local
sur un serveur
dans Docker
derrière une application desktop
au cœur d’une application métier
```

Sa valeur repose sur :

- un noyau minimal et stable ;
- des protocoles publics bien conçus ;
- des greffons indépendants ;
- des workflows portables ;
- des applications découplées ;
- une sécurité explicite ;
- une grande liberté de déploiement.

> **Le core fournit les mécanismes. Les greffons fournissent les capacités. Les workflows fournissent l’orchestration. Les applications fournissent l’expérience.**
