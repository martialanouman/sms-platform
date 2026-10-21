-- name: Begin :exec
SELECT set_config('app.actor', sqlc.arg(actor)::text, true), set_config('app.request_id', sqlc.arg(request_id)::text, true);

-- name: Record :exec
INSERT INTO control_plane.audit_log (operator, operation_id, target, request_id, status, outcome, at)
VALUES (sqlc.arg(operator), sqlc.arg(operation_id), sqlc.arg(target), sqlc.narg(request_id), sqlc.arg(status)::bigint,
  sqlc.narg(outcome), clock_timestamp());

-- name: AuditLog :many
SELECT id, operator, operation_id, target, request_id, status, outcome, at
FROM control_plane.audit_log
WHERE (sqlc.narg(operator)::text IS NULL OR operator = sqlc.narg(operator))
  AND (sqlc.narg(from_date)::timestamptz IS NULL OR at >= sqlc.narg(from_date))
  AND (sqlc.narg(to_date)::timestamptz IS NULL OR at < sqlc.narg(to_date))
  AND (sqlc.narg(after_at)::timestamptz IS NULL OR (at, id) < (sqlc.narg(after_at), sqlc.narg(after_id)::uuid))
ORDER BY at DESC, id DESC
LIMIT sqlc.arg(lim)::bigint;

-- name: Changes :many
SELECT id, actor, table_name, pk::text AS pk, op, before, after, at
FROM control_plane.audit_changes
WHERE request_id = sqlc.arg(request_id)
  AND (sqlc.narg(after_at)::timestamptz IS NULL OR (at, id) > (sqlc.narg(after_at), sqlc.narg(after_id)::uuid))
ORDER BY at, id
LIMIT sqlc.arg(lim)::bigint;

-- name: Requested :one
SELECT EXISTS (SELECT FROM control_plane.audit_log WHERE request_id = $1);
