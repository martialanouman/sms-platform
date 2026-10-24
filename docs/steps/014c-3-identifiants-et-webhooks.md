# Step 014c-3 — Identifiants en cartes, modale de rotation, webhooks

Troisième et dernière des trois PR de la 014c (coupe : arbitrage 383). Les arbitrages de toute la 014c vivent dans
`docs/steps/014c-1-kit-listes-et-fiche-client.md` (383–402) ; celle-ci applique 398 et 399, avec le kit livré en 014c-1
et les onglets de 014c-2 (403, 411). Les règles des identifiants et des webhooks sont celles de la 014b (341–382) ;
rien n'en change : aucun contrat, aucun Go. Maquette qui fait foi : `docs/steps/014c/maquette/CompteIdentifiants.dc.html`,
`Rotation.dc.html`, `CompteWebhooks.dc.html`. Spec : `docs/spec/08-dashboard.md` §8.8 (« Un secret s'affiche une fois »,
« Un contrôle interdit est désactivé et expliqué »). Dette tenue : #177 (pas de grâce de rotation des webhooks). Mode : PR.

**Terminé quand**, dans le navigateur sur contabo169 : l'onglet Identifiants montre deux cartes, « Bind SMPP » et « Clé
d'API », à vide ou renseignées comme la maquette ; un état vide crée ; la rotation passe par la modale de la charte
(bandeau, grâce, case « Je comprends… », « Effectuer la rotation » en danger) puis montre le secret une fois ; une carte
révoquée se lit et se ravive par rotation ; l'onglet Webhooks est la table de la maquette (badge MO/DLR, URL mono,
pilule, cinq actions) ; « Ajouter » est figé et expliqué quand les deux webhooks existent ; un toast suit chaque écriture
sans réponse propre ; le focus revient sur le contrôle d'origine ou va au titre s'il a disparu ; axe sans violation.

## Arbitrages

Fable, 2026-10-24, dans le cadre de 398 et 399.

413. **Deux cartes fixes** (398, maquette) : « Bind SMPP » puis « Clé d'API », dans la grille `.ui-cards`, toujours
     rendues toutes deux, chacune `Card({ title, aside: pilule, footer })`. Une carte a trois états : vide, renseignée,
     révoquée. Le `h2` « Identifiants » et la `section` qui l'enveloppait disparaissent : l'onglet nomme déjà la
     section, les cartes portent leurs `h2` ; l'onglet rend un fragment, comme Canaux. Sans `credentials:read`, l'URL
     tapée rend la seule phrase `needs('credentials:read', 'Les identifiants')`, aucune carte, aucun appel (404).
414. **Carte vide** (maquette) : un état propre à la carte, pas `EmptyState` du kit (icône et gabarit de liste) : titre
     en gras « Aucun bind SMPP » ou « Aucune clé d'API », phrase muette (« Le bind sert au canal SMPP. La passerelle
     génère le mot de passe et le montre une seule fois. » ; « La clé sert au canal REST. La passerelle la génère et la
     montre une seule fois. »), bouton primaire « Créer un bind SMPP » ou « Créer une clé d'API ». Le bind ouvre une
     modale « Nouveau bind SMPP » réduite au champ `system_id` (`required`, `maxLength` 15, motif `[!-~]{1,15}`) et à sa
     phrase ; la carte dit déjà le type, le `Select` « Type » de 014b-1 tombe. La clé n'a rien à demander : le bouton
     appelle `POST` lui-même (`loading`), puis `SecretOnce` ; un clic de trop se répare par une révocation. Refus en
     place dans la carte (`role="alert"`) : 409 « Ce system_id est déjà pris. » ou « Ce compte a déjà une clé d'API. »
     (course), 422 « Ce compte est fermé ou n'existe plus. ». `UNIQUE (account_id, type)` : une carte renseignée ou
     révoquée n'offre jamais « Créer ».
