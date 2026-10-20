package migrations_test

import (
	"context"
	"io/fs"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/martialanouman/sms-platform/internal/platform/pgtest"
	"github.com/martialanouman/sms-platform/internal/schema"
	"github.com/martialanouman/sms-platform/migrations"
)

func classification(t *testing.T, database string) schema.Classification {
	data, err := fs.ReadFile(migrations.Database(database), "audit.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c, err := schema.ParseClassification(data)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// observe counts as captured only a row trigger running the audit_capture of the database's schema after every
// insert, update and delete, unconditionally, outside replica mode.
func observe(t *testing.T, conn *pgx.Conn, database string) map[string]schema.Observed {
	rows, err := conn.Query(context.Background(), `
		SELECT c.relname,
		  EXISTS (SELECT 1 FROM pg_trigger tg
		    WHERE tg.tgrelid = c.oid AND tg.tgfoid = to_regproc($1 || '.audit_capture')
		      AND tg.tgtype & 31 = 29 AND tg.tgenabled IN ('O', 'A')
		      AND cardinality(tg.tgattr::int2[]) = 0 AND tg.tgqual IS NULL),
		  EXISTS (SELECT 1 FROM pg_index i WHERE i.indrelid = c.oid AND i.indisprimary)
		FROM pg_class c
		WHERE c.relnamespace = $1::regnamespace AND c.relkind IN ('r', 'p')
		  AND NOT c.relispartition`, database)
	if err != nil {
		t.Fatal(err)
	}
	tables := map[string]schema.Observed{}
	for rows.Next() {
		var name string
		var o schema.Observed
		if err := rows.Scan(&name, &o.Captured, &o.HasPrimaryKey); err != nil {
			t.Fatal(err)
		}
		tables[name] = o
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return tables
}

func TestEveryTableOfEachDatabaseIsClassified(t *testing.T) {
	pg := pgtest.ControlPlane(t)
	for _, database := range migrations.Databases {
		conn := connect(t, pg.In(database).As("migrator"))
		if problems := classification(t, database).Check(observe(t, conn, database)); len(problems) != 0 {
			t.Errorf("%s disagrees with its audit.yaml: %q", database, problems)
		}
	}
}

func TestTheGuardReadsTheDatabase(t *testing.T) {
	conn := connect(t, pgtest.ControlPlane(t).As("migrator"))
	capture := ` FOR EACH ROW EXECUTE FUNCTION control_plane.audit_capture()`
	exec(t, conn,
		`CREATE TABLE control_plane.unclassified (id int PRIMARY KEY)`,
		`CREATE TABLE control_plane.captured (id int PRIMARY KEY)`,
		`CREATE TRIGGER audit_capture AFTER INSERT OR UPDATE OR DELETE ON control_plane.captured`+capture,
		`CREATE TABLE control_plane.inserts_only (id int PRIMARY KEY)`,
		`CREATE TRIGGER audit_capture AFTER INSERT ON control_plane.inserts_only`+capture,
		`CREATE TABLE control_plane.disabled (id int PRIMARY KEY)`,
		`CREATE TRIGGER audit_capture AFTER INSERT OR UPDATE OR DELETE ON control_plane.disabled`+capture,
		`ALTER TABLE control_plane.disabled DISABLE TRIGGER audit_capture`,
		`CREATE TABLE control_plane.replica (id int PRIMARY KEY)`,
		`CREATE TRIGGER audit_capture AFTER INSERT OR UPDATE OR DELETE ON control_plane.replica`+capture,
		`ALTER TABLE control_plane.replica ENABLE REPLICA TRIGGER audit_capture`,
		`CREATE TABLE control_plane.some_columns (id int PRIMARY KEY, name text)`,
		`CREATE TRIGGER audit_capture AFTER INSERT OR UPDATE OF name OR DELETE ON control_plane.some_columns`+capture,
		`CREATE TABLE control_plane.conditional (id int PRIMARY KEY)`,
		`CREATE TRIGGER audit_capture AFTER INSERT OR UPDATE OR DELETE ON control_plane.conditional
			FOR EACH ROW WHEN (true) EXECUTE FUNCTION control_plane.audit_capture()`,
		`CREATE TABLE control_plane.keyless (id int)`,
		`CREATE TRIGGER audit_capture AFTER INSERT OR UPDATE OR DELETE ON control_plane.keyless`+capture,
		`CREATE TABLE control_plane.other (id int PRIMARY KEY)`,
		`CREATE TRIGGER other AFTER INSERT OR UPDATE OR DELETE ON control_plane.other
			FOR EACH ROW EXECUTE FUNCTION control_plane.audit_append_only()`,
		`CREATE TABLE control_plane.parted (at timestamptz) PARTITION BY RANGE (at)`,
		`CREATE TABLE control_plane.parted_default PARTITION OF control_plane.parted DEFAULT`,
	)
	c := classification(t, "control_plane")
	c["captured"] = schema.Table{Audit: schema.Diff}
	c["inserts_only"] = schema.Table{Audit: schema.Diff}
	c["disabled"] = schema.Table{Audit: schema.Diff}
	for _, name := range []string{"other", "replica", "some_columns", "conditional", "keyless"} {
		c[name] = schema.Table{Audit: schema.Diff}
	}
	c["parted"] = schema.Table{Audit: schema.Exempt, Reason: "sonde"}
	want := []string{
		`"conditional": diff without audit_capture`,
		`"disabled": diff without audit_capture`,
		`"inserts_only": diff without audit_capture`,
		`"keyless": diff without a primary key`,
		`"other": diff without audit_capture`,
		`"replica": diff without audit_capture`,
		`"some_columns": diff without audit_capture`,
		`"unclassified": not classified`,
	}
	if got := c.Check(observe(t, conn, "control_plane")); !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestADiffTableOfADatabaseWithoutCaptureIsRefused(t *testing.T) {
	conn := connect(t, pgtest.ControlPlane(t).In("dashboard").As("migrator"))
	exec(t, conn, `CREATE TABLE dashboard.probe (id int PRIMARY KEY)`)
	c := classification(t, "dashboard")
	c["probe"] = schema.Table{Audit: schema.Diff}
	if got, want := c.Check(observe(t, conn, "dashboard")), []string{`"probe": diff without audit_capture`}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
