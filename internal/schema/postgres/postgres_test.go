package postgres_test

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/martialanouman/sms-platform/internal/platform/pgtest"
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
	secret := rand.Text()
	if err := os.WriteFile(filepath.Join(dir, "admin_api_svc"), []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, secret
}

func as(pg pgtest.DB, role, password string) string {
	return strings.Replace(pg.As(role), role+":"+role+"@", role+":"+password+"@", 1)
}

func apply(t *testing.T, pg pgtest.DB, dir string) []string {
	t.Helper()
	roles, err := postgres.Apply(context.Background(), connect(t, pg.As("migrator")), migrations.Grants(), os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	return roles
}

func TestApplyLetsEachDeclaredRoleLogInWithItsSecretOnly(t *testing.T) {
	pg := pgtest.ControlPlane(t)
	dir, secret := secrets(t)
	for range 2 {
		if roles := apply(t, pg, dir); !reflect.DeepEqual(roles, []string{"admin_api_svc"}) {
			t.Fatalf("Apply provisioned %v, want admin_api_svc", roles)
		}
		svc := connect(t, as(pg, "admin_api_svc", secret))
		if _, err := svc.Exec(context.Background(), `SELECT count(*) FROM control_plane.audit_log`); err != nil {
			t.Fatal(err)
		}
	}
	_, err := pgx.Connect(context.Background(), as(pg, "admin_api_svc", "wrong"))
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "28P01" {
		t.Errorf("login with another secret: %v, want 28P01", err)
	}
}

func TestApplyRefusesAMissingSecretWithoutSettingAny(t *testing.T) {
	pg := pgtest.ControlPlane(t)
	_, err := postgres.Apply(context.Background(), connect(t, pg.As("migrator")), migrations.Grants(), os.DirFS(t.TempDir()))
	if err == nil || !strings.Contains(err.Error(), "admin_api_svc") {
		t.Fatalf("Apply = %v, want a refusal naming admin_api_svc", err)
	}
	var set bool
	if err := connect(t, pg.As("postgres")).QueryRow(context.Background(),
		`SELECT rolpassword IS NOT NULL FROM pg_authid WHERE rolname = 'admin_api_svc'`).Scan(&set); err != nil || set {
		t.Errorf("password set %v, %v; want none", set, err)
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
			_, errs[i] = postgres.Apply(context.Background(), conn, migrations.Grants(), os.DirFS(dir))
		})
	}
	start.Done()
	done.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatal(err)
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
	dir, secret := secrets(t)
	apply(t, pg, dir)
	logs := pg.Logs(t)
	if !strings.Contains(logs, `ALTER ROLE "admin_api_svc" PASSWORD '`) {
		t.Fatalf("the server log does not capture the statement, the absence of the secret proves nothing:\n%s", logs)
	}
	if strings.Contains(logs, secret) {
		t.Error("the server log holds the secret")
	}
}
