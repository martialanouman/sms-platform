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

func classification(t *testing.T) schema.Classification {
	data, err := fs.ReadFile(migrations.ControlPlane(), "audit.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c, err := schema.ParseClassification(data)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// observe counts as captured only an enabled row trigger running audit_capture after all three writes.
func observe(t *testing.T, conn *pgx.Conn) map[string]schema.Observed {
	rows, err := conn.Query(context.Background(), `
		SELECT c.relname,
		  EXISTS (SELECT 1 FROM pg_trigger tg
		    WHERE tg.tgrelid = c.oid AND tg.tgfoid = 'control_plane.audit_capture'::regproc
		      AND tg.tgtype & 31 = 29 AND tg.tgenabled <> 'D'),
		  EXISTS (SELECT 1 FROM pg_index i WHERE i.indrelid = c.oid AND i.indisprimary)
		FROM pg_class c
		WHERE c.relnamespace = 'control_plane'::regnamespace AND c.relkind IN ('r', 'p')
		  AND NOT c.relispartition`)
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

func TestEveryControlPlaneTableIsClassified(t *testing.T) {
	conn := connect(t, pgtest.ControlPlane(t).As("migrator"))
	if problems := classification(t).Check(observe(t, conn)); len(problems) != 0 {
		t.Fatalf("control_plane disagrees with audit.yaml: %q", problems)
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
		`CREATE TABLE control_plane.other (id int PRIMARY KEY)`,
		`CREATE TRIGGER other AFTER INSERT OR UPDATE OR DELETE ON control_plane.other
			FOR EACH ROW EXECUTE FUNCTION control_plane.audit_append_only()`,
		`CREATE TABLE control_plane.parted (at timestamptz) PARTITION BY RANGE (at)`,
		`CREATE TABLE control_plane.parted_default PARTITION OF control_plane.parted DEFAULT`,
	)
	c := classification(t)
	c["captured"] = schema.Table{Audit: schema.Diff}
	c["inserts_only"] = schema.Table{Audit: schema.Diff}
	c["disabled"] = schema.Table{Audit: schema.Diff}
	c["other"] = schema.Table{Audit: schema.Diff}
	c["parted"] = schema.Table{Audit: schema.Exempt, Reason: "sonde"}
	want := []string{
		`"disabled": diff without audit_capture`,
		`"inserts_only": diff without audit_capture`,
		`"other": diff without audit_capture`,
		`"unclassified": not classified`,
	}
	if got := c.Check(observe(t, conn)); !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}
