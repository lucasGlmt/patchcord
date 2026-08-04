# ADR-0046 — Registre officiel « patchcord » (fallback intégré) et confiance restreinte au namespace `io.patchcord.*`

## Statut
Accepté

## Contexte

L'ADR-0044 a livré `internal/registry` (résolution d'un id de package contre
des registres configurés par l'utilisateur — répertoire local ou URL
http(s) servant un `index.json` statique) et a explicitement laissé « hors
scope, différé, pas oublié » deux points : (1) `plugin install`/`app
install` ne résolvent pas encore par identifiant, seul `bundle install` le
fait ; (2) aucune distribution des clés publiques de confiance, `trust add`
suppose un canal hors bande.

Lucas souhaite une expérience proche de `pub.dev` (Flutter) ou npmjs :
`patchcord plugin install text` doit fonctionner sans que l'utilisateur
clone le dépôt, build localement, ou configure quoi que ce soit à la main.
Trois décisions ont été prises avec lui pour fermer ce sujet sans
contredire les non-négociables de CLAUDE.md §1 (en particulier §1.9, le
cloud n'est jamais requis) ni le modèle de confiance de l'ADR-0043
(aucune confiance implicite, aucun trust-on-first-install) :

1. Hébergement de l'index et des packages officiels : un dépôt GitHub
   public statique dédié — pas de service applicatif à opérer.
2. « Par défaut » signifie un **fallback intégré au binaire**, jamais une
   ligne écrite en base au premier démarrage — aucun appel réseau tant
   qu'aucune commande `install`/`update` n'est exécutée.
3. La clé de signature officielle est pré-approuvée dans le binaire, mais
   **restreinte** au namespace réservé `io.patchcord.*` — pas un
   blanc-seing pour n'importe quel id.

Le document de vision (§ « Revenus possibles / Cloud facultatif ») nomme
déjà un « registre officiel » comme offre facultative, distincte de la
« marketplace » (payante, explicitement hors scope actuel, section 1 des
non-objectifs). Cette décision ne construit pas de marketplace : le
registre reste un index statique gratuit, sans commerce.

## Décision

**Fermeture du point 1 de l'ADR-0044.** `plugin install <ref>` et `app
install <ref>` acceptent désormais, en plus d'un chemin de fichier
existant, une référence `id` ou `id@version` résolue via
`internal/registry`, exactement le même flux que `bundle install`
(`registry.ParseRef` → `registry.Resolve` → `registry.Fetch` → staging
temporaire → `InstallPackage` inchangé). Aucune nouvelle logique de
vérification : `packaging.Verify`/`trust.CheckPolicy` s'appliquent au
fichier obtenu, qu'il vienne d'un chemin local ou d'un registre.

**Registre « patchcord » — fallback intégré, pas une ligne en base.** Le
nom `patchcord` est réservé. `registry.Resolve` consulte les registres
configurés par l'utilisateur dans leur ordre d'ajout (comportement
inchangé de l'ADR-0044) puis, uniquement si aucun d'eux ne connaît l'id
demandé, consulte en dernier recours une location constante compilée dans
le binaire (URL du dépôt GitHub statique). Ce fallback :

- n'est jamais listé comme une ligne de la table `registries` tant qu'il
  n'a pas été explicitement reconfiguré ;
- ne déclenche aucun accès réseau avant une résolution réelle (`install`/
  `update`) — `patchcord serve`, `registry list`, etc. restent inertes ;
- `registry add patchcord <url>` le remplace normalement, comme configurer
  n'importe quel registre par ce nom — l'utilisateur peut pointer vers son
  propre miroir ;
- `registry remove patchcord`, appelé alors qu'aucune ligne `patchcord`
  n'a été ajoutée, insère une ligne tombstone (location vide) plutôt que de
  renvoyer `ErrNotFound` : c'est le seul moyen de désactiver durablement un
  fallback qui n'existe pas en base. `Resolve` traite une location vide
  comme « registre désactivé », l'ignore silencieusement sans erreur, et ne
  retombe pas sur la constante compilée. `registry add patchcord <url>`
  efface cette tombstone.
- `registry list` affiche ce fallback (nom `patchcord`, location = la
  constante) uniquement quand il est actif, pour que l'utilisateur voie
  toujours contre quoi une résolution serait tentée — jamais un
  comportement invisible.

**Confiance restreinte au préfixe `io.patchcord.*`.** Une clé publique
Ed25519 « officielle Patchcord » est embarquée comme constante dans le
binaire (la clé privée correspondante est générée hors du dépôt, via
`patchcord key generate`, et n'est jamais commit — process identique à
toute autre clé de signature de l'ADR-0043). Le trust store la considère
pré-approuvée **uniquement** pour les ids commençant par `io.patchcord.` —
la condition de confiance porte sur la paire (préfixe, clé), pas sur la
clé seule : un package hors de ce préfixe signé par cette même clé n'est
pas automatiquement approuvé, pour éviter qu'une clé « officielle » ne
devienne une confiance générale. Cette pré-approbation reste une entrée
comme une autre du point de vue de l'utilisateur : `trust list` la montre,
`trust remove io.patchcord.<x> <clé>` la révoque id par id, exactement le
modèle déjà existant — aucune nouvelle catégorie de confiance « spéciale »
dans le schéma.

**Namespace réservé `io.patchcord.*`** (déjà anticipé par le document de
vision §9.1) : réservé aux packages publiés par ou avec l'accord de
l'équipe Patchcord. Convention documentée, non policée techniquement au
niveau du registre lui-même — un registre tiers reste libre de son propre
préfixe, cohérent avec l'absence de PKI/autorité centrale (ADR-0043). Seule
la pré-approbation de confiance ci-dessus applique une restriction
technique sur ce préfixe.

## Explicitement hors scope (différé, pas oublié)

- Le contenu réel publié dans le dépôt de registre au-delà du greffon de
  référence (`io.patchcord.text`) — les sept autres exemples de
  `plugins/examples/` restent à packager/signer/publier au fur et à mesure,
  pas dans ce même chantier.
- Hébergement au-delà d'un dépôt GitHub statique (CDN dédié, domaine
  `registry.patchcord.dev`) — reviendra si le volume ou la latence le
  justifient.
- Distribution de clés de confiance *pour des registres tiers* — seule la
  clé officielle Patchcord est traitée ici.
- Rotation de la clé officielle — nécessitera une nouvelle release
  embarquant une nouvelle constante ; pas de mécanisme de rotation à
  distance.
- `plugin update`/`app update` par identifiant — seul `bundle update`
  existe (ADR-0044) ; l'extension aux deux autres formats suit la même
  mécanique mais n'est pas incluse ici.
- Semver, installation automatique des dépendances greffons manquantes —
  déjà différés par les ADR précédents de la phase.

## Conséquences positives

- `patchcord plugin install text` (ou `io.patchcord.text@1`) fonctionne
  sans clonage ni build local, en réutilisant entièrement les mécanismes
  déjà éprouvés de l'ADR-0044 pour `bundle install`.
- Le cœur de la promesse « cloud facultatif » (CLAUDE.md §1.9) reste
  respecté : rien n'appelle le réseau avant une commande d'installation
  explicite, et le fallback reste désactivable et remplaçable comme tout
  autre registre.
- La confiance implicite est strictement bornée : compromettre ou
  détourner un id hors `io.patchcord.*` ne bénéficie d'aucune pré-approbation,
  même avec la clé officielle.
- Pose une convention de namespace explicite (`io.patchcord.*`) que la
  documentation utilisateur peut désormais citer sans ambiguïté.

## Conséquences négatives

- La clé publique officielle et l'URL du registre fallback sont toutes
  deux figées dans le binaire à la compilation : les faire évoluer (rotation
  de clé, changement d'hébergeur) exige une nouvelle version publiée du
  binaire, pas une simple mise à jour de configuration côté serveur.
- Le mécanisme de tombstone pour `registry remove patchcord` introduit un
  cas particulier (location vide = désactivé) que `Resolve`/`List` doivent
  traiter explicitement — une petite dette de complexité par rapport au
  modèle purement « présence en base » de l'ADR-0044.
- Le dépôt GitHub statique reste un point de défaillance unique pour le
  fallback par défaut (mêmes limites que tout registre http(s) déjà
  documentées par l'ADR-0044 : `index.json` non signé, pas de cache) — un
  utilisateur qui veut une garantie plus forte doit pointer vers son propre
  miroir.
