package dashboard_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/martialanouman/sms-platform/internal/dashboard"
)

func (h *harness) actor(t *testing.T) string {
	t.Helper()
	h.activate(t, "owner@example.test")
	return h.operatorID(t, "owner@example.test")
}

func (h *harness) since(t *testing.T, n int) []dashboard.Entry {
	t.Helper()
	all := h.log(t)
	return all[:len(all)-n]
}

func (h *harness) find(t *testing.T, email string) dashboard.OperatorRecord {
	t.Helper()
	all, err := h.svc.Operators(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(all, func(o dashboard.OperatorRecord) bool { return o.Email == email })
	if i < 0 {
		t.Fatalf("%s absent from %+v", email, all)
	}
	return all[i]
}

func events(log []dashboard.Entry) []dashboard.Event {
	out := make([]dashboard.Event, len(log))
	for i, e := range log {
		out[len(log)-1-i] = e.Event
	}
	return out
}

func TestCreateOperatorQueuesItsActivationAndRefusesATakenEmail(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	actor := h.actor(t)
	before := len(h.log(t))
	created, err := h.svc.CreateOperator(ctx, actor, " Ada@Example.test ", "Ada")
	if err != nil {
		t.Fatal(err)
	}
	if created.Email != "ada@example.test" || created.DisplayName != "Ada" || !created.Active || len(created.Roles) != 0 ||
		created.SecondFactor || created.Link == nil || *created.Link != (dashboard.LinkState{Kind: dashboard.Activation, State: dashboard.LinkQueued}) {
		t.Fatalf("created %+v", created)
	}
	if got := h.find(t, "ada@example.test"); got.ID != created.ID {
		t.Fatalf("listed %+v, created %+v", got, created)
	}
	log := h.since(t, before)
	if !slices.Equal(events(log), []dashboard.Event{dashboard.EventOperatorCreated, dashboard.EventAccessLinkRequested}) {
		t.Fatalf("logged %v", events(log))
	}
	for _, e := range log {
		if e.Actor != actor || e.Target != created.ID || !e.Success {
			t.Errorf("logged %+v", e)
		}
	}
	if _, err := h.svc.CreateOperator(ctx, actor, "ADA@example.test", "Other"); !errors.Is(err, dashboard.ErrEmailTaken) {
		t.Fatalf("a taken email: %v", err)
	}

	var wg sync.WaitGroup
	results := make(chan error, 4)
	for range 4 {
		wg.Go(func() {
			_, err := h.svc.CreateOperator(ctx, actor, "race@example.test", "Race")
			results <- err
		})
	}
	wg.Wait()
	close(results)
	var ok, taken int
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, dashboard.ErrEmailTaken):
			taken++
		default:
			t.Errorf("concurrent creation: %v", err)
		}
	}
	if ok != 1 || taken != 3 {
		t.Fatalf("%d created, %d taken; want 1 and 3", ok, taken)
	}
}

func TestDisablingAnOperatorClosesItsSessionsAndReenablingKeepsItsRoles(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	actor := h.actor(t)
	h.activate(t, "ada@example.test")
	ada := h.operatorID(t, "ada@example.test")
	h.grant(t, "ada@example.test", "Support", "Audit")
	l := h.login(t, "ada@example.test")
	before := len(h.log(t))

	got, err := h.svc.SetOperatorStatus(ctx, actor, ada, false)
	if err != nil || got.Active {
		t.Fatalf("disable: %+v, %v", got, err)
	}
	if _, err := h.svc.Resolve(ctx, l.Cookie); !errors.Is(err, dashboard.ErrUnauthenticated) {
		t.Fatalf("a session of the disabled operator: %v", err)
	}
	var sessions int
	if err := h.owner.QueryRow(ctx, `SELECT count(*) FROM dashboard.sessions WHERE operator_id = $1`, ada).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("%d sessions left: %v", sessions, err)
	}
	if _, err := h.svc.SetOperatorStatus(ctx, actor, ada, false); err != nil {
		t.Fatal(err)
	}
	got, err = h.svc.SetOperatorStatus(ctx, actor, ada, true)
	if err != nil || !got.Active || len(got.Roles) != 2 || got.Roles[0].Name != "Audit" || got.Roles[1].Name != "Support" {
		t.Fatalf("enable: %+v, %v", got, err)
	}
	for _, r := range got.Roles {
		var name string
		if err := h.owner.QueryRow(ctx, `SELECT name FROM dashboard.roles WHERE id = $1`, r.ID).Scan(&name); err != nil || name != r.Name {
			t.Errorf("role %s named %q, listed %q: %v", r.ID, name, r.Name, err)
		}
	}
	log := h.since(t, before)
	if !slices.Equal(events(log), []dashboard.Event{dashboard.EventOperatorDisabled, dashboard.EventOperatorEnabled}) {
		t.Fatalf("logged %v", events(log))
	}
	for _, e := range log {
		if e.Actor != actor || e.Target != ada {
			t.Errorf("logged %+v", e)
		}
	}

	if _, err := h.svc.SetOperatorStatus(ctx, actor, actor, false); !errors.Is(err, dashboard.ErrSelfLockout) {
		t.Fatalf("disabling oneself: %v", err)
	}
	for _, id := range []string{"x", "01a15c07-0000-7000-8000-000000000000"} {
		if _, err := h.svc.SetOperatorStatus(ctx, actor, id, false); !errors.Is(err, dashboard.ErrNotFound) {
			t.Errorf("status of %q: %v", id, err)
		}
	}
}

