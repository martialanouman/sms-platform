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
     `LoadingState` et ses squelettes ; `Icon` aux trois glyphes employés (`diamond`, `bang`, `times`). Pas de toast : le
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

## Arbitrages de revue

Premier tour, 2026-10-21 : correction et accessibilité, code en trop, fidélité à l'origine.

188. **La modale rend elle-même le focus** à l'élément qui l'a ouverte, à son démontage. Un `focus()` donné pendant
     qu'elle est encore modale tombe sur un élément inerte et finit sur `body` (WCAG 2.4.3). Le double de `showModal` des
     tests rend inerte ce qui est hors de la modale, comme le navigateur. Elle se ferme aussi par le voile (origine).
     L'écran ne retient plus l'ouvreur.
189. **Une modale se ferme dès que son action réussit**, sans attendre la relecture de la liste : sinon son bouton se
     rouvre et un second clic renvoie la requête (second lien, qui invalide le premier). Un succès ne ferme que la modale
     qui l'a demandé ; une autre, ouverte entre-temps, reste.
190. **« Réactiver » a son état par ligne** (attente, refus près du bouton), par `useSubmission`.
191. **Textes de conséquence vérifiés contre le serveur.** Désactiver : les rôles, le second facteur et un lien en cours
     restent ; « Réactiver » les rend. Lien : celui envoyé plus tôt cesse de valoir dès la demande ; pour une
     réinitialisation, mot de passe, facteur et sessions restent valables jusqu'à l'usage (origine).
192. **Fidélité** : états de session de la coquille en `ErrorState` (« Impossible de vérifier la session ») et
     `LoadingState` (« Ouverture de la session ») ; erreur de lecture des rôles en `ErrorState` avec « Réessayer » ; phrase
     d'audit des rôles dans le paragraphe du choix ; indicateur d'attente du bouton (`ui-button__spinner`) ; lignes de
     tableau denses ; squelettes de 20 px dans la modale ; libellés d'interdiction de l'origine, à nos permissions ;
     courriel de la note de création pris dans la réponse du serveur ; CSS complété (survol du rail, titre, icône
     d'erreur, fermeture de la modale, animations).
193. **Écarts assumés à l'origine** : la phrase « Vos données locales restent affichées » et la ligne de requête de
     l'`ErrorState` ne sont pas portées — aucun écran ne garde ses données affichées sous l'erreur, et la charte veut
     l'honnêteté avant le réconfort ; la validation des champs reste celle du navigateur (pas de schéma côté SPA) ; la
     première colonne du tableau est un en-tête de ligne ; « Aucun opérateur » est un état vide (§1.9) ; les actions de
     ligne sont bloquées sans `operators:write` (contrat) ; la `Toolbar` n'a pas d'emplacement de gauche tant qu'aucun
     filtre n'existe.
194. **L'infobulle d'interdiction** se ferme par Échap, se survole sans trou, et n'est pas rognée dans une modale.
195. **`/account` sans redirection d'une session non élevée** : Fable, 183 ; le serveur refuse les facteurs à une session
     non élevée et `mfa_required` mène à la vérification.

Second tour, 2026-10-21 : aucun bloquant.

196. **La modale retient son ouvreur au premier rendu**, pas dans son effet : sous `StrictMode`, l'effet rejoué captait
     « Fermer ». Un clic sur le voile ne ferme que s'il a commencé sur le voile : une sélection glissée hors d'un champ
     ne perd pas la saisie.
197. **L'infobulle se ferme par Échap** dès qu'elle est visible, au survol comme au focus (WCAG 1.4.13) ; dans une
     modale, le premier Échap la ferme, le second la modale.
198. **Le refus de la lecture des rôles** se dit par son message, comme celui des opérateurs.
199. **Assumés** : Safari ne donne pas le focus à un bouton cliqué, le focus d'une modale fermée à la souris y revient
     donc au document (au clavier, il revient à l'ouvreur) ; un refus arrivé après la fermeture de sa modale n'est pas
     montré, la liste relue dit l'état réel ; la densité des lignes est celle de l'unique tableau.

## Design arrêté

- **`web/src/ui.tsx`** : `Button`, `Icon`, `Modal`, `DataTable`, `Toolbar`, `EmptyState`, `ErrorState`, `LoadingState`
  (et ses squelettes), `PageHeader` et son nœud.
- **`web/src/routes/_shell.tsx`** : lien d'évitement, rail groupé et filtré, barre du haut (titre en portail, nom vers
  `/account`, déconnexion), contenu.
- **`web/src/routes/_shell.index.tsx`** : accueil vide, redirection non élevée.
- **`web/src/routes/_shell.account.tsx`** : « Mon compte ».
- **`web/src/routes/_shell.operators.tsx`** : l'écran de l'origine.
- **`web/src/styles/app.css`** : classes de l'origine (`shell`, `rail`, `topbar`, `page-header`, `ui-toolbar`,
  `ui-table`, `ui-modal` (et son `::backdrop`), `ui-empty`, `ui-error`, `ui-loading`, `ui-skeleton`,
  `ui-tooltip`), en jetons ; les classes de 012d qui ne servent plus disparaissent.
