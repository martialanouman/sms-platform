package http_test

import (
	"context"
	"net/netip"
	"testing"
)

func (a *api) holdOnly(t *testing.T, roles ...string) {
	t.Helper()
	ctx := context.Background()
	if _, err := a.owner.Exec(ctx, `DELETE FROM dashboard.operator_roles`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.owner.Exec(ctx, `INSERT INTO dashboard.operator_roles (operator_id, role_id)
		SELECT o.id, r.id FROM dashboard.operators o, dashboard.roles r WHERE r.name = ANY($1)`, roles); err != nil {
		t.Fatal(err)
	}
}

func TestASessionNotElevatedReachesOnlyAuthWhenItsRolesRequireMFA(t *testing.T) {
	a := newAPI(t, netip.MustParsePrefix("127.0.0.0/8"))
	a.activate(t)
	s := a.signIn(t)
	for _, tc := range []struct {
		roles        []string
		status       int
		code, reason string
	}{
		{[]string{"Audit"}, 403, "mfa_required", "audit:read is sensitive"},
		{[]string{"Propriétaire"}, 403, "mfa_required", "the owner writes"},
		{[]string{"Reporting", "Clientèle"}, 403, "mfa_required", "a write among ordinary reads"},
		{[]string{"Reporting"}, 403, "forbidden_scope", "ordinary reads are not confined, and lack audit:read"},
		{nil, 403, "forbidden_scope", "no role"},
	} {
		a.holdOnly(t, tc.roles...)
		if got := a.do(t, call{method: "GET", path: "/api/access-log", cookie: s.cookie}); got.status != tc.status || got.code() != tc.code {
			t.Errorf("%v (%s): access log %d %s", tc.roles, tc.reason, got.status, got.raw)
		}
		if got := a.do(t, call{method: "GET", path: "/api/auth/me", cookie: s.cookie}); got.status != 200 || got.body["permissions"] == nil {
			t.Errorf("%v: me %d %s", tc.roles, got.status, got.raw)
		}
	}

	a.holdOnly(t, "Audit")
	enrolled := a.do(t, call{method: "POST", path: "/api/auth/mfa/totp/enroll", cookie: s.cookie, body: `{}`})
	if enrolled.status != 200 {
		t.Fatalf("an Audit session not elevated may enrol under /auth: %d %s", enrolled.status, enrolled.raw)
	}
	verified := a.verify(t, s, "totp", totpAt(t, enrolled.body["secret"].(string), 0))
	if verified.status != 204 {
		t.Fatalf("verify: %d %s", verified.status, verified.raw)
	}
	if got := a.do(t, call{method: "GET", path: "/api/access-log", cookie: verified.cookies[0].Value}); got.status != 200 {
		t.Errorf("access log once elevated: %d %s", got.status, got.raw)
	}
}
