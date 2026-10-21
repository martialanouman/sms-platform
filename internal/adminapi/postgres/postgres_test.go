package postgres_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/martialanouman/sms-platform/internal/adminapi"
	"github.com/martialanouman/sms-platform/internal/adminapi/adminapitest"
	"github.com/martialanouman/sms-platform/internal/adminapi/postgres"
	"github.com/martialanouman/sms-platform/internal/platform/pgtest"
	"github.com/martialanouman/sms-platform/migrations"
)

const operator = adminapitest.Operator

type db struct {
	store    postgres.Store
	migrator *pgx.Conn
}

func setup(t *testing.T) db {
	t.Helper()
	ctx := context.Background()
	pg := pgtest.ControlPlane(t)
	migrator := connect(t, pg.As("migrator"))
	exec(t, migrator, string(migrations.Grants("control_plane")), `ALTER ROLE admin_api_svc PASSWORD 'admin_api_svc'`,
		`CREATE TABLE control_plane.probe (id int PRIMARY KEY)`,
		`CREATE TRIGGER audit_capture AFTER INSERT OR UPDATE OR DELETE ON control_plane.probe
			FOR EACH ROW EXECUTE FUNCTION control_plane.audit_capture()`,
		`GRANT INSERT ON control_plane.probe TO admin_api_svc`)
	pool, err := pgxpool.New(ctx, pg.As("admin_api_svc")+"&pool_max_conns=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return db{store: postgres.New(pool), migrator: migrator}
}

func connect(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

func exec(t *testing.T, conn *pgx.Conn, statements ...string) {
	t.Helper()
	for _, s := range statements {
		if _, err := conn.Exec(context.Background(), s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func TestAWriteInATransactionIsCapturedUnderItsActorAndRequest(t *testing.T) {
	d := setup(t)
	ctx := context.Background()

	err := d.store.Tx(ctx, operator, "req-1", func(tx adminapi.Tx) error {
		_, err := postgres.DB(tx).Exec(ctx, `INSERT INTO control_plane.probe VALUES (1)`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var actor, request string
	if err := d.migrator.QueryRow(ctx, `SELECT actor, request_id FROM control_plane.audit_changes`).Scan(&actor, &request); err != nil {
		t.Fatal(err)
	}
	if actor != operator || request != "req-1" {
		t.Fatalf("captured under %q, %q", actor, request)
	}
}

func TestTheActorDoesNotOutliveItsTransaction(t *testing.T) {
	d := setup(t)
	ctx := context.Background()
	if err := d.store.Tx(ctx, operator, "req-1", func(adminapi.Tx) error { return nil }); err != nil {
		t.Fatal(err)
	}
	var actor, request string
	if err := postgres.Pool(d.store).QueryRow(ctx, `SELECT coalesce(current_setting('app.actor', true), ''),
		coalesce(current_setting('app.request_id', true), '')`).Scan(&actor, &request); err != nil {
		t.Fatal(err)
	}
	if actor != "" || request != "" {
		t.Fatalf("the pooled connection kept actor %q, request %q", actor, request)
	}
}

func TestAFailedTransactionRecordsNothing(t *testing.T) {
	d := setup(t)
	ctx := context.Background()
	refused := errors.New("refused")

	err := d.store.Tx(ctx, operator, "req-1", func(tx adminapi.Tx) error {
		if err := tx.Record(ctx, adminapi.Entry{Operator: operator, OperationID: "op", Target: "/t", Status: 200}); err != nil {
			return err
		}
		return refused
	})
	if !errors.Is(err, refused) {
		t.Fatalf("err = %v", err)
	}
	if n := count(t, d); n != 0 {
		t.Fatalf("%d lines after a rollback", n)
	}
}

func count(t *testing.T, d db) int {
	t.Helper()
	var n int
	if err := d.migrator.QueryRow(context.Background(), `SELECT count(*) FROM control_plane.audit_log`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestARecordIsWrittenOnceWithItsOutcomeTime(t *testing.T) {
	d := setup(t)
	ctx := context.Background()
	before := time.Now()

	if err := d.store.Record(ctx, adminapi.Entry{Operator: operator, OperationID: "list-audit-log", Target: "/admin/audit-log",
		RequestID: new("req-1"), Status: 403}); err != nil {
		t.Fatal(err)
	}
	var page []adminapi.Entry
	if err := d.store.Tx(ctx, operator, "req-2", func(tx adminapi.Tx) (err error) {
		page, err = tx.AuditLog(ctx, adminapi.Filter{}, nil, 10)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(page) != 1 {
		t.Fatalf("%d lines", len(page))
	}
	e := page[0]
	if e.ID == "" || e.Operator != operator || e.OperationID != "list-audit-log" || e.Target != "/admin/audit-log" ||
		e.RequestID == nil || *e.RequestID != "req-1" || e.Status != 403 || e.Outcome != nil || e.At.Before(before.Add(-time.Second)) {
		t.Fatalf("line = %+v", e)
	}
}

func TestTheAuditLogIsFilteredAndPagedNewestFirst(t *testing.T) {
	d := setup(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i, who := range []string{"a", "b", "a", "a", "a"} {
		if _, err := d.migrator.Exec(ctx, `INSERT INTO control_plane.audit_log (operator, operation_id, target, status, at)
			VALUES ($1, $2, '/t', 200, $3)`, who, string(rune('0'+i)), base.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	read := func(f adminapi.Filter, after *adminapi.Key, limit int) (ops []string, page []adminapi.Entry) {
		t.Helper()
		if err := d.store.Tx(ctx, operator, "r", func(tx adminapi.Tx) (err error) {
			page, err = tx.AuditLog(ctx, f, after, limit)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		for _, e := range page {
			ops = append(ops, e.OperationID)
		}
		return ops, page
	}
	from, to := base.Add(2*time.Hour), base.Add(4*time.Hour)

	if ops, _ := read(adminapi.Filter{}, nil, 10); !slices.Equal(ops, []string{"4", "3", "2", "1", "0"}) {
		t.Errorf("all: %v", ops)
	}
	if ops, _ := read(adminapi.Filter{Operator: new("a"), From: &from, To: &to}, nil, 10); !slices.Equal(ops, []string{"3", "2"}) {
		t.Errorf("a in [2h, 4h): %v", ops)
	}
	_, first := read(adminapi.Filter{Operator: new("a")}, nil, 2)
	last := first[len(first)-1]
	if ops, _ := read(adminapi.Filter{Operator: new("a")}, &adminapi.Key{At: last.At, ID: last.ID}, 10); !slices.Equal(ops, []string{"2", "0"}) {
		t.Errorf("a after %s: %v", last.OperationID, ops)
	}
}

func TestTheChangesOfARequestArePagedOldestFirst(t *testing.T) {
	d := setup(t)
	ctx := context.Background()
	if err := d.store.Record(ctx, adminapi.Entry{Operator: operator, OperationID: "op", Target: "/t", RequestID: new("req-1"), Status: 200}); err != nil {
		t.Fatal(err)
	}
	exec(t, d.migrator, `BEGIN`, `SET LOCAL app.actor = 'service:test'`, `SET LOCAL app.request_id = 'req-1'`,
		`INSERT INTO control_plane.probe VALUES (1), (2), (3)`, `COMMIT`,
		`BEGIN`, `SET LOCAL app.actor = 'service:test'`, `SET LOCAL app.request_id = 'req-2'`,
		`INSERT INTO control_plane.probe VALUES (4)`, `COMMIT`)

	var page, rest []adminapi.Change
	var requested, unknown bool
	if err := d.store.Tx(ctx, operator, "r", func(tx adminapi.Tx) (err error) {
		if page, err = tx.Changes(ctx, "req-1", nil, 2); err != nil {
			return err
		}
		last := page[len(page)-1]
		if rest, err = tx.Changes(ctx, "req-1", &adminapi.Key{At: last.At, ID: last.ID}, 10); err != nil {
			return err
		}
		if requested, err = tx.Requested(ctx, "req-1"); err != nil {
			return err
		}
		unknown, err = tx.Requested(ctx, "req-2")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	pks := func(cs []adminapi.Change) (out []string) {
		for _, c := range cs {
			out = append(out, c.PK)
		}
		return out
	}
	if !slices.Equal(pks(page), []string{`{"id": 1}`, `{"id": 2}`}) || !slices.Equal(pks(rest), []string{`{"id": 3}`}) {
		t.Fatalf("pages %v then %v", pks(page), pks(rest))
	}
	c := page[0]
	if c.Actor != "service:test" || c.Table != "probe" || c.Op != "insert" || c.Before != nil || string(c.After) != `{"id": 1}` {
		t.Fatalf("change = %+v", c)
	}
	if !requested || unknown {
		t.Fatalf("requested req-1 %v, req-2 %v", requested, unknown)
	}
}
