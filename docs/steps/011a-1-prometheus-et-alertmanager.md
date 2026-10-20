# Step 011a-1 — Prometheus et Alertmanager

Plan : `docs/plan/composants.md` §011, première des quatre PR de 011a (Fable, 2026-10-20). Spec :
`docs/spec/08-dashboard.md` ; `docs/spec/10-exploitation.md` §10.1 ; `docs/spec/02-architecture.md` §2.3. Décision :
`docs/design/06-confiance-interne.md` §4 (port ops). Précédents : `docs/steps/007c-networkpolicy-et-alertes.md`,
`docs/steps/010a-brique-openbao.md`, `docs/steps/010b-2-content-key-svc-service.md`. Origine : `go-gateway-bo`. Règles :
`.claude/rules/ci-deploy.md`, `.claude/rules/tests.md`. Mode : PR.

Ce document porte les arbitrages de **toute la 011a** ; les PR suivantes y renvoient et n'arrêtent que leur design.

## Contexte

- Les règles d'alerte de 007c (`deploy/alerts/`) passent `promtool test rules` en CI, mais rien ne les évalue sur
  l'environnement de test : aucun Prometheus, aucun Alertmanager.
- Le port ops de chaque service (`/metrics`, `/readyz`, `/healthz`, 9090) n'admet que le kubelet (`--src-type LOCAL`
  de kube-router) ; `internal/netpol` n'ouvre aucune entrée ops. OpenBao n'admet sur 8200 que `content-key-svc`, alors
  que `OpenBaoSealed` lit `vault_core_unsealed` sur ce port (télémétrie sans authentification, 010a).
- Le contrat BFF (`contracts/openapi-bff.yaml` 1.0.0) et les codes d'erreur du dashboard existent déjà ; aucun code du
  dashboard n'existe. `go-gateway-bo` en porte l'essentiel, avec des écarts à la spec : inactivité de 2 h, pas de
  `POST /auth/activity`, pas de rôle `noc`, chaque requête prolonge la session, premier opérateur né avec un mot de
  passe, journal dans une table d'audit générique.
- Dettes du territoire de 011a : #7, #8, #9, #11, #12, #17, #18, #19, #20, #21, #22, #5, #6 (origine), #14, #43 ;
  #3, #4, #13, #16 vont à 011b. Pour 011a-1 : aucune.

**Terminé quand** (011a-1) : Prometheus collecte le port ops de `content-key-svc`, OpenBao et cert-manager sur
l'environnement de test, charge les six fichiers de `deploy/alerts/` et remet ses alertes à Alertmanager ; aucun
autre pod n'atteint le port ops.

## Arbitrages

Fable, 2026-10-20, sauf A à D, tranchés par l'humain le même jour sur recommandation.

1. **Coupe en quatre PR.** 011a-1 : Prometheus, Alertmanager, entrée ops de `internal/netpol`, entrée 8200 d'OpenBao ;
   aucun code métier, le consommateur (les règles de 007c) existe déjà. 011a-2 : cœur `internal/dashboard`, adaptateurs
   Postgres et SMTP, migrations `migrations/dashboard`, gardes de schéma et de GRANT paramétrées par base. 011a-3 :
   `cmd/dashboard`, HTTP, base `dashboard`, Pooler, Mailpit, Secret, manifest, PDB, `deploy.sh`, preuve `/readyz` sur
   `contabo169`. 011a-4 : SPA, chaîne pnpm/Vite/Biome/Vitest dans `check`, types TypeScript générés, trois écrans,
   embarquement réel. La SPA vient après le `cmd/` : sans lui, elle n'a rien contre quoi se prouver.
2. **Un contexte, `internal/dashboard`** (nom du service et de la base) : fichiers par souci (`operator`, `accesslink`,
   `password`, `session`, `accesslog`), adaptateurs `postgres/`, `smtp/`, `http/`. Un lien `reset` retire les facteurs
   et ferme les sessions dans la même transaction (contrat) : un seul port de transaction, donc un seul cœur.
