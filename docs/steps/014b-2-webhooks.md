# Step 014b-2 — Webhooks

Plan : `docs/plan/composants.md` §014b, seconde des deux PR de 014b (coupe : arbitrage 341). Arbitrages : 341 à 365 dans
`docs/steps/014b-1-identifiants.md` (ceux des webhooks : 355 à 359, 360 à 363) ; suite ici. Spec :
`docs/spec/README.md` (I1, I2), `docs/spec/04-*.md` §4.4 (livraison, signature), `docs/spec/08-dashboard.md` §8.8 ;
ADR-0016. Contrat : `contracts/openapi-admin.yaml` (`list-webhooks`, `create-webhook`, `update-webhook`,
`delete-webhook`, `rotate-webhook-secret`, sous `accounts:read` et `accounts:write`). Issue : #177 (dette, grâce de rotation). Règles : `go-code.md` (un secret
rejoué est scellé par `ConfigSecrets`), `ci-deploy.md`, `migrations.md`. Mode : PR.

## Contexte

- 014b-1 (#176) a livré les identifiants, `SecretOnce` au kit et le refus de renaissance sur compte clos.
- `content-key-svc` (010b) sert `ConfigSecretsService.Seal` en gRPC mTLS ; `Callers` admet déjà `admin-api-svc → Seal`
  seul ; l'ingress de `content-key-svc` depuis `admin-api-svc` est généré, son egress ne l'est pas.
- `audit_capture()` retire `_sealed` et `_kms_key_ref` de `audit_changes`.

**Terminé quand**, contre un vrai Postgres et un `content-key-svc` en processus, puis dans le navigateur sur contabo169 :
`Propriétaire` crée un webhook MO et un DLR depuis la fiche compte, voit chaque secret une fois, modifie l'URL et le
statut, tourne le secret, supprime ; aucune réponse de liste, aucune ligne d'audit ne porte le secret ni son scellé ;
un `Seal` en échec rend 503 sans rien écrire ; un compte clos refuse ; chaque mutation est auditée sous son acteur.

## Arbitrages

Fable, 2026-10-23, sauf mention. Un brouillon de l'implémenteur a été relu, corrigé et complété par Fable.

366. **Compte clos, symétrie de 014b-1** : `account_is_closed()` en `BEFORE INSERT` sur `webhooks` → 422 sans champ,
     « Ce compte est clos ou n'existe plus. » ; `account_is_closed_revival` en `BEFORE UPDATE` quand le scellé change ou
     que le statut devient `active` → 409 par `closedConflict`. Changer l'URL, désactiver et supprimer restent permis.
     **Contrat, premier commit** : `update-webhook` et `rotate-webhook-secret` gagnent `'409'` (précédent : 337).
367. **Secret** : 32 octets `crypto/rand` en hex (64 caractères), sans préfixe. La clé HMAC du contrat public est la
     chaîne montrée : on scelle `[]byte(secret)`, que `delivery-svc` (030) ouvrira et passera tel quel au HMAC.
368. **Port `Sealer`** (`Seal(ctx, plaintext []byte) (sealed []byte, keyRef string, err error)`, la signature de
     `contentkey.ConfigSecrets.Seal`) ; adaptateur `internal/adminapi/contentkey` : `grpc.NewClient` paresseux, mTLS par
     `infra.TLS.Client()`, SAN pris de l'hôte de `CONTENT_KEY_ADDR` ; 5 s par appel, `WaitForReady` ; toute erreur → une
     ligne `Error` au seul code gRPC et `errs.ErrServiceUnavailable` ; `Close` à l'arrêt ; `/readyz` n'en dépend pas.
     `Open` hors de portée : `TestAdminAPISealsAndCannotOpen` (010b) le prouve déjà.
369. **Configuration** : `Infra.ContentKeyAddr` sans défaut, refusée vide par `validate` ; `internal/netpol` apprend
     `CONTENT_KEY_ADDR` (`<pod>:<port>` → egress `sms/<pod>:<port>`) ; `deploy/k8s/admin-api-svc.yaml` la pose ;
     `grpclog` branché comme dans `content-key-svc`.
370. **Écran** : bloc « Webhooks » sous « Identifiants », lisible avec la fiche (`accounts:read`), actions sous
     `accounts:write` : Événement (« Messages entrants (MO) », « Accusés de réception (DLR) »), URL, Statut ; Modifier
     (URL et statut, un PATCH), Rotation du secret (« L'ancien secret cesse immédiatement d'être accepté. », puis
     `SecretOnce`), Supprimer ; « Nouveau webhook » (événements absents, `type="url"`, `pattern`, 2048) figé quand les
     deux existent ou le compte est clos ; liste illisible → `Failure` ; vide → « Aucun webhook. ».
371. **Messages** : création 409 → « Ce compte a déjà un webhook pour cet événement. », 422 → « Ce compte est clos ou
     n'existe plus. » ; modification et rotation 409 → « Ce compte est clos. » ; 503 → « Le scellement est indisponible :
     rien n'a été modifié. ».
372. **`webhooks`** (`00007`) : `id`, `account_id` `ON DELETE CASCADE`, `event_type` (`mo`, `dlr`), `url` sous
     `webhooks_url_check` (motif du contrat, ≤ 2048), `status` (`active`, `disabled`), `secret_sealed bytea NOT NULL`,
     `secret_kms_key_ref text NOT NULL CHECK (<> '')` (ADR-0016), `created_at`, `updated_at`, `UNIQUE (account_id,
     event_type)` ; `diff` ; GRANT. Traduction : unicité → 409 sur `event_type`, `webhooks_url_check` → 422 sur `url`.
373. **URL : rien au-delà du contrat** ; ni DNS ni filtre d'adresse à l'admin, le contrat les place à l'envoi (030).
374. **Liste** : `ORDER BY event_type`, non paginée.
375. **`update-webhook`** : `url`, `status` en `value[T]` ; `coalesce`, filtré par `id AND account_id`, `RETURNING` la
     ligne ; `{}` → 200.
376. **`rotate-webhook-secret` sans grâce** (humain, sur recommandation) : lecture, secret neuf, `Seal`, un `UPDATE` des
     deux colonnes ; statut inchangé. La grâce que le contrat public anticipe est la dette #177 (porteur 030),
     marquée sur la requête de rotation.
377. **`delete-webhook`** : `:execrows`, 0 → 404 ; permis sur un compte clos ; la cascade sert la suppression du compte.
378. **Création** : `tx.Account` d'abord (404 avant 422 et 503), secret, `Seal`, `INSERT` ; `Sealer` par
     `huma.Config.Sealer`, composé dans `cmd/admin-api-svc`.
379. **503 et audit** : un `Seal` en échec rend 503 ; le middleware annule et écrit la ligne d'audit 503 ; aucun
     `webhooks`, aucun `audit_changes`, le log sans le secret.
380. **Postgres réel** avec un `Sealer` de test à erreur basculable : scellé en base égal au scellé du secret montré,
     `secret_kms_key_ref`, deux créations deux secrets, DTO sans `secret` ni `*_sealed` ni `*_kms_key_ref`, audit sans
     ces clés, `no-store`, 503 sans écriture.
381. **Adaptateur en processus** : serveur `contentkeygrpc.Register` sur un KMS de test, `Authorize`, `mtlstest` ;
     `Seal` transmis ; serveur éteint → `ErrServiceUnavailable` sous le délai ; autre SAN → refus ; log sans le clair.
382. **Preuve sur contabo169** : premier `Seal` réel depuis le pod ; `secret_sealed` en `vault:v1:`, `secret_kms_key_ref
     = config-secrets` ; les deux webhooks créés, modifiés, tournés, supprimés ; compte clos refusé par l'API ; audit
     sans scellé ; axe.

## Design arrêté

- **Contrat, premier commit** : `'409'` sur `update-webhook` et `rotate-webhook-secret` ; régénérer.
- **Migration `00007_webhooks.sql`** : table de 372, triggers de 366, `touch_updated_at`, `audit_capture` ; `audit.yaml` ;
  GRANT.
- **`internal/adminapi/webhooks.go`** : `Webhook`, port `Webhooks` dans `Tx`, port `Sealer`, secret de 367, création et
  rotation qui scellent avant d'écrire, `closedConflict` sur PATCH et rotation.
- **`internal/adminapi/contentkey`** : client gRPC (368), test en processus (381).
- **`internal/adminapi/postgres`** : `queries/webhooks.sql`, `fields` ; tests (380, 379, cascade, compte clos et
  suspendu, filtres de compte, `{}`, ordre).
- **`internal/adminapi/huma/webhooks.go`** : les cinq opérations, `no-store`, `value[T]`, `Config.Sealer`.
- **`cmd/admin-api-svc`**, `config.Infra`, `internal/netpol`, `networkpolicies.yaml`, `deploy/k8s/admin-api-svc.yaml`.
- **SPA** : `web/src/webhooks.tsx`, bloc de la fiche compte ; Vitest avec `axe`.
- **Dette** : #177, `debt: #177` sur la requête de rotation.
- **Tests** : 379 à 381, netpol, config ; mutation ; preuve (382).

## Écarts au design, constatés à l'implémentation

- **L'échec du scellement devient 503 dans le cœur** (révise 368) : `CreateWebhook` et `RotateWebhookSecret` répondent
  `errs.ErrServiceUnavailable` à toute erreur du `Sealer` ; l'adaptateur gRPC journalise le code et rend l'erreur telle
  quelle. Un seul endroit tient la règle, pour tout adaptateur.
- **`CONTENT_KEY_ADDR` refusée par `net.SplitHostPort`** : vide ou sans port, une seule garde ; l'adaptateur en tire le
  `ServerName` (le `tls.Config` client est cloné : le client JWKS le partage).
- **Le test de l'adaptateur résout `content-key-svc` par un résolveur `dns` manuel** (`grpc.WithResolvers`) : le nom
  et le SAN restent ceux de la production.
- **Création, PATCH et rotation rendent la ligne par `RETURNING`**, sans relecture ; la rotation lit d'abord, pour un
  404 avant tout scellement (testé : KMS en panne, webhook d'un autre compte → 404).
- **Un composant `Form`** porte la modale et le formulaire des trois actions du bloc ; `UrlField` l'URL `https`.

## Mutations

À la main, par le script de copie, mutation et restauration (`cmp`). SQL généré 5/5 ; cœur 4/4 après un test (lecture
avant rotation) ; traduction 1/1 ; migration 4/4 ; adaptateur gRPC 3/3 ; huma 2/4, deux équivalents : le motif d'URL à
la création et l'`enum` du statut au PATCH, que les CHECK de la base refusent aussi en 422 ; SPA 18/18 après un test
(rechargement après modification).
