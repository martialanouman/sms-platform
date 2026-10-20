# Step 011b-1 — TOTP, codes de récupération, session élevée

Plan : `docs/plan/composants.md` §011, première des trois PR de 011b. Arbitrages antérieurs : 1 à 81
(`docs/steps/011a-*.md`). Spec : `docs/spec/08-dashboard.md` §8.3, §8.9 ; `docs/spec/README.md` (I2, I13). Contrat :
`contracts/openapi-bff.yaml` (`enrollTotp`, `confirmTotp`, `verifyMfa`, `me`). Règles : `.claude/rules/tests.md`,
`.claude/rules/ci-deploy.md`. Mode : PR.

Ce document porte les arbitrages de **toute la 011b** (82 à 99) ; 011b-2 et 011b-3 y renvoient.

## Contexte

- Le login ouvre une session de premier facteur et émet un défi de 5 min (arbitrage 6), que rien ne consomme
  encore ; `me` rend `elevated: false` et des facteurs à zéro.
- La réinitialisation ferme les sessions, mais ne retire pas les facteurs (§8.3 l'exige) : il n'y en avait pas.
- Le catalogue d'erreurs, l'`enum` du contrat et §9.3 portent déjà les codes `mfa_*` et `webauthn_*` ; le journal
  (`CHECK` de `access_log.event`) porte déjà `mfa`, `factor_enrolled` et `factor_removed`.
- Dettes du territoire : #3, #4, #13 (WebAuthn), soldées en 011b-3 ; #16, fermée sans objet en 011b-3 (humain, T).

**Terminé quand** : sur l'environnement de test, un opérateur sans facteur se connecte, enrôle une application TOTP,
élève sa session par un code (ce qui la confirme), voit `me` élevée ; un code de récupération élève une autre session
et ne sert qu'une fois ; cinq codes faux donnent 429 ; un remplacement exige la preuve de l'ancien ; une
réinitialisation retire tout.

## Arbitrages

Fable, 2026-10-20, sauf Q à T, tranchés par l'humain le même jour. S est la proposition de l'humain, retenue contre la
recommandation de Fable (le premier facteur confirmé par `confirmTotp` élevait la session).

82. **Coupe** (humain, Q) : 011b-1, TOTP, codes de récupération et session élevée côté serveur, preuve `curl` ;
    011b-2, écrans de vérification et d'enrôlement TOTP, preuve navigateur ; 011b-3, WebAuthn serveur et écran
    ensemble (authentificateur virtuel de Chrome), dettes #3, #4, #13, #16.
83. **Clé du secret TOTP** (humain, R) : fichier `TOTP_KEY_FILE`, clé `totp-key` de 32 octets du Secret `dashboard`
    (`secrets.sh`, `ensure_key`), distincte de `cookie-key` : tourner l'une ferme des sessions, tourner l'autre
    détruirait les secrets. Chiffré : un octet de version (`1`), un nonce de 12 octets, AES-256-GCM, l'identifiant de
    l'opérateur en données associées (§8.3). La rotation par re-chiffrement est reportée (issue `debt`) ; d'ici là,
    une clé perdue laisse le lien de réinitialisation comme voie de retour. Transit est écarté : ADR 0016 le réserve
    aux secrets rejoués vers un tiers, et le BFF n'atteint OpenBao qu'en 012.
84. **Le premier facteur se confirme en servant** (humain, S) : sur un compte sans facteur confirmé, `enrollTotp` ne
    demande rien ; `verifyMfa` (`method: totp`) accepte alors le code de l'application **en attente**, ce qui la
    confirme et élève la session dans la même transaction. Un compte qui a un facteur confirmé n'accepte jamais
    l'application en attente à `verify`. Les codes de récupération naissent à l'enrôlement et ne valent qu'une fois
    leur application confirmée. `confirmTotp` ne sert qu'au remplacement, sous session élevée, comme le dit le
    contrat. Un défi échu entre l'enrôlement et le premier code demande une nouvelle connexion ; l'enrôlement en
    attente survit. Le contrat reçoit deux retouches de description (`enrollTotp`, `verifyMfa`), sans changer de
    forme.
85. **Remplacement** : `enrollTotp` sur un compte gardé exige la preuve du facteur en place (`method` et `code`, TOTP
    ou code de récupération), faute de quoi 409 `mfa_already_enrolled` ; une preuve refusée, 409
    `mfa_replacement_refused`, et compte au compteur (91). La spec demande une session élevée pour remplacer ; le
    contrat (004c-3), plus strict, exige la preuve de l'ancien, et l'élévation au `confirm` : le code suit le contrat.
    `confirmTotp` rend l'application en attente seule active, avec ses codes, et retire l'ancienne et ses codes, en
    une transaction.
86. **TOTP** par la bibliothèque standard (RFC 6238 : HMAC-SHA1, 6 chiffres, pas de 30 s, secret de 20 octets),
    prouvé par les vecteurs de l'annexe B. Fenêtre ±1 pas ; anti-rejeu par le dernier pas consommé (`last_step`),
    accepté seulement au-delà. Le pas courant vient de la base (arbitrage 26). URI
    `otpauth://totp/<émetteur>:<e-mail>?secret=…&issuer=…`, émetteur « Tableau de bord SMS ».
87. **Codes de récupération** : dix codes de dix caractères base32 (50 bits), gardés en SHA-256 (`Digest`, comme les
    jetons ; argon2 coûterait dix hachages par tentative), à usage unique ; la saisie est normalisée (majuscules,
    espaces et tirets retirés).
88. **Session élevée** : colonne `sessions.elevated_at`, posée par `verify`, valable jusqu'à la fin de la session
    (ni la spec ni le contrat ne redemandent le facteur). `verify` consomme le défi, régénère le jeton (fixation de
    session) et pose `elevated_at` dans un seul `UPDATE` ; le défi n'est consommé qu'au succès. Le 204 porte le
    nouveau cookie.
89. **Aucun accès n'est fermé à une session non élevée en 011b** : la règle de §8.3 est « MFA obligatoire selon le
    rôle », et les rôles arrivent en 012, qui posera `mfa_required`. L'arbitrage 46 (« 011b l'ajoute avec
    `Elevated` ») se lit ainsi : 011b pose `Elevated` et le rend dans `me`. La SPA (011b-2) envoie un compte sans
    facteur à l'enrôlement, les autres à la vérification.
