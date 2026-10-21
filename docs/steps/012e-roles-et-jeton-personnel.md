# Step 012e — Écrans Rôles et jeton personnel

Plan : `docs/plan/composants.md` §012, cinquième PR de 012 (coupe : arbitrage 138). Arbitrages antérieurs : 1 à 199.
Spec : `docs/spec/08-dashboard.md` §8.4, §8.5, §8.8, §8.9. Contrat : `contracts/openapi-bff.yaml` (`listRoles`,
`createRole`, `updateRole`, `deleteRole`, `issueGatewayToken`). Origine : `martialanouman/go-gateway-bo`,
`web/src/routes/_shell.roles.tsx`, `web/src/components/admin/permission-picker.tsx`, `web/src/routes/_shell.account.tsx`
(`PersonalToken`). Kit : `docs/steps/012d-2-coquille-de-l-origine.md`. Mode : PR.

## Contexte

012b sert les opérations sur les rôles, 012c le jeton personnel ; aucun écran ne les appelle. Le 2026-10-21, l'humain a
décidé que `sessions:*` (fermeture forcée des sessions du dashboard, #149) revient au seul `Propriétaire`.

**Terminé quand** : dans Chromium, sur l'environnement de test, un `Propriétaire` élevé ouvre « Rôles » par le rail,
compose un rôle, le modifie, voit ses permissions, le supprime, au clavier seul ; un rôle par défaut refuse
modification et suppression, expliquées ; il délivre son jeton personnel, le copie, et le jeton disparaît à la
fermeture ; un opérateur `Exploitation` n'a plus `sessions:*` ; axe ne relève aucune violation.

## Arbitrages

Fable, 2026-10-21, sur la décision humaine pour 200.

200. **`sessions:*` réservé au `Propriétaire`** (humain) : la phrase « Hors de tout rôle par défaut sauf `Propriétaire` »
     du §8.5 gagne `sessions:*` ; `Exploitation` perd `sessions:read` et `sessions:write` (révise 012a, G). Le semis à
     chaque démarrage réécrit le rôle ; l'union est lue à chaque requête, rien à faire en base.
201. **Écran `/roles`** (`roles:read`), rail « Administration » → « Rôles » : comme l'origine. `PageHeader` « Rôles » ;
     `Toolbar` « Nouveau rôle » (bloqué et expliqué sans `roles:write`) ; tableau Nom, Description, Permissions (nombre),
     Détenteurs (nombre) ; par ligne « Voir les permissions » (modale, clés en mono), « Modifier » et « Supprimer »
     bloqués et expliqués sur un rôle par défaut ou sans `roles:write`, « Supprimer » bloqué sur un rôle détenu en
     nommant ses détenteurs (`Role.holders`) ; confirmation de suppression. Éditeur en modale : Nom (création seule),
     Description, sélecteur ; phrase d'audit. Les refus du serveur (`role_name_taken`, `role_held` par course,
     `self_lockout`) se disent dans la modale. Après un enregistrement : relecture des rôles et de `me` (révisé en
     revue, 205). Mécaniques de 012d-2 : fermeture au succès, une modale ne ferme qu'elle-même.
202. **Catalogue du sélecteur** : les permissions du rôle `Propriétaire`, lu par `listRoles` — il est « Tout » (§8.5) et
     semé comme `scopes.All ∪ scopes.Permissions` (140). Groupées par ressource (le préfixe avant `:`), triées, clés en
     mono, sans description : le contrat n'en porte pas. Ni catalogue généré ni endpoint nouveau.
