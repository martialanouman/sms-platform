// Command migrate applies the pending migrations of each database as migrator, then exits.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/martialanouman/sms-platform/internal/platform/config"
	"github.com/martialanouman/sms-platform/internal/platform/observability"
	"github.com/martialanouman/sms-platform/internal/platform/pgdial"
	"github.com/martialanouman/sms-platform/migrations"
)

var errNoURL = errors.New("POSTGRES_URL is empty")

func main() {
	infra, err := config.Load()
	log := observability.NewLogger(os.Stdout, "migrate", infra.LogLevel)
	if err == nil {
		err = run(context.Background(), infra, log)
	}
	if err != nil {
		log.Error("migrate failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, infra config.Infra, log *slog.Logger) error {
	if infra.PostgresURL == "" {
		return errNoURL
	}
	for _, database := range migrations.Databases {
		url, err := pgdial.Database(infra.PostgresURL, database)
		if err != nil {
			return err
		}
		if err := migrate(ctx, url, database, log); err != nil {
			return fmt.Errorf("%s: %w", database, err)
		}
	}
	return nil
}

func migrate(ctx context.Context, url, database string, log *slog.Logger) error {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer func() { _ = db.Close() }()
	if err := pgdial.Retry(ctx, log, db.PingContext); err != nil {
		return fmt.Errorf("reach: %w", err)
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Database(database), goose.WithSessionLocker(locker))
	if err != nil {
		return err
	}
	results, err := p.Up(ctx)
	for _, r := range results {
		if r.Error == nil {
			log.Info("migration applied", "database", database, "version", r.Source.Version, "duration", r.Duration)
		}
	}
	return err
}
