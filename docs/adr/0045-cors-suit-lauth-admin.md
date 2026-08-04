# ADR-0045 — Le CORS de l'API publique suit l'activation de l'auth admin

## Statut
Accepté

## Contexte

`internal/api/router.go`'s `withCORS` posait `Access-Control-Allow-Origin: *`
de façon inconditionnelle sur toute l'API publique, y compris les routes
protégées par `withAdminAuth`/`withRunAuth`. Ce choix (ADR-0024) répondait à
un besoin réel : un serveur de dev Vite, sur une autre origine que l'agent,
doit pouvoir appeler l'API pendant le développement d'une application.

Mais `withAdminAuth` (ADR-0036) est un mécanisme opt-in : tant qu'aucun
opérateur n'a créé de premier jeton admin (`patchcord auth token create`),
toute requête passe sans authentification — c'est le comportement par
défaut d'un agent fraîchement installé, cohérent avec le non-négociable
« le cloud reste facultatif » et l'esprit local-first du projet.

La combinaison des deux est le problème : avec un CORS `*` inconditionnel,
n'importe quelle page ouverte dans le même navigateur peut scripter des
requêtes vers `http://127.0.0.1:7331/v1/...` et lire la réponse — lister
workflows/runs/connecteurs, déclencher un run, lire les événements SSE —
sans autre interaction utilisateur que d'avoir un onglet ouvert quelque
part. C'est exactement la fenêtre que l'ADR-0036 cherchait à limiter, et le
CORS permissif la rouvrait entièrement pour tout agent qui n'a pas encore
été explicitement sécurisé — c'est-à-dire l'état par défaut.

## Décision

Le CORS de l'API publique suit désormais le même signal que
`withAdminAuth` : `auth.AnyTokensExist`.

- **Aucun jeton admin n'existe** : comportement inchangé, CORS permissif
  (`Access-Control-Allow-Origin: *`), pour ne pas casser le flux de
  développement d'application actuel (ADR-0024).
- **Au moins un jeton admin existe** : les en-têtes CORS permissifs ne sont
  plus émis du tout. Un navigateur refuse alors la lecture cross-origin de
  toute réponse, même si la requête elle-même atteint l'agent.
- **Erreur de lecture de la base** (table absente, DB injoignable) : on
  échoue fermé — pas d'en-têtes permissifs — le gestionnaire sous-jacent
  reportera de toute façon la vraie erreur 500 via son propre appel à
  `auth.AnyTokensExist`.

Les appels same-origin (CLI, `curl`, une application servie depuis
`/apps/{id}/` elle-même) ne sont pas affectés dans un sens comme dans
l'autre : un navigateur ne consulte pas les en-têtes CORS pour une requête
same-origin.

Conséquence assumée : un développeur d'application qui utilise un serveur
Vite en dev **et** a déjà créé un jeton admin sur le même agent perd le
CORS permissif pour cet usage-là. Le contournement est de développer contre
un agent sans jeton admin (état par défaut), ou de servir l'app depuis
`/apps/{id}/` en same-origin. Une restriction d'origines explicite
(allowlist configurable) reste hors scope de cette décision — elle n'est
introduite que si le besoin se confirme.

## Conséquences positives
- Referme la fenêtre « n'importe quelle page web pilote l'agent local » dès
  qu'un opérateur active l'authentification admin — sans rien changer au
  comportement par défaut que les utilisateurs connaissent déjà.
- Aucun nouveau flag de configuration : réutilise un signal qui existe déjà
  (`auth.AnyTokensExist`), cohérent avec le modèle opt-in de l'ADR-0036.
- Le CORS n'a jamais été et ne devient pas une frontière de sécurité à lui
  seul (ADR-0024 reste valable sur ce point) — mais il cesse d'annuler
  silencieusement celle que `withAdminAuth` fournit.

## Conséquences négatives
- Un développeur d'application qui active l'auth admin sur son agent de dev
  doit changer de flux (agent sans jeton, ou same-origin) pour continuer à
  utiliser un serveur Vite cross-origin.
- Une requête OPTIONS de preflight coûte désormais une lecture DB
  (`auth.AnyTokensExist`) au lieu d'être répondue sans aucun accès —
  négligeable en local, à surveiller si un jour le CORS doit répondre à un
  débit élevé.
