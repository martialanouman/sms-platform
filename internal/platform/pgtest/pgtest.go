// Package pgtest starts a Postgres for a test, migrated by a migrator role that is not a superuser.
package pgtest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/martialanouman/sms-platform/internal/platform/tcpproxy"
	"github.com/martialanouman/sms-platform/migrations"
)

// DB is a migrated database; each role it holds logs in with its own name as password.
type DB struct {
	endpoint string
	ctr      testcontainers.Container
}

// Logs is what the server has logged so far.
func (db DB) Logs(t *testing.T) string {
	t.Helper()
	r, err := db.ctr.Logs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// As is the connection string of the database as role.
func (db DB) As(role string) string {
	return db.AsWith(role, role)
}

// Late is the database behind an address that refuses connections for delay, as a network that has not admitted its
// client yet.
func (db DB) Late(t *testing.T, delay time.Duration) DB {
	return DB{endpoint: tcpproxy.Later(t, db.endpoint, delay), ctr: db.ctr}
}

// AsWith is the connection string of the database as role, logging in with password.
func (db DB) AsWith(role, password string) string {
	return fmt.Sprintf("postgres://%s:%s@%s/control_plane?sslmode=disable", role, password, db.endpoint)
}

// Secret is a random service-role secret of 32 printable ASCII characters, safe in a connection string.
func Secret() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// ControlPlane starts a Postgres whose control_plane database is migrated to the latest version.
func ControlPlane(t *testing.T) DB {
	t.Helper()
	db := Empty(t)
	migrator, err := sql.Open("pgx", db.As("migrator"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = migrator.Close() }()
	p, err := goose.NewProvider(goose.DialectPostgres, migrator, migrations.ControlPlane())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Up(context.Background()); err != nil {
		t.Fatalf("migrate control_plane: %v", err)
	}
	return db
}

// Empty starts a Postgres whose control_plane database, owned by migrator, holds no migration.
func Empty(t *testing.T) DB {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	pg, err := testcontainers.Run(ctx, "postgres:18-alpine",
		testcontainers.WithExposedPorts("5432/tcp"),
		testcontainers.WithEnv(map[string]string{"POSTGRES_PASSWORD": "postgres"}),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			wait.ForListeningPort("5432/tcp"),
		),
	)
	testcontainers.CleanupContainer(t, pg)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := pg.PortEndpoint(ctx, "5432/tcp", "")
	if err != nil {
		t.Fatal(err)
	}
	db := DB{endpoint: endpoint, ctr: pg}
	super, err := sql.Open("pgx", fmt.Sprintf("postgres://postgres:postgres@%s/postgres?sslmode=disable", endpoint))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = super.Close() }()
	for _, s := range []string{
		`CREATE ROLE migrator LOGIN CREATEROLE PASSWORD 'migrator'`,
		`CREATE DATABASE control_plane OWNER migrator`,
	} {
		if _, err := super.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	return db
}
