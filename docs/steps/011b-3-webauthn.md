# Step 011b-3 — WebAuthn

Plan : `docs/plan/composants.md` §011, dernière PR de 011b. Arbitrages antérieurs : 1 à 118 (`docs/steps/011a-*.md`,
`docs/steps/011b-*.md`). Spec : `docs/spec/08-dashboard.md` §8.3, §8.8, §8.9. Contrat : `contracts/openapi-bff.yaml`
(`beginWebauthnRegistration`, `finishWebauthnRegistration`, `beginWebauthnAssertion`, `verifyMfa`,
`listWebauthnPasskeys`, `deleteWebauthnPasskey`, `enrollTotp`). Origine : `martialanouman/go-gateway-bo`,
`internal/mfa/webauthn.go`, `internal/bff/webauthn.go`, `internal/store/webauthn.go`. Règles :
`.claude/rules/tests.md`. Mode : PR.

## Contexte

- 011b-1 a posé le TOTP, les codes et la session élevée ; `verifyMfa` rend le 401 unique pour `webauthn` et `me`
  dit `passkeys: 0`. 011b-2 a posé la charte et les écrans du TOTP.
- Dettes du territoire : #3 (un défi vivant par session et par objet, sans index), #4 (rien ne purge les défis),
  #13 (construction de WebAuthn avant `net.Listen`, tenue par un commentaire), #16 (attestation non vérifiée).

**Terminé quand** : sur l'environnement de test, dans Chromium muni d'un authentificateur virtuel, un opérateur sans
facteur enregistre une clé d'accès et élève sa session avec elle ; à la connexion suivante, la clé l'élève ; depuis
`/`, il ajoute une seconde clé, en retire une, se voit refuser le retrait de la dernière ; chaque fait a sa ligne au
journal ; axe ne relève aucune violation.

## Arbitrages

Fable, 2026-10-20, sauf AA (le design entier, tel que présenté) et AB (au tour de revue), tranchés par l'humain le même jour.

