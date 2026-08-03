# ADR-0036 — Authentification admin par jetons, opt-in

## Statut
Accepté

## Contexte
L'API publique n'a jamais eu d'authentification de niveau admin — un fait déjà documenté à deux endroits avant cette décision : [ADR-0024](0024-declenchement-asynchrone-workflows-api-http.md) note que router au-delà de `127.0.0.1` sans authentification relève explicitement de la Phase 6 ("authentification distante") ; [ADR-0026](0026-applications-manifeste-hebergement-session-limitee.md) flague que `POST /v1/apps/{id}/sessions` n'est protégé par rien et "devra être revu en même temps" que l'authentification admin arrivera. Ce qui existe (`internal/auth.Store`, les sessions d'application) est un mécanisme différent et déjà scopé — un jeton limité aux permissions déclarées par le manifeste d'une app, en mémoire, jamais persisté — pas une extension possible vers un accès admin complet.

Après le trigger `schedule` ([ADR-0035](0035-trigger-schedule-scheduler-persistant.md)), Lucas a choisi ce chantier comme deuxième étape de la Phase 6, précisément parce qu'ouvrir l'agent au-delà de `127.0.0.1` (webhooks, Docker, exposition serveur) sans rien pour protéger l'accès serait dangereux par défaut.

Un piège de conception a été identifié et évité avant l'implémentation : l'intuition naturelle — "n'exiger un jeton que si l'agent écoute sur autre chose que `127.0.0.1`" — est exactement le branchement conditionnel local/serveur interdit par le non-négociable #2 de CLAUDE.md ("le même binaire fonctionne en local et sur serveur — pas de branchement conditionnel 'mode local' vs 'mode serveur' dans la logique métier"). Le déclencheur ne pouvait donc pas dépendre de l'adresse d'écoute.

Trois décisions ont été prises avec Lucas avant l'implémentation :

1. **Déclencheur d'application** : opt-in par état de données plutôt que toujours actif. Tant qu'aucun jeton admin n'a jamais été créé, l'API reste aussi ouverte qu'aujourd'hui — zéro friction en local, y compris pour le dashboard en développement. Dès qu'un premier jeton existe, l'authentification s'applique à toutes les requêtes admin-gated, même en local. C'est un état de la base (`SELECT EXISTS(...)` sur la table des jetons), jamais une lecture de l'adresse d'écoute.
2. **Sessions d'application** : `POST /v1/apps/{id}/sessions` exige désormais un jeton admin dès qu'un en existe — fermant explicitement la dette flaguée par l'ADR-0026. Avant, n'importe qui joignant l'agent pouvait se faire émettre une session pour n'importe quelle app installée.
3. **Surface de gestion** : CLI uniquement pour cette première version (`patchcord auth token create/list/revoke`), pas de route HTTP de gestion des jetons — le tout premier jeton ne pourrait de toute façon pas passer par une API déjà verrouillée, et une surface HTTP supplémentaire n'apporte rien tant qu'aucun besoin réel (rotation à distance, par exemple) ne l'exige.

C'est une décision d'architecture au sens de CLAUDE.md section 6 : elle étend la frontière publique de l'API (tous les endpoints admin-gated), introduit un nouveau composant du core (`internal/auth`'s token store) et une nouvelle table persistée.

## Décision

**Modèle de jeton.** Un seul niveau, "admin" — accès complet, non scopé, à l'opposé d'une session d'application. Généré aléatoirement (32 octets, `crypto/rand`), préfixé `pcat_` pour être visuellement distinct d'un jeton de session (un UUID brut) dans un log. Seul son hash SHA-256 est stocké (`internal/auth/token.go`, table `admin_tokens` — `migrations/0007_admin_tokens.sql`) ; le hachage simple (pas bcrypt/argon2) est approprié parce qu'un jeton est déjà un secret à haute entropie, pas un mot de passe choisi par un humain. Le texte en clair n'est affiché qu'une seule fois, à la création — aucune récupération possible ensuite, seulement la création d'un nouveau jeton et la révocation de l'ancien.

