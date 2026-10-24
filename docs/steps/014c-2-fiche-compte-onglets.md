# Step 014c-2 — Fiche compte en onglets

Deuxième des trois PR de la 014c (coupe : arbitrage 383). Les arbitrages de toute la 014c vivent dans
`docs/steps/014c-1-kit-listes-et-fiche-client.md` (383–402) ; celle-ci applique 393 à 396, avec le kit livré en
014c-1. Maquette qui fait foi : `docs/steps/014c/maquette/CompteIdentifiants.dc.html`, `CompteCanaux.dc.html`,
`CompteSessions.dc.html`, `CompteWebhooks.dc.html`. Spec : `docs/spec/08-dashboard.md` §8.8. Mode : PR.

**Terminé quand**, dans le navigateur sur contabo169 : la fiche compte porte l'en-tête de la maquette (fil d'Ariane
Clients › client › compte, nom en mono, pilule du statut effectif, « Compte de {client} · créé le … », « Renommer »,
actions de statut) et quatre onglets qui sont des routes ; une bascule de canal s'applique sans modale, le dernier canal
est figé et expliqué ; une bascule d'opération SMPP passe par une confirmation ; les limites de session s'enregistrent en
place ; un toast suit chaque écriture ; axe sans violation.

## Arbitrages

Fable, 2026-10-24, dans le cadre de 393 à 396 ; brouillon relu et corrigé par Fable le même jour.

403. **Routes** : `/_shell/accounts/$id` devient la mise en page (requête du compte, en-tête, onglets, `Outlet`) ;
     filles `index` (redirection), `credentials`, `channels`, `sessions`, `webhooks`. L'index redirige vers
     `credentials` si l'opérateur tient `credentials:read`, sinon vers `channels` (393), en `replace`, une fois `me`
     chargé (composant qui rend `Navigate`, rien avant ; pas de `beforeLoad`, la permission vient d'un hook). Seule
     la mise en page interroge : elle expose le compte aux filles par un contexte React (`useAccount()`), car un
     `useQuery` fille refetcherait à chaque remontage (`staleTime` 0). Sans `accounts:read`, la mise en page rend
     `Forbidden` sans `Outlet`.
404. **Onglet sans permission** : seul « Identifiants » en demande une (`credentials:read`) ; il reste rendu, figé et
     expliqué par `needs('credentials:read')` (393). `Tabs` reçoit `blockedReason?` par onglet et rend alors un
     `button` `.ui-tabs__tab` par `Explained`, sans `href` (pas de `span` focalisable, biome le refuse). L'URL
     `/credentials` tapée reste atteignable : `Credentials` y rend déjà sa phrase sans permission. Les trois autres onglets relèvent d'`accounts:read`, déjà exigé par la
     page.
405. **En-tête** : `PageHeader` avec miettes Clients › {client, lien} › {nom, mono} ; titre mono ; `status` =
     `EffectiveStatus` ; `meta` = « Compte de {client, lien} · créé le {date} · Renommer » ; actions Suspendre,
     Réactiver, Clôturer, Supprimer (les deux dernières en danger). Toast après chaque action de statut et après le
     renommage ; le focus reste sur le contrôle cliqué ou va au titre s'il disparaît, comme sur la fiche client.
     Le bouton « ← client » disparaît (le fil d'Ariane et la ligne d'en-tête le portent). « Renommer » est un petit
     bouton de la ligne méta ; son toast existe déjà (`AccountForm`). Le client n'est un lien, en miette comme en
     méta, que sous `customers:read` (388). « Modifié le » disparaît avec le bloc Identité (maquette).
406. **`Switch`** sur `Switch` de Base UI, dans `ui.tsx` : libellé, explication muette, `blockedReason` par
     `Explained`, `readOnly` et `aria-busy` pendant l'appel. Un seul `aria-describedby` : l'explication muette si la bascule est
     libre, la raison d'`Explained` si elle est figée. YAGNI : pas de variante, pas de taille.
407. **Canaux** (394) : chaque bascule envoie `PATCH /channels` avec les deux drapeaux ; au 200, la réponse remplace
     le compte dans le cache (`setQueryData`), puis toast « Canal {SMPP|REST} activé|désactivé. » ; un refus s'affiche
     dans la carte (`role="alert"`), la bascule reste à son état et le compte est invalidé pour revenir à l'état réel
     (un 422 de dernier canal après une course) ; un 422 de compte fermé rend « Ce compte est fermé
     définitivement. » Le dernier canal actif est figé : « Au moins un canal
     reste activé. »
408. **Opérations SMPP** (395) : la bascule ouvre une `Confirmation` (« Les drapeaux se lisent au bind : les binds
     ouverts du compte sont déconnectés. ») qui envoie le seul drapeau changé ; Annuler n'envoie rien et la bascule garde sa valeur
     (contrôlée par le compte) ; au 200, `invalidate={accountsKey}` et toast par `onDone`.
409. **Sessions** (396) : carte « Limites de session », formulaire en place (`max_sessions` en mono avec « Baisser
     la limite ne coupe aucun bind ouvert. » ; cases « Émetteur `transmitter` », « Récepteur `receiver` »,
     « Émetteur-récepteur `transceiver` » par `Choices`, « Au moins un type reste autorisé. », le dernier figé ;
     « Enregistrer » toujours actif ; `useSubmission` ; toast) ; aside « limite {max_sessions} » sur « Binds vivants » ; carte « Binds vivants » en état « module
     désactivé » de la maquette, sans appel, jusqu'à 025b (planifié, pas un report : pas d'issue `debt`).
