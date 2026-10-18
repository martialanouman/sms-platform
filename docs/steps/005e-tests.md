# Step 005e — Bibliothèques : tests

Plan : `docs/plan/socle.md` §005, dernière des cinq PR. Règles : `.claude/rules/tests.md` (Intégration).
Mode : PR.

## Contexte

go-gateway avait un `tcpproxy` générique, et des `Cuttable` propres à chaque magasin (`pgtest`,
`redistest`) : un conteneur derrière le proxy, et la configuration du client du magasin pointée sur lui.
Aucun magasin n'est câblé ici avant la step 006.

**Terminé quand** (part de 005e) : `tcpproxy` a ses tests, en processus et devant un vrai conteneur en
CI ; la coupure et la reprise ont été prouvées par une mutation.

## Arbitrages

Fable a tranché le 2026-10-18 :

- **Périmètre** : `tcpproxy` seul. Un `Cuttable` construit le client d'un magasin : il arrive avec le
  magasin (006 pour Postgres). Amendé : plan §005. Aucune enveloppe générique de testcontainers, qui
  entre dans `go.mod` par le test de preuve.
- **Forme** : `tcpproxy.New(t, upstream)`, `Addr`, `Cut` (ferme les connexions vives et refuse les
  nouvelles : la connexion aboutit, puis tombe), `Resume`, `Close` (enregistré en `t.Cleanup`). `Close`
  n'attend pas les relais : chacun s'arrête de lui-même, son client fermé ou sa composition vers l'amont
  échue (3 s) ; l'attendre faisait payer ces 3 s au nettoyage d'un test sur un amont injoignable.
- **Docker et Ryuk** : aucune bibliothèque. La step 002 a placé la garde sur la sortie de `go test -v`
  (`ciguard` refuse tout `--- SKIP` en CI) ; en local sans Docker, le test d'intégration saute si le
  fournisseur Docker de testcontainers ne répond pas (`SkipIfProviderIsNotHealthy`). Ryuk reste au défaut de testcontainers, la CI ne
  le désactive pas et la règle l'interdit. « Socket exporté » est de la documentation, déjà dans
  `tests.md` : rien à coder, aucune lecture d'environnement.
- **Preuve** : un test en processus, sur un écho `net.Listen`, qui porte les mutations ; un test
  d'intégration sur `redis:8-alpine`, pingé en TCP brut à travers le proxy : `Cut`, un second client
  direct répond toujours, le client à travers le proxy échoue, `Resume`, il répond de nouveau.
- **Emplacement** : `internal/platform/tcpproxy` (ADR 0025).
- **Pas de build tag** : le saut reste visible et gardé ; la dette #2 n'est pas touchée.

## Design arrêté

- `internal/platform/tcpproxy` : `New(testing.TB, upstream string) *Proxy`, `(*Proxy) Addr() string`,
  `Cut()`, `Resume()`, `Close()`.
- `go.mod` : `testcontainers-go`, pour le test d'intégration.
- Vérification du critère : en processus, des octets traversent, `Cut` ferme les connexions vives et
  fait tomber les nouvelles, `Resume` rétablit, `Close` libère tout ; devant Redis, la coupure se lit
  par un second client non coupé.
