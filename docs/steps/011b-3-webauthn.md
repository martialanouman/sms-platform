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

Fable, 2026-10-20, sauf AA, tranché par l'humain le même jour (le design entier, tel que présenté).

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

## Design arrêté

- **`go.mod`** : `go-webauthn/webauthn`, `descope/virtualwebauthn` (tests).
- **`migrations/dashboard/00003_webauthn.sql`** : `webauthn_challenges`, `webauthn_credentials` ; classement d'audit
  ; GRANT du dashboard.
- **Cœur `internal/dashboard`** : `webauthn.go` (configuration, cérémonies, retrait) ; `mfa.go` (`webauthn` dans
  `prove`, règle 129) ; `session.go` (`Passkeys`) ; `accesslink.go` (retrait des clés) ; `store.go` (ports).
- **`internal/dashboard/postgres`** : requêtes sqlc des défis et des clés.
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
