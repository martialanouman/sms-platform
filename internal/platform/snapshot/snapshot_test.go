package snapshot_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/martialanouman/sms-platform/internal/platform/pgtest"
	"github.com/martialanouman/sms-platform/internal/platform/snapshot"
)

type logs struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logs) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *logs) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func (l *logs) logger() *slog.Logger { return slog.New(slog.NewTextHandler(l, nil)) }

func connect(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()
	c, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	return c
}

func exec(t *testing.T, c *pgx.Conn, statements ...string) {
	t.Helper()
	for _, s := range statements {
		if _, err := c.Exec(context.Background(), s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func probes(t *testing.T) (pgtest.DB, *pgx.Conn) {
	t.Helper()
	pg := pgtest.ControlPlane(t)
	owner := connect(t, pg.As("migrator"))
	for _, table := range []string{"probe", "other"} {
		exec(t, owner,
			`CREATE TABLE control_plane.`+table+` (id int, name text, secret_sealed bytea, key_kms_key_ref text,
				wrapped_key bytea, token_hash text, unwrapped_key text, PRIMARY KEY (id, name))`,
			`CREATE TRIGGER audit_capture AFTER INSERT OR UPDATE OR DELETE ON control_plane.`+table+`
				FOR EACH ROW EXECUTE FUNCTION control_plane.audit_capture()`,
		)
	}
	exec(t, owner,
		`CREATE ROLE reader LOGIN PASSWORD 'reader'`,
		`GRANT USAGE ON SCHEMA control_plane TO reader`,
		`GRANT SELECT ON control_plane.probe, control_plane.other TO reader`,
	)
	return pg, owner
}

func write(t *testing.T, c *pgx.Conn, statement string) {
	t.Helper()
	exec(t, c, `BEGIN`, `SET LOCAL app.actor = 'sub-1'`, statement, `COMMIT`)
}

func silently(t *testing.T, c *pgx.Conn, table, statement string) {
	t.Helper()
	exec(t, c,
		`ALTER TABLE control_plane.`+table+` DISABLE TRIGGER audit_capture`,
		statement,
		`ALTER TABLE control_plane.`+table+` ENABLE TRIGGER audit_capture`,
	)
}

func raw(rows []json.RawMessage) (any, error) { return rows, nil }

var tables = []snapshot.Table{{Name: "probe", Build: raw}, {Name: "other", Build: raw}}

func run(t *testing.T, s *snapshot.Snapshots) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

func open(t *testing.T, dsn string, tables ...snapshot.Table) (*snapshot.Snapshots, *logs) {
	t.Helper()
	l := &logs{}
	s, err := snapshot.Open(context.Background(), l.logger(), dsn, tables...)
	if err != nil {
		t.Fatal(err)
	}
	run(t, s)
	return s, l
}

func rows(t *testing.T, s *snapshot.Snapshots, table string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, r := range s.Get(table).([]json.RawMessage) {
		var row map[string]any
		if err := json.Unmarshal(r, &row); err != nil {
			t.Fatal(err)
		}
		out = append(out, row)
	}
	return out
}

func names(t *testing.T, s *snapshot.Snapshots, table string) []string {
	t.Helper()
	var out []string
	for _, r := range rows(t, s, table) {
		out = append(out, r["name"].(string))
	}
	slices.Sort(out)
	return out
}

func until(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !cond(); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("still not %s after 5 s", what)
		}
	}
}

func eventually(t *testing.T, s *snapshot.Snapshots, table string, want ...string) {
	t.Helper()
	var got []string
	until(t, table+" = "+strings.Join(want, ","), func() bool {
		got = names(t, s, table)
		return slices.Equal(got, want)
	})
}

func observed(t *testing.T, reg *prometheus.Registry) (count uint64, sum float64) {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() == "config_propagation_seconds" {
			h := f.GetMetric()[0].GetHistogram()
			return h.GetSampleCount(), h.GetSampleSum()
		}
	}
	t.Fatal("config_propagation_seconds is not registered")
	return 0, 0
}

func registry(t *testing.T) *prometheus.Registry {
	t.Helper()
	reg := prometheus.NewRegistry()
	if err := snapshot.Register(reg); err != nil {
		t.Fatal(err)
	}
	return reg
}

