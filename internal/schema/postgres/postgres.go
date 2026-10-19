// Package postgres applies the generated grants to a Postgres and gives each service role its secret.
package postgres

import (
	"context"
	"crypto/rand"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5"

	"github.com/martialanouman/sms-platform/internal/schema"
)

// Apply runs the grants file as one transaction, then sets the SCRAM verifier of every role the file lets log in,
// from the file of secrets named after the role. It returns those roles.
func Apply(ctx context.Context, conn *pgx.Conn, grants []byte, secrets fs.FS) ([]string, error) {
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(hashtext('grants'))`); err != nil {
		return nil, fmt.Errorf("lock grants: %w", err)
	}
	defer func() { _, _ = conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock(hashtext('grants'))`) }()
	if _, err := conn.Exec(ctx, string(grants)); err != nil {
		return nil, fmt.Errorf("apply grants: %w", err)
	}
	rows, err := conn.Query(ctx, `SELECT r.rolname FROM pg_catalog.pg_auth_members m JOIN pg_catalog.pg_roles r ON r.oid = m.roleid
		WHERE m.member = current_user::regrole AND m.admin_option AND r.rolcanlogin ORDER BY 1`)
	if err != nil {
		return nil, fmt.Errorf("list service roles: %w", err)
	}
	roles, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("list service roles: %w", err)
	}
	verifiers := make([]string, len(roles))
	for i, role := range roles {
		secret, err := fs.ReadFile(secrets, role)
		if err != nil {
			return nil, fmt.Errorf("role %s: %w", role, err)
		}
		salt := make([]byte, 16)
		_, _ = rand.Read(salt)
		if verifiers[i], err = schema.SCRAMVerifier(string(secret), salt); err != nil {
			return nil, fmt.Errorf("role %s: %w", role, err)
		}
	}
	for i, role := range roles {
		if _, err := conn.Exec(ctx, "ALTER ROLE "+pgx.Identifier{role}.Sanitize()+" PASSWORD '"+verifiers[i]+"'"); err != nil {
			return nil, fmt.Errorf("role %s: set its secret: %w", role, err)
		}
	}
	return roles, nil
}