119. **Une PR** (82, confirmé par l'humain, AA) : serveur et écrans ensemble, prouvés dans Chromium.
120. **Bibliothèque** : `github.com/go-webauthn/webauthn` v0.18.2, celle de l'origine ; tests par
     `github.com/descope/virtualwebauthn` v1.0.5.
121. **Partie de confiance** : `rpID` est l'hôte de `DASHBOARD_PUBLIC_URL`, l'origine admise est cette URL même, déjà
     validée pour le contrôle d'origine du BFF ; jamais la requête (§8.3). Aucune variable nouvelle. Nom affiché :
     « Tableau de bord SMS ». `webauthn.New`, qui refuse une IP, se construit avec la configuration, avant
     `net.Listen` ; un test de démarrage prouve qu'une configuration refusée n'ouvre jamais le port. Solde #13.
122. **Options** : aucune attestation demandée ni vérifiée (contrat : « no attestation preference ») ; `residentKey`
     et `userVerification` préférés ; algorithmes de la bibliothèque. Les clés déjà détenues sont exclues de
     l'enregistrement (`WithExclusions`), sous test. #16 est fermée sans objet (humain, T, en 011b-1) : aucune
     exigence de clé matérielle.
123. **Défis** : table `webauthn_challenges`, clé primaire `(session_id, purpose)`, données de cérémonie en `jsonb`.
     Ouvrir une cérémonie remplace le défi de même objet (upsert) ; la consommer le retire (`DELETE … RETURNING`).
     L'échéance (5 min) est celle de la bibliothèque (`Timeouts`, appliquée), portée par les données de cérémonie.
     Un seul défi vivant par session et par objet, par la clé : solde #3. Au plus deux lignes par session, retirées
     en cascade avec elle, que la purge retire déjà : solde #4.
124. **Clés** : table `webauthn_credentials` : identifiant (`uuidv7`, celui du contrat), opérateur, identifiant de
     l'authentificateur (unique), nom (1 à 64 caractères), compteur de signature, la `webauthn.Credential` de la
     bibliothèque en `jsonb`, date. Une clé déjà enregistrée, ici ou chez un autre, est refusée par le 422 unique.
125. **Compteur** : monotone, zéro constant admis (§8.3) ; tranché en base par un `UPDATE` conditionnel, qui refuse
     l'assertion si une autre l'a devancé.
126. **Enregistrement** : `register/begin` exige une session élevée si un facteur est en place (TOTP confirmé ou
     clé) ; `register/finish` aussi, revérifié (une clé posée entre-temps). Le défi est consommé quelle que soit
     l'issue. Sur un compte sans facteur, la clé enregistrée est en vigueur aussitôt : la cérémonie en est la
     preuve ; la session n'est pas élevée pour autant, l'élévation passe par `verifyMfa` (la SPA mène à `/mfa`).
127. **Assertion** : `assert/begin` rend 404 sans clé ; `verifyMfa` (`method: webauthn`) consomme le défi
     d'assertion, valide, avance le compteur, élève et régénère le jeton dans la transaction de 011b-1 ; tout refus
     est le 401 unique (99) et compte au compteur `mfa` (91).
128. **Retrait** : sous session élevée ; 404 pour une clé d'un autre ; 409 `mfa_last_factor` pour la seule clé d'un
     compte sans TOTP confirmé. Pas de retrait du TOTP en 011b : aucune opération ne le porte.
129. **TOTP sur un compte gardé par une clé seule** : `enrollTotp` exige une session élevée (contrat,
     `mfa_elevation_required`) et ne demande pas de preuve ; `confirmTotp` le met en vigueur. Sur `/`, « Ajouter
     l'application » remplace « Remplacer l'application » pour un tel compte.
130. **Réinitialisation** : retire aussi les clés (§8.3), dans la transaction de `UseAccessLink`.
131. **Journal** (§8.9) : `factor_enrolled` à l'enregistrement d'une clé, `factor_removed` à son retrait et à la
     réinitialisation qui en retire, `mfa` à chaque vérification par clé. L'ouverture d'une cérémonie n'est pas un
     fait.
132. **Débit** : le 429 des deux `begin` est déclaré et non servi (précédent : 96) : une ouverture remplace un défi
     et ne vérifie rien.
133. **SPA, sans bibliothèque** : `PublicKeyCredential.parseCreationOptionsFromJSON`,
     `parseRequestOptionsFromJSON` et `toJSON()`, présents dans les Chrome, Firefox et Safari courants (108) ;
     l'attestation et l'assertion passent telles quelles. Sans ces méthodes, l'écran le dit et garde les autres
     voies. Une cérémonie abandonnée (fenêtre refermée) est dite en français, sans accuser.
134. **Écrans** : `/enroll` propose la clé d'accès (nom puis « Enregistrer une clé d'accès ») avant l'application ;
     après l'enregistrement, `/mfa`. `/mfa` lit `me` : « Utiliser une clé d'accès » si le compte en a, le formulaire
     de code s'il a l'application. `/` : section « Clés d'accès » (liste, ajout nommé, retrait par clé).

135. **Élévation récente pour ajouter ou retirer un facteur** (humain, AB, au tour de revue) : un cookie élevé capturé
     enregistrait sa propre clé, puis retirait celles de l'opérateur, qui n'étaient plus le dernier facteur ; le
     contrat ne demande la preuve de l'ancien facteur qu'au remplacement du TOTP. `register/begin` et
     `register/finish` sur un compte gardé, `enrollTotp` sur un compte gardé par une clé seule, `confirmTotp` et le
     retrait d'une clé exigent une élévation de moins de 5 minutes, sinon le même 409 `mfa_elevation_required`. La
     session reste élevée jusqu'à sa fin pour le reste (88). La fenêtre qui demeure, un cookie capturé dans ces
     5 minutes, est reportée : issue #146 (`debt`, `go-live`).