func TestAnAttributedWriteReachesTheSnapshotWithoutItsSealedColumns(t *testing.T) {
	reg := registry(t)
	before, _ := observed(t, reg)
	pg, owner := probes(t)
	write(t, owner, `INSERT INTO control_plane.probe VALUES (1, 'a', '\x01', 'k', '\x02', 'h', 'u')`)
	s, _ := open(t, pg.As("reader"), tables...)
	eventually(t, s, "probe", "a")

	row := rows(t, s, "probe")[0]
	for _, sealed := range []string{"secret_sealed", "key_kms_key_ref", "wrapped_key"} {
		if _, ok := row[sealed]; ok {
			t.Errorf("%s is in the snapshot: %v", sealed, row)
		}
	}
	if row["token_hash"] != "h" || row["unwrapped_key"] != "u" {
		t.Errorf("token_hash, unwrapped_key = %v, %v, want h, u", row["token_hash"], row["unwrapped_key"])
	}

	write(t, owner, `INSERT INTO control_plane.probe (id, name) VALUES (2, 'b')`)
	eventually(t, s, "probe", "a", "b")
	write(t, owner, `UPDATE control_plane.probe SET name = 'c' WHERE id = 2`)
	eventually(t, s, "probe", "a", "c")
	write(t, owner, `DELETE FROM control_plane.probe WHERE id = 1`)
	eventually(t, s, "probe", "c")
	if after, _ := observed(t, reg); after-before != 2 {
		t.Errorf("propagations observed = %d, want 2 (insert, update; neither the load nor the delete)", after-before)
	}
}

func TestACutListenerIsFollowedByAFullResyncThatSeesTheDelay(t *testing.T) {
	reg := registry(t)
	pg, owner := probes(t)
	write(t, owner, `INSERT INTO control_plane.probe (id, name) VALUES (1, 'a')`)
	cuttable, proxy := pg.Cuttable(t)
	s, l := open(t, cuttable.As("reader"), tables...)
	count, sum := observed(t, reg)

	proxy.Cut()
	write(t, owner, `INSERT INTO control_plane.probe (id, name) VALUES (2, 'b')`)
	committed := time.Now()
	until(t, "refused a reconnection", func() bool { return strings.Contains(l.String(), "reconnection failed") })
	if got := names(t, s, "probe"); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("probe = %v during the cut, want the current view [a]", got)
	}
	cut := time.Since(committed)
	proxy.Resume()
	eventually(t, s, "probe", "a", "b")

	afterCount, afterSum := observed(t, reg)
	if afterCount-count != 1 || afterSum-sum < cut.Seconds() {
		t.Fatalf("observed %d propagations summing %.3f s, want 1 of at least %.3f s", afterCount-count, afterSum-sum,
			cut.Seconds())
	}
}

func TestAStarNotificationReloadsItsTableAndOnlyIt(t *testing.T) {
	pg, owner := probes(t)
	s, l := open(t, pg.As("reader"), tables[1], tables[0])
	silently(t, owner, "probe", `INSERT INTO control_plane.probe (id, name) VALUES (1, 'a')`)
	silently(t, owner, "other", `INSERT INTO control_plane.other (id, name) VALUES (2, 'silent')`)
	exec(t, owner,
		`NOTIFY config, 'garbage'`,
		`NOTIFY config, 'probe:{not json'`,
		`NOTIFY config, 'unknown:*'`,
	)
	write(t, owner, `INSERT INTO control_plane.other (id, name) VALUES (1, 'barrier')`)
	eventually(t, s, "other", "barrier")
	if got := names(t, s, "probe"); len(got) != 0 {
		t.Fatalf("probe = %v before probe:*, want nothing: an unreadable notification reloaded it", got)
	}
	if got := l.String(); !strings.Contains(got, "without a readable key") || strings.Contains(got, "not json") {
		t.Fatalf("logs = %q, want the unreadable notification logged without its payload", got)
	}

	exec(t, owner, `NOTIFY config, 'probe:*'`)
	eventually(t, s, "probe", "a")
	if got := names(t, s, "other"); !slices.Equal(got, []string{"barrier"}) {
		t.Fatalf("other = %v, want [barrier]: probe:* resynced every table", got)
	}
}