203. **Jeton personnel** : section « Jeton d'API personnel » de « Mon compte » (place de l'origine ; la mise en page de
     la page reste à revoir, #157), bloquée et expliquée sans `gateway:personal_token`. « Délivrer » : `POST
     /gateway-token` sans corps, en `application/json` comme tout POST (garde du BFF) ; tous les scopes permis, sans
     choix. Le jeton s'affiche une fois en modale (I2, §8.8) : jeton en mono, « Copier », heure d'expiration, scopes.
     Appel direct, sans `useMutation` (son cache garderait le secret) ; état local remis à zéro à la fermeture. Un refus
     ou une indisponibilité du signataire (503) se dit en ligne, sans nouvel essai automatique.
204. **Une PR**, preuve Chromium sur `contabo169`.

## Arbitrages de revue

Premier tour, 2026-10-21 : aucun bloquant.

205. **Après l'enregistrement d'un rôle, la liste des opérateurs n'est pas relue** : un opérateur ne porte que l'`id` et le
     nom de ses rôles, et un nom ne change pas. Rôles et `me` sont relus même sur un refus : un refus par course
     (`role_held`) dit que la liste a changé.
206. **Une suppression ne ferme qu'elle-même**, comme toute modale (189).
207. **La confirmation devient un composant du kit** (`Confirmation`, `web/src/ui.tsx`) : désactivation, envoi de lien et
     suppression de rôle la partagent ; elle relit la requête qu'on lui nomme.
208. **Fidélité** : nombres alignés à droite ; légendes du sélecteur et nom du rôle en mono (valeurs machine) ; groupes du
     sélecteur alignés en haut ; « ne retirera » ; titre d'erreur d'écran en `h2` ; jeton fermé par « J'ai copié le
     secret », copié par « Copier le secret » ; raison d'interdiction et durée du jeton de l'origine. Écarts assumés :
     sélecteur à colonnes de 220 px (pas de description), pas de mention « optionnel », refus sans titre de contexte
     (012d-2), jeton dans le bloc de secret de 011b.
209. **Assumés** : un nom fait d'espaces est refusé par le serveur (422) ; une clé hors catalogue reste théorique tant que
     `Propriétaire` est « Tout » ; le jeton n'entre jamais dans le cache par construction (appel direct), le test prouve
     son absence du document ; le corps vide du `POST /gateway-token` est prouvé dans le navigateur.
210. **§8.5 dit « sessions SMPP »** dans les portées d'`Exploitation` et de `Support`, pour ne pas les confondre avec
     `sessions:*`.

Second tour, 2026-10-21 : aucun bloquant.

211. **Le focus va au titre** après une suppression seulement si sa modale était encore ouverte : fermée par Échap
     pendant la requête, le succès ne déplace rien.
212. **La relecture sur refus** (205) ne suit qu'une réponse du serveur en 4xx, hors 429 : une panne ou une limitation ne
     déclenche pas de nouvelle lecture qui remplacerait l'écran par son erreur.
213. **Assumés** : après un refus qui retire la ligne (rôle supprimé ailleurs, 404), le focus revient au document ; la
     phrase « Personne ne détient ce rôle » reste sous un refus `role_held`, que l'alerte contredit.

## Design arrêté

- **`docs/spec/08-dashboard.md`** §8.5 : `sessions:*` hors de tout rôle par défaut sauf `Propriétaire`.
- **`internal/dashboard/roles.go`**, **`roles_test.go`** : `Exploitation` sans `sessions:*`, table de vérité.
- **`web/src/routes/_shell.tsx`** : entrée « Rôles » (`roles:read`) du groupe « Administration ».
- **`web/src/routes/_shell.roles.tsx`** : l'écran, l'éditeur et le sélecteur.
- **`web/src/routes/_shell.account.tsx`** : la section du jeton personnel.
- **`web/src/styles/app.css`** : classes du sélecteur et de la liste des clés, en jetons.
- **Tests** (Go : table de vérité ; Vitest, `fetch` simulé, axe, `StrictMode`) : rail, liste, états, création,
  modification, suppression, refus du serveur, contrôles bloqués et expliqués, catalogue tiré de `Propriétaire`, jeton
  (délivrance, copie, refus, 503, absence du document après fermeture). Mutation manuelle, consignée.
- **Preuve sur `contabo169`**, dans Chromium : le parcours du « Terminé quand », captures, axe.

## Écarts au design, constatés à l'implémentation

- **Les requêtes de liste sont mises en commun** (`web/src/queries.ts`) : l'écran Opérateurs et l'écran Rôles lisent les
  mêmes, avec la même distinction entre session perdue, refus et panne.
- **Le double de `showModal` donne le focus au premier contrôle** de la modale et la ferme par Échap d'où que vienne la
  touche, comme le navigateur : le jeton fermé par Échap n'était pas vu sinon.
- **Les légendes du sélecteur perdaient le mono** contre la règle de `.role-choice` ; trouvé dans Chromium.

## Mutations

Une quarantaine, à la main, sur la table de vérité, le rail, l'écran, l'éditeur, la confirmation, le jeton et les
requêtes ; toutes tombent. Survivants corrigés en route : le catalogue lu sur le premier rôle par défaut (fixture
réordonnée), la session perdue pendant la lecture des rôles, la fermeture du jeton par Échap, l'exclusion du 429.

## Preuve sur `contabo169`

Jouée le 2026-10-21 depuis la branche, sur le cluster en v0.23.0 : image `dashboard` en snapshot, importée sous
`proof-012e`, `proof-012e-b`, puis `proof-012e-c` (légendes) ; timer suspendu. Dans Chromium (`agent-browser`), au
clavier, en `Propriétaire` :

1. « Rôles » par le rail ([capture](012e/roles.png)) ; nombres alignés à droite ; « Modifier » et « Supprimer » des
   rôles par défaut bloqués, raison au focus ([capture](012e/par-defaut.png)).
2. « Voir les permissions » d'`Exploitation` : 37 clés, `smpp-sessions:*` mais plus `sessions:*`
   ([capture](012e/permissions.png)) ; Échap rend le focus au bouton.
3. « Nouveau rôle » ([capture](012e/nouveau-role.png)) : 30 groupes tirés de `Propriétaire` ; nom, description,
   `traffic:read` à l'espace, Entrée : le rôle paraît, focus rendu à « Nouveau rôle ». « Modifier » : `accounts:read`
   ajouté ([capture](012e/modifier-role.png)), deux permissions. « Supprimer » : confirmation
   ([capture](012e/supprimer.png)), la ligne disparaît, focus sur le titre « Rôles ».
4. « Mon compte », « Délivrer » : le `POST /gateway-token` sans corps est servi ; la modale montre le jeton une fois,
   l'heure d'expiration et 47 scopes, sans `content:read`, `content:erase` ni `gdpr:erase` ; « Copier le secret » :
   « Copié. » ; « J'ai copié le secret » : la modale se ferme, le jeton n'est plus dans le document, focus rendu à
   « Délivrer » ([capture](012e/jeton.png), prise après fermeture : le dépôt est public).
5. axe (WCAG 2.1 AA) : aucune violation sur l'écran Rôles, l'éditeur ouvert et la modale du jeton.

Le cluster reste sur `proof-012e-c`, timer suspendu ; il se réarme au merge.
