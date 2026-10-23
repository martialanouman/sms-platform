-- name: Groups :many
SELECT g.id, g.name, g.description, g.status, g.created_by, g.created_at, g.updated_at,
  (SELECT count(*) FROM control_plane.customers c WHERE c.group_id = g.id) AS member_count
FROM control_plane.customer_groups g
WHERE (sqlc.narg(status)::text IS NULL OR g.status = sqlc.narg(status))
  AND (sqlc.narg(after_at)::timestamptz IS NULL OR (g.created_at, g.id) < (sqlc.narg(after_at), sqlc.narg(after_id)::uuid))
ORDER BY g.created_at DESC, g.id DESC
LIMIT sqlc.arg(lim)::bigint;

-- name: Group :one
SELECT g.id, g.name, g.description, g.status, g.created_by, g.created_at, g.updated_at,
  (SELECT count(*) FROM control_plane.customers c WHERE c.group_id = g.id) AS member_count
FROM control_plane.customer_groups g
WHERE g.id = $1;

-- name: CreateGroup :one
INSERT INTO control_plane.customer_groups (name, description) VALUES ($1, $2) RETURNING id;

-- name: UpdateGroup :one
UPDATE control_plane.customer_groups SET
  name = coalesce(sqlc.narg(name), name),
  description = CASE WHEN sqlc.arg(set_description)::bool THEN sqlc.narg(description) ELSE description END,
  status = coalesce(sqlc.narg(status), status)
WHERE id = sqlc.arg(id)
RETURNING id;

-- name: DeleteGroup :execrows
DELETE FROM control_plane.customer_groups WHERE id = $1;

-- name: GroupCustomers :many
SELECT id, name, status, group_id, max_age_otp, max_age_transactional, max_age_marketing, created_at, updated_at
FROM control_plane.customers
WHERE group_id = sqlc.arg(group_id)
  AND (sqlc.narg(after_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(after_at), sqlc.narg(after_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(lim)::bigint;