func TestAnAccessLinkActivatesANewAccountAndResetsAnActiveOne(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	actor := h.actor(t)
	created, err := h.svc.CreateOperator(ctx, actor, "new@example.test", "New")
	if err != nil {
		t.Fatal(err)
	}
	before := len(h.log(t))
	for _, tc := range []struct {
		id   string
		kind dashboard.LinkKind
	}{{created.ID, dashboard.Activation}, {actor, dashboard.Reset}} {
		if err := h.svc.RequestAccessLink(ctx, actor, tc.id); err != nil {
			t.Fatal(err)
		}
		var kind string
		if err := h.owner.QueryRow(ctx, `SELECT kind FROM dashboard.access_links WHERE operator_id = $1 AND token_hash IS NULL AND sent_at IS NULL`, tc.id).Scan(&kind); err != nil || kind != string(tc.kind) {
			t.Fatalf("queued %q for %s, want %q: %v", kind, tc.id, tc.kind, err)
		}
	}
	log := h.since(t, before)
	if !slices.Equal(events(log), []dashboard.Event{dashboard.EventAccessLinkRequested, dashboard.EventAccessLinkRequested}) ||
		log[1].Actor != actor || log[1].Target != created.ID {
		t.Fatalf("logged %+v", log)
	}
	if _, err := h.svc.SetOperatorStatus(ctx, actor, created.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.RequestAccessLink(ctx, actor, created.ID); !errors.Is(err, dashboard.ErrOperatorDisabled) {
		t.Fatalf("a link to a disabled account: %v", err)
	}
	if err := h.svc.RequestAccessLink(ctx, actor, "x"); !errors.Is(err, dashboard.ErrNotFound) {
		t.Fatalf("a link to nobody: %v", err)
	}
}

func TestTheListTellsTheStateOfTheAccessLinkAndOfTheSecondFactor(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	actor := h.actor(t)
	created, err := h.svc.CreateOperator(ctx, actor, "ada@example.test", "Ada")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		sql  string
		want *dashboard.LinkState
	}{
		{`UPDATE dashboard.access_links SET attempts = 10`, &dashboard.LinkState{Kind: dashboard.Activation, State: dashboard.LinkFailed}},
		{`UPDATE dashboard.access_links SET attempts = 9`, &dashboard.LinkState{Kind: dashboard.Activation, State: dashboard.LinkQueued}},
		{`UPDATE dashboard.access_links SET sent_at = now(), token_hash = '\x01', expires_at = now() + interval '1 hour'`,
			&dashboard.LinkState{Kind: dashboard.Activation, State: dashboard.LinkSent}},
		{`UPDATE dashboard.access_links SET expires_at = now() - interval '1 second'`, nil},
		{`UPDATE dashboard.access_links SET token_hash = NULL, expires_at = now() + interval '1 hour'`, nil},
	} {
		h.exec(t, tc.sql+` WHERE operator_id = $1`, created.ID)
		got := h.find(t, "ada@example.test").Link
		if (got == nil) != (tc.want == nil) || got != nil && *got != *tc.want {
			t.Errorf("after %s: %+v, want %+v", tc.sql, got, tc.want)
		}
	}
	if owner := h.find(t, "owner@example.test"); owner.Link != nil || owner.SecondFactor ||
		!slices.Equal(owner.Roles, []dashboard.RoleRef{{ID: owner.Roles[0].ID, Name: dashboard.Owner}}) {
		t.Fatalf("owner %+v", owner)
	}
	l := h.login(t, "owner@example.test")
	if _, err := h.svc.EnrollTotp(ctx, l.Cookie, nil); err != nil {
		t.Fatal(err)
	}
	if !h.find(t, "owner@example.test").SecondFactor {
		t.Fatal("a pending authenticator app is a second factor a reset would remove")
	}
}

