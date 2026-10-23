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
