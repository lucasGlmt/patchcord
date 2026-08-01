# ADR-0007 — Le cloud n'est jamais requis

## Statut
Accepté

## Contexte
Un modèle économique viable pour Patchcord pourrait s'appuyer sur des services cloud (registre de greffons, synchronisation, webhooks relayés, IA managée, etc.). Il serait commercialement tentant de rendre certaines de ces briques obligatoires dès le démarrage de l'agent, par exemple via une validation de licence distante ou un compte obligatoire.

## Décision
Aucune fonctionnalité du core ne doit exiger un compte distant, une licence distante ou une validation auprès d'une plateforme propriétaire pour démarrer ou fonctionner. Le cloud, quand il existera, restera une couche strictement additive et optionnelle (registre, synchronisation, webhooks relayés, sauvegarde, monitoring, IA managée).

## Conséquences positives
- Patchcord reste pleinement utilisable local-first, sans dépendance réseau ni service tiers, ce qui répond directement au besoin des entreprises qui veulent garder leurs données sur leur propre infrastructure.
- Aucune fonctionnalité cœur ne peut être rendue indisponible par la panne ou la disparition d'un service cloud externe.
- La confiance des intégrateurs et entreprises est renforcée : l'agent ne peut pas devenir inutilisable si un serveur de licence tombe.

## Conséquences négatives
- Le modèle de revenu ne peut pas reposer sur un gating obligatoire du runtime ; il doit se construire sur des services optionnels, du support, ou du marketplace — ce qui est structurellement plus lent à monétiser.
- Toute fonctionnalité future "pratique à livrer via le cloud" (ex. registre de greffons) doit être conçue avec un chemin local de secours dès sa conception.
