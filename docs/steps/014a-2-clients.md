# Step 014a-2 — Clients

Plan : `docs/plan/composants.md` §014 à 020, deuxième des trois PR de 014a (coupe : arbitrage 282). Arbitrages antérieurs :
1 à 310 (`docs/steps/014a-1-schema-et-groupes.md` porte ceux de toute la 014a). Spec : `docs/spec/01-exigences.md` §1.1,
§1.2 ; `docs/spec/03-pipeline-mt.md` §3.6 ; `docs/spec/07-plan-de-controle.md` §7.3 à §7.5 ; `docs/spec/08-dashboard.md`
§8.8 ; ADR-0006. Contrat : `contracts/openapi-admin.yaml` (tag `customers`). Issue : #173. Règles : `.claude/rules/tests.md`,
`go-code.md`, `contracts.md`. Mode : PR.

## Contexte

- 014a-1 (#172) a livré les trois tables, les opérations des groupes, le PATCH à trois états (`value[T]`, `nullable[T]`),
  la pagination partagée, la traduction des SQLSTATE réduite aux groupes, l'écran Groupes, `Select` et
  `Toolbar.filters`, et le client `gw` qui pose `application/json` sur toute écriture.
- La table `customers` existe ; aucune opération ne l'écrit.

**Terminé quand**, contre un vrai Postgres puis dans le navigateur : `Propriétaire` crée un client dans un groupe, le
filtre par groupe et par statut, le modifie (âges maximaux compris), le suspend après l'aperçu d'impact (ses comptes non
clos suspendus), le réactive (ses comptes restent suspendus), le clôt (ses comptes clos) ; un client avec comptes refuse
la suppression (409) ; chaque mutation est auditée sous son acteur, cascade comprise.

## Arbitrages

Fable, 2026-10-23, sauf mention.

311. **Filtre par groupe : `Select` « Groupe : Tous »** (révise la forme de 305 après 310). Annuaire `list-customer-groups?
     limit=500`, sans filtre de statut, une page, trié par nom, un archivé suffixé « (archivé) » ; il nourrit aussi la
     colonne « Groupe » et le sélecteur du formulaire (actifs, plus celui déjà posé, 295). Plafond : au 501ᵉ groupe, le
     filtre passe au `Combobox`. URL : `group_id`, `status`, `cursor`, `trail`.
312. **Fiche client : route `/customers/$id`** (304 : elle portera les comptes en 014a-3, et la fiche compte y remontera).
     Lien « ← Clients » au-dessus du titre (nom du client) ; bloc « Identité » (nom, groupe lié au filtre de la liste,
     trois âges ou « plateforme », créé le, modifié le) ; « Modifier » ouvre le formulaire de création en modale. La liste
     n'a pas d'action par ligne : le nom mène à la fiche, qui porte tout. « Nouveau client » reste sur la liste.
313. **`max_age_override`** : l'objet, quand il est envoyé, remplace les trois valeurs (contrat : objet facultatif au PATCH,
     trois champs requis nullables) ; `null` sur l'objet est refusé (`value[maxAge]`). Création : absent → trois `NULL`.
     Formulaire : trois champs numériques en secondes, vide = `null`, `placeholder` au plafond de la plateforme (600,
     86400, 259200), une phrase d'aide ; l'objet part entier dès qu'une valeur diffère.
314. **Messages français fixes par appel et statut** (la passerelle écrit en anglais) : suppression 409 → « Ce client a
     des comptes : suspendez-le ou clôturez-le. » ; suspension 409 et réactivation ou clôture 422 → « Ce client est clos
     définitivement. » ; formulaire 422 → « Ce groupe n'existe plus. ». Tout autre refus garde le message de la passerelle.
315. **Actions de statut**, toujours rendues (spec 08 §8.8) : « Suspendre » (`suspend-customer`, confirmation qui charge
     l'aperçu : « Calcul de l'impact… », « Impact indisponible. » sans bloquer, sinon les comptes à suspendre et les clos) ;
     « Réactiver » (`update-customer {status: active}`, « Ses comptes restent suspendus ; réactivez-les un à un. ») ;
     « Clôturer » (`update-customer {status: closed}`, danger, définitif). Figés et expliqués : « Déjà suspendu. »,
     « Déjà actif. », « Clos définitivement. » (les trois et le formulaire). « Supprimer » reste ouvert à un clos (296).
