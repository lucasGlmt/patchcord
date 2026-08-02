# ADR-0020 — Modèle de connecteur et références de secrets par variable d'environnement

## Statut
Accepté

## Contexte
La phase 4 ("Connecteurs") couvre huit éléments : modèle de connecteur,
configuration, références de secrets, tests, binding, capacités, greffon HTTP,
greffon IA, greffon PostgreSQL. C'est trop pour un seul incrément — cette passe
livre uniquement les deux premiers éléments (modèle + références de secrets), en
CRUD autonome, testable sans attendre qu'un greffon consommateur de connecteur
existe. Même logique que pour la phase 2 (`text.uppercase@1` avant tout le reste) et
la phase 3 (moteur de base d'abord, timeouts/annulation/SSE en passes séparées,
ADR-0017 → ADR-0018 → ADR-0019).

ADR-0009 avait explicitement anticipé cette dépendance : les secrets ne doivent
jamais transiter dans les workflows, et cette contrainte "doit être anticipée dans
l'ordre des phases (connecteurs en phase 4)". Cette ADR ferme ce renvoi.

`internal/connectors/` et `internal/secrets/` existaient déjà comme dossiers vides
(`.gitkeep` uniquement) depuis le commit initial — la structure du dépôt réservait
déjà leur place (CLAUDE.md section 2).

## Décision

**Le connecteur est une ressource CRUD à part entière**, distincte des greffons et
des workflows : `Connector{ID, Type, Config, SecretRefs, CreatedAt}`. `ID` est un
slug choisi par l'utilisateur (comme `workflow.Definition.ID`), pas un UUID.
`connector create` **rejette les doublons** plutôt que d'écraser silencieusement
(contrairement à `plugin install`, qui fait un upsert par id) : un ID de connecteur
est une référence stable que d'autres workflows pointeront une fois le binding
implémenté — un upsert sur une faute de frappe de commande pourrait re-pointer
discrètement la config/les secrets d'un connecteur déjà utilisé ailleurs. C'est un
simple `INSERT` (contrainte `PRIMARY KEY`), avec l'erreur de contrainte SQLite
traduite en `ErrAlreadyExists` — pas de pré-lecture puis insertion, qui serait une
race TOCTOU face à `patchcord serve` tournant en parallèle sur le même fichier
SQLite. Pas de commande `update` dans cette passe : modifier un connecteur, c'est
`remove` puis `create`.

**Les secrets ne sont jamais persistés**, même chiffrés : `secret_refs` ne stocke
que des références logiques (`{"type":"env","key":"PG_PASSWORD"}`), résolues à la
volée par un `secrets.Store`. Aucune nouvelle table pour les secrets eux-mêmes.

**Les variables d'environnement comme premier (et seul) adaptateur de secrets** :
choisi parce qu'il n'introduit aucune nouvelle dépendance crypto/keychain, reste
identique en local et sur serveur (non-négociable #2), et — plus fort qu'un
"chiffré en base avec une clé qu'il faut gérer quelque part" — la valeur du secret
ne touche jamais le fichier SQLite. `EnvStore.Resolve` rejette explicitement tout
`Reference.Type != "env"`, pour que `Type` ne soit pas un champ décoratif. **Mise en
garde explicite** : une variable d'environnement n'est isolée qu'à la hauteur du
process/utilisateur qui lance l'agent — nettement plus faible qu'un vrai
Keychain/Credential Manager/Vault. C'est un adaptateur de démarrage, pas la cible
finale ; les autres adaptateurs listés en section 15.3 du document de vision restent
à construire.

**Validation du type de référence dès la création**, pas seulement à la résolution :
`connectors.Create` rejette un `secret_refs` dont un `Type` n'est pas supporté (via
`secrets.ValidateType`), pour attraper une faute de frappe (`emv:` au lieu de
`env:`) immédiatement. La résolution elle-même (variable effectivement positionnée)
reste vérifiée paresseusement, à `connector inspect` — on peut légitimement créer un
connecteur avant d'avoir exporté la variable qu'il référence.

