# ADR-0060 — Run.watch() et package React du SDK TypeScript

## Statut
Accepté

## Contexte

Le cas d'usage « lancer un workflow au clic d'un bouton, et afficher sa
progression proprement via SSE » demandait, avec le SDK TypeScript existant
(`sdk/typescript`), de réécrire à la main — dans chaque application — le
même code de plomberie : consommer `Run.events()` (des deltas bruts), les
réduire soi-même en un état complet (liste de steps + statut du run), gérer
séparément le `.fetch()` final pour récupérer les outputs (`events()` ne les
porte jamais), et nettoyer proprement l'abonnement SSE au démontage d'un
composant. `apps/examples/dashboard/src/pages/WorkflowDetailPage.tsx`
illustre ce coût : ~80 lignes d'état React pur avant même de dessiner un
bouton. `apps/examples/greeter/index.html` illustre la conséquence quand ce
coût est jugé trop élevé : il n'utilise même pas le SDK, et fait du polling
`GET /v1/runs/{id}` à la main plutôt que du SSE.

Cette réduction n'appartient pas entièrement à une seule couche :

1. la fusion événements → état (`Run.events()` → un état complet) est
   agnostique de tout framework — c'est une mauvaise ergonomie d'API, pas
   une question de React/Vue/vanilla ;
2. le binding de cette fusion au cycle de vie d'un composant (état
   start/running/done/error, nettoyage au démontage) dépend du framework.

## Décision

Deux changements, un par couche :

1. **Dans `sdk/typescript` (agnostique)** : `Run.watch()`, un générateur
   async qui produit un `RunSnapshot` (statut + erreur + steps déjà
   fusionnés par id) après chaque événement, au lieu des deltas bruts de
   `Run.events()`. Le dernier `RunSnapshot` produit est toujours celui
   réobtenu par un `GET /v1/runs/{id}` (même compromis que `Run.result()`)
   — le seul à porter `outputs` et les `input`/`output` de chaque step, que
   `events()` ne porte jamais. `Run.events()` et `Run.watch()` acceptent
   tous deux une option `{ signal }` (`AbortSignal`) pour arrêter l'écoute
   côté client sans attendre la fin du run — sans jamais annuler le run
   lui-même (`Run.cancel()` reste le seul moyen de faire ça).

2. **Nouveau package `sdk/typescript-react`** (`@glmtsolutions/patchcord-react`),
   frère de `sdk/typescript` plutôt que sous-module de celui-ci — pour que
   `sdk/typescript` reste sans dépendance à un framework web particulier, les
   applications Patchcord n'étant pas toutes React (Vite/React aujourd'hui,
   mais Flutter/Electron envisagés par le vocabulaire du document de vision).
   Il expose un unique hook, `useWorkflowRun(client, workflowId)`, qui
   encapsule `Run.watch()` dans le cycle de vie d'un composant React (état
   `idle/running/succeeded/failed/cancelled`, `start`/`cancel`/`reset`,
   désabonnement au démontage via `AbortController`). Il ne fournit aucun
   composant de rendu (pas de `<RunWorkflowButton>`) — dessiner l'UI reste
   la responsabilité de l'application (document de vision : « les
   applications fournissent l'expérience »), le hook ne retire que la
   plomberie d'état.

## Conséquences positives
- Toute application (React ou non) bénéficie de `Run.watch()` : un seul
  générateur à consommer pour afficher une progression, sans réduire soi-même
  un flux de deltas.
- Les applications React (dont `apps/examples/dashboard`, à terme) peuvent
  remplacer leur logique d'état de run manuelle par `useWorkflowRun`.
- `sdk/typescript` reste sans dépendance framework — la frontière publique du
  SDK « protocole » (non-négociable #5) n'est pas touchée par ce choix
  d'ergonomie React.
- Le désabonnement au démontage ne coupe que la connexion SSE côté client,
  jamais le run lui-même — cohérent avec le fait qu'un workflow est censé
  survivre à l'onglet qui l'a déclenché.

## Conséquences négatives
- Un package npm supplémentaire à versionner et publier (`@glmtsolutions/patchcord-react`),
  avec sa propre chaîne de test (React 18/19, `@testing-library/react`, `jsdom`
  via `global-jsdom`) — de la surface de maintenance en plus pour un existant
  qui n'avait qu'un seul package SDK.
- `RunSnapshot` est un nouveau type public dans `sdk/typescript` ; toute
  évolution de sa forme est désormais soumise à la même discipline de
  compatibilité que le reste du contrat public du SDK.
- Aucun binding équivalent n'existe pour un autre framework (Vue, Svelte...)
  ni pour une application sans build (comme `apps/examples/greeter`) — ces
  cas continuent de consommer `Run.watch()` directement, sans l'ergonomie
  supplémentaire du hook.
