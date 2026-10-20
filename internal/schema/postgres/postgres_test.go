package postgres_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/martialanouman/sms-platform/internal/platform/pgtest"
	"github.com/martialanouman/sms-platform/internal/schema"
	"github.com/martialanouman/sms-platform/internal/schema/postgres"
	"github.com/martialanouman/sms-platform/migrations"
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

func secrets(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	secret := pgtest.Secret()
	for role, s := range map[string]string{"admin_api_svc": secret, "dashboard": pgtest.Secret()} {
		if err := os.WriteFile(filepath.Join(dir, role), []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir, secret
}

func verifier(t *testing.T, pg pgtest.DB, role string) string {
	t.Helper()
	var v *string
	if err := connect(t, pg.As("postgres")).QueryRow(context.Background(), `SELECT rolpassword FROM pg_authid WHERE rolname = $1`, role).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v == nil {
		return ""
	}
	return *v
}

func apply(t *testing.T, pg pgtest.DB, dir string) []string {
	t.Helper()
	roles, err := postgres.Apply(context.Background(), connect(t, pg.As("migrator")), migrations.Grants("control_plane"), os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	return roles
}

func TestApplyLetsEachDeclaredRoleLogInWithItsSecretOnly(t *testing.T) {
	pg := pgtest.ControlPlane(t)
	exec := func(conn *pgx.Conn, s string) {
		if _, err := conn.Exec(context.Background(), s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	exec(connect(t, pg.As("migrator")), `CREATE ROLE retired_svc LOGIN`)
	super := connect(t, pg.As("postgres"))
	exec(super, `CREATE ROLE foreign_svc LOGIN`)
	exec(super, `GRANT foreign_svc TO migrator`)
	dir, secret := secrets(t)
	var verifiers []string
	for range 2 {
		if roles := apply(t, pg, dir); !reflect.DeepEqual(roles, []string{"admin_api_svc", "dashboard"}) {
			t.Fatalf("Apply provisioned %v, want every declared role, dashboard included", roles)
		}
		svc := connect(t, pg.AsWith("admin_api_svc", secret))
		if _, err := svc.Exec(context.Background(), `SELECT count(*) FROM control_plane.audit_log`); err != nil {
			t.Fatal(err)
		}
		verifiers = append(verifiers, verifier(t, pg, "admin_api_svc"))
	}
	if verifiers[0] == verifiers[1] {
		t.Error("two applications set the same verifier: the salt is not drawn")
	}
	_, err := pgx.Connect(context.Background(), pg.AsWith("admin_api_svc", "wrong"))
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "28P01" {
		t.Errorf("login with another secret: %v, want 28P01", err)
	}
}

func TestApplyRefusesAMissingSecretWithoutSettingAny(t *testing.T) {
	pg := pgtest.ControlPlane(t)
	dir, _ := secrets(t)
	audit := map[string]map[string][]string{"control_plane": {"audit_log": {"select"}}}
	two := schema.GrantsSQL("control_plane", []schema.Grants{
		{Service: "admin-api-svc", Databases: audit},
		{Service: "billing-svc", Databases: audit},
	})
	_, err := postgres.Apply(context.Background(), connect(t, pg.As("migrator")), two, os.DirFS(dir))
	if err == nil || !strings.Contains(err.Error(), "billing_svc") {
		t.Fatalf("Apply = %v, want a refusal naming billing_svc", err)
	}
	if v := verifier(t, pg, "admin_api_svc"); v != "" {
		t.Errorf("admin_api_svc got its verifier before the missing secret of billing_svc was seen")
	}
}

func TestTwoSimultaneousAppliesBothSucceed(t *testing.T) {
	pg := pgtest.ControlPlane(t)
	dir, _ := secrets(t)
	conns := []*pgx.Conn{connect(t, pg.As("migrator")), connect(t, pg.As("migrator"))}
	var start, done sync.WaitGroup
	start.Add(1)
	errs := make([]error, len(conns))
	for i, conn := range conns {
		done.Go(func() {
			start.Wait()
			_, errs[i] = postgres.Apply(context.Background(), conn, migrations.Grants("control_plane"), os.DirFS(dir))
		})
	}
	start.Done()
	done.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatal(err)
	}
}

func eventually(t *testing.T, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); !ok() && time.Now().Before(deadline); {
		time.Sleep(50 * time.Millisecond)
	}
}

func TestTheServerNeverReceivesTheSecret(t *testing.T) {
	pg := pgtest.ControlPlane(t)
	super := connect(t, pg.As("postgres"))
	for _, s := range []string{`ALTER SYSTEM SET log_statement = 'all'`, `SELECT pg_reload_conf()`} {
		if _, err := super.Exec(context.Background(), s); err != nil {
			t.Fatal(err)
		}
	}
	eventually(t, func() bool {
		var level string
		return connect(t, pg.As("migrator")).QueryRow(context.Background(), `SHOW log_statement`).Scan(&level) == nil && level == "all"
	})
	dir, secret := secrets(t)
	apply(t, pg, dir)
	var logs string
	eventually(t, func() bool {
		logs = pg.Logs(t)
		return strings.Contains(logs, `ALTER ROLE "admin_api_svc" PASSWORD '`)
	})
	if !strings.Contains(logs, `ALTER ROLE "admin_api_svc" PASSWORD '`) {
		t.Fatalf("the server log does not capture the statement, the absence of the secret proves nothing:\n%s", logs)
	}
	if strings.Contains(logs, secret) {
		t.Error("the server log holds the secret")
	}
}
