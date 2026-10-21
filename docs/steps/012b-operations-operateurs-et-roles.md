# Step 012b — Opérations opérateurs et rôles

Plan : `docs/plan/composants.md` §012, deuxième des cinq PR de 012 (coupe : arbitrage 138,
`docs/steps/012a-roles-et-autorisation.md`). Arbitrages antérieurs : 1 à 148. Spec : `docs/spec/08-dashboard.md` §8.3,
§8.5, §8.9. Contrat : `contracts/openapi-bff.yaml` (`listOperators`, `createOperator`, `updateOperator`,
`setOperatorRoles`, `requestOperatorAccessLink`, `listRoles`, `createRole`, `updateRole`, `deleteRole`). Origine :
`martialanouman/go-gateway-bo`, `internal/bff/operators.go`. Règles : `.claude/rules/tests.md`. Mode : PR.

## Contexte

- 012a a posé les rôles, leur semis, l'union des permissions et la garde `mfa_required` : ces neuf opérations, toutes
  sous une permission du BFF, n'atteignent qu'une session élevée.
- Le catalogue d'erreurs, l'`enum` du contrat et §9.3 portent déjà `email_taken`, `role_name_taken`, `self_lockout`,
  `operator_disabled`, `role_is_default`, `role_held`. Le `CHECK` du journal porte déjà les faits.

**Terminé quand** : sur l'environnement de test, par `curl`, un `Propriétaire` élevé crée un opérateur, lui pose
`Support`, demande son lien (Mailpit), le désactive puis le réactive ; il compose un rôle taillé, le pose, se voit
refuser sa suppression tant qu'il est détenu, le retire puis le supprime ; un rôle par défaut refuse la modification ;
se désactiver, ou se retirer `operators:write`, rend 409 `self_lockout` ; chaque geste a sa ligne au journal.

## Arbitrages

Fable, 2026-10-21.

149. **Périmètre** : les neuf opérations ; preuve `curl` ; aucun écran (012d, 012e).
150. **`self_lockout`** (contrat) : refusé à son auteur seul : se désactiver ; perdre `operators:write` ou `roles:write`
     par `setOperatorRoles` sur soi, ou par `updateRole` d'un rôle qu'il détient. L'auteur détient la permission de
     l'opération et ne peut se la retirer : il reste toujours un détenteur actif. Vider `operators:write` d'un rôle
     qu'on ne détient pas n'enferme personne : « qui édite les rôles s'accorde tout » (§8.5).
151. **Élévation** : la garde de 012a suffit ; pas d'élévation récente (135) ; #146 reste ouverte.
152. **Validation** : un `roleId` inconnu ou non uuid → 422 nommant `roleIds[i]` ; une permission hors
     `scopes.All` ∪ `scopes.Permissions` → 422 nommant `permissions[i]`. Doublons retirés sans erreur ; permissions
     stockées triées. Un rôle taillé peut porter toute permission générée ; `permissions: []` admis.
153. **`updateOperator`** : désactiver ferme ses sessions dans la transaction, `operator_disabled` ; réactiver,
     `operator_enabled` ; ses rôles survivent. Un lien en attente d'un compte désactivé ne part ni ne s'use (`NextDue`,
     `ConsumeLink` filtrent `active`).
154. **`createOperator`** : email normalisé comme au bootstrap ; lien d'activation en file ; `operator_created` puis
     `access_link_requested`, acteur l'auteur, cible le nouveau. `email_taken` se lit sur la violation de
     `operators_email`, pas par une lecture préalable (deux créations concurrentes).
155. **`requestOperatorAccessLink`** : `activation` sans mot de passe, sinon `reset` ; l'upsert de `QueueLink` invalide
     le précédent ; 409 `operator_disabled` ; `access_link_requested`, acteur l'auteur.
156. **Un geste sans effet** (ensemble de rôles, statut, description et permissions inchangés) : 200, aucune ligne au
     journal. Sinon `roles_changed` (cible l'opérateur), `role_created`, `role_updated`, `role_deleted` (cible le rôle).
157. **`Operator`** : `secondFactorEnrolled` vrai pour un TOTP, même non confirmé, ou une clé ; `accessLink` null sans
     ligne, consommé ou expiré ; `queued` non envoyé sous le plafond, `failed` au plafond (`maxDeliveries`, la même
     constante que `NextDue` et `Undeliverable`), `sent` envoyé et vivant.
158. **Rôles** : `role_name_taken` sans égard à la casse (141) ; `updateRole` et `deleteRole` refusent un rôle par
     défaut (`role_is_default`) ; `role_held` nomme les emails des détenteurs, triés ; `holders` de `listRoles`, triés,
     désactivés compris.
159. **404** : un `operatorId` ou `roleId` inconnu ou non uuid, sans distinction.
160. **GRANT** : `roles` et `operator_roles` reçoivent `delete`.

## Design arrêté

- **Cœur `internal/dashboard`** : `operators.go` (liste, création, statut, rôles, lien), `roles.go` (liste,
  composition, modification, suppression) ; erreurs typées ; ports de `store.go`.
- **`internal/dashboard/postgres`** : requêtes sqlc ; `email_taken` et `role_name_taken` lus sur la violation d'unicité.
- **`internal/dashboard/http`** : les neuf opérations rejoignent `include-operation-ids` ; DTO déclarés.
- **`contracts/grants/dashboard.yaml`** : `delete` sur `roles`, `operator_roles` ; sortie régénérée.
- **Tests** : Go contre Postgres réel, cœur et HTTP : chaque opération, chaque refus du contrat, chaque ligne du
  journal, les gestes sans effet, les deux créations concurrentes, l'auteur seul sujet de `self_lockout`. Mutation
  manuelle, consignée.
- **Preuve sur `contabo169`** : le parcours du « Terminé quand », par `curl`.
