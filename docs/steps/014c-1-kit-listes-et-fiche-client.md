# Step 014c-1 — Kit de la charte, listes et fiche client

Step corrective de la 014, première des trois PR de 014c (coupe : arbitrage 383). Origine : l'essai humain a relevé
de nombreux écarts entre les écrans Clients et Comptes livrés en 014 et la maquette d'origine (kit `sms-gateway-design`)
ou l'ancien dépôt (`go-gateway-bo`). Un inventaire les a classés : la plupart sont des règles de la charte ignorées en
silence, alors que l'arbitrage 100 la fait primer pour le visuel. Une **maquette fondée sur la nouvelle spec** a été
faite puis validée par l'humain le 2026-10-23 : `docs/steps/014c/maquette/*.dc.html` (huit écrans). Elle fait foi pour
le visuel de toute la 014c. Ce document porte les arbitrages de toute la 014c. Spec : `docs/spec/08-dashboard.md`
§8.8 ; charte : `.claude/skills/sms-gateway-design/README.md`. Issue : #160 (payée ici). Mode : PR.

Retours humains appliqués à la maquette : ni UUID sur la fiche client, ni ligne d'explication sous les actions (la
raison d'un bouton figé vit dans sa bulle, au survol et au focus), ni ligne « mot de passe masqué » ni colonne
« secret » (un secret qu'on ne peut montrer n'apparaît pas).

**Terminé quand**, dans le navigateur sur contabo169 : les listes Clients, Comptes et Groupes et la fiche client
suivent la maquette (carte, pilules de statut, mono pour les valeurs machine, en-tête avec fil d'Ariane, statut, ligne
de dates et actions, état vide distinct d'« aucun résultat », toast après chaque action) ; aucun contrôle figé ne porte
`title` et sa raison s'ouvre au focus clavier ; « Fermé » remplace « Clos » ; axe sans violation.

## Arbitrages

Fable, 2026-10-23, sauf mention. Les décisions humaines du même jour sont marquées.

383. **Coupe en trois PR verticales** (humain, sur recommandation) : **014c-1** kit, listes (Clients, Comptes, Groupes)
     et fiche client ; **014c-2** fiche compte en onglets (routes filles, en-tête, bascules immédiates et confirmées,
     onglet Sessions) ; **014c-3** identifiants en cartes, modale de rotation, webhooks. Chaque PR apporte ses
     composants avec le premier écran qui les exerce (102).
384. **Composants neufs dans `web/src/ui.tsx`**, sur les jetons existants : `StatusPill({ tone: ok | warn | bad |
     neutral })` (point + libellé) ; `Card({ title, aside?, footer?, flush? })` (`section` + `h2` 15/600) ;
     `KeyValue({ items })` (`dl` en deux colonnes) ; `PageHeader` étendu (388) ; `Tabs` (liens `aria-current`, les
     `.audit-tabs` généralisées en `.ui-tabs`) ; `Switch` sur Base UI (014c-2) ; `Toasts` et `useToast()` sur
     `Toast` de Base UI (397) ; `Explained` extrait de `Button` (387) ; `Pager` avec « N … affichés » et boutons
     toujours rendus, figés et expliqués. Classes seules : `.ui-mono`, `.ui-badge--mo|dlr` (014c-3), `.ui-chip`,
     `.ui-banner--warn` (014c-3), `.ui-why`. Teintes : Actif ok ; **Suspendu warn (ambre)** (humain, sur
     recommandation : une suspension se lève, le rouge reste à l'irréversible ; écart au README de la charte, qui met
     « suspended » en rouge) ; Fermé, Désactivé, Archivé neutral ; Révoqué bad.
385. **YAGNI** : `Tabs` de Base UI (nos onglets sont des routes, un `role="tab"` sur un lien serait faux) ; composants
     `Badge`, `Chip`, `Banner` (une classe suffit) ; `MaskedSecret` ; `IconButton`, `Segmented`, `Tag`, `MetricTile` ;
     bulle d'identifiant technique sous un libellé français (premier état technique en 016) ; squelette par gabarit.
386. **Mono partout où la charte l'exige** : noms de compte, `system_id`, canaux, `query_sm`, `cancel_sm`, types de
     bind, dates, secondes des âges (`toLocaleString('fr')`), URL de webhook, secret ; jamais une phrase.
387. **La bulle** : la raison d'un contrôle figé vit dans `Explained` — déclencheur focalisable (`aria-disabled`, jamais
     `disabled`), `aria-describedby`, `Tooltip` de Base UI au survol et au focus ; `title` banni (il ne s'ouvre jamais
     au clavier). `Select` passe par `Explained` ; un test refuse tout `[title]` rendu. **#160 payée** : son
     déclencheur est le prochain écran qui ajoute une raison ; `blockedReason` devient `ReactNode`, `needs(permission)`
     rend « Cette action demande {libellé} `clé` » (`permissionLabel`), la marque tombe, l'issue se ferme.
