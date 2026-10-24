package postgres_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type accountBody struct {
	ID               string          `json:"id"`
	CustomerID       string          `json:"customer_id"`
	CustomerName     string          `json:"customer_name"`
	CustomerStatus   string          `json:"customer_status"`
	Name             string          `json:"name"`
	Status           string          `json:"status"`
	SMPPEnabled      *bool           `json:"smpp_enabled"`
	RESTEnabled      *bool           `json:"rest_enabled"`
	QuerySMEnabled   *bool           `json:"query_sm_enabled"`
	CancelSMEnabled  *bool           `json:"cancel_sm_enabled"`
	AllowedBindTypes []string        `json:"allowed_bind_types"`
	MaxSessions      *int            `json:"max_sessions"`
	RateLimit        *map[string]any `json:"rate_limit"`
	CreatedAt        *time.Time      `json:"created_at"`
	UpdatedAt        *time.Time      `json:"updated_at"`
}

type accountPage struct {
	Data       []accountBody `json:"data"`
	NextCursor *string       `json:"next_cursor"`
	Total      *int          `json:"total"`
}

func (g gateway) createAccount(t *testing.T, body string) accountBody {
	t.Helper()
	return as[accountBody](t, g.send(t, http.MethodPost, "/admin/accounts", "accounts:write", body), http.StatusCreated)
}

func flags(a accountBody) [4]bool {
	return [4]bool{*a.SMPPEnabled, *a.RESTEnabled, *a.QuerySMEnabled, *a.CancelSMEnabled}
}

func TestAnAccountIsCreatedWithItsDefaultsAndReadBackWithItsCustomer(t *testing.T) {
	g := newGateway(t)
	c := g.createCustomer(t, `{"name": "Acme"}`)

	a := g.createAccount(t, `{"customer_id": "`+c.ID+`", "name": "api"}`)
	if a.CustomerID != c.ID || a.CustomerName != "Acme" || a.CustomerStatus != "active" || a.Name != "api" ||
		a.Status != "active" || flags(a) != [4]bool{true, true, true, true} ||
		!slices.Equal(a.AllowedBindTypes, []string{"transceiver"}) || a.MaxSessions == nil || *a.MaxSessions != 1 ||
		a.RateLimit != nil || a.CreatedAt == nil || a.UpdatedAt == nil {
		t.Fatalf("created = %+v", a)
	}
	rec := g.send(t, http.MethodGet, "/admin/accounts/"+a.ID, "accounts:read", "")
	if body := rec.Body.String(); !strings.Contains(body, `"rate_limit":null`) {
		t.Fatalf("rate_limit not served null: %s", body)
	}
	read := as[accountBody](t, rec, http.StatusOK)
	if read.ID != a.ID || read.CustomerName != "Acme" || read.Name != "api" {
		t.Fatalf("read = %+v", read)
	}

	set := g.createAccount(t, `{"customer_id": "`+c.ID+`", "name": "smpp", "rest_enabled": false, "cancel_sm_enabled": false,
		"allowed_bind_types": ["transmitter", "receiver"], "max_sessions": 4}`)
	if flags(set) != [4]bool{true, false, true, false} || !slices.Equal(set.AllowedBindTypes, []string{"transmitter", "receiver"}) ||
		*set.MaxSessions != 4 {
		t.Fatalf("set = %+v", set)
	}
	crossed := g.createAccount(t, `{"customer_id": "`+c.ID+`", "name": "crossed", "rest_enabled": false, "query_sm_enabled": false}`)
	if flags(crossed) != [4]bool{true, false, false, true} {
		t.Fatalf("crossed = %+v", crossed)
	}
	for _, body := range []string{
		`{"customer_id": "` + c.ID + `", "name": "x", "allowed_bind_types": null}`,
		`{"customer_id": "` + c.ID + `", "name": "x", "max_sessions": 2147483648}`,
		`{"customer_id": "` + c.ID + `", "name": "x", "allowed_bind_types": ["bogus"]}`,
		`{"customer_id": "` + c.ID + `", "name": "x", "allowed_bind_types": []}`,
	} {
		if rec := g.send(t, http.MethodPost, "/admin/accounts", "accounts:write", body); rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: %d %s", body, rec.Code, rec.Body)
		}
	}
	lines := g.lines(t)
	if n := g.count(t, `SELECT count(*) FROM control_plane.audit_changes WHERE table_name = 'accounts' AND op = 'insert'
		AND actor = '`+operator+`' AND request_id = '`+lines[1].RequestID+`'`); n != 1 {
		t.Fatalf("%d inserts captured under the operator", n)
	}
	if rec := g.send(t, http.MethodGet, "/admin/accounts/00000000-0000-7000-8000-000000000000", "accounts:read", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown account: %d", rec.Code)
	}
}

