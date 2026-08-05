# ADR-0051 — Une commande CLI pour minter une session d'application hors du navigateur

## Statut
Accepté

## Contexte
Le scaffold Vite (ADR-0050) appelle `client.apps.createSession(appId)` depuis le code du navigateur — le seul mécanisme documenté jusqu'ici (`building-with-sdk-ts.md`). Lucas a buté dessus en pratique avec `smallcrm` : dès qu'un jeton admin existe sur l'agent, `POST /v1/apps/{id}/sessions` l'exige aussi (ADR-0036, point 2, qui ferme explicitement la dette laissée par ADR-0026). Un jeton admin ne devant jamais transiter dans du code livré à un navigateur, cet appel ne peut structurellement plus fonctionner depuis le navigateur une fois l'authentification admin active — quelle que soit la configuration CORS (voir aussi ADR-0045, déjà traité dans cette même session pour un problème voisin).

Le document de vision (§10.2) esquissait déjà la bonne forme sans jamais l'outiller : `token: window.__PATCHCORD_SESSION_TOKEN__` suppose qu'une session est fournie à l'application depuis l'extérieur, jamais négociée par elle-même. Rien n'existait pour produire concrètement cette valeur.

Contrainte technique qui a guidé la conception : une session vit **uniquement en mémoire** du process `patchcord serve` en cours (`internal/auth/session.go`, ADR-0026) — jamais en base SQLite. Contrairement à toute autre commande `app`/`bundle` (qui écrivent dans la base et laissent l'agent, s'il tourne, refléter le changement à la prochaine requête), une commande qui mint une session ne peut pas se contenter d'écrire dans la base : elle doit véritablement appeler l'agent en cours d'exécution en HTTP. C'est la première commande CLI de tout le dépôt à faire ça.

## Décision
Nouvelle commande `patchcord app session create <id>` :
- Vérifie que `<id>` est installé (lecture directe en base, `apps.Get`, comme toute autre commande `app`).
- Appelle elle-même `POST {--base-url}/v1/apps/<id>/sessions` sur l'agent en cours d'exécution (`--base-url`, défaut `http://127.0.0.1:7331` — le même défaut que `serve`), avec `--admin-token` (ou `PATCHCORD_ADMIN_TOKEN`) en `Authorization: Bearer` si fourni. L'agent lui-même décide si un jeton était requis (règle opt-in d'ADR-0036) — la commande ne duplique pas cette logique, elle relaie simplement l'appel.
- Écrit le résultat (`{"token": "..."}`) en JSON dans `<static-dir>/patchcord-session.json` par défaut (`--output` pour surcharger) — même origine que les fichiers de l'application, donc son propre code peut le récupérer par un simple `fetch("/patchcord-session.json")` au démarrage, sans jamais appeler `createSession` lui-même.

Convention côté application (documentée dans `building-with-sdk-ts.md`, pas ajoutée au SDK lui-même — trois lignes de `fetch` ne justifient pas une nouvelle méthode publique) : tenter de lire `patchcord-session.json` en même origine ; si absent (agent pas encore configuré pour cette app, ou dev local sans jamais avoir lancé cette commande), retomber sur un client sans token — même filet de sécurité que le pattern déjà documenté pour `createSession` qui échoue.

`internal/cli/appsession.go` isole ce code (le seul appel HTTP sortant de tout le CLI) plutôt que de le mélanger à `app.go`, avec un commentaire explicite sur la raison technique (session en mémoire, pas en base) pour qu'un futur contributeur ne soit pas tenté de le "simplifier" vers le patron des autres commandes `app`.

Testé en mockant le transport (`httptest.Server`), jamais contre un vrai `patchcord serve` — cohérent avec la règle déjà appliquée au protocole de greffons (CLAUDE.md section 5).

## Conséquences positives
- Répond directement au trou d'outillage identifié : il existe maintenant une réponse concrète à "comment faire dans un vrai bundle", conforme à ce qu'esquissait déjà le document de vision.
- Aucune session ne transite plus jamais par le navigateur pour être négociée — seul le jeton déjà minté y arrive, exactement le modèle de moindre privilège qu'ADR-0036 visait.
- Aucun changement du modèle de persistance des sessions (`internal/auth/session.go` reste en mémoire, inchangé) — la commande s'adapte à cette contrainte plutôt que de la remettre en cause.
- Testable sans process externe (`httptest.Server`), cohérent avec la règle déjà en vigueur pour le protocole de greffons.

## Conséquences négatives
- Premier cas où une commande CLI fait un appel HTTP sortant vers l'agent plutôt que de passer uniquement par la base SQLite partagée — un léger écart par rapport au patron uniforme de toutes les autres commandes `app`/`bundle`, documenté explicitement dans le code pour ne pas être pris pour un oubli.
- Le fichier `patchcord-session.json` doit être re-généré après chaque rebuild qui remplace le contenu de `static-dir` (un nouveau `vite build`, un nouvel `app install`) — aucune automatisation de ce ré-enchaînement pour l'instant (pourrait être une étape de plus dans `bundle dev --watch` à l'avenir, non traité ici faute de besoin confirmé).
- Le jeton admin doit être fourni explicitement à cette commande (`--admin-token`/`PATCHCORD_ADMIN_TOKEN`) à chaque appel — aucune récupération possible depuis la base (ADR-0036 : seul le hash est stocké), cohérent avec le modèle de sécurité mais un peu de friction opérationnelle à chaque rotation.
- Pour le template `static` (non-Vite) de `app new`, `static-dir` est le dossier source lui-même (pas de séparation build/dist) — un `patchcord-session.json` y atterrirait aux côtés du code source, sans `.gitignore` pour l'en protéger aujourd'hui. Non traité ici (aucun `.gitignore` n'existe déjà pour ce template) ; à surveiller si quelqu'un utilise cette commande sur une app scaffoldée en `static`.
