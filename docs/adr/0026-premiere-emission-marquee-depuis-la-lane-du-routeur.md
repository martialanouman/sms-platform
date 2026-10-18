# ADR-0026 : La première émission d'un sender ID est marquée depuis la lane du routeur

**Status:** Accepted
**Date:** 2026-10-18
**Deciders:** Demande humaine du 18/10/2026 (step 004b), sur recommandation de Fable.
**Réf spec:** spec 02 §2.2 ; spec 09 §9.1 ; I9. Amende ADR-0023 (§2 et §3).

## Context

L'ADR-0023 pose `sender_ids.first_used_at` depuis un groupe de router-svc sur `mt.outcome`. Dans le
monorepo, il ne s'applique plus tel quel :

- ni la spec 02 §2.2 ni le catalogue (`contracts/topics.yaml`) ne connaissent ce groupe ;
- l'adresse à marquer est l'adresse **soumise** (ADR-0023 §4), que `MtOutcome` ne porte pas : le pool ne
  connaît que l'adresse qu'il a peut-être réécrite ;
- son §3 suppose que les échecs sans envoi écrivent le CDR directement, ce que la décision 2 a supprimé :
  tout passe par le journal.

## Decision

1. **La marque se pose dans la lane MT du routeur, à la publication de `routed`.** Le routeur détient
   l'adresse soumise, celle que l'autorisation vient de vérifier (étape 6 du pipeline).
2. **Par lot, en différé**, avec l'instruction idempotente de l'ADR-0023 §2
   (`UPDATE … FROM unnest(…) WHERE first_used_at IS NULL`), jamais sur le chemin d'un message : la lane ne
   l'attend pas.
3. Aucun groupe de consommateurs sur `mt.outcome`, aucun champ de payload de plus.
4. Restent valables de l'ADR-0023 : la colonne (§1), l'adresse soumise (§4), la suppression atomique (§5).

## Consequences

- Un message `routed` part ou finit terminal (I9) : la marque a la même portée qu'à l'issue d'envoi. Un
  sender ID routé dont tous les envois échouent avant l'opérateur est marqué quand même ; il a été
  autorisé et tenté, et ne se supprime plus.
- Le routeur écrit dans `control_plane` (`sender_ids`), sous son acteur `service:router-svc` (I13), comme
  pour la table `unrouted_mo`.
- La dette « retard de la marque non publié » de l'ADR-0023 tombe : la marque suit la lane, dont le lag
  est publié.
