// Package pgtest starts a Postgres for a test, migrated by a migrator role that is not a superuser.
package pgtest

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/martialanouman/sms-platform/migrations"
)

// DB is a migrated database; each role it holds logs in with its own name as password.
type DB struct {
	endpoint string
}

// As is the connection string of the database as role.
func (db DB) As(role string) string {
	return fmt.Sprintf("postgres://%s:%s@%s/control_plane?sslmode=disable", role, role, db.endpoint)
}

// ControlPlane starts a Postgres whose control_plane database is migrated to the latest version.
func ControlPlane(t *testing.T) DB {
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
	db := DB{endpoint: endpoint}
	super, err := sql.Open("pgx", fmt.Sprintf("postgres://postgres:postgres@%s/postgres?sslmode=disable", endpoint))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = super.Close() }()
	for _, s := range []string{
		`CREATE ROLE migrator LOGIN PASSWORD 'migrator'`,
		`CREATE DATABASE control_plane OWNER migrator`,
	} {
		if _, err := super.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	migrator, err := sql.Open("pgx", db.As("migrator"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = migrator.Close() }()
	p, err := goose.NewProvider(goose.DialectPostgres, migrator, migrations.ControlPlane())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("migrate control_plane: %v", err)
	}
	return db
}
