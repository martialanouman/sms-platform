# Catalogue des topics

Page générée depuis `contracts/topics.yaml` par `go generate` : ne pas l'éditer.

## Topics

| Topic | Clé | Ordre | Corps | Partitions | Rétention | Payload | Producteurs | Doublon |
|---|---|---|---|---|---|---|---|---|
| `connector.account-status` | `connector_account_id` | per_key | none | 8 | compacté | `sms.v1.ConnectorAccountStatus` | connector-pool-svc | dernier état |
| `dlr.events` | `connector_account_id` | per_key | none | 64 | 96h | `sms.v1.DlrEvent` | connector-pool-svc | événement → corrélation |
| `dlr.routed` | `account_id` | per_key | none | 64 | 96h | `sms.v1.DlrRouted` | message-state-svc | événement → idempotence côté client, sinon rien |
| `dlr.uncorrelated` | `connector_account_id` | none | none | 64 | 96h | `sms.v1.DlrEvent` | message-state-svc | événement |
| `dlr.unmatched` | `connector_account_id` | none | none | 64 | 96h | `sms.v1.DlrEvent` | message-state-svc | événement |
| `message.state` | `message_id` | per_key | none | 128 | 96h | `sms.v1.MessageState` | message-state-svc | version de l'état → CDR |
| `mo.inbound` | `connector_account_id` | per_key | clear_bounded | 64 | 96h | `sms.v1.MoInbound` | connector-pool-svc | événement |
| `mo.outcome` | `message_id` | per_key | none | 64 | 96h | `sms.v1.MoOutcome` | delivery-svc | transition → clé primaire du journal d'état |
| `mo.routed` | `account_id` | per_key | clear_bounded | 64 | 96h | `sms.v1.MoRouted` | router-svc | événement |
| `mt.billing-wait.marketing` | `message_id` | none | clear_bounded | 64 | 96h | `sms.v1.MtInbound` | router-svc | message → clé primaire du journal d'état ; SMS en double possible à la frontière du routeur (ADR-0014) |
| `mt.billing-wait.otp` | `message_id` | none | clear_bounded | 64 | 96h | `sms.v1.MtInbound` | router-svc | message → clé primaire du journal d'état ; SMS en double possible à la frontière du routeur (ADR-0014) |
| `mt.billing-wait.transactional` | `message_id` | none | clear_bounded | 64 | 96h | `sms.v1.MtInbound` | router-svc | message → clé primaire du journal d'état ; SMS en double possible à la frontière du routeur (ADR-0014) |
| `mt.inbound.marketing` | `message_id` | none | clear_bounded | 64 | 96h | `sms.v1.MtInbound` | rest-api-svc, smpp-server-svc | message → clé primaire du journal d'état ; SMS en double possible à la frontière du routeur (ADR-0014) |
| `mt.inbound.otp` | `message_id` | none | clear_bounded | 64 | 96h | `sms.v1.MtInbound` | rest-api-svc, smpp-server-svc | message → clé primaire du journal d'état ; SMS en double possible à la frontière du routeur (ADR-0014) |
| `mt.inbound.transactional` | `message_id` | none | clear_bounded | 64 | 96h | `sms.v1.MtInbound` | rest-api-svc, smpp-server-svc | message → clé primaire du journal d'état ; SMS en double possible à la frontière du routeur (ADR-0014) |
| `mt.outcome` | `message_id` | per_key | none | 128 | 96h | `sms.v1.MtOutcome` | router-svc, connector-pool-svc | transition → clé primaire du journal d'état |
| `mt.reroute-park` | `message_id` | none | clear_bounded | 64 | 96h | `sms.v1.MtRouted` | connector-pool-svc | segment → clé primaire de la facturation (décision 1 §6.1) |
| `mt.routed.marketing` | `manual` | per_partition | clear_bounded | 384 | 96h | `sms.v1.MtRouted` | router-svc, connector-pool-svc | segment → SMS en double possible à la frontière du pool (décision 3 §6.7) |
| `mt.routed.otp` | `manual` | per_partition | clear_bounded | 384 | 96h | `sms.v1.MtRouted` | router-svc, connector-pool-svc | segment → SMS en double possible à la frontière du pool (décision 3 §6.7) |
| `mt.routed.transactional` | `manual` | per_partition | clear_bounded | 384 | 96h | `sms.v1.MtRouted` | router-svc, connector-pool-svc | segment → SMS en double possible à la frontière du pool (décision 3 §6.7) |

## Groupes

