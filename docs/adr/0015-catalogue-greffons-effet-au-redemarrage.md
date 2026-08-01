# ADR-0015 — Catalogue de greffons persistant, effet au redémarrage de l'agent

## Statut
Accepté

## Contexte
Câbler les greffons dans l'agent posait une question non tranchée jusqu'ici : comment `patchcord plugin install/list/inspect/uninstall` doit-il se comporter vis-à-vis d'un `patchcord serve` potentiellement déjà en cours d'exécution ? Deux chemins possibles :

1. la CLI agit comme client HTTP d'un agent déjà lancé, via une future API `/v1/plugins` ;
2. la CLI opère directement sur le même catalogue SQLite que l'agent lit à son démarrage, sans communication avec un agent déjà actif.

Le non-négociable #8 (CLI et API utilisent les mêmes services internes) n'impose pas un aller-retour HTTP : `patchcord` est un seul binaire qui peut s'exécuter en `serve` ou en commande ponctuelle, et les deux peuvent appeler la même couche de service en process. Construire dès maintenant une API `/v1/plugins` et un client HTTP pour la CLI aurait anticipé une frontière publique non encore nécessaire à ce stade.

## Décision
Un catalogue persistant (table SQLite `plugins`, migration `0002_plugins.sql`) enregistre chaque greffon installé, avec le manifeste négocié au moment de l'installation (`internal/plugins.Install` lance le binaire, effectue le handshake, puis enregistre l'entrée). Les commandes `patchcord plugin install/list/inspect/uninstall` opèrent directement sur ce catalogue via `internal/plugins`, sans passer par une API HTTP.

Un agent (`patchcord serve`) déjà en cours d'exécution ne recharge pas son ensemble de greffons à chaud : une installation ou une désinstallation ne prend effet qu'au prochain démarrage de l'agent, qui relit le catalogue et relance tous les greffons enregistrés.

## Conséquences positives
- Aucune construction prématurée d'API HTTP (`/v1/plugins`) ni de client HTTP côté CLI : ces éléments arriveront quand un besoin concret les justifiera (probablement avec la supervision complète, section 8.4).
- Les mêmes fonctions de service (`plugins.Install`, `List`, `Get`, `Uninstall`) seront réutilisées sans changement le jour où une API publique les exposera — le non-négociable #8 reste respecté par construction, pas par contournement.
- Modèle mental simple et prévisible : « l'installation prend effet au prochain démarrage », cohérent avec de nombreux gestionnaires de services système.

## Conséquences négatives
- Un agent déjà lancé ne voit pas immédiatement un greffon nouvellement installé ou désinstallé ; il faut redémarrer `patchcord serve` pour que le changement s'applique.
- Si `patchcord serve` et une commande `patchcord plugin ...` s'exécutent en parallèle sur le même `--data-dir`, SQLite (mode WAL) garantit la cohérence des écritures, mais aucune notification ne prévient l'agent en cours d'exécution qu'une modification a eu lieu.
- Cette décision devra être révisée quand le vrai Plugin Supervisor (redémarrage, health checks, quarantaine — section 8.4) et une éventuelle API `/v1/plugins` seront construits, si un rechargement à chaud devient nécessaire.
