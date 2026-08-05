# ADR-0053 — Réinstallation de bundle idempotente pour un workflow embarqué inchangé

## Statut
Accepté

## Contexte

Bug remonté par Lucas : `patchcord bundle dev` (avec ou sans `--watch`)
échouait de façon répétée, y compris sur un bundle tout juste créé, avec :

```
bundle dev: install bundle "fr.glmtsolutions.claycrm" workflow "workflows/main.yaml":
record workflow fr.glmtsolutions.claycrm_workflow version 1: constraint failed:
UNIQUE constraint failed: workflow_versions.workflow_id, workflow_versions.version (1555)
```

`internal/cli/bundle.go`'s `newBundleDevCommand` (backé par
`bundles.InstallDir`) réinstalle **tout** le bundle — app embarquée et
chaque workflow embarqué — à chaque déclenchement de `--watch`
(`internal/cli/watch.go`'s `watchDir`), quel que soit le fichier
effectivement modifié : un changement qui ne touche que l'app (par ex. la
sortie de build d'un `vite build --watch` embarqué) déclenche quand même
une tentative de réinstallation de chaque workflow embarqué.

Or `internal/runs/store.go`'s `InstallWorkflow` fait un simple `INSERT`
dans `workflow_versions (workflow_id, version, definition)`, sans
condition : la contrainte `UNIQUE(workflow_id, version)` qui garantit
l'immutabilité d'ADR-0008 rejette donc **toute** réinstallation d'une
version déjà enregistrée, y compris quand le contenu du fichier workflow
n'a strictement pas changé. Résultat : une fois qu'un workflow embarqué a
été installé une première fois avec succès, chaque déclenchement suivant
de `--watch` — même pour une modification totalement étrangère au workflow
— échoue systématiquement sur ce même workflow, en boucle, rendant `bundle
dev --watch` inutilisable au-delà de la toute première sauvegarde.

`bundles.InstallDir`'s propre commentaire documentait déjà l'intention
inverse de ce que le code faisait : « editing a workflow's body requires
bumping its `version` field » — cette phrase distingue implicitement
« changer le contenu sans bumper la version » (à rejeter, cas réel
couvert par `TestInstallDir_RejectsRedeclaringAWorkflowVersionWithDifferentContent`)
de « réinstaller un contenu identique » (qui n'était couvert par aucun
test et se comportait, en pratique, comme le premier cas).

C'est le même type de bug que celui qui a motivé ADR-0044 côté app
embarquée (`installEmbeddedApp` appelait `apps.Install`, strict, au lieu
d'`apps.InstallOrUpdate`) : un comportement « strict par défaut » qui
convient à une commande explicite pilotée par un développeur
(`workflow install <file>`, où une version non bumpée est probablement une
erreur à signaler), mais qui casse une boucle de réinstallation automatique
pilotée par un watcher de fichiers, où l'absence de changement est le cas
courant, pas l'exception.

## Décision

**`runs.InstallWorkflow` reste strict**, inchangé : c'est la primitive
utilisée directement par `workflow install` et par le protocole public de
greffons/workflows — réinstaller la même `(id, version)` telle quelle y
reste une erreur (`TestInstallWorkflow`'s « rejects reinstalling the exact
same version »), pour continuer à signaler explicitement un oubli de bump
de version lors d'une publication manuelle.

**Nouvelle fonction interne à `internal/bundles`**,
`installWorkflowIfChanged(ctx, db, source, knownActions)` : parse `source`,
compare son contenu byte-à-byte à la version déjà installée pour le même
`(id, version)` (via `runs.WorkflowSource`) :

- absente ou différente → délègue à `runs.InstallWorkflow` sans changement
  de comportement (installe, ou rejette si le contenu diffère à version
  égale — ADR-0008 intact) ;
- strictement identique → no-op silencieux, retourne la définition déjà
  installée sans toucher la base.

Utilisée aux deux points d'entrée qui réinstallent un bundle dans son
ensemble : `bundles.InstallDir` (`bundle dev`) et
`installEmbeddedWorkflow` (`bundles.InstallPackage`, donc `bundle
install`/`bundle update`) — les deux souffraient du même bug pour la même
raison (réinstallation globale d'un bundle déjà installé).

## Conséquences positives

- Débloque `bundle dev --watch` pour l'usage réel qu'il est censé servir :
  itérer sur l'app embarquée sans toucher aux workflows ne fait plus
  échouer chaque réinstallation.
- Réinstaller deux fois de suite un bundle inchangé (`bundle install`
  relancé sans modification) devient également un no-op propre au lieu
  d'échouer sur le workflow embarqué — cohérent avec l'upsert déjà en place
  pour l'app embarquée (ADR-0044) et pour la ligne de provenance du bundle
  (`bundles.record`).
- Tests de régression ajoutés dans `internal/bundles/package_test.go` :
  `TestInstallDir_ReinstallingUnchangedWorkflowIsANoOp` et
  `TestInstallPackage_ReinstallingUnchangedPackageIsANoOp` — les deux
  échouaient avant le correctif avec exactement l'erreur remontée par
  Lucas.
- `TestInstallDir_RejectsRedeclaringAWorkflowVersionWithDifferentContent`
  et `TestInstallWorkflow`'s « rejects reinstalling the exact same
  version » continuent de passer sans modification : la garantie
  d'immutabilité d'ADR-0008 pour un contenu réellement différent est
  intacte, à tous les niveaux.

## Conséquences négatives

- Une comparaison de contenu supplémentaire (`runs.WorkflowSource`) est
  faite avant chaque installation de workflow embarqué — coût négligeable
  (une lecture SQLite locale) mais un aller-retour de plus qu'avant.
- Le no-op est silencieux : une réinstallation qui ne change réellement
  rien ne produit aucune trace différenciée dans la sortie de `bundle dev`/
  `bundle install` par rapport à une première installation réussie — pas de
  régression fonctionnelle, mais un développeur cherchant à confirmer
  qu'aucun workflow n'a changé ne trouvera pas cette information dans les
  logs actuels.