func TestAFailedBuildKeepsThePreviousViewWithoutResync(t *testing.T) {
	pg, owner := probes(t)
	refusing := snapshot.Table{Name: "probe", Build: func(rows []json.RawMessage) (any, error) {
		for _, r := range rows {
			var row struct{ Name string }
			if err := json.Unmarshal(r, &row); err != nil || row.Name == "bad" {
				return nil, errors.New("refused")
			}
		}
		return rows, nil
	}}
	write(t, owner, `INSERT INTO control_plane.probe (id, name) VALUES (1, 'a')`)
	s, _ := open(t, pg.As("reader"), refusing, tables[1])
	silently(t, owner, "other", `INSERT INTO control_plane.other (id, name) VALUES (2, 'silent')`)
	write(t, owner, `INSERT INTO control_plane.probe (id, name) VALUES (2, 'bad')`)
	write(t, owner, `INSERT INTO control_plane.other (id, name) VALUES (1, 'barrier')`)
	eventually(t, s, "other", "barrier")
	if got := names(t, s, "probe"); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("probe = %v, want the previous view [a]", got)
	}
}

func TestTheListenerListensBeforeItLoads(t *testing.T) {
	pg, owner := probes(t)
	loading, release := make(chan struct{}), make(chan struct{})
	var first atomic.Bool
	blocking := snapshot.Table{Name: "probe", Build: func(rows []json.RawMessage) (any, error) {
		if first.CompareAndSwap(false, true) {
			close(loading)
			<-release
		}
		return rows, nil
	}}
	opened := make(chan *snapshot.Snapshots, 1)
	go func() {
		s, err := snapshot.Open(context.Background(), (&logs{}).logger(), pg.As("reader"), blocking)
		if err != nil {
			t.Error(err)
		}
		opened <- s
	}()
	<-loading
	write(t, owner, `INSERT INTO control_plane.probe (id, name) VALUES (1, 'a')`)
	close(release)
	s := <-opened
	if s == nil {
		t.FailNow()
	}
	run(t, s)
	eventually(t, s, "probe", "a")
}

func TestOpenWaitsForTheNetworkToAdmitIt(t *testing.T) {
	pg, _ := probes(t)
	s, _ := open(t, pg.Late(t, 2*time.Second).As("reader"), tables...)
	eventually(t, s, "probe")
}

func TestOpenRefusesAndLeavesNoConnection(t *testing.T) {
	pg, owner := probes(t)
	exec(t, owner,
		`CREATE TABLE control_plane.ungranted (id int PRIMARY KEY)`,
		`CREATE TABLE control_plane.keyless (id int)`,
		`CREATE TABLE control_plane.hashed (token_hash text PRIMARY KEY)`,
		`CREATE TABLE control_plane.zoned (id int, at timestamptz, PRIMARY KEY (id, at))`,
		`GRANT SELECT ON control_plane.keyless, control_plane.hashed, control_plane.zoned TO reader`,
	)
	write(t, owner, `INSERT INTO control_plane.probe (id, name) VALUES (1, 'a')`)
	failing := func([]json.RawMessage) (any, error) { return nil, errors.New("refused") }
	for _, tc := range []struct {
		name  string
		table snapshot.Table
		want  string
	}{
		{"a table without grant", snapshot.Table{Name: "ungranted", Build: raw}, "permission denied"},
		{"a table without primary key", snapshot.Table{Name: "keyless", Build: raw}, "no primary key"},
		{"a key the notification filters out", snapshot.Table{Name: "hashed", Build: raw}, "filtered out"},
		{"a key in the session time zone", snapshot.Table{Name: "zoned", Build: raw}, "time zone"},
		{"a failed build", snapshot.Table{Name: "probe", Build: failing}, "refused"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := snapshot.Open(context.Background(), (&logs{}).logger(), pg.As("reader"), tables[1], tc.table)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Open = %v, want an error with %q", err, tc.want)
			}
		})
	}
	super := connect(t, pg.As("postgres"))
	until(t, "every connection of reader closed", func() bool {
		var n int
		err := super.QueryRow(context.Background(),
			`SELECT count(*) FROM pg_stat_activity WHERE usename = 'reader'`).Scan(&n)
		return err == nil && n == 0
	})
}