func (h *harness) roleID(t *testing.T, name string) string {
	t.Helper()
	var id string
	if err := h.owner.QueryRow(context.Background(), `SELECT id::text FROM dashboard.roles WHERE name = $1`, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func names(roles []dashboard.RoleRef) []string {
	out := make([]string, len(roles))
	for i, r := range roles {
		out[i] = r.Name
	}
	return out
}

func TestSetOperatorRolesReplacesThemAndLogsOnlyAChange(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	actor := h.actor(t)
	h.activate(t, "ada@example.test")
	ada := h.operatorID(t, "ada@example.test")
	support, audit := h.roleID(t, "Support"), h.roleID(t, "Audit")
	before := len(h.log(t))

	got, err := h.svc.SetOperatorRoles(ctx, actor, ada, []string{support, audit, support})
	if err != nil || !slices.Equal(names(got.Roles), []string{"Audit", "Support"}) {
		t.Fatalf("set: %+v, %v", got, err)
	}
	if _, err := h.svc.SetOperatorRoles(ctx, actor, ada, []string{audit, support}); err != nil {
		t.Fatal(err)
	}
	if got, err = h.svc.SetOperatorRoles(ctx, actor, ada, []string{}); err != nil || len(got.Roles) != 0 {
		t.Fatalf("cleared: %+v, %v", got, err)
	}
	log := h.since(t, before)
	if !slices.Equal(events(log), []dashboard.Event{dashboard.EventRolesChanged, dashboard.EventRolesChanged}) {
		t.Fatalf("logged %v", events(log))
	}
	for _, e := range log {
		if e.Actor != actor || e.Target != ada {
			t.Errorf("logged %+v", e)
		}
	}

	for _, tc := range []struct {
		ids   []string
		field string
	}{
		{[]string{support, "01a15c07-0000-7000-8000-000000000000"}, "roleIds[1]"},
		{[]string{"x", support}, "roleIds[0]"},
	} {
		var invalid dashboard.InvalidField
		if _, err := h.svc.SetOperatorRoles(ctx, actor, ada, tc.ids); !errors.As(err, &invalid) || invalid.Field != tc.field {
			t.Errorf("%v: %v, want %s refused", tc.ids, err, tc.field)
		}
	}
	if got := h.find(t, "ada@example.test"); len(got.Roles) != 0 {
		t.Fatalf("a refused change left %v", names(got.Roles))
	}
	if _, err := h.svc.SetOperatorRoles(ctx, actor, "x", []string{support}); !errors.Is(err, dashboard.ErrNotFound) {
		t.Fatalf("roles of nobody: %v", err)
	}
}

func TestNobodyTakesFromHimselfTheRightToManageOperatorsOrRoles(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	actor := h.actor(t)
	owner, support, audit := h.roleID(t, dashboard.Owner), h.roleID(t, "Support"), h.roleID(t, "Audit")
	if _, err := h.svc.SetOperatorRoles(ctx, actor, actor, []string{support}); !errors.Is(err, dashboard.ErrSelfLockout) {
		t.Fatalf("the owner drops his own role: %v", err)
	}
	if got := h.find(t, "owner@example.test"); !slices.Equal(names(got.Roles), []string{dashboard.Owner}) {
		t.Fatalf("a refused lockout left %v", names(got.Roles))
	}
	if _, err := h.svc.SetOperatorRoles(ctx, actor, actor, []string{owner, support}); err != nil {
		t.Fatalf("keeping the owner role: %v", err)
	}

	h.exec(t, `INSERT INTO dashboard.roles (name, description, permissions) VALUES ('Gestion', '', '{operators:read,operators:write}')`)
	gestion := h.roleID(t, "Gestion")
	h.activate(t, "ada@example.test")
	ada := h.operatorID(t, "ada@example.test")
	h.grant(t, "ada@example.test", "Gestion")
	if _, err := h.svc.SetOperatorRoles(ctx, ada, ada, []string{gestion, audit}); err != nil {
		t.Fatalf("roles:write was never held, so never lost: %v", err)
	}
	if _, err := h.svc.SetOperatorRoles(ctx, ada, ada, []string{audit}); !errors.Is(err, dashboard.ErrSelfLockout) {
		t.Fatalf("ada drops operators:write: %v", err)
	}
	h.exec(t, `INSERT INTO dashboard.roles (name, description, permissions) VALUES ('Composition', '', '{roles:write}')`)
	composition := h.roleID(t, "Composition")
	if _, err := h.svc.SetOperatorRoles(ctx, ada, ada, []string{gestion, composition}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.SetOperatorRoles(ctx, ada, ada, []string{gestion}); !errors.Is(err, dashboard.ErrSelfLockout) {
		t.Fatalf("ada drops roles:write: %v", err)
	}
	if _, err := h.svc.SetOperatorRoles(ctx, ada, actor, []string{support}); err != nil {
		t.Fatalf("taking from another is no lockout: %v", err)
	}
}
