# Step 014d-2 — Écran Rôles

Deuxième des quatre PR de 014d (coupe : arbitrage 426). Les arbitrages de toute la 014d sont dans
`docs/steps/014d-1-operateurs.md` (426 à 451) ; cette PR applique 430 (pied « N rôles »), 433 à 440 et 448, 449 pour
sa part. Maquette : `docs/steps/014d/maquette/RolesListe.dc.html`, `RoleCreation.dc.html`, `RoleFiche.dc.html`.
Spec : `docs/spec/08-dashboard.md` §8.5, §8.8. Mode : PR.

**Terminé quand**, dans le navigateur sur contabo169 : la liste des rôles suit la maquette (carte, Nom en bouton,
Description, Type en étiquette, Permissions, Détenteurs, Supprimer, pied « N rôles ») ; un clic sur le nom ouvre la
fiche en modale, en lecture seule pour un rôle par défaut ; la création et la fiche montrent les permissions en
français, groupées par ressource dans l'ordre de la maquette, sur trois colonnes, la clé technique en bulle au survol
et au focus, les dix sensibles étiquetées ; cocher Trafic › Lire coche et fige Temps réel ; le serveur ajoute
`stream:read` à tout rôle qui lit le trafic ; axe sans violation, aucun `[title]`.

## Design arrêté

- **Cœur** (`internal/dashboard/roles.go`) : `generated()` ajoute `stream:read` quand `traffic:read` est présent, puis
  `permissions()` trie et dédoublonne (433). Tests : `CreateRole` et `UpdateRole` avec `traffic:read` seul rendent
  `stream:read` ; `stream:read` seul reste tel quel ; tout rôle par défaut à `traffic:read` porte `stream:read`.
- **Spec** : §8.5 « Toute portée qui lit le trafic (`traffic:read`) comprend `stream:read` ».
- **Générateur** (`tools/openapi`) : `scopes.gen.ts` exporte `sensitive: readonly Scope[]` depuis `scopes.Sensitive`,
  comme `personalExcluded` ; son test exige les dix (436).
- **`web/src/permissions.ts`** : `resources` réordonnée comme la maquette et renommée (435) ; verbe par suffixe
  (`read` Lire, `write` Modifier, `erase` Effacer, `delete` Supprimer, `publish` Publier, `reveal` Révéler) et trois
  exceptions (`billing:topup`, `cdr:export_bulk` « Exporter en masse », `gateway:personal_token` « Émettre ») ; les
  verbes spéciaux d'avant tombent. Les textes de `needs()` suivent ; les tests qui citent un ancien libellé sont
  réalignés.
- **`web/src/ui.tsx`** : `Hint({ hint, render })`, la bulle du kit sans `aria-disabled` ; `Explained` devient `Hint`
  plus `aria-disabled` (437). Le déclencheur est l'option entière ; si un test montre que le `Tooltip` de Base UI ne
  s'ouvre pas sur un focus propagé, le déclencheur est la case (437). La clé est liée à la case par
  `aria-describedby`.
- **`web/src/picker.tsx`** : grille de trois colonnes, un `fieldset` par ressource dans l'ordre de `resources` (une
  ressource hors table en fin, triée) ; une case par permission, libellé français, clé en bulle, étiquette
  `.ui-tag--sensitive` « sensible » sur les dix ; case Temps réel figée, cochée si et seulement si Trafic › Lire l'est,
  bulle « `stream:read` · compris avec la lecture du trafic », `stream:read` envoyé avec `traffic:read` (433, 434,
  436, 437).