func TestAnAccountIsRefusedATakenNameAnUnknownCustomerAndAClosedOne(t *testing.T) {
	g := newGateway(t)
	c := g.createCustomer(t, `{"name": "Acme"}`)
	g.createAccount(t, `{"customer_id": "`+c.ID+`", "name": "api"}`)
	other := g.createAccount(t, `{"customer_id": "`+c.ID+`", "name": "other"}`)

	for _, rec := range []struct {
		method, path, body string
	}{
		{http.MethodPost, "/admin/accounts", `{"customer_id": "` + c.ID + `", "name": "api"}`},
		{http.MethodPatch, "/admin/accounts/" + other.ID, `{"name": "api"}`},
	} {
		p := as[problem](t, g.send(t, rec.method, rec.path, "accounts:write", rec.body), http.StatusConflict)
		if p.Code != "conflict" || len(p.Errors) != 1 || p.Errors[0].Field != "body.name" {
			t.Fatalf("%s: %+v", rec.method, p)
		}
	}

	closed := g.createCustomer(t, `{"name": "Gone"}`)
	g.send(t, http.MethodPatch, "/admin/customers/"+closed.ID, "customers:write", `{"status": "closed"}`)
	for _, id := range []string{"00000000-0000-7000-8000-000000000000", closed.ID} {
		p := as[problem](t, g.send(t, http.MethodPost, "/admin/accounts", "accounts:write", `{"customer_id": "`+id+`", "name": "x"}`),
			http.StatusUnprocessableEntity)
		if p.Code != "validation_error" || len(p.Errors) != 1 || p.Errors[0].Field != "body.customer_id" {
			t.Fatalf("%s: %+v", id, p)
		}
	}

	suspended := g.createCustomer(t, `{"name": "Paused"}`)
	g.send(t, http.MethodPost, "/admin/customers/"+suspended.ID+"/suspend", "customers:write", "")
	a := g.createAccount(t, `{"customer_id": "`+suspended.ID+`", "name": "x"}`)
	if a.Status != "active" || a.CustomerStatus != "suspended" {
		t.Fatalf("under a suspended customer = %+v", a)
	}
}

func TestAccountsAreFilteredByCustomerGroupAndStatusAndPaged(t *testing.T) {
	g := newGateway(t)
	banks := g.createGroup(t, `{"name": "Banques"}`)
	acme := g.createCustomer(t, `{"name": "Acme", "group_id": "`+banks.ID+`"}`)
	bolt := g.createCustomer(t, `{"name": "Bolt", "group_id": "`+banks.ID+`"}`)
	solo := g.createCustomer(t, `{"name": "Solo"}`)
	g.accounts(t, acme.ID, "active:a1", "suspended:a2")
	g.accounts(t, bolt.ID, "active:b1")
	g.accounts(t, solo.ID, "active:s1")

	names := func(path string) []string {
		var out []string
		cursor := ""
		totals := map[int]bool{}
		for range 6 {
			p := as[accountPage](t, g.send(t, http.MethodGet, path+"&limit=1&cursor="+cursor, "accounts:read", ""), http.StatusOK)
			if p.Total == nil {
				t.Fatalf("%s: no total", path)
			}
			totals[*p.Total] = true
			for _, a := range p.Data {
				out = append(out, a.Name+"@"+a.CustomerName+":"+a.Status+"/"+a.CustomerStatus)
			}
			if p.NextCursor == nil {
				if len(totals) != 1 || !totals[len(out)] {
					t.Errorf("%s: totals %v for %d rows", path, totals, len(out))
				}
				return out
			}
			cursor = *p.NextCursor
		}
		t.Fatalf("%s does not end", path)
		return nil
	}
	for path, want := range map[string][]string{
		"/admin/accounts?": {"s1@Solo:active/active", "b1@Bolt:active/active", "a2@Acme:suspended/active",
			"a1@Acme:active/active"},
		"/admin/accounts?customer_id=" + acme.ID:                    {"a2@Acme:suspended/active", "a1@Acme:active/active"},
		"/admin/accounts?group_id=" + banks.ID:                      {"b1@Bolt:active/active", "a2@Acme:suspended/active", "a1@Acme:active/active"},
		"/admin/accounts?status=suspended":                          {"a2@Acme:suspended/active"},
		"/admin/accounts?group_id=" + banks.ID + "&status=active":   {"b1@Bolt:active/active", "a1@Acme:active/active"},
		"/admin/accounts?customer_id=" + solo.ID + "&status=active": {"s1@Solo:active/active"},
	} {
		if got := names(path); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", path, got, want)
		}
	}
}

