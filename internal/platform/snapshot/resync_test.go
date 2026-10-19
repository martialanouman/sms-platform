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

func exec(t *testing.T, c *pgx.Conn, statements ...string) {
	t.Helper()
	for _, s := range statements {
		if _, err := c.Exec(context.Background(), s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func connect(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()
	c, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	return c
}

func count(rows []json.RawMessage) (any, error) { return len(rows), nil }

func counted(t *testing.T, tables ...string) (pgtest.DB, *pgx.Conn, *Snapshots) {
	t.Helper()
	pg := pgtest.ControlPlane(t)
	owner := connect(t, pg.As("migrator"))
	exec(t, owner, `CREATE ROLE reader LOGIN PASSWORD 'reader'`, `GRANT USAGE ON SCHEMA control_plane TO reader`)
	var held []Table
	for _, table := range tables {
		exec(t, owner,
			`CREATE TABLE control_plane.`+table+` (id int PRIMARY KEY)`,
			`GRANT SELECT ON control_plane.`+table+` TO reader`,
			`INSERT INTO control_plane.`+table+` VALUES (0)`,
		)
		held = append(held, Table{Name: table, Build: count})
	}
	ctx, cancel := context.WithCancel(context.Background())
	s, err := Open(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), pg.As("reader"), held...)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return pg, owner, s
}

func holds(t *testing.T, s *Snapshots, table string, want int) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); s.Get(table) != want; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%s = %v rows, want %d", table, s.Get(table), want)
		}
	}
}

func TestAPeriodicResyncUnderNotificationsCountsOnlyTheRowsCommittedSince(t *testing.T) {
	defer func(d time.Duration) { resyncEvery = d }(resyncEvery)
	resyncEvery = 200 * time.Millisecond
	_, owner, s := counted(t, "probe")
	notifier := connect(t, s.dsn)
	stop := make(chan struct{})
	flooding := make(chan struct{})
	go func() {
		defer close(flooding)
		for {
			select {
			case <-stop:
				return
			case <-time.After(50 * time.Millisecond):
				if _, err := notifier.Exec(context.Background(), `NOTIFY config, 'unknown:*'`); err != nil {
					t.Error(err)
					return
				}
			}
		}
	}()
	defer func() {
		close(stop)
		<-flooding
	}()

	before := observed(t)
	exec(t, owner, `INSERT INTO control_plane.probe VALUES (1)`)
	holds(t, s, "probe", 2)
	exec(t, owner, `INSERT INTO control_plane.probe VALUES (2)`)
	holds(t, s, "probe", 3)
	exec(t, owner,
		`ALTER TABLE control_plane.probe ADD COLUMN x int DEFAULT 0`,
		`INSERT INTO control_plane.probe VALUES (3)`,
	)
	holds(t, s, "probe", 4)
	if got := observed(t) - before; got != 3 {
		t.Errorf("propagations observed = %d, want 3: a resync counts the rows committed since, not the columns added", got)
	}
}

func TestAResyncStuckOnALockGivesUpAndReconnects(t *testing.T) {
	defer func(r, q time.Duration) { resyncEvery, queryTimeout = r, q }(resyncEvery, queryTimeout)
	resyncEvery, queryTimeout = 200*time.Millisecond, 300*time.Millisecond
	pg, owner, s := counted(t, "other", "probe")
	locker := connect(t, pg.As("migrator"))
	exec(t, locker, `BEGIN`, `LOCK TABLE control_plane.probe IN ACCESS EXCLUSIVE MODE`)
	defer exec(t, locker, `ROLLBACK`)
	super := connect(t, pg.As("postgres"))
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		var waiting bool
		if err := super.QueryRow(context.Background(), `SELECT EXISTS (SELECT FROM pg_stat_activity
			WHERE usename = 'reader' AND wait_event_type = 'Lock')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the resync never waited on the lock")
		}
	}
	exec(t, owner, `INSERT INTO control_plane.other VALUES (1)`)
	holds(t, s, "other", 2)
}
