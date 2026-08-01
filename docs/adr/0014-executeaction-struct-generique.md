# ADR-0014 — ExecuteAction et Struct générique pour les entrées/sorties d'action

## Statut
Accepté

## Contexte
La passe précédente (ADR-0013) n'avait posé que le RPC `Handshake` : suffisant pour découvrir un greffon, pas pour exécuter quoi que ce soit. Pour valider réellement la tranche verticale de référence de la section 20 du document de vision (« développer → compiler → lancer → exécuter, sans recompiler le core »), il fallait que le greffon d'exemple `text.uppercase@1` soit véritablement invocable, pas seulement déclaratif.

Le modèle complet d'action (entrées/sorties typées, capacités, connecteurs compatibles, erreurs connues, timeout — section 7.4) n'est pas encore conçu : il appartient normalement au compilateur de workflows (section 12.5, phase 3). Concevoir ce modèle de typage complet maintenant, uniquement pour faire fonctionner un greffon de démonstration, aurait anticipé une décision qui n'est pas encore mûre.

## Décision
Le protocole gagne un RPC `PluginService.ExecuteAction(ExecuteActionRequest) returns (ExecuteActionResponse)`. Les entrées et sorties utilisent `google.protobuf.Struct` — un type bien connu, générique, proche d'un objet JSON — plutôt qu'un message typé par action. Côté SDK Go, cela se traduit par de simples `map[string]any` (`ActionInput` / `ActionOutput`).

## Conséquences positives
- Débloque la preuve complète de la boucle développer→compiler→lancer→exécuter sans attendre la conception du modèle de typage complet des actions.
- `Struct` est un type protobuf standard, déjà supporté nativement par tous les runtimes protobuf, sans encodage maison à inventer ni à documenter.
- Surface minimale : un seul RPC, deux messages génériques — cohérent avec l'esprit "core minimal" de cette phase.

## Conséquences négatives
- Aucune sécurité de type à la compilation ni au niveau du protocole : une incompatibilité de type sur une entrée d'action n'est détectée qu'à l'exécution (ex. l'assertion `input["value"].(string)` de `text.uppercase@1`).
- Cette forme générique sera très probablement complétée par un schéma typé par action (référencé depuis le manifeste, potentiellement JSON Schema) une fois que le compilateur de workflows en aura besoin pour valider les entrées d'une étape avant le démarrage d'un run (section 12.5) — `ExecuteActionRequest` gagnera alors sans doute une référence de schéma plutôt que d'être remplacé.
- `Struct` ne représente pas tous les types sans perte (pas de distinction int64/float64, pas de binaire natif sans encodage base64) — acceptable pour le greffon d'exemple actuel, à revisiter quand de vrais connecteurs/actions (phase 4) auront besoin de types plus riches.
