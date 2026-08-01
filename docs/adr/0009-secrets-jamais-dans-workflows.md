# ADR-0009 — Les secrets ne transitent jamais dans les workflows

## Statut
Accepté

## Contexte
Un workflow est sérialisé en YAML/JSON, versionné, potentiellement exporté, partagé entre environnements ou stocké dans un système de contrôle de version par l'utilisateur. Si un workflow pouvait contenir une valeur de secret en clair (jeton d'API, mot de passe), chaque export, chaque partage et chaque version publiée deviendrait une surface de fuite potentielle.

## Décision
Une définition de workflow ne contient jamais de valeur de secret. Elle contient uniquement des références logiques (par exemple un identifiant de connecteur ou de binding), résolues à l'exécution par le Secret Manager, lui-même adossé à un magasin de secrets (Keychain, Credential Manager, Secret Service, Vault, variables d'environnement, etc.).

## Conséquences positives
- Un workflow peut être exporté, versionné ou partagé sans risque de fuite de secret, quel que soit le canal utilisé.
- La rotation d'un secret n'impose aucune modification du workflow qui l'utilise.
- L'audit de sécurité peut se concentrer sur le Secret Manager et les connecteurs plutôt que sur chaque définition de workflow individuellement.

## Conséquences négatives
- Aucune action ne peut résoudre un secret sans passer par l'indirection connecteur/Secret Manager, ce qui ajoute une étape de résolution à chaque exécution touchant un système externe.
- Le débogage d'un run devient légèrement plus indirect : il faut inspecter la référence logique puis le connecteur associé pour comprendre quel secret a réellement été utilisé.
- Le Secret Manager et le modèle de connecteur doivent exister et être stables avant qu'une action utilisant un secret puisse fonctionner de bout en bout — cette dépendance doit être anticipée dans l'ordre des phases (connecteurs en phase 4).