func TestTheAccountSettingsAreEachSetByTheirOperation(t *testing.T) {
	g := newGateway(t)
	c := g.createCustomer(t, `{"name": "Acme"}`)
	a := g.createAccount(t, `{"customer_id": "`+c.ID+`", "name": "api"}`)
	path := "/admin/accounts/" + a.ID

	renamed := as[accountBody](t, g.send(t, http.MethodPatch, path, "accounts:write", `{"name": "web"}`), http.StatusOK)
	if renamed.Name != "web" || renamed.CustomerName != "Acme" || renamed.Status != "active" {
		t.Fatalf("renamed = %+v", renamed)
	}
	if same := as[accountBody](t, g.send(t, http.MethodPatch, path, "accounts:write", `{}`), http.StatusOK); same.Name != "web" {
		t.Fatalf("empty patch = %+v", same)
	}
	channels := as[accountBody](t, g.send(t, http.MethodPatch, path+"/channels", "accounts:write",
		`{"smpp_enabled": false, "rest_enabled": true}`), http.StatusOK)
	if flags(channels) != [4]bool{false, true, true, true} {
		t.Fatalf("channels = %+v", channels)
	}
	p := as[problem](t, g.send(t, http.MethodPatch, path+"/channels", "accounts:write",
		`{"smpp_enabled": false, "rest_enabled": false}`), http.StatusUnprocessableEntity)
	if p.Code != "validation_error" || len(p.Errors) != 0 {
		t.Fatalf("no channel: %+v", p)
	}
	limits := as[accountBody](t, g.send(t, http.MethodPatch, path+"/session-limits", "accounts:write",
		`{"max_sessions": 0, "allowed_bind_types": ["receiver", "transmitter"]}`), http.StatusOK)
	if *limits.MaxSessions != 0 || !slices.Equal(limits.AllowedBindTypes, []string{"receiver", "transmitter"}) {
		t.Fatalf("limits = %+v", limits)
	}
	ops := as[accountBody](t, g.send(t, http.MethodPatch, path+"/smpp-ops", "accounts:write", `{"cancel_sm_enabled": false}`),
		http.StatusOK)
	if flags(ops) != [4]bool{false, true, true, false} || *ops.MaxSessions != 0 ||
		!slices.Equal(ops.AllowedBindTypes, []string{"receiver", "transmitter"}) {
		t.Fatalf("ops = %+v", ops)
	}
	for _, body := range []string{`{"max_sessions": 1, "allowed_bind_types": null}`, `{"max_sessions": 2147483648, "allowed_bind_types": ["receiver"]}`} {
		if rec := g.send(t, http.MethodPatch, path+"/session-limits", "accounts:write", body); rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("session-limits %s: %d %s", body, rec.Code, rec.Body)
		}
	}
	for _, body := range []string{`{}`, `{"query_sm_enabled": null}`} {
		if rec := g.send(t, http.MethodPatch, path+"/smpp-ops", "accounts:write", body); rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("smpp-ops %s: %d %s", body, rec.Code, rec.Body)
		}
	}
	for _, body := range []string{`{"name": null}`, `{"status": null}`} {
		if rec := g.send(t, http.MethodPatch, path, "accounts:write", body); rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("update %s: %d %s", body, rec.Code, rec.Body)
		}
	}
	if rec := g.send(t, http.MethodPatch, "/admin/accounts/00000000-0000-7000-8000-000000000000/channels", "accounts:write",
		`{"smpp_enabled": true, "rest_enabled": true}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown account: %d", rec.Code)
	}
	if n := g.count(t, `SELECT count(*) FROM control_plane.audit_changes WHERE table_name = 'accounts' AND op = 'update'
		AND actor = '`+operator+`'`); n != 5 {
		t.Fatalf("%d updates captured under the operator", n)
	}
	g.send(t, http.MethodPatch, "/admin/customers/"+c.ID, "customers:write", `{"name": "Acme SA"}`)
	if read := as[accountBody](t, g.send(t, http.MethodGet, path, "accounts:read", ""), http.StatusOK); read.CustomerName != "Acme SA" {
		t.Fatalf("customer renamed, account reads %q", read.CustomerName)
	}
}

func TestAnAccountIsSuspendedReactivatedAndClosedForGood(t *testing.T) {
	g := newGateway(t)
	c := g.createCustomer(t, `{"name": "Acme"}`)
	a := g.createAccount(t, `{"customer_id": "`+c.ID+`", "name": "api"}`)
	path := "/admin/accounts/" + a.ID

	for range 2 {
		if s := as[accountBody](t, g.send(t, http.MethodPost, path+"/suspend", "accounts:write", ""), http.StatusOK); s.Status != "suspended" {
			t.Fatalf("suspended = %+v", s)
		}
	}
	if renamed := as[accountBody](t, g.send(t, http.MethodPatch, path, "accounts:write", `{"name": "web"}`), http.StatusOK); renamed.Status != "suspended" {
		t.Fatalf("a rename changed the status: %+v", renamed)
	}
	g.send(t, http.MethodPost, "/admin/customers/"+c.ID+"/suspend", "customers:write", "")
	r := as[accountBody](t, g.send(t, http.MethodPatch, path, "accounts:write", `{"status": "active"}`), http.StatusOK)
	if r.Status != "active" || r.CustomerStatus != "suspended" {
		t.Fatalf("reactivated under a suspended customer = %+v", r)
	}
	if closed := as[accountBody](t, g.send(t, http.MethodPatch, path, "accounts:write", `{"status": "closed"}`), http.StatusOK); closed.Status != "closed" {
		t.Fatalf("closed = %+v", closed)
	}
	p := as[problem](t, g.send(t, http.MethodPost, path+"/suspend", "accounts:write", ""), http.StatusConflict)
	if p.Code != "conflict" {
		t.Fatalf("suspend closed: %+v", p)
	}
	p = as[problem](t, g.send(t, http.MethodPatch, path, "accounts:write", `{"status": "active"}`), http.StatusUnprocessableEntity)
	if p.Code != "validation_error" || len(p.Errors) != 1 || p.Errors[0].Field != "body.status" {
		t.Fatalf("reopen closed: %+v", p)
	}
	if rec := g.send(t, http.MethodPost, "/admin/accounts/00000000-0000-7000-8000-000000000000/suspend", "accounts:write", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown account: %d", rec.Code)
	}
}

func TestAnAccountIsDeleted(t *testing.T) {
	g := newGateway(t)
	c := g.createCustomer(t, `{"name": "Acme"}`)
	a := g.createAccount(t, `{"customer_id": "`+c.ID+`", "name": "api"}`)
	path := "/admin/accounts/" + a.ID

	if rec := g.send(t, http.MethodDelete, path, "accounts:write", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if rec := g.send(t, http.MethodGet, path, "accounts:read", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("read after delete: %d", rec.Code)
	}
	if rec := g.send(t, http.MethodDelete, path, "accounts:write", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("delete again: %d", rec.Code)
	}
	if rec := g.send(t, http.MethodDelete, "/admin/customers/"+c.ID, "customers:write", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("customer without accounts: %d", rec.Code)
	}
}

func TestAnAccountCreatedWhileItsCustomerClosesWaitsAndIsRefused(t *testing.T) {
	g := newGateway(t)
	c := g.createCustomer(t, `{"name": "Acme"}`)
	ctx := context.Background()
	closing, err := pgx.Connect(ctx, g.pg.As("migrator"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closing.Close(ctx) })
	if _, err := closing.Exec(ctx, `BEGIN; SET LOCAL app.actor = 'test';
		UPDATE control_plane.customers SET status = 'closed' WHERE id = '`+c.ID+`'`); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/accounts", strings.NewReader(`{"customer_id": "`+c.ID+`", "name": "api"}`))
	req.Header.Set("Authorization", "Bearer "+g.k.Token(t, "v1", "accounts:write"))
	req.Header.Set("Content-Type", "application/json")
	done := make(chan int)
	go func() {
		rec := httptest.NewRecorder()
		g.h.ServeHTTP(rec, req)
		done <- rec.Code
	}()
	for deadline := time.Now().Add(5 * time.Second); g.count(t,
		`SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND usename = 'admin_api_svc'`) == 0; {
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := closing.Exec(ctx, `COMMIT`); err != nil {
		t.Fatal(err)
	}
	if code := <-done; code != http.StatusUnprocessableEntity {
		t.Fatalf("created under a closing customer: %d", code)
	}
}
