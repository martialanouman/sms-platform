-- name: CreateOperator :one
INSERT INTO dashboard.operators (email, display_name) VALUES (@email, @email)
ON CONFLICT ((lower(email))) DO NOTHING
RETURNING id::text AS id;

-- name: OperatorByEmail :one
SELECT id::text, coalesce(password_hash, '')::text AS password_hash, status = 'active' AS active
FROM dashboard.operators WHERE lower(email) = lower(@email);

-- name: SetPassword :exec
UPDATE dashboard.operators SET password_hash = @phc WHERE id = @id::uuid;

-- name: QueueLink :exec
INSERT INTO dashboard.access_links (operator_id, kind) VALUES (@operator_id::uuid, @kind)
ON CONFLICT (operator_id) DO UPDATE SET kind = excluded.kind, token_hash = NULL, expires_at = NULL, sent_at = NULL,
  attempts = 0, next_attempt_at = now();

-- name: ConsumeLink :one
UPDATE dashboard.access_links l SET token_hash = NULL
FROM dashboard.operators o
WHERE l.token_hash = @digest AND l.expires_at > now() AND o.id = l.operator_id AND o.status = 'active'
RETURNING l.operator_id::text AS operator_id;

-- name: NextDue :one
SELECT l.operator_id::text AS operator_id, o.email, l.kind
FROM dashboard.access_links l JOIN dashboard.operators o ON o.id = l.operator_id
WHERE l.sent_at IS NULL AND l.attempts < @max_attempts::bigint AND l.next_attempt_at <= now() AND o.status = 'active'
ORDER BY l.next_attempt_at
LIMIT 1
FOR UPDATE OF l SKIP LOCKED;

-- name: MarkSent :exec
UPDATE dashboard.access_links
SET token_hash = @digest, expires_at = now() + make_interval(secs => @ttl_seconds::float8), sent_at = now()
WHERE operator_id = @operator_id::uuid;

-- name: MarkFailed :exec
UPDATE dashboard.access_links
SET next_attempt_at = now() + make_interval(secs => least(sqlc.arg(base_seconds)::float8 * power(2, attempts),
    sqlc.arg(ceiling_seconds)::float8)),
  attempts = attempts + 1
WHERE operator_id = @operator_id::uuid;

-- name: Undeliverable :one
SELECT count(*)::bigint
FROM dashboard.access_links l JOIN dashboard.operators o ON o.id = l.operator_id
WHERE l.sent_at IS NULL AND l.attempts >= @max_attempts::bigint AND o.status = 'active';

-- name: Reserve :one
INSERT INTO dashboard.login_attempt_counters AS c (subject, failures, last_failure_at) VALUES (@subject, 1, now())
ON CONFLICT (subject) DO UPDATE SET
  failures = CASE WHEN c.last_failure_at + make_interval(secs => @window_seconds::float8) <= now() THEN 1 ELSE c.failures + 1 END,
  last_failure_at = now()
WHERE c.last_failure_at + make_interval(secs => @window_seconds::float8) <= now() OR c.failures < @max_failures::bigint
RETURNING true;

-- name: LockedFor :one
SELECT extract(epoch FROM last_failure_at + make_interval(secs => @window_seconds::float8) - now())::float8
FROM dashboard.login_attempt_counters WHERE subject = @subject;

-- name: Forget :exec
DELETE FROM dashboard.login_attempt_counters WHERE subject = @subject;

-- name: OpenSession :one
INSERT INTO dashboard.sessions (operator_id, token_hash, expires_at, challenge_hash, challenge_expires_at)
VALUES (@operator_id::uuid, @digest, now() + make_interval(secs => @lifetime_seconds::float8), @challenge,
  now() + make_interval(secs => @challenge_seconds::float8))
RETURNING challenge_expires_at::timestamptz;

-- name: SessionOf :one
SELECT s.id::text AS id, s.operator_id::text AS operator_id, o.email, o.display_name, s.expires_at,
  (s.elevated_at IS NOT NULL)::bool AS elevated,
  EXISTS (SELECT 1 FROM dashboard.totp_factors f WHERE f.operator_id = s.operator_id AND f.confirmed_at IS NOT NULL) AS totp,
  (SELECT count(*) FROM dashboard.recovery_codes c JOIN dashboard.totp_factors f ON f.id = c.factor_id
    WHERE f.operator_id = s.operator_id AND f.confirmed_at IS NOT NULL AND c.used_at IS NULL)::int AS recovery_codes_remaining,
  (now() < s.expires_at AND now() < s.last_seen_at + make_interval(secs => @idle_seconds::float8) AND o.status = 'active')::bool AS live
