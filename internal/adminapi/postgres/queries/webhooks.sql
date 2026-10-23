-- name: AccountWebhooks :many
SELECT id, account_id, event_type, url, status, created_at, updated_at
FROM control_plane.webhooks
WHERE account_id = $1
ORDER BY event_type;

-- name: Webhook :one
SELECT id, account_id, event_type, url, status, created_at, updated_at
FROM control_plane.webhooks
WHERE id = $1 AND account_id = $2;

-- name: CreateWebhook :one
INSERT INTO control_plane.webhooks (account_id, event_type, url, secret_sealed, secret_kms_key_ref)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, account_id, event_type, url, status, created_at, updated_at;

-- name: UpdateWebhook :one
UPDATE control_plane.webhooks SET
  url = coalesce(sqlc.narg(url), url),
  status = coalesce(sqlc.narg(status), status)
WHERE id = sqlc.arg(id) AND account_id = sqlc.arg(account_id)
RETURNING id, account_id, event_type, url, status, created_at, updated_at;

-- name: SealWebhook :one
-- debt: #177
UPDATE control_plane.webhooks SET secret_sealed = sqlc.arg(secret_sealed), secret_kms_key_ref = sqlc.arg(secret_kms_key_ref)
WHERE id = sqlc.arg(id) AND account_id = sqlc.arg(account_id)
RETURNING id, account_id, event_type, url, status, created_at, updated_at;

-- name: DeleteWebhook :execrows
DELETE FROM control_plane.webhooks WHERE id = $1 AND account_id = $2;