316. **Colonnes** : Nom (lien), Groupe, Statut, Créé le ; aucun tri (curseur `(created_at, id)` décroissant). La colonne
     « Clients » de Groupes lie vers `/customers?group_id=` (spec 01 §1.2 : filtre partout où un client apparaît).
317. **Dette #173** (humain : `debt` et `go-live`) : suspension et sortie d'`active` ne déconnectent pas les binds ; une
     seule marque, au-dessus de `BindsToDisconnect` dans le constructeur de l'aperçu du cœur.
318. **Clos définitif, 409 ou 422** : `refused()` traduit `check_violation` en 422 sur le champ de la contrainte
     (`closed_is_final` → `status`, `customers_max_age_*_check` → `max_age_override.*`) ; seule la suspension relève ce
     cas en 409. `23503` à l'écriture → 422 sur `group_id` ; `23001` à la suppression → 409.
319. **Cascade** dans la transaction : `UPDATE accounts SET status = @status WHERE customer_id = @id AND status <>
     'closed'` après celui du client, pour `suspended` et `closed` (292, 293) ; suspendre un suspendu rend 200. L'aperçu lit
     `(id, name, status)` des comptes ; le cœur les sépare.
320. **`binds_to_disconnect` sérialisé `[]`**, jamais `null` ; un test le pince.
321. **Groupe archivé accepté par l'API** (295) ; l'écran seul restreint.
322. **Rail** : « Clients » (`customers:read`) avant « Groupes ».
323. **Trois états** : `name`, `status` en `value[string]`, `group_id` en `nullable[string]`, `max_age_override` en
     `value[maxAge]` ; `{}` rend 200 sans changement.

## Design arrêté