| Groupe | Assignation | Tenant | Dépendances | Concurrence | Âge maximal | Départ |
|---|---|---|---|---|---|---|
| `router-svc.mt.inbound.otp` | group | account_id | billing-svc : park_global, control-plane : blocking, kafka : blocking | per_partition | category | earliest |
| `router-svc.mt.inbound.transactional` | group | account_id | billing-svc : park_global, control-plane : blocking, kafka : blocking | per_partition | category | earliest |
| `router-svc.mt.inbound.marketing` | group | account_id | billing-svc : park_global, control-plane : blocking, kafka : blocking | per_partition | category | earliest |
| `router-svc.mt.billing-wait.otp` | group | account_id | billing-svc : blocking, control-plane : blocking, kafka : blocking | per_partition | category | earliest |
| `router-svc.mt.billing-wait.transactional` | group | account_id | billing-svc : blocking, control-plane : blocking, kafka : blocking | per_partition | category | earliest |
| `router-svc.mt.billing-wait.marketing` | group | account_id | billing-svc : blocking, control-plane : blocking, kafka : blocking | per_partition | category | earliest |
| `router-svc.mo.inbound` | group | none | control-plane : blocking, kafka : blocking, redis-state : blocking | per_partition | — | earliest |
| `router-svc.connector.account-status` | broadcast | none | kafka : blocking | per_partition | — | earliest |
| `message-state-svc.mt.inbound.otp` | group | none | kafka : blocking, message-state-db : blocking | per_partition | — | earliest |
| `message-state-svc.mt.inbound.transactional` | group | none | kafka : blocking, message-state-db : blocking | per_partition | — | earliest |
| `message-state-svc.mt.inbound.marketing` | group | none | kafka : blocking, message-state-db : blocking | per_partition | — | earliest |
| `message-state-svc.mt.routed.otp` | group | none | kafka : blocking, message-state-db : blocking | per_partition | — | earliest |
| `message-state-svc.mt.routed.transactional` | group | none | kafka : blocking, message-state-db : blocking | per_partition | — | earliest |
| `message-state-svc.mt.routed.marketing` | group | none | kafka : blocking, message-state-db : blocking | per_partition | — | earliest |
| `message-state-svc.mt.outcome` | group | none | kafka : blocking, message-state-db : blocking | per_partition | — | earliest |
| `message-state-svc.dlr.events` | group | none | kafka : blocking, message-state-db : blocking | per_partition | — | earliest |
| `message-state-svc.dlr.uncorrelated` | group | none | kafka : blocking, message-state-db : blocking | per_partition | 5m | earliest |
| `message-state-svc.dlr.unmatched` | group | none | kafka : blocking, message-state-db : blocking | per_partition | — | earliest |
| `message-state-svc.mo.inbound` | group | none | kafka : blocking, message-state-db : blocking | per_partition | — | earliest |
| `message-state-svc.mo.routed` | group | none | kafka : blocking, message-state-db : blocking | per_partition | — | earliest |
| `message-state-svc.mo.outcome` | group | none | kafka : blocking, message-state-db : blocking | per_partition | — | earliest |
| `message-state-svc.projection.message.state` | group | none | clickhouse : blocking, kafka : blocking | per_partition | — | earliest |
| `message-state-svc.projection.mt.inbound.otp` | group | none | clickhouse : blocking, content-key-svc : blocking, control-plane : blocking, kafka : blocking | per_partition | — | earliest |
| `message-state-svc.projection.mt.inbound.transactional` | group | none | clickhouse : blocking, content-key-svc : blocking, control-plane : blocking, kafka : blocking | per_partition | — | earliest |
| `message-state-svc.projection.mt.inbound.marketing` | group | none | clickhouse : blocking, content-key-svc : blocking, control-plane : blocking, kafka : blocking | per_partition | — | earliest |
| `message-state-svc.projection.mo.inbound` | group | none | clickhouse : blocking, content-key-svc : blocking, control-plane : blocking, kafka : blocking | per_partition | — | earliest |
| `connector-pool-svc.mt.routed.otp` | manual | connector_account_id | control-plane : blocking, kafka : blocking, redis-state : fail_open | per_partition | category | earliest |
| `connector-pool-svc.mt.routed.transactional` | manual | connector_account_id | control-plane : blocking, kafka : blocking, redis-state : fail_open | per_partition | category | earliest |
| `connector-pool-svc.mt.routed.marketing` | manual | connector_account_id | control-plane : blocking, kafka : blocking, redis-state : fail_open | per_partition | category | earliest |
| `connector-pool-svc.mt.reroute-park` | group | connector_account_id | control-plane : blocking, kafka : blocking | per_partition | category | earliest |
| `billing-svc.mt.routed.otp` | group | none | control-plane : blocking, kafka : blocking | per_partition | — | earliest |
| `billing-svc.mt.routed.transactional` | group | none | control-plane : blocking, kafka : blocking | per_partition | — | earliest |
| `billing-svc.mt.routed.marketing` | group | none | control-plane : blocking, kafka : blocking | per_partition | — | earliest |
| `billing-svc.mt.outcome` | group | none | control-plane : blocking, kafka : blocking | per_partition | — | earliest |
| `billing-svc.dlr.routed` | group | none | control-plane : blocking, kafka : blocking | per_partition | — | earliest |
| `billing-svc.mo.routed` | group | none | control-plane : blocking, kafka : blocking | per_partition | — | earliest |
| `delivery-svc.dlr.routed` | group | account_id | control-plane : blocking, endpoint : park_by_tenant, kafka : blocking | per_tenant (8 en vol) | 24h | earliest |
| `delivery-svc.mo.routed` | group | account_id | control-plane : blocking, endpoint : park_by_tenant, kafka : blocking | per_tenant (8 en vol) | 24h | earliest |

