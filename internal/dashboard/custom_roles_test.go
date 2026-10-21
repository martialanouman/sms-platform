package dashboard_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/martialanouman/sms-platform/internal/dashboard"
)

func (h *harness) role(t *testing.T, name string) dashboard.RoleRecord {
	t.Helper()
	all, err := h.svc.Roles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(all, func(r dashboard.RoleRecord) bool { return r.Name == name })
	if i < 0 {
		t.Fatalf("%s absent", name)
	}
	return all[i]
}

func TestACustomRoleIsComposedFromTheGeneratedPermissionsUnderAFreeName(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	actor := h.actor(t)
	before := len(h.log(t))
	created, err := h.svc.CreateRole(ctx, actor, " Lecture CDR ", " Lit les CDR en clair. ", []string{"msisdn:reveal", "cdr:read", "msisdn:reveal"})
	if err != nil || created.Name != "Lecture CDR" || created.Description != "Lit les CDR en clair." || created.Default ||
		!slices.Equal(created.Permissions, []string{"cdr:read", "msisdn:reveal"}) || len(created.Holders) != 0 {
		t.Fatalf("created %+v, %v", created, err)
	}
	if listed := h.role(t, "Lecture CDR"); listed.ID != created.ID {
		t.Fatalf("listed %+v", listed)
	}
	if log := h.since(t, before); len(log) != 1 || log[0].Event != dashboard.EventRoleCreated || log[0].Actor != actor || log[0].Target != created.ID {
		t.Fatalf("logged %+v", log)
	}
	for _, name := range []string{"lecture cdr", "AUDIT", " Audit"} {
		if _, err := h.svc.CreateRole(ctx, actor, name, "", nil); !errors.Is(err, dashboard.ErrRoleNameTaken) {
			t.Errorf("%q: %v", name, err)
		}
	}
	var invalid dashboard.InvalidField
	if _, err := h.svc.CreateRole(ctx, actor, "Autre", "", []string{"cdr:read", "admin:write"}); !errors.As(err, &invalid) || invalid.Field != "permissions[1]" {
		t.Fatalf("an unknown permission: %v", err)
	}
	if _, err := h.svc.CreateRole(ctx, actor, " \t ", "", nil); !errors.As(err, &invalid) || invalid.Field != "name" {
		t.Fatalf("a blank name: %v", err)
	}
	all, err := h.svc.Roles(ctx)
	if err != nil || len(all) != 11 || !slices.IsSortedFunc(all, func(a, b dashboard.RoleRecord) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	}) {
		t.Fatalf("%d roles, %v; want eleven, by name", len(all), err)
	}
	if owner := h.role(t, dashboard.Owner); !owner.Default || !slices.Equal(owner.Holders, []string{"owner@example.test"}) {
		t.Fatalf("owner %+v", owner)
	}
}