- **`web/src/routes/_shell.roles.tsx`** (438 à 440) : liste dans une `Card` (Nom en bouton à l'apparence de lien,
  Description muette, Type « Par défaut » ou « Personnalisé » en `.ui-tag`, Permissions et Détenteurs en nombres mono
  à droite, Supprimer), pied « N rôles » ; `RoleForm` partagé : création (Nom, Description, grille ; « Créer le rôle »)
  et fiche (en-tête nom et étiquette de type ; Description, grille ; Supprimer à gauche, Annuler, Enregistrer) ; rôle
  par défaut en lecture seule (description en texte, cases figées, ligne « Les rôles par défaut ne se modifient pas :
  chaque déploiement les réécrit. », pied « Fermer ») ; rôle personnalisé sans `roles:write` : cases figées,
  Enregistrer et Supprimer figés et expliqués ; Supprimer figé et expliqué aussi quand le rôle est détenu ; toasts « Rôle {nom} créé. »,
  « Rôle {nom} enregistré. », « Rôle {nom} supprimé. » (397) ;
  `useClosedOn`. « Voir les permissions », « Modifier », `View` et `.role-keys` tombent.
- **Tests** : 449 pour la part 014d-2 ; mutation consignée ; preuve sur contabo169, captures dans `docs/steps/014d-2/`.

## Écarts au design, constatés à l'implémentation

- **Cases figées par `readOnly`, pas `disabled`** : une case `disabled` sort de l'ordre de tabulation et sa bulle de
  clé ne s'ouvre plus au clavier ; `readOnly` (Base UI, `aria-readonly`) la garde focalisable. Vaut pour un rôle par
  défaut, un rôle personnalisé sans `roles:write` et la case Temps réel.
- **`streamFollowsTraffic`** : la dérivation de Temps réel est une option du sélecteur, que seul l'écran Rôles allume ;
  le sélecteur du jeton personnel (014a) n'y gagne aucune règle que 433 ne lui donne pas.
- **Bulle sur l'option entière** : un test montre que la bulle s'ouvre au focus de la case quand le déclencheur est le
  `label` ; la clé est liée à la case par `aria-describedby` (`Hint` reçoit l'`id`).
- **Nom accessible de la fiche** : l'étiquette de type est dans le titre ; la modale s'annonce « Support Par défaut ».
- **Focus après suppression** : il va au `h1` quand la liste relue ne contient plus le rôle ; plus tôt, le retour de
  focus de la modale vers son ouvreur l'emporte, puis la ligne disparaît et le focus tombe sur `body`.
- **Ordre envoyé** : `stream:read` suit `traffic:read` dans le corps ; le serveur trie (`generated()`).
- **Nom en police courante** : `.ui-input--mono` n'avait plus d'appelant.
- **Correctif de revue 1** : dans un groupe, les cases suivent l'ordre de la maquette — Lire, Modifier, puis le verbe
  particulier — pour les rôles comme pour le jeton personnel ; le catalogue passe tel quel, le sélecteur ordonne.
- **Correctif de revue 2** (révise 433 sur un point) : Temps réel n'est cochée et figée que si Trafic › Lire l'est ;
  sinon c'est une case libre, et un `stream:read` coché seul n'est jamais retiré — décocher Trafic laisse Temps réel
  cochée. L'écran suit ainsi le serveur, qui garde un `stream:read` seul tel quel ; avant, changer une autre case d'un
  tel rôle lui retirait le temps réel sans bruit.
- **Correctif de revue 3** : libellé et étiquette « sensible » dans un seul `span` ; la grille de l'option renvoyait
  l'étiquette sous la case. L'espace du texte remplace la marge CSS.
- **Correctif de revue 4** : `Hint` et `Explained` ne sont plus exportés ; `locked` vaut `!writes` ; le type d'un rôle
  s'écrit une fois (`Kind`) ; le test « tout rôle par défaut au trafic porte `stream:read` », recouvert par la table de
  §8.5 et jamais vu échouer, tombe.
- **Rôles déjà en base** : ceux qui portent `traffic:read` sans `stream:read` ne sont pas réalignés ; ils le seront à
  leur prochaine écriture. Aucun rattrapage (433) : pas de production, contabo169 se réinitialise.
