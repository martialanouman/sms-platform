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
     `self_lockout`) se disent dans la modale. Après un enregistrement : relecture des rôles, de `me` et des
     opérateurs. Mécaniques de 012d-2 : fermeture au succès, une modale ne ferme qu'elle-même.
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

## Design arrêté

- **`docs/spec/08-dashboard.md`** §8.5 : `sessions:*` hors de tout rôle par défaut sauf `Propriétaire`.
- **`internal/dashboard/roles.go`**, **`roles_test.go`** : `Exploitation` sans `sessions:*`, table de vérité.
- **`web/src/routes/_shell.tsx`** : entrée « Rôles » (`roles:read`) du groupe « Administration ».
- **`web/src/routes/_shell.roles.tsx`** : l'écran, l'éditeur et le sélecteur.
- **`web/src/routes/_shell.account.tsx`** : la section du jeton personnel.
- **`web/src/styles/app.css`** : classes du sélecteur et de la liste des clés, en jetons.
- **Tests** (Go : table de vérité ; Vitest, `fetch` simulé, axe, `StrictMode`) : rail, liste, états, création,
  modification, suppression, refus du serveur, contrôles bloqués et expliqués, catalogue tiré de `Propriétaire`, jeton
  (délivrance, copie, refus, 503, absence dans le cache après fermeture). Mutation manuelle, consignée.
- **Preuve sur `contabo169`**, dans Chromium : le parcours du « Terminé quand », captures, axe.
