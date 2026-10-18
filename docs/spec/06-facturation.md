# 6. Facturation

Optionnelle : désactivée, aucun de ces composants n'existe et l'étape de réservation est un no-op
(drapeau global, puis `customers.billing_enabled`). Détail : décision 1.

## 6.1 Modèle

- **Crédits entiers**, jamais monétaires.
- **Deux directions** (ADR-0002). Le solde **MT** bloque. Le solde **MO** est un compteur postpayé qui
  ne bloque rien : un MO est toujours remis, toujours compté jusqu'à `mo_billing_floor`, puis alerte
  `mo_balance_floor_reached`. Un STOP n'est jamais compté.
- **Propriétaire** : `customers.balance_scope` = `customer` (défaut, pool partagé par ses comptes) ou
  `account` (un solde par compte). La bascule exige tous les soldes du client à zéro et aucune
  enveloppe ouverte. En `account`, ni découvert ni limite dure : le plancher est 0.
- **Formule** : `credits = segment_count × credits_per_segment(destination, sender_type)`, depuis le plan
  tarifaire du client. Le routeur l'évalue sur son instantané ; le résultat voyage dans le fait et n'est
  jamais recalculé.
- **Modes de paiement** : prépayé (plancher 0 ou `-overdraft_limit`), postpayé à limite dure (plancher
  `-credit_limit`), postpayé à limite souple (alerte seulement).
- **`customers.charge_on`** ∈ {`submit`, `accept`, `delivery`}, défaut `accept`.

**Seule autorité du solde : `billing-svc`**, au moins 2 répliques. Redis n'est pas sur le chemin de la
facturation.

## 6.2 Enveloppes (MT bloquant)

Prépayé et postpayé à limite dure. Les autres cas se règlent depuis les faits, sans enveloppe.

1. **Octroi.** Au premier message d'un propriétaire, puis en avance sous le seuil θ = S/2, le pod du
   routeur demande une enveloppe. `billing-svc` l'inscrit en une transaction (`credit_leases` et une
   ligne `lease_grant` au grand livre), par commit de groupe toutes les 10 à 20 ms, idempotent par
   `request_key`. Le bail est rendu en durée (30 s), converti sur l'horloge monotone du pod.
2. **Consommation** en mémoire à l'étape 11 du pipeline : aucun appel réseau quand l'enveloppe couvre
   le message. Chaque record `mt.routed` porte `lease_id`, crédits, `charge_on` et version du plan.
3. **Clôture.** Le compteur du pod ne fait jamais foi ; la vérité est la somme des faits dédoublonnés.
   Propre (drain, ou 10 s sans consommation) : le pod transmet `fence_offsets`, `billing-svc` règle quand
   son consommateur les a dépassés. Pod mort : il s'interdit toute consommation après
   `expires_at − 5 s`, `billing-svc` règle après `expires_at + 30 s`.
4. **Règlement** : `lease_refund` restitue `granted − Σ crédits consommés non libérés` ; l'enveloppe
   passe `settled`.

**Taille** : `S = max(S_min, r × 2 s)`, r = débit observé du propriétaire sur le pod, plafonné à R/P.
**Solde bas** : sous P × S, les octrois rétrécissent à A / P ; à zéro, plus rien n'est accordé.
**Borne** (I5) : dépassement ou faux refus ≤ T × R, T = 1 s, R = débit plafond du propriétaire en
crédits/s.

## 6.3 Règlement des segments

`billing-svc` lit `mt.routed.*` sans toucher au corps et écrit `billing_settlements`, clé primaire
`(message_id, segment_seq)`, `ON CONFLICT DO NOTHING` (I4). Il lit les issues sur `mt.outcome` et,
en mode `delivery`, sur `dlr.routed` (ADR-0024). Le mode appliqué est celui que le fait a figé.

| Issue du segment | `submit` | `accept` | `delivery` |
|---|---|---|---|
| ROK | capturé | capturé | en attente |
| Erreur, ou pas de réponse | capturé | libéré | libéré |
| `not_sent` (annulation, dead-letter, repli épuisé, expiré) | libéré | libéré | libéré |
| DLR `delivered` | — | — | capturé |
| DLR d'échec | — | — | libéré |
| Pas de DLR à 24 h | — | — | capturé, « sans DLR » |

Le statut de règlement d'un segment se **déduit** de cette table, de son issue et du `charge_on` du
fait : la projection CDR le calcule elle-même, sans flux de facturation dédié.

## 6.4 Grand livre

Seuls les mouvements du solde : `lease_grant`, `lease_refund`, `late_release` (libération arrivée après
le règlement de l'enveloppe), `charge` (MT postpayé à limite souple, depuis les faits), `mo_charge`,
`topup`, `transfer`, `adjustment`. `SUM(grand livre) == solde` (I5).
Les écritures hors flux passent par l'API gRPC de `billing-svc`, appelée par `admin-api-svc` ; personne
d'autre n'écrit le grand livre. Le solde durable est replié en différé à partir du grand livre
(ADR-0022). Partitionné par jour, conservé 13 mois et plus.

## 6.5 Facturation injoignable

Les pods consomment les enveloppes qu'ils tiennent (environ 2 s), puis **garent** : un message sans
enveloppe dont l'octroi échoue faute de facturation part sur `mt.billing-wait.<cat>` (`park_global`).

- Il ne bloque ni sa lane ni les propriétaires non facturés.
- Il est rejoué dès qu'un octroi réussit à nouveau.
- À l'âge maximal de sa catégorie (chapitre 3 §3.6), il est rejeté `billing_unavailable`, non facturé.
- Jamais envoyé sans crédit, jamais rejeté avant l'âge maximal.

## 6.6 Pannes

| Panne | Effet |
|---|---|
| Crash du pod pendant un octroi | L'enveloppe orpheline expire et est restituée |
| Redémarrage d'un pod | Nouvelle epoch ; les enveloppes de l'ancienne suivent la règle du pod mort |
| Retard du consommateur de faits | Clôtures retardées, plus de faux refus près de zéro ; alerté, jamais de dépassement |
| Décalage d'horloge | Aucun effet : bail en durée monotone, marge de 5 s |
| Déploiement de `billing-svc` | Aucun effet : HA, octroi transactionnel sans état local |
| `billing-svc` ou Postgres | Enveloppes en cours, puis garage (§6.5) |
| Segment sans issue | `outcome_missing` après 1 h : alerte, jamais de libération automatique |

## 6.7 Critère

Sur le banc de charge, un seul propriétaire tient **8 000 SMS/s traversants**.
