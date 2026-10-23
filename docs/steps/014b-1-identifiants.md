# Step 014b-1 — Identifiants

Plan : `docs/plan/composants.md` §014b, première des deux PR de 014b (coupe : arbitrage 341). Arbitrages antérieurs : 1 à
340 (`docs/steps/014a-*.md`). Ce document porte ceux de toute la 014b. Spec : `docs/spec/README.md` (I1, I2, I13),
`docs/spec/01-exigences.md` §1.2, `docs/spec/07-plan-de-controle.md` §7.1 à §7.3, `docs/spec/08-dashboard.md` §8.8 ;
ADR-0006, ADR-0016. Contrat : `contracts/openapi-admin.yaml` (tag `credentials`). Issue : #173 (amendée). Règles :
`.claude/rules/go-code.md` (« Un secret : le vérifie-t-on ou le rejoue-t-on ? »), `tests.md`, `migrations.md`. Mode : PR.

## Contexte

- 014a (#172, #174, #175) a livré groupes, clients et comptes, la fiche compte en quatre blocs et le motif « clos refuse
  un enfant neuf » (327).
- `audit_capture()` retire déjà de `audit_changes` toute colonne en `_hash`, `_sealed` ou `_kms_key_ref` (006a).
- Le hachage argon2id existe dans `internal/dashboard/password.go`, le jeton haché sha256 dans `dashboard.Digest`.
- La modale « montré une fois » existe pour le jeton personnel (`_shell.account.tsx`).

**Terminé quand**, contre un vrai Postgres puis dans le navigateur : `Propriétaire` crée, tourne (avec et sans grâce),
désactive, réactive et révoque la clé d'API et le bind d'un compte depuis sa fiche, chaque secret montré une fois et
vérifiable contre son haché ; aucune réponse de liste, aucune ligne d'audit, aucun log ne porte un secret ni un haché ;
un compte clos refuse (422) ; un type ou un `system_id` pris refuse (409) ; chaque mutation est auditée sous son acteur.

## Arbitrages

Fable, 2026-10-23, sauf mention. Les décisions humaines du même jour sont marquées.

341. **Coupe en deux PR verticales** (humain, sur recommandation) : **014b-1, identifiants** (table `credentials`,
     hachage, cinq opérations, bloc « Identifiants », modale « montré une fois » partagée) ; **014b-2, webhooks** (table
     `webhooks`, port `Sealer`, client gRPC vers `content-key-svc`, variable et egress, cinq opérations, bloc
     « Webhooks »). 014b-2 porte la première sortie réseau d'`admin-api-svc` et sa propre preuve.
342. **`credentials`** (`00006`) : `id`, `account_id` → `accounts` **`ON DELETE CASCADE`** (humain), `type` (`smpp_bind`,
     `api_key`), `system_id` (présent ssi bind, `^[\x21-\x7E]{1,15}$`), `status` (`active`, `disabled`, `revoked`),
     `secret_hash`, `previous_secret_hash`, `grace_expires_at`, `last_used_at`, `rotated_at`, `created_at`,
     `updated_at`. `UNIQUE (account_id, type)` (ADR-0006), `UNIQUE (system_id)` (un bind arrive sans compte),
     `credentials_revoked_check` (révoqué ssi haché nul), grâce cohérente (ancien haché ssi échéance). `audit: diff`,
     GRANT `admin-api-svc` complet.
343. **Les hachés n'entrent jamais dans `audit_changes`** : leurs noms tombent sous `_hash$` ; un test le prouve pour
     `credentials` après création et rotation.
344. **Deux hachages** (humain, sur recommandation) : clé d'API = sha256 hex d'un aléa de 256 bits (vérifiée à 8 000/s
     en 021) ; mot de passe de bind = argon2id PHC (≤ 8 octets en SMPP 3.4 ; ADR-0016). Une colonne `secret_hash`, le
     `type` dit le format.
345. **argon2id sort vers `internal/platform/argon2id`** (`Hash`, `Verify`, paramètres de `password.go`) ; le dashboard
     le consomme.
346. **Formats** : clé d'API `smsgw_` + 32 octets base64url ; aucun préfixe par clé en base ni au contrat. Mot de passe de
     bind : 8 caractères `[A-Za-z0-9]` par `crypto/rand`. `system_id` choisi par l'exploitant.
347. **Montré une fois** : création et rotation rendent `CredentialWithSecret` sous `Cache-Control: no-store` ; la liste
     sert un DTO sans secret ni haché. Le secret ne vit que dans le handler, la réponse et la modale.
348. **Rotation et grâce dans la ligne** : grâce absente, `null` ou `0` → aucune ; sinon l'ancien haché et l'échéance
     sont posés ; `rotated_at` ; un désactivé le reste, un révoqué redevient actif, toujours sans grâce. La grâce
     s'éteint chez le vérificateur (021, 025b) par comparaison à `now()`. Un seul `UPDATE … RETURNING`.
349. **Révocation** (humain) : `revoked`, hachés et échéance effacés, 204, idempotente.
350. **Révoqué définitif sauf rotation**, tenu par `credentials_revoked_check` : `update-credential-status` sur un révoqué
     → 409 (« Révoqué : seule une rotation le ravive. »). `disabled` ↔ `active` : 200, idempotent.
351. **Compte clos refuse un identifiant et un webhook neufs** (humain, symétrie de 327) : trigger `account_is_closed()`
     `BEFORE INSERT`, `FOR SHARE` → 422 sur `account_id`, « Ce compte est clos ou n'existe plus. ». Suspendu : accepté.
352. **Déconnexion des binds** (désactivation d'un bind, révocation, rotation sans grâce) : non émise, dette #173 amendée
     pour nommer ces opérations, sans marque de plus (336) ; 503 déclaré, non émis.
353. **`last_used_at` servi `null`**, planifié : aucun vérificateur n'existe ; 021 décidera de son écrivain (inscrit au
     plan §021).
354. **Refus de création** : type présent → 409 sur `type` (« Ce compte a déjà un identifiant de ce type. ») ;
     `system_id` pris → 409 sur `system_id` (« Ce system_id est déjà pris. ») ; `system_id` absent pour un bind ou
     présent pour une clé → 422 sur `system_id`.
355. **`webhooks`** (014b-2, `00007`) : `event_type` (`mo`, `dlr`), `url` https ≤ 2048, `status`, `secret_sealed`,
     `secret_kms_key_ref` non vide, `UNIQUE (account_id, event_type)`, `CASCADE`, `diff`.
356. **Scellement par `ConfigSecrets.Seal` de `content-key-svc`** (livré en 010b) : port `Sealer`, adaptateur gRPC mTLS,
     `CONTENT_KEY_ADDR`, egress généré par `internal/netpol`. Aucun report.
357. **Sceller avant d'écrire**, dans la transaction ; toute erreur gRPC → 503, rien n'est écrit.
358. **`admin-api-svc` n'ouvre jamais un secret** : `Open` hors de sa carte ; un test le prouve.
359. **Preuves d'I2 et I1** : DTO sans `secret`, `*_hash`, `*_sealed` (décodé en `map`) ; `audit_changes` sans ces clés ;
     ligne d'audit et log d'un échec sans le secret ; le secret rendu vérifie le haché ; deux créations, deux secrets ;
     la modale fermée, le secret n'est plus dans le DOM ni dans le cache de requêtes ; `no-store` pincé.
360. **Écrans** : bloc « Identifiants » sous « Opérations SMPP » de `/accounts/$id` (sous `credentials:read`, sinon une
     phrase) : une ligne par identifiant (Clé d'API ; Bind SMPP et `system_id`), Statut, Grâce, Dernier usage,
     Rotation le ; actions Rotation (grâce : aucune, 1 h, 24 h, 7 jours), Désactiver ou Activer, Révoquer ; « Nouvel
     identifiant » (type, `system_id` pour un bind), figé quand les deux types existent. « Webhooks » en 014b-2.
361. **`SecretOnce`** extrait de `_shell.account.tsx` vers le kit : secret en `<code>`, « Copier le secret », « J'ai
     copié le secret », « Ce secret ne sera plus jamais affiché. ». Aucune action « révéler » (I2).
362. **Figés et expliqués** : compte clos → tout figé, « Clos définitivement. » ; révoqué → statut figé « Révoqué : seule
     une rotation le ravive. » ; désactivé → la rotation dit qu'il le reste ; sans `credentials:write` → bulle.
363. **Trois états** : `update-credential-status` exige `status` ; `rotate-credential` a un corps facultatif,
     `grace_period_sec` nullable ; `update-webhook` en `value[T]` (014b-2).
364. **Pas de contexte `internal/credentials`** (301) : `internal/adminapi` porte entités, règles et génération ; le
     vérificateur vient avec 021 et 025b, par l'instantané (qui garde `_hash`).
365. **Contrat inchangé** ; le motif de `system_id` vit en base (durcir une requête est une rupture).

## Design arrêté

- **Migration `00006_credentials.sql`** : table de 342, `account_is_closed()` (351), `touch_updated_at`,
  `audit_capture` ; `audit.yaml` ; GRANT déclaré puis régénéré.
- **`internal/platform/argon2id`** : `Hash`, `Verify` ; `dashboard.Hasher` les appelle.
- **`internal/adminapi/credentials.go`** : `Credential`, génération des secrets (346), règles de rotation et de
  révocation (348 à 350), port `Credentials` dans `Tx`.
- **`internal/adminapi/postgres`** : `queries/credentials.sql` (liste par compte, lecture, insertion, un `UPDATE` par
  transition) ; traduction de 350 et 354.
- **`internal/adminapi/huma/credentials.go`** : les cinq opérations ; DTO sans secret ; `no-store`.
- **SPA** : `SecretOnce` au kit ; `web/src/credentials.tsx` ; bloc « Identifiants » de la fiche compte.
- **Dette et plan** : #173 amendée (352) ; plan §021 (353).
- **Tests** : Postgres réel sous `admin_api_svc` (chaque opération, unicités, CHECK, compte clos, transitions, hachés
  vérifiés, cascade, audit sans haché, DTO sans secret, `no-store`) ; cœur sans base (transitions, formats) ; Vitest avec
  `axe`. Mutation manuelle ; preuve sur contabo169.

## Écarts au design, constatés à l'implémentation

- **Les transitions vivent dans la requête** (révise 364) : `RotateCredential` est un `UPDATE` à `CASE` (ancien haché et
  échéance seulement avec une grâce, révoqué remis `active`), `RevokeCredential` efface les trois colonnes ; aucune
  table de transitions en Go (301 : les règles en base).
- **Aucun contrôle de `system_id` dans le cœur** : le CHECK `credentials_system_id_check` refuse déjà un bind sans
  `system_id` et une clé avec, en 422 sur ce champ ; le contrôle Go était un mutant survivant.
- **Compte clos : 422 sans champ** (révise 351) : `account_id` vient du chemin, et la traduction préfixe `body.` ; le
  message suffit, comme pour `accounts_channel_check`.
- **`CopyButton` passe au kit avec `SecretOnce`** : `ui.tsx` ne peut importer `factor.tsx`, qui importe le kit.
- **Un `Select` « Type »** ne propose que les types absents ; le 409 de création se dit selon le type choisi (« Ce
  system_id est déjà pris. » pour un bind ; le type pris ne survient qu'en course).
- **Désactiver et Activer passent par une confirmation** (le journal d'audit le dit) ; Révoquer dit que le `system_id`
  reste réservé au compte.

## Mutations

À la main, par le script de copie, mutation et restauration (`cmp`). `argon2id` 8/8 (un test de sel trop court
ajouté) ; SQL généré 12/12, un équivalent retiré (la garde de statut sur l'ancien haché : un révoqué n'en a pas) ;
traduction 3/3 ; adaptateur 2/2 ; cœur 7/7, le contrôle de `system_id` retiré ; migration 3/4, survivant équivalent :
la borne `{1,15}` du motif (huma borne à 15 avant la base, qui la garde en filet) ; huma 6/6 après un test (`status:
revoked` refusé au PATCH). Un premier passage filtrait les tests par un `-run` qui omettait celui de la rotation : les
mutants de la rotation ont été rejoués. SPA 16/16 après deux tests (activation, grâce expirée).

## Correctifs de revue, tour 1

Trois revues (correction et sécurité ; tests creux ; code en trop).

1. **Un compte clos ranimait un identifiant par l'API** (moyenne, prouvé contre Postgres) : rotation ou activation
   rendaient un secret neuf ou un statut actif sur un compte clos ; seul l'écran figeait. La base refuse désormais,
   sous le même verrou : trigger `account_is_closed_revival` `BEFORE UPDATE`, quand le haché change pour un nouveau ou
   que le statut devient `active` → 409 (révise 351 pour ces deux chemins). Révoquer et désactiver restent permis.
2. **Bombe à retardement** : une échéance de grâce de test au 24/10/2026, comparée à l'horloge réelle ; portée en 2099.
3. **Filtres de compte prouvés** : l'`UPDATE` de statut rend la ligne entière (plus de relecture qui masquait son
   filtre), la rotation ne filtre plus que par sa lecture préalable ; PATCH et révocation par un autre compte → 404,
   testés.
4. **Tests ajoutés** : compte suspendu accepté, grâce négative refusée, deux mots de passe de bind distincts, ordre de
   la liste, préfixe et clé trop longue refusés par `argon2id` ; côté SPA, rechargement après création, date de
   rotation, type pris, rotation d'un révoqué, phrases conditionnelles, statut désactivé, liste en échec.
5. **Identifiants illisibles** : le bloc montre `Failure` et « Réessayer », plus « Aucun identifiant. ».
6. **Code en trop retiré** : l'entrée de traduction `credentials_revoked_check` (le 409 passe par `closedConflict`),
   l'export de `newSecret` et sa constante, la déstructuration du secret (le type `CredentialWithSecret` du contrat
   suffit), une invalidation écrite deux fois.
7. **Gardé** : la grâce en `time.Duration` dans le cœur ; `Dummy` sans preuve de coût (défaut antérieur à 014b, un test
   de temps serait fragile) ; « Ses binds ouverts sont déconnectés. » suit le contrat, comme en 014a-3 (#173).
