# ADR-0002 — Les greffons sont des processus externes

## Statut
Accepté

## Contexte
Go permet de charger des plugins natifs (`.so`, `plugin` package) directement dans le processus du core. Cette approche est performante et simple à mettre en œuvre, mais elle lie fortement la version du greffon à celle du binaire hôte, ne survit pas à un crash du greffon, impose Go comme langage unique pour tout greffon, et nécessite une recompilation ou au minimum une compatibilité binaire stricte à chaque évolution du core.

## Décision
Les greffons fonctionnent exclusivement comme des processus indépendants, lancés et supervisés par l'agent, communiquant via un protocole RPC public. Aucun greffon n'est jamais chargé en mémoire du core sous forme de plugin natif Go.

## Conséquences positives
- Un crash de greffon n'entraîne jamais la chute de l'agent (isolation de processus).
- Un greffon peut être écrit dans n'importe quel langage capable de parler le protocole, pas uniquement Go.
- Le cycle de vie (installation, mise à jour, désactivation, désinstallation) d'un greffon est indépendant de celui du core, sans recompilation de Patchcord Agent.
- La compatibilité est négociée explicitement au démarrage (handshake), ce qui rend les évolutions de version observables et contrôlables.

## Conséquences négatives
- Le coût de communication inter-processus (sérialisation, RPC) est supérieur à un appel de fonction in-process.
- Le Plugin Supervisor doit gérer une complexité opérationnelle réelle : démarrage, arrêt, redémarrage, health check, timeout, quarantaine.
- Le protocole public doit être conçu, versionné et documenté avec un soin particulier dès la première version, car toute erreur de conception s'y répercute sur l'ensemble de l'écosystème de greffons.
