package migrations_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/martialanouman/sms-platform/internal/platform/pgtest"
)

func connect(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func TestAuditJournalsAreAppendOnlyButForThePurgeDoor(t *testing.T) {
	conn := connect(t, pgtest.ControlPlane(t).Migrator)
	ctx := context.Background()
	for table, insert := range map[string]string{
		"control_plane.audit_log": `INSERT INTO control_plane.audit_log
			(operator, operation_id, target, status, at) VALUES ('sub', 'op', 'x', 200, now() - $1::interval)`,
		"control_plane.audit_changes": `INSERT INTO control_plane.audit_changes
			(actor, table_name, pk, op, at) VALUES ('sub', 't', '{"id": 1}', 'insert', now() - $1::interval)`,
	} {
		t.Run(table, func(t *testing.T) {
			for _, age := range []string{"8759 hours", "8761 hours"} {
				if _, err := conn.Exec(ctx, insert, age); err != nil {
					t.Fatal(err)
				}
			}
			for _, refused := range []string{
				`UPDATE ` + table + ` SET at = at`,
				`DELETE FROM ` + table + ` WHERE at < now() - interval '8760 hours'`,
				`TRUNCATE ` + table,
			} {
				if _, err := conn.Exec(ctx, refused); sqlState(err) != "42501" {
					t.Errorf("%s: err = %v, want 42501", refused, err)
				}
			}
			underPurge := func(statement string) error {
				tx, err := conn.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = tx.Rollback(ctx) }()
				if _, err := tx.Exec(ctx, `SET LOCAL audit_log.purge = 'on'`); err != nil {
					t.Fatal(err)
				}
				if _, err := tx.Exec(ctx, statement); err != nil {
					return err
				}
				return tx.Commit(ctx)
			}
			old := ` WHERE at < now() - interval '8760 hours'`
			if err := underPurge(`UPDATE ` + table + ` SET at = at` + old); sqlState(err) != "42501" {
				t.Errorf("update through the purge door: err = %v, want 42501", err)
			}
			if err := underPurge(`DELETE FROM ` + table + ` WHERE at > now() - interval '8760 hours'`); sqlState(err) != "42501" {
				t.Errorf("purge under the floor: err = %v, want 42501", err)
			}
			if err := underPurge(`DELETE FROM ` + table + old); err != nil {
				t.Errorf("purge beyond the floor: %v", err)
			}
			var left int
			if err := conn.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&left); err != nil {
				t.Fatal(err)
			}
			if left != 1 {
				t.Errorf("%d rows left, want the young one", left)
			}
		})
	}
}
