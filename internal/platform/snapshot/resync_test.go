package snapshot

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	dto "github.com/prometheus/client_model/go"

	"github.com/martialanouman/sms-platform/internal/platform/pgtest"
)

func observed(t *testing.T) uint64 {
	t.Helper()
	var m dto.Metric
	if err := propagation.Write(&m); err != nil {
		t.Fatal(err)
	}
	return m.GetHistogram().GetSampleCount()
}

func TestAPeriodicResyncCatchesAWriteThatNeverNotifiedAndCountsOnlyIt(t *testing.T) {
	defer func(d time.Duration) { resyncEvery = d }(resyncEvery)
	resyncEvery = 200 * time.Millisecond
	ctx := context.Background()
	pg := pgtest.ControlPlane(t)
	owner, err := pgx.Connect(ctx, pg.As("migrator"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close(ctx) }()
	for _, s := range []string{
		`CREATE TABLE control_plane.probe (id int PRIMARY KEY)`,
		`CREATE ROLE reader LOGIN PASSWORD 'reader'`,
		`GRANT USAGE ON SCHEMA control_plane TO reader`,
		`GRANT SELECT ON control_plane.probe TO reader`,
		`INSERT INTO control_plane.probe VALUES (0)`,
	} {
		if _, err := owner.Exec(ctx, s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	runCtx, cancel := context.WithCancel(ctx)
	s, err := Open(runCtx, slog.New(slog.NewTextHandler(io.Discard, nil)), pg.As("reader"),
		Table{Name: "probe", Build: func(rows []json.RawMessage) (any, error) { return len(rows), nil }})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(runCtx)
	}()
	defer func() {
		cancel()
		<-done
	}()

	before := observed(t)
	for want := 2; want <= 3; want++ {
		if _, err := owner.Exec(ctx, `INSERT INTO control_plane.probe VALUES ($1)`, want); err != nil {
			t.Fatal(err)
		}
		for deadline := time.Now().Add(5 * time.Second); s.Get("probe") != want; time.Sleep(20 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("probe = %v rows, want %d", s.Get("probe"), want)
			}
		}
	}
	if got := observed(t) - before; got != 2 {
		t.Errorf("propagations observed = %d, want 2: a resync counts only the rows it changes", got)
	}
}