388. **En-tête de page** : la barre supérieure porte le fil d'Ariane (`nav aria-label="Fil d'Ariane"`, dernier segment
     en texte) ; le contenu porte `h1` 28/600, la pilule de statut, une ligne muette et les actions à droite.
     `PageHeader({ crumbs, title, status?, meta?, actions? })` ; tous les écrans passent au nouveau contrat (un titre,
     une miette). Un segment n'est un lien que si sa destination est lisible.
389. **Listes** : la table dans une carte, le pager dedans ; **état vide ≠ aucun résultat** : sans filtre, « Aucun
     client » et comment en créer ; avec filtre, « Aucun résultat », « Élargissez les filtres. » et « Retirer les
     filtres ». Clients : Nom, Statut (pilule), Groupe, Créé le (révise l'ordre de 316). Comptes : Nom (mono), Client,
     Statut, Canaux (mono), Créé le (révise 324) ; le filtre client devient une puce « Client : {nom} × ». Groupes :
     Créé le, « Supprimer » en danger, actions espacées. Rail : « Audit » devient « Journal d'audit ».
390. **« Fermé »** (humain) remplace « Clos » pour clients et comptes, filtre « Fermés », messages « … fermé
     définitivement. » ; « Clôturer » reste le verbe ; le statut API `closed` ne change pas.
391. **Fiche client** (révise 312, 315, 338) : en-tête avec Modifier, Suspendre, Réactiver, Clôturer et Supprimer (les
     deux derniers en danger) ; cartes Identité (Groupe, Statut), Âges maximaux, Comptes (« Tous les comptes »,
     « Nouveau compte », colonnes Nom · Statut · Canaux · Créé le) ; « Créé le · modifié le » dans l'en-tête ; plus
     d'UUID ; **aucune ligne « Comptes : N »** (humain, sur recommandation : sans total au contrat, elle serait fausse
     au-delà d'une page) ; toast après chaque action de statut.
392. **Statut effectif** (révise 331) : la pilule montre l'effectif et la mention muette « par son client » quand il
     vient du client ; `effective()` rend `{ status, byCustomer }`.
393. **Fiche compte en onglets par routes filles** (014c-2, révise 329) : `/accounts/$id/{credentials, channels,
     sessions, webhooks}`, l'index redirige selon `credentials:read` ; un onglet sans permission reste rendu, figé et
     expliqué ; « Renommer » dans la ligne d'en-tête, sur tous les onglets.
394. **Canaux en bascules immédiates** (014c-2) : un `PATCH /channels` par bascule, contrôle `readOnly` et
     `aria-busy` pendant l'appel, mise à jour du cache au 200 puis toast, refus en place sinon ; dernier canal figé.
395. **Opérations SMPP en bascules confirmées** (014c-2) : la bascule ouvre une confirmation (binds déconnectés) qui
     envoie le seul drapeau changé.
396. **Onglet Sessions** (014c-2) : limites en formulaire en place ; « Binds vivants » en état « module désactivé »
     jusqu'à 025b (planifié, pas un report).
397. **Toasts** : `Toast` de Base UI dans la coquille, `role="status"`, 5 s, pile de 3 ; un toast après chaque écriture
     réussie sans réponse visible propre ; aucun toast d'erreur ; jamais un secret ni un corps.
398. **Identifiants en cartes** (014c-3, révise 360, 362) : une carte par type, état vide qui crée ; modale de rotation
     de la charte (bandeau, grâce, case « Je comprends… », « Effectuer la rotation » en danger).
399. **Webhooks** (014c-3, révise 370) : badge MO/DLR, URL mono, pilule, Activer/Désactiver, « Modifier l'URL »,
     « Rotation du secret », Supprimer.
400. **Hors 014c, sans issue `debt`** (rien de promis n'est reporté) : total de pagination et compteurs du rail (aucun
     total au contrat), recherche par nom (contrat d'abord), sender IDs et débits (015), sessions vivantes (025a, 026),
     diagnostic d'échec de bind (absent du contrat et de la spec).
401. **Tests** : Vitest et `axe` sur chaque écran ; kit : aucun `[title]`, bulle au focus clavier, pilule par statut,
     pager figé expliqué, vide contre aucun résultat, clé de permission en mono ; mutation à la main ; preuve sur
     contabo169 par PR.
402. **Choix humains** : coupe en trois, ambre pour « Suspendu », pas de ligne « Comptes : N » (ci-dessus). Payer #160
     et « Renommer » sur tous les onglets : Fable.

## Design arrêté

- **`web/src/ui.tsx`** : `Explained` extrait de `Button`, adopté par `Select` (plus de `title`) ; `blockedReason:
  ReactNode` et `needs(permission)` (#160) ; `StatusPill`, `Card`, `KeyValue`, `Tabs` (`.ui-tabs`, Audit migré),
  `Toasts` et `useToast`, `Pager` refait ; `PageHeader({ crumbs, title, status?, meta?, actions? })`.
- **`app.css`** : `.ui-pill`, `.ui-card*`, `.ui-kv`, `.ui-tabs`, `.ui-toast*`, `.ui-chip`, `.ui-mono`, `.ui-why`, en-tête
  de page ; `.row-actions` dans les colonnes Actions.
- **Coquille** : `Toasts` montés ; « Journal d'audit » au rail ; tous les écrans au nouveau `PageHeader`.
- **`customers.tsx`, `accounts.tsx`** : libellés `{ label, tone }`, « Fermé », messages de 390, `effective()` de 392.
- **Listes Clients, Comptes, Groupes** et **fiche client** : 389, 391, 392.
- **Aucun changement de contrat ni de Go.**
- **Tests** : 401 ; mutation consignée ; preuve sur contabo169, captures dans `docs/steps/014c-1/`.

## Écarts au design, constatés à l'implémentation

- **`Card.title` optionnel** : les cartes des listes n'ont pas d'en-tête dans la maquette ; un `h2` « Clients » y
  doublerait le `h1`. Sans titre, la carte est une `section` sans nom (pas une région).
- **`needs(permission, sujet?)`** : un sujet optionnel couvre les raisons qui ne commencent pas par « Cette action »
  (« La création d'un opérateur », « La composition d'un rôle », « La liste des rôles à attribuer », « Le groupe »,
  « La liste des comptes ») ; la phrase finit par un point après la clé. La raison du jeton personnel devient
  « Cette action demande Jeton personnel : se délivrer `gateway:personal_token`. Seul le rôle Propriétaire la détient
  par défaut ; un administrateur peut l'accorder. » ; « Les comptes demandent accounts:read. » de la fiche client passe
  aussi par `needs`.
- **`crumbs: ReactNode[]`** : le fil complet, ancêtres en `Link`, dernier segment en texte, marqué
  `aria-current="page"` par `PageHeader`.
- **`Pager({ count, noun: [singulier, pluriel] })`** : l'accord le demande (« 1 client affiché », « entrées
  affichées »). Les journaux d'audit et la modale des modifications l'adoptent aussi.
- **`useToast()` rend `notify`** directement. Le toast parle par la région polie de Base UI (« Notifications ») ; sa
  racine perd le `role="dialog"` et l'`aria-modal` que Base UI pose (correctif de revue 7).
- **Teinte `bad` et onglet figé de `Tabs`** : retirés, faute de consommateur dans cette PR (102) ; ils reviennent avec
  014c-2 et 014c-3 (correctif de revue 8).
- **`effective()`** (392) : inline dans `EffectiveStatus`, son seul usage, même table de vérité (correctif de revue 9).
- **Groupe non lisible** : texte muet « non lisible », sans bulle ; biome refuse un `span` focalisable, seul déclencheur
  possible d'une bulle sur une valeur (correctif de revue 4).
- **Filtres hors de la sous-barre** : une rangée `.ui-filters` dans le contenu, et l'action principale dans l'en-tête
  (Clients, Groupes, Opérateurs, Rôles), comme la maquette. `Toolbar` perd sa prop `filters`, et les règles mortes
  `.ui-filter` et `.ui-toolbar .ui-select` tombent ; l'avertissement CSS préexistant de biome disparaît avec elles.
- **`.ui-muted`** ajoutée aux classes de 384 pour les valeurs muettes (« Aucun », « (archivé) », dates).
- **Liste Clients** : un groupe archivé porte la mention muette « (archivé) », comme la maquette.
- **Âges maximaux** : le plafond affiché est le `max` de `ageFields` (10 min, 24 h, 72 h), déjà celui du formulaire.
- **Statut effectif** : seul un compte actif d'un client suspendu est « Suspendu, par son client » ; un compte actif
  d'un client fermé n'existe pas (la clôture cascade).
- **Écran Audit** : titre et fil « Journal d'audit », alignés sur le rail.
- **Test du `title`** : `violations()` de `web/src/test.tsx` signale aussi tout `[title]` rendu ; chaque écran qui
  appelle axe est donc couvert.
- **Toasts** : pas de toast pour les écritures des groupes (hors de la liste de 397 retenue pour 014c-1) ; le
  renommage d'un compte en reçoit un, par `AccountForm`.
- **Fiche compte** : seulement adaptée (fil « Comptes › nom », statut effectif en pilule, « fermé ») ; son lien
  « ← client » reste jusqu'à 014c-2.
- **Non fait ici** : la preuve sur contabo169 et ses captures (`docs/steps/014c-1/`), à la main de l'humain avant le
  merge.

## Mutations

À la main, une ligne à la fois, restauration vérifiée par `cmp` ; suite Vitest ciblée ou complète.

| Unité | Tués | Survivants |
|---|---|---|
| `Explained`, `needs`, `Select` (`ui.tsx`) | 7/7 | — |
| `PageHeader`, fil d'Ariane, rail | 6/6 | — |
| `Pager` (`list.tsx`) | 5/5 | — |
| `Toasts`, `useToast` et leurs appels (`ui.tsx`, `customers.tsx`, `accounts.tsx`, fiche client) | 12/12 | — |
| `Tabs` | 3/3 | — |
| `StatusPill`, `Card`, `KeyValue`, racine du toast | 7/7 | — |
| Liste Clients | 9/9 | 2 au premier passage (un filtre seul compte comme filtre) : test ajouté sur chaque filtre |
| Liste Comptes | 9/9 | — |
| `effective`, `channels`, `EffectiveStatus` (`accounts.tsx`) | 6/7 | 1 équivalent |
| Liste Groupes | 6/6 | — |
| Fiche client | 16/16 | 2 au premier passage (mono de la date de création, carte Comptes `flush` à vide) : assertions ajoutées |

**Équivalents nommés** : `customer === 'suspended'` → `customer !== 'active'` dans `effective` (un compte actif sous un
client fermé n'existe pas, la clôture cascade) ; une mutation de motif ambigu a touché la `key` React d'un
`Select.Item`, changement de clé seul, sans effet observable.

## Correctifs de revue, tour 1

1. **Moyenne** : `Explained` démontait le contrôle quand sa raison apparaissait ; après Suspendre au clavier, le focus
   tombait sur `body`. L'arborescence est désormais fixe (`Tooltip.Root disabled`, attributs et raison conditionnels).
   Tests : focus conservé après suspension au clavier ; bouton libre sans `aria-describedby` ni bulle ; bouton en
   chargement inerte.
2. **Basse** : le focus va au `h1` quand le contrôle cliqué disparaît : croix de la puce Client, « Retirer les
   filtres » des trois listes (`titleRef`), suppression d'un client (titre de la liste atteinte).
3. **Basse** : la puce « Client : … » se nomme d'après le premier compte listé avant de retomber sur l'identifiant.
4. **Basse** : sans `customer-groups:read`, le groupe d'un client (liste et fiche) est « non lisible », jamais son UUID.
5. **Basse** : une miette ancêtre n'est un lien que si sa destination est lisible (fiche client, fiche compte).
6. **Basse** : dates à la minute (`atMinute`) dans les listes et la fiche client ; l'Audit garde les secondes.
7. **Basse** : plus de `role="status"` par toast, la région polie de Base UI annonce seule ; `toasted()` lit la région.
8. **Code en trop** : teinte `bad` et onglet figé de `Tabs` retirés.
9. **Code en trop** : `effective()` inline dans `EffectiveStatus`.
10. **Code en trop** : `customerStatus` privé.
11. **Code en trop** : `NoResults({ clear })` partagé dans `list.tsx`.
12. **Code en trop** : `GroupFilter` et `statusOf` partagés dans `customers.tsx` ; la conversion `'' → undefined` du
    filtre, équivalente à la validation de l'adresse (mutant survivant), est retirée.
13. **Code en trop** : table `{ label, tone }` des groupes.
14. **Code en trop** : constante « Fermé définitivement. » dans la fiche client.
15. **Tests** : `stub()` des comptes paramétré par la liste, `row()` partagé dans le test de la liste Clients.
16. à 24. **Tests creux** comblés, chacun vu tuer sa mutation : raison de « Nouveau client » ; raisons des actions des
    groupes ; toast de renommage ; aucun toast d'erreur sur un 503 ; `[title]` cherché dans `document.body` et axe avec
    la modale ouverte ; délai des toasts à 4 999 ms puis 1 ms ; compteurs des journaux (« 1 entrée affichée »,
    « 2 entrées affichées », « 2 modifications affichées ») ; raison de permission avant la fermeture ; assertion
    « Comptes : N » déplacée dans le test qui lit les comptes.

**Mutations du tour** : 47 tuées ; survivants au premier passage comblés par un test (statut `active` lu depuis
l'adresse, pluriel du journal d'accès, lien de la miette quand la fiche lisible échoue) ou retirés (conversion du
filtre de groupe) ; un équivalent nommé : l'ordre `customer.data?.name ?? listed` de la puce, les deux sources donnant
le nom courant du même client.

## Correctifs de revue, tour 2

Une revue des correctifs du tour 1. Aucun défaut introduit. Deux remarques basses, antérieures au tour, laissées : la
racine d'un toast est focalisable sans rôle (règle expérimentale d'axe), et `useGroups().name` rend l'identifiant quand
l'annuaire échoue ou ne contient pas le groupe (au-delà de 500).

## Avis de sécurité en cours de step

`pnpm audit` a signalé GHSA-mrhx-c7xc-m3w5 (`zod` < 4.6.6, sévérité élevée), tiré par l'outillage du routeur. Le
correctif avait 12 jours, sous la quarantaine de 14 jours de `minimumReleaseAge`. Décision humaine : exempter
`zod@4.6.6` seul et le forcer par `overrides` ; dette #179 (retrait le 2026-10-25).

La CI de la PR a ensuite levé GHSA-4qv8-2p9j-2685 et GHSA-f72v-6w45-q472 (`undici` < 8.12.2, modérés), tirés
par `jsdom`, donc par les tests seuls. Correctif publié le 2026-10-15, 8 jours. Même décision humaine :
`undici@8.12.2` exempté et forcé, ajouté à la dette #179 (retrait le 2026-10-29).

## Preuve dans le navigateur

Sur contabo169, `Propriétaire` (compte de preuve), image du dashboard construite depuis la branche, fenêtre
1440 × 960. Captures dans `docs/steps/014c-1/`.

- liste Clients : pastilles « Actif » (vert), « Suspendu » (ambre), « Fermé » (neutre), « 3 clients affichés » ;
  filtres Groupe et Statut portés par l'URL ; croisés sans résultat, « Aucun résultat » et « Retirer les filtres »,
  qui vide l'URL et rend le focus au titre ;
- fiche client : fil d'Ariane, titre et pastille, dates, actions, cartes Identité, Âges maximaux et Comptes ;
  « Réactiver » figé, son explication « Déjà actif. » s'ouvre au focus clavier ; aucun attribut `title` dans la page ;
- « Suspendre » au clavier, confirmé : pastille ambre, toast « Client Arolitec suspendu. », le focus reste sur
  « Suspendre », désormais figé (« Déjà suspendu. ») ; réactivé ensuite, focus sur « Réactiver » ;
- liste Comptes : colonne Client en lien, « Suspendu par son client » ; liste Groupes inchangée dans le kit ;
- axe (WCAG 2.1 AA) sur les quatre écrans : aucune violation.