415. **Carte renseignée** (maquette) : `KeyValue` : `system_id` (bind seul, mono) ; « Dernière utilisation » :
     `last_used_at` à la minute en mono, sinon « jamais » muet (servi `null` jusqu'en 021, 353 : aucun vérificateur
     n'existe, « jamais » est vrai) ; « Dernière rotation » : `rotated_at` en mono, sinon « jamais » muet ; « Ancien
     secret » : « accepté jusqu'au {`grace_expires_at`, mono} », ligne rendue seulement tant que la grâce court (une
     grâce échue est de l'histoire, le journal d'audit la garde ; la colonne « Grâce » et son « expirée le » tombent) ;
     « Créé le » en mono muet. Dates par `atMinute` (014c-1, correctif 6).
416. **Pilule** dans l'en-tête de la carte : Actif ok, Désactivé neutral, **Révoqué bad** (384) : la teinte `bad`
     (`Tone`, `.ui-pill--bad` sur `--tint-red`), retirée en 014c-1 faute de consommateur, revient avec lui.
417. **Pied de carte** (maquette) : `.ui-card__foot` (filet haut, rangée de boutons `size="sm"`) : « Rotation »,
     « Désactiver » ou « Activer » (un même bouton dont le libellé suit le statut, le focus y reste), « Révoquer » en
     danger. Figés et expliqués (387) : sans `credentials:write`, `needs('credentials:write')` ; compte fermé, « Fermé
     définitivement. » sur les trois (362, 410 ; l'API permet encore désactiver et révoquer, mais un compte fermé n'a
     plus rien à protéger : ses identifiants sont inertes, le vérificateur lira le statut du compte, constat de 014b-1) ;
     révoqué : « Activer » figé « Révoqué : seule une rotation le ravive. », « Révoquer » figé « Déjà révoqué. ».
     Désactiver, Activer et Révoquer gardent leur `Confirmation` de 014b-1 (un bind désactivé ou révoqué déconnecte ses
     binds, le contrat le dit) ; au 200, `onDone` pose le toast : « Bind SMPP désactivé|activé|révoqué. », « Clé d'API
     désactivée|activée|révoquée. ».
418. **Carte révoquée** : pilule « Révoqué », mêmes lignes (le `system_id` reste réservé au compte, `UNIQUE
     (system_id)` ; aucun « Ancien secret », les hachés sont effacés), pied de 417. Aucune ligne d'explication sous les
     actions : les bulles la portent (retour humain de 014c-1), la modale de rotation dit le reste (419).
419. **Modale de rotation** (`Rotation.dc.html`) : composant du kit `RotationModal({ title, warning, children?, onClose,
     onSubmit, waiting, refusal })`, partagé par les identifiants et les webhooks (422) : bandeau `.ui-banner--warn`
     (384) ; champs facultatifs (`children`) ; case « Je comprends que le nouveau secret ne sera affiché qu'une seule
     fois. » (`Check` du kit, extrait de `Choice` : `Checkbox` de Base UI, classes `.ui-check`), **décochée à
     l'ouverture** (la maquette la montre cochée ; une condition déjà remplie n'en serait pas une) ; « L'action est
     enregistrée dans le journal d'audit. » ; pied Annuler · « Effectuer la rotation » en danger, figé « Cochez d'abord la
     case. » tant que la case ne l'est pas (387, jamais `disabled`). `Modal.title` passe à `ReactNode` pour porter le
     `system_id` en mono (« Rotation du bind SMPP `banque01` », « Rotation de la clé d'API »).
     Pour un identifiant, `children` est le `Select` « Fenêtre de grâce » Aucune · 1 h · 24 h · 7 jours (valeurs de
     014b-1), absent pour un révoqué (le contrat le ravive sans grâce). Le bandeau suit la grâce choisie et remplace la
     ligne muette de la maquette, qui redisait les options du `Select` : bind sans grâce « Sans grâce, l'ancien mot de
     passe cesse aussitôt d'être accepté et les binds ouverts du compte sont déconnectés. » ; bind avec grâce « Pendant
     {1 h|24 h|7 jours}, l'ancien mot de passe reste accepté ; aucun bind n'est déconnecté. » ; clé sans grâce « Sans
     grâce, l'ancienne clé cesse aussitôt d'être acceptée. » ; clé avec grâce « Pendant {grâce}, l'ancienne clé reste
     acceptée. » ; révoqué « Révoqué : il redevient actif avec un secret neuf, sans grâce. » ; désactivé, une ligne de plus
     « L'identifiant reste désactivé. » (contrat). Corps envoyé : `{ grace_period_sec }` (0 pour Aucune, rien pour un
     révoqué) ; 409 « Ce compte est fermé. » en place.