410. **Figé partout** : sans `accounts:write`, `needs('accounts:write')` ; compte fermé, « Fermé définitivement. » ;
     même règle pour bascules, formulaire et actions d'en-tête, sauf « Supprimer », que seul `accounts:write` fige
     (code actuel, même règle que la fiche client).
411. **Identifiants et Webhooks** : les composants existants passent tels quels dans leur onglet ; leur refonte est la
     014c-3 (398, 399). Les modales `Settings` et `SessionsForm` et le composant `Block` disparaissent.
412. **Tests** : par onglet, rendu, axe, permissions figées, redirection de l'index selon `credentials:read`, bascule
     immédiate (corps envoyé, cache, toast, refus en place, dernier canal figé), confirmation qui n'envoie que le drapeau
     changé, formulaire de session (corps, toast), onglet figé sans `href` et raison au focus, redirection en
     `replace`, 422 en place sur une bascule ; mutation à la main ; preuve sur contabo169.

## Design arrêté

- **`web/src/ui.tsx`** : `Switch` (406) ; `Tabs` avec `blockedReason?` par onglet (404).
- **`app.css`** : `.ui-switch*` sur les jetons existants.
- **Routes** : `_shell.accounts.$id.tsx` (mise en page, en-tête 405, onglets) ; `_shell.accounts.$id.index.tsx`
  (403) ; `_shell.accounts.$id.credentials.tsx`, `.channels.tsx`, `.sessions.tsx`, `.webhooks.tsx`.
- **Canaux et opérations** : 407, 408 ; **Sessions** : 409 ; figé : 410 ; retraits : 411.
- **Aucun changement de contrat ni de Go.**
- **Tests** : 412 ; captures dans `docs/steps/014c-2/`.

## Écarts au design, constatés à l'implémentation

- **422 des canaux** (407) : l'API n'en rend qu'un, la contrainte `accounts_channel_check` (les deux canaux coupés) ;
  `closed_is_final` ne garde que le statut, aucun 422 de compte fermé n'existe sur `/channels`. Un 422 s'y dit donc
  « Au moins un canal reste activé. » ; tout autre refus garde le message de la passerelle. Les bascules d'un compte
  fermé sont figées côté écran (410). De même, `/smpp-ops` ne rend 422 que sans drapeau, ce que l'écran n'envoie
  jamais : la confirmation n'a pas de refus traduit.
- **Raison de l'onglet figé** : `needs('credentials:read', 'Cet onglet')`, « Cet onglet demande Identifiants : lire
  `credentials:read`. » ; « Cette action » ne nomme pas un onglet.
