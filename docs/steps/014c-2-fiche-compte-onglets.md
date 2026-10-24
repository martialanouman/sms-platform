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