420. **Secret montré une fois** (maquette, 361) : `SecretOnce` titré « Nouveau mot de passe » ou « Nouvelle clé d'API »,
     `children` = `KeyValue` Compte (mono) et `system_id` (mono, bind) ; la phrase « Ce secret ne sera plus jamais
     affiché. Fermer cette fenêtre l'efface de l'écran. » devient celle de `SecretOnce`, après le `code`, pour tous ses
     appelants (jeton personnel, webhooks), qui cessent de la répéter. Ni création ni rotation ne posent de toast : la
     modale du secret est la réponse visible (397) et la carte montre la rotation à sa fermeture ; question soulevée au
     brouillon, tranchée par 397 sans recours à l'humain. Le secret ne va ni dans
     le cache de requêtes ni dans un `setQueryData` (359) : création et rotation invalident la liste, rien de plus.
421. **Focus après une modale** (règle de 014c-1 et 014c-2) : `Modal` retient l'élément actif à son montage et, au
     démontage, le refocalise s'il est encore dans le document, sinon le `h1` de la page (`#contenu h1`). Une seule règle
     dans le kit, pour toutes les modales. Une modale qui en ouvre une autre (rotation puis secret, création puis secret)
     lui passe son ouvreur (`Modal({ opener? })`, relayé par `SecretOnce` et `RotationModal`) : à la fermeture du secret,
     le focus revient sur « Rotation » ou « Ajouter », ou va au titre quand « Créer un bind SMPP » a laissé place à la
     carte. Une suppression retire sa ligne du cache (`setQueryData`) avant de fermer, pour que l'ouvreur soit déjà
     détaché ; la liste est invalidée ensuite.
422. **Webhooks** (399, maquette) : une `Card` `flush` titrée « Webhooks MO / DLR », `aside` = « Ajouter » (primaire,
     `size="sm"`), figé par `useAccount().frozen` (`accounts:write`, compte fermé) ou « Un webhook par événement au
     plus. » quand MO et DLR existent, raison dans la bulle seule (387 ; la maquette l'écrit à côté, le retour humain de
     014c-1 la met dans la bulle). Table `DataTable` : Événement = badge `.ui-badge--mo|dlr` (mono, « MO » sur
     `--tint-blue`, jeton ajouté, « DLR » sur `--tint-teal`) suivi de « SMS entrants » ou « accusés de réception » en
     `.ui-why` ; URL en `.ui-mono` ; Statut en `StatusPill` (Actif ok, Désactivé neutral) ; Actions en `.row-actions` :
     « Désactiver » ou « Activer », « Modifier l'URL », « Rotation du secret », « Supprimer » en danger. Vide : « Aucun
     webhook » et « Les SMS entrants et les accusés de réception de ce compte ne sont remis à aucune URL. » ; illisible :
     `Failure`. Le `h2` « Webhooks » et la `Toolbar` tombent. Phrase de pied `.ui-why` (maquette) : « Le secret signe
     chaque appel en HMAC-SHA256 ; il n'est montré qu'à sa création ou à sa rotation, et la rotation coupe l'ancien
     aussitôt. Chaque action est enregistrée dans le journal d'audit. » ; « coupe l'ancien aussitôt » suit #177 et change
     quand elle est payée, sans marque de plus (la marque vit sur la requête Go, 352).
