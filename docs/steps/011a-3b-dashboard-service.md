# Step 011a-3b — Dashboard : service et déploiement

Plan : `docs/plan/composants.md` §011, seconde des deux PR de 011a-3. Arbitrages : `docs/steps/011a-3a-base-dashboard-et-pooler.md`
(45 à 58), et avant eux 1 à 44 (`011a-1`, `011a-2a`, `011a-2b`). Spec : `docs/spec/08-dashboard.md` §8.1, §8.3 ;
`docs/spec/10-exploitation.md` §10.1, §10.2. Contrat : `contracts/openapi-bff.yaml`. Précédent :
`docs/steps/010b-2-content-key-svc-service.md`. Règles : `.claude/rules/ci-deploy.md`, `.claude/rules/go-code.md`,
`.claude/rules/tests.md`. Mode : PR.

## Contexte

- Le cœur `internal/dashboard` et ses adaptateurs Postgres et SMTP existent (011a-2b) ; la base `dashboard`, son rôle
  et le Pooler existent sur le cluster (011a-3a). Aucun `cmd/`, aucun HTTP.
- Le Secret `sms/dashboard` ne porte que `postgres-password`.
- Dettes du territoire : #12 (fermeture du pool), #9 (sondes ; CSP et assets), #43 (refus de handshake hors slog),
  #20 (binaire sans Node, en 011a-4).

**Terminé quand** : la release déploie `dashboard` sur l'environnement de test, deux répliques, `/readyz` 200 ; par
`port-forward`, l'opérateur de bootstrap reçoit son lien dans Mailpit, pose son mot de passe, se connecte, lit `/auth/me`,
voit sa session close après 15 min sans geste, et le journal d'accès porte chaque fait.

## Arbitrages propres à 011a-3b

Tranchés par l'agent, sans contredire 1 à 58.

59. **`Me` dans l'adaptateur HTTP** : `Current` plus les constantes du cœur (`IdleTimeout`) ; `permissions` vide,
    `elevated` faux, facteurs à zéro (arbitrage 58).
60. **Contexte de session** : un middleware résout la session une fois par requête protégée et la pose dans le
    contexte ; l'`AuthenticationFunc` du validateur la lit pour juger la permission de l'opération. `logout` est
    public au contrat : il lit le cookie lui-même.
61. **Adresse source** : `RemoteAddr`, sauf si elle appartient à `TRUSTED_PROXIES`, auquel cas la dernière entrée de
    `X-Forwarded-For` ; une adresse illisible refuse la requête (le cœur refuse déjà une source invalide).
62. **Clé du cookie et mot de passe SMTP par fichier** (`COOKIE_KEY_FILE`, `SMTP_PASSWORD_FILE`), jamais par
    l'environnement : un secret ne passe pas par `/proc/<pid>/environ`.
63. **Sans SMTP joignable**, le service démarre : la livraison réessaie (backoff du cœur) et la jauge des liens
    indélivrables le montre ; le bootstrap n'attend pas l'envoi.

## Design arrêté

- **`internal/platform/config`** : `HTTP_ADDR`, `DASHBOARD_PUBLIC_URL`, `DASHBOARD_BOOTSTRAP_EMAIL`, `SMTP_ADDR`,
  `SMTP_FROM`, `SMTP_USERNAME`, `SMTP_PASSWORD_FILE`, `TRUSTED_PROXIES`, `COOKIE_KEY_FILE`, sans défaut ; `Load` ne lit
  que la forme.
- **`internal/dashboard`** : `Tx.Undeliverable` et `Service.Undeliverable`, le nombre de liens à dix tentatives jamais
  envoyés.
- **`internal/dashboard/http`** : `oapi-codegen.yaml` et `bff.gen.go` (arbitrages 46, 47) ; `Handler(Config)` monte
  `/api` : Origin, 413, 415, résolution de session, validation kin-openapi (permission comprise), serveur strict ;
  erreurs plates du contrat, cookie `__Host-dashboard_session` (`HttpOnly`, `Secure`, `SameSite=Strict`, `Path=/`).
