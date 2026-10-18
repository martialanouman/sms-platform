# Step 004c-3 — OpenAPI BFF, permissions

Plan : `docs/plan/socle.md` §004, dernier des trois contrats. Spec : `docs/spec/08-dashboard.md`,
`docs/spec/09-contrats.md` (en-tête, §9.3), décision 5 §3.3 et §3.4. Mode : PR.

## Contexte

L'ancien contrat BFF (`go-gateway-bo/api/openapi-bff.yaml`, 63 opérations, en français) relayait 38
opérations de la passerelle. Le proxy aveugle `/api/gw/*` les remplace (spec 08 §8.2) : le contrat BFF ne
décrit plus que ce que le BFF possède (spec 08 §8.1). Son relevé compte 25 opérations gardées, 38 retirées
et 4 ajoutées. Fable a tranché ses trous le 2026-10-18 ; l'humain a fixé la langue (anglais) et le découpage.

**Terminé quand** (part de 004c-3, et fin de 004) : `contracts/openapi-bff.yaml` est à l'état des
décisions ; les permissions du BFF sont générées avec les scopes ; chaque garde statique est rouge sur un
contrat fautif ; `oasdiff` valide et compare les trois contrats.

## Arbitrages

**Humain** : contrat en anglais, descriptions comprises (004c-1).

**Fable** :

- **Surface** (29 opérations) : santé, connexion, lien d'accès, déconnexion, session (`me`), geste
  d'activité, MFA (TOTP, WebAuthn, passkeys), jeton personnel, opérateurs, rôles, notifications, et trois
  ajouts de la spec 08 §8.1 : journal d'accès (`listAccessLog`), sessions du dashboard (`listSessions`,
  `deleteSession`). Aucun chemin sous `/gw` : la SPA a deux clients, BFF (`/api`) et Admin (`/api/gw`,
  chemin Admin verbatim).
- **Permissions** : `operators:read|write`, `roles:read|write`, `sessions:read|write`,
  `gateway:personal_token` (même forme que les scopes, ni joker ni hiérarchie). Le journal d'accès est sous
  `audit:read`, scope Admin lu dans la session : le rôle `Audit` voit les deux onglets (spec 08 §8.8).
  Notifications, MFA, `me` et activité : session seule. Le tag Admin `smpp-sessions` (004c-2) laisse
  `sessions:*` au BFF.
- **Sécurité** : schéma `session` (apiKey, cookie `__Host-dashboard_session`) ; chaque opération déclare
  `security` : `[]` pour les quatre publiques (`health`, `login`, `setPasswordFromAccessLink`, `logout`),
  `[{session: []}]` ou `[{session: [<permission>]}]` sinon. CSRF porté : 403 `forbidden_origin` et 415
  `unsupported_media_type` sur toute méthode non sûre, proxy compris.
- **MFA** (spec 08 §8.3) : propriété de la session. Tant que la session d'un opérateur dont un rôle porte
  plus que des lectures ordinaires n'est pas élevée, elle n'atteint que `/auth/*` ; ailleurs, et au proxy, 403 `mfa_required`.
- **Session** (décision 8 P7) : `Me.idleTimeoutSeconds` (`null` pour `noc`) ; seul `POST /auth/activity`
  (204), appelé par la SPA sur un geste, prolonge ; borne absolue de 12 h pour tous.
- **Jeton personnel** : scopes des rôles, sans traduction, moins `content:read`, `content:erase` et
  `gdpr:erase` (irréversibles ou hors écran, ADR-0019 « aucune automatisation externe »).
- **Codes** : `Error.code` en `enum` ; 400 → 422 `validation_error` ; `password_policy`, `mfa_code_refused`,
  `webauthn_ceremony_refused` en 422 ; refus de permission `forbidden_scope` ; 429 `rate_limited`
  (anti-brute-force) ; 500 `internal_error` déclaré partout ; un seul `forbidden_origin`. Les 17 codes
  propres au BFF entrent au §9.3, dans un bloc BFF, avec 410 et 415.
- **Temps réel** : la socket `/ws` reste hors des opérations (une montée de protocole n'a pas de réponse
  typable) ; ses messages sont des schémas. Sujets `metrics`, `sessions`, `connectors` sous `stream:read`,
  `notifications` sans permission. Les trames de la passerelle sont des `$ref` vers `openapi-admin.yaml`,
  sans recopie.