**Convention de nommage du `Type`** (documentation seulement, rien de vérifié en
code pour l'instant) : `--type` est encouragé à suivre la même convention que les
ids d'action (`<nom>.<sous-type>@<version>`, ex. `postgresql.connection@1`) plutôt
qu'un mot nu (`postgresql`), documentée dans l'aide de `connector create`. Objectif :
que les connecteurs créés maintenant aient une chance de correspondre le jour où la
validation contre le catalogue des greffons arrivera.

**Quatre exclusions de scope explicites** (pas des oublis) :
- *Validation de `Type` contre les connecteurs déclarés par les greffons installés* :
  aucun greffon aujourd'hui n'en déclare (`text` ne déclare que des actions) — valider
  maintenant rendrait la fonctionnalité intestable à la main tant qu'aucun greffon
  connecteur n'existe. Activer cette validation plus tard ne nécessite de gate que les
  futurs `Create`, pas de migration des lignes déjà en base (même précédent que
  `workflow.Validate`, qui ne revalide jamais rétroactivement une version déjà
  installée quand le catalogue de greffons change).
- *`patchcord connector test`* : un vrai test de connexion doit être délégué à un
  greffon (non-négociable #3 — le core ne peut pas savoir tester une connexion
  PostgreSQL/HTTP lui-même), et aucun greffon connecteur n'existe encore. À la place,
  `connector inspect` affiche, pour chaque référence de secret, si elle *résout*
  actuellement — un diagnostic honnête mais plus étroit qu'un vrai test de connexion,
  qui laisse le nom `test` libre pour la vraie commande plus tard plutôt que de
  devoir la renommer.
- *Le binding d'un connecteur à une étape de workflow*, et l'extension du protocole
  de greffons (RPC `ExecuteAction`) pour transporter la config résolue d'un
  connecteur. Le document de vision montre `connector: "${{ bindings.ai_provider
  }}"` — une expression vers une map `bindings`, pas un simple champ scalaire sur
  `Step`. Ça mérite sa propre conception quand un greffon consommateur réel
  (HTTP/PostgreSQL) existera, pour la valider contre un cas d'usage réel plutôt que
  de la deviner maintenant.
- *Typage de `Config`* : peuplé uniquement par des flags CLI `--config k=v`
  aujourd'hui, donc toutes les valeurs sont des strings — même limitation déjà
  acceptée pour `workflow run --input`, pas une régression nouvelle. Un `connector
  create --config-file` façon `workflow install <path.yaml>` est la réponse
  naturelle plus tard.

## Conséquences positives
- Ferme le renvoi explicite d'ADR-0009 : les secrets ont maintenant un mécanisme de
  référence utilisable, avant même qu'un greffon connecteur existe.
- Le modèle CRUD (`internal/connectors`) et la résolution de secrets
  (`internal/secrets`) sont testables et démontrables indépendamment de tout
  protocole ou binding — même stratégie de tranche verticale minimale que les phases
  précédentes.
- Aucune nouvelle dépendance externe (pas de crypto, pas de bibliothèque keychain) ;
  `internal/secrets` reste un point d'extension propre (interface `Store`) pour les
  adaptateurs futurs sans casser les appelants actuels.
- Les quatre exclusions de scope sont documentées explicitement, donc la prochaine
  passe (binding + greffon HTTP/PostgreSQL) ne repart pas de zéro sur ces questions.

## Conséquences négatives
- Un connecteur créé aujourd'hui avec un `Type` mal formé (`postgresql` au lieu de
  `postgresql.connection@1`) ne sera pas détecté avant l'activation de la validation
  contre le catalogue de greffons — seulement une convention documentée, rien
  d'imposé.
- La sécurité réelle des secrets dépend entièrement de l'isolation du process agent
  et de son environnement — pas de chiffrement au repos, pas de rotation, pas
  d'audit d'accès. Acceptable pour un adaptateur de démarrage local-first, mais à ne
  pas présenter comme suffisant pour un déploiement serveur multi-utilisateurs sans
  un adaptateur plus robuste (Keychain/Vault) d'abord.
- `connector inspect` ne prouve rien sur la connexion réelle — un connecteur peut
  passer "resolved" sur toutes ses références et rester complètement inutilisable
  (mauvais mot de passe, hôte injoignable). Ne pas confondre avec un vrai `connector
  test` tant que celui-ci n'existe pas.
- Sans commande `update`, changer un seul champ de configuration impose de tout
  recréer (`remove` + `create`), avec une fenêtre où le connecteur n'existe plus.
