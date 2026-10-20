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

func TestEachDatabaseMigratesUpDownUpAsMigrator(t *testing.T) {
	pg := pgtest.ControlPlane(t)
	for _, database := range migrations.Databases {
		db, err := sql.Open("pgx", pg.In(database).As("migrator"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Database(database))
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		if _, err := p.DownTo(ctx, 0); err != nil {
			t.Fatalf("%s down: %v", database, err)
		}
		var schemas int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_namespace WHERE nspname = $1`, database).Scan(&schemas); err != nil {
			t.Fatal(err)
		}
		if schemas != 0 {
			t.Fatalf("down left the %s schema", database)
		}
		var public bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT FROM pg_database d,
			aclexplode(coalesce(d.datacl, acldefault('d', d.datdba))) a
			WHERE d.datname = $1 AND a.grantee = 0 AND a.privilege_type = 'CONNECT')`, database).Scan(&public); err != nil || !public {
			t.Fatalf("down left %s closed to PUBLIC (%v, %v)", database, public, err)
		}
		if _, err := p.Up(ctx); err != nil {
			t.Fatalf("%s up again: %v", database, err)
		}
	}
}
