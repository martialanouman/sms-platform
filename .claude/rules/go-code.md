---
paths:
  - "internal/**/*.go"
  - "cmd/**/*.go"
---

# Code Go — ce qu'un compilateur vert ne garantit pas

- **Le corps d'un message ne sort jamais** (I1) : type `Body` qui ne se sérialise pas, `Reveal()`
  audité pour le clair. Un DTO de sortie est un struct déclaré, jamais une `map` ni un type de domaine.
- **Aucune goroutine sans condition d'arrêt.**
- **Redis : une opération atomique est un script Lua**, jamais un read-modify-write en Go.
- **Un secret : le vérifie-t-on ou le rejoue-t-on ?** Vérifié (clé d'API, mot de passe de bind
  entrant) → haché, montré une fois. Rejoué vers un tiers (bind sortant, fournisseur HTTP, webhook) →
  scellé par `ConfigSecrets`, jamais haché. Dans les deux cas il ne ressort pas de l'API (I2).
- **Un échec sur le chemin chaud se classe par liste de refus** : seuls les codes déclarés définitifs
  refusent ; tout le reste est transitoire. `codes.Internal` est ambigu côté client.
- **Le métier vit sous `internal/`** ; `cmd/<service>/main.go` ne fait que câbler. Interfaces côté
  consommateur.
- **Une garde descendue dans le code partagé** exige que chaque appelant tire la même conséquence de
  l'absence. **Resserrer une porte** oblige à vérifier les portes voisines.
- **Une absence se prouve par une recherche complète**, jamais par un `head -N` ni un glob avorté.
- **Un commentaire qui décrit un mécanisme** se relit contre le code ; il en invente souvent un.

## Pièges de bibliothèques, vérifiés

- **franz-go** : le record timeout ne borne pas un lot en vol ; un commit tardif peut faire reculer
  l'offset du nouveau propriétaire ; `UniformBytesPartitioner` panique hors client.
- **HTTP/TLS** : annoncer `h2` annule `ReadHeaderTimeout` ; `resp.Proto` ne prouve aucune ALPN ;
  `crypto/tls` côté client n'a pas de `GetConfigForClient` ; go-oidc recharge le JWKS sans limite et
  aplatit ses erreurs en `%v`.
- **huma** : n'applique pas le `security:` global (une opération sans scope est **publique**) ; injecte
  500/default et `$schema`.
- **Scripts** : gopher-lua `SetMx` fait `os.Exit` ; goja n'a pas de compteur d'instructions, seul le mur
  horaire borne.

La doc d'une bibliothèque se lit par `ctx7` avant d'écrire : un usage périmé compile parfois (pgx v4→v5,
huma v1→v2).
