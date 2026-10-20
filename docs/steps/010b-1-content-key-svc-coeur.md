# Step 010b-1 — `content-key-svc` : cœur et adaptateurs

Plan : `docs/plan/composants.md` §010, première des deux PR de 010b (Fable, 2026-10-20). Spec :
`docs/spec/02-architecture.md` §2.2, §2.5 ; `docs/spec/07-plan-de-controle.md` §7.1 ; `docs/spec/09-contrats.md` §9.2.
Décision : `docs/design/06-confiance-interne.md` §2 D5, §6. ADR : 0011, 0016, 0025. Contrat :
`contracts/proto/contentkey/v1/contentkey.proto`. Précédent : `docs/steps/010a-brique-openbao.md`. Règles :
`.claude/rules/go-code.md`, `.claude/rules/tests.md`. Mode : PR.

## Contexte

- 010a a livré OpenBao : la clé Transit `config-secrets`, la politique `content-key-svc` (`encrypt` et `decrypt` de
  cette clé seule), le rôle Kubernetes `content-key-svc` (SA `sms/content-key-svc`, audience `openbao`, jeton d'une
  heure, sans politique `default`) ; l'entrée 8200 n'admet que les pods `content-key-svc`.
- Le contrat porte deux services : `ContentKeyService` (clés par client, 032) et `ConfigSecretsService` (`Seal`,
  `Open`) ; le code généré vit sous `internal/gen/contentkey/v1`.
- Aucun service (non-Job) n'existe encore ; aucun appelant de `content-key-svc` non plus (`admin-api-svc` en 013).
- Dettes du territoire : #26 (allowlist du manifest lue par aucun test), #43 (refus de handshake hors slog), #70 (CA
  lue une fois), #76 (pod bloqué sans Secret TLS), #93 (exportateur OTLP), #122 (certificat du listener d'OpenBao).

**Terminé quand** (part de 010b-1) : un secret scellé par l'adaptateur s'ouvre contre un vrai OpenBao, sous un jeton
obtenu par le login Kubernetes ; seul un appelant de la carte atteint une méthode, et seulement les siennes.

## Arbitrages

Fable, 2026-10-20 ; aucun renvoi à l'humain.

1. **Coupe au `cmd/`** : 010b-1, `internal/contentkey/**` et les amendements d'ADR ; 010b-2, `cmd/content-key-svc`,
   GoReleaser, variables de `config`, `internal/netpol`, manifest, certificat, garde, `deploy.sh`, preuve sur le VPS.
   CLAUDE.md lie GoReleaser, manifest et `deploy.sh` au `cmd/` : une coupe « service / déploiement » ne tiendrait pas.
2. **Périmètre** : `ConfigSecretsService` seul ; `ContentKeyService` attend 032. Ni Postgres, ni GRANT, ni Secret de
   service. Aucune métrique neuve, donc aucune alerte ; la première s'enregistrera par
   `metrics.Guard(prometheus.DefaultRegisterer)`.