## Design arrêté

- **`go.mod`** : `go-webauthn/webauthn`, `descope/virtualwebauthn` (tests).
- **`migrations/dashboard/00003_webauthn.sql`** : `webauthn_challenges`, `webauthn_credentials` ; classement d'audit
  ; GRANT du dashboard.
- **Cœur `internal/dashboard`** : `webauthn.go` (partie de confiance, cérémonies, liste, retrait) ; `mfa.go` (branche
  `webauthn` de `Verify`, règles 129 et 135) ; `session.go` (`Passkeys`, `Fresh`) ; `service.go` (`Passkeys`) ;
  `store.go` (ports).
- **`internal/dashboard/postgres`** : requêtes sqlc des défis et des clés ; `DropFactors` retire aussi les clés (130).
- **`internal/dashboard/http`** : les cinq opérations rejoignent `include-operation-ids` ; `verifyMfa` accepte
  `webauthn`.
- **`cmd/dashboard`** : construction de WebAuthn avant l'écoute.
- **`web/`** : `passkey.ts` (cérémonies natives) ; `/enroll`, `/mfa`, `/` ; classes de la charte au besoin.
- **Issues** : #3, #4, #13 fermées par la PR ; #16 fermée sans objet.
- **Tests** : Go contre Postgres réel avec `virtualwebauthn` : enregistrer puis élever ; rejeu d'une attestation ;
  défi d'assertion présenté à l'enregistrement ; défi d'une autre session ; autre origine ; clé d'un autre opérateur
  ; exclusions ; élévation requise ; compteur devancé refusé, zéro constant admis ; dernier facteur ; retrait d'une
  clé d'un autre ; réinitialisation ; journal ; compteur `mfa` et 429 ; ouverture qui remplace le défi ; démarrage
  refusé sans écoute. SPA : écrans contre `fetch` et `navigator.credentials` simulés, axe. Mutation manuelle,
  consignée.
- **Preuve sur `contabo169`**, dans Chromium par `port-forward`, authentificateur virtuel posé par CDP
  (`WebAuthn.addVirtualAuthenticator`) : le parcours du « Terminé quand », captures, journal, axe.

## Écarts au design, constatés à l'implémentation

- **`github.com/go-webauthn/webauthn` rejoint la liste de depguard du cœur** : la bibliothèque ne fait que du calcul
  (CBOR, signatures) ; le cœur reste sans I/O.
- **L'URL publique des tests de `cmd/dashboard` passe de `127.0.0.1` à `localhost`** : WebAuthn refuse un `rpID` en
  adresse IP, et c'est précisément le cas que ces tests gardent désormais (#13).
- **Les options rendues au navigateur sont celles de la bibliothèque, filtrées par les types du contrat** :
  sérialisées en JSON puis relues dans les types générés, qui écartent les champs hors schéma
  (`requireResidentKey`, `hints`).
- **La garde `CloneWarning` de la bibliothèque n'est pas reprise** : la bibliothèque n'avance pas un compteur qui
  régresse, et l'`UPDATE` conditionnel refuse alors l'assertion.
- **`/mfa` lit `me`** : il ne propose que les voies que le compte détient.

## Mutations

Soixante-cinq, à la main, sur le SQL généré, le cœur, l'adaptateur, les handlers et la SPA ; toutes tombent, sauf
deux équivalents consignés : le filtre `operator_id` du retrait en SQL (le cœur prouve la propriété avant ;
gardé en défense) et `sign_count = 0` dans la clause du zéro (un zéro n'atteint la base que si la bibliothèque l'a
admis, donc sur un compteur stocké à zéro).

## Correctifs de revue, tour 1

Trois revues (correction et sécurité ; tests creux ; code en trop). Aucun bloquant ; un constat tranché par
l'humain.