- **Tests** (Vitest, `fetch` simulé, axe) : rail (entrée masquée, groupe vidé), nom → `/account`, titre en portail,
  accueil, `Button` bloqué (focalisable, clic neutre, raison décrite), modale (Fermer, Échap, focus rendu), écran
  (création par modale, statuts, confirmations, phrase générique du lien, actions bloquées et expliquées, refus du
  serveur). Mutation manuelle, consignée.
- **Issue `debt` #155** : la redirection de `/` renvoie un rôle sans MFA ; `debt: #155` sur la redirection.

## Écarts au design, constatés à l'implémentation

- **« Désactiver » et « Réactiver » sont un seul bouton** (`Status`) : deux composants à la même place faisaient perdre le
  focus après une désactivation, le nœud du bouton étant remplacé.
- **Les tests rendent en `StrictMode`**, comme `main.tsx` : il a révélé que l'effet rejoué rappelait `showModal` sur une
  modale ouverte, ce que Chromium refuse. Le double de `showModal` ferme sur Échap après la propagation, comme le
  navigateur, pour qu'un `preventDefault` le retienne.
- **L'infobulle est ancrée au bord droit de son bouton** : ancrée à gauche, elle sortait de l'écran sous les actions de
  ligne. Trouvé dans Chromium.

## Mutations

Une cinquantaine, à la main, sur la coquille, le kit, l'écran et ses modales ; toutes tombent. Survivants corrigés en
route : le repli du focus sur le titre (jamais atteint, retiré), l'attente de « Réactiver », la fermeture par « Fermer »
(simplifiée), la condition de statut sur sa propre ligne (impossible, retirée), « Réessayer » des rôles, l'indicateur
d'attente, la variante de « Réactiver », le second Échap dans une modale.

## Preuve sur `contabo169`

Jouée le 2026-10-21 depuis la branche, sur le cluster en v0.22.0 : image `dashboard` en snapshot, importée sous
`proof-012d2` puis `proof-012d2-b` (correctif de l'infobulle) ; timer suspendu. Dans Chromium (`agent-browser`), par
tunnel SSH vers `port-forward`, sur `localhost:8080`, au clavier :

1. Connexion du `Propriétaire` (TOTP) : accueil vide, rail « Administration » → « Opérateurs »
   ([capture](012d-2/accueil.png)) ; le nom dans la barre du haut mène à « Mon compte », titre dans la barre
   ([capture](012d-2/mon-compte.png)).
2. « Opérateurs » par le rail ([capture](012d-2/operateurs.png)). « Nouvel opérateur » (Entrée) : la modale
   ([capture](012d-2/creation.png)) ; adresse en casse mixte, nom, Entrée dans le champ : la modale se ferme, la note dit
   l'adresse normalisée par le serveur, le focus revient à « Nouvel opérateur » ([capture](012d-2/cree.png)).
3. « Modifier les rôles » de Grace : « Support » coché à l'espace ([capture](012d-2/roles.png)), « Enregistrer les
   rôles » : focus rendu au bouton de la ligne. « Envoyer un lien » : confirmation d'activation, 72 heures
   ([capture](012d-2/lien.png)) ; « Envoyer » : focus rendu. « Désactiver » : confirmation
   ([capture](012d-2/desactiver.png)), puis le même bouton devient « Réactiver », focus conservé
   ([capture](012d-2/desactive.png)) ; Entrée le réactive.
4. Sa propre ligne : « Désactiver » bloqué, infobulle au focus ([capture](012d-2/infobulle.png)), fermée par Échap.
5. Grace activée par son lien (Mailpit), application enrôlée : le rail ne montre que la marque, sans groupe ; `/operators`
   tapé à la main dit « Il faut la permission operators:read pour lire les opérateurs. » ([capture](012d-2/support.png)).
6. axe (WCAG 2.1 AA, `color-contrast` comprise) : aucune violation sur l'accueil, « Mon compte », l'écran, la modale de
   création et la vue `Support`. Modale des rôles ouverte : une, le contraste d'un bouton « Désactiver » **sous le
   voile**, inerte pendant la modale (même constat qu'en 012d).

Validé par l'humain le 2026-10-21 ; la mise en page de « Mon compte » sera revue plus tard (#157).

Le cluster reste sur `proof-012d2-b`, timer suspendu, pour l'essai humain ; le timer se réarme au merge.