- **`cmd/dashboard`** : `run` refuse une configuration incohérente (arbitrage 49), lit les fichiers, ouvre le pool
  (`pgdial.Retry`), joue le bootstrap, puis supervise `ops`, `delivery` (5 s), `purge` (1 min) et `http` ; drain :
  `/readyz` 503, HTTP arrêté d'abord ; `pool.Close()` après (#12). Jauge `dashboard_access_links_undeliverable`.
- **`internal/netpol`** : la sortie de `SMTP_ADDR` (`<nom>.<ns>.svc:<port>`), l'entrée du port de `HTTP_ADDR` pour
  personne (le `port-forward` passe par le kubelet).
- **Livraison** : `deploy/k8s/dashboard.yaml` (ServiceAccount, Service, Deployment de deux répliques,
  PodDisruptionBudget) ; `deploy/bricks/mailpit.yaml` et son certificat dans `ca.yaml` ; `secrets.sh` ajoute
  `cookie-key` et `postgres-url` au Secret existant ; `.goreleaser.yaml` ; `deploy.sh` (boucle des services, attente
  de Mailpit) ; `deploy/alerts/dashboard.yaml` (`AccessLinkUndeliverable`) ; garde étendue (PDB, port du Service égal
  à `HTTP_ADDR`, image de Mailpit égale à celle du test).
- **Tests** : `config` ; `http` (chaque refus du contrat : 401, 403 `forbidden_origin` et `forbidden_scope`, 404 hors
  surface, 410, 413, 415, 422, 429 avec `Retry-After`, 503 ; cookie posé et périmé ; aucune permission avant 012) ;
  `cmd/` sur le câblage réel contre Postgres et Mailpit (refus de démarrage, bootstrap et lien reçu, parcours complet,
  drain qui ferme le pool) ; `netpol` ; garde ; alerte.
- **Preuve sur `contabo169`** : rollout, `/readyz`, parcours complet par `port-forward`, Mailpit sous STARTTLS,
  sondes, refus d'un pod non admis vers Mailpit.

## Preuve sur `contabo169`

Jouée le 2026-10-20 depuis la branche, sur le cluster en v0.12.0 : image construite localement et importée sous le tag
`proof`, timer de déploiement suspendu pendant la preuve, puis réarmé.

1. `secrets.sh` ajoute `cookie-key` et `postgres-url` au Secret `sms/dashboard` existant ; Mailpit sert sous le
   certificat `mailpit-tls` ; le dashboard tourne à deux répliques prêtes.
2. Par `port-forward`, le lien d'activation de l'opérateur de bootstrap arrive dans Mailpit (objet « Activation de
   votre compte », lien `http://localhost:8080/access#…`). Ensuite :
   - `access-link` : 204 ;
   - `login` : 200, cookie `__Host-dashboard_session` avec `Path=/`, `HttpOnly`, `Secure` et `SameSite=Strict` ;
   - `me` : 200 (`permissions []`, `elevated false`, `idleTimeoutSeconds 900`) ;
   - `access-log` : 403 `forbidden_scope` (`audit:read`) ;
   - `logout` : 204, puis `me` : 401.
3. Une session antidatée de 15 min en base rend `me` 401. Le journal porte, dans l'ordre :
   - `access_link_requested` ;
   - `access_link_used` ;
   - `login`, puis `logout` ;
   - un second `login`, puis `session_refused`.
4. Prometheus collecte les deux répliques : `dashboard_access_links_undeliverable` vaut 0 sur chacune.
5. Mailpit : un pod au label `dashboard` lit la bannière `220`, un pod `other` est refusé.
6. `rollout restart` : drain sans dépassement ; les rejets d'admission de kube-router au démarrage sont absorbés par
   `pgdial.Retry`.

Rouge vu : un premier sondage de Mailpit concluait « admis » sur le statut de `head` ; rejoué en exigeant `220`.
