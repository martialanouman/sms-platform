// Package snapshot holds an immutable snapshot of control_plane tables, kept fresh by LISTEN/NOTIFY and a periodic
// resync (docs/design/07-distribution-de-config-et-redis.md §3).
package snapshot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/martialanouman/sms-platform/internal/platform/pgdial"
)

var (
	resyncEvery  = 5 * time.Minute
	queryTimeout = 10 * time.Second
)

var propagation = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "config_propagation_seconds",
	Help:    "Delay from the commit of a configuration row to its arrival in a snapshot.",
	Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 300},
})

// Register registers the propagation histogram on r.
func Register(r prometheus.Registerer) error { return r.Register(propagation) }

// Table is a control_plane table to hold, and how to build its view from its rows, given in no order.
type Table struct {
	Name  string
	Build func(rows []json.RawMessage) (any, error)
}

// Snapshots holds the view of each table; Run keeps them fresh.
type Snapshots struct {
	log    *slog.Logger
	dsn    string
	tables []Table
	conn   *pgx.Conn
	keys   map[string][]string
	rows   map[string]map[string]string
	newest map[string]time.Time
	views  atomic.Pointer[map[string]any]
}

// Open listens on a direct connection to dsn and returns once every table is loaded.
func Open(ctx context.Context, log *slog.Logger, dsn string, tables ...Table) (*Snapshots, error) {
	s := &Snapshots{log: log, dsn: dsn, tables: tables, keys: map[string][]string{},
		rows: map[string]map[string]string{}, newest: map[string]time.Time{}}
	s.views.Store(&map[string]any{})
	if err := pgdial.Retry(ctx, log, s.connect); err != nil {
		if s.conn != nil {
			_ = s.conn.Close(context.Background())
		}
		return nil, err
	}
	return s, nil
}

// Get is the current view of table, nil for a table it does not hold.
func (s *Snapshots) Get(table string) any {
	return (*s.views.Load())[table]
}

// Run applies notifications and resyncs until ctx ends; while Postgres is unreachable, the current views serve.
func (s *Snapshots) Run(ctx context.Context) {
	defer func() { _ = s.conn.Close(context.Background()) }()
	for {
		err := s.listen(ctx)
		if ctx.Err() != nil {
			return
		}
		s.log.Warn("config listener lost", "error", err)
		for err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			if err = s.connect(ctx); err != nil {
				s.log.Warn("config listener reconnection failed", "error", err)
			}
		}
	}
}

func (s *Snapshots) connect(ctx context.Context) error {
	if s.conn != nil {
		_ = s.conn.Close(ctx)
	}
	bounded, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	conn, err := pgx.Connect(bounded, s.dsn)
	if err != nil {
		return err
	}
	s.conn = conn
	if _, err := conn.Exec(bounded, "LISTEN config"); err != nil {
		return err
	}
	return s.resync(ctx)
}

func (s *Snapshots) resync(ctx context.Context) error {
	for _, t := range s.tables {
		if err := s.load(ctx, t.Name, ""); err != nil {
			return err
		}
	}
	return nil
}

func (s *Snapshots) listen(ctx context.Context) error {
	next := time.Now().Add(resyncEvery)
	for {
		wait, cancel := context.WithDeadline(ctx, next)
		n, err := s.conn.WaitForNotification(wait)
		cancel()
		switch {
		case err != nil && ctx.Err() == nil && errors.Is(wait.Err(), context.DeadlineExceeded):
			if err := s.resync(ctx); err != nil {
				return err
			}
			next = time.Now().Add(resyncEvery)
		case err != nil:
			return err
		default:
			if err := s.apply(ctx, n.Payload); err != nil {
				return err
			}
		}
	}
}

func (s *Snapshots) apply(ctx context.Context, payload string) error {
	table, key, _ := strings.Cut(payload, ":")
	if _, held := s.rows[table]; !held {
		return nil
	}
	if key == "*" {
		return s.load(ctx, table, "")
	}
	if !json.Valid([]byte(key)) {
		s.log.Warn("config notification without a readable key", "table", table)
		return nil
	}
	return s.load(ctx, table, key)
}

