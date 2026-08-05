# ADR-0055 — Auto-incrémentation de version des workflows embarqués en mode développement

## Statut
Accepté

## Contexte

`bundle dev`/`patchcord dev` installent un workflow embarqué via la même
règle que `workflow install` : une version publiée est immuable
(ADR-0008), donc réinstaller `(id, version)` avec un contenu différent est
rejeté. ADR-0053 a déjà rendu la réinstallation d'un contenu **inchangé**
idempotente (`installWorkflowIfChanged`), pour débloquer `--watch` — mais
un contenu **réellement modifié** sans bump du champ `version:` reste
rejeté, avec une erreur `UNIQUE constraint failed`.

Lucas a remonté cette friction : en développement actif d'un workflow
embarqué, chaque sauvegarde qui touche au corps du workflow exige de penser
à remonter manuellement `version:` avant de resauvegarder, sous peine
d'erreur — un geste qui n'a de sens qu'au moment de *publier* une version,
pas à chaque itération locale.

`runs.LatestWorkflow`/`runs.WorkflowSource(id, 0)` résolvent déjà "la
dernière version installée" — tout déclencheur qui ne pin pas une version
explicite (manuel, `schedule`, `webhook`) en bénéficie automatiquement. Un
auto-bump de version en mode dev est donc totalement transparent en aval :
rien à changer côté exécution, seulement côté installation.

## Décision

En mode dev uniquement — `bundles.InstallDir`, donc `bundle dev` **et**
`patchcord dev` qui délègue au même appel — un contenu qui diffère d'une
version déjà installée est désormais installé sous la version entière
suivante disponible, au lieu d'être rejeté :

- `runs.InstallWorkflowAtVersion(ctx, db, source, version, knownActions)`
  (nouvelle fonction) installe `source` sous `version` plutôt que sous le
  `version:` que le fichier déclare lui-même — la copie *persistée* voit
  son propre champ `version:` normalisé pour correspondre
  (`workflow.RewriteVersion`, remplacement chirurgical de la seule ligne
  `version:` top-level, le reste du texte — commentaires compris — reste
  identique octet pour octet), sinon reparser cette copie plus tard
  (`LatestWorkflow`, `workflow export`) redonnerait la version obsolète
  déclarée dans le fichier plutôt que la version réellement enregistrée.
- `runs.NextWorkflowVersion` calcule la version suivante disponible
  (`MAX(version) + 1`, ou `1` si aucune n'est installée).
- `bundles.installWorkflowForDev` (nouvelle fonction, remplace
  `installWorkflowIfChanged` dans `InstallDir` uniquement) : contenu
  identique à la version déclarée → no-op inchangé (ADR-0053) ; version
  absente → installation normale ; version présente avec un contenu
  différent → auto-bump via les deux fonctions ci-dessus.
- **Le fichier source sur disque n'est jamais réécrit** — seule la copie
  interne stockée en base voit son `version:` normalisé.

`runs.InstallWorkflow` (utilisée par `workflow install` et
`InstallPackage`/`bundle install`/`bundle update`) ne change pas : reste
strict, ADR-0008 intact sans exception pour un package qu'un développeur a
choisi de publier.

## Conséquences positives

- `bundle dev`/`patchcord dev --watch` : éditer le corps d'un workflow
  embarqué et sauvegarder fonctionne toujours, sans bump manuel — la
  friction remontée par Lucas disparaît.
- Aucune régression sur ADR-0008 : `bundle install`/`bundle update`/
  `workflow install` restent strictement sous l'immutabilité, avec les
  mêmes tests (`TestInstallWorkflow`'s "rejects reinstalling the exact same
  version") inchangés.
- Version 1 (et toute version déjà publiée) reste immuable même sous ce
  nouveau chemin : seule une version encore jamais enregistrée peut
  recevoir le nouveau contenu, la version existante n'est jamais réécrite.

## Conséquences négatives

- Le numéro de version qu'un développeur voit s'accumuler dans
  `workflow list`/`bundle dev`'s logs en mode dev ne correspond plus au
  `version:` qu'il a lui-même écrit dans le fichier — une source de
  confusion possible si on regarde uniquement le fichier source sans
  regarder ce que `workflow list`/`LatestWorkflow` rapportent réellement.
- `workflow.RewriteVersion` fait une hypothèse structurelle sur le format
  (un `version:` top-level non indenté, non quoté) qui est vraie pour tout
  ce que `Scaffold`/`ScaffoldTemplate` génèrent aujourd'hui, mais resterait
  à réviser si le format gagnait un jour une syntaxe alternative pour ce
  champ.
