# Step 004c-1 — OpenAPI public

Plan : `docs/plan/socle.md` §004, troisième tiers, premier des trois contrats. Spec : `docs/spec/09-contrats.md`
(en-tête, §9.3), `docs/spec/03-pipeline-mt.md`, `docs/spec/04-voie-retour.md`. Mode : PR.

## Contexte

Les trois contrats OpenAPI des anciens dépôts (public, Admin, BFF) sont repris dans `contracts/`, mis à
l'état des décisions, en version 1.0.0. Les relevés d'écarts, croisés avec la spec, ont donné 54 écarts sur
le public, 129 sur l'Admin et une soixantaine sur le BFF. Fable a tranché 115 trous et contradictions sans
contredire la spec. L'humain a décidé le reste le 2026-10-18.

**Terminé quand** (part de 004c-1) : `contracts/openapi-public.yaml` est à l'état des décisions ; `oasdiff`
valide le contrat et compare au contrat de la base, bloquant sur une rupture `ERR`.

## Arbitrages

**Humain** :

1. **Découpage** : 004c devient trois PR, une par contrat. 004c-1 (public et `oasdiff`), 004c-2 (Admin,
   générateur de scopes, gardes statiques des trois contrats), 004c-3 (BFF, permissions). Plan amendé.
2. **Langue** : tous les contrats en anglais, descriptions comprises. Écrit dans `.claude/rules/contracts.md`.
3. **Scripts de routage** : JavaScript seul. L'amendement de spec 01 porte sur l'Admin : 004c-2.

**Fable**, pour le public (verdicts complets dans les relevés, non commités) :

- **Surface** : `submit-message`, `get-message`, webhooks `on-mo` et `on-dlr`. Retirés : le lot, la liste
  `GET /messages`, `GET /account`, `GET /health`, l'annulation (ADR-0009), le HMAC de requête (la clé est
  stockée hachée, spec 03 §3.2.1).
- **Sécurité** : `BearerApiKey` seul, déclaré sur chaque opération (huma n'applique pas le `security`
  global) ; une clé = un compte ; canal REST et statut du compte refusés en 403.
- **Codes après l'ACK** (contradiction interne à §9.3) : jamais une réponse synchrone (spec 03 §3.1).
  Hors de `Error.code`, présents dans `ErrorCode` (état, DLR). §9.3 amendée : colonne HTTP « — » pour
  `invalid_destination`, `invalid_source`, `sender_id_not_authorized`, `recipient_opted_out`,
  `content_blocked`, `no_route`, `insufficient_credit`.
- **Expiration** : un seul code, `message_expired` ; statut public `expired` dès que
  `error_code = message_expired`. Spec 03 §3.3.1 et §3.4.1 amendées.
- **`trace_id`** exposé en lecture (spec 01 §1.4) : ajouté à `sms.v1.MessageState`, additif.
- **Soumission** : `to` en contrôle syntaxique seul (`^\+?[0-9]{1,20}$`), E.164 au routeur ; `text` borné à
  2 000 caractères par le schéma (422), 413 réservé au corps HTTP ; `registered_delivery` booléen
  facultatif, absent = défaut du connecteur ; `validity_period` date-time absolue ; ni `encoding`, ni
  `data_coding`, ni `category`, ni `client_ref` ; idempotence par compte, 24 h.
- **Lecture** : 404 pour un message inconnu ou d'un autre compte ; 429 et 503 déclarés ;
  `segment_count` nul avant le routage ; `accepted_at` partout (`submitted` est le SMSC) ; ni `encoding`,
  ni `category`, ni `credits_charged` ; correspondance événement → statut publiée dans `MessageStatus`.
- **Webhooks** : `X-Signature = hex(HMAC_SHA256(secret, X-Timestamp + "." + raw_body))`, tolérance 5 min,
  en-têtes déclarés ; DLR sur l'enum `DlrStatus`, par segment, dédoublonné par
  `(message_id, segment_seq, status)` ; MO avec `stop`, corps en `text` ou `octets` (base64).
- **Exemples** : numéros de la plage fictive NANP (`+12025550101`) ; `servers` en `example.com`.
- **Version** : `1.0.0` ; décision 2 §10 amendée.
- **Emplacement** : `contracts/` ; spec 07 §7.3 et 08 §8.1 amendées.
- **Génération** : en 004c, seule la liste des scopes et des permissions (004c-2, 004c-3). huma ne génère
  aucun type Go depuis le contrat ; les types Go du BFF et TypeScript de la SPA arrivent avec leur
  composant. Spec 09 amendée : huma pour l'Admin et le public, oapi-codegen strict-server pour le BFF.

**Outillage** :

4. `oasdiff` par la directive `tool`. `oasdiff validate --fail-on WARN` tient lieu de validation : aucun
   code à nous.
5. `make openapi` dans `check` : validation du contrat public, puis `oasdiff breaking`
   contre `OPENAPI_BASE` (défaut `origin/main`), `--fail-on ERR` pour le public. Une base sans le fichier
   n'impose rien, une base introuvable est une erreur (même logique que `proto`).
6. Les gardes statiques (un scope par opération, sécurité du public, `Error.code` ⊆ §9.3) naissent en
   004c-2 avec l'outil qui lit les contrats : une garde du public seul n'aurait qu'un cas.
7. **Revue** :
   - `Error` et ses `errors[]` fermés : l'erreur par défaut de huma renvoie la valeur refusée (I1) ;
   - idempotence : seul le `202` est gardé, une clé en cours répond `idempotency_conflict` ;
   - signature `v1=<hex>`, une valeur par secret valide pendant une rotation, re-signée à chaque
     tentative ; redirection non suivie ;
   - `from` et la clé d'idempotence bornés ; `validity_period` ne fait que raccourcir ;
   - retirés : tags, licence, descriptions redites, adresses et `accepted_at` du DLR, `segment_count`
     du MO.
   - **Fable** : aucun accusé pour un rejet, la promesse retirée de spec 03 §3.1 et §9.3 ; `source`
     (`state`, `history`) sur `Message`, spec 05 §5.5 ; colonne SMPP de §9.3 à « — » pour les codes
     d'après l'ACK et `submit_failed` ; sender ID lu parmi ceux du client du compte, spec 03 §3.2.
   - Écartés : schémas de réponse fermés (la comparaison du contrat servi au fichier attrape un champ
     non déclaré, et fermer interdirait l'ajout compatible) ; forme de la clé d'API et frein avant
     l'authentification (implémentation).

## Design arrêté

- `contracts/openapi-public.yaml` : OpenAPI 3.1, version 1.0.0, en anglais, sans commentaire.
- `contracts/proto/sms/v1/state.proto` : `MessageState.trace_id` ; code régénéré.
- `go.mod` : `tool github.com/oasdiff/oasdiff`.
- `Makefile` : cible `openapi` dans `check`.
- `.claude/rules/contracts.md` : conventions (anglais, nullable `type: [T, "null"]`, erreur plate
  `{code, message, errors[]}`, `operationId` kebab-case, champs snake_case, MSISDN jamais dans une URL).
- Amendements : §9.3 (colonne HTTP), en-tête de spec 09 (génération, huma et oapi-codegen), spec 03 §3.3.1
  et §3.4.1, spec 07 §7.3 et 08 §8.1 (`contracts/`), décision 2 §10, plan §004 (découpage).
- Vérification du critère : `oasdiff validate` rouge sur un contrat invalide ; `oasdiff breaking` rouge sur
  un chemin retiré contre la base ; le contrat chargé par `oasdiff` sans avertissement.
