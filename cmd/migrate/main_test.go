package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/martialanouman/sms-platform/internal/platform/config"
	"github.com/martialanouman/sms-platform/internal/platform/pgtest"
	"github.com/martialanouman/sms-platform/migrations"
)

func TestRunAppliesEveryMigrationOnceAsMigrator(t *testing.T) {
	pg := pgtest.Empty(t)
	url := pg.As("migrator")
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	ctx := context.Background()
	if err := run(ctx, config.Infra{PostgresURL: url}, log); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.ControlPlane())
	if err != nil {
		t.Fatal(err)
	}
	sources := p.ListSources()
	latest := sources[len(sources)-1].Version
	if got, err := p.GetDBVersion(ctx); err != nil || got != latest {
		t.Fatalf("version = %d, %v; want %d", got, err, latest)
	}
	if n := strings.Count(logs.String(), `"msg":"migration applied"`); n != len(sources) {
		t.Errorf("%d migrations logged, want %d:\n%s", n, len(sources), logs.String())
	}
	if strings.Contains(logs.String(), url) {
		t.Errorf("the URL reached the log:\n%s", logs.String())
	}
	logs.Reset()
	if err := run(ctx, config.Infra{PostgresURL: url}, log); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.String(), `"msg":"migration applied"`) {
		t.Errorf("a second run applied a migration:\n%s", logs.String())
	}
}

func TestRunRefusesAnEmptyURL(t *testing.T) {
	err := run(context.Background(), config.Infra{}, slog.New(slog.DiscardHandler))
	if !errors.Is(err, errNoURL) {
		t.Fatalf("run = %v, want %v", err, errNoURL)
	}
}
