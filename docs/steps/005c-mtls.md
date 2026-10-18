# Step 005c — Bibliothèques : mTLS

Plan : `docs/plan/socle.md` §005, troisième des cinq PR. Spec : `docs/design/06-confiance-interne.md`
(D2, D3, §4, §9), `docs/spec/02-architecture.md` §2.5, `docs/spec/09-contrats.md` §9.2. Mode : PR.

## Contexte

Portée de `go-gateway` (`internal/platform/tlsconf`) et réduite à ce que le plan demande : le mTLS
interne, serveur et client, depuis la CA interne. go-gateway admettait toute la CA sur une allowlist
vide, et pouvait tourner en clair (`TLS.Enabled=false`) ; la décision 6 supprime l'un et l'autre.

**Terminé quand** (part de 005c) : le serveur et le client ont leurs tests ; le refus d'une allowlist
vide et d'un appelant non nommé ont été prouvés par une mutation.

## Arbitrages

Fable a tranché le 2026-10-18 :

- **Allowlist vide refusée partout**, sans notion d'environnement : admise hors production, elle
  admettrait toute la CA, ce que §4 supprime. Amendé : D3, §4 et §9 de la décision 6, spec 02 §2.5, plan
  §005 (« vide = échec du démarrage », sans « en production »).
- **Identité** : l'allowlist est comparée aux SAN DNS exacts du certificat client, nom de service nu
  (colonne « appelants admis » du §2.2), jamais au CN, qui reste le principal Kafka. Amendé : décision 6
  §4, spec 02 §2.5. Les `dnsNames` des certificats sont l'affaire du déploiement (007) ; le client fixe
  `ServerName` au nom nu du serveur.
- **Rotation à chaud** : les fichiers sont relus à chaque handshake, derrière un cache (taille et date de
  modification du certificat et de la CA ; une clé neuve vient toujours avec un certificat neuf) : `GetConfigForClient` côté serveur (`ClientCAs` ne se rafraîchit pas
  autrement), `GetClientCertificate` côté client. Sans elle, un renouvellement de cert-manager exige un
  redémarrage. Le client garde `RootCAs` figé : `crypto/tls` n'a pas de rappel pour lui, une rotation de
  CA est un redémarrage (runbook, 007). Pas d'avertissement d'expiration (l'alerte cert-manager à 7 jours,
  §10.1, le porte) ni de CA changée. **Une relecture qui échoue sert la dernière génération valide** (une
  rotation lue entre l'écriture du certificat et celle de la clé, un fichier absent un instant) ; seule la
  construction refuse des fichiers illisibles. L'allowlist et la vérification de chaîne s'appliquent
  toujours. La CA d'un pair n'est pas bornée ici : pas d'intermédiaire, ou des contraintes de nom, et des
  `usages` explicites relèvent des certificats (007).
- **Aucun mode en clair** (§9.2 : « toujours sous mTLS ») ; les tests génèrent leur CA.
- **Périmètre** : un `*tls.Config` serveur et un client, rien d'autre. La configuration publique
  (REST, SMPP) vient avec le premier composant public, celle des magasins avec leurs bibliothèques, les
  aides gRPC avec le premier serveur gRPC. Le serveur reçoit la liste ALPN complète de l'appelant :
  `GetConfigForClient` remplace `NextProtos`, et net/http ne voit pas la sienne.
- **Forme** : TLS 1.3 minimum des deux côtés ; l'allowlist dans `VerifyConnection` (gosec G123), rappelé
  sur une session reprise ; l'allowlist copiée à la construction.
- **Aide de test** : CA et certificats générés au vol, privés au test du paquet.
- **gosec G402** : le test dresse un pair en TLS 1.2 pour prouver qu'il est refusé ; exclusion nommée du
  seul test dans `.golangci.yml`.
- La feuille est lue sans garde dans la chaîne vérifiée : `RequireAndVerifyClientCert` en garantit une, y
  compris sur une reprise.

## Design arrêté

- `internal/platform/mtls` : `Files{Cert, Key, CA string}` ; `(Files) Server(allowed, nextProtos
  []string) (*tls.Config, error)`, qui refuse une allowlist vide et des fichiers illisibles ;
  `(Files) Client() (*tls.Config, error)`.
- Vérification du critère : allowlist vide et fichiers illisibles refusés à la construction ; un appelant
  nommé admis, y compris par un SAN qui n'est pas le premier ; un appelant non nommé (même si son CN
  l'est), sans certificat ou d'une autre CA refusé pour cette raison ; une relecture qui échoue sert la
  génération précédente ; une CA ajoutée sur disque admise par le serveur sans redémarrage ; un certificat renouvelé sur disque servi sans redémarrage, côté serveur et côté client ;
  TLS 1.2 refusé des deux côtés ; l'allowlist copiée ; la liste ALPN servie.
