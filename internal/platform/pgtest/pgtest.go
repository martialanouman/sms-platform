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

// DB holds the connection strings of a migrated database, as its superuser and as migrator.
type DB struct {
	Super    string
	Migrator string
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
	dsn := func(user, database string) string {
		return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", user, user, endpoint, database)
	}
	db := DB{Super: dsn("postgres", "control_plane"), Migrator: dsn("migrator", "control_plane")}

	exec(t, dsn("postgres", "postgres"),
		`CREATE ROLE migrator LOGIN PASSWORD 'migrator'`,
		`CREATE DATABASE control_plane OWNER migrator`,
	)
	migrator, err := sql.Open("pgx", db.Migrator)
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

func exec(t testing.TB, dsn string, statements ...string) {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, s := range statements {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}