3. **Premier opérateur** (humain, D) : `DASHBOARD_BOOTSTRAP_EMAIL` dans `config.Infra` ; au démarrage, l'opérateur naît
   sans mot de passe (`INSERT … ON CONFLICT DO NOTHING`) et son lien d'activation n'est demandé **qu'à la création**.
   L'unicité de l'e-mail tient deux répliques sans verrou (#11 soldée). `/auth/me` rend `permissions: []` jusqu'en 012 ;
   le rôle `noc` et la colonne `noc` des sessions viennent avec les rôles, en 012.
4. **Base `dashboard`** : CRD `Database` de CloudNativePG (`owner: migrator`, `databaseReclaimPolicy: retain`), schéma
   `dashboard`, première migration `REVOKE CONNECT ON DATABASE dashboard FROM PUBLIC`. Rôle `dashboard` né du fichier de
   GRANT généré, jamais d'une migration : `contracts/grants/dashboard.yaml`, un `migrations/grants/<base>.sql` par base,
   chacun ne retirant que les rôles absents de l'union. `migrate` et `catalog-apply` dérivent l'URL de chaque base du
   chemin de `POSTGRES_URL` ; un répertoire `migrations/<base>` par base. Tables du dashboard classées dans
   `migrations/dashboard/audit.yaml`, toutes exemptées (journal du dashboard ou état d'exécution) ; I13 et
   `audit_capture()` restent propres à `control_plane`.
5. **Cookie** : jeton de 256 bits signé HMAC-SHA256 par la clé `cookie-key` du Secret `dashboard`, seul le `sha256` du
   jeton en base, sceau refusé avant toute lecture ; `HttpOnly`, `Secure`, `SameSite=Strict`, `__Host-`. Une rotation de
   la clé ferme toutes les sessions.
6. **Défi MFA émis dès 011a**, en colonnes de `sessions` (digest, échéance), nullées à la consommation (011b).
7. **Anti-brute-force** : 5 échecs en 15 min, par e-mail soumis et par source, fenêtre par oubli écrite une fois (#18),
   réservation avant hachage, 429 et `Retry-After` entier ≥ 1. Source : HMAC de la même clé sous le domaine `source:`,
   IPv6 au /64, `X-Forwarded-For` lu seulement depuis `TRUSTED_PROXIES` (vide par défaut). Hachage factice sous le même
   créneau sur e-mail inconnu ou compte désactivé. Purge des sessions expirées et des compteurs oisifs par un ticker du
   service (#8, #19). Horloge : celle de Postgres.
8. **Journal d'accès** : table `access_log` aux colonnes d'`AccessLogEntry`, aucune adresse IP ; écrite dans la
   transaction du fait ; `select, insert` seuls au rôle `dashboard`. 011a écrit `login`, `logout`,
   `access_link_requested`, `access_link_used`, `session_refused` (sceau valide, session close, expirée ou d'un compte
   désactivé). Un 429 n'écrit rien. Rétention (humain, A) : plancher `8760 hours` et porte de purge de 006a (#7).
9. **E-mail** : outbox (le contrat dit « queued »), une ligne par opérateur, jeton tiré à l'envoi, backoff 30 s × 2
   plafonné à 10 min, livraison sous `SKIP LOCKED` ; jeton dans le fragment de `DASHBOARD_PUBLIC_URL`. SMTP (humain, B) :
   STARTTLS quand le serveur l'offre, `PLAIN` si `SMTP_USERNAME` est posé, prouvé contre Mailpit en TLS.
10. **Exposition** (humain, C) : `kubectl port-forward` seul jusqu'en 026 ; `DASHBOARD_PUBLIC_URL=http://localhost:<port>`.
    Deux répliques et PDB dès 011a-3 (10 §10.2).
11. **HTTP** : oapi-codegen strict-server sur `net/http` (ServeMux), sans chi ; validation des requêtes par kin-openapi
    contre le contrat ; Origin, 413 et 415 en trois middlewares.
12. **SPA** : `dist` non commité, construit par `make web` avant les tests Go dans `check` et par les `before.hooks` de
    GoReleaser ; un `.gitkeep` tient le motif `embed` ; image sans Node (#20). openapi-typescript sous `go generate`,
    openapi-fetch, Vitest, Testing Library et axe ; pas de Playwright.
13. **Prometheus et Alertmanager** : chart `prometheus-community/prometheus` (Alertmanager en sous-chart), namespace
    `monitoring`, sans opérateur, sans node-exporter, kube-state-metrics ni pushgateway (aucune règle ne les lit). Les
    règles restent dans `deploy/alerts/`, montées par un ConfigMap. Alertmanager : un receiver sans intégration jusqu'au
    webhook du BFF (026).
14. **PgBouncer** : Pooler CNPG en mode transaction dès 011a-3, premier service à pool ; `migrate` et `catalog-apply`
    restent en direct.
15. **Dettes** : soldées en 011a : #7, #8, #11, #12, #18, #19, #20, #22, moitié « sondes » de #9. Reportées : #9 moitié
    CSP et assets (le répartiteur, 026), #17 (`contabo169` n'est pas la cible), #14, #43 moitié `net/http`, #5, #6, #21
    (sans objet si leur sujet n'est pas porté : le dire dans la PR qui le constate).
16. **Préalables** : la paramétrisation par base de `schema` et de `ParseGrants` ouvre 011a-2 ; le Secret `dashboard`
    porte deux clés, que `secrets.sh` et la garde du volume `role-secrets` doivent admettre (011a-3) ; `/health` du
    contrat et `/healthz`-`/readyz` du port ops coexistent.

Arbitrages propres à 011a-1, tranchés par l'agent sans contredire la spec :

17. **Chart 29.35.0** (Prometheus v3.15.0, la version du `promtool` du `Makefile`) : la plus récente publiée depuis au
    moins deux semaines.
18. **Collecte** : `scrape_interval` de 15 s (10 §10.1) ; les jobs par défaut du chart retirés sauf `prometheus` ; trois
    jobs : les pods de `sms` qui déclarent un port nommé `ops` (pas d'annotation : le nom du port est déjà la source de
    la sonde) ; OpenBao en HTTPS sur `/v1/sys/metrics?format=prometheus`, vérifié par la CA interne ; cert-manager sur
    9402.
19. **CA de Prometheus** : un `Certificate` `monitoring/prometheus` de l'émetteur interne, dont seul `ca.crt` sert ; les
    vérificateurs reçoivent le certificat, jamais la clé racine (06 §4).
20. **Entrée ops** : `internal/netpol` ouvre, pour chaque workload dont un conteneur nomme un port `ops`, ce port aux
    pods `app.kubernetes.io/name: prometheus` de `monitoring` ; `openbao.yaml` ouvre 8200 au même pair. Le namespace
    `monitoring` n'a pas de deny par défaut, comme `cert-manager`.
21. **Règles** : `deploy.sh` crée le ConfigMap `alert-rules` depuis les fichiers de `deploy/alerts/` hors `_test`, avant
    la brique ; le reloader du chart le surveille.
22. **Stockage long terme par remote-write** (10 §10.1) : reporté, aucun stockage cible ; issue `debt` neuve.

## Design arrêté

- **`deploy/bricks/namespaces.yaml`** : `monitoring`.
- **`deploy/bricks/ca.yaml`** : `Certificate` `monitoring/prometheus`.
- **`deploy/bricks/monitoring.yaml`** : `HelmChart` `prometheus` 29.35.0 (arbitrages 13, 17, 18), rétention locale du
  chart, `extraConfigmapMounts` des règles, `extraSecretMounts` de la CA, `rule_files` sur le montage.
- **`deploy/bricks/openbao.yaml`** : entrée 8200 depuis Prometheus.
- **`internal/netpol`** : entrée du port `ops` (arbitrage 20) ; `deploy/k8s/networkpolicies.yaml` régénéré.
- **`deploy/test-env/deploy.sh`** : ConfigMap des règles, brique, attente du chart et du rollout de Prometheus.
- **`deploy/guard_test.go`** : la brique entre sous les règles existantes des briques (image épinglée par le chart).
- **`.claude/rules/ci-deploy.md`** : l'entrée ops ouverte ; `docs/plan/composants.md` : la coupe en quatre.
- **Tests** : `netpol` (entrée ops d'un Deployment au port nommé, aucune pour un Job sans port `ops`, un cas faussé
  par règle) ; garde des briques si elle s'étend.
- **Preuve sur `contabo169`** : cibles `up` (content-key-svc, OpenBao, cert-manager) ; six groupes de règles chargés ;
  `OpenBaoSealed` muette OpenBao descellé ; une alerte forcée visible dans Alertmanager ; port ops refusé depuis un pod
  non admis.
