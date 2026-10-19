package snapshot_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/martialanouman/sms-platform/internal/platform/pgtest"
	"github.com/martialanouman/sms-platform/internal/platform/snapshot"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

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
			`CREATE TABLE control_plane.`+table+` (id int PRIMARY KEY, name text, secret_sealed bytea,
				key_kms_key_ref text, wrapped_key bytea, token_hash text)`,
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

func raw(rows []json.RawMessage) (any, error) { return rows, nil }

var tables = []snapshot.Table{{Name: "probe", Build: raw}, {Name: "other", Build: raw}}

func open(t *testing.T, dsn string, tables ...snapshot.Table) *snapshot.Snapshots {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s, err := snapshot.Open(ctx, quiet, dsn, tables...)
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
	return s
}

func rows(t *testing.T, s *snapshot.Snapshots, table string) map[float64]map[string]any {
	t.Helper()
	out := map[float64]map[string]any{}
	for _, r := range s.Get(table).([]json.RawMessage) {
		var row map[string]any
		if err := json.Unmarshal(r, &row); err != nil {
			t.Fatal(err)
		}
		out[row["id"].(float64)] = row
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

func eventually(t *testing.T, s *snapshot.Snapshots, table string, want ...string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := names(t, s, table)
		if slices.Equal(got, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s = %v, want %v", table, got, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func observed(t *testing.T, reg *prometheus.Registry) uint64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() == "config_propagation_seconds" {
			return f.GetMetric()[0].GetHistogram().GetSampleCount()
		}
	}
	t.Fatal("config_propagation_seconds is not registered")
	return 0
}

func TestAnAttributedWriteReachesTheSnapshotWithoutItsSealedColumns(t *testing.T) {
	reg := prometheus.NewRegistry()
	if err := snapshot.Register(reg); err != nil {
		t.Fatal(err)
	}
	before := observed(t, reg)
	pg, owner := probes(t)
	write(t, owner, `INSERT INTO control_plane.probe VALUES (1, 'a', '\x01', 'k', '\x02', 'h')`)
	s := open(t, pg.As("reader"), tables...)
	eventually(t, s, "probe", "a")

	write(t, owner, `INSERT INTO control_plane.probe VALUES (2, 'b', '\x01', 'k', '\x02', 'h')`)
	eventually(t, s, "probe", "a", "b")
	write(t, owner, `UPDATE control_plane.probe SET name = 'c' WHERE id = 2`)
	eventually(t, s, "probe", "a", "c")
	write(t, owner, `DELETE FROM control_plane.probe WHERE id = 1`)
	eventually(t, s, "probe", "c")

	row := rows(t, s, "probe")[2]
	for _, sealed := range []string{"secret_sealed", "key_kms_key_ref", "wrapped_key"} {
		if _, ok := row[sealed]; ok {
			t.Errorf("%s is in the snapshot: %v", sealed, row)
		}
	}
	if row["token_hash"] != "h" {
		t.Errorf("token_hash = %v, want h", row["token_hash"])
	}
	if got := observed(t, reg) - before; got != 2 {
		t.Errorf("propagations observed = %d, want 2 (insert, update; neither the load nor the delete)", got)
	}
}

func TestACutListenerIsFollowedByAFullResync(t *testing.T) {
	pg, owner := probes(t)
	cuttable, proxy := pg.Cuttable(t)
	s := open(t, cuttable.As("reader"), tables...)
	eventually(t, s, "probe")

	proxy.Cut()
	exec(t, owner,
		`ALTER TABLE control_plane.probe DISABLE TRIGGER audit_capture`,
		`INSERT INTO control_plane.probe (id, name) VALUES (1, 'a')`,
	)
	proxy.Resume()
	eventually(t, s, "probe", "a")
}

func TestAStarNotificationReloadsTheTable(t *testing.T) {
	pg, owner := probes(t)
	s := open(t, pg.As("reader"), tables...)
	exec(t, owner,
		`ALTER TABLE control_plane.probe DISABLE TRIGGER audit_capture`,
		`INSERT INTO control_plane.probe (id, name) VALUES (1, 'a')`,
		`NOTIFY config, 'garbage'`,
		`NOTIFY config, 'probe:{not json'`,
		`NOTIFY config, 'unknown:*'`,
	)
	write(t, owner, `INSERT INTO control_plane.other (id, name) VALUES (1, 'barrier')`)
	eventually(t, s, "other", "barrier")
	if got := names(t, s, "probe"); len(got) != 0 {
		t.Fatalf("probe = %v before probe:*, want nothing: an unreadable notification reloaded it", got)
	}
	exec(t, owner, `NOTIFY config, 'probe:*'`)
	eventually(t, s, "probe", "a")
}

func TestAFailedBuildKeepsThePreviousView(t *testing.T) {
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
	s := open(t, pg.As("reader"), refusing, tables[1])
	write(t, owner, `INSERT INTO control_plane.probe (id, name) VALUES (2, 'bad')`)
	write(t, owner, `INSERT INTO control_plane.other (id, name) VALUES (1, 'barrier')`)
	eventually(t, s, "other", "barrier")
	if got := names(t, s, "probe"); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("probe = %v, want the previous view [a]", got)
	}
}

func TestOpenRefuses(t *testing.T) {
	pg, owner := probes(t)
	exec(t, owner,
		`CREATE TABLE control_plane.ungranted (id int PRIMARY KEY)`,
		`CREATE TABLE control_plane.keyless (id int)`,
		`GRANT SELECT ON control_plane.keyless TO reader`,
	)
	write(t, owner, `INSERT INTO control_plane.probe (id, name) VALUES (1, 'a')`)
	failing := func([]json.RawMessage) (any, error) { return nil, errors.New("refused") }
	for name, table := range map[string]snapshot.Table{
		"a table without grant":       {Name: "ungranted", Build: raw},
		"a table without primary key": {Name: "keyless", Build: raw},
		"a failed build":              {Name: "probe", Build: failing},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := snapshot.Open(context.Background(), quiet, pg.As("reader"), table); err == nil {
				t.Fatal("Open succeeded")
			}
		})
	}
}
