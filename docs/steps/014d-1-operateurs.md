# Step 014d-1 — Écran Opérateurs et second facteur pour tous

Step corrective de la 014, première des quatre PR de 014d (coupe : arbitrage 426). Origine : après la 014c, les écrans
Opérateurs, Rôles et Journal d'audit restaient hors de la charte (ni carte, ni pilule, ni toast, ni bulle). Une
**maquette fondée sur la spec** a été faite puis validée par l'humain le 2026-10-24 : `docs/steps/014d/maquette/*.dc.html`
(neuf écrans). Elle fait foi pour le visuel de toute la 014d. Ce document porte les arbitrages de toute la 014d.
Spec : `docs/spec/08-dashboard.md` §8.3, §8.5, §8.8, §8.9 ; charte : `.claude/skills/sms-gateway-design/README.md`.
Issue : #155 (payée ici). Mode : PR.

Retours humains appliqués à la maquette (2026-10-24) :

- **H1** — le second facteur est obligatoire pour **tout** opérateur, quel que soit son rôle (révise §8.3).
- **H2** — la fiche d'un rôle est une modale, jamais une page ; création et fiche partagent la même mise en page : Nom,
  Description, puis les groupes de permissions sur trois colonnes, une case par permission.
- **H3** — les permissions se lisent en français ; la clé technique vit dans une bulle, au survol et au focus.
- **H4** — Opérateurs et Rôles ne sont pas paginés (le contrat rend un tableau) : ni Précédent ni Suivant. Pied
  Opérateurs « N entrées affichées sur T » ; pied Rôles « N rôles ».
- **H5** — le Journal d'audit n'a pas de total : « N entrées affichées », Plus récentes / Plus anciennes.
- **H6** — Clients, Comptes et Groupes affichent un total en pied : « N entrées affichées sur T » (révise 400).
- **H7** — plus de colonne « Lien d'accès » dans la liste Opérateurs ; la fiche d'un rôle ne liste pas ses détenteurs.
- **H8** — la création d'un opérateur ne prend que le nom affiché et l'adresse e-mail ; les rôles s'attribuent ensuite.
- Pas de phrase d'introduction sur l'écran Journal d'audit, ni dans la modale « Nouveau rôle » ; pas d'e-mail dans la
  modale des rôles d'un opérateur ; pas de pointillé sous les libellés de permission.

**Terminé quand**, dans le navigateur sur contabo169 : la liste Opérateurs suit la maquette (carte, pilules Statut et
Second facteur, puces de rôles, filtres Statut et Rôle dans l'adresse, pied « N entrées affichées sur T ») ; la création
et l'attribution des rôles passent par leurs modales et finissent par un toast ; le libellé de l'action de lien suit
l'activation réelle ; une session non élevée, quel que soit son rôle, n'atteint que `/auth/*` ; axe sans violation.

## Arbitrages

Fable, 2026-10-24, sauf mention. Les décisions humaines du même jour sont marquées.

426. **Coupe en quatre PR** (humain, sur recommandation) : **014d-1** Opérateurs et second facteur pour tous (§8.3,
     BFF, #155) ; **014d-2** Rôles (modale fiche et création, ordre et libellés, étiquette « sensible », bulle de clé,
     `stream:read` compris avec le trafic) ; **014d-3** Journal d'audit (pilule Résultat, filtres Du/Au, modale des
     modifications, libellés du pager) ; **014d-4** totaux des listes paginées (contrat Admin, admin-api-svc, `Pager`,
     pieds Clients, Comptes, Groupes). La MFA va avec l'écran dont la pilule « Second facteur » change de sens ; la
     seule PR qui touche le contrat Admin et le Go d'admin-api-svc passe seule et en dernier. Chaque PR apporte ses
     composants avec le premier écran qui les exerce (383).
