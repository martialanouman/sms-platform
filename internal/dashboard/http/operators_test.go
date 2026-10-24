package http_test

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func (a *api) elevated(t *testing.T) string {
	t.Helper()
	s := a.signIn(t)
	enrolled := a.do(t, call{method: "POST", path: "/api/auth/mfa/totp/enroll", cookie: s.cookie, body: `{}`})
	verified := a.verify(t, s, "totp", totpAt(t, enrolled.body["secret"].(string), 0))
	if verified.status != 204 {
		t.Fatalf("verify: %d %s", verified.status, verified.raw)
	}
	return verified.cookies[0].Value
}

func (a *api) roleID(t *testing.T, name string) string {
	t.Helper()
	var id string
	if err := a.owner.QueryRow(context.Background(), `SELECT id::text FROM dashboard.roles WHERE name = $1`, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (a answer) field() string {
	errs, _ := a.body["errors"].([]any)
	if len(errs) != 1 {
		return ""
	}
	f, _ := errs[0].(map[string]any)["field"].(string)
	return f
}

func TestAnElevatedOwnerManagesOperators(t *testing.T) {
	a := newAPI(t, netip.MustParsePrefix("127.0.0.0/8"))
	a.activate(t)
	c := a.elevated(t)
	created := a.do(t, call{method: "POST", path: "/api/operators", cookie: c, body: `{"email":"Ada@example.test","displayName":"Ada"}`})
	link, _ := created.body["accessLink"].(map[string]any)
	if created.status != 201 || created.body["email"] != "ada@example.test" || created.body["displayName"] != "Ada" || created.body["status"] != "active" ||
		fmt.Sprint(created.body["roles"]) != "[]" || created.body["secondFactorEnrolled"] != false || created.body["activated"] != false || link["kind"] != "activation" || link["state"] != "queued" {
		t.Fatalf("create: %d %s", created.status, created.raw)
	}
	id := created.body["id"].(string)
	path := "/api/operators/" + id
	support := a.roleID(t, "Support")
	var owner string
	if err := a.owner.QueryRow(context.Background(), `SELECT id::text FROM dashboard.operators WHERE email = 'owner@example.test'`).Scan(&owner); err != nil {
		t.Fatal(err)
	}

	for name, tc := range map[string]struct {
		c           call
		status      int
		code, field string
	}{
		"taken email":      {call{method: "POST", path: "/api/operators", body: `{"email":"ADA@example.test","displayName":"X"}`}, 409, "email_taken", ""},
		"no email":         {call{method: "POST", path: "/api/operators", body: `{"email":"ada","displayName":"X"}`}, 422, "validation_error", "email"},
		"unknown role":     {call{method: "PUT", path: path + "/roles", body: `{"roleIds":["` + support + `","x"]}`}, 422, "validation_error", "roleIds[1]"},
		"own lockout":      {call{method: "PUT", path: "/api/operators/" + owner + "/roles", body: `{"roleIds":["` + support + `"]}`}, 409, "self_lockout", ""},
		"roles of nobody":  {call{method: "PUT", path: "/api/operators/x/roles", body: `{"roleIds":[]}`}, 404, "not_found", ""},
		"disabling self":   {call{method: "PATCH", path: "/api/operators/" + owner, body: `{"status":"disabled"}`}, 409, "self_lockout", ""},
		"status of nobody": {call{method: "PATCH", path: "/api/operators/x", body: `{"status":"disabled"}`}, 404, "not_found", ""},
		"link to nobody":   {call{method: "POST", path: "/api/operators/x/access-link"}, 404, "not_found", ""},
	} {
		tc.c.cookie = c
		if got := a.do(t, tc.c); got.status != tc.status || got.code() != tc.code || got.field() != tc.field || got.body["message"] == "" {
			t.Errorf("%s: %d %s", name, got.status, got.raw)
		}
	}

	roles := a.do(t, call{method: "PUT", path: path + "/roles", cookie: c, body: `{"roleIds":["` + support + `"]}`})
	if roles.status != 200 || fmt.Sprint(roles.body["roles"]) != fmt.Sprintf("[map[id:%s name:Support]]", support) {
		t.Fatalf("roles: %d %s", roles.status, roles.raw)
	}
	disabled := a.do(t, call{method: "PATCH", path: path, cookie: c, body: `{"status":"disabled"}`})
	if disabled.status != 200 || disabled.body["status"] != "disabled" {
		t.Fatalf("disable: %d %s", disabled.status, disabled.raw)
	}
	if got := a.do(t, call{method: "POST", path: path + "/access-link", cookie: c}); got.status != 409 || got.code() != "operator_disabled" {
		t.Fatalf("link to a disabled account: %d %s", got.status, got.raw)
	}
	if got := a.do(t, call{method: "PATCH", path: path, cookie: c, body: `{"status":"active"}`}); got.status != 200 || got.body["status"] != "active" {
		t.Fatalf("enable: %d %s", got.status, got.raw)
	}
	if got := a.do(t, call{method: "POST", path: path + "/access-link", cookie: c}); got.status != 202 {
		t.Fatalf("link: %d %s", got.status, got.raw)
	}
	list := a.do(t, call{method: "GET", path: "/api/operators", cookie: c})
	if list.status != 200 || !strings.Contains(list.raw, `"email":"ada@example.test"`) || !strings.Contains(list.raw, `"email":"owner@example.test"`) ||
		strings.Count(list.raw, `"secondFactorEnrolled":true`) != 1 || strings.Count(list.raw, `"activated":true`) != 1 || !strings.Contains(list.raw, `"accessLink":null`) {
		t.Fatalf("list: %d %s", list.status, list.raw)
	}
}

func TestAnElevatedOwnerComposesRoles(t *testing.T) {
	a := newAPI(t, netip.MustParsePrefix("127.0.0.0/8"))
	a.activate(t)
	c := a.elevated(t)
	list := a.do(t, call{method: "GET", path: "/api/roles", cookie: c})
	if list.status != 200 || strings.Count(list.raw, `"isDefault":true`) != 10 || !strings.Contains(list.raw, `"holders":["owner@example.test"]`) {
		t.Fatalf("list: %d %s", list.status, list.raw)
	}
	created := a.do(t, call{method: "POST", path: "/api/roles", cookie: c, body: `{"name":"Lecture","description":"Lit.","permissions":["routes:read","cdr:read"]}`})
	if created.status != 201 || created.body["name"] != "Lecture" || created.body["description"] != "Lit." || created.body["isDefault"] != false ||
		fmt.Sprint(created.body["permissions"]) != "[cdr:read routes:read]" || fmt.Sprint(created.body["holders"]) != "[]" {
		t.Fatalf("create: %d %s", created.status, created.raw)
	}
	path := "/api/roles/" + created.body["id"].(string)
	audit := "/api/roles/" + a.roleID(t, "Audit")
	if _, err := a.owner.Exec(context.Background(), `INSERT INTO dashboard.operator_roles (operator_id, role_id)
		SELECT o.id, r.id FROM dashboard.operators o, dashboard.roles r WHERE r.name = 'Lecture'`); err != nil {
		t.Fatal(err)
	}

	for name, tc := range map[string]struct {
		c           call
		status      int
		code, field string
	}{
		"taken name":     {call{method: "POST", path: "/api/roles", body: `{"name":"lecture","description":"","permissions":[]}`}, 409, "role_name_taken", ""},
		"blank name":     {call{method: "POST", path: "/api/roles", body: `{"name":"  ","description":"","permissions":[]}`}, 422, "validation_error", "name"},
		"unknown scope":  {call{method: "POST", path: "/api/roles", body: `{"name":"X","description":"","permissions":["cdr:read","admin:write"]}`}, 422, "validation_error", "permissions[1]"},
		"default update": {call{method: "PATCH", path: audit, body: `{"description":"","permissions":[]}`}, 409, "role_is_default", ""},
		"bad update":     {call{method: "PATCH", path: path, body: `{"description":"","permissions":["nope:read"]}`}, 422, "validation_error", "permissions[0]"},
		"update nobody":  {call{method: "PATCH", path: "/api/roles/x", body: `{"description":"","permissions":[]}`}, 404, "not_found", ""},
		"default delete": {call{method: "DELETE", path: audit}, 409, "role_is_default", ""},
		"held delete":    {call{method: "DELETE", path: path}, 409, "role_held", ""},
		"delete nobody":  {call{method: "DELETE", path: "/api/roles/x"}, 404, "not_found", ""},
	} {
		tc.c.cookie = c
		got := a.do(t, tc.c)
		if got.status != tc.status || got.code() != tc.code || got.field() != tc.field || got.body["message"] == "" {
			t.Errorf("%s: %d %s", name, got.status, got.raw)
		}
		if tc.code == "role_held" && !strings.Contains(got.raw, "owner@example.test") {
			t.Errorf("role_held names no holder: %s", got.raw)
		}
	}

	updated := a.do(t, call{method: "PATCH", path: path, cookie: c, body: `{"description":"Lit le routage.","permissions":["routes:read"]}`})
	if updated.status != 200 || updated.body["description"] != "Lit le routage." || fmt.Sprint(updated.body["permissions"]) != "[routes:read]" ||
		fmt.Sprint(updated.body["holders"]) != "[owner@example.test]" {
		t.Fatalf("update: %d %s", updated.status, updated.raw)
	}

	a.holdOnly(t, "Lecture")
	if _, err := a.owner.Exec(context.Background(), `UPDATE dashboard.roles SET permissions = '{roles:write}' WHERE name = 'Lecture'`); err != nil {
		t.Fatal(err)
	}
	if got := a.do(t, call{method: "PATCH", path: path, cookie: c, body: `{"description":"","permissions":[]}`}); got.status != 409 || got.code() != "self_lockout" {
		t.Errorf("own lockout: %d %s", got.status, got.raw)
	}
	a.holdOnly(t, "Propriétaire")
	if got := a.do(t, call{method: "DELETE", path: path, cookie: c}); got.status != 204 {
		t.Fatalf("delete: %d %s", got.status, got.raw)
	}
}

func TestAnAuthorStrippedWhileHisRequestWaitsIsRefused(t *testing.T) {
	a := newAPI(t, netip.MustParsePrefix("127.0.0.0/8"))
	a.activate(t)
	c := a.elevated(t)
	ctx := context.Background()
	created := a.do(t, call{method: "POST", path: "/api/roles", cookie: c, body: `{"name":"Lecture","description":"","permissions":[]}`})
	role := "/api/roles/" + created.body["id"].(string)
	operator := "/api/operators/" + a.do(t, call{method: "POST", path: "/api/operators", cookie: c, body: `{"email":"ada@example.test","displayName":"Ada"}`}).body["id"].(string)
	for name, req := range map[string]call{
		"status":      {method: "PATCH", path: operator, body: `{"status":"disabled"}`},
		"roles":       {method: "PUT", path: operator + "/roles", body: `{"roleIds":[]}`},
		"create role": {method: "POST", path: "/api/roles", body: `{"name":"X","description":"","permissions":[]}`},
		"update role": {method: "PATCH", path: role, body: `{"description":"x","permissions":[]}`},
		"delete role": {method: "DELETE", path: role},
		"access link": {method: "POST", path: operator + "/access-link"},
	} {
		req.cookie = c
		a.holdOnly(t, "Propriétaire")
		other, err := a.owner.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, sql := range []string{`SELECT pg_advisory_xact_lock(hashtext('dashboard.roles'))`,
			`DELETE FROM dashboard.operator_roles WHERE operator_id IN (SELECT id FROM dashboard.operators WHERE email = 'owner@example.test')`} {
			if _, err := other.Exec(ctx, sql); err != nil {
				t.Fatal(err)
			}
		}
		done := make(chan answer, 1)
		go func() { done <- a.do(t, req) }()
		select {
		case got := <-done:
			t.Fatalf("%s did not wait for the concurrent write: %d %s", name, got.status, got.raw)
		case <-time.After(300 * time.Millisecond):
		}
		if err := other.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if got := <-done; got.status != 403 || got.code() != "forbidden_scope" || got.body["message"] == "" {
			t.Errorf("%s by an author stripped meanwhile: %d %s", name, got.status, got.raw)
		}
	}
}
