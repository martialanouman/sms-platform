package migrations_test

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/martialanouman/sms-platform/internal/platform/pgtest"
	"github.com/martialanouman/sms-platform/migrations"
)

func TestControlPlaneMigratesUpDownUpAsMigrator(t *testing.T) {
	pg := pgtest.ControlPlane(t)
	db, err := sql.Open("pgx", pg.Migrator)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.ControlPlane())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := p.DownTo(ctx, 0); err != nil {
		t.Fatalf("down: %v", err)
	}
	var schemas int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM pg_namespace WHERE nspname = 'control_plane'`).Scan(&schemas); err != nil {
		t.Fatal(err)
	}
	if schemas != 0 {
		t.Fatal("down left the control_plane schema")
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("up again: %v", err)
	}
}
