# ADR-0054 — Commande `patchcord dev` unifiant serve, bundle dev et le serveur de dev de l'app embarquée

## Statut
Accepté

## Contexte

Développer un bundle avec une app embarquée (typiquement le template
`--template vite`) exige aujourd'hui de lancer, dans l'ordre et dans trois
terminaux séparés :

```bash
patchcord bundle dev ./my-bundle --watch
patchcord serve
cd my-bundle/app && npm run dev
```

Lucas a remonté cette friction directement : trois commandes à retenir et à
relancer dans le bon ordre à chaque session de travail, pour un besoin qui
reste toujours le même (l'agent tourne, le bundle est réinstallé à chaque
sauvegarde, l'app a son propre serveur de dev). Aucune de ces trois
commandes n'a de raison de fonctionnelle de rester séparée pendant une
session de développement — seule leur combinaison change selon qu'on
développe un bundle sans app (2 commandes utiles) ou avec (3).

## Décision

Nouvelle commande `patchcord dev <dir>` qui compose les trois, sans
dupliquer aucune logique (non-négociable #8) :

1. Tente `runtime.NewAgent` avec la même résolution de configuration que
   `serve` (désormais extraite dans `resolveRuntimeConfig`,
   `internal/cli/serve.go`, partagée par les deux commandes). Si le bind du
   listener échoue avec `EADDRINUSE`, un agent tourne déjà sur cette
   adresse — `dev` le réutilise silencieusement au lieu d'échouer, plutôt
   que de sonder l'agent existant via une requête HTTP : `NewAgent` bind
   déjà son listener avant de retourner, ce qui donne un signal fiable et
   gratuit (aucune requête HTTP à construire, aucun endpoint de santé à
   inventer pour ce seul usage).
2. Installe `dir` et le surveille exactement comme `bundle dev --watch`
   (`bundles.InstallDir` + le même watcher de fichiers,
   `internal/cli/watch.go`).
3. Si l'app embarquée déclare un script npm `dev`, le lance (`npm run dev`
   par défaut, `--app-dev-cmd` pour le surcharger, `--no-app-dev` pour le
   désactiver) — détecté via `bundle.yaml`'s `app` (ou son répertoire
   parent, pour couvrir le cas Vite où `app` pointe vers `app/dist` mais
   `package.json` vit dans `app/`).

Les trois composants sont supervisés avec `golang.org/x/sync/errgroup` : la
première erreur réelle (pas un échec de réinstallation pendant le watch,
qui reste seulement affiché comme aujourd'hui) annule le contexte partagé
et arrête proprement les deux autres ; `Ctrl+C` arrête normalement les
trois.

`bundle dev` n'est pas retiré : reste utile en scriptable/CI, ou quand un
`serve` est déjà géré ailleurs (agent partagé, conteneur) et qu'on ne veut
itérer que sur le bundle.

## Conséquences positives

- Une seule commande à retenir et à relancer pour le cas d'usage courant
  (bundle avec app embarquée en développement actif).
- Aucune logique dupliquée : `dev` appelle exactement les mêmes fonctions
  internes que `serve` et `bundle dev` (`runtime.NewAgent`,
  `bundles.InstallDir`, `watchDir`) — un bug corrigé dans l'une de ces
  commandes reste corrigé pour `dev`.
- La détection "agent déjà démarré" ne coûte rien de plus qu'un appel déjà
  nécessaire (`NewAgent`) — pas de sondage HTTP, pas de nouvel endpoint.

## Conséquences négatives

- La supervision de trois composants concurrents (agent, watch, sous-processus
  npm) dans un seul process introduit de la complexité d'orchestration
  (`golang.org/x/sync/errgroup`, gestion de groupe de processus pour arrêter
  proprement `npm run dev` et ses enfants) qui n'existait dans aucune
  commande CLI précédente.
- `--app-dev-cmd` découpe la commande sur les espaces sans invoquer de shell
  — pas de `&&`, pas de globbing ; documenté, pas une limitation surprise,
  mais une limitation réelle pour un script de dev inhabituel.
- La détection du répertoire npm de l'app embarquée reste une heuristique à
  deux candidats (`app`'s field, puis son parent) — correcte pour les deux
  templates existants (`static`, `vite`), pas garantie pour une structure de
  projet radicalement différente qu'un développeur construirait à la main.