90. **`me`** : `elevated`, et `secondFactors` (`totp` : une application confirmée ; `recoveryCodesRemaining` : ses
    codes non consommés ; `passkeys` : 0 jusqu'en 011b-3).
91. **Anti-devinette** : les compteurs de connexion (`login_attempt_counters`, `Reserve` et `Forget`) sous un domaine
    `mfa` de `Keys`, le sujet étant l'opérateur ; cinq échecs en 15 min donnent 429 et `Retry-After`. Ils servent
    `verify` et la preuve de remplacement ; un succès oublie le compteur. Aucune table.
92. **Enrôlement en attente** : un seul par opérateur, remplacé par un nouvel `enroll`, sans échéance ni purge
    propre ; il meurt avec son remplacement, sa confirmation ou la réinitialisation. `enroll` n'est pas un fait du
    §8.9 et ne se journalise pas.
93. **Réinitialisation** : `UseAccessLink` retire aussi les facteurs et leurs codes (§8.3), et journalise
    `factor_removed` quand il y en avait.
94. **Journal** (§8.9) : `mfa`, succès ou échec, à chaque `verify` qui atteint un opérateur ; `factor_enrolled` à la
    confirmation (par `verify` ou `confirmTotp`) ; `factor_removed` au remplacement et à la réinitialisation.
95. **I2** : le secret, l'URI et les codes ne sortent qu'une fois, dans la réponse d'`enroll` ; ni journal, ni erreur,
    ni trace ne les portent.
96. **Surface HTTP** : `enrollTotp`, `confirmTotp`, `verifyMfa` (méthodes `totp` et `recovery_code` ; `webauthn`
    rend le 401 unique jusqu'en 011b-3) rejoignent `include-operation-ids`. Le 503 de `enrollTotp` et `verifyMfa`
    est déclaré et non servi : aucun argon2 n'y tourne (précédent : arbitrage 36).
97. **Base** : migration `00002` ; tables `totp_factors` (identifiant, opérateur, chiffré, `last_step`,
    `confirmed_at`, `created_at` ; au plus une confirmée et une en attente par opérateur, par index partiels) et
    `recovery_codes` (application, digest unique, `used_at`) ; colonne `sessions.elevated_at`. GRANT du rôle du
    dashboard et classement d'audit avec la migration.
98. **`Cache-Control: no-store`** sur toute réponse de `/api` (constaté à l'implémentation) : l'enrôlement rend un
    secret (I2), et aucune réponse du BFF n'est cachable.
99. **Un seul corps pour tout refus de `verifyMfa`** (constaté à l'implémentation ; le contrat le dit, « or no session
    lives ») : le garde de session rend, pour cette opération seule, le corps du refus du second facteur, et non
    « Aucune session ouverte. ».

## Design arrêté

- **Contrat** : descriptions d'`enrollTotp` et de `verifyMfa` (84) ; régénération des types Go et TypeScript.
- **`migrations/dashboard/00002_mfa.sql`**, son classement dans `migrations/dashboard/audit.yaml`, la déclaration de
  GRANT du dashboard.
- **Cœur `internal/dashboard`** :
  - `totp.go` : code d'un pas, fenêtre et anti-rejeu, URI, secret ; vecteurs de la RFC ;
  - `cipher.go` : scellement et ouverture du secret sous `TOTP_KEY_FILE`, l'opérateur en données associées ;
  - `mfa.go` : `EnrollTotp`, `ConfirmTotp`, `Verify` ; codes de récupération ;
  - `session.go` : `Current.Elevated` et les facteurs ; `accesslink.go` : retrait des facteurs ;
  - `keys.go` : sujet `mfa` ; `store.go` : les ports nouveaux.
- **`internal/dashboard/postgres`** : requêtes sqlc des facteurs, des codes, de l'élévation.
- **`internal/dashboard/http`** : les trois opérations, `me` complet, le nouveau cookie au 204 de `verify`.
- **`cmd/dashboard`** : `TOTP_KEY_FILE` ; **`deploy/`** : clé `totp-key` (`secrets.sh`), montage dans
  `dashboard.yaml`.
- **Issue `debt`** : rotation de `totp-key` par re-chiffrement, marquée `debt: #N` où la clé se charge.
- **Tests** : vecteurs RFC 6238, fenêtre et rejeu ; scellement (données associées d'un autre opérateur refusées,
  version inconnue refusée) ; cas d'usage contre Postgres réel (premier facteur confirmé par `verify`, application
  en attente refusée quand un facteur est confirmé, code de récupération à usage unique et refusé avant
  confirmation, remplacement avec et sans preuve, compteur et 429, défi consommé au seul succès, jeton régénéré,
  réinitialisation qui retire tout, lignes du journal) ; HTTP (codes et corps du contrat, cookie régénéré, `me`) ;
  I2 (ni secret ni code dans les journaux). Mutation manuelle, consignée.
- **Preuve sur `contabo169`**, par `port-forward` et `curl` : login, `enroll`, `verify` avec un code calculé depuis
  le secret, `me` élevée ; code de récupération sur une seconde session, puis rejoué et refusé ; cinq codes faux,
  429 ; remplacement sans preuve refusé, avec preuve accepté puis confirmé ; réinitialisation, facteurs retirés.

## Écarts au design, constatés à l'implémentation

- **La clé TOTP** se range comme celle du cookie, en 64 caractères hexadécimaux (`openssl rand -hex 32`) ; le démarrage
  la décode en 32 octets et refuse tout autre contenu.
- **Une confirmation réussie oublie le compteur** : le test du remplacement l'a montré (sans l'oubli, les échecs
  passés menaient au 429 au premier faux pas suivant).
- **Le verrou du facteur** (`FOR UPDATE`) se prouve par une transaction du test qui tient la ligne pendant que deux
  vérifications du même code attendent : sans lui, les deux passent.
- **G117, G401 et G505** de gosec sont exclus par chemin dans `.golangci.yml` : la réponse d'enrôlement porte le secret
  par contrat, et TOTP est HMAC-SHA1 par la RFC.
- **Le test du binaire** enrôle un facteur : le câblage de la clé n'était prouvé par rien d'autre.

## Mutations

Une soixantaine, à la main, consignées au fil des commits : TOTP (14), scellement (9), cœur (25), SQL généré (12),
HTTP (16), démarrage (6). Toutes tombent, sauf `len(key) < 32` dans `NewTotpKey`, équivalente : `aes.NewCipher` refuse
de lui-même toute longueur au-delà de 32 octets.

## Preuve sur `contabo169`

Jouée le 2026-10-20 depuis la branche, sur le cluster en v0.14.0 : images `migrate`, `catalog-apply` et `dashboard`
construites par GoReleaser en snapshot, importées sous `proof-011b1` ; `secrets.sh` de la branche a ajouté `totp-key` ;
`migrate` a appliqué la migration 2, `catalog-apply` les GRANT ; timer suspendu, puis réarmé, et dashboard rendu à la
v0.14.0, compatible avec le schéma 2. Par `port-forward`, un script Python qui calcule les codes :

1. Enrôlement : 200, dix codes, URI `otpauth://totp/`, `Cache-Control: no-store` ; `me` non élevée, aucun facteur.
2. Premier code : 204, cookie régénéré, l'ancien refusé ; `me` élevée, TOTP, dix codes.
3. Remplacement sans preuve : 409 `mfa_already_enrolled` ; avec un code de récupération : 200 ; confirmation : 204.
4. Code de récupération de l'ancien facteur : 401 ; du nouveau, en minuscules : 204, neuf restants ; rejoué : 401.
5. Quatre codes faux de plus : 401 ; la sixième tentative, code juste : 429 et `Retry-After`.
6. Réinitialisation par un lien de Mailpit : 204 ; à la connexion suivante, aucun facteur.
7. Journal : `factor_enrolled` et `mfa` au premier code ; `factor_removed` puis `factor_enrolled` au remplacement ;
   `mfa` en échec à chaque refus ; `factor_removed` puis `access_link_used` à la réinitialisation.

Rouge vu : la première passe rendait 500 sur `enroll` et `me` (`permission denied for table totp_factors`). Seul
`migrate` avait été joué, alors que les GRANT viennent de `catalog-apply`, joué après lui par `deploy.sh`. Rejoué dans
l'ordre du script.

## Correctifs de revue, tour 1

Trois revues (correction et sécurité ; tests creux ; code en trop). Aucun bloquant ; 29 constats.

1. **Le compteur ne s'oublie qu'après une preuve vérifiée** (arbitrage 91) : l'oubli vit dans `prove`, sur sa seule
   branche de succès. Une preuve offerte à l'enrôlement d'un compte sans facteur confirmé l'effaçait sans rien prouver,
   ce qui aurait ouvert un trou en 011b-3 (compte gardé par une seule passkey).
2. **Un enrôlement en attente se remplace par un upsert** sur son index partiel, ses codes recréés : deux enrôlements
   concurrents sur un compte sans ligne heurtaient l'index (500), le `FOR UPDATE` ne verrouillant rien d'absent.
3. **Retraits** : la sentinelle `errRefused` (le refus est `ErrUnauthenticated`), `MethodWebauthn` (011b-3 l'apportera),
   l'`ORDER BY` de `Factors`, deux commentaires de champ.
4. **Tests** : la réinitialisation retire aussi l'application en attente (sans quoi elle élèverait le compte vidé de
   facteur confirmé) ; double enrôlement ; queue exacte du journal au remplacement ; `webauthn` sur un défi frais ;
   code faux de sept chiffres, sans `t.Skip` ; franchissement de pas toléré au premier code ; aucune ligne
   `factor_removed` sans facteur ; journal applicatif sans secret ni code ; enrôlement en attente qui survit à une
   nouvelle connexion (arbitrage 84).
5. **Gardés tels quels** :
   - `created_at`, nommé par l'arbitrage 97 ;
   - la double résolution de session d'`EnrollTotp` et `ConfirmTotp`, qui garde au cœur une signature par cookie ;
   - `CurrentStep` en port distinct ;
   - le `totpAt` du test HTTP, oracle indépendant du code de production.
6. **Fenêtre de déploiement progressif**, acceptée : un pod de la version précédente qui traite une réinitialisation
   ferme les sessions sans retirer les facteurs ; l'opérateur refait le lien. Le schéma reste compatible dans les deux
   sens.

Mutations du tour : neuf, toutes tombent (dont l'ordre des lignes du journal, le retrait limité aux facteurs
confirmés, l'upsert et la recréation des codes).
