package dashboard_test

import (
	"context"
	"slices"
	"testing"

	"github.com/martialanouman/sms-platform/internal/dashboard"
)

func (h *harness) holds(t *testing.T, email string) []string {
	t.Helper()
	var names []string
	if err := h.owner.QueryRow(context.Background(), `SELECT coalesce(array_agg(r.name ORDER BY r.name), '{}')
		FROM dashboard.operator_roles x JOIN dashboard.roles r ON r.id = x.role_id JOIN dashboard.operators o ON o.id = x.operator_id
		WHERE lower(o.email) = lower($1)`, email).Scan(&names); err != nil {
		t.Fatal(err)
	}
	return names
}

func (h *harness) grant(t *testing.T, email string, roles ...string) {
	t.Helper()
	h.exec(t, `INSERT INTO dashboard.operator_roles (operator_id, role_id)
		SELECT o.id, r.id FROM dashboard.operators o, dashboard.roles r WHERE lower(o.email) = lower($1) AND r.name = ANY($2)`, email, roles)
}

func rolesChanged(log []dashboard.Entry) []dashboard.Entry {
	return slices.DeleteFunc(slices.Clone(log), func(e dashboard.Entry) bool { return e.Event != dashboard.EventRolesChanged })
}

func TestBootstrapSeedsTheDefaultRolesAndRestoresThemOnEveryStart(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.svc.Bootstrap(ctx, "owner@example.test"); err != nil {
		t.Fatal(err)
	}
	var support string
	if err := h.owner.QueryRow(ctx, `SELECT id::text FROM dashboard.roles WHERE name = 'Support'`).Scan(&support); err != nil {
		t.Fatal(err)
	}
	h.exec(t, `UPDATE dashboard.roles SET description = 'x', permissions = '{audit:read}', is_default = false WHERE name = 'Support'`)
	if err := h.svc.Bootstrap(ctx, "owner@example.test"); err != nil {
		t.Fatal(err)
	}
	type row struct {
		Name, Description string
		Permissions       []string
		Default           bool
	}
	rows, err := h.owner.Query(ctx, `SELECT name, description, permissions, is_default FROM dashboard.roles`)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]row{}
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.Name, &r.Description, &r.Permissions, &r.Default); err != nil {
			t.Fatal(err)
		}
		got[r.Name] = r
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := dashboard.DefaultRoles()
	if len(got) != len(want) {
		t.Fatalf("%d roles stored, want %d", len(got), len(want))
	}
	for _, w := range want {
		r := got[w.Name]
		if r.Description != w.Description || !slices.Equal(r.Permissions, w.Permissions) || !r.Default {
			t.Errorf("%s stored as %+v, want %+v, default", w.Name, r, w)
		}
	}
	var still string
	if err := h.owner.QueryRow(ctx, `SELECT id::text FROM dashboard.roles WHERE name = 'Support'`).Scan(&still); err != nil || still != support {
		t.Fatalf("Support is %s after the reseed, was %s: %v", still, support, err)
	}
}

func TestBootstrapGivesTheOwnerRoleToItsOperatorWhenNoActiveOperatorHoldsIt(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	bootstrap := func(email string) {
		t.Helper()
		if err := h.svc.Bootstrap(ctx, email); err != nil {
			t.Fatal(err)
		}
	}
	bootstrap("owner@example.test")
	id := h.operatorID(t, "owner@example.test")
	if got := h.holds(t, "owner@example.test"); !slices.Equal(got, []string{dashboard.Owner}) {
		t.Fatalf("the new operator holds %v", got)
	}
	if got := rolesChanged(h.log(t)); len(got) != 1 || got[0].Target != id || got[0].Actor != "" || !got[0].Success {
		t.Fatalf("roles_changed %+v, want one targeting %s", got, id)
	}

	bootstrap("owner@example.test")
	bootstrap("other@example.test")
	if got := h.holds(t, "other@example.test"); len(got) != 0 {
		t.Fatalf("an owner is active, yet the other operator holds %v", got)
	}
	if got := rolesChanged(h.log(t)); len(got) != 1 {
		t.Fatalf("%d roles_changed while an owner is active", len(got))
	}

	h.exec(t, `DELETE FROM dashboard.operator_roles`)
	bootstrap("owner@example.test")
	if got := h.holds(t, "owner@example.test"); !slices.Equal(got, []string{dashboard.Owner}) {
		t.Fatalf("the existing operator without role holds %v", got)
	}

	h.exec(t, `UPDATE dashboard.operators SET status = 'disabled' WHERE lower(email) = 'owner@example.test'`)
	bootstrap("other@example.test")
	if got := h.holds(t, "other@example.test"); !slices.Equal(got, []string{dashboard.Owner}) {
		t.Fatalf("the only owner is disabled, yet the other operator holds %v", got)
	}
	if got := rolesChanged(h.log(t)); len(got) != 3 {
		t.Fatalf("%d roles_changed, want 3", len(got))
	}
}

func TestASessionCarriesTheUnionOfTheRolesOfItsOperator(t *testing.T) {
	h := newHarness(t)
	h.activate(t, "owner@example.test")
	h.exec(t, `DELETE FROM dashboard.operator_roles`)
	h.grant(t, "owner@example.test", "Audit", "Reporting", "Scripts")
	l := h.login(t, "owner@example.test")
	c, err := h.svc.Resolve(context.Background(), l.Cookie)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"audit:read", "billing:read", "routing-scripts:read", "routing-scripts:write", "stream:read", "traffic:read"}
	if !slices.Equal(c.Permissions, want) {
		t.Fatalf("permissions %v, want %v", c.Permissions, want)
	}
	h.exec(t, `DELETE FROM dashboard.operator_roles`)
	if c, err = h.svc.Resolve(context.Background(), l.Cookie); err != nil || len(c.Permissions) != 0 {
		t.Fatalf("permissions %v after the roles were withdrawn: %v", c.Permissions, err)
	}
}
