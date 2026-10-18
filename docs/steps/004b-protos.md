# Step 004b — Protos

Plan : `docs/plan/socle.md` §004, deuxième tiers. Spec : `docs/spec/09-contrats.md` §9.1 (payloads),
§9.2 (gRPC). Mode : PR.

## Contexte

Les payloads des topics et le gRPC interne n'ont aucun contrat écrit. Cette step pose `contracts/proto/`,
`buf lint` et `buf breaking` bloquants, la génération Go, et la garde « chaque `payload` du catalogue
existe » promise par la 004a.

**Terminé quand** (part de 004b) : `buf breaking` tourne sur la PR ; les payloads du catalogue et les
services de §9.2 existent ; le code généré est gardé par `check-generated`.

## Arbitrages

Brouillon champ par champ (chaque champ cite sa section), 26 trous et 10 contradictions relevés, classés
par Fable (spec, Fable, humain) ; les questions humaines tranchées le 2026-10-18.

**Humain** (recommandations retenues) :

1. **Deux producteurs de plus** : `smpp-server-svc` publie `CANCELLED` sur `mt.outcome` (vainqueur du
   jeton d'annulation), `router-svc` publie `UNROUTED` sur `mo.outcome`. Une RPC d'écriture sur l'état
   serait perdue à la reconstruction par rejeu (spec 05 §5.5). Catalogue, spec 02 §2.2, spec 09 §9.1
   amendés.
2. **Tout dans le payload** : ce que spec 06 §6.3, décision 1 §6.1 et décision 3 §4.2 appelaient
   « en-têtes » de `mt.routed` (bail, crédits, `charge_on`, version du plan, connecteur, compte
   opérateur) sont des champs de `MtRouted`, versionnés et gardés par `buf breaking`. Les quatre en-têtes
   communs de §9.1 restent les seuls. Spec 06 §6.3 et les deux décisions reformulées.
3. **Plan de contrôle** : `RotateContentKey(customer_id)`, appelée par `admin-api-svc` sous acteur (I13),
   provisionne ou fait tourner la clé ; `GetSealingKey` ne crée rien et rend `NOT_FOUND`. Spec 09 §9.2
   amendée. ADR-0023 amendé par l'**ADR 0026** : `sender_ids.first_used_at` se pose depuis la lane du
   routeur à `routed`, sans groupe sur `mt.outcome` ni champ de plus.
4. **UDH client** : un `submit_sm` UDHI est un message d'un segment, UDH conservé (`udh` sur
   `MtInbound`), facturé 1.
5. **Transfert** : crédits MT seuls, entre comptes d'un même client en `balance_scope = account`.
6. **Renommage** : l'entité `smpp_accounts` devient `accounts`, la valeur `balance_scope = smpp_account`
   devient `account` : un compte porte les deux canaux (spec 01 §1.1, ADR-0006), le nom SMPP trompait.
   Aucune table n'existe encore. Les ADR acceptés gardent l'ancien nom ; la spec et les décisions
   changent.

**Fable** (sans contredire la spec), dont cinq corrections du brouillon :

- **Corps** (T6) : `oneof body { string text ; bytes octets }`, `data_coding` pour les octets seuls ; le
  routeur encode une seule fois (spec 01 §1.4, pas de logique dupliquée).
- **`MessageStatus`** (T12) : enum dès maintenant, valeurs recopiées de l'ancien contrat public
  (`accepted`, `enroute`, `delivered`, `failed`, `expired`, `rejected`, `rerouted`, `cancelled`) ; un
  `string` changé en enum en 004c casserait `buf breaking`.
