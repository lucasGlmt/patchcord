# ADR-0061 — Page de listage des apps sous `/apps/`

## Statut
Accepté

## Contexte

`GET /apps/{id}/` sert déjà l'interface statique d'une application installée (ADR-0026). Mais `GET /apps` (ou `/apps/`, sans id) répondait jusqu'ici par un 404 brut du `http.ServeMux` — aucune route ne le prenait en charge — et il n'existait aucun moyen de découvrir depuis un navigateur quelles apps sont installées sur un agent donné, sans déjà en connaître l'id.

Lucas a demandé une page de listage façon Apache (liste de liens cliquables), configurable, et **utilisable sur tout Patchcord installé** — pas un artefact local. Trois points nécessitaient un arbitrage explicite avant implémentation :

1. **Auth** : la page doit-elle être protégée par jeton admin (comme `GET /v1/apps`) ou publique (comme `GET /apps/{id}/`) ?
2. **Comportement par défaut** : activée d'emblée, ou opt-in ?
3. **Contenu** : le manifeste d'app actuel ne porte que `id` et `version` (pas de nom affichable ni description) — faut-il l'étendre maintenant ?

Lucas a tranché : publique, désactivée par défaut, contenu minimal (id + version), sans étendre le manifeste.

## Décision

**Nouvelle route `GET /apps/`**, enregistrée dans `internal/api/router.go` à côté de `GET /apps/{id}/` — le `http.ServeMux` de la stdlib (Go 1.22+) les distingue sans ambiguïté : `{id}` exige un segment non vide, donc `/apps/` (segment vide) ne peut matcher que le nouveau pattern littéral, jamais l'ancien. Elle reste **hors de `withAdminAuth`**, pour la même raison que `GET /apps/{id}/` : elle n'expose rien qu'un end user ne pourrait déjà atteindre une app à la fois en devinant/recevant son id — au pire elle rend cette découverte plus confortable, elle ne franchit pas de nouvelle frontière de confidentialité.

**Nouveau réglage `apps.directory_listing.enabled`** dans `internal/config.Config`, suivant exactement le mécanisme de précédence fichier → env → flag déjà en place (ADR-0038) :
- Fichier YAML : `apps.directory_listing.enabled: true`.
- Variable d'environnement : `PATCHCORD_APPS_DIRECTORY_LISTING_ENABLED` (valeur parsée par `strconv.ParseBool`; une valeur invalide est traitée comme absente plutôt que de faire échouer le démarrage — ce réglage ne vaut pas la peine de refuser de démarrer).
- Pas de flag CLI dédié pour ce premier jet — rien ne l'exigeait, cohérent avec la discipline "pas d'ajout sans besoin réel" déjà pratiquée pour ce package.
- **Défaut : `false`** — `GET /apps/` continue de répondre 404 tant que l'opérateur n'a pas activé le réglage explicitement (ADR-0007 : pas de changement de comportement sans un geste conscient).

`Config.Merge` traite ce booléen par "OR" plutôt que par la sémantique "vide = pas d'avis" utilisée pour les champs `string` : un `override.Enabled == true` active, un `override.Enabled == false` ne désactive jamais un `true` posé par une source de précédence inférieure. C'est une limitation assumée (aucune source ne peut aujourd'hui forcer la désactivation depuis une couche plus prioritaire) — sans conséquence tant qu'aucun flag CLI n'existe pour ce réglage, puisque rien ne le pousserait à `true` accidentellement en amont d'une couche qui voudrait le couper.

**Contenu de la page : id + version uniquement**, aucune extension du manifeste (`api/app/`). Le gabarit HTML vit dans `internal/api/templates/apps_directory.html.tmpl`, chargé via `//go:embed` et rendu avec `html/template` (échappement automatique — les id d'app viennent de la base mais sont rendus comme n'importe quelle donnée non fiable). C'est le premier rendu HTML du core (jusqu'ici uniquement `application/json` et le passthrough `http.FileServer` d'une app installée) — un choix jugé sans risque architectural : ni dépendance externe (stdlib uniquement), ni logique métier, ni frontière publique nouvelle au-delà de la route elle-même.

## Conséquences positives
- Un opérateur peut naviguer `http://<agent>/apps/` pour découvrir les apps installées sans connaître leurs id à l'avance — sur n'importe quel agent Patchcord, local ou serveur, dès que le réglage est activé (même binaire, même mécanisme de config partout — non-négociable #2).
- Comportement par défaut inchangé (404) : aucune migration, aucun agent existant ne change de comportement sans action explicite de son opérateur.
- Réutilise intégralement le mécanisme de config à trois couches (ADR-0038) et le service `apps.List` déjà existant (`GET /v1/apps`) — aucune nouvelle surface de persistance.

## Conséquences négatives
- Le manifeste n'ayant ni `name` ni `description`, la page n'affiche que des id techniques — moins lisible qu'un vrai "directory index" applicatif. Une extension future du manifeste (nom, description, icône) devra reprendre cette page si Lucas la juge nécessaire.
- `Config.Merge` ne peut pas désactiver ce réglage depuis une couche plus prioritaire — acceptable aujourd'hui (pas de flag CLI), mais à revisiter si un flag `--apps-directory-listing` est ajouté plus tard : il faudrait alors une vraie sémantique tri-state (`*bool`) plutôt que l'OR actuel.
- Introduit `html/template` et un fichier `.tmpl` embarqué dans `internal/api` — premier pattern de rendu HTML du core, qu'il faudra garder cohérent si d'autres pages HTML apparaissent plus tard (ne pas laisser deux conventions de gabarits diverger).
