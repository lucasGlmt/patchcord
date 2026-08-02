# CLAUDE.md — Patchcord Core

Ce fichier encadre le travail de Claude Code sur le dépôt **Patchcord** (anciennement *GLMT Compagnon*). Lis-le entièrement avant toute action. Il prime sur toute intuition générique "projet Go standard".

## 0. Ce qu'est Patchcord (résumé obligatoire)

Patchcord est un **agent d'exécution universel, local-first**, écrit en Go, qui :

- charge et supervise des **greffons** (processus indépendants, jamais liés statiquement au core) ;
- gère des **connecteurs** (configurations persistantes d'accès à un système externe) ;
- exécute des **actions** (opérations atomiques déclarées : entrées, sorties, capacités, timeout) ;
- orchestre des **workflows** (déclaratifs, versionnés, immuables une fois publiés) ;
- expose une **API publique stable** (HTTP/SSE/WebSocket) consommée par une CLI, un SDK TypeScript et des applications tierces.

Principe directeur à respecter dans **chaque** décision de conception :

> Le core fournit les mécanismes. Les greffons fournissent les capacités. Les workflows fournissent l'orchestration. Les applications fournissent l'expérience.

Devise produit : *Connect anything. Automate everything. Build on top.*

Le document de référence complet est `PATCHCORD_VISION_ARCHITECTURE.md` dans le dossier `docs/`. **En cas de doute sur une décision d'architecture, consulte ce document avant de trancher toi-même** — ne devine pas, ne réinvente pas une architecture "standard" à sa place.

## 1. Non-négociables architecturaux

Ces règles viennent directement du document de vision (section 21, "Critères architecturaux de réussite"). Toute contribution doit les respecter :

1. Patchcord Agent doit pouvoir fonctionner **sans interface graphique**.
2. Le même binaire fonctionne en local et sur serveur — pas de branchement conditionnel "mode local" vs "mode serveur" dans la logique métier.
3. Le **core ne doit jamais importer, référencer ou connaître un service métier concret** (pas de `import "github.com/.../openai"`, pas de `import "github.com/.../gmail"` dans `internal/`). Ces capacités arrivent uniquement via le protocole de greffons.
4. Un greffon ne doit dépendre **que** du protocole public et du SDK Go (`sdk/go-plugin`) — jamais d'un package `internal/`.
5. Les frontières publiques (API clients, protocole de greffons, format des packages/workflows) sont contractuelles : Protobuf / JSON Schema / OpenAPI, **versionnées**. L'intérieur du core peut changer librement ; ces trois frontières non.
6. Les workflows ne contiennent **jamais** de secret — uniquement des références logiques résolues via le Secret Manager.
7. Un crash de greffon ne doit jamais faire tomber l'agent (isolation de processus + supervision).
8. CLI, applications et dashboards passent tous par les **mêmes services internes** que l'API publique — jamais de logique dupliquée dans la CLI.
9. Le cloud reste facultatif : aucune fonctionnalité du core ne doit exiger un compte distant ou un serveur de licence pour démarrer.

Non-objectifs explicites (ne pas construire, ne pas proposer, sauf demande explicite de Lucas) : canvas visuel complet, SaaS multi-tenant, marketplace, exécution distribuée multi-agents, orchestration Kubernetes, IDE intégré, exécution de code arbitraire dans un workflow.

## 2. Structure du dépôt

Monorepo pendant toute la phase où le protocole évolue vite. Respecte ces frontières **comme si les composants vivaient déjà dans des dépôts séparés** — c'est voulu, ce n'est pas un détail cosmétique :

```
patchcord/
├── cmd/patchcord/          # binaire principal, point d'entrée uniquement
├── internal/
│   ├── runtime/
│   ├── workflow/           # moteur de workflows (machine à états)
│   ├── runs/                # gestionnaire d'exécutions
│   ├── scheduler/
│   ├── plugins/             # supervision de processus greffons
│   ├── connectors/
│   ├── apps/                 # hébergement d'applications web
│   ├── auth/
│   ├── permissions/
│   ├── secrets/
│   ├── persistence/          # SQLite en phase locale
│   └── api/                  # handlers HTTP/SSE/WS
├── api/                       # définitions de contrats publics (OpenAPI/Protobuf)
│   ├── agent/ ├── plugin/ ├── workflow/ └── app/
├── sdk/
│   ├── go-plugin/             # SDK officiel pour écrire un greffon Go
│   └── typescript/
├── plugins/examples/
├── apps/examples/
├── docs/
└── migrations/
```

Règle stricte : rien dans `internal/` ne doit être importable par `plugins/`, `sdk/`, ou `apps/`. Si un greffon d'exemple a besoin de quelque chose d'`internal/`, c'est un signal que ce quelque chose doit passer par `sdk/go-plugin` ou par `api/`.

## 3. Vocabulaire — ne pas confondre

| Terme | Définition stricte |
|---|---|
| **Greffon (plugin)** | Processus indépendant, lancé/supervisé par l'agent, communique en RPC. Jamais chargé en mémoire du core (pas de plugin natif Go `.so`). |
| **Connecteur** | Configuration persistante d'accès à un système (ex. connexion PostgreSQL). Ce n'est **pas** une action. |
| **Action** | Opération atomique exécutable par un workflow (ex. `postgresql.query@1`). Déclare entrées/sorties/capacités/timeout/erreurs connues. |
| **Workflow** | Orchestration déclarative d'actions, versionnée et immuable une fois publiée, sérialisée en YAML/JSON. |
| **Application** | Client de l'agent (Vite, Flutter, Electron...) utilisant uniquement l'API publique et le SDK TypeScript — jamais de session avec privilèges admin complets. |
| **Run** | Instance d'exécution d'un workflow. États : `queued`, `running`, `succeeded`, `failed`, `cancelled`. |

Quand tu écris du code, utilise ces mots exactement dans ce sens. Ne pas inventer de synonymes ("module" pour "greffon", "job" pour "run", etc.) — la cohérence terminologique traverse le code, l'API et la doc.

## 4. Langue officielle du projet : anglais

Tout ce qui vit dans le code source est en **anglais**, sans exception :

- noms de packages, types, fonctions, variables ;
- commentaires et doc Go (`// ...`) ;
- messages d'erreur (`errors.New("...")`, `fmt.Errorf("...")`) ;
- logs structurés (clés et messages) ;
- noms de commandes/flags CLI, textes d'aide ;
- identifiants d'actions/connecteurs/évènements (`postgresql.query@1`, `run.started`...) ;
- contrats publics (`api/`, manifestes, schémas) ;
- messages de commit.

Le français reste la langue de conversation avec Lucas (issues internes, échanges, ce fichier lui-même), mais **jamais** celle du code livré. Si un texte utilisateur destiné à une future UI doit être en français, il passe par un mécanisme d'i18n explicite — il n'est jamais codé en dur en français dans le core.

## 5. Tests unitaires — obligatoires

Aucune contribution de code (nouveau package, nouvelle fonction publique, correctif de bug) n'est complète sans ses tests unitaires associés :

- Tout nouveau comportement dans `internal/` (moteur de workflows, scheduler, plugin supervisor, connecteurs, secrets, permissions...) est accompagné de tests table-driven Go standard (`_test.go` à côté du code testé).
- La machine à états du moteur de workflows (transitions de `Run` et `Step`) doit avoir une couverture de test explicite sur les transitions valides **et** invalides.
- Le protocole de greffons (handshake, manifeste, compatibilité de version) doit être testé indépendamment de toute implémentation réseau réelle — mocker le transport plutôt que de dépendre d'un process externe.
- Un correctif de bug s'accompagne toujours d'un test qui aurait échoué avant le correctif.
- `go test ./...` doit passer avant toute proposition de changement considérée comme terminée. Ne jamais désactiver ou supprimer un test existant pour faire passer une build sans en discuter explicitement.
- Les tests suivent la même règle de langue que le reste du code (noms de tests, assertions, messages) : anglais.

## 6. Architecture Decision Records (`docs/adr/`)

Le dépôt contient un dossier `docs/adr/` : un ADR par décision d'architecture significative, format court (contexte / décision / conséquences).

**Règle stricte : dès qu'une décision d'architecture est prise pendant une session de travail (par toi seul, ou en accord avec Lucas), crée l'ADR correspondant dans `docs/adr/` avant de considérer la tâche terminée.** Ne pas attendre qu'on te le demande explicitement à chaque fois. Une décision d'architecture, ici, veut dire : un choix qui touche une frontière publique (API, protocole de greffons, format de package/workflow), un choix structurant qui serait coûteux à défaire plus tard, ou un choix qui contredit/précise un des non-négociables de la section 1.

ADR déjà existants (ne pas les renuméroter, ne pas les dupliquer) :

```
ADR-0001 — Patchcord Agent est le produit fondamental
ADR-0002 — Les greffons sont des processus externes
ADR-0003 — Les frontières publiques sont indépendantes de Go
ADR-0004 — Le core ne contient aucune intégration métier
ADR-0005 — La CLI et l'API utilisent la même couche applicative
ADR-0006 — Le projet commence dans un monorepo
ADR-0007 — Le cloud n'est jamais requis
ADR-0008 — Les workflows publiés sont immuables
ADR-0009 — Les secrets ne transitent jamais dans les workflows
ADR-0010 — La première version est mono-espace de travail
```

Convention de nommage de fichier : `docs/adr/NNNN-titre-court-en-kebab-case.md`, `NNNN` sur 4 chiffres, en continuité stricte du dernier numéro existant (le prochain est `0011`). Ne jamais réutiliser ou sauter un numéro.

Template obligatoire (français, comme les ADR existants — les ADR font exception à la règle "anglais" de la section 4, car ce sont des documents de décision destinés à Lucas et aux contributeurs francophones du projet, pas du code) :

```markdown
# ADR-NNNN — <titre de la décision>

## Statut
Accepté

## Contexte
<pourquoi cette décision se pose>

## Décision
<ce qui est décidé, formulé sans ambiguïté>

## Conséquences positives
- ...

## Conséquences négatives
- ...
```

Si une décision ultérieure remplace un ADR existant, ne pas modifier l'ADR d'origine : créer un nouvel ADR qui le remplace et passer le statut de l'ancien à `Remplacé par ADR-NNNN` plutôt que de réécrire l'historique.

## 7. Conventions Go

- Go moderne standard : `context.Context` explicite et propagé partout où une opération peut être annulée ou timeout (chaque run, chaque étape).
- Erreurs : `errors.Is`/`errors.As`, wrapping avec `%w`, pas de `panic` en dehors de l'initialisation stricte du process.
- Le moteur de workflows est une **machine à états explicite** — ne pas remplacer la persistance des transitions par de la simple concurrence en mémoire (goroutines ≠ garanties de reprise après redémarrage).
- Logs structurés (pas de `fmt.Println` en dehors du CLI output destiné à l'utilisateur).
- Toute nouvelle capacité "métier concrète" (ex. un appel à une API tierce) qui semble naturelle à ajouter dans `internal/` doit d'abord être questionnée : est-ce qu'elle appartient à un greffon plutôt qu'au core ? (cf. non-négociable #3)
- Les frontières API (`api/`) sont sources de vérité pour les contrats — le code Go interne les implémente, il ne les redéfinit pas ailleurs.

## 8. Méthode de travail attendue (analyse statique avant exploration runtime)

- Avant de modifier ou d'étendre un composant, **lis le code existant et `PATCHCORD_VISION_ARCHITECTURE.md`** plutôt que de lancer le binaire pour "voir ce qui se passe". Priorité à l'analyse statique.
- Ne lance `go run` / `patchcord serve` / tests d'intégration que lorsque c'est nécessaire pour valider un changement précis — pas comme méthode d'exploration par défaut.
- Avant d'ajouter une dépendance externe au core (`internal/`), vérifie qu'elle ne viole pas la règle #3 (aucun service métier concret dans le core).
- Pour toute nouvelle action/connecteur/greffon, vérifie d'abord la tranche verticale de référence en section 20 du document de vision (`text.uppercase@1`) comme patron minimal avant d'ajouter de la complexité.
- En cas d'ambiguïté entre "vitesse de livraison" et "respect des frontières architecturales", privilégier les frontières — le document de vision est explicite sur ce compromis (section 5.6, 23).

## 9. Roadmap actuelle (pour prioriser)

Le projet suit les phases du document de vision (section 19). Sauf indication contraire de Lucas, considère que la phase active est la plus basse **non complétée** :

0. Spécification (vision, vocabulaire, non-objectifs, licence, contrats publics)
1. **Core minimal** — binaire Go, CLI, config, logs structurés, API santé, SQLite, cycle de vie, serveur HTTP local, arrêt propre
2. Protocole de greffons (Protobuf/JSON-RPC, handshake, manifeste, supervision, SDK Go, greffon d'exemple `text.uppercase@1`)
3. Moteur de workflows (modèles versionnés, compilation, runner séquentiel, persistance, historique, événements temps réel)
4. Connecteurs (modèle, secrets, tests, greffons HTTP/IA/PostgreSQL)
5. SDK TypeScript et applications
6. Déploiement serveur (Docker, TLS, secret providers, webhooks)
7. Écosystème (registre, signature, bundles)

Ne pas anticiper des mécanismes de phases ultérieures (ex. registre de greffons, marketplace, sandbox WASM) tant que la phase courante n'est pas stable — cf. non-objectifs section 4 du document de vision.

## 10. Commandes utiles

```bash
go build ./...
go vet ./...
go test ./...
patchcord serve --listen 127.0.0.1:7331   # une fois le binaire buildé
```

## 11. Documentation utilisateur (`docs/book/`)

En plus du document de vision (`docs/PATCHCORD_VISION_ARCHITECTURE.md`, le *pourquoi*) et des ADR (`docs/adr/`, l'historique des décisions), le dépôt contient une documentation utilisateur au format **mdBook** dans `docs/book/` — le *comment utiliser* Patchcord.

- Outil : [mdBook](https://rust-lang.github.io/mdBook/) (`cargo install mdbook`). Zéro dépendance Node, un seul fichier `book.toml`, une nav explicite dans `docs/book/src/SUMMARY.md`. Commandes : `make docs-build` / `make docs-serve`.
- Structure : cinq parties top-level qui reflètent les frontières du dépôt, pas une hiérarchie inventée — **CLI**, **Plugins** (avec les connecteurs en sous-partie, car un connecteur est une configuration exposée par un greffon), **Workflows**, **SDK TypeScript**, **Apps**. Une page d'introduction (`introduction.md`) porte le tableau de vocabulaire et renvoie vers le document de vision plutôt que de le dupliquer.
- Langue : contrairement aux ADR, le contenu de `docs/book/` est en **anglais** (cohérent avec la règle section 4 — ce sont des frontières publiques consommées potentiellement par des tiers).
- Ton : clair et précis, pas de tournures narratives. Cette doc est lue autant par des développeurs que par des agents de codage (dont toi, dans de futures sessions) — privilégie les faits vérifiables (chemins de fichiers, noms de commandes, numéros d'ADR) aux formulations vagues.
- Une page renvoie vers l'ADR ou la section du document de vision concernée plutôt que de réexpliquer un choix d'architecture — pas de duplication entre `docs/book/` et `docs/adr/`.
- Règle de maintenance : toute nouvelle commande CLI, tout nouveau greffon d'exemple, toute nouvelle capacité de SDK ou de manifeste d'application mérite une mise à jour de la page correspondante dans `docs/book/src/`. Ne pas laisser les pages stub ("Placeholder — content pending.") diverger silencieusement du code une fois qu'une section est rédigée.