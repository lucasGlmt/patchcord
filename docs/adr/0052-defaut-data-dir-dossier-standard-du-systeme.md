# ADR-0052 — Défaut `--data-dir` : dossier standard du système plutôt que `./data`

## Statut
Accepté

## Contexte
Le défaut intégré de `--data-dir`/`PATCHCORD_DATA_DIR` était `./data`, un chemin **relatif au dossier courant** — posé par l'ADR-0038 pour `serve` seule, puis repris tel quel par l'ADR-0049 en étendant la variable d'environnement à toutes les commandes ponctuelles (`bundle`, `plugin`, `app`, `workflow`, `run`, `auth`, `connector`, `trust`, `secret`, `registry`).

Lucas a buté sur la conséquence de ce choix en pratique : sans `--data-dir`/`PATCHCORD_DATA_DIR` explicite, deux invocations lancées depuis deux dossiers différents résolvent silencieusement **deux bases SQLite différentes** — plugins installés, connecteurs, secrets, workflows publiés, jetons admin, tout redevient vide dans un nouveau dossier. Pour un agent local-first pensé comme un seul agent continu (non-négociable #1 et #2 du CLAUDE.md), cette dépendance au dossier courant est le vrai problème, pas un détail d'ergonomie : `./data` ne se comporte pas comme une base d'agent persistante, mais comme un cache par-projet accidentel.

La discussion est partie d'une piste plus étroite (générer un `.envrc` par bundle scaffoldé pour `direnv`), mais Lucas a explicitement redirigé vers la cause racine : un dossier standard du système, résolu une fois pour toutes par utilisateur/machine, indépendamment du dossier courant — l'équivalent de ce que font la plupart des CLI locales (`~/.config`, `~/.local/share`, `%LOCALAPPDATA%`...).

La création paresseuse de la base au premier usage existe déjà (`persistence.Open` fait `os.MkdirAll` puis migre) pour n'importe quel `--data-dir`, y compris `./data` aujourd'hui — ce mécanisme n'a pas besoin de changer, seul le défaut résolu en amont change. Une commande `patchcord init` explicite a été envisagée puis écartée par Lucas : elle n'apporterait rien que la création paresseuse ne fasse déjà.

## Décision
Le défaut intégré de `--data-dir`/`PATCHCORD_DATA_DIR` (le niveau le plus bas de la précédence posée par l'ADR-0038 et étendue par l'ADR-0049 — flag > variable d'environnement > défaut intégré, sans fichier `--config` en dehors de `serve`) devient un **dossier standard par utilisateur**, suivant la convention de chaque OS :

- macOS : `~/Library/Application Support/patchcord`
- Linux/BSD : `$XDG_DATA_HOME/patchcord`, ou `~/.local/share/patchcord` si `XDG_DATA_HOME` n'est pas défini
- Windows : `%LOCALAPPDATA%\patchcord`

Implémentation : `internal/config.DefaultDataDir()` (nouveau, `internal/config/datadir.go`), dont la logique par OS est isolée dans `defaultDataDirFor(goos, getenv, homeDir)` pour être testée en table sur les trois branches depuis un seul binaire de test, sans dépendre de l'OS qui exécute réellement `go test`. Si le dossier home ne peut pas être résolu (conteneur minimal sans `$HOME`, par exemple), elle se rabat sur l'ancien défaut relatif `./data` plutôt que d'échouer — `--data-dir`/`PATCHCORD_DATA_DIR` restent de toute façon un override complet.

`internal/cli/serve.go` reste le point d'entrée unique qui expose ce défaut à toutes les commandes (`var defaultDataDir = config.DefaultDataDir()`, résolu une fois), exactement comme l'ancienne constante `"./data"` — aucun autre fichier CLI ne change, ils référencent déjà l'identifiant `defaultDataDir`.

Aucune commande d'installation/initialisation n'est ajoutée : la création paresseuse existante (`persistence.Open` → `MkdirAll` + migration, déclenchée par la première commande qui touche la base) suffit, qu'elle pointe vers `./data` ou vers ce nouveau dossier système. C'est un changement de *valeur* du défaut, pas de *mécanisme*.

## Conséquences positives
- Résout directement le problème vécu par Lucas : toutes les commandes lancées par le même utilisateur, depuis n'importe quel dossier, partagent la même base par défaut — plus de disparition apparente de plugins/connecteurs/secrets selon le `cwd`.
- Cohérent avec la conséquence négative déjà notée par l'ADR-0049 (« un opérateur qui configure `PATCHCORD_DATA_DIR` pour `serve` doit quand même le repasser... ») : avec ce nouveau défaut, il n'a souvent plus besoin de le repasser du tout.
- Changement localisé à un seul point de résolution (`internal/config.DefaultDataDir()` + une ligne dans `internal/cli/serve.go`) — toutes les autres commandes CLI continuent de référencer le même identifiant `defaultDataDir`, aucune ne change.
- N'ajoute aucun nouveau mécanisme de fichier ni de commande : la précédence flag > env > défaut posée par l'ADR-0038/ADR-0049 est inchangée, seule la valeur du dernier niveau change.
- Isoler une base par projet (bundle en développement, test, CI) reste possible et explicite via `--data-dir`/`PATCHCORD_DATA_DIR` — rien ne régresse pour cet usage.

## Conséquences négatives
- Change un comportement déjà documenté (ADR-0038, ADR-0049, `docs/book/src/cli/configuration.md`) : un script ou une habitude qui comptait implicitement sur `./data` relatif au dossier courant doit désormais passer `--data-dir`/`PATCHCORD_DATA_DIR` explicitement pour retrouver ce comportement.
- Deux utilisateurs sur la même machine, ou un même utilisateur passant de son poste à un conteneur/CI sans `$HOME` cohérent, peuvent atterrir sur des bases différentes sans s'en rendre compte — moins visible qu'un `./data` qu'on voit apparaître dans le dossier courant après coup.
- La résolution dépend maintenant de variables d'environnement propres à l'OS (`XDG_DATA_HOME`, `LOCALAPPDATA`) en plus de `PATCHCORD_DATA_DIR` — une source de confusion de plus si un jour un utilisateur cherche pourquoi son dossier de données n'est pas celui attendu.