- **`Topup`** (T21) porte une direction : le compteur MO se recrédite après paiement.
- **`ConfigSecrets.Open`** (C9) prend `kms_key_ref` et `sealed`, comme l'ADR-0016.
- Le reste du brouillon est confirmé : identité du MO réassemblé (partie UDH 1), expiration au routeur en
  `REJECTED` + `message_expired`, `account_id` et `customer_id` sur `MtOutcome` et `customer_id` sur
  `MtInbound`, adresses soumises sur `DlrRouted`, `sent_source_addr`, chaîne de repli sur `MtRouted`,
  `rate_plan_version` en `int64`, `Mutation { request_id, actor }` sur les écritures de facturation,
  `OpenSession` idempotent, `DeliverSm` portant le payload, `observed_at` sur l'état de compte.

**Spec** : aucun flux `billing.events` ; cinq statuts DLR sur le fil, repliés en trois au journal ; une
enveloppe par octroi ; aucune RPC `Release` ; `Disconnect` et `ListActiveBinds` découlent de spec 01 et
complètent §9.2 ; les colonnes CDR (route, catégorie, canal, crédits, `charge_on`) sont des `attrs`.

**Outillage** :

7. `buf.yaml` à la racine, module `contracts/proto` ; `lint: STANDARD` ; `breaking: WIRE_JSON` (un record
   en rétention ou un pod précédent doit rester lisible, spec 09 : la compatibilité de fil suffit, un
   renommage de champ qui la garde est permis).
8. Génération par `buf generate`, plugins `protoc-gen-go` et `protoc-gen-go-grpc` par la directive
   `tool` ; sortie `internal/gen/` (managed mode, `go_package` dérivé), hors de la règle `core` de
   depguard (deux niveaux sous `internal/`) et exclue du lint comme code généré.
9. `make proto` : `buf lint`, puis `buf breaking` contre `origin/main` ; une base sans `contracts/proto`
   n'impose rien, une base introuvable est une erreur.
10. **Garde des payloads** : `(Catalog) MissingPayloads(known func(string) bool) []error` dans le cœur ;
    `tools/catalog` lui passe le registre protobuf du code généré.
11. **Revue** : `DestroyContentKeys` porte acteur et `request_id` (I13) ; `registered_delivery` devient
    `optional uint32` (absent = défaut du connecteur, valeurs SMPP) ; `DlrRouted.submitted_at` pour l'accusé ;
    `raw_status` réduit au jeton `stat` (I1) ; `credits = 0` dit « non facturable » ; la garde des
    payloads n'accepte qu'un message (`FindMessageByName`) ; `clean: true` supprime un `.pb.go` orphelin.
    Le chemin de `Disconnect` vers le pod qui tient le bind relève du composant `session-manager-svc`.
12. **Report** (T3) : les parties 2..N d'un MO concaténé gardent une ligne `received` seule au journal
    et au CDR jusqu'à la purge : issue `debt` dans la PR.

## Design arrêté

- `contracts/proto/` :
  - `sms/v1/` : `common.proto`, `mt.proto`, `dlr.proto`, `mo.proto`, `state.proto`, `connector.proto` ;
    les dix payloads du catalogue ;
  - `billing/v1`, `messagestate/v1`, `session/v1`, `smppserver/v1`, `contentkey/v1` : les services de
    §9.2, plus `RotateContentKey`, `Disconnect`, `ListActiveBinds`.
- `buf.yaml`, `buf.gen.yaml` ; `internal/gen/` généré ; `go:generate go tool buf generate` dans `doc.go`.
- `internal/catalog` : `MissingPayloads` ; `tools/catalog` la branche.
- `Makefile` : cible `proto` dans `check`.
- Amendements : `contracts/topics.yaml` (producteurs), spec 02 §2.2, spec 06 §6.3, spec 09 §9.1 et §9.2,
  décisions 1 et 3, renommage `account` (spec, décisions, catalogue) ; ADR 0026 et bandeau sur 0023.
- Vérification du critère : `buf lint` rouge sur une faute de style ; `buf breaking` rouge sur un champ
  renuméroté contre la base ; la garde des payloads rouge sur un payload absent ; le code généré à jour.