FROM dashboard.sessions s JOIN dashboard.operators o ON o.id = s.operator_id
WHERE s.token_hash = @digest;

-- name: Touch :execrows
UPDATE dashboard.sessions s SET last_seen_at = now()
FROM dashboard.operators o
WHERE s.token_hash = @digest AND o.id = s.operator_id AND o.status = 'active'
  AND now() < s.expires_at AND now() < s.last_seen_at + make_interval(secs => @idle_seconds::float8);

-- name: CloseSession :one
DELETE FROM dashboard.sessions WHERE token_hash = @digest RETURNING operator_id::text;

-- name: CloseSessions :exec
DELETE FROM dashboard.sessions WHERE operator_id = @operator_id::uuid;

-- name: PurgeSessions :exec
DELETE FROM dashboard.sessions
WHERE expires_at <= now() OR last_seen_at + make_interval(secs => @idle_seconds::float8) <= now();

-- name: PurgeCounters :exec
DELETE FROM dashboard.login_attempt_counters
WHERE last_failure_at + make_interval(secs => @window_seconds::float8) <= now();

-- name: Log :exec
INSERT INTO dashboard.access_log (actor, event, target, outcome)
VALUES (sqlc.narg(actor)::uuid, @event, sqlc.narg(target), CASE WHEN @success::bool THEN 'success' ELSE 'failure' END);

-- name: AccessLog :many
SELECT id::text, at, coalesce(actor::text, '')::text AS actor, event, coalesce(target, '')::text AS target, outcome = 'success' AS success
FROM dashboard.access_log
WHERE sqlc.narg(before_at)::timestamptz IS NULL OR (at, id) < (sqlc.narg(before_at)::timestamptz, sqlc.narg(before_id)::uuid)
ORDER BY at DESC, id DESC
LIMIT @page_size::bigint;

-- name: CurrentStep :one
SELECT floor(extract(epoch FROM now()) / 30)::bigint;

-- name: Factors :many
SELECT id::text AS id, sealed, last_step, (confirmed_at IS NOT NULL)::bool AS confirmed
FROM dashboard.totp_factors WHERE operator_id = @operator_id::uuid
ORDER BY confirmed_at NULLS LAST
FOR UPDATE;

-- name: DropPending :exec
DELETE FROM dashboard.totp_factors WHERE operator_id = @operator_id::uuid AND confirmed_at IS NULL;

-- name: AddPending :one
INSERT INTO dashboard.totp_factors (operator_id, sealed) VALUES (@operator_id::uuid, @sealed) RETURNING id::text;

-- name: AddRecoveryCodes :copyfrom
INSERT INTO dashboard.recovery_codes (factor_id, digest) VALUES ($1, $2);

-- name: ConsumeStep :exec
UPDATE dashboard.totp_factors SET last_step = @step WHERE id = @id::uuid;

-- name: DropConfirmed :execrows
DELETE FROM dashboard.totp_factors WHERE operator_id = @operator_id::uuid AND confirmed_at IS NOT NULL;

-- name: Confirm :exec
UPDATE dashboard.totp_factors SET confirmed_at = now() WHERE id = @id::uuid;

-- name: UseRecoveryCode :execrows
UPDATE dashboard.recovery_codes SET used_at = now()
WHERE factor_id = @factor_id::uuid AND digest = @digest AND used_at IS NULL;

-- name: DropFactors :execrows
DELETE FROM dashboard.totp_factors WHERE operator_id = @operator_id::uuid;

-- name: Elevate :execrows
UPDATE dashboard.sessions s SET token_hash = @new_digest, elevated_at = now(), last_seen_at = now(),
  challenge_hash = NULL, challenge_expires_at = NULL
FROM dashboard.operators o
WHERE s.token_hash = @digest AND s.challenge_hash = @challenge AND s.challenge_expires_at > now()
  AND o.id = s.operator_id AND o.status = 'active'
  AND now() < s.expires_at AND now() < s.last_seen_at + make_interval(secs => @idle_seconds::float8);