3. **Clean architecture** : cœur `internal/contentkey` (règles de `Seal`/`Open`, port `KMS`, erreurs du domaine ; le
   cœur n'importe ni `net/http` ni `grpc/codes`), adaptateurs `internal/contentkey/openbao` et
   `internal/contentkey/grpc`.
4. **Client OpenBao par `net/http`** : trois appels JSON ne justifient pas le module `openbao/api` dans le binaire qui
   doit avoir le moins de dépendances (ADR-0011). Chaque appel porte le contexte de l'appelant.
5. **Format scellé** : `sealed` est le chiffré Transit tel quel (`vault:v1:…`), `kms_key_ref` le nom de la clé,
   `config-secrets`. Une rotation de la clé garde le nom (`vault:v2:…`) : le rescellement d'ADR-0016 devient un
   `rewrap`. `Seal` refuse un clair vide ; `Open` refuse un autre `kms_key_ref`.
6. **Classement des échecs** (liste de refus, ADR-0016 addendum) : clair vide et `kms_key_ref` inconnu →
   `InvalidArgument` ; chiffré refusé par OpenBao (400 au déchiffrement) → `DataLoss` ; tout le reste (403, 503 scellé,
   réseau) → `Unavailable`. Un 400 couvre aussi une requête malformée par notre code : à réexaminer quand `Open` entrera
   sur un chemin chaud (016, 023, 030).
7. **Jeton** : login paresseux au premier appel, jeton projeté relu à chaque login, réutilisé jusqu'à la moitié de son
   `lease_duration`, oublié sur un 403 ; un seul login à la fois ; pas de `renew-self` (`max_ttl` = `ttl`).
8. **Autorisation par méthode** : une carte `appelant → méthodes complètes` dans l'adaptateur gRPC ; tout le reste
   rend `PermissionDenied`. L'appelant est le SAN DNS de la chaîne vérifiée **présent dans la carte**, pas le premier
   SAN. Contenu : `admin-api-svc → Seal` ; `Open` entre avec ses consommateurs. La carte remplace
   `cmd/content-key-svc/authz.go` d'ADR-0016.
9. **Aucun secret dans une erreur** (I2) : messages de statut gRPC constants ; aucun corps de réponse d'OpenBao relayé.
10. **Test** : vrai OpenBao (`openbao/openbao:2.7.1`, version de la brique, en `-dev`) ; méthode `kubernetes` dont
    `kubernetes_host` vise un faux TokenReview servi par le test (`HostAccessPorts`), qui répond `authenticated`,
    `system:serviceaccount:sms:content-key-svc` et l'audience `openbao` ; le jeton envoyé est un JWT aux claims
    `kubernetes.io`, signé en ES256 (OpenBao refuse HS256 même sans vérifier la signature). Serveur gRPC en processus sous `mtlstest`.
11. **ADR** : ADR-0011 et 06 §11 (`LocalKMS` non porté, 010a arbitrage 3) ; ADR-0016 (clé par domaine au lieu du tag,
    `transit/encrypt` au lieu de `WrapDataKey`, carte dans l'adaptateur gRPC, `kms_key_ref` nom de clé Transit).
12. **Pour 010b-2** (arrêté ici, conçu là) : `/readyz` sans dépendance à OpenBao ; `GRPC_ADDR` et `OPENBAO_ADDR`
    sans défaut, refusés vides ; démarrage refusé si `TLS_ALLOWED_CLIENTS` diffère des clés de la carte, et la garde de
    `deploy/` les compare au manifest (#26) ; NetworkPolicy de sortie dérivée d'`OPENBAO_ADDR`, d'entrée de
    `TLS_ALLOWED_CLIENTS` × `GRPC_ADDR` ; sondes du kubelet admises par kube-router (`--src-type LOCAL`), à prouver sur
    le VPS ; deux boucles dans `deploy.sh` ; `grpclog` vers slog (moitié gRPC de #43) ; déclencheur de #93 réécrit.
13. **Dettes** : #70 (la CA du client OpenBao est lue une fois, comme `mtls.Client`) et #122 reportées, porteurs
    inchangés ; #26, #43, #76, #93 en 010b-2.

## Design arrêté

- **`internal/contentkey/configsecrets.go`** (cœur) :
  - `KMS` : `Encrypt(ctx, key, plaintext) ([]byte, error)`, `Decrypt(ctx, key, sealed) ([]byte, error)` ;
  - `ConfigSecretsKey = "config-secrets"` ;
  - `ErrEmptySecret`, `ErrUnknownKey`, `ErrUndecryptable` (que l'adaptateur KMS rend pour un chiffré refusé) ;
  - `ConfigSecrets{KMS}` : `Seal(ctx, plaintext) (sealed, keyRef, err)` refuse le vide sans appeler la KMS ;
    `Open(ctx, keyRef, sealed)` refuse un autre `keyRef` sans l'appeler.
- **`internal/contentkey/openbao/transit.go`** : `New(addr, client *http.Client, role, jwtFile)` ;
  `Encrypt`/`Decrypt` sur `transit/{encrypt,decrypt}/<key>`, en-tête `X-Vault-Token` ; login
  `auth/kubernetes/login` sous mutex ; 400 au déchiffrement → `ErrUndecryptable` ; 403 → jeton oublié ; erreurs
  nommées par chemin et statut, jamais par le corps.
- **`internal/contentkey/grpc/server.go`** : `Callers` (la carte), `Authorize` (intercepteur unaire), `Register(s,
  ConfigSecrets)` ; projection des erreurs vers les codes de l'arbitrage 6, messages constants.
- **Tests** : cœur sur un double qui note ses appels ; `openbao` contre le conteneur (aller-retour, chiffré altéré,
  un seul login pour des appels concurrents, nouveau login passé la moitié du TTL, nouveau login après un jeton
  révoqué) ; `grpc` en processus (`admin-api-svc` scelle, n'ouvre pas ; un SAN admis par TLS hors carte n'atteint rien ;
  la carte trouve un appelant dont le SAN n'est pas le premier ; chaque erreur du cœur a son code ; un clair dans une
  erreur de la KMS n'atteint pas le statut).
- **ADR** : en-têtes d'ADR-0011 et d'ADR-0016, ligne ADR-0011 de 06 §11.
- **Plan** : §010 coupé en 010b-1 et 010b-2.

## Correctifs de revue, tour 1

Trois revues (correction et sécurité, tests creux, code en trop). Bloquants : 1, 2, 3.

1. **Un login refusé en 400 passait pour un chiffré refusé** : `Decrypt` classait tout 400, celui du login compris
   (rôle inconnu, jeton vide), en `ErrUndecryptable`, donc `DataLoss` et dead-letter d'un backlog sur une erreur de
   déploiement. Seul le 400 de `transit/decrypt` l'est désormais.
2. **Le faux TokenReview ne lisait pas le jeton** : il compare `spec.token` au fichier projeté ; un test réécrit le
   fichier entre deux logins (le kubelet le fait tourner).
3. **La garde `VerifiedChains` n'était pas prouvée** : le test présente un certificat que le serveur demande sans le
   vérifier (`RequestClientCert`).
4. **Le contexte de l'appelant borne aussi l'attente du login** : un sémaphore à canal remplace le mutex, un appel
   dont l'échéance tombe pendant le login d'un autre revient.
5. **Un 200 sans chiffré est une erreur** : `Seal` aurait rendu un `sealed` vide, persisté comme un secret perdu.
6. **Une réponse indécodable ne cite pas son contenu** : l'erreur de `json` peut citer une valeur du corps, donc un
   clair ; l'erreur ne garde que le chemin.
7. **Tests resserrés** : moitié du bail encadrée au plus près, un seul login après des 400, clair vide `[]byte{}`,
   déchiffrement d'un chiffré produit par OpenBao hors de l'adaptateur.
8. **Retraits** : la comparaison au jeton oublié (un 403 sur un ancien jeton coûte au pire un login de plus) ; la
   branche du login sans jeton (un jeton vide rend un 403, déjà classé) ; le drain du corps d'erreur.
9. **Pour 010b-2** : le client HTTP refuse les redirections (`X-Vault-Token` et le JWT suivraient un 307) ; un test du
   `cmd/` construit le serveur réel, intercepteur compris (`Register` ne l'installe pas).