## Dead-letters

Dérivées de chaque groupe : corps de la source, ordre `none`.

| Topic | Corps | Partitions | Rétention |
|---|---|---|---|
| `billing-svc.dlr.routed.dead-letter` | none | 8 | 96h |
| `billing-svc.mo.routed.dead-letter` | clear_bounded | 8 | 96h |
| `billing-svc.mt.outcome.dead-letter` | none | 8 | 96h |
| `billing-svc.mt.routed.marketing.dead-letter` | clear_bounded | 8 | 96h |
| `billing-svc.mt.routed.otp.dead-letter` | clear_bounded | 8 | 96h |
| `billing-svc.mt.routed.transactional.dead-letter` | clear_bounded | 8 | 96h |
| `connector-pool-svc.mt.reroute-park.dead-letter` | clear_bounded | 8 | 96h |
| `connector-pool-svc.mt.routed.marketing.dead-letter` | clear_bounded | 8 | 96h |
| `connector-pool-svc.mt.routed.otp.dead-letter` | clear_bounded | 8 | 96h |
| `connector-pool-svc.mt.routed.transactional.dead-letter` | clear_bounded | 8 | 96h |
| `delivery-svc.dlr.routed.dead-letter` | none | 8 | 96h |
| `delivery-svc.mo.routed.dead-letter` | clear_bounded | 8 | 96h |
| `message-state-svc.dlr.events.dead-letter` | none | 8 | 96h |
| `message-state-svc.dlr.uncorrelated.dead-letter` | none | 8 | 96h |
| `message-state-svc.dlr.unmatched.dead-letter` | none | 8 | 96h |
| `message-state-svc.mo.inbound.dead-letter` | clear_bounded | 8 | 96h |
| `message-state-svc.mo.outcome.dead-letter` | none | 8 | 96h |
| `message-state-svc.mo.routed.dead-letter` | clear_bounded | 8 | 96h |
| `message-state-svc.mt.inbound.marketing.dead-letter` | clear_bounded | 8 | 96h |
| `message-state-svc.mt.inbound.otp.dead-letter` | clear_bounded | 8 | 96h |
| `message-state-svc.mt.inbound.transactional.dead-letter` | clear_bounded | 8 | 96h |
| `message-state-svc.mt.outcome.dead-letter` | none | 8 | 96h |
| `message-state-svc.mt.routed.marketing.dead-letter` | clear_bounded | 8 | 96h |
| `message-state-svc.mt.routed.otp.dead-letter` | clear_bounded | 8 | 96h |
| `message-state-svc.mt.routed.transactional.dead-letter` | clear_bounded | 8 | 96h |
| `message-state-svc.projection.message.state.dead-letter` | none | 8 | 96h |
| `message-state-svc.projection.mo.inbound.dead-letter` | clear_bounded | 8 | 96h |
| `message-state-svc.projection.mt.inbound.marketing.dead-letter` | clear_bounded | 8 | 96h |
| `message-state-svc.projection.mt.inbound.otp.dead-letter` | clear_bounded | 8 | 96h |
| `message-state-svc.projection.mt.inbound.transactional.dead-letter` | clear_bounded | 8 | 96h |
| `router-svc.connector.account-status.dead-letter` | none | 8 | 96h |
| `router-svc.mo.inbound.dead-letter` | clear_bounded | 8 | 96h |
| `router-svc.mt.billing-wait.marketing.dead-letter` | clear_bounded | 8 | 96h |
| `router-svc.mt.billing-wait.otp.dead-letter` | clear_bounded | 8 | 96h |
| `router-svc.mt.billing-wait.transactional.dead-letter` | clear_bounded | 8 | 96h |
| `router-svc.mt.inbound.marketing.dead-letter` | clear_bounded | 8 | 96h |
| `router-svc.mt.inbound.otp.dead-letter` | clear_bounded | 8 | 96h |
| `router-svc.mt.inbound.transactional.dead-letter` | clear_bounded | 8 | 96h |