- **`Tabs` sans `params`** : un `Link` hérite des paramètres de l'adresse courante ; la prop ajoutée a survécu à sa
  mutation et est retirée.
- **Index sans garde sur `me`** : la mise en page ne rend l'`Outlet` qu'une fois `me` et le compte chargés ; la garde
  « rien avant `me` » était morte (mutant survivant) et tombe.
- **Limites de session** (409) : au 200, la réponse remplace le compte dans le cache, comme les canaux (407), plutôt
  qu'une invalidation ; l'aside « limite N » suit sans relecture.
- **Titre mono** : `PageHeader({ mono })` pose `.page-header__title--mono` (famille seule) ; `.ui-mono` ramènerait le
  titre à 12 px.
- **Focus après suppression** : `customerQuery(id)` est partagée par `customers.tsx` ; la suppression précharge le
  client avant de naviguer, sinon le `h1` provisoire « Client » prendrait le focus puis serait remplacé.
- **En-tête provisoire** (chargement, refus, échec) : « Clients › Compte », Clients en lien sous `customers:read`.
- **Textes choisis** : toasts « Compte {nom} suspendu|réactivé|fermé|supprimé. », « Canal SMPP|REST
  activé|désactivé. », « {opération} autorisé|interdit. », « Limites de session enregistrées. » ; confirmations
  « Autoriser|Interdire {opération} » ; explication muette du canal SMPP, absente de la maquette (elle le montre
  figé) : « Les binds SMPP de ce compte ne sont acceptés que si ce canal est activé. »
- **`max_sessions`** : la valeur est en mono ; le libellé reste en texte (`Field.label` est une chaîne).
- **Libellé d'une bascule** : nommé par `aria-labelledby`, il n'est pas cliquable.
- **Non fait ici** : la preuve sur contabo169 et ses captures (`docs/steps/014c-2/`), à la main de l'humain avant le
  merge.

## Mutations

À la main, une ligne à la fois, restauration vérifiée par `cmp` ; suite Vitest ciblée.

| Unité | Tués | Survivants |
|---|---|---|
| `Switch`, `Tabs` figé, `PageHeader({ mono })` (`ui.tsx`) | 9/10 | `params` de `Tabs` : retiré (écart ci-dessus) |
| Mise en page (`_shell.accounts.$id.tsx`) | 20/21 | 1 équivalent |
| Canaux et opérations | 30/30 | 4 au premier passage : `smpp_enabled` sur une bascule REST, verbe de l'action, mono de l'opération (tests ajoutés) ; `enabled[name] &&` retiré, équivalent sous la contrainte d'un canal au moins |
| Sessions | 18/18 | 6 au premier passage : relecture après enregistrement (réponse mise en cache, testée), aucun toast sur refus, mono de la valeur et des types, dernier type seul figé, `preventDefault` (tests ajoutés) |
| Index | 2/2 | la garde sur `me`, morte, est retirée |
| Onglets Identifiants et Webhooks | 2/2 | — |
| `customerQuery` | 0/1 | 1 équivalent |

**Équivalents nommés** : `readable && account.data` → `account.data` (la requête est désactivée sans
`accounts:read`, aucune donnée n'existe alors) ; la clé de `customerQuery` (partagée par ses deux lecteurs, toujours
sous `customersKey`, que les écritures invalident).

## Correctifs de revue, tour 1

Deux revues en lecture seule.

