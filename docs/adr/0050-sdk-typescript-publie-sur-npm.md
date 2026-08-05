# ADR-0050 — Le SDK TypeScript est publié sur npm, et le scaffold Vite en dépend par défaut

## Statut
Accepté

## Contexte
`@glmtsolutions/patchcord-sdk` (`sdk/typescript`) existait déjà, mais n'était consommé que depuis l'intérieur du monorepo, en dépendance `file:` relative (`apps/examples/dashboard/package.json` : `"@glmtsolutions/patchcord-sdk": "file:../../../sdk/typescript"`). C'est un chemin qui ne marche que parce que le dashboard vit à un emplacement fixe et connu du dépôt.

Lucas a demandé que le template Vite scaffoldé par `patchcord app new --template vite`/`patchcord bundle new --template vite` (`internal/apps/scaffold.go`) embarque le SDK TS prêt à l'emploi. Le blocage : `app new`/`bundle new --template vite dir` acceptent un `dir` **arbitraire**, potentiellement hors du monorepo (ex. `patchcord bundle dev ~/mes-projets/littlecrm`) — il n'existe donc aucun chemin relatif `file:...` universel à écrire en dur dans le `package.json` généré.

Trois options ont été présentées :
1. Publier `@glmtsolutions/patchcord-sdk` sur le registre npm public — le scaffold écrit une dépendance npm normale (`"@glmtsolutions/patchcord-sdk": "^0.1.0"`), qui marche peu importe où `dir` se trouve.
2. Détecter si `dir` est sous le monorepo et calculer un `file:` relatif dans ce cas, scaffolder sans la dépendance sinon.
3. Vendorer le `dist/` buildé du SDK directement dans le dossier scaffoldé.

Lucas a choisi l'option 1.

## Décision
`@glmtsolutions/patchcord-sdk` est publié sur le registre npm public, sous ce nom, en accès public (`publishConfig.access: "public"` — nécessaire pour tout package scope, `@glmtsolutions/*` ou autre). Le nom initialement visé, `@patchcord/sdk`, aurait supposé une organisation npm nommée exactement `patchcord` ; ce nom d'organisation s'est révélé indisponible sur le registre, d'où le repli sur `glmtsolutions`, organisation déjà créée par Lucas. `sdk/typescript/package.json` porte désormais les métadonnées attendues d'un package publié (`license`, `repository`, `homepage`, `bugs`, `keywords`) et `sdk/typescript/README.md` sert de page npm.

`ScaffoldVite` (`internal/apps/scaffold.go`) déclare `"@glmtsolutions/patchcord-sdk": "^0.1.0"` dans le `package.json` qu'il génère, et `src/main.ts` instancie un `PatchcordClient` et affiche le statut de santé de l'agent (`client.system.health()`) dès le chargement — un `npm install` dans le projet scaffoldé suffit, aucun câblage manuel. `src/vite-env.d.ts` (`/// <reference types="vite/client" />`) est ajouté au scaffold pour que `import.meta.env.DEV` type-check.

**Ajustement CORS (même session) :** une première version de `main.ts` distinguait `vite dev` (`baseUrl` en dur sur `http://127.0.0.1:7331`, cross-origin) de l'app buildée (`window.location.origin`, même origine). Ça marche tant qu'aucun jeton admin n'existe — mais ADR-0045 fait que l'agent arrête d'envoyer des en-têtes CORS dès le premier `patchcord auth token create`, ce qui bloque alors `vite dev` (cas réel rencontré par Lucas avec le bundle `smallcrm`). Correction : `vite.config.ts` scaffoldé porte désormais un `server.proxy` qui relaie `/v1` vers `http://127.0.0.1:7331` côté serveur Node de Vite, et `main.ts` utilise uniquement `window.location.origin` — le navigateur ne parle jamais qu'à une seule origine (`localhost:5173` en dev, l'agent lui-même une fois buildé/installé), donc aucun CORS n'entre en jeu, avec ou sans auth admin activée.

Le code interne au monorepo (`apps/examples/dashboard`, `bundles/examples/*`) continue d'utiliser la dépendance `file:` vers `sdk/typescript` en développement — publier sur npm n'oblige pas les exemples du dépôt à consommer leur propre paquet publié, et évite un aller-retour de publication à chaque changement du SDK pendant son développement actif.

La publication effective (`npm login` + `npm publish --access public` depuis `sdk/typescript`) reste un acte manuel de Lucas — hors du périmètre qu'un agent peut exécuter sans compte npm ni confirmation explicite à chaque nouvelle version.

## Conséquences positives
- Le template Vite est réellement « prêt à l'emploi » avec l'API de l'agent, y compris hors du monorepo — cas d'usage réel visé par cette demande (ex. `patchcord bundle dev` sur un projet développé ailleurs sur le disque).
- Aucune détection de chemin fragile (option 2) : `npm install` résout la dépendance de la même façon partout, monorepo ou non.
- Le SDK devient une vraie frontière publique versionnée (non-négociable §1.5) : un changement cassant dans `sdk/typescript` oblige désormais à une discipline semver explicite avant publication, ce qui était implicite jusqu'ici.

## Conséquences négatives
- Publier sur npm crée un canal de distribution externe à maintenir (versions, changelog implicite via semver, dépréciations éventuelles) qui n'existait pas quand le SDK ne vivait que dans le monorepo.
- Deux façons de consommer `@glmtsolutions/patchcord-sdk` coexistent (npm pour un projet externe, `file:` pour le code du monorepo) — un contributeur qui ajoute un nouvel exemple sous `apps/examples/` ou `bundles/examples/` doit savoir laquelle utiliser (réponse : `file:`, comme le dashboard).
- Un oubli de publier une nouvelle version du SDK après un changement d'API laisse le scaffold Vite pointer vers une version npm périmée tant que personne ne relance `npm publish` — pas d'automatisation CI pour ça à ce stade.
- Le nom de package publié (`@glmtsolutions/patchcord-sdk`) ne correspond pas au nom du produit (`Patchcord`) ni au module Go public (`github.com/lucasglmt/patchcord`, ADR-0011) — un contributeur qui chercherait naïvement `@patchcord/sdk` sur npm ne le trouvera pas. Si l'organisation npm `patchcord` devient disponible plus tard, migrer vers ce nom est possible (`npm deprecate` sur l'ancien scope + republication sous le nouveau) mais referait cette décision — non traité tant que le besoin ne se présente pas.