**Déclencheur.** `auth.AnyTokensExist(ctx, db)` — une requête `SELECT EXISTS(...)` bon marché, appelée à chaque requête HTTP par le middleware plutôt que mise en cache, cohérent avec l'approche du projet de ne pas complexifier avant qu'un vrai besoin de performance ne l'exige. Tant qu'elle renvoie `false`, chaque route admin-gated se comporte exactement comme avant cette fonctionnalité.

**Routage (`internal/api/router.go`, `internal/api/adminauth.go`).** Trois catégories :
- **Jamais protégées** : `GET /v1/system/health` (une sonde de vivacité doit répondre avant qu'un appelant puisse prouver quoi que ce soit), `GET /v1/openapi.json` (documentation publique, convention habituelle même pour une API authentifiée), `GET /apps/{id}/` (sert le HTML/JS/CSS d'une application installée à l'utilisateur final qui la charge dans son navigateur — cet utilisateur n'est jamais censé détenir un jeton admin).
- **Protégées par `withAdminAuth`** dès qu'un jeton existe : toutes les autres routes — listing et détail de workflows, runs (liste/détail/annulation/événements), connecteurs (CRUD + test), greffons, liste des apps, et `POST /v1/apps/{id}/sessions` (point 2 ci-dessus).
- **Cas particulier, `withRunAuth`** sur `POST /v1/workflows/{id}/run` : la seule route qu'une application installée doit pouvoir appeler avec sa seule session, jamais avec un jeton admin à elle. Accepte donc soit un jeton admin (accès complet, comme toute autre route admin-gated), soit une session d'application valide et scopée à ce workflow — en réutilisant `appSessionAllowsRun`, extrait du comportement `withOptionalAppSession` original de l'ADR-0026, qui reste inchangé tant qu'aucun jeton admin n'existe.

**Gestion.** `patchcord auth token create <name>` (imprime le jeton une seule fois), `list` (jamais le texte en clair ni son hash), `revoke <id>` — ouvre SQLite directement, même patron que `workflow install`/`connector create`. Révoquer le dernier jeton restant ramène l'API à son état par défaut, ouvert.

**Documentation OpenAPI.** `@securityDefinitions.apikey BearerAuth` dans `internal/api/doc.go`, `@Security BearerAuth` sur chaque endpoint admin-gated — `make swagger` régénère `api/agent/swagger.json`/`.yaml` en conséquence.

## Conséquences positives
- Ferme la dette documentée depuis deux ADR (0024, 0026) sans casser le flux local actuel : un agent fraîchement démarré, ou un dashboard en développement, continue de fonctionner sans aucune friction tant que personne n'a créé de jeton.
- Respecte le non-négociable #2 : aucune branche "mode local" vs "mode serveur" nulle part dans le code — le comportement dépend uniquement de l'existence d'un jeton, jamais de l'adresse d'écoute.
- Une application installée n'a jamais besoin d'un jeton admin pour fonctionner — sa session reste suffisante sur la seule route qu'elle appelle, même une fois l'authentification admin activée.
- Réutilise le mécanisme de transport déjà existant (`Authorization: Bearer <token>`, déjà utilisé par les sessions d'app) — aucun changement nécessaire côté SDK TypeScript, qui accepte déjà un jeton générique à la construction du client.

## Conséquences négatives
- Un seul niveau de jeton ("admin", accès complet) — pas de jetons à portée réduite (lecture seule, un workflow précis...). Écarté pour cette première version faute de besoin réel identifié, comme les autres granularités déjà différées ailleurs dans le projet (providers de secrets, politique d'erreur déclarative).
- Aucune expiration ni rotation automatique des jetons — ils vivent jusqu'à révocation explicite. Acceptable pour un usage local/petite équipe, pas pour un usage serveur multi-utilisateurs à grande échelle.
- `AnyTokensExist` interroge la base à chaque requête plutôt que de mettre en cache l'état "authentification activée" — coût negligible aujourd'hui (table minuscule, requête indexée), à revisiter seulement si ça devient un goulot d'étranglement mesuré.
- Pas de route HTTP de gestion des jetons : un opérateur distant doit avoir un accès shell/CLI à la machine qui héberge l'agent pour créer ou révoquer un jeton. Cohérent avec le choix "CLI uniquement" de cette version, mais une contrainte réelle pour un déploiement serveur pur.
