-- name: Accounts :many
SELECT a.id, a.customer_id, c.name AS customer_name, c.status AS customer_status, a.name, a.status, a.smpp_enabled,
  a.rest_enabled, a.query_sm_enabled, a.cancel_sm_enabled, a.allowed_bind_types, a.max_sessions, a.created_at, a.updated_at
FROM control_plane.accounts a JOIN control_plane.customers c ON c.id = a.customer_id
WHERE (sqlc.narg(customer_id)::uuid IS NULL OR a.customer_id = sqlc.narg(customer_id))
  AND (sqlc.narg(group_id)::uuid IS NULL OR c.group_id = sqlc.narg(group_id))
  AND (sqlc.narg(status)::text IS NULL OR a.status = sqlc.narg(status))
  AND (sqlc.narg(after_at)::timestamptz IS NULL OR (a.created_at, a.id) < (sqlc.narg(after_at), sqlc.narg(after_id)::uuid))
ORDER BY a.created_at DESC, a.id DESC
LIMIT sqlc.arg(lim)::bigint;

-- name: CountAccounts :one
SELECT count(*) FROM control_plane.accounts a JOIN control_plane.customers c ON c.id = a.customer_id
WHERE (sqlc.narg(customer_id)::uuid IS NULL OR a.customer_id = sqlc.narg(customer_id))
  AND (sqlc.narg(group_id)::uuid IS NULL OR c.group_id = sqlc.narg(group_id))
  AND (sqlc.narg(status)::text IS NULL OR a.status = sqlc.narg(status));

-- name: Account :one
SELECT a.id, a.customer_id, c.name AS customer_name, c.status AS customer_status, a.name, a.status, a.smpp_enabled,
  a.rest_enabled, a.query_sm_enabled, a.cancel_sm_enabled, a.allowed_bind_types, a.max_sessions, a.created_at, a.updated_at
FROM control_plane.accounts a JOIN control_plane.customers c ON c.id = a.customer_id
WHERE a.id = $1;

-- name: CreateAccount :one
INSERT INTO control_plane.accounts (customer_id, name, smpp_enabled, rest_enabled, query_sm_enabled, cancel_sm_enabled,
  allowed_bind_types, max_sessions)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: UpdateAccount :one
UPDATE control_plane.accounts SET
  name = coalesce(sqlc.narg(name), name),
  status = coalesce(sqlc.narg(status), status),
  smpp_enabled = coalesce(sqlc.narg(smpp_enabled), smpp_enabled),
  rest_enabled = coalesce(sqlc.narg(rest_enabled), rest_enabled),
  query_sm_enabled = coalesce(sqlc.narg(query_sm_enabled), query_sm_enabled),
  cancel_sm_enabled = coalesce(sqlc.narg(cancel_sm_enabled), cancel_sm_enabled),
  allowed_bind_types = coalesce(sqlc.narg(allowed_bind_types), allowed_bind_types),
  max_sessions = coalesce(sqlc.narg(max_sessions), max_sessions)
WHERE id = sqlc.arg(id)
RETURNING id;

-- name: DeleteAccount :execrows
DELETE FROM control_plane.accounts WHERE id = $1;
