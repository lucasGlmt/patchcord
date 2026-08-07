# ADR-0069 — Génération de types depuis les descripteurs d'actions

## Statut
Accepté

## Contexte

Chaque greffon déclare des JSON Schemas pour les entrées/sorties de ses
actions et la configuration de ses connecteurs (ADR-0062). Le moteur de
workflows utilise déjà ces schémas pour valider les steps à la compilation
(ADR-0063). Les développeurs d'applications TypeScript/React qui consomment
ces actions doivent aujourd'hui re-déclarer manuellement les types que le
greffon expose déjà — une source de désynchronisation entre le contrat du
greffon et le code applicatif.

## Décision

Patchcord fournit une commande CLI `patchcord dev codegen <plugin-id> --ts`
qui génère des interfaces TypeScript typées depuis les descripteurs JSON
Schema d'un greffon installé.

- Source : lecture directe du catalogue SQLite via `plugins.Get()` — pas
  besoin d'agent en cours d'exécution.
- Premier langage cible : TypeScript (`--ts`). Le flag est extensible
  (`--dart`, `--java`) sans changement de l'interface existante.
- Le fichier généré est écrit dans `--out` (défaut : répertoire courant)
  sous le nom `<plugin-id>.ts`.
- Pas de régénération automatique ni de mode watch : la commande est
  exécutée explicitement par le développeur.
- Le moteur de conversion vit dans `internal/codegen/`, séparé de la CLI,
  pour faciliter l'ajout de futurs langages cibles.

## Conséquences positives

- Les types applicatifs sont toujours synchronisés avec le contrat du
  greffon — une seule source de vérité (le JSON Schema déclaré par le
  greffon).
- Zéro dépendance externe : la conversion est du pur traitement de
  `map[string]any` vers du texte.
- Extensible à d'autres langages cibles sans modifier la commande CLI
  existante.

## Conséquences négatives

- Le sous-ensemble de JSON Schema supporté est volontairement limité aux
  constructions réellement utilisées par les greffons existants ; les cas
  avancés (`oneOf`, `allOf`, `$ref`) génèrent `unknown` plutôt que de
  tenter une conversion complexe.
- La commande ne gère pas les conflits de noms entre actions de plugins
  différents — chaque fichier généré est isolé par plugin.
