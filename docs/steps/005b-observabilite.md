# Step 005b — Bibliothèques : observabilité

Plan : `docs/plan/socle.md` §005, deuxième des cinq PR. Spec : `docs/spec/10-exploitation.md` §10.1,
`docs/spec/01-exigences.md` (observabilité, traçage, volumétrie des traces), I1. Mode : PR.

## Contexte

Portée de `go-gateway` (`internal/observability`) et adaptée à la spec. La configuration d'environnement
(005d), le mTLS (005c), `/metrics` et `/readyz` avec le drain (005d) restent hors de cette PR.

**Terminé quand** (part de 005b) : le logger, la garde des labels et les traces ont leurs tests ; la
garde des labels et l'échantillonnage des échecs ont été prouvés par une mutation.

## Arbitrages

Fable a tranché le 2026-10-18, sans amendement de spec :

- **Garde des labels, à l'enregistrement** (§10.1) : un `prometheus.Registerer` qui lit les noms exacts
  des labels variables et constants de chaque `Desc` en construisant une métrique constante
  (`NewConstMetric`, autant de valeurs vides que de labels variables, trouvées par essais successifs) et
  en l'écrivant (`Write`). Il refuse un collecteur qui ne décrit rien ; un `Desc` invalide, le registre le
  refuse lui-même, avec sa cause. La garde
  enveloppe le registre lui-même : un `WrapRegistererWith` posé entre elle et le registre ajoute des
  labels qu'elle ne voit pas. Pas de contrôle au `Gather` : il ne couvrirait qu'un collecteur maison qui
  déclare un label et en émet un autre, absent ici.
- **Liste de refus seule**, sans liste blanche (§10.1). Elle ne lit que les noms : c'est un filet contre
  un label nommé d'après le champ qu'il porte, pas une barrière ; on y met les noms qu'on écrirait ici.
  Un nom qui n'est pas en snake_case minuscule (`^[a-z][a-z0-9_]*$`, la convention Prometheus, que
  suivent les collecteurs standard) est refusé : sans cela, `messageId`, `phoneNumber` ou `msisdn}}`
  passeraient la liste. Sont refusés : un nom entier `to`, `from`, `trace_id`, `span_id` (un `trace_id`
  par message, §10.1), `customer_group`, `client_group` ; un nom qui finit par `message_id`, `msg_id`,
  `messageid` ou `msgid` (`smsc_message_id`, `dlr_msg_id`) ; un nom dont un segment `_`, ou son singulier
  (`-s`, `-es`, `-ies`), est `msisdn`, `phone`, `number`, `addr`, `address`, `recipient`, `destination`,
  `body`, `text`, `content` ou `payload`.
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
- **Revue** : le rendu `String()` d'un `Desc`, lu d'abord, n'échappe pas les noms de labels (un label
  constant nommé `lane="msisdn",code` s'y lisait comme deux labels admis) ; la lecture par une métrique
  constante le remplace, et l'analyseur disparaît. Troisième tour, arbitré par l'humain : les
  identifiants de message par suffixe, `address` et les pluriels en `-es` et `-ies` entrent dans la
  liste ; le refus propre d'un `Desc` invalide, doublon du registre, disparaît.

## Design arrêté

- `internal/platform/metrics` : `Guard(prometheus.Registerer) prometheus.Registerer`, garde de
  `Register` et `MustRegister`.
- `internal/platform/observability` : `NewLogger(io.Writer, service string, slog.Level) *slog.Logger` ;
  `InitTracing(sdktrace.SpanExporter, service string, ratio float64) *sdktrace.TracerProvider`, qui pose
  le fournisseur et le propagateur globaux (`debt: #93`) ; `RecordError(trace.Span, error)`.
- Dépendances directes : `client_golang`, `client_model`, `otel`, `otel/sdk`, `otel/trace`.
- Vérification du critère : chaque nom, suffixe et segment refusé, chaque forme de pluriel, un nom hors
  snake_case (camelCase, premier caractère, nom constant qui imite un rendu), un collecteur muet et un
  label constant rouges ; une erreur du registre remonte ; un span en erreur non échantillonné exporté, un
  span réussi ou `Ok` non échantillonné non exporté ; le texte d'une erreur jamais sur un span.