func TestAnUpdateReplacesACustomRoleButNeverADefaultOne(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	actor := h.actor(t)
	created, err := h.svc.CreateRole(ctx, actor, "Lecture", "", []string{"cdr:read"})
	if err != nil {
		t.Fatal(err)
	}
	before := len(h.log(t))
	updated, err := h.svc.UpdateRole(ctx, actor, created.ID, " Lit tout. ", []string{"routes:read", "cdr:read"})
	if err != nil || updated.Description != "Lit tout." || !slices.Equal(updated.Permissions, []string{"cdr:read", "routes:read"}) || updated.Name != "Lecture" {
		t.Fatalf("updated %+v, %v", updated, err)
	}
	if _, err := h.svc.UpdateRole(ctx, actor, created.ID, "Lit tout.", []string{"cdr:read", "routes:read"}); err != nil {
		t.Fatal(err)
	}
	if updated, err = h.svc.UpdateRole(ctx, actor, created.ID, "Lit le routage et les CDR.", []string{"cdr:read", "routes:read"}); err != nil ||
		updated.Description != "Lit le routage et les CDR." {
		t.Fatalf("a new description: %+v, %v", updated, err)
	}
	log := h.since(t, before)
	if !slices.Equal(events(log), []dashboard.Event{dashboard.EventRoleUpdated, dashboard.EventRoleUpdated}) {
		t.Fatalf("logged %v", events(log))
	}
	for _, e := range log {
		if e.Actor != actor || e.Target != created.ID {
			t.Errorf("logged %+v", e)
		}
	}
	if _, err := h.svc.UpdateRole(ctx, actor, h.roleID(t, "Support"), "", nil); !errors.Is(err, dashboard.ErrRoleIsDefault) {
		t.Fatalf("a default role: %v", err)
	}
	if got := h.role(t, "Support"); len(got.Permissions) == 0 {
		t.Fatal("a refused update emptied Support")
	}
	for _, id := range []string{"x", "01a15c07-0000-7000-8000-000000000000"} {
		if _, err := h.svc.UpdateRole(ctx, actor, id, "", nil); !errors.Is(err, dashboard.ErrNotFound) {
			t.Errorf("%q: %v", id, err)
		}
	}
	var invalid dashboard.InvalidField
	if _, err := h.svc.UpdateRole(ctx, actor, created.ID, "", []string{"nope:read"}); !errors.As(err, &invalid) || invalid.Field != "permissions[0]" {
		t.Fatalf("an unknown permission: %v", err)
	}

	gestion, err := h.svc.CreateRole(ctx, actor, "Gestion", "", []string{"operators:write"})
	if err != nil {
		t.Fatal(err)
	}
	composition, err := h.svc.CreateRole(ctx, actor, "Composition", "", []string{"roles:write"})
	if err != nil {
		t.Fatal(err)
	}
	h.activate(t, "ada@example.test")
	ada := h.operatorID(t, "ada@example.test")
	h.grant(t, "ada@example.test", "Gestion", "Composition")
	for _, r := range []dashboard.RoleRecord{gestion, composition} {
		if _, err := h.svc.UpdateRole(ctx, ada, r.ID, "", []string{"cdr:read"}); !errors.Is(err, dashboard.ErrSelfLockout) {
			t.Errorf("ada empties %s: %v", r.Name, err)
		}
	}
	if _, err := h.svc.UpdateRole(ctx, actor, gestion.ID, "", nil); err != nil {
		t.Fatalf("the owner empties a role he does not need: %v", err)
	}
}

func TestOnlyACustomRoleNobodyHoldsIsDeleted(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	actor := h.actor(t)
	created, err := h.svc.CreateRole(ctx, actor, "Lecture", "", []string{"cdr:read"})
	if err != nil {
		t.Fatal(err)
	}
	for _, email := range []string{"zed@example.test", "ada@example.test"} {
		h.activate(t, email)
		h.grant(t, email, "Lecture")
	}
	h.exec(t, `UPDATE dashboard.operators SET status = 'disabled' WHERE email = 'zed@example.test'`)
	var held dashboard.RoleHeld
	if err := h.svc.DeleteRole(ctx, actor, created.ID); !errors.As(err, &held) || !slices.Equal(held.Holders, []string{"ada@example.test", "zed@example.test"}) {
		t.Fatalf("a held role: %v", err)
	}
	if got := h.role(t, "Lecture"); !slices.Equal(got.Holders, []string{"ada@example.test", "zed@example.test"}) {
		t.Fatalf("holders %v", got.Holders)
	}
	if err := h.svc.DeleteRole(ctx, actor, h.roleID(t, "Audit")); !errors.Is(err, dashboard.ErrRoleIsDefault) {
		t.Fatalf("a default role: %v", err)
	}
	h.exec(t, `DELETE FROM dashboard.operator_roles WHERE role_id = $1`, created.ID)
	before := len(h.log(t))
	if err := h.svc.DeleteRole(ctx, actor, created.ID); err != nil {
		t.Fatal(err)
	}
	if log := h.since(t, before); len(log) != 1 || log[0].Event != dashboard.EventRoleDeleted || log[0].Actor != actor || log[0].Target != created.ID {
		t.Fatalf("logged %+v", log)
	}
	if err := h.svc.DeleteRole(ctx, actor, created.ID); !errors.Is(err, dashboard.ErrNotFound) {
		t.Fatalf("deleted twice: %v", err)
	}
}
