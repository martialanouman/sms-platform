# Step 014e — Retours de l'essai humain sur les identifiants et les statuts

Step corrective de la 014, après l'essai humain du 2026-10-24 sur contabo169. Trois captures annotées ; la conduite a
été validée par l'humain le même jour. Mode : PR.

**Terminé quand**, dans le navigateur sur contabo169 : l'interface dit « System ID » partout où elle disait
`system_id` ; la modale « Nouveau bind SMPP » propose un identifiant par « Générer » ; la fiche client et l'en-tête
d'un compte n'offrent qu'une action de statut à la fois ; les pieds des deux cartes d'identifiants ont la même
hauteur, boutons centrés ; axe sans violation.

## Arbitrages

Humain, 2026-10-24, sur recommandation.

452. **« System ID » partout dans l'interface** : libellés de champ et de clé-valeur, titres, messages et refus
     (« Ce System ID est déjà pris. », « Le System ID … reste réservé à ce compte. ») ; la valeur reste en mono. Le nom
     technique `system_id` ne vit plus que dans le contrat et le code.
453. **« Générer » propose un System ID** dans la modale « Nouveau bind SMPP » : le nom du compte en minuscules, réduit
     à `[a-z0-9]`, coupé à 10 caractères, puis `_` et 4 chiffres au hasard (15 caractères au plus, la borne du
     contrat ; exemple `bicici_1123`). Le champ reste éditable ; un doublon est refusé par le serveur (409), comme
     aujourd'hui. Un nom sans caractère retenu donne `bind` comme racine.
454. **Une seule action de statut** sur la fiche client et l'en-tête d'un compte (révise 391 et l'en-tête de 014c-2) :
     actif → « Suspendre » ; suspendu → « Réactiver » ; fermé → aucune des deux. Un compte actif d'un client suspendu
     garde « Suspendre » (son propre statut).
455. **Pieds des cartes d'identifiants** : la carte est une colonne dont le pied est poussé en bas ; même hauteur
     minimale de pied, boutons centrés verticalement.

## Design arrêté

- **`web/src/credentials.tsx`** : 452 sur tous les textes ; 453 dans la modale de création (bouton secondaire
  « Générer » à côté du champ, fonction pure `systemIdFrom(name, random)` testée à part) ; le nom du compte vient
  de la fiche déjà chargée.
- **`_shell.customers.$id.tsx`, `_shell.accounts.$id.tsx`** : 454.
- **`app.css`** : 455 sur `.ui-card` des identifiants (colonne flex, pied en `margin-top: auto`, `min-height`,
  `align-items: center`).
- **Aucun changement de contrat ni de Go.**
- **Tests** : textes de 452 ; `systemIdFrom` (racine, coupe, chiffres, borne de 15, racine de repli) ; « Générer »
  remplit le champ ; 454 par statut ; mutation consignée ; preuve sur contabo169, captures dans `docs/steps/014e/`.

## Écarts au design, constatés à l'implémentation

- **Lettres accentuées** : le nom est décomposé (NFD) avant d'être réduit à `[a-z0-9]` ; « Société » donne `societe`,
  pas `socit`.
- **`.form__inline`** : une rangée du formulaire où le champ prend la place et « Générer » s'aligne en bas, à la
  hauteur du champ (`--control-height` des deux côtés).
- **Une seule action, un seul bouton** (454) : le bouton garde sa place et change de libellé ; après une suspension,
  le focus reste sur lui, devenu « Réactiver ». Les raisons « Déjà suspendu. » et « Déjà actif. » tombent.
- **455 vérifié au rendu** : CSS construite, deux cartes de corps inégaux côte à côte dans `agent-browser` ; les deux
  pieds ont le même haut (245 px) et la même hauteur (53 px), boutons à 13 et 12 px des bords.
- **Pas de `min-height` sur le pied** : les deux pieds portent les mêmes boutons, donc la même hauteur ; le pied
  poussé en bas suffit (455).
