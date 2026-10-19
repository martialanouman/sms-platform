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

func grantsFile(t *testing.T) string {
	data, err := os.ReadFile("grants/control_plane.sql")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func declarations(t *testing.T) []schema.Grants {
	declared, err := schema.LoadGrants(os.DirFS("../contracts/grants"), classification(t))
	if err != nil {
		t.Fatal(err)
	}
	return declared
}

// granted applies the committed file over a drift, as catalog-apply does over a running database.
func granted(t *testing.T) pgtest.DB {
	pg := pgtest.ControlPlane(t)
	migrator := connect(t, pg.As("migrator"))
	exec(t, migrator, grantsFile(t))
	for _, g := range declarations(t) {
		role := schema.Role(g.Service)
		exec(t, migrator,
			`GRANT ALL ON ALL TABLES IN SCHEMA control_plane TO `+role+` WITH GRANT OPTION`,
			`GRANT CREATE ON SCHEMA control_plane TO `+role,
			`GRANT CREATE ON DATABASE control_plane TO `+role,
			`GRANT UPDATE (target) ON control_plane.audit_log TO `+role,
		)
	}
	exec(t, migrator, grantsFile(t))
	return pg
}

func privileges(t *testing.T, conn *pgx.Conn) []string {
	rows, err := conn.Query(context.Background(), `
		SELECT coalesce(nullif(a.grantee, 0)::regrole::text, 'PUBLIC') || ' ' || c.relname || ' '
		  || lower(a.privilege_type) || CASE WHEN a.is_grantable THEN ' grantable' ELSE '' END
		FROM pg_class c, aclexplode(c.relacl) a
		WHERE c.relnamespace = 'control_plane'::regnamespace AND a.grantee <> c.relowner
		UNION ALL
		SELECT 'column acl on ' || c.relname || '.' || a.attname
		FROM pg_attribute a JOIN pg_class c ON c.oid = a.attrelid
		WHERE c.relnamespace = 'control_plane'::regnamespace AND a.attacl IS NOT NULL
		UNION ALL
		SELECT c.relname || ' owned by ' || c.relowner::regrole
		FROM pg_class c
		WHERE c.relnamespace = 'control_plane'::regnamespace AND c.relowner <> 'migrator'::regrole`)
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
	conn := connect(t, granted(t).As("migrator"))
	var want []string
	for _, g := range declarations(t) {
		role := schema.Role(g.Service)
		for table, ps := range g.Tables {
			for _, p := range ps {
				want = append(want, role+" "+table+" "+p)
			}
		}
		var usage, create, createSchema bool
		if err := conn.QueryRow(context.Background(), `SELECT has_schema_privilege($1, 'control_plane', 'USAGE'),
			has_schema_privilege($1, 'control_plane', 'CREATE'), has_database_privilege($1, 'control_plane', 'CREATE')`,
			role).Scan(&usage, &create, &createSchema); err != nil {
			t.Fatal(err)
		}
		if !usage || create || createSchema {
			t.Errorf("%s: usage %v, create in control_plane %v, create schemas %v; want usage only",
				role, usage, create, createSchema)
		}
	}
	slices.Sort(want)
	if got := privileges(t, conn); !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestAServiceRoleIsRefusedWhatItDidNotDeclare(t *testing.T) {
	pg := granted(t)
	exec(t, connect(t, pg.As("postgres")), `ALTER ROLE admin_api_svc LOGIN PASSWORD 'admin_api_svc'`)
	exec(t, connect(t, pg.As("migrator")), `CREATE TABLE control_plane.undeclared (id int)`)
	svc := connect(t, pg.As("admin_api_svc"))
	exec(t, svc,
		`INSERT INTO control_plane.audit_log (operator, operation_id, target, status) VALUES ('s', 'o', 't', 200)`,
		`SELECT count(*) FROM control_plane.audit_log`,
		`SELECT count(*) FROM control_plane.audit_changes`,
		`DELETE FROM control_plane.audit_log WHERE false`,
		`DELETE FROM control_plane.audit_changes WHERE false`,
	)
	for _, refused := range []string{
		`UPDATE control_plane.audit_changes SET actor = actor WHERE false`,
		`INSERT INTO control_plane.audit_changes (actor, table_name, pk, op) VALUES ('s', 't', '{}', 'insert')`,
		`SELECT count(*) FROM control_plane.undeclared`,
		`CREATE TABLE control_plane.own (id int)`,
	} {
		if _, err := svc.Exec(context.Background(), refused); sqlState(err) != "42501" {
			t.Errorf("%s: err = %v, want 42501", refused, err)
		}
	}
}

func TestTheGrantsAdoptOnlyAPlainRole(t *testing.T) {
	pg := pgtest.ControlPlane(t)
	super := connect(t, pg.As("postgres"))
	exec(t, super, `CREATE ROLE admin_api_svc NOLOGIN`, `CREATE ROLE intruder LOGIN`)
	migrator := connect(t, pg.As("migrator"))
	for _, privileged := range []struct{ grant, revoke string }{
		{`GRANT migrator TO admin_api_svc`, `REVOKE migrator FROM admin_api_svc`},
		{`GRANT admin_api_svc TO intruder`, `REVOKE admin_api_svc FROM intruder`},
		{`GRANT admin_api_svc TO intruder WITH INHERIT FALSE`, `REVOKE admin_api_svc FROM intruder`},
		{`ALTER ROLE admin_api_svc SUPERUSER`, `ALTER ROLE admin_api_svc NOSUPERUSER`},
		{`ALTER ROLE admin_api_svc CREATEROLE`, `ALTER ROLE admin_api_svc NOCREATEROLE`},
		{`ALTER ROLE admin_api_svc CREATEDB`, `ALTER ROLE admin_api_svc NOCREATEDB`},
		{`ALTER ROLE admin_api_svc BYPASSRLS`, `ALTER ROLE admin_api_svc NOBYPASSRLS`},
		{`ALTER ROLE admin_api_svc REPLICATION`, `ALTER ROLE admin_api_svc NOREPLICATION`},
	} {
		exec(t, super, privileged.grant)
		if _, err := migrator.Exec(context.Background(), grantsFile(t)); sqlState(err) != "P0001" {
			t.Errorf("after %s: err = %v, want P0001", privileged.grant, err)
		}
		exec(t, migrator, `ROLLBACK`)
		exec(t, super, privileged.revoke)
	}
	exec(t, migrator, grantsFile(t))
}
