package migrations_test

import (
	"context"
	"os"
	"reflect"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/martialanouman/sms-platform/internal/platform/pgtest"
	"github.com/martialanouman/sms-platform/internal/schema"
)

func granted(t *testing.T) (pgtest.DB, []schema.Grants) {
	declared, err := schema.LoadGrants(os.DirFS("../contracts/grants"), classification(t))
	if err != nil {
		t.Fatal(err)
	}
	pg := pgtest.ControlPlane(t)
	migrator := connect(t, pg.As("migrator"))
	exec(t, migrator, string(schema.GrantsSQL(declared)))
	for _, g := range declared {
		exec(t, migrator, `GRANT ALL ON ALL TABLES IN SCHEMA control_plane TO `+schema.Role(g.Service))
	}
	exec(t, migrator, string(schema.GrantsSQL(declared)))
	return pg, declared
}

func privileges(t *testing.T, conn *pgx.Conn) []string {
	rows, err := conn.Query(context.Background(), `
		SELECT coalesce(nullif(a.grantee, 0)::regrole::text, 'PUBLIC') || ' ' || c.relname || ' '
		  || lower(a.privilege_type)
		FROM pg_class c, aclexplode(c.relacl) a
		WHERE c.relnamespace = 'control_plane'::regnamespace AND a.grantee <> c.relowner`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	return got
}

func TestEachServiceRoleHoldsExactlyItsDeclaration(t *testing.T) {
	pg, declared := granted(t)
	conn := connect(t, pg.As("migrator"))
	var want []string
	for _, g := range declared {
		role := schema.Role(g.Service)
		for table, ps := range g.Tables {
			for _, p := range ps {
				want = append(want, role+" "+table+" "+string(p))
			}
		}
		var usage bool
		if err := conn.QueryRow(context.Background(),
			`SELECT has_schema_privilege($1, 'control_plane', 'USAGE')`, role).Scan(&usage); err != nil {
			t.Fatal(err)
		}
		if !usage {
			t.Errorf("%s has no USAGE on control_plane", role)
		}
	}
	slices.Sort(want)
	if got := privileges(t, conn); !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestAServiceRoleIsRefusedWhatItDidNotDeclare(t *testing.T) {
	pg, _ := granted(t)
	exec(t, connect(t, pg.As("postgres")), `ALTER ROLE admin_api_svc LOGIN PASSWORD 'admin_api_svc'`)
	exec(t, connect(t, pg.As("migrator")), `CREATE TABLE control_plane.undeclared (id int)`)
	svc := connect(t, pg.As("admin_api_svc"))
	exec(t, svc,
		`INSERT INTO control_plane.audit_log (operator, operation_id, target, status) VALUES ('s', 'o', 't', 200)`,
		`SELECT count(*) FROM control_plane.audit_changes`,
	)
	for _, refused := range []string{
		`UPDATE control_plane.audit_log SET target = target`,
		`SELECT count(*) FROM control_plane.undeclared`,
	} {
		if _, err := svc.Exec(context.Background(), refused); sqlState(err) != "42501" {
			t.Errorf("%s: err = %v, want 42501", refused, err)
		}
	}
}