func (s *Snapshots) describe(ctx context.Context, table string) error {
	rows, err := s.conn.Query(ctx, `SELECT a.attname::text,
			a.attname ~ '(_sealed|_hash|_kms_key_ref)$' OR a.attname = 'wrapped_key',
			a.atttypid IN ('timestamptz'::regtype, 'timetz'::regtype)
		FROM pg_index i JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY (i.indkey)
		WHERE i.indrelid = $1::regclass AND i.indisprimary ORDER BY a.attnum`, ident(table))
	if err != nil {
		return err
	}
	var key []string
	var column string
	var filtered, zoned bool
	_, err = pgx.ForEachRow(rows, []any{&column, &filtered, &zoned}, func() error {
		switch {
		case filtered:
			return fmt.Errorf("key column %s is filtered out of its notification", column)
		case zoned:
			return fmt.Errorf("key column %s is rendered in the session time zone", column)
		}
		key = append(key, column)
		return nil
	})
	if err != nil {
		return err
	}
	if len(key) == 0 {
		return errors.New("no primary key")
	}
	s.keys[table] = key
	return nil
}

func (s *Snapshots) load(ctx context.Context, table, key string) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	if key == "" {
		if err := s.describe(ctx, table); err != nil {
			return fmt.Errorf("snapshot: %s: %w", table, err)
		}
	}
	q := fmt.Sprintf(`SELECT (SELECT jsonb_object_agg(c, to_jsonb(t) -> c) FROM unnest($2::text[]) c)::text AS pk,
		(to_jsonb(t) - ARRAY(SELECT attname::text FROM pg_attribute WHERE attrelid = $1::regclass AND attnum > 0
			AND NOT attisdropped AND (attname ~ '(_sealed|_kms_key_ref)$' OR attname = 'wrapped_key')))::text AS row,
		pg_xact_commit_timestamp(t.xmin) AS committed
		FROM %s t`, ident(table))
	args := []any{ident(table), s.keys[table]}
	held, loaded := s.rows[table]
	next := map[string]string{}
	if key != "" {
		q = `SELECT * FROM (` + q + `) r WHERE r.pk::jsonb = $3::jsonb`
		args = append(args, key)
		next = held
		delete(next, key)
	}
	rows, err := s.conn.Query(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("snapshot: %s: %w", table, err)
	}
	newest := s.newest[table]
	latest := newest
	var pk, row string
	var committed *time.Time
	_, err = pgx.ForEachRow(rows, []any{&pk, &row, &committed}, func() error {
		if committed != nil && loaded && committed.After(newest) {
			propagation.Observe(time.Since(*committed).Seconds())
		}
		if committed != nil && committed.After(latest) {
			latest = *committed
		}
		next[pk] = row
		return nil
	})
	if err != nil {
		return fmt.Errorf("snapshot: %s: %w", table, err)
	}
	s.rows[table] = next
	s.newest[table] = latest
	return s.build(table, loaded)
}

func (s *Snapshots) build(table string, keepOnFailure bool) error {
	i := slices.IndexFunc(s.tables, func(t Table) bool { return t.Name == table })
	rows := make([]json.RawMessage, 0, len(s.rows[table]))
	for _, r := range s.rows[table] {
		rows = append(rows, json.RawMessage(r))
	}
	view, err := s.tables[i].Build(rows)
	if err != nil && keepOnFailure {
		s.log.Error("config view not rebuilt, the previous one serves", "table", table, "error", err)
		return nil
	}
	if err != nil {
		return fmt.Errorf("snapshot: build %s: %w", table, err)
	}
	views := maps.Clone(*s.views.Load())
	views[table] = view
	s.views.Store(&views)
	return nil
}

func ident(table string) string {
	return pgx.Identifier{"control_plane", table}.Sanitize()
}
