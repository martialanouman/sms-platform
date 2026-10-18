# Step 005b — Bibliothèques : observabilité

Plan : `docs/plan/socle.md` §005, deuxième des cinq PR. Spec : `docs/spec/10-exploitation.md` §10.1,
`docs/spec/01-exigences.md` (observabilité, traçage, volumétrie des traces), I1. Mode : PR.

## Contexte

Portée de `go-gateway` (`internal/observability`) et adaptée à la spec : la liste blanche des labels,
la configuration, le serveur d'exploitation et l'exportateur OTLP restent hors de cette PR. La
configuration d'environnement arrive en 005d, le mTLS en 005c, `/metrics` et `/readyz` avec le drain en
005d.

**Terminé quand** (part de 005b) : le logger, la garde des labels et les traces ont leurs tests ; la
garde des labels et l'échantillonnage des échecs ont été prouvés par une mutation.

## Arbitrages

Fable a tranché le 2026-10-18, sans amendement de spec :

- **Garde des labels, à l'enregistrement** (§10.1) : un `prometheus.Registerer` qui lit les labels
  variables et constants de chaque `Desc` par `String()` (la seule voie que `client_golang` v1.23
  expose), ancré sur la dernière occurrence de `variableLabels: {`. Il refuse un collecteur qui ne
  décrit rien (invérifiable) et un `Desc` qu'il ne sait pas lire. Un test épingle le format de
  `String()` : une montée de version qui le change casse en CI. Le refus d'un `Desc` illisible reste au
  démarrage, en second rempart ; aucun `Desc` réel ne s'écrit autrement, sa mutation est donc équivalente. Pas de contrôle au
  `Gather` : il ne couvrirait qu'un collecteur maison qui déclare un label et en émet un autre, absent ici.
- **Liste de refus seule**, sans liste blanche (§10.1). Un nom, en minuscules, est refusé si lui-même ou
  l'un de ses segments `_` est dans la liste. Segments : `msisdn`, `phone`, `number`, `addr`,
  `recipient`, `destination`, `body`, `text`, `content`, `payload`. Noms entiers : `to`, `from`,
  `message_id`, `msg_id`, `trace_id`, `span_id` (un `trace_id` par message, §10.1), `customer_group`,
  `client_group`.
- **Exportateur injecté** : la bibliothèque construit le `TracerProvider` depuis un `SpanExporter`.
  L'exportateur OTLP et son transport sont de la composition (`cmd/`) et attendent 005c et 005d :
  dette #93.
- **Échantillonnage** : tête `AlwaysRecord(ParentBased(TraceIDRatioBased(ratio)))`, et un processeur qui
  exporte aussi un span non échantillonné dont le statut est `Error`, ré-estampillé échantillonné. La
  trace entière d'un message en échec et les seuils de durée relèvent de la queue, au collecteur (007).
  « Délai » est un code d'erreur d'expiration posé sur le span (`message_expired`, `delivery_expired`,
  `ambiguous_submit`). Coût mesuré dans go-gateway : tous les spans sont enregistrés, +247 ns et
  +953 o par span ; baisser le ratio économise la bande passante, pas le CPU.
- **Erreur de span** : seul le `Code` porté par l'erreur (statut et événement), repli `internal_error` ;
  jamais le texte de l'erreur, qui peut porter un MSISDN ou un hôte.
- **Logger** : JSON `slog`, `trace_id` et `span_id` tirés du contexte quand il porte un span valide,
  attribut `service`, niveau en paramètre. Sous `WithGroup`, `trace_id` tombe dans le groupe : personne
  n'en a besoin. « Jamais un secret » est l'affaire de `ConfigSecrets`, pas du logger.
- **Propagation** : `TraceContext` seul, posé globalement avec le `TracerProvider`. Pas de `Baggage` :
  rien ne le consomme, et c'est un canal clé-valeur inter-services, donc une fuite possible. Le porteur
  d'en-têtes Kafka vient avec la bibliothèque Kafka.

## Design arrêté

- `internal/platform/metrics` : `Guard(prometheus.Registerer) prometheus.Registerer`, garde de
  `Register` et `MustRegister`.
- `internal/platform/observability` : `NewLogger(io.Writer, service string, slog.Level) *slog.Logger` ;
  `InitTracing(sdktrace.SpanExporter, service string, ratio float64) *sdktrace.TracerProvider`, qui pose
  le fournisseur et le propagateur globaux (`debt: #93`) ; `RecordError(trace.Span, error)`.
- Dépendances directes : `client_golang`, `otel`, `otel/sdk`, `otel/trace`.
- Vérification du critère : chaque segment et nom refusé, un collecteur muet, un label constant et un
  `Desc` illisible rouges ; un span en erreur non échantillonné exporté, un span réussi non échantillonné
  non exporté ; un `Body` dans un log, un attribut ou une erreur de span ne sort pas en clair.
