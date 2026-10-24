package http_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"
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

func TestASessionNotElevatedReachesOnlyAuthWhateverItsRoles(t *testing.T) {
	a := newAPI(t, netip.MustParsePrefix("127.0.0.0/8"))
	a.activate(t)
	s := a.signIn(t)
	for _, roles := range [][]string{{"Reporting"}, nil} {
		a.holdOnly(t, roles...)
		if got := a.do(t, call{method: "GET", path: "/api/access-log", cookie: s.cookie}); got.status != 403 || got.code() != "mfa_required" {
			t.Errorf("%v: access log %d %s", roles, got.status, got.raw)
		}
		if got := a.do(t, call{method: "GET", path: "/api/gw/admin/customers", cookie: s.cookie}); got.status != 403 || got.code() != "mfa_required" {
			t.Errorf("%v: proxy %d %s", roles, got.status, got.raw)
		}
		if got := a.do(t, call{method: "GET", path: "/api/auth/me", cookie: s.cookie}); got.status != 200 || got.body["permissions"] == nil {
			t.Errorf("%v: me %d %s", roles, got.status, got.raw)
		}
	}

	a.holdOnly(t, "Reporting")
	me := a.do(t, call{method: "GET", path: "/api/auth/me", cookie: s.cookie})
	if got, _ := me.body["permissions"].([]any); fmt.Sprint(got) != "[billing:read stream:read traffic:read]" {
		t.Errorf("me of a Reporting session: %s", me.raw)
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
	a.holdOnly(t)
	if got := a.do(t, call{method: "GET", path: "/api/access-log", cookie: verified.cookies[0].Value}); got.status != 403 || got.code() != "forbidden_scope" || !strings.Contains(got.raw, "audit:read") {
		t.Errorf("an elevated session without a role meets its scope: %d %s", got.status, got.raw)
	}
}

func TestTheAuditDirectoryNamesEveryOperatorAndNothingMore(t *testing.T) {
	a := newAPI(t, netip.MustParsePrefix("127.0.0.0/8"))
	a.activate(t)
	c := a.elevated(t)
	if got := a.do(t, call{method: "POST", path: "/api/operators", cookie: c, body: `{"email":"ada@example.test","displayName":"Ada"}`}); got.status != 201 {
		t.Fatalf("create: %d %s", got.status, got.raw)
	}
	if _, err := a.owner.Exec(context.Background(), `UPDATE dashboard.operators SET status = 'disabled' WHERE email = 'ada@example.test'`); err != nil {
		t.Fatal(err)
	}

	a.holdOnly(t, "Audit")
	got := a.do(t, call{method: "GET", path: "/api/audit/operators", cookie: c})
	if got.status != 200 {
		t.Fatalf("an Audit session: %d %s", got.status, got.raw)
	}
	var listed []map[string]any
	if err := json.Unmarshal([]byte(got.raw), &listed); err != nil {
		t.Fatal(err)
	}
	status := map[string]any{}
	for _, o := range listed {
		status[fmt.Sprint(o["email"], " ", o["displayName"])] = o["status"]
	}
	if fmt.Sprint(status) != "map[ada@example.test Ada:disabled owner@example.test owner@example.test:active]" {
		t.Errorf("directory %s", got.raw)
	}
	for _, leak := range []string{"roles", "accessLink", "secondFactorEnrolled", "activated"} {
		if strings.Contains(got.raw, leak) {
			t.Errorf("directory %s leaks %s", got.raw, leak)
		}
	}

	a.holdOnly(t, "Reporting")
	if got := a.do(t, call{method: "GET", path: "/api/audit/operators", cookie: c}); got.status != 403 || got.code() != "forbidden_scope" {
		t.Errorf("a Reporting session: %d %s", got.status, got.raw)
	}
}