- **Notifications** : Alertmanager seule source ; `{id, severity, message, createdAt}`, `read` en liste.
- **Conventions** : camelCase des `operationId`, champs et paramètres (convention héritée du fichier),
  écrite dans `.claude/rules/contracts.md` ; `servers: /api` ; `info.version` 1.0.0.
- **Génération** : la liste des permissions seulement. Les types Go du BFF (oapi-codegen strict-server)
  viennent avec le composant `dashboard`, les types TypeScript avec la SPA.
- **Amendements** : §9.3 (bloc BFF, `forbidden_scope` et `rate_limited` étendus au BFF) ; spec 08 §8.3
  (session de premier facteur bornée à `/auth/*`, borne absolue 12 h), §8.4 (exclusions du jeton
  personnel), §8.5 (une lecture du trafic comprend `stream:read`), §8.8 (journal d'accès sous
  `audit:read`).

**Vérifié avant le design** : un `$ref` vers `openapi-admin.yaml` passe `oasdiff validate`, et
`oapi-codegen` v2.8.0 (`chi-server`, `strict-server`, `import-mapping` vers le paquet courant) génère depuis
lui malgré la montée de kin-openapi de 004c-1.

**Outillage** :

1. **Cœur** `internal/openapi` : `(Contract) Permissions(scopes []string) ([]string, []error)`, garde du BFF.
   Chaque opération déclare `security` ; `[]` réservé aux quatre opérations publiques ; sinon une exigence
   `session` à zéro ou une valeur, prise dans les scopes Admin ou les sept permissions BFF. Aucune clé de
   `paths` sous `/gw`. Aucune permission BFF n'est un scope Admin. Rend les permissions BFF déclarées,
   triées. `Go` rend aussi `Permissions`.
2. `tools/openapi` lit le BFF : sa garde, ses codes (`UnknownCodes`), et la génération.
3. `make openapi` : `oasdiff validate` et `breaking` informatif pour le BFF.
4. **Revue, premier tour** : une permission BFF qu'aucune opération ne porte est refusée (spec 08 §8.5,
   orphelins) ; la liste rendue est une copie de la liste figée, triée.
5. **Revue, premier tour, contrat** :
   - jeton personnel hors de `/auth/*` (`/gateway-token`), donc soumis à l'élévation (ADR-0019 « MFA
     comprise ») ; un scope demandé au-delà de ce que le jeton porte rend `forbidden_scope` ;
   - `NotificationEntry` valide (`unevaluatedProperties`, base ouverte) ; identifiants de chemin sans
     borne ; compteur de connexion indexé sur l'adresse soumise, qu'un compte la porte ou non ; refus du
     proxy `not_found` ; récepteur Alertmanager hors de `/api` ;
   - §9.3 : `unauthenticated`, `password_policy`, `service_unavailable` et `payload_too_large` précisés
     pour le BFF ; spec 09 : liste des permissions générée ;
   - **Fable** : 413 `payload_too_large` sur les douze opérations à corps (un code par cause) ; journal
     d'accès étendu (demande et usage d'un lien d'accès, retrait de facteur, réactivation, édition d'un rôle, fermeture forcée ;
     `target` opérateur ou rôle), spec 08 §8.9 et décision 5 §4.5 amendées ; MFA obligatoire dès un scope
     `:write`, un scope sensible ou une permission du BFF, spec 08 §8.3 et §9.3 amendées.

## Design arrêté

- `contracts/openapi-bff.yaml` : OpenAPI 3.1, 1.0.0, en anglais, sans commentaire, 29 opérations.
- `internal/openapi` : `(Contract) Permissions([]string) ([]string, []error)` ; `Go(scopes, permissions
  []string) []byte`.
- `internal/platform/scopes/scopes.gen.go` : `All` et `Permissions`.
- `tools/openapi`, `Makefile`, `.claude/rules/contracts.md`, spec 08 et 09 ci-dessus.
- Vérification du critère : chaque garde rouge sur un contrat muté (tests du cœur, câblage sur copies
  fautives des vrais contrats) ; la liste régénérée identique ; les trois contrats validés par `oasdiff`.
