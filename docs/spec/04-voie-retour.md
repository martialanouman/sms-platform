# 4. Voie retour

Ce qui revient des opérateurs : les MO (messages entrants) et les DLR (accusés de remise), jusqu'au
client.

## 4.1 Vue d'ensemble

```
 opérateur ─▶ pool ─┬─ dlr.events ─▶ message-state ─ corrélation ─▶ dlr.routed ─┐
   (deliver_sm,     │                                                           ├─▶ delivery ─▶ client
    webhook, poll)  └─ mo.inbound ─▶ router (MO) ─ STOP, compte ─▶ mo.routed ───┘    (bind, webhook)
```

Le pool reçoit tout, sur les binds du détenteur, sur l'ingress `/callbacks/{connector_account_id}` ou
par polling (décision 3). Il acquitte le fournisseur **après** production durable : un `deliver_sm_resp`
ou un `200` n'est rendu qu'une fois le record écrit. `origin_ts` est posé à la réception.

## 4.2 DLR

1. **Normalisation** par le pool : statut brut → `delivered`, `undeliverable`, `expired`, `rejected`,
   `failed` ; `remote_id` ; `connector_account_id`. Publié sur `dlr.events`.
2. **Corrélation** par `message-state-svc`, seul à la faire (décision 2) : clé
   `(connector_account_id, remote_id)` dans `dlr_correlation`, inscrite à chaque `submitted`. Un DLR HTTP
   qui couvre tout le message porte `segment_seq = 0` et s'applique à chaque segment.
3. **Transition** au journal, puis DLR enrichi (`message_id`, `account_id`, `customer_id`) sur
   `dlr.routed`, clé `account_id`.
4. **DLR en avance** sur sa corrélation : `dlr.uncorrelated`, rejoué jusqu'à 5 min absolues, puis
   `dlr.unmatched` (compté, alerté, rejouable).
5. **Consommateurs de `dlr.routed`** : `delivery-svc` (remise) et `billing-svc` (mode `delivery`).

La fenêtre DLR vaut `DLR_RETENTION` (24 h), ou la `validity_period` si elle est plus longue, plafonnée
à 72 h. Sans DLR à l'échéance : `dlr_window_closed`. Un DLR plus tardif encore ne trouve plus de
corrélation : `dlr.unmatched`.

## 4.3 MO

La lane MO du routeur consomme `mo.inbound` :

1. **Réassemblage** d'un MO concaténé : tampon sur `redis-state`, par référence UDH, en un script
   atomique. Un groupe incomplet à l'échéance du TTL est écarté et compté : l'opérateur n'a jamais livré
   le message entier, ce n'est pas une perte au sens de I9.
2. **E.164** de l'émetteur.
3. **STOP.** Le premier token comparé aux mots-clés STOP du pays de référence. Un STOP inscrit un
   opt-out **scopé au numéro entrant** (ADR-0007), sous l'acteur `system:mo-stop` (I13) ; le trigger le
   propage au Bloom par le canal `optout`. Le MO est quand même remis, et jamais facturé.
4. **Résolution du compte.** `dest_addr → inbound_numbers` : dédié, son compte ; partagé,
   `inbound_keywords` sur le premier token, par priorité. Sans correspondance : compte de repli, sinon
   `unrouted`, visible dans la file « MO non routés » du dashboard. Jamais un abandon silencieux.
5. **Publication** sur `mo.routed`, clé `account_id`. La remise n'est jamais conditionnée à un solde.

**Comptage MO** (si la facturation est active) : `billing-svc` lit `mo.routed` et règle
`segment_count × credits_per_segment_mo` sur le solde MO, compteur postpayé sans enveloppe (ADR-0002,
décision 1). Il ne bloque rien.

## 4.4 Remise (`delivery-svc`)

Une lane par topic (`dlr.routed`, `mo.routed`), `tenant = account_id`, dépendance « endpoint client » en
`park_by_tenant`, `max_age = 24 h` depuis `origin_ts` (décision 4).

**Canal**, dans cet ordre :

1. un bind **actif** du compte en `receiver` ou `transceiver`, trouvé par `session-manager-svc` ;
   `deliver_sm` passé par le `smpp-server-svc` qui le tient ;
2. sinon, le webhook actif du compte pour ce type d'événement : `POST` JSON signé HMAC-SHA256 (secret
   scellé, ADR-0016), identifiant d'événement stable pour que le client dédoublonne ;
3. aucun moyen de réception déclaré : `Refuse(no_receiver)`, sans appel ni garage. Un DLR est compté et
   abandonné (l'état reste lisible par `GET /messages/{id}`) ; un MO va en dead-letter.

**Isolation** (I11) :

- **Client injoignable** : son disjoncteur s'ouvre, ses records vont au garage `lane_parking` sans
  appel ; une sonde par intervalle (présence du bind toutes les 5 s, ou appel du webhook) le referme ;
  le garage se vide dans l'ordre d'arrivée, à débit borné.
- **Client lent mais vivant** : `concurrency: per_tenant`, 8 envois en vol par compte au plus. L'offset
  commité ne dépasse jamais le plus ancien record non terminé.
- **Au-delà de 24 h** : abandon compté, jamais remis. La plupart des opérateurs ne garantissent plus un
  DLR au-delà ; nous non plus.

**Issue d'un MO** : `delivery-svc` publie `delivered_to_account` ou `delivery_failed` sur `mo.outcome`
(clé `message_id`), que `message-state-svc` inscrit au journal. L'issue de remise d'un DLR n'est pas
suivie : le DLR décrit un MT, dont l'état est déjà connu.

Le corps d'un MO quitte la plateforme ici, dans le `deliver_sm` ou le webhook, et nulle part ailleurs
(I1).

## 4.5 Doublons

Un client peut recevoir deux fois le même MO ou le même DLR : rejeu Kafka après un crash, ou webhook
reçu par le client mais dont la réponse s'est perdue. L'identifiant d'événement stable lui permet de
dédoublonner. Côté plateforme, la corrélation et la clé primaire du journal absorbent les doublons.

## 4.6 Pannes

| Panne | Effet |
|---|---|
| `message-state-svc` | Les DLR s'accumulent dans `dlr.events`, sans perte ; remise retardée |
| Endpoint d'un client | Garage de ce client seul, 24 h au plus |
| `session-manager-svc` | Bind introuvable : traité comme endpoint en panne, garage |
| `redis-state` | Réassemblage MO en `blocking` : la lane MO attend, rien n'est perdu |
| Postgres `control_plane` | Instantanés servis ; l'inscription d'un STOP attend (dépendance `blocking` de la lane MO) |
| Fournisseur HTTP qui rejoue un callback | La corrélation absorbe le doublon |
