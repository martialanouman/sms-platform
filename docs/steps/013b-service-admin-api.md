# Step 013b — `admin-api-svc` : service et déploiement

Plan : `docs/plan/composants.md` §013, deuxième des quatre PR de 013 (coupe : arbitrage 229). Arbitrages antérieurs : 1
à 238 (`docs/steps/013a-coeur-de-la-passerelle.md`). Modèles : `docs/steps/010b-2-content-key-svc-service.md`,
`docs/steps/012c-2-*.md`. Spec : `docs/spec/07-plan-de-controle.md` §7.3, §7.7 ; `docs/spec/10-exploitation.md` §10.1,
§10.2. Décision : `docs/design/06-confiance-interne.md` §4, §7. Règles : `.claude/rules/ci-deploy.md`, `go-code.md`,
`tests.md`. Mode : PR.

## Contexte

- 013a (#164) a livré `internal/adminapi` et ses adaptateurs `jwks`, `postgres`, `huma` ; aucun `cmd/`.
- Le dashboard sert son JWKS à `/.well-known/jwks.json` sur `INTERNAL_ADDR` (8443), sous mTLS, à `admin-api-svc`
  seul (`dashboardhttp.JWKSCallers`) ; son `iss` est `DASHBOARD_PUBLIC_URL` normalisé par `origin()` de `cmd/dashboard`.
- `dashboard-tls` n'a que l'usage `server auth`.
- `catalog-apply` projette déjà `admin-api-svc/postgres-password` ; `secrets.sh` crée le Secret `admin-api-svc` sans
  `postgres-url`.
- Relevé en revue de 013a : l'émetteur vide, le `Log` du JWKS, la composition `jwks` + vérificateur + huma et « rien
  n'est lu au démarrage » se prouvent ici.

**Terminé quand** : la release déploie `admin-api-svc` sur `contabo169` et son `/readyz` y répond ; un jeton personnel
`audit:read` lit `/admin/audit-log` sous mTLS depuis un pod au SAN `dashboard`, et le second appel montre la ligne du
premier ; un jeton sans `audit:read` rend 403 et laisse sa ligne ; un SAN non admis est refusé au handshake, en JSON ;
un pod non étiqueté prend `connection refused`.

## Arbitrages

Fable, 2026-10-21, sauf mention. 242 et 241 confirmés par l'humain le même jour.

239. **Pas de coupe** : un `cmd/`, un manifest, un Certificate, un cas `netpol`, deux compteurs et leurs règles ; une
     alerte entre avec le composant qui émet sa métrique.
240. **Écouteur** : `INTERNAL_ADDR=:8443`, seul écouteur, sous mTLS ; `TLS_ALLOWED_CLIENTS` égal à
     `adminapihuma.Callers = ["dashboard"]`, exigé au démarrage et par la garde (design 06 §4).
241. **Émetteur** (humain) : la passerelle lit `DASHBOARD_PUBLIC_URL`, normalisé par `origin()`, déplacé de
     `cmd/dashboard` vers `internal/platform/config`. Vide ou invalide → démarrage refusé. La garde de `deploy/` exige la
     même valeur dans les deux manifests : un `iss` divergent serait un 401 silencieux pour tous.
242. **JWKS** : `JWKS_URL=https://dashboard:8443/.well-known/jwks.json`, `https://` exigé ; client
     `TLSClientConfig: infra.TLS.Client()`, le SAN `dashboard` vérifié par Go depuis l'hôte de l'URL. `netpol` ajoute la
     sortie vers les pods `app.kubernetes.io/name: <hôte>` de `sms` sur le port. Certificate `admin-api-svc-tls`
     (`server auth`, `client auth`). **`client auth` ajouté à `dashboard-tls` dès 013b** (humain) : la preuve appelle avec
     le vrai certificat du dashboard ; 013c n'écrit que le client Go.
243. **Journaux** : `jwks.Keys{Log: log}` ; `slog.SetDefault(log)` (le middleware journalise par le slog de paquet) ;
     `ErrorLog` du serveur en slog, comme le dashboard.
244. **Postgres par le Pooler** (mode `transaction`) : `set_config(..., true)` vit dans la transaction de
     `pgx.BeginFunc` ; `Record` est un `INSERT` autocommit. `postgres-url` posée par `secrets.sh`. Repli
     `default_query_exec_mode=exec` si la preuve rougit.
245. **Démarrage refusé** sur `INTERNAL_ADDR` vide, allowlist ≠ `Callers`, `JWKS_URL` non `https://`,
     `DASHBOARD_PUBLIC_URL` invalide, `POSTGRES_URL` ou fichiers TLS vides ; puis `pgdial.Retry` sur un `Ping`. Rien n'est
     lu du JWKS au démarrage (232).
246. **`/readyz`** sans dépendance, drain seul : JWKS et Postgres se voient par leurs refus et leurs alertes (spec 07 §7.7).
247. **Métriques** sans label : `admin_api_uncommitted_calls_total` (chaque réponse 503 d'un appel non commis) et
     `admin_api_jwks_reload_failures_total`. Alertes `deploy/alerts/admin-api.yaml` : `AdminCallsUncommitted`
     (`increase[5m] > 0`, sans `for`), `JWKSReloadFailing` (`increase[10m] > 0`).
248. **Manifest** : ServiceAccount, Service ClusterIP 8443, Deployment à une réplique (pas de PDB, spec 10 §10.2), grâce
     125 s, sondes sur `ops`, un volume `admin-api-svc-tls` ; GoReleaser ; `deploy.sh` attend le certificat et ajoute
     le service à sa boucle.
249. **Preuve** : image snapshot `proof-013b`, timer suspendu ; l'humain émet le jeton personnel (session élevée) ; pod
     étiqueté `dashboard` montant `dashboard-tls`, sortie temporaire vers 8443 hors dépôt. Après la release, le paquet
     GHCR `admin-api-svc` est rendu public (humain).
250. **Dettes** : #43 (cette part payée par `ErrorLog`, reste `rest-api-svc`) ; #70 gagne `admin-api-svc` comme
     porteur ; #76, #93, #122, #162, #163 intactes.

## Design arrêté

- **`internal/platform/config`** : `JWKS_URL` ; `Origin` (sorti de `cmd/dashboard`).
- **`internal/adminapi/huma`** : `Callers` ; compteur des appels non commis dans `Config`.
- **`internal/adminapi/jwks`** : compteur des échecs de rechargement.
- **`cmd/admin-api-svc/main.go`** : `run(ctx, infra, log)` (245), `slog.SetDefault`, pool, JWKS, vérificateur, huma,
  ops, superviseur.
- **`internal/netpol`** : sortie de `JWKS_URL` ; `networkpolicies.yaml` régénéré.
- **`deploy/k8s/admin-api-svc.yaml`**, **`deploy/bricks/ca.yaml`** (`admin-api-svc-tls` ; `client auth` de
  `dashboard-tls`), **`deploy/bricks/secrets.sh`**, **`deploy/test-env/deploy.sh`**, **`.goreleaser.yaml`**,
  **`deploy/alerts/admin-api.yaml`** et son test, **`deploy/guard_test.go`** (allowlist, `DASHBOARD_PUBLIC_URL` égal).
- **Tests** : `cmd/` sur le câblage réel (chaque refus de démarrage ; `/readyz` 200 JWKS injoignable ; premier appel 401
  et ligne JSON « jwks not reloaded » ; appel signé d'un JWKS servi sous mTLS → 200 et ligne d'audit contre Postgres ;
  compteurs) ; `netpol` ; garde ; règles promtool.

## Écarts au design, constatés à l'implémentation

- **Pas d'`ErrorLog` sur le serveur** (243) : `slog.SetDefault` fait déjà passer le paquet `log` par le journal JSON ;
  le refus de handshake y sort, le test le lit. Le mutant qui retirait `ErrorLog` survivait.
- **Garde de `DASHBOARD_PUBLIC_URL`** : lue par une expression sur chaque manifest de `deploy/k8s`, toutes les valeurs
  égales.
- **Test du `cmd/`** : le JWKS de test est servi sous le SAN `localhost` (le nom `dashboard` ne se résout pas hors du
  cluster) ; la vérification du nom reste celle de Go, depuis l'hôte de l'URL.

## Mutations

À la main, par le script de 013a. Configuration (`Origin`) 7/7 ; compteurs 4/4 ; `cmd/` 15/16, le survivant
(`ErrorLog`) retiré ; `netpol` 4/4 après un port de test qui n'est plus le défaut ; garde 3/3 ; règles d'alerte 4/4.

## Preuve sur `contabo169`

Jouée le 2026-10-21 depuis la branche, sur le cluster en v0.26.0 : image `admin-api-svc` construite localement
(même `Dockerfile` que GoReleaser), importée sous `proof-013b` ; timer suspendu, puis réarmé.

1. `ca.yaml` : `admin-api-svc-tls` émis ; `dashboard-tls` porte `server auth` et `client auth`. `secrets.sh` pose
   `admin-api-svc/postgres-url`. NetworkPolicy et manifest appliqués : `rollout status` réussi. Le premier `Ping` prend
   « connection refused » (kube-router), `pgdial.Retry` le rejoue, le pod passe `Ready` par le Pooler.
2. Depuis un pod étiqueté `dashboard` (jamais `Ready`, hors des endpoints du Service `dashboard`), certificat
   `dashboard-tls`, sortie temporaire vers 8443 : sans jeton, 401 `unauthenticated`.
3. Certificat `router-svc` depuis ce pod : connexion coupée au handshake ; ligne JSON « mtls: client [router-svc] is not
   an allowed caller [dashboard] ». Pod `other` : `connection refused`.
4. Jetons personnels émis par l'humain, session élevée, depuis la console (l'écran n'offre pas le choix des scopes,
   #165) : `iss` `http://localhost:8080`, `aud` `gateway-admin`. `audit:read` : 200, page vide, `X-Request-Id` ; second
   appel : la ligne du premier (`sub` de l'opérateur, `list-audit-log`, 200, même `request_id`). `routes:read` : 403
   `forbidden_scope`, et sa ligne au journal (statut 403).

Objets de preuve retirés (pods, certificat, Secret, NetworkPolicy, jetons) ; `admin-api-svc` reste déployé sous
`proof-013b` jusqu'à la release. Après la release : rendre public le paquet GHCR `admin-api-svc` (humain).
