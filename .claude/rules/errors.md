---
paths:
  - "internal/platform/errors/**"
---

# Codes d'erreur

Un `code` est un contrat partagé : réponse REST, `command_status` SMPP, `error_code` de l'état et du
CDR. En ajouter un touche, **ensemble** :

1. le catalogue Go (sentinelle et mapping HTTP/SMPP) ;
2. l'`enum` du champ `code` des contrats OpenAPI concernés (un test garde la correspondance) ;
3. la table de `docs/spec/09-contrats.md` §9.3.

Modèle plat `{ code, message, errors[] }`. Un refus d'authentification SMPP répond `ESME_RINVPASWD`,
jamais `ESME_RINVSYSID` : il révélerait les `system_id` existants.
