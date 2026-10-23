-- name: AccountCredentials :many
SELECT id, account_id, type, system_id, status, last_used_at, grace_expires_at, created_at, rotated_at
FROM control_plane.credentials
WHERE account_id = $1
ORDER BY type;

-- name: Credential :one
SELECT id, account_id, type, system_id, status, last_used_at, grace_expires_at, created_at, rotated_at
FROM control_plane.credentials
WHERE id = $1 AND account_id = $2;

-- name: CreateCredential :one
INSERT INTO control_plane.credentials (account_id, type, system_id, secret_hash)
VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: SetCredentialStatus :one
UPDATE control_plane.credentials SET status = sqlc.arg(status)
WHERE id = sqlc.arg(id) AND account_id = sqlc.arg(account_id)
RETURNING id;

-- name: RotateCredential :one
UPDATE control_plane.credentials SET
  previous_secret_hash = CASE WHEN sqlc.arg(grace_sec)::integer > 0 THEN secret_hash END,
  grace_expires_at = CASE WHEN status <> 'revoked' AND sqlc.arg(grace_sec)::integer > 0
    THEN now() + make_interval(secs => sqlc.arg(grace_sec)::integer) END,
  secret_hash = sqlc.arg(secret_hash),
  status = CASE WHEN status = 'revoked' THEN 'active' ELSE status END,
  rotated_at = now()
WHERE id = sqlc.arg(id) AND account_id = sqlc.arg(account_id)
RETURNING id;

-- name: RevokeCredential :execrows
UPDATE control_plane.credentials SET status = 'revoked', secret_hash = NULL, previous_secret_hash = NULL,
  grace_expires_at = NULL
WHERE id = $1 AND account_id = $2;
