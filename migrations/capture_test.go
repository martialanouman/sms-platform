package migrations_test

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/martialanouman/sms-platform/internal/platform/pgtest"
)

func exec(t *testing.T, conn *pgx.Conn, statements ...string) {
	t.Helper()
	for _, s := range statements {
		if _, err := conn.Exec(context.Background(), s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func probe(t *testing.T) (pg pgtest.DB, writer *pgx.Conn) {
	pg = pgtest.ControlPlane(t)
	exec(t, connect(t, pg.As("migrator")),
		`CREATE TABLE control_plane.probe (id int PRIMARY KEY, name text UNIQUE,
			secret_sealed bytea, token_hash text, key_kms_key_ref text, wrapped_key bytea)`,
		`CREATE TRIGGER audit_capture AFTER INSERT OR UPDATE OR DELETE ON control_plane.probe
			FOR EACH ROW EXECUTE FUNCTION control_plane.audit_capture()`,
	)
	exec(t, connect(t, pg.As("postgres")),
		`CREATE ROLE writer LOGIN PASSWORD 'writer'`,
		`GRANT USAGE ON SCHEMA control_plane TO writer`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON control_plane.probe TO writer`,
	)
	return pg, connect(t, pg.As("writer"))
}

func TestAWriteWithoutActorFails(t *testing.T) {
	pg, _ := probe(t)
	ctx := context.Background()
	for name, setActor := range map[string]string{
		"unset": `SELECT 1`,
		"empty": `SET LOCAL app.actor = ''`,
	} {
		t.Run(name, func(t *testing.T) {
			tx, err := connect(t, pg.As("writer")).Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			if _, err := tx.Exec(ctx, setActor); err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(ctx, `INSERT INTO control_plane.probe (id) VALUES (1)`)
			if sqlState(err) != "22023" {
				t.Fatalf("err = %v, want 22023", err)
			}
		})
	}
}

type change struct {
	RequestID *string
	Actor     string
	Op        string
	PK        map[string]any
	Before    map[string]any
	After     map[string]any
}

func TestAnAttributedWriteIsCapturedWithoutSealedColumns(t *testing.T) {
	pg, writer := probe(t)
	ctx := context.Background()
	exec(t, writer,
		`BEGIN`,
		`SET LOCAL app.actor = 'sub-1'`,
		`SET LOCAL app.request_id = 'req-1'`,
		`INSERT INTO control_plane.probe VALUES (1, 'a', '\x01', 'h', 'k', '\x02')`,
		`UPDATE control_plane.probe SET name = 'b', secret_sealed = '\x03' WHERE id = 1`,
		`COMMIT`,
		`BEGIN`,
		`SET LOCAL app.actor = 'breakglass:ops'`,
		`DELETE FROM control_plane.probe WHERE id = 1`,
		`COMMIT`,
	)
	rows, err := connect(t, pg.As("migrator")).Query(ctx, `SELECT request_id, actor, op, pk, before, after
		FROM control_plane.audit_changes WHERE table_name = 'probe' ORDER BY at, id`)
	if err != nil {
		t.Fatal(err)
	}
	var got []change
	for rows.Next() {
		var c change
		if err := rows.Scan(&c.RequestID, &c.Actor, &c.Op, &c.PK, &c.Before, &c.After); err != nil {
			t.Fatal(err)
		}
		got = append(got, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	req := "req-1"
	pk := map[string]any{"id": 1.0}
	a := map[string]any{"id": 1.0, "name": "a"}
	b := map[string]any{"id": 1.0, "name": "b"}
	want := []change{
		{RequestID: &req, Actor: "sub-1", Op: "insert", PK: pk, After: a},
		{RequestID: &req, Actor: "sub-1", Op: "update", PK: pk, Before: a, After: b},
		{Actor: "breakglass:ops", Op: "delete", PK: pk, Before: b},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
}

func TestAnAttributedWriteNotifiesItsKeyAtCommit(t *testing.T) {
	pg, writer := probe(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	listener := connect(t, pg.As("migrator"))
	exec(t, listener, `LISTEN config`)
	exec(t, writer,
		`BEGIN`,
		`SET LOCAL app.actor = 'sub-1'`,
		`INSERT INTO control_plane.probe (id) VALUES (7)`,
		`COMMIT`,
	)
	n, err := listener.WaitForNotification(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n.Payload != `probe:{"id": 7}` {
		t.Fatalf("payload = %q", n.Payload)
	}
}

func TestAKeyChangeNotifiesTheOldKeyAndTheNewOne(t *testing.T) {
	pg, writer := probe(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	write := func(statement string) {
		exec(t, writer, `BEGIN`, `SET LOCAL app.actor = 'sub-1'`, statement, `COMMIT`)
	}
	write(`INSERT INTO control_plane.probe (id) VALUES (7)`)
	listener := connect(t, pg.As("migrator"))
	exec(t, listener, `LISTEN config`)
	write(`UPDATE control_plane.probe SET id = 8 WHERE id = 7`)
	write(`UPDATE control_plane.probe SET name = 'n' WHERE id = 8`)
	write(`INSERT INTO control_plane.probe (id) VALUES (9)`)
	var got []string
	for range 4 {
		n, err := listener.WaitForNotification(ctx)
		if err != nil {
			t.Fatalf("after %v: %v", got, err)
		}
		got = append(got, n.Payload)
	}
	slices.Sort(got[:2])
	want := []string{`probe:{"id": 7}`, `probe:{"id": 8}`, `probe:{"id": 8}`, `probe:{"id": 9}`}
	if !slices.Equal(got, want) {
		t.Fatalf("payloads = %q, want %q", got, want)
	}
}

func TestTheCaptureIgnoresTheWritersSearchPath(t *testing.T) {
	pg, writer := probe(t)
	exec(t, connect(t, pg.As("postgres")), `GRANT CREATE ON DATABASE control_plane TO writer`)
	exec(t, writer,
		`CREATE SCHEMA hijack`,
		`GRANT USAGE ON SCHEMA hijack TO migrator`,
		`CREATE FUNCTION hijack.lower(text) RETURNS text LANGUAGE plpgsql AS $$
			BEGIN RAISE EXCEPTION 'hijacked'; END $$`,
		`SET search_path = hijack, pg_catalog`,
		`BEGIN`,
		`SET LOCAL app.actor = 'sub-1'`,
		`INSERT INTO control_plane.probe (id) VALUES (1)`,
		`COMMIT`,
	)
}

func TestNoServiceCanCaptureATableIntoTheJournal(t *testing.T) {
	_, writer := probe(t)
	exec(t, writer, `CREATE TEMP TABLE customers (id int PRIMARY KEY)`)
	_, err := writer.Exec(context.Background(), `CREATE TRIGGER audit_capture AFTER INSERT ON customers
		FOR EACH ROW EXECUTE FUNCTION control_plane.audit_capture()`)
	if sqlState(err) != "42501" {
		t.Fatalf("err = %v, want 42501", err)
	}
}