1. **Éviction par un cookie élevé capturé** (important) : arbitrage 135 (humain, AB) ; reste #146.
2. **Correction** : le défi d'assertion est consommé même par un refus (pris hors de la transaction de `Verify`) ;
   les clés sont comptées dans cette transaction, plus dans `Resolve` (une clé posée entre les deux laissait une
   assertion confirmer l'application en attente) ; une assertion ne confirme jamais l'application en attente.
3. **SPA** : tout le cache est vidé à l'élévation (les noms de clés de l'opérateur précédent restaient) ; une panne
   de la liste est dite, plus rendue vide ; le focus revient au titre après un retrait ; message du 409 :
   « reconnectez-vous ».
4. **Code en trop retiré** : la boucle qui retrouvait la clé (le compteur avance par `credential_id`) ; le booléen
   de `RemovePasskey` ; le `Forget` recopié ; un `aria-labelledby` redondant ; deux lambdas identiques ; le cas de
   test bâti sur `asCreation`, que l'absence de défi refusait seule.
5. **Tests** : revérification de `finish` ; échéance posée à l'ouverture ; deux clés, leur ordre et leurs
   compteurs ; retour du compteur à zéro ; défi d'assertion consommé par un refus ; élévation récente sur les
   quatre opérations ; nom fait d'espaces ; `me` illisible sur `/mfa` ; navigateur sans méthodes JSON.
6. **Gardé** : un retrait refusé n'a pas de ligne au journal (§8.9 nomme les retraits faits) ; la lecture de la
   liste garde le `FOR UPDATE` commun ; `/mfa` montre un instant le `me` de la connexion avant sa relecture ; la
   forme réelle des options n'est éprouvée côté SPA que par la preuve dans Chromium.

## Preuve sur `contabo169`

Jouée le 2026-10-20 depuis la branche, sur le cluster en v0.16.0 : images `migrate`, `catalog-apply` et `dashboard`
construites par GoReleaser en snapshot, importées sous `proof-011b3` ; `migrate` a appliqué la migration 3,
`catalog-apply` les GRANT (`webauthn_credentials`, `webauthn_challenges`) ; timer suspendu, puis réarmé, et dashboard
rendu à la v0.16.0, compatible avec le schéma 3 (tables seulement ajoutées). Compte de test réinitialisé par un lien
`reset` avant et après. Dans Chromium (`agent-browser`), par tunnel SSH vers `port-forward`, sur `localhost:8080`, un
authentificateur virtuel posé par CDP (`WebAuthn.addVirtualAuthenticator`, CTAP2 interne, vérification de
l'utilisateur) :

1. Connexion sans facteur : `/enroll` ([capture](011b-3/enrolement.png)) ; « Enregistrer une clé d'accès » sous le
   nom « Clé virtuelle » : `/mfa`, la clé seule proposée ([capture](011b-3/verification.png)) ; la clé élève : `/`
   ([capture](011b-3/accueil.png)).
2. Connexion suivante : `/mfa` sans champ de code ; la clé élève.
3. Depuis `/`, l'authentificateur remplacé par un neuf (Chrome n'en tient qu'un interne) : « Seconde clé » ajoutée
   ([capture](011b-3/deux-cles.png)) ; « Clé virtuelle » retirée, focus sur « Clés d'accès » ; le retrait de la
   seconde, la dernière, refusé ([capture](011b-3/dernier-facteur.png)).
4. Journal : `factor_enrolled`, `mfa`, `mfa`, `factor_enrolled`, `factor_removed`, dans cet ordre.
5. axe dans la page, `color-contrast` comprise : aucune violation sur les six états ; cent requêtes, toutes vers
   l'origine ; aucune violation de CSP.

Non prouvé dans le navigateur : une connexion élevée par la seconde clé. Le 429 de la connexion (plus de cinq
connexions de la même source en 15 minutes, preuve comprise) l'a arrêtée, puis l'authentificateur est tombé avec son
script ; le test Go `TestEachPasskeyKeepsItsOwnCounterInRegistrationOrder` couvre ce cas.
