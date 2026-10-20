// Package pgtest starts a Postgres for a test, its databases migrated by a migrator role that is not a superuser.
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

// DB is a database of the server, control_plane unless In names another; each role logs in with its own name as
// password.
type DB struct {
	endpoint string
	ctr      testcontainers.Container
	database string
}

// In is the database name of the same server.
func (db DB) In(name string) DB {
	db.database = name
	return db
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
	db.endpoint = tcpproxy.Later(t, db.endpoint, delay)
	return db
}

// Cuttable is the database behind a proxy the test can cut.
func (db DB) Cuttable(t *testing.T) (DB, *tcpproxy.Proxy) {
	p := tcpproxy.New(t, db.endpoint)
	db.endpoint = p.Addr()
	return db, p
}

// AsWith is the connection string of the database as role, logging in with password.
func (db DB) AsWith(role, password string) string {
	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", role, password, db.endpoint, db.database)
}

// Secret is a random service-role secret of 32 printable ASCII characters, safe in a connection string.
func Secret() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// ControlPlane starts a Postgres whose databases are migrated to the latest version, and is its control_plane.
func ControlPlane(t *testing.T) DB {
	t.Helper()
	db := Empty(t)
	for _, database := range migrations.Databases {
		migrate(t, db.In(database))
	}
	return db
}

func migrate(t *testing.T, db DB) {
	t.Helper()
	migrator, err := sql.Open("pgx", db.As("migrator"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = migrator.Close() }()
	p, err := goose.NewProvider(goose.DialectPostgres, migrator, migrations.Database(db.database))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Up(context.Background()); err != nil {
		t.Fatalf("migrate %s: %v", db.database, err)
	}
}

// Empty starts a Postgres whose databases, owned by migrator, hold no migration, and is its control_plane.
func Empty(t *testing.T) DB {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	pg, err := testcontainers.Run(ctx, "postgres:18-alpine",
		testcontainers.WithExposedPorts("5432/tcp"),
		testcontainers.WithCmd("postgres", "-c", "track_commit_timestamp=on"),
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
	db := DB{endpoint: endpoint, ctr: pg, database: "control_plane"}
	super, err := sql.Open("pgx", fmt.Sprintf("postgres://postgres:postgres@%s/postgres?sslmode=disable", endpoint))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = super.Close() }()
	statements := []string{`CREATE ROLE migrator LOGIN CREATEROLE PASSWORD 'migrator'`}
	for _, database := range migrations.Databases {
		statements = append(statements, `CREATE DATABASE `+database+` OWNER migrator`)
	}
	for _, s := range statements {
		if _, err := super.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	return db
}
