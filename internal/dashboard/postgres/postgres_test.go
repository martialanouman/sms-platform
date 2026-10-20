package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/martialanouman/sms-platform/internal/dashboard"
	"github.com/martialanouman/sms-platform/internal/dashboard/postgres"
	"github.com/martialanouman/sms-platform/internal/platform/pgtest"
	"github.com/martialanouman/sms-platform/migrations"
)

func TestConsumeLinkSpendsOnlyALiveLinkOfAnActiveAccount(t *testing.T) {
	ctx := context.Background()
	pg := pgtest.ControlPlane(t).In("dashboard")
	owner, err := pgx.Connect(ctx, pg.As("migrator"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(ctx) })
	for _, s := range []string{string(migrations.Grants("dashboard")), `ALTER ROLE dashboard PASSWORD 'dashboard'`} {
		if _, err := owner.Exec(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	pool, err := pgxpool.New(ctx, pg.As("dashboard"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	store := postgres.New(pool)
	for name, tc := range map[string]struct {
		expiresIn, status string
		spent             bool
	}{
		"live":     {"1 hour", "active", true},
		"expired":  {"-1 second", "active", false},
		"disabled": {"1 hour", "disabled", false},
	} {
		token := dashboard.NewToken()
		if _, err := owner.Exec(ctx, `WITH o AS (INSERT INTO dashboard.operators (email, display_name, status) VALUES ($1, 'x', $2) RETURNING id)
			INSERT INTO dashboard.access_links (operator_id, kind, token_hash, expires_at, sent_at)
			SELECT id, 'reset', $3, now() + $4::interval, now() FROM o`, name+"@example.test", tc.status, dashboard.Digest(token), tc.expiresIn); err != nil {
			t.Fatal(err)
		}
		var spent bool
		var kind dashboard.LinkKind
		if err := store.Tx(ctx, func(tx dashboard.Tx) (err error) {
			_, kind, spent, err = tx.ConsumeLink(ctx, dashboard.Digest(token))
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if spent != tc.spent || (spent && kind != dashboard.Reset) {
			t.Errorf("%s link: spent %v as %q, want %v", name, spent, kind, tc.spent)
		}
	}
}
