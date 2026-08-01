# ADR-0016 — Plugin Supervisor : health checks gRPC standard, redémarrage à délai fixe borné

## Statut
Accepté

## Contexte
La checklist de la phase 2 (section 19 du document de vision) prévoit un Plugin Supervisor gérant démarrage, arrêt, redémarrage, health check, timeout, journalisation, détection de crash, quarantaine et désactivation après échecs répétés (section 8.4). Jusqu'ici, le non-négociable #7 — « un crash de greffon ne doit jamais faire tomber l'agent » — n'était vrai que par absence de crash observé en pratique : rien ne détectait ni ne réagissait à un greffon qui meurt de manière inattendue une fois lancé.

Deux choix structurants se posaient : comment vérifier qu'un greffon reste en bonne santé une fois lancé, et quelle politique adopter quand il ne l'est plus (crash ou health check en échec répété).

## Décision
**Health checks** : le SDK (`sdk/go-plugin`) enregistre automatiquement le protocole de santé gRPC standard (`google.golang.org/grpc/health`, service `grpc.health.v1.Health`) pour chaque greffon, avec le statut `SERVING` dès le démarrage. L'agent (`internal/plugins.Supervisor`) interroge `Check()` périodiquement via un `grpc_health_v1.HealthClient` obtenu sur la même connexion que le reste du protocole. Aucun RPC de santé maison n'a été ajouté à `plugin.proto`.

**Détection de crash** : indépendante des health checks — `Process.Exited()` expose un canal fermé dès que le processus se termine (via un unique `cmd.Wait()` interne), que ce soit une fermeture propre ou un crash.

**Politique de redémarrage et quarantaine** : délai fixe entre tentatives (`SupervisorConfig.RestartDelay`), nombre de tentatives borné (`SupervisorConfig.MaxRestarts`, défaut 3). Au-delà, le greffon est mis en quarantaine : retiré de l'ensemble des greffons actifs pour le reste de cette session de l'agent, sans être désinstallé du catalogue. L'état de quarantaine vit uniquement en mémoire — un nouveau démarrage de l'agent redonne sa chance à chaque greffon installé.

Un crash détecté et un health check en échec répété convergent vers la même logique de redémarrage/quarantaine (`Supervisor.restart`), qu'ils soient traités comme deux déclencheurs indépendants dans la boucle de supervision.

## Conséquences positives
- Réutilise un standard éprouvé et interopérable plutôt que d'inventer un RPC de santé maison, gardant `plugin.proto` minimal (continuité avec ADR-0013/ADR-0014).
- Un crash et une simple non-réponse sont traités uniformément par la même machinerie de redémarrage/quarantaine, ce qui simplifie le raisonnement et les tests.
- Politique de retry simple, déterministe et facile à tester (`internal/plugins/supervisor_test.go` couvre redémarrage après crash, quarantaine après échecs répétés, et détection par health check, contre de vrais processus).
- Le non-négociable #7 est désormais vrai par construction et vérifié par un smoke-test manuel (`kill -9` sur un greffon en cours d'exécution → détection, redémarrage, agent toujours opérationnel).

## Conséquences négatives
- Un délai fixe est moins adaptatif qu'un backoff exponentiel face à une défaillance transitoire qui met du temps à se résorber (dépendance instable, par exemple) — risque de tentatives trop rapprochées en conditions réelles.
- L'état de quarantaine n'étant pas persisté, un greffon durablement cassé repart de zéro (toutes ses tentatives de redémarrage) à chaque redémarrage de l'agent, sans trace consultable au-delà des logs de la session précédente.
- Aucune visibilité en direct (CLI ou API) sur l'état courant du Supervisor pour un agent déjà en cours d'exécution (greffon actif, en cours de redémarrage, ou en quarantaine) — cohérent avec ADR-0015 qui reporte toute API d'état en direct, mais une vraie limite opérationnelle dès que ce sujet deviendra prioritaire.
