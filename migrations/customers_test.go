package migrations_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/martialanouman/sms-platform/internal/platform/pgtest"
)

func TestCustomerTablesHoldTheirRules(t *testing.T) {
	conn := connect(t, pgtest.ControlPlane(t).As("migrator"))
	ctx := context.Background()
	exec(t, conn, `SET app.actor = 'test'`,
		`INSERT INTO control_plane.customer_groups (id, name) VALUES ('00000000-0000-7000-8000-000000000001', 'g')`,
		`INSERT INTO control_plane.customers (id, name, group_id) VALUES
			('00000000-0000-7000-8000-000000000011', 'c', '00000000-0000-7000-8000-000000000001'),
			('00000000-0000-7000-8000-000000000012', 'closed', NULL)`,
		`INSERT INTO control_plane.accounts (id, customer_id, name) VALUES
			('00000000-0000-7000-8000-000000000021', '00000000-0000-7000-8000-000000000011', 'a')`,
		`UPDATE control_plane.customers SET status = 'closed' WHERE name = 'closed'`,
	)
	const customer = `'00000000-0000-7000-8000-000000000011'`
	for _, c := range []struct {
		name, statement, state, constraint string
	}{
		{"group name taken", `INSERT INTO control_plane.customer_groups (name) VALUES ('g')`, "23505", "customer_groups_name_key"},
		{"group status", `INSERT INTO control_plane.customer_groups (name, status) VALUES ('h', 'gone')`, "23514", "customer_groups_status_check"},
		{"customer status", `INSERT INTO control_plane.customers (name, status) VALUES ('x', 'gone')`, "23514", "customers_status_check"},
		{"unknown group", `INSERT INTO control_plane.customers (name, group_id) VALUES ('x', uuidv7())`, "23503", "customers_group_id_fkey"},
		{"otp age", `INSERT INTO control_plane.customers (name, max_age_otp) VALUES ('x', 601)`, "23514", "customers_max_age_otp_check"},
		{"otp age floor", `INSERT INTO control_plane.customers (name, max_age_otp) VALUES ('x', 0)`, "23514", "customers_max_age_otp_check"},
		{"transactional age", `INSERT INTO control_plane.customers (name, max_age_transactional) VALUES ('x', 86401)`, "23514", "customers_max_age_transactional_check"},
		{"transactional age floor", `INSERT INTO control_plane.customers (name, max_age_transactional) VALUES ('x', 0)`, "23514", "customers_max_age_transactional_check"},
		{"marketing age floor", `INSERT INTO control_plane.customers (name, max_age_marketing) VALUES ('x', 0)`, "23514", "customers_max_age_marketing_check"},
		{"marketing age", `INSERT INTO control_plane.customers (name, max_age_marketing) VALUES ('x', 259201)`, "23514", "customers_max_age_marketing_check"},
		{"closed customer reopened", `UPDATE control_plane.customers SET status = 'active' WHERE name = 'closed'`, "23514", "closed_is_final"},
		{"customer with accounts", `DELETE FROM control_plane.customers WHERE id = ` + customer, "23001", "accounts_customer_id_fkey"},
		{"unknown customer", `INSERT INTO control_plane.accounts (customer_id, name) VALUES (uuidv7(), 'b')`, "23503", "accounts_customer_id_fkey"},
		{"account name taken", `INSERT INTO control_plane.accounts (customer_id, name) VALUES (` + customer + `, 'a')`, "23505", "accounts_customer_id_name_key"},
		{"no channel", `INSERT INTO control_plane.accounts (customer_id, name, smpp_enabled, rest_enabled) VALUES (` + customer + `, 'b', false, false)`, "23514", "accounts_channel_check"},
		{"no bind type", `INSERT INTO control_plane.accounts (customer_id, name, allowed_bind_types) VALUES (` + customer + `, 'b', '{}')`, "23514", "accounts_allowed_bind_types_check"},
		{"unknown bind type", `INSERT INTO control_plane.accounts (customer_id, name, allowed_bind_types) VALUES (` + customer + `, 'b', '{trx}')`, "23514", "accounts_allowed_bind_types_check"},
		{"negative sessions", `INSERT INTO control_plane.accounts (customer_id, name, max_sessions) VALUES (` + customer + `, 'b', -1)`, "23514", "accounts_max_sessions_check"},
		{"account status", `INSERT INTO control_plane.accounts (customer_id, name, status) VALUES (` + customer + `, 'b', 'gone')`, "23514", "accounts_status_check"},
	} {
		t.Run(c.name, func(t *testing.T) {
			tx, err := conn.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			_, err = tx.Exec(ctx, c.statement)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != c.state || pgErr.ConstraintName != c.constraint {
				t.Fatalf("err = %v, want %s on %s", err, c.state, c.constraint)
			}
		})
	}

	t.Run("closed account reopened", func(t *testing.T) {
		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		_, err = tx.Exec(ctx, `UPDATE control_plane.accounts SET status = 'closed'`)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, `UPDATE control_plane.accounts SET status = 'suspended'`)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.ConstraintName != "closed_is_final" {
			t.Fatalf("err = %v, want 23514 on closed_is_final", err)
		}
	})

	t.Run("defaults", func(t *testing.T) {
		var smpp, rest, query, cancel bool
		var binds []string
		var sessions int
		var status string
		if err := conn.QueryRow(ctx, `SELECT smpp_enabled, rest_enabled, query_sm_enabled, cancel_sm_enabled,
			allowed_bind_types, max_sessions, status FROM control_plane.accounts`).Scan(
			&smpp, &rest, &query, &cancel, &binds, &sessions, &status); err != nil {
			t.Fatal(err)
		}
		if !smpp || !rest || !query || !cancel || len(binds) != 1 || binds[0] != "transceiver" || sessions != 1 || status != "active" {
			t.Fatalf("defaults = %v %v %v %v %v %d %s", smpp, rest, query, cancel, binds, sessions, status)
		}
	})

	t.Run("updated_at follows a write", func(t *testing.T) {
		for _, table := range []string{"customer_groups", "customers", "accounts"} {
			var before, after time.Time
			if err := conn.QueryRow(ctx, `SELECT updated_at FROM control_plane.`+table+` ORDER BY id LIMIT 1`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			exec(t, conn, `UPDATE control_plane.`+table+` SET name = name WHERE id = (SELECT id FROM control_plane.`+table+` ORDER BY id LIMIT 1)`)
			if err := conn.QueryRow(ctx, `SELECT updated_at FROM control_plane.`+table+` ORDER BY id LIMIT 1`).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if !after.After(before) {
				t.Errorf("%s: updated_at %v, then %v", table, before, after)
			}
		}
	})

	t.Run("deleting a group detaches its customers", func(t *testing.T) {
		exec(t, conn, `DELETE FROM control_plane.customer_groups`)
		var detached int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM control_plane.customers WHERE group_id IS NULL`).Scan(&detached); err != nil || detached != 2 {
			t.Fatalf("detached = %d (%v), want 2", detached, err)
		}
	})
}

func TestAGroupIsCreatedByTheOperatorOfItsActorAndByNoOneUnderBreakGlass(t *testing.T) {
	conn := connect(t, pgtest.ControlPlane(t).As("migrator"))
	const operator = "0199f0a2-7c1e-7000-8000-000000000001"
	exec(t, conn,
		`SET app.actor = '`+operator+`'`,
		`INSERT INTO control_plane.customer_groups (name) VALUES ('operator')`,
		`SET app.actor = 'breakglass:alice'`,
		`INSERT INTO control_plane.customer_groups (name) VALUES ('breakglass')`,
		`SET app.actor = 'service:x'`,
		`INSERT INTO control_plane.customer_groups (name) VALUES ('service')`,
	)
	rows, err := conn.Query(context.Background(), `SELECT name, coalesce(created_by::text, '') FROM control_plane.customer_groups`)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for rows.Next() {
		var name, by string
		if err := rows.Scan(&name, &by); err != nil {
			t.Fatal(err)
		}
		got[name] = by
	}
	if got["operator"] != operator || got["breakglass"] != "" || got["service"] != "" || len(got) != 3 {
		t.Fatalf("created_by = %v", got)
	}
}
