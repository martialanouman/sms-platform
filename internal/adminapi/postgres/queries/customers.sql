-- name: Groups :many
SELECT g.id, g.name, g.description, g.status, g.created_by, g.created_at, g.updated_at,
  (SELECT count(*) FROM control_plane.customers c WHERE c.group_id = g.id) AS member_count
FROM control_plane.customer_groups g
WHERE (sqlc.narg(status)::text IS NULL OR g.status = sqlc.narg(status))
  AND (sqlc.narg(after_at)::timestamptz IS NULL OR (g.created_at, g.id) < (sqlc.narg(after_at), sqlc.narg(after_id)::uuid))
ORDER BY g.created_at DESC, g.id DESC
LIMIT sqlc.arg(lim)::bigint;

-- name: CountGroups :one
SELECT count(*) FROM control_plane.customer_groups g
WHERE sqlc.narg(status)::text IS NULL OR g.status = sqlc.narg(status);

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

-- name: Customers :many
SELECT id, name, status, group_id, max_age_otp, max_age_transactional, max_age_marketing, created_at, updated_at
FROM control_plane.customers
WHERE (sqlc.narg(group_id)::uuid IS NULL OR group_id = sqlc.narg(group_id))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(after_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(after_at), sqlc.narg(after_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(lim)::bigint;

-- name: CountCustomers :one
SELECT count(*) FROM control_plane.customers
WHERE (sqlc.narg(group_id)::uuid IS NULL OR group_id = sqlc.narg(group_id))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status));

-- name: Customer :one
SELECT id, name, status, group_id, max_age_otp, max_age_transactional, max_age_marketing, created_at, updated_at
FROM control_plane.customers
WHERE id = $1;

-- name: CreateCustomer :one
INSERT INTO control_plane.customers (name, group_id, max_age_otp, max_age_transactional, max_age_marketing)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: UpdateCustomer :one
UPDATE control_plane.customers SET
  name = coalesce(sqlc.narg(name), name),
  status = coalesce(sqlc.narg(status), status),
  group_id = CASE WHEN sqlc.arg(set_group)::bool THEN sqlc.narg(group_id)::uuid ELSE group_id END,
  max_age_otp = CASE WHEN sqlc.arg(set_max_age)::bool THEN sqlc.narg(max_age_otp)::integer ELSE max_age_otp END,
  max_age_transactional = CASE WHEN sqlc.arg(set_max_age)::bool THEN sqlc.narg(max_age_transactional)::integer ELSE max_age_transactional END,
  max_age_marketing = CASE WHEN sqlc.arg(set_max_age)::bool THEN sqlc.narg(max_age_marketing)::integer ELSE max_age_marketing END
WHERE id = sqlc.arg(id)
RETURNING id;

-- name: SetAccountsStatus :exec
UPDATE control_plane.accounts SET status = sqlc.arg(status) WHERE customer_id = sqlc.arg(customer_id) AND status NOT IN ('closed', sqlc.arg(status));

-- name: DeleteCustomer :execrows
DELETE FROM control_plane.customers WHERE id = $1;

-- name: CustomerAccounts :many
SELECT id, name, status FROM control_plane.accounts WHERE customer_id = $1 ORDER BY name, id;
