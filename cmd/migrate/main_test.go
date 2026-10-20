package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"

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
	applied := 0
	for _, database := range migrations.Databases {
		db, err := sql.Open("pgx", pg.In(database).As("migrator"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Database(database))
		if err != nil {
			t.Fatal(err)
		}
		sources := p.ListSources()
		latest := sources[len(sources)-1].Version
		if got, err := p.GetDBVersion(ctx); err != nil || got != latest {
			t.Fatalf("%s at version %d, %v; want %d", database, got, err, latest)
		}
		if n := strings.Count(logs.String(), `"database":"`+database+`"`); n != len(sources) {
			t.Errorf("%s: %d migrations logged, want %d:\n%s", database, n, len(sources), logs.String())
		}
		applied += len(sources)
	}
	if n := strings.Count(logs.String(), `"msg":"migration applied"`); n != applied {
		t.Errorf("%d migrations logged, want %d", n, applied)
	}
	if strings.Contains(logs.String(), "migrator") {
		t.Errorf("the URL or its password reached the log:\n%s", logs.String())
	}
	logs.Reset()
	if err := run(ctx, config.Infra{PostgresURL: url}, log); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.String(), `"msg":"migration applied"`) {
		t.Errorf("a second run applied a migration:\n%s", logs.String())
	}
}

func TestTwoSimultaneousRunsBothSucceed(t *testing.T) {
	infra := config.Infra{PostgresURL: pgtest.Empty(t).As("migrator")}
	db, err := sql.Open("pgx", infra.PostgresURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := database.NewStore(database.DialectPostgres, goose.DefaultTablename)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateVersionTable(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := store.Insert(context.Background(), db, database.InsertRequest{Version: 0}); err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 2)
	start := make(chan struct{})
	for range 2 {
		go func() {
			<-start
			errs <- run(context.Background(), infra, slog.New(slog.DiscardHandler))
		}()
	}
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
}

func TestRunWaitsForTheNetworkToAdmitIt(t *testing.T) {
	var logs bytes.Buffer
	err := run(context.Background(), config.Infra{PostgresURL: pgtest.Empty(t).Late(t, 2*time.Second).As("migrator")}, slog.New(slog.NewJSONHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "postgres unreachable, retrying") {
		t.Errorf("log = %s, want the retries", logs.String())
	}
}

func TestRunRefusesAnEmptyURL(t *testing.T) {
	err := run(context.Background(), config.Infra{}, slog.New(slog.DiscardHandler))
	if !errors.Is(err, errNoURL) {
		t.Fatalf("run = %v, want %v", err, errNoURL)
	}
}
