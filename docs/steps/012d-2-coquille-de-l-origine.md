# Step 012d-2 — Coquille et écran Opérateurs de l'origine

Plan : `docs/plan/composants.md` §012, entre 012d et 012e (humain, 2026-10-21 : « oui, lance 012d-2 »). Arbitrages
antérieurs : 1 à 180. Spec : `docs/spec/08-dashboard.md` §8.5, §8.8, §8.9 ; §8.3 (durées des liens). Charte :
`docs/steps/011b-2a-charte.md`, `.claude/skills/sms-gateway-design/`. Origine : `martialanouman/go-gateway-bo`,
`web/src/components/{shell,rail,top-bar,page-header}.tsx`, `web/src/components/ui/`,
`web/src/routes/_shell.{index,account,operators}.tsx`, `web/src/styles/{app,components,feedback}.css`. Mode : PR.

## Contexte

012d a livré la coquille et l'écran Opérateurs au plus court, sur la seule charte : marque dans l'en-tête, « Mon compte »
en tête d'un rail plat, création en ligne au-dessus d'une `<table>` nue. L'humain demande la mise en page et le
formulaire de l'origine.

**Terminé quand** : dans Chromium, sur l'environnement de test, un `Propriétaire` élevé ouvre `/`, atteint « Mon compte »
par son nom dans la barre du haut, crée un opérateur par la modale « Nouvel opérateur », lui pose `Support`, lui envoie un
lien, le désactive (confirmation) puis le réactive, au clavier seul ; un opérateur `Support` ne voit pas « Opérateurs »
dans le rail ; axe ne relève aucune violation.

## Arbitrages

Fable, 2026-10-21, sur la décision humaine.

181. **Rail de l'origine** : lien d'évitement « Aller au contenu » ; marque « SMS Gateway » (lien vers `/`) ; groupes
     titrés. Une entrée sans sa permission de lecture est **masquée** et un groupe vidé disparaît : le rail est une
     navigation, pas un contrôle, et §8.5 (« La SPA masque ce qui est interdit ») est la raison qu'exige §8.8. §8.8 reste
     la règle des contrôles dans un écran. Seul groupe : « Administration » → « Opérateurs » (« Rôles » en 012e).
     Révise 175.
182. **Barre du haut** : à gauche le titre de l'écran, que l'écran déclare par `PageHeader`, rendu en portail dans un
     nœud de la coquille ; à droite le nom de l'opérateur, lien vers `/account`, et « Se déconnecter ». Le statut temps
     réel et le centre de notification viennent avec 026 (plan), pas une dette.
183. **`/account`** = « Mon compte » (contenu de l'actuel `/`) ; **`/`** = accueil, état vide « Le cockpit d'exploitation
     se construit », sans appel. La redirection d'une session non élevée vers `/login?notice=verification-interrupted`
     **reste sur `/`**, cible de `login.tsx` et de `useElevated` : elle sert le parcours de 011b. Plafond connu, déjà vrai
     en 012d : elle renvoie aussi un rôle sans MFA (`Reporting`, `noc`) ; issue `debt`. Révise 176.
184. **Kit porté, réduit à l'usage, sans Base UI** (102 tient) : `Modal` sur `<dialog>` natif (titre, bouton « Fermer »
     `aria-label`, glyphe `aria-hidden`, corps, pied ; `showModal` une fois, focus rendu à l'ouvreur) ; `DataTable`
     (`<caption>` masquée visuellement, `scope="col"`, sans tri) ; `Toolbar` ; `EmptyState`, `ErrorState`,
     `LoadingState` et `Skeleton` ; `Icon` aux trois glyphes employés (`diamond`, `bang`, `times`). Pas de toast : le
     succès de création se dit en `role="status"` sur l'écran.
185. **`Button`** (révise 106 sans la contredire : il rend un `<button>` à classe) : variantes `primary`, `secondary`,
     `danger`, taille `sm`, `loading` (`aria-busy`), `blockedReason` : `aria-disabled`, `aria-describedby` vers un texte
     toujours présent masqué visuellement, infobulle CSS au survol et au focus (108 : jamais `title` seul). Le bouton
     reste focalisable ; le clic neutralisé ne soumet ni ne navigue.
186. **Écran Opérateurs de l'origine** : `PageHeader` « Opérateurs » ; `Toolbar` « Nouvel opérateur » (bloqué et expliqué
     sans `operators:write`) → modale (paragraphe de conséquence, « Adresse e-mail », « Nom affiché », « Annuler » /
     « Créer l'opérateur ») ; tableau : Opérateur (nom et e-mail), Statut (Désactivé, Actif, Activation en attente, Envoi
     en attente, Lien envoyé, Envoi en échec), Rôles, Second facteur (Configuré, Aucun), Actions : « Modifier les rôles »
     (modale ; `roles:read` requis, sinon bloqué et expliqué), « Désactiver » (danger, confirmation, bloqué sur soi) ou
     « Réactiver » (direct), « Envoyer un lien » (confirmation, bloqué sur un compte désactivé). Phrase d'audit (§8.9) dans
     chaque modale. La durée du lien se dit par `accessLink.kind` ; `accessLink` null ne la devine pas (activation
     consommée, expirée ou jamais en file : 012b, arb. I) : « un lien à usage unique : activation (72 h) si le compte n'a
     pas encore de mot de passe, réinitialisation (1 h) sinon ». Sans `operators:read`, l'écran le dit sans interroger
     (012d). Permissions du contrat : `operators:read|write`, `roles:read`.
187. **Une PR**, preuve Chromium sur `contabo169` comme 012d.

## Design arrêté

- **`web/src/ui.tsx`** : `Button`, `Icon`, `Modal`, `DataTable`, `Toolbar`, `EmptyState`, `ErrorState`, `LoadingState`,
  `Skeleton`, `PageHeader` et son nœud.
- **`web/src/routes/_shell.tsx`** : lien d'évitement, rail groupé et filtré, barre du haut (titre en portail, nom vers
  `/account`, déconnexion), contenu.
- **`web/src/routes/_shell.index.tsx`** : accueil vide, redirection non élevée.
- **`web/src/routes/_shell.account.tsx`** : « Mon compte ».
- **`web/src/routes/_shell.operators.tsx`** : l'écran de l'origine.
- **`web/src/styles/app.css`** : classes de l'origine (`shell`, `rail`, `topbar`, `page-header`, `ui-toolbar`,
  `ui-table`, `ui-modal`, `ui-scrim` en `::backdrop`, `ui-empty`, `ui-error`, `ui-loading`, `ui-skeleton`,
  `ui-tooltip`), en jetons ; les classes de 012d qui ne servent plus disparaissent.
- **Tests** (Vitest, `fetch` simulé, axe) : rail (entrée masquée, groupe vidé), nom → `/account`, titre en portail,
  accueil, `Button` bloqué (focalisable, clic neutre, raison décrite), modale (Fermer, Échap, focus rendu), écran
  (création par modale, statuts, confirmations, phrase générique du lien, actions bloquées et expliquées, refus du
  serveur). Mutation manuelle, consignée.
- **Issue `debt` #155** : la redirection de `/` renvoie un rôle sans MFA ; `debt: #155` sur la redirection.
