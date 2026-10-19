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
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/martialanouman/sms-platform/internal/platform/pgdial"
)

var resyncEvery = 5 * time.Minute

const reconnectEvery = time.Second

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
	views  atomic.Pointer[map[string]any]
}

// Open listens on a direct connection to dsn and returns once every table is loaded.
func Open(ctx context.Context, log *slog.Logger, dsn string, tables ...Table) (*Snapshots, error) {
	s := &Snapshots{log: log, dsn: dsn, tables: tables, keys: map[string][]string{}, rows: map[string]map[string]string{}}
	s.views.Store(&map[string]any{})
	if err := pgdial.Retry(ctx, log, func(ctx context.Context) error { return s.connect(ctx, false) }); err != nil {
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
			case <-time.After(reconnectEvery):
			}
			if err = s.connect(ctx, true); err != nil {
				s.log.Warn("config listener reconnection failed", "error", err)
			}
		}
	}
}

func (s *Snapshots) connect(ctx context.Context, observe bool) error {
	if s.conn != nil {
		_ = s.conn.Close(ctx)
	}
	conn, err := pgx.Connect(ctx, s.dsn)
	if err != nil {
		return err
	}
	s.conn = conn
	if _, err := conn.Exec(ctx, "LISTEN config"); err != nil {
		return err
	}
	return s.resync(ctx, observe)
}

func (s *Snapshots) resync(ctx context.Context, observe bool) error {
	for _, t := range s.tables {
		name := t.Name
		var key []string
		err := s.conn.QueryRow(ctx, `SELECT array_agg(a.attname::text ORDER BY a.attnum) FROM pg_index i
			JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY (i.indkey)
			WHERE i.indrelid = $1::regclass AND i.indisprimary`, ident(name)).Scan(&key)
		if err != nil {
			return fmt.Errorf("snapshot: %s: %w", name, err)
		}
		if len(key) == 0 {
			return fmt.Errorf("snapshot: %s has no primary key", name)
		}
		s.keys[name] = key
		if err := s.load(ctx, name, "", observe); err != nil {
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
			if err := s.resync(ctx, true); err != nil {
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
	table, key, ok := strings.Cut(payload, ":")
	if _, held := s.rows[table]; !ok || !held {
		return nil
	}
	if key == "*" {
		return s.load(ctx, table, "", true)
	}
	if !json.Valid([]byte(key)) {
		s.log.Warn("config notification without a readable key", "table", table)
		return nil
	}
	return s.load(ctx, table, key, true)
}

// load reads key's row of table, or every row when key is empty, and rebuilds the table's view.
func (s *Snapshots) load(ctx context.Context, table, key string, observe bool) error {
	q := fmt.Sprintf(`SELECT (SELECT jsonb_object_agg(c, to_jsonb(t) -> c) FROM unnest($2::text[]) c)::text AS pk,
		(to_jsonb(t) - ARRAY(SELECT attname::text FROM pg_attribute WHERE attrelid = $1::regclass AND attnum > 0
			AND NOT attisdropped AND (attname ~ '(_sealed|_kms_key_ref)$' OR attname = 'wrapped_key')))::text AS row,
		pg_xact_commit_timestamp(t.xmin) AS committed
		FROM %s t`, ident(table))
	args := []any{ident(table), s.keys[table]}
	held := s.rows[table]
	next := map[string]string{}
	if key != "" {
		q = `SELECT * FROM (` + q + `) r WHERE r.pk::jsonb = $3::jsonb`
		args = append(args, key)
		next = maps.Clone(held)
		delete(next, key)
	}
	rows, err := s.conn.Query(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("snapshot: %s: %w", table, err)
	}
	var pk, row string
	var committed pgtype.Timestamptz
	_, err = pgx.ForEachRow(rows, []any{&pk, &row, &committed}, func() error {
		if observe && held[pk] != row && committed.Valid {
			propagation.Observe(time.Since(committed.Time).Seconds())
		}
		next[pk] = row
		return nil
	})
	if err != nil {
		return fmt.Errorf("snapshot: %s: %w", table, err)
	}
	s.rows[table] = next
	return s.build(table, observe)
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
