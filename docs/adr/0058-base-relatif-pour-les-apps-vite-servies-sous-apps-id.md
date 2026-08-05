# ADR-0058 — Base relative (`base: "./"`) pour les applications Vite servies sous `/apps/{id}/`

## Statut
Accepté

## Contexte

`handleServeApp` (`internal/api/apps.go`, ADR-0026) sert une application
installée sous `GET /apps/{id}/` via `http.StripPrefix` +
`http.FileServer(http.Dir(app.StaticDir))`. L'id de montage n'existe qu'au
moment de l'installation (`patchcord-app.yaml`) — il n'est connu ni du code
source de l'application, ni de sa configuration de build.

Vite, par défaut (`base: "/"`), émet dans `index.html` des URLs d'assets
absolues à la racine du domaine (`/assets/index-xxxx.js`). Une fois l'app
installée et servie sous `/apps/{id}/`, le navigateur résout ces URLs
contre l'origine (`http://127.0.0.1:7331/assets/...`), pas contre
`/apps/{id}/index.html` — 404 systématique sur tous les fichiers statiques
générés, page blanche. `npm run dev`/`vite preview` ne révèlent jamais ce
défaut : Vite y sert lui-même à la racine, donc les URLs absolues
tombent juste par coïncidence. Le bug n'apparaît qu'au premier
`app install`/`bundle install` d'un build réel — repéré sur
`fr.glmtsolutions.test`, une app tierce, mais qui aurait frappé
`apps/examples/dashboard` et tout scaffold `--template vite` dès leur
premier déploiement.

## Décision

**Toute application Vite destinée à être servie par l'agent déclare
`base: "./"` dans `vite.config.ts`.** Une base relative rend chaque URL
émise relative à `index.html` lui-même plutôt qu'à la racine du domaine —
elle se résout correctement sous n'importe quel `{id}`, sans que le build
ait besoin de connaître son point de montage final. C'est la seule option
qui ne demande aucune information au build : une base absolue
(`base: "/apps/dashboard/"`) fonctionnerait mais coderait l'id en dur dans
la configuration, cassant la moindre réinstallation sous un id différent.

Corrigé à deux endroits, cohérents avec la portée de chacun :

- `internal/apps/scaffold.go` (`scaffoldViteConfig`, `patchcord app new
  --template vite` / `patchcord bundle new --template vite`) — tout
  nouveau projet scaffoldé démarre avec `base: "./"` et un commentaire
  expliquant pourquoi, pour que le prochain lecteur ne le retire pas par
  erreur en pensant simplifier la config.
- `apps/examples/dashboard/vite.config.ts` — l'application de référence
  suit sa propre recommandation ; son `dist/` reconstruit confirme le
  correctif (`./assets/index-*.js` au lieu de `/assets/index-*.js`).

Le core lui-même ne change pas : `handleServeApp` reste un
`http.FileServer` générique, sans connaissance du bundler qui a produit
les fichiers qu'il sert — cohérent avec le non-négociable #3 (aucune
capacité métier concrète dans `internal/`). C'est délibérément une
convention documentée pour les auteurs d'applications, pas un mécanisme
que l'agent applique ou vérifie.

## Explicitement hors scope (différé, pas oublié)

- Réécriture des URLs à la volée côté serveur (un `handleServeApp` qui
  injecterait `/apps/{id}/` dans le HTML servi) — résoudrait le même
  problème sans coopération de l'auteur d'app, mais ferait de l'agent un
  proxy HTML avec état, pour un problème que la configuration de build
  résout proprement à la source.
- Vérification à l'installation qu'une app buildée n'a pas d'URLs
  absolues cassées (`apps.Install`/`InstallOrUpdate` ne parsent aujourd'hui
  aucun fichier statique) — laisserait l'agent inspecter le contenu HTML
  d'une app, hors de la portée de son rôle d'hébergement générique.
- Autres bundlers (Webpack, Parcel, esbuild direct) — même principe
  (chemin relatif ou base configurée), non documenté ici faute d'exemple
  de référence dans le dépôt utilisant l'un d'eux.

## Conséquences positives

- `patchcord app new --template vite` / `bundle new --template vite`
  produisent désormais un projet installable tel quel — build, install,
  ouvrir dans un navigateur — sans piège silencieux au premier déploiement
  réel.
- `apps/examples/dashboard`, seule app Vite du dépôt jusqu'ici, sert de
  preuve vérifiée (`npm run build` + `patchcord app install` +
  `curl /apps/dashboard/assets/...` → 200) plutôt que d'une affirmation.
- Aucun changement de surface publique (API, protocole de greffons, format
  de package) : correctif localisé aux gabarits d'applications.

## Conséquences négatives

- La convention n'est appliquée qu'aux projets scaffoldés par l'agent ou à
  l'exemple de référence — une application Vite écrite indépendamment (ou
  migrée depuis un autre gabarit) doit encore appliquer `base: "./"`
  elle-même ; rien dans `apps.Install`/`InstallOrUpdate` ne le détecte ni
  ne le signale si elle l'omet, ce qui reproduit la même page blanche
  silencieuse tant que ce n'est pas documenté ailleurs qu'ici et repéré par
  l'auteur de l'app.