423. **Actions des webhooks** : figées par `useAccount().frozen` : « Ajouter », « Activer », « Rotation du secret » (l'API
     les refuse sur un compte fermé, 366) ; par la seule permission : « Désactiver », « Modifier l'URL », « Supprimer »
     (permis sur un compte fermé, 366, déjà testé). « Désactiver » et « Activer » : `Confirmation` (« Désactiver le webhook
     MO », « Les SMS entrants de ce compte ne sont plus remis par webhook. » ; « Activer … », « … sont de nouveau remis
     par webhook. ») qui envoie `PATCH { status }`, 409 « Ce compte est fermé. », toast « Webhook MO|DLR
     désactivé|activé. ». « Modifier l'URL » : la modale de 014b-2 réduite au champ URL, `PATCH { url }` seul, toast
     « URL du webhook MO|DLR modifiée. » ; son 409 traduit tombe, un `PATCH` d'URL seule ne le déclenche pas (le trigger
     ne regarde que le scellé et le passage à `active`). « Rotation du secret » : `RotationModal` sans `children`, bandeau
     « L'ancien secret cesse immédiatement d'être accepté. » (#177), 409 « Ce compte est fermé. », 503 « Le scellement est
     indisponible : rien n'a été modifié. », puis `SecretOnce` « Nouveau secret du webhook MO|DLR » avec `KeyValue` Compte
     et Événement et « Ce secret signe chaque appel en HMAC-SHA256. » ; aucun toast (420). « Supprimer » : `Confirmation`
     de 014b-2, ligne retirée du cache (421), toast « Webhook MO|DLR supprimé. ». « Nouveau webhook » : la modale de
     014b-2 (Événement parmi les absents, URL `https`, phrase), refus 409, 422, 503 inchangés, puis `SecretOnce`.
424. **Tests** (401, 412) : par onglet, rendu et axe, aucun `[title]` ; identifiants : les deux cartes toujours
     rendues, état vide par carte, création d'un bind (corps `system_id`) et d'une clé (appel direct sans modale),
     secret montré une fois puis absent du DOM et du cache, lignes de la carte (mono, « jamais », « Ancien secret »
     tant que la grâce court et pas après, « Créé le »), pilule par statut dont « Révoqué » bad, pied figé par
     permission, par compte fermé et par révocation avec sa raison au focus, toasts de Désactiver, Activer, Révoquer ;
     modale de rotation : bandeau par type et par grâce, révoqué sans `Select`, désactivé « reste désactivé », bouton
     figé tant que la case n'est pas cochée (raison au focus) puis libre, corps `grace_period_sec`, 409 en place, aucun
     toast ; focus : retour sur « Rotation » après le secret, titre après une création depuis l'état vide ; webhooks :
     badge et texte muet par événement, URL mono, pilule, « Ajouter » figé « Un webhook par événement au plus. »,
     confirmation de statut (corps `{ status }`, toast), URL seule au `PATCH`, rotation par `RotationModal` (case, corps
     vide, 503), suppression (ligne retirée, focus au titre, toast), compte fermé (Ajouter, Activer, Rotation figés ;
     Désactiver, Modifier, Supprimer libres) ; kit : `Modal` refocalise l'ouvreur ou le `h1` s'il a disparu, `opener`
     relayé, `Check`, `.ui-pill--bad`, phrase de `SecretOnce` ; tests de 014b retirés avec ce qu'ils couvraient (`Select`
     « Type », colonnes de l'ancienne table, `Select` « Statut » de la modale d'édition, « expirée le ») ; mutation à la
     main ; preuve sur contabo169, captures dans `docs/steps/014c-3/`.