1. **Moyenne** : deux bascules de canal pouvaient partir ensemble et se contredire (`busy` ne tenait qu'un canal,
   l'autre lu du cache). Les deux bascules de canal sont tenues (`readOnly`, `aria-busy`) tant qu'un appel est en vol.
   Test : pendant l'appel SMPP, REST est tenue et un clic n'envoie rien. Mutants tués : `busy={false}`, sans
   `setBusy(true)`, sans `setBusy(false)`.
2. **Basse** : après une bascule de canal ou l'enregistrement des limites, la réponse reste mise en cache et
   `accountsKey` est invalidée, pour la fiche client et la liste Comptes. Test : la relecture est en attente, la
   bascule et « limite 0 » montrent déjà la réponse, et le compte est relu. Mutants tués : sans `setQueryData`, sans
   invalidation (canaux et sessions).
3. **Basse** : l'onglet figé de la route affichée (`/credentials` tapée sans `credentials:read`) porte
   `aria-current="page"` (`useMatchRoute`). Tests dans le kit et sur la fiche. Mutants tués : sans `aria-current`,
   `aria-current` toujours posé.
4. **Code en trop** : miette « Clients » définie une fois (`customersCrumb`).
5. **Code en trop** : `accountQuery(id)` dans `accounts.tsx`, sur le modèle de `customerQuery` ; sa clé sert la mise
   en page, les canaux et les sessions. Mutation de la clé : équivalente (partagée, toujours sous `accountsKey`).
6. **Code en trop** : constante `disconnects` en ligne.
7. **Code en trop** : `onCheckedChange={onChange}` sans relais ; le test lit le premier argument. Mutant tué :
   `onChange(false)`.
8. **Tests** : retirés, car couverts ailleurs : identifiants tapés sans permission (`credentials.test.tsx`), onglet
   Webhooks (`webhooks.test.tsx`).
9. **Tests** : « aucun toast » lit la région Notifications, qui doit exister (`getByRole`, `toHaveLength(0)`).
10. **Tests** : colonne `textContent` vide retirée des types de bind ; requêtes en double retirées.
11. **Tests** : « sends REST alone » corrigé en une boucle sur les deux canaux ; cast inutile retiré.

## Revue, tour 2

Relecture des onze correctifs : aucun défaut introduit.

## Preuve dans le navigateur

Sur contabo169, `Propriétaire` (compte de preuve), image du dashboard construite depuis la branche, fenêtre
1440 × 960 ; comptes de preuve `preuve-014c-2` et `preuve-014c-2b` créés sous Arolitec puis supprimés. Captures dans
`docs/steps/014c-2/`.

- index : `/accounts/{id}` mène à `/credentials` ; en-tête de la maquette (fil d'Ariane Clients › Arolitec ›
  `preuve-014c-2`, titre mono, pilule, « Compte de Arolitec · créé le … · Renommer », actions) ;
- Canaux : REST coupé à la barre d'espace, sans modale ; toast « Canal REST désactivé. », focus sur la bascule ;
  SMPP aussitôt figé, sa raison « Au moins un canal reste activé. » seule en `aria-describedby` ; REST rallumé ;
- Opérations SMPP : `cancel_sm` basculé, confirmation « Interdire cancel_sm » ; Annuler n'envoie rien et la bascule
  garde sa valeur ; confirmé, « cancel_sm interdit. » ;
- Sessions : `max_sessions` 4 et « Émetteur » ajouté, « Limites de session enregistrées. », aside « limite 4 » ;
  « Binds vivants » en module désactivé ;
- Webhooks : l'onglet rend le composant existant ;
- Suspendre : toast, pilule ambre, focus gardé, bascules libres ; Clôturer : Renommer, Suspendre, Réactiver,
  Clôturer et les quatre bascules figés par « Fermé définitivement. », Supprimer libre ; Supprimer : retour sur la
  fiche client, focus sur son titre, toast « Compte preuve-014c-2 supprimé. » ;
- aucun attribut `title` ; axe (WCAG 2.1 AA) sur les quatre onglets et la fiche client : aucune violation après les
  deux correctifs ci-dessous.

Deux défauts que le DOM de test ne voit pas, trouvés par la preuve et corrigés (rouge lu, puis vert) :

- `link-in-text-block` (axe, sérieux) : le lien du client dans la ligne méta ne se distinguait que par la couleur ;
  `.page-header__meta a` est souligné (`styles.test.ts`) ;
- libellé et terme d'un type de bind éclatés sur deux lignes : la grille de `.role-choice__option` plaçait le texte
  et le `span` mono dans des cellules distinctes ; ils sont enveloppés dans un seul `span` (test : le terme a un
  `span` pour parent ; rouge sans le correctif).

Reste visible, sans défaut : le champ `max_sessions` occupe la largeur de la carte, là où la maquette le borne ;
les cartes suivent la grille `.ui-cards` du kit plutôt que la colonne de 340 px de la maquette.