427. **`total` requis au contrat Admin** sur `CustomerPage`, `CustomerGroupPage`, `AccountPage` (`list-group-customers`
     l'hérite) : `{ type: integer, minimum: 0 }`, nombre de lignes répondant aux filtres, indépendant du curseur.
     Requis : le serveur le connaît toujours, un optionnel ferait afficher « sur ? ». Ni sur les pages de l'audit (H5).
428. **Seconde enveloppe huma, compte séparé** (014d-4) : `countedPageOutput[T]` `{data, next_cursor, total}` à côté de
     `pageOutput[T]`, qui reste à l'audit ; `counted()` appelle `page()` puis le compte. Pas de `count(*) OVER ()` : le
     prédicat keyset est dans le `WHERE`, la fenêtre compterait après le curseur et une page vide n'en porterait aucun.
     Requêtes sqlc `CountCustomers`, `CountGroups`, `CountAccounts`, mêmes filtres sans curseur ni limite, dans la
     transaction de la lecture. Un `count` indexé de plus par liste du plan de contrôle (des milliers de lignes au plus).
429. **Preuve du total** (014d-4) : un test handler par liste (`limit=1` → une ligne, curseur posé, `total` entier ; même
     `total` en page 2 ; filtre de statut → le compte filtré) ; un test lit le contrat servi et exige `total` dans
     `required` des trois schémas (`contract_test` ne compare pas les schémas de réponse).
430. **`Pager({ search, next, go, count, total?, labels? })`** (révise l'écart « `noun` » de 014c-1) : un seul libellé,
     « N entrées affichées » (« 1 entrée affichée »), « sur T » quand `total` est fourni ; `labels` vaut Précédent /
     Suivant, l'audit passe Plus récentes / Plus anciennes. `noun` tombe (014d-3). Les listes sans navigation
     (Opérateurs, Rôles) n'ont pas de composant : le `footer` de `Card` porte un texte muet.
431. **MFA pour tous : `RequiresMFA` supprimée** avec son test. Middleware : `!s.auth[operation] && !current.Elevated`
     → `mfa_required` ; proxy : `!current.Elevated` → `mfa_required`. La MFA est le premier mur, le scope le second :
     une session non élevée sans rôle reçoit `mfa_required`, élevée elle reçoit `forbidden_scope`.
432. **§8.3 réécrit** : « MFA obligatoire pour tout opérateur, quel que soit son rôle. Tant que sa session n'est pas
     élevée, elle n'atteint que `/auth/*` ; ailleurs, proxy compris, `mfa_required`. » **#155 fermée** : sa prémisse,
     un rôle sans MFA, n'existe plus. La redirection de `_shell.index.tsx` reste, sans sa marque : une session non
     élevée dans la coquille est une vérification interrompue. #148 (noc sans délai d'inactivité) n'est pas touchée.
433. **`stream:read` : le BFF complète à l'écriture, la SPA dérive** (014d-2). Dans `generated()` : `traffic:read`
     présent ⇒ `stream:read` ajouté, idempotent ; un refus punirait un client en retard pour une règle que le serveur
     sait appliquer. « Lit le trafic » = `traffic:read` seul (§8.6 ; `cdr:read` est l'archive, `dead-letters:read` une
     file) ; §8.5 le précise. La case Temps réel est figée, cochée si et seulement si Trafic › Lire l'est. Un test
     vérifie que tout rôle par défaut à `traffic:read` porte `stream:read`. Aucun rattrapage : pas de production.
434. **Sélecteur : un `fieldset` par ressource, grille de trois colonnes**, dans l'ordre de la maquette (Clients …
     Jeton personnel), qui est l'ordre des clés de `resources` dans `permissions.ts` ; une ressource hors table va en
     fin, triée. Le regroupement reste « une ressource = un groupe » (202).
435. **Libellés** (014d-2, révise la table `actions`) : verbe par suffixe — `read` Lire, `write` Modifier, `erase`
     Effacer, `delete` Supprimer, `publish` Publier, `reveal` Révéler — et trois exceptions : `billing:topup` (inchangé),
     `cdr:export_bulk` « Exporter en masse », `gateway:personal_token` « Émettre ». Ressources renommées : Numéros
     exacts, Réécriture, Temps réel, Messages en échec, Tâches, Désabonnements, Mots d'opt-out, Numéros des abonnés,
     Données personnelles, Sessions du tableau de bord. Les textes de `needs()` suivent.
436. **Étiquette « sensible » générée** (014d-2) : `tools/openapi` émet `sensitive: readonly Scope[]` depuis
     `scopes.Sensitive`, comme `personalExcluded` ; une classe `.ui-tag--sensitive`, aucun composant (385).
437. **Bulle de clé sur une case** (014d-2, révise 385 pour les permissions) : `title` reste banni (387). Le
     déclencheur est l'option entière, la clé liée à la case par `aria-describedby`. `Hint({ hint, render })` est
     `Explained` sans `aria-disabled`, et `Explained` devient `Hint` plus `aria-disabled`. Vérifier par `ctx7` que le
     `Tooltip` de Base UI s'ouvre sur un focus propagé ; sinon le déclencheur est la case.
438. **Fiche de rôle en modale** (014d-2) : le nom dans la liste est un bouton à l'apparence de lien ; état local, sans
     adresse. Un `RoleForm` sert la création et la fiche : Nom (création seule), Description, grille 434 ; en-tête nom
     et puce « Par défaut | Personnalisé » ; pied Supprimer à gauche, Annuler, Enregistrer ou « Créer le rôle ».
     « Voir les permissions », « Modifier », `View` et `.role-keys` tombent.
439. **Lecture seule** (014d-2) : un rôle par défaut s'ouvre dans la même modale, description en texte, cases figées,
     une ligne « Les rôles par défaut ne se modifient pas : chaque déploiement les réécrit. », pied « Fermer » seul. Un
     rôle personnalisé sans `roles:write` a ses cases figées, Enregistrer et Supprimer figés et expliqués (`needs`).
440. **Supprimer** dans la fiche et dans la liste, même confirmation ; la fiche se ferme au succès, focus sur le `h1`.
     Figé et expliqué : rôle par défaut, sans `roles:write`, ou détenu (« Détenu par a, b : il se retire d'abord à ces
     opérateurs. »). Liste : Nom, Description, Type, Permissions, Détenteurs, Supprimer ; pied « N rôles ».
441. **Pilules Opérateurs** (humain, sur recommandation) : Statut `disabled` → Désactivé neutral ; sinon lien d'accès
     en échec → **« Envoi en échec » warn** ; sinon Actif ok. Écart à la maquette : un courriel non parti est le seul
     état du lien qui demande un geste, et sans sa colonne (H7) il doit vivre quelque part ; `queued` et `sent` ne
     s'affichent plus. Second facteur : Enrôlé ok, Aucun neutral.
442. **`Operator.activated` au contrat BFF** (humain, sur recommandation) : booléen requis, vrai dès que le mot de passe
     est posé. La maquette déduit le libellé du second facteur, ce qui ment pour l'opérateur qui a posé son mot de passe
     sans enrôler de facteur — précisément celui que H1 bloque à `/enroll`. `!activated` → « Renvoyer l'activation »
     (72 h) ; `activated` → « Réinitialiser l'accès » (1 h) ; figé et expliqué si désactivé. Contrat d'abord.
443. **Filtres Statut et Rôle dans l'adresse** (`validateSearch`) : Statut `active | disabled`, Rôle par `id` ; les
     options de rôle viennent des rôles des opérateurs listés (aucune permission de plus). Sans filtre et vide :
     « Aucun opérateur » ; avec filtre : aucun résultat (389). Pas d'option « Sans rôle ».
444. **Toasts et création** : `.page__note` tombe pour `useToast` (397). « Opérateur {nom} créé. Le lien d'activation
     part à {email} ; il vaut 72 heures. », « Rôles de {nom} enregistrés. », « {nom} désactivé. » ou « réactivé. »,
     « Lien d'activation renvoyé à {email}. » ou « Lien de réinitialisation envoyé à {email}. ». Création : Nom
     affiché, Adresse e-mail (mono), aide « Le lien d'activation part à cette adresse. », ligne « L'opérateur naît sans
     rôle : attribuez-les ensuite par « Rôles » dans la liste. La création est enregistrée dans le journal d'audit. »,
     bouton « Créer et inviter ». Modale Rôles : une case par rôle avec sa description, puces `.ui-chip` dans la liste.
445. **Pilule Résultat** (014d-3) : `outcome` présent → `granted` ok « lecture accordée », `unreadable` warn
     « illisible », `not_found` warn « introuvable » ; sinon `< 400` ok « code », `< 500` warn « code refusé », sinon
     bad « code erreur ». Accès : Réussi ok, Échec warn. Colonne Détail : « Modifications » ou « aucune » muet.
446. **Filtres de l'audit** (014d-3) : Du, Au, Opérateur, sans bouton « Filtrer » : chaque changement navigue ;
     « Retirer les filtres » revient au défaut, figé et expliqué « Aucun filtre actif. » quand on y est.
447. **Modale des modifications** (014d-3) : titre « Modifications de `operation_id` » ; Date, Opérateur (nom),
     Requête (mono). Par changement : puce Création | Modification | Suppression, table et `pk` en mono ; table Champ /
     Avant / Après des seuls champs changés, `null` → « vide » muet ; un `details` « N champs inchangés » qui les liste ;
     ligne finale « Les secrets et leurs empreintes ne sont jamais journalisés : seuls les champs lisibles
     apparaissent. » (vraie : `audit_capture()` omet scellés, empreintes et références de clé).
448. **YAGNI** : lien profond sur un rôle ; option « Sans rôle » ; composants `Tag`, `Badge`, `Chip` ; miroir TS de
     `Sensitive` écrit à la main ; rattrapage de `stream:read` ; garde MFA par route de la coquille ; `count(*) OVER ()` ;
     `total` optionnel.
449. **Tests** — Go : 429, 431, 433 ; `tools/openapi` émet `sensitive` ; `Operator.activated` faux avant activation,
     vrai après. SPA (Vitest et axe par écran, aucun `[title]`, 401) : pilules (dont l'échec d'envoi) ; libellé du lien
     selon `activated` ; filtres dans l'adresse, vide contre aucun résultat ; textes des toasts ; ordre des groupes ;
     case Temps réel dérivée ; bulle de clé au focus ; étiquette sensible ; lecture seule ; table de la pilule Résultat ;
     champs changés contre repliés ; pager « sur T ». Mutation à la main, consignée.
450. **Risques et preuve** : dès 014d-1, un compte de contabo169 sans facteur est arrêté à `/enroll` (voulu ; vérifier
     que le propriétaire de bootstrap a le sien avant de déployer) ; `api.gen.ts` et `admin.gen.ts` régénérés.
     `docs/plan/composants.md` reçoit 014c et 014d. Preuve sur contabo169 par PR, captures dans `docs/steps/014d-N/`.
     Aucune issue `debt` attendue.
451. **Choix humains** (2026-10-24, sur recommandation) : quatre PR ; « Envoi en échec » gardé ; `Operator.activated`
     au contrat ; pas de ligne « Comptes : N » sur la fiche client (sa carte n'est pas paginée, elle montre tout).

## Design arrêté

- **Contrat BFF** : `Operator.activated: boolean`, requis (442) ; `api.gen.ts` et `bff.gen.go` régénérés.
- **BFF** : `RequiresMFA` et son test supprimés ; middleware et proxy refusent toute session non élevée hors `/auth/*`
  (431) ; `activated` lu de l'état du compte (mot de passe posé) et rendu par `GET /operators`.
- **Spec** : §8.3 réécrit (432) ; `docs/plan/composants.md` complété pour 014c et 014d.
- **`_shell.operators.tsx`** : `PageHeader` et « Nouvel opérateur » ; filtres Statut et Rôle en `validateSearch` (443) ;
  table dans une `Card` (Opérateur : nom puis e-mail mono ; Statut ; Rôles en puces ; Second facteur ; Actions :
  Rôles, action de lien, Désactiver ou Réactiver), pied « N entrées affichées sur T » ; pilules 441 ; libellé 442 ;
  modales Nouvel opérateur et Rôles de {nom} (444) ; toasts 444 ; `useClosedOn` à la place de l'effet écrit à la main.
- **`_shell.index.tsx`** : la marque de dette de #155 tombe, la redirection reste (432).
- **Tests** : 449 pour la part 014d-1 ; mutation consignée ; preuve sur contabo169, captures dans `docs/steps/014d-1/`.

## Écarts au design, constatés à l'implémentation

- **`.ui-tags` et `.ui-tag`** : les rôles d'un opérateur sont des étiquettes neutres, pas la puce turquoise `.ui-chip`
  d'un filtre qu'on retire (vu à la preuve).
- **Aide de l'adresse e-mail** : liée au champ par `aria-describedby` avec l'erreur de saisie, qui s'y ajoute ; le
  lecteur d'écran annonce les deux. `explained` (`web/src/test.tsx`) lit désormais tous les identifiants décrits.
- **Ordre des champs** : Nom affiché, puis Adresse e-mail, comme la maquette ; le focus d'un formulaire vide va au nom.
- **Titres de confirmation du lien** : « Envoyer un lien d'activation / de réinitialisation à {nom} ? », bouton
  « Envoyer » ; seule la branche `unknown` tombe.
- **Correctif de revue 1** : le sous-titre de la maquette passe par `meta` de `PageHeader`.
- **Correctif de revue 2** : un filtre Rôle que plus aucun opérateur listé ne détient s'offre sous « Rôle sans
  détenteur », jamais par son identifiant brut.
- **Correctif de revue 3** : tests durcis — un compte désactivé au lien en échec reste « Désactivé » ; « Aucun
  résultat » prouvé filtre par filtre ; options de rôle triées et dédoublonnées.
- **Correctif de revue 4** : l'échec de lecture de la liste passe par `Failure` (`list.tsx`), comme Clients et Comptes.
- **Correctif de revue 5** : le test Go de la MFA ne garde que les cas qui comptent, un rôle à lectures ordinaires et
  aucun rôle ; les raisons de l'ancienne règle tombent.
- **Correctif de revue 6** : les textes du lien (action, toast, titre, effet) viennent d'un seul objet selon `activated`.
- **Correctif de preuve** : dans « Rôles de {nom} », chaque rôle est une option encadrée, comme la maquette
  (`.role-choice--boxed`).

## Preuve sur contabo169

2026-10-24, image construite depuis la branche, déploiement automatique suspendu le temps de la preuve. Compte de
preuve `ada.preuve` (Propriétaire) : lien de réinitialisation, nouveau mot de passe, second facteur enrôlé, session
élevée ; identifiants gardés dans le scratchpad puis effacés.

- Liste : carte, pilules, étiquettes de rôles, sous-titre, pied « 4 entrées affichées sur 4 » ; axe sans violation,
  aucun `[title]` (`01-operateurs.png`).
- Filtre `?status=disabled` : « 1 entrée affichée sur 4 » (`02-filtre-desactives.png`).
- Création d'« Aïcha Bamba » dans sa modale (`03-nouvel-operateur.png`), toast « Opérateur Aïcha Bamba créé. Le lien
  d'activation part à aicha.bamba@example.test ; il vaut 72 heures. », action « Renvoyer l'activation »
  (`04-toast-creation.png`).
- Rôles de l'opérateur en options encadrées, axe sans violation (`05-roles-operateur.png`) ; Support attribué, toast
  « Rôles de Aïcha Bamba enregistrés. ».
- Second facteur pour tous : Aïcha, rôle Support seul (lectures ordinaires), activée puis connectée sans facteur :
  `GET /api/gw/admin/accounts` → 403 `mfa_required`, `GET /api/auth/me` → 200. Dans la liste, elle est « Actif »,
  second facteur « Aucun », et son action devient « Réinitialiser l'accès » : le cas où l'heuristique du facteur
  aurait menti (442) (`06-active-sans-facteur.png`).