- **`internal/adminapi`** : `CustomerPatch`, filtre de liste, `SuspensionImpact` et son constructeur (marque #173) ; le
  port `Customers` (liste, lecture, création, mise à jour avec cascade, suppression, comptes d'un client) dans `Tx`.
- **`internal/adminapi/postgres`** : requêtes sqlc des clients ; traduction de 318.
- **`internal/adminapi/huma`** : les sept opérations `customers`.
- **SPA** : `_shell.customers.index.tsx` (liste, filtres, création), `_shell.customers.$id.tsx` (fiche, actions,
  modification) ; rail ; lien de la colonne « Clients » de Groupes.
- **Tests** : Postgres réel sous `admin_api_svc` — chaque opération, filtres et curseur, trois états, cascade de la
  suspension et de la clôture (comptes insérés par le test, clos épargnés), réactivation sans cascade, clos définitif en 409
  et 422, groupe inconnu 422, suppression 409 puis 204, aperçu (`[]` pour les binds), diff et acteur de la cascade ;
  Vitest des deux écrans avec `axe`. Mutation manuelle, consignée.

## Écarts au design, constatés à l'implémentation

- **Marque #173 dans l'adaptateur huma** (`internal/adminapi/huma/customers.go`), sur la ligne qui pose
  `binds_to_disconnect` ; le cœur rend deux listes de comptes sans type `Session`, qu'aucune opération ne peuple avant
  025a.
- **`list-group-customers` passe par `list-customers` filtrée** (`CustomersPage`) : la requête `GroupCustomers` de 014a-1
  disparaît.
- **Cascade restreinte aux comptes qui changent** (`status NOT IN ('closed', @status)`) : suspendre un client ne réécrit
  pas un compte déjà suspendu, l'audit n'a pas de diff vide.
- **`Confirmation.refusals`** : une table statut → message français, pour les refus de 314.
- **`Select.blockedReason`** (correctif de revue) : sans `customer-groups:read`, le filtre et le sélecteur de groupe sont
  désactivés et expliqués (spec 08 §8.8), l'annuaire n'est pas lu, la colonne montre l'identifiant brut.
- **TanStack Router garde les clés d'adresse non validées** : retirer `group_id` de `validateSearch` ne change pas la
  requête ; la ligne garde le typage et la normalisation de `''`.

## Mutations

À la main, par le script de copie, mutation et restauration (`cmp`). Cœur 6/6 ; adaptateur 7/7 ; SQL généré 10/10 (un
onzième, identique à l'original, était une erreur de liste) ; couche huma 6/6, puis 1/1 (âges nuls à la création) ;
module partagé de la SPA 12/12 ; fiche 11/11 ; liste 4/5, le survivant équivalent ci-dessus ; `Confirmation.refusals`
1/1. Après la revue : les huit survivants de la revue des tests tués par des tests ajoutés, `Select.blockedReason` 2/2.

## Correctifs de revue, tour 1

Trois revues (correction et sécurité ; tests creux ; code en trop). Aucun bloquant.

1. **`max_age_override: null` accepté à la création** (basse) : refusé, 422, comme au PATCH (`value[maxAge]`).
2. **Groupe vidé en silence sans `customer-groups:read`** (basse, rôle personnalisé seulement) : bloqué et expliqué.
3. **Tests ajoutés** : lien de la colonne « Clients » de Groupes, curseur de la page suivante, 422 de la réactivation et de
   la clôture, 422 du formulaire en modification, groupe archivé offert à son client, aperçu sans compte, réactivation
   bloquée sans `customers:write`.
4. **Code en trop retiré** : les trois entrées `customers_max_age_*_check` de la traduction (huma borne avant Postgres),
   une classe CSS sans règle, la clé et le type de groupe dupliqués, un `return` inutile, une conversion écrite à la main,
   une boucle de conversion dupliquée, un commentaire qui répétait 311.
5. **Gardé** : le squelette « lisible ? barre et liste : refus » des trois écrans reste écrit trois fois ; un composant
   commun prendrait plus de paramètres qu'il n'économise de lignes.

## Correctifs de revue, tour 2

Une revue des correctifs du tour 1. Aucun bloquant.

1. **`Select` bloqué inatteignable au clavier** (basse à moyenne) : `disabled` le sortait de la tabulation. Il reste
   focalisable, `aria-disabled`, décrit par sa raison, et son menu est tenu fermé, comme `Button.blockedReason`.
2. **Filtre actif invisible sans `customer-groups:read`** (basse) : le déclencheur montre la valeur brute (l'identifiant du
   groupe) quand aucun libellé ne la nomme ; même chose dans le formulaire.

## Preuve dans le navigateur

Sur contabo169, `Propriétaire` (compte de preuve, mot de passe et TOTP).

1. **Aperçu de suspension** (constaté par la preuve) : un compte déjà suspendu figurait parmi « N comptes seront
   suspendus », alors que la cascade ne le touche pas. `accounts_to_suspend` ne retient plus que les comptes actifs ; un
   compte suspendu n'apparaît dans aucune des deux listes.

Joué ensuite, captures dans `docs/steps/014a-2/` :

- création de « Banque Exemple » dans le groupe Banque (OTP 300 s) et de « Commerce Exemple » sans groupe ; le filtre
  « Groupe : Banque » ne garde que le premier, l'adresse porte `group_id` ;
- modification : OTP vidé (« Plafond de la plateforme »), transactionnel 7200 s ;
- trois comptes insérés en base (actif, suspendu, clos ; 014a-3 n'a pas encore d'écran) ; l'aperçu annonce « 1 compte sera
  suspendu : compte-actif. 1 compte clos le reste : compte-clos. » ; la suspension suspend le seul compte actif ;
- réactivation : le client redevient actif, ses comptes restent suspendus ; « Statut : Suspendus » vide, « Statut :
  Actifs » montre les deux clients ;
- suppression refusée : « Ce client a des comptes : suspendez-le ou clôturez-le. » (409 dans l'audit) ;
- clôture : les deux comptes non clos passent clos ; Modifier, Suspendre, Réactiver et Clôturer figés, Supprimer ouvert ;
- audit : sept opérations sous l'opérateur, douze lignes de diff, la cascade sous le même acteur que l'opération ;
- axe (WCAG 2.1 AA) sur la liste, la fiche et une confirmation : aucune violation ; un « à revoir » `aria-hidden-focus`
  dans la modale, porté par les sentinelles de focus de Base UI.