425. **Hors 014c, sans issue `debt`** (400) : `last_used_at` écrit par le vérificateur (021, 353) ; déconnexion des binds
     (#173) ; grâce de rotation des webhooks (#177) ; binds vivants (025a, 025b, 409). Rien de promis en 014c n'est
     reporté.

## Design arrêté

- **`web/src/ui.tsx`** : `Tone` reprend `bad` (416) ; `Check` extrait de `Choice` (419) ; `RotationModal` (419) ;
  `Modal.title: ReactNode`, `Modal({ opener? })` et retour de focus sur l'ouvreur ou le `h1` (421) ; `SecretOnce` porte
  la phrase finale et relaie `opener` (420, 421).
- **`app.css`** : `.ui-pill--bad`, `.ui-card__foot`, `.ui-badge`, `.ui-badge--mo`, `.ui-badge--dlr` (jeton `--tint-blue`),
  `.ui-banner--warn`, état vide de carte ; sur les jetons existants.
- **`web/src/credentials.tsx`** : cartes de 413 à 418, création de 414, rotation de 419, secret de 420 ; `Toolbar`,
  `DataTable`, `Select` « Type », colonnes « Grâce » et « expirée le » retirés.
- **`web/src/webhooks.tsx`** : carte et table de 422, actions de 423 ; `Toolbar`, `h2`, `Select` « Statut » de la
  modale d'édition et son 409 retirés ; `Form` et `UrlField` restent pour la création et l'URL.
- **`web/src/routes/_shell.account.tsx`** : la phrase du jeton personnel ne redit plus celle de `SecretOnce` (420).
- **Routes** `_shell.accounts.$id.credentials.tsx` et `.webhooks.tsx` : inchangées.
- **Aucun changement de contrat ni de Go.**
- **Tests** : 424 ; captures dans `docs/steps/014c-3/`.

## Écarts au design, constatés à l'implémentation

- **Raison de l'onglet sans `credentials:read`** (413) : `needs('credentials:read', 'Cet onglet')`, « Cet onglet
  demande Identifiants : lire `credentials:read`. », comme l'onglet figé de 014c-2 ; `needs` conjugue au singulier,
  « Les identifiants demande » serait fautif.
- **`opener` relayé par `SecretOnce` seul** (421) : `RotationModal` est toujours ouverte par un bouton de la page,
  l'élément actif à son montage est déjà l'ouvreur ; sa prop `opener` survivait à sa mutation et est retirée. Le relais
  de `SecretOnce` est nécessaire : sous `StrictMode`, la modale du secret retient sinon le bouton de la modale fermée,
  et le focus irait au titre au lieu de « Rotation » (test du kit rendu sous `StrictMode`, comme l'application).
  Base UI rend aussi le focus, mais de façon asynchrone ; le retour synchrone du kit reste.
- **Routes Identifiants et Webhooks** : `component: Credentials` et `component: Webhooks`, qui lisent `useAccount()`
  (Webhooks y prend aussi `frozen`, 422) ; les enveloppes qui passaient le compte en prop tombent (correctif B2).
- **Rotation d'un révoqué** (419) : le corps est `{}`.
- **Bandeau d'un identifiant désactivé** : deux paragraphes dans `.ui-banner--warn`, la grâce puis « L'identifiant
  reste désactivé. ».
- **Classes** : `.ui-badge` porte la forme commune, `--mo` et `--dlr` la teinte (`--tint-blue` existait déjà, avec
  `--text-info`) ; état vide de carte en `.ui-card__empty` et `.ui-card__empty-title` ; la ligne de la case réutilise
  `.role-choice__option`, texte enveloppé dans un `span` (grille à deux colonnes). Paires de contraste ajoutées :
  avertissement sur ambre, info sur bleu, accent sur teal.
- **Biome** : `inputComponents` passe de `CheckboxRoot` à `Check` ; l'alias `CheckboxRoot` tombe.
- **`renderAt`** rend aussi son `QueryClient`, pour prouver que le secret n'entre pas dans le cache (420).
- **Textes choisis** : Select de création « MO (SMS entrants) », « DLR (accusés de réception) » ; titres « Nouveau
  bind SMPP », « Modifier l'URL du webhook MO|DLR », « Rotation du secret du webhook MO|DLR », « Nouveau secret du
  webhook MO|DLR » (création comme rotation) ; confirmations d'identifiant gardant leur titre de 014b-1
  (« Désactiver : Bind SMPP acme01 »).
- **Refus** : le `return` après un refus traduit tombe (création, rotation) ; un refus n'a jamais de `data`, le
  mutant était équivalent.
- **Non repris** : la phrase de pied de la maquette des identifiants (« Aucune action ne révèle un secret
  existant… »), absente des arbitrages.
- **Non fait ici** : la preuve sur contabo169 et ses captures (`docs/steps/014c-3/`), à la main de l'humain avant le
  merge.

## Mutations

À la main, une ligne à la fois, restauration vérifiée par `cmp` ; suite Vitest ciblée.

| Unité | Tués | Survivants |
|---|---|---|
| Kit : `Modal` (ouvreur, titre), `SecretOnce`, `RotationModal`, `Check`, `Choice` (`ui.tsx`) | 22/22 | 3 au premier passage : relais `opener` de `SecretOnce` (test du kit passé sous `StrictMode`), `opener` de `RotationModal` (retiré, écart ci-dessus) |
| Identifiants (`credentials.tsx`) | 69/69 | 5 au premier passage : texte « Ses binds ouverts » hors désactivation d'un bind, bouton occupé pendant la création d'une clé (tests ajoutés) ; `return` après refus et `opener` de la rotation (retirés) |
| Webhooks (`webhooks.tsx`) | 48/48 | 2 au premier passage : refus d'une modification d'URL, colonne Actions alignée en fin (tests ajoutés) |

**Équivalents nommés** : aucun.

## Correctifs de revue, tour 1

Deux revues en lecture seule et une première passe dans le navigateur.

1. **A1, moyenne** : pendant le POST de « Créer une clé d'API », une rotation ou une création de bind pouvait partir ;
   sa réponse écrasait le secret de la clé et fermait la modale ouverte. La création de la clé remonte dans
   `Credentials` ; tant qu'elle attend, toutes les actions des cartes sont figées, « Une clé d'API est en cours de
   création. ». Tests : actions du bind et « Créer un bind SMPP » figés pendant l'attente, aucun dialogue ouvert, puis
   libres. Mutant tué : raison d'attente retirée. La garde « ne jamais écraser un secret montré » n'est pas ajoutée :
   la course fermée, aucun chemin ne l'atteint et son mutant serait équivalent.
2. **A2, basse** : « Ajouter » n'est rendu qu'une fois la liste lue (ni au chargement, ni après un échec). Tests :
   chargement en attente puis bouton, échec sans bouton. Mutant tué : `webhooks.data &&` → `true &&`.
3. **A3, basse** : un 409 à la création d'une clé relit la liste ; la carte montre la clé existante. Mutant tué :
   invalidation retirée.
4. **A4, assumé** : une grâce qui échoit sur une page laissée ouverte reste affichée jusqu'à la relecture ;
   `refetchOnWindowFocus` la rattrape.
5. **A5, test** : clé créée, la liste passe à `[bind, key]` et le focus va au `h1` à la fermeture du secret. Mutant
   tué : `opener?.isConnected` → `opener`.
6. **A6, test** : après la confirmation Désactiver, le focus reste sur la bascule, devenue « Activer ». Mutant tué :
   retour toujours au titre.
7. **A7, navigateur** : la colonne Actions des webhooks n'était pas alignée en fin (`.row-actions` en flex ignorait
   `text-align`) ; `.ui-table__cell--end .row-actions { justify-content: flex-end }`. Le test précédent (classe de
   l'en-tête) passait malgré ce rendu ; la règle est testée dans `styles.test.ts`. Mutant tué : `flex-start`.
8. **B1, code en trop** : `Toolbar`, `.ui-toolbar` et `--subbar-height` retirés, sans lecteur.
9. **B2, code en trop** : `Credentials` lit `useAccount()`, la route devient `component: Credentials`.
10. **B3, code en trop** : `close()` au lieu de `setPending(undefined)`.
11. **B4 à B9, tests** : retirés car creux ou en double : `not.toContain('secret:')`, mono de la fixture du kit,
    rotation d'une clé active, porte de la case reprise dans les identifiants et les webhooks (gardée dans le kit),
    pilule `bad` du kit, deux lignes de la phrase du jeton personnel.
12. **B10, B11, tests** : aides `noToast`, `cached(queryClient)` et `understood` dans `web/src/test.tsx`, adoptées par
    les tests des identifiants, des webhooks, de la fiche compte et de la liste Clients ; `value()` typé
    `HTMLElement | undefined`.

## Preuve dans le navigateur

Sur contabo169, `Propriétaire` (compte de preuve), image du dashboard construite depuis la branche, fenêtre
1440 × 960 ; compte de preuve `preuve-014c-3` créé sous Arolitec, puis clôturé et supprimé (ses identifiants et
webhooks partent avec lui). Captures dans `docs/steps/014c-3/` ; sur celle du secret, le secret est masqué avant la
prise.

- Identifiants vides : deux cartes, « Aucun bind SMPP » et « Aucune clé d'API », chacune avec son bouton de création ;
- bind créé (`system_id` seul), « Nouveau mot de passe » avec Compte et `system_id`, secret absent du DOM après
  « J'ai copié le secret », focus sur le titre, carte renseignée (mono, « jamais », « Créé le ») ;
- rotation : « Effectuer la rotation » figé « Cochez d'abord la case. » et sans effet au clic ; grâce 24 h, bandeau
  « Pendant 24 h, l'ancien mot de passe reste accepté ; aucun bind n'est déconnecté. » ; après le secret, focus sur
  « Rotation », aucun toast, « Ancien secret accepté jusqu'au … » ;
- clé d'API créée sans modale, puis révoquée : toast « Clé d'API révoquée. », focus sur « Révoquer », pilule rouge,
  « Activer » et « Révoquer » figés et expliqués, « Rotation » libre ;
- webhooks MO et DLR créés (scellement réel par `content-key-svc`), « Ajouter » alors figé « Un webhook par événement
  au plus. » ; MO désactivé (toast, focus sur « Activer ») ; secret du DLR tourné (bandeau « L'ancien secret cesse
  immédiatement d'être accepté. », focus sur « Rotation du secret ») ; MO supprimé (toast, focus sur le titre) ;
  actions alignées en fin de ligne après correctif ;
- compte fermé : « Ajouter », « Rotation du secret » et les six actions des identifiants figés « Fermé définitivement. » ;
  « Désactiver », « Modifier l'URL », « Supprimer » d'un webhook libres (366) ;
- aucun attribut `title` ; axe (WCAG 2.1 AA) sur les deux onglets : aucune violation.

Incident de preuve : la lecture de la modale du premier secret par l'outil a recopié le mot de passe du bind de preuve
dans le journal de session (hors dépôt). Le bind a été tourné aussitôt, puis supprimé avec son compte ; la capture de
cette modale masque le secret.

## Correctifs de revue, tour 2

Une revue des correctifs du tour 1 : deux défauts bas, introduits par lui.

1. **C1** : un 409 à la création d'une clé focalisait son alerte, que la relecture démontait ; le focus tombait sur
   `body`. Le 409 ne pose plus de message traduit : il rend le focus au `h1` (421) et relit la liste, et la carte
   montre la clé existante. Test : après un 409 dont la relecture rend la clé, focus au titre, aucune alerte, aucun
   toast (397). Mutant tué : focus au titre retiré. L'erreur brute posée par `useSubmission` reste dans l'état de
   `Credentials`, sans lecteur : la carte vide de la clé ne revient jamais après un 409 (un identifiant ne se supprime
   pas, un révoqué garde sa carte). Un `fail('')` qui l'effaçait a survécu à sa mutation, équivalent, et est retiré.
   Si la relecture ne montrait pas la clé, le message de la passerelle resterait lisible dans la carte.
2. **C2** : un 429 avec `Retry-After` gardait `waiting` vrai N secondes et figeait toutes les cartes avec une raison
   fausse. Les cartes sont figées par un booléen `creating`, vrai le temps du seul POST ; le bouton de la clé garde
   son attente. Test : après un 429 à `Retry-After: 30`, le bouton de la clé est occupé, « Rotation » du bind est
   libre. Mutants tués : `setCreating(true)` retiré, `setCreating(false)` retiré, raison d'attente retirée, garde du
   422.
