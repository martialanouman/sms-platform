# Step 012d — Coquille et écran Opérateurs

Plan : `docs/plan/composants.md` §012, quatrième PR de 012 (coupe : arbitrage 138). Arbitrages antérieurs : 1 à 172.
Spec : `docs/spec/08-dashboard.md` §8.3, §8.5, §8.8 ; `docs/spec/01-*.md` §1.2. Contrat : `contracts/openapi-bff.yaml`
(`me`, `listOperators`, `createOperator`, `updateOperator`, `setOperatorRoles`, `requestOperatorAccessLink`,
`listRoles`). Charte : `docs/steps/011b-2a-charte.md`. Origine : `martialanouman/go-gateway-bo`, `web/src/routes/_shell*`,
`web/src/components/shell`. Règles : `.claude/rules/tests.md`. Mode : PR.

## Contexte

- 012b sert les opérations ; aucune page ne les appelle. `/` est une carte `AuthLayout` (identité, déconnexion,
  facteurs), sans rail (107 : « la coquille arrive avec le premier écran qui la peuple »).
- La garde de 012a confine à `/auth/*` une session non élevée **dont les rôles demandent la MFA** ; un rôle de lectures
  ordinaires travaille sans élévation.

**Terminé quand** : dans Chromium, sur l'environnement de test, un `Propriétaire` élevé ouvre la coquille, crée un
opérateur, lui pose `Support`, lui envoie un lien, le désactive puis le réactive, au clavier seul ; sa propre ligne
refuse la désactivation, expliquée ; un opérateur `Support` voit l'entrée « Opérateurs » désactivée et expliquée ; axe ne
relève aucune violation.

## Arbitrages

Fable, 2026-10-21.

173. **Une PR** : coquille et écran ensemble, prouvés dans Chromium (précédent 011b-3).
174. **Garde de la coquille** : route sans chemin `_shell`, `beforeLoad` sur `me` : session absente → `/login` ; `me`
     illisible → la coquille rend son erreur avec « Réessayer », sans déconnecter. **Pas de redirection d'une session non
     élevée** : un rôle de lectures ordinaires y travaille (§8.3). Le 403 `mfa_required` se traite une fois, au client
     API : facteur enrôlé → `/mfa`, aucun → `/enroll`. Ni copie de `RequiresMFA` côté SPA, ni champ nouveau au contrat.
     `useActivity` et la fermeture à l'inactivité passent dans la coquille.
175. **Rail** : les seuls écrans qui existent : « Mon compte » et « Opérateurs » ; « Rôles » vient en 012e. Une entrée
     sans sa permission de lecture (`me.permissions`) est désactivée et expliquée, jamais masquée (§8.8).
176. **`/` devient « Mon compte »** sous la coquille, sans `AuthLayout` : identité et facteurs. La déconnexion passe à
     l'en-tête de la coquille. `/` garde sa redirection d'une session non élevée vers `/login`
     (`verification-interrupted`) : elle sert le parcours de connexion de 011b, que 012d ne change pas.
177. **Écran `/operators`** (`operators:read`) : tableau (nom, email, statut, rôles, second facteur, lien) ; création en
     ligne (email, nom) sous `operators:write` ; par ligne : « Rôles » (dialogue à cases, rôles de `listRoles`, donc
     `roles:read` en plus, sinon désactivé et expliqué), « Envoyer un lien » (désactivé et expliqué sur un compte
     désactivé), « Désactiver » / « Réactiver » (désactivé et expliqué sur sa propre ligne). « Rôles » reste actif sur
     sa propre ligne : le `self_lockout` du serveur se dit en ligne. Les refus du serveur s'affichent par leur
     `message`, en ligne près de l'action, sans toast. États vide, chargement, erreur.
178. **Dialogue** : `<dialog>` natif (`showModal`, qui porte `inert` et le piège du focus) ; Échap et la fermeture rendent
     le focus au bouton d'origine. Aucune bibliothèque nouvelle.
179. **Après une mutation** : la liste est relue (pas d'optimiste) ; une mutation sur soi relit aussi `me`.
180. **Charte** : composants et classes de 011b-2a ; nouveaux (coquille, rail, tableau, dialogue) seulement au besoin, en
     jetons.

## Design arrêté

- **`web/src/routes/_shell.tsx`** : garde, en-tête (identité, déconnexion), rail, `Outlet`, erreur et « Réessayer ».
- **`web/src/routes/_shell.index.tsx`** : « Mon compte » (contenu de l'ancien `index.tsx`, qui disparaît).
- **`web/src/routes/_shell.operators.tsx`** : l'écran.
- **`web/src/api.ts`** ou **`session.tsx`** : le 403 `mfa_required` mène à `/mfa` ou `/enroll`.
- **`web/src/styles/app.css`** : coquille, rail, tableau, dialogue, en jetons.
- **Tests** (Vitest, `fetch` simulé, axe) : garde (absente, illisible), rail (permission absente), écran (liste, vide,
  erreur, création, rôles, lien, statut, refus du serveur, actions désactivées et expliquées), dialogue (focus, Échap),
  `mfa_required`. Mutation manuelle, consignée.
- **Preuve sur `contabo169`**, dans Chromium par `port-forward` : le parcours du « Terminé quand », captures, axe.
