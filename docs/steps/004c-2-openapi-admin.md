# Step 004c-2 — OpenAPI Admin, scopes générés, gardes

Plan : `docs/plan/socle.md` §004, deuxième des trois contrats. Spec : `docs/spec/07-plan-de-controle.md`
(§7.3, §7.4), `docs/spec/08-dashboard.md` §8.5, `docs/spec/09-contrats.md`. Mode : PR.

## Contexte

L'ancien contrat Admin (136 opérations, scopes `admin:read` et `admin:write`) est repris dans
`contracts/openapi-admin.yaml`, à l'état des décisions. Son relevé d'écarts compte 129 entrées (71 tranchées par
la spec, 52 trous, 6 contradictions). Fable a tranché les trous et les contradictions le 2026-10-18, sauf
une, décidée par l'humain : JavaScript seul. La liste des scopes se génère depuis le contrat, et les gardes
statiques des contrats naissent avec l'outil qui les lit.

**Terminé quand** (part de 004c-2) : le contrat Admin est à l'état des décisions ; la liste des scopes est
générée et gardée par `check-generated` ; chaque garde statique est rouge sur un contrat fautif.

## Arbitrages

**Humain** : scripts de routage en JavaScript seul (`language` disparaît) ; spec 01 §1.1 et §1.2 amendées.

**Fable** (relevé et verdicts complets dans les notes de session, non commités) :

- **Scopes** (spec 07 §7.4) : 25 tags kebab-case ; chaque opération déclare exactement un scope,
  `<tag>:read`, `<tag>:write` ou une action sensible de son tag (`routing-scripts:publish`,
  `suppressions:delete`, `billing:topup`, `content:erase`, `gdpr:erase`, `cdr:export_bulk`). Seuls les
  scopes déclarés par une opération existent, plus `msisdn:reveal`, qu'aucune opération ne déclare
  (spec 08 §8.5 refuse les orphelins) ; spec 07 §7.4 amendée. `smpp-sessions` évite la collision avec la
  permission BFF `sessions`. `assign-routing-script` et `rollback-routing-script` sous
  `routing-scripts:publish`, `adjust-balance` sous `billing:topup`, `rotate-content-key` sous
  `customers:write`.
- **Surface** : 10 opérations retirées (fournisseurs de facturation, `set-connector-bind-pool`,
  `stream-billing-alerts`, doublons), 19 ajoutées : 144 opérations (dead-letters et leur rejeu, jobs, comptes opérateur,
  `stream-connectors`, impact d'une suspension, rotation du secret de webhook, débit du compte,
  ajustement, propriétaires au plancher MO, fiche message, diff d'audit, retour arrière d'un script).
- **Conventions** : pagination par curseur opaque partout sauf identifiants et webhooks ; paramètres
  snake_case ; statut typé par ressource ; PATCH à trois états ; MSISDN hors des URL (recherches en
  `POST`), schéma `Msisdn` partagé, masqué sauf `msisdn:reveal` ; `servers` sans `/v1` ; sécurité
  `http bearer` JWT, sans `oauth2` (aucun compte de service).
- **Codes** : `Error.code` réduit aux 11 codes que l'Admin peut rendre ; rejeu refusé en 409 `conflict`.
- **Données** (décisions 1, 3, spec 06, 07) : `account` partout ; `charge_on` sur la facturation du client ;
  un champ, un écrivain, un scope (facturation et contenu hors de `update-customer`) ;
  `max_age_override` par client, à la baisse ; `platform_policy.billing_enabled` ; comptes opérateur
  scindés du connecteur, créés en `202` + job ; secret de webhook généré par la passerelle ; aucune
  `least_loaded`, aucun `lua`.
- **Protos** (additifs) : `MessageState.antispam_verdict` et `antispam_rule_id` ; `Session.connected_at` et
  `remote_addr` ; `ListLedgerEntriesRequest.direction` ; `BillingService.ListOwnersAtMoFloor`.
- **Amendements** : §9.3 (`insufficient_credit` en 402 sur un transfert de solde Admin) ; spec 07 §7.1 (`max_age_override`, `billing_enabled`) et §7.4 ; spec 08 §8.6 (flux
  `connectors`) ; décision 3 §4.2 (`202` + job) ; spec 01 (JavaScript seul).

**Outillage** :

1. **Cœur** `internal/openapi` : décode le strict nécessaire d'un contrat (yaml.v3, déjà admis par
   depguard ; `go/format` ajouté à sa liste blanche : bibliothèque standard sans I/O, que `$gostd` de
   depguard omet) ; `Scopes` (liste triée et opération → scope) ; trois gardes ; rendu Go de la liste.
   `tools/openapi` compose : lit les contrats et la spec, échoue sur une garde, écrit la liste avec `-gen`.
2. **Gardes** :
   - Admin : chaque opération a exactement un tag et exactement un scope de son tag (read, write ou action
     sensible admise) ;
   - public : chaque opération déclare `BearerApiKey` sans scope, et rien d'autre ;
   - `Error.code` et `ErrorCode` de chaque contrat ⊆ codes de la table §9.3, lue dans
     `docs/spec/09-contrats.md`. Le catalogue Go de la step 005 reprendra ce rôle.
   La validation, l'unicité des `operationId` et les ruptures restent à `oasdiff`.
3. **Liste générée** : `internal/platform/scopes/scopes.gen.go`, `type Scope string`, `All` (trié),
   `ByOperation`. Pas de constante par scope : aucun consommateur encore. `go:generate` dans `doc.go`.
4. `make openapi` : la garde, puis `oasdiff validate` des deux contrats ; `oasdiff breaking` bloquant pour
   le public, informatif pour l'Admin (spec 09).

## Design arrêté

- `contracts/openapi-admin.yaml` : OpenAPI 3.1, 1.0.0, en anglais, sans commentaire.
- `internal/openapi` : `Parse([]byte) (Contract, error)`, `(Contract) Scopes() (Scopes, []error)` qui porte
  la garde Admin, `(Contract) PublicSecurity() []error`, `(Contract) UnknownCodes([]string) []error`,
  `SpecCodes([]byte) []string`, `(Scopes) Go() ([]byte, error)`.
- `tools/openapi` ; `internal/platform/scopes/scopes.gen.go` ; `doc.go`.
- `Makefile` : `openapi` étendue.
- Protos et amendements ci-dessus.
- Vérification du critère : chaque garde rouge sur un contrat muté (tests du cœur) ; la liste régénérée
  identique ; le contrat Admin validé par `oasdiff` sans avertissement.
