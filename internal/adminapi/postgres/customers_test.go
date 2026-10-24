package postgres_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type maxAgeBody struct {
	OTP           *int `json:"otp"`
	Transactional *int `json:"transactional"`
	Marketing     *int `json:"marketing"`
}

type customerBody struct {
	ID             string      `json:"id"`
	Name           string      `json:"name"`
	Status         string      `json:"status"`
	GroupID        *string     `json:"group_id"`
	MaxAgeOverride *maxAgeBody `json:"max_age_override"`
	CreatedAt      *time.Time  `json:"created_at"`
	UpdatedAt      *time.Time  `json:"updated_at"`
}

type customerPage struct {
	Data       []customerBody `json:"data"`
	NextCursor *string        `json:"next_cursor"`
	Total      *int           `json:"total"`
}

func (g gateway) createCustomer(t *testing.T, body string) customerBody {
	t.Helper()
	return as[customerBody](t, g.send(t, http.MethodPost, "/admin/customers", "customers:write", body), http.StatusCreated)
}

func (g gateway) accounts(t *testing.T, customer string, names ...string) {
	t.Helper()
	for _, n := range names {
		status, name, _ := strings.Cut(n, ":")
		g.exec(t, `SET app.actor = 'test'`,
			`INSERT INTO control_plane.accounts (customer_id, name) VALUES ('`+customer+`', '`+name+`')`)
		if status != "active" {
			g.exec(t, `UPDATE control_plane.accounts SET status = '`+status+`' WHERE name = '`+name+`'`)
		}
	}
}

func (g gateway) statuses(t *testing.T, customer string) map[string]string {
	t.Helper()
	rows, err := g.migrator.Query(context.Background(),
		`SELECT name, status FROM control_plane.accounts WHERE customer_id = $1`, customer)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for rows.Next() {
		var name, status string
		if err := rows.Scan(&name, &status); err != nil {
			t.Fatal(err)
		}
		out[name] = status
	}
	return out
}

func ages(c customerBody) [3]*int {
	if c.MaxAgeOverride == nil {
		return [3]*int{}
	}
	return [3]*int{c.MaxAgeOverride.OTP, c.MaxAgeOverride.Transactional, c.MaxAgeOverride.Marketing}
}

func same(got [3]*int, want [3]int) bool {
	for i, w := range want {
		if (w == 0) != (got[i] == nil) || (got[i] != nil && *got[i] != w) {
			return false
		}
	}
	return true
}

func TestACustomerIsCreatedInItsGroupAndReadBack(t *testing.T) {
	g := newGateway(t)
	group := g.createGroup(t, `{"name": "Banques"}`)
	before := time.Now()

	c := g.createCustomer(t, `{"name": "Acme", "group_id": "`+group.ID+`",
		"max_age_override": {"otp": 60, "transactional": null, "marketing": 3600}}`)
	if c.Name != "Acme" || c.Status != "active" || c.GroupID == nil || *c.GroupID != group.ID ||
		!same(ages(c), [3]int{60, 0, 3600}) || c.CreatedAt == nil || c.CreatedAt.Before(before.Add(-time.Minute)) ||
		c.UpdatedAt == nil || c.UpdatedAt.Before(before.Add(-time.Minute)) {
		t.Fatalf("created = %+v", c)
	}
	read := as[customerBody](t, g.send(t, http.MethodGet, "/admin/customers/"+c.ID, "customers:read", ""), http.StatusOK)
	if read.ID != c.ID || read.Name != "Acme" || *read.GroupID != group.ID || !same(ages(read), [3]int{60, 0, 3600}) {
		t.Fatalf("read = %+v", read)
	}
	if rec := g.send(t, http.MethodPost, "/admin/customers", "customers:write", `{"name": "x", "max_age_override": null}`); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("null ages at creation: %d %s", rec.Code, rec.Body)
	}
	bare := g.createCustomer(t, `{"name": "Bare"}`)
	if bare.GroupID != nil || bare.MaxAgeOverride == nil || !same(ages(bare), [3]int{}) {
		t.Fatalf("bare = %+v", bare)
	}
	lines := g.lines(t)
	if len(lines) != 4 || lines[1].OperationID != "create-customer" || lines[1].Status != 201 || lines[2].Status != 422 {
		t.Fatalf("lines = %+v", lines)
	}
	if n := g.count(t, `SELECT count(*) FROM control_plane.audit_changes WHERE table_name = 'customers' AND op = 'insert'
		AND actor = '`+operator+`' AND request_id = '`+lines[1].RequestID+`'`); n != 1 {
		t.Fatalf("%d inserts captured under the operator", n)
	}
	if rec := g.send(t, http.MethodGet, "/admin/customers/00000000-0000-7000-8000-000000000000", "customers:read", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown customer: %d", rec.Code)
	}
}

func TestAnUnknownGroupIsAValidationError(t *testing.T) {
	g := newGateway(t)
	c := g.createCustomer(t, `{"name": "Acme"}`)
	const unknown = `{"group_id": "00000000-0000-7000-8000-000000000000"}`
	for _, rec := range []*httptest.ResponseRecorder{
		g.send(t, http.MethodPost, "/admin/customers", "customers:write", `{"name": "x", "group_id": "00000000-0000-7000-8000-000000000000"}`),
		g.send(t, http.MethodPatch, "/admin/customers/"+c.ID, "customers:write", unknown),
	} {
		p := as[problem](t, rec, http.StatusUnprocessableEntity)
		if p.Code != "validation_error" || len(p.Errors) != 1 || p.Errors[0].Field != "body.group_id" {
			t.Fatalf("problem = %+v", p)
		}
	}
}

func TestCustomersAreFilteredByGroupAndStatusAndPaged(t *testing.T) {
	g := newGateway(t)
	banks := g.createGroup(t, `{"name": "Banques"}`)
	g.exec(t, `SET app.actor = 'test'`,
		`INSERT INTO control_plane.customers (name, group_id) VALUES ('a', '`+banks.ID+`'), ('b', '`+banks.ID+`'), ('c', NULL)`,
		`UPDATE control_plane.customers SET status = 'suspended' WHERE name = 'b'`)

	names := func(path string) []string {
		var out []string
		cursor := ""
		totals := map[int]bool{}
		for range 5 {
			p := as[customerPage](t, g.send(t, http.MethodGet, path+"&limit=1&cursor="+cursor, "customers:read", ""), http.StatusOK)
			if p.Total == nil {
				t.Fatalf("%s: no total", path)
			}
			totals[*p.Total] = true
			for _, c := range p.Data {
				out = append(out, c.Name)
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
	for path, want := range map[string]int{
		"/admin/customers?":                                        3,
		"/admin/customers?group_id=" + banks.ID:                    2,
		"/admin/customers?status=suspended":                        1,
		"/admin/customers?group_id=" + banks.ID + "&status=active": 1,
	} {
		got := names(path)
		seen := map[string]bool{}
		for _, n := range got {
			seen[n] = true
		}
		if len(got) != want || len(seen) != want {
			t.Errorf("%s: %v", path, got)
		}
	}
	if got := names("/admin/customers?status=suspended"); len(got) != 1 || got[0] != "b" {
		t.Fatalf("suspended = %v", got)
	}
}

func TestACustomerPatchKeepsAbsentFieldsAndClearsNullOnes(t *testing.T) {
	g := newGateway(t)
	group := g.createGroup(t, `{"name": "Banques"}`)
	c := g.createCustomer(t, `{"name": "Acme", "group_id": "`+group.ID+`", "max_age_override": {"otp": 60, "transactional": 600, "marketing": null}}`)
	path := "/admin/customers/" + c.ID

	renamed := as[customerBody](t, g.send(t, http.MethodPatch, path, "customers:write", `{"name": "Acme SA"}`), http.StatusOK)
	if renamed.Name != "Acme SA" || renamed.GroupID == nil || !same(ages(renamed), [3]int{60, 600, 0}) {
		t.Fatalf("renamed = %+v", renamed)
	}
	aged := as[customerBody](t, g.send(t, http.MethodPatch, path, "customers:write",
		`{"max_age_override": {"otp": null, "transactional": 300, "marketing": 7200}}`), http.StatusOK)
	if !same(ages(aged), [3]int{0, 300, 7200}) || aged.GroupID == nil {
		t.Fatalf("aged = %+v", aged)
	}
	detached := as[customerBody](t, g.send(t, http.MethodPatch, path, "customers:write", `{"group_id": null}`), http.StatusOK)
	if detached.GroupID != nil || detached.Name != "Acme SA" || !same(ages(detached), [3]int{0, 300, 7200}) {
		t.Fatalf("detached = %+v", detached)
	}
	untouched := as[customerBody](t, g.send(t, http.MethodPatch, path, "customers:write", `{}`), http.StatusOK)
	if untouched.Name != "Acme SA" || untouched.Status != "active" {
		t.Fatalf("untouched = %+v", untouched)
	}
	for name, body := range map[string]string{
		"null name": `{"name": null}`, "null status": `{"status": null}`, "null ages": `{"max_age_override": null}`,
		"partial ages": `{"max_age_override": {"otp": 60}}`, "otp too old": `{"max_age_override": {"otp": 601, "transactional": null, "marketing": null}}`,
	} {
		if rec := g.send(t, http.MethodPatch, path, "customers:write", body); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
	if rec := g.send(t, http.MethodPatch, "/admin/customers/00000000-0000-7000-8000-000000000000", "customers:write", `{"name": "x"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown: %d", rec.Code)
	}
}

func TestSuspendingACustomerSuspendsItsOpenAccountsUnderTheOperator(t *testing.T) {
	g := newGateway(t)
	c := g.createCustomer(t, `{"name": "Acme"}`)
	other := g.createCustomer(t, `{"name": "Other"}`)
	g.accounts(t, c.ID, "active:a", "suspended:b", "closed:c")
	g.accounts(t, other.ID, "active:o")

	s := as[customerBody](t, g.send(t, http.MethodPost, "/admin/customers/"+c.ID+"/suspend", "customers:write", ""), http.StatusOK)
	if s.Status != "suspended" {
		t.Fatalf("suspended = %+v", s)
	}
	if got := g.statuses(t, c.ID); got["a"] != "suspended" || got["b"] != "suspended" || got["c"] != "closed" {
		t.Fatalf("accounts = %v", got)
	}
	if got := g.statuses(t, other.ID); got["o"] != "active" {
		t.Fatalf("another customer's account = %v", got)
	}
	lines := g.lines(t)
	last := lines[len(lines)-1]
	if last.OperationID != "suspend-customer" || last.Status != 200 {
		t.Fatalf("line = %+v", last)
	}
	if n := g.count(t, `SELECT count(*) FROM control_plane.audit_changes WHERE table_name = 'accounts' AND op = 'update'
		AND actor = '`+operator+`' AND request_id = '`+last.RequestID+`'`); n != 1 {
		t.Fatalf("%d account changes under the operator", n)
	}
	again := as[customerBody](t, g.send(t, http.MethodPost, "/admin/customers/"+c.ID+"/suspend", "customers:write", ""), http.StatusOK)
	if again.Status != "suspended" {
		t.Fatalf("again = %+v", again)
	}
	if rec := g.send(t, http.MethodPost, "/admin/customers/00000000-0000-7000-8000-000000000000/suspend", "customers:write", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown: %d", rec.Code)
	}
}

func TestAStatusPatchCascadesSuspensionAndClosureButNotReactivation(t *testing.T) {
	g := newGateway(t)
	c := g.createCustomer(t, `{"name": "Acme"}`)
	g.accounts(t, c.ID, "active:a", "active:b")
	path := "/admin/customers/" + c.ID

	as[customerBody](t, g.send(t, http.MethodPatch, path, "customers:write", `{"status": "suspended"}`), http.StatusOK)
	if got := g.statuses(t, c.ID); got["a"] != "suspended" || got["b"] != "suspended" {
		t.Fatalf("after suspension = %v", got)
	}
	active := as[customerBody](t, g.send(t, http.MethodPatch, path, "customers:write", `{"status": "active"}`), http.StatusOK)
	if active.Status != "active" {
		t.Fatalf("reactivated = %+v", active)
	}
	if got := g.statuses(t, c.ID); got["a"] != "suspended" || got["b"] != "suspended" {
		t.Fatalf("after reactivation = %v", got)
	}
	g.exec(t, `UPDATE control_plane.accounts SET status = 'active' WHERE name = 'a'`)
	closed := as[customerBody](t, g.send(t, http.MethodPatch, path, "customers:write", `{"status": "closed"}`), http.StatusOK)
	if closed.Status != "closed" {
		t.Fatalf("closed = %+v", closed)
	}
	if got := g.statuses(t, c.ID); got["a"] != "closed" || got["b"] != "closed" {
		t.Fatalf("after closure = %v", got)
	}
}

func TestAClosedCustomerStaysClosed(t *testing.T) {
	g := newGateway(t)
	c := g.createCustomer(t, `{"name": "Acme"}`)
	path := "/admin/customers/" + c.ID
	as[customerBody](t, g.send(t, http.MethodPatch, path, "customers:write", `{"status": "closed"}`), http.StatusOK)

	reopened := as[problem](t, g.send(t, http.MethodPatch, path, "customers:write", `{"status": "active"}`), http.StatusUnprocessableEntity)
	if reopened.Code != "validation_error" || len(reopened.Errors) != 1 || reopened.Errors[0].Field != "body.status" {
		t.Fatalf("reopened = %+v", reopened)
	}
	suspended := as[problem](t, g.send(t, http.MethodPost, path+"/suspend", "customers:write", ""), http.StatusConflict)
	if suspended.Code != "conflict" {
		t.Fatalf("suspended = %+v", suspended)
	}
	if read := as[customerBody](t, g.send(t, http.MethodGet, path, "customers:read", ""), http.StatusOK); read.Status != "closed" {
		t.Fatalf("read = %+v", read)
	}
}

func TestTheSuspensionImpactListsActiveAndClosedAccountsAndNoBind(t *testing.T) {
	g := newGateway(t)
	c := g.createCustomer(t, `{"name": "Acme"}`)
	g.accounts(t, c.ID, "active:b", "suspended:a", "closed:c")
	g.accounts(t, g.createCustomer(t, `{"name": "Other"}`).ID, "active:o")

	rec := g.send(t, http.MethodGet, "/admin/customers/"+c.ID+"/suspension-impact", "customers:read", "")
	type ref struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	impact := as[struct {
		Suspend []ref  `json:"accounts_to_suspend"`
		Closed  []ref  `json:"accounts_closed"`
		Binds   *[]any `json:"binds_to_disconnect"`
	}](t, rec, http.StatusOK)
	if len(impact.Suspend) != 1 || impact.Suspend[0].Name != "b" || impact.Suspend[0].ID == "" ||
		len(impact.Closed) != 1 || impact.Closed[0].Name != "c" || impact.Binds == nil || len(*impact.Binds) != 0 {
		t.Fatalf("impact = %s", rec.Body)
	}
	if got := g.statuses(t, c.ID); got["b"] != "active" {
		t.Fatalf("the preview changed an account: %v", got)
	}
	if rec := g.send(t, http.MethodGet, "/admin/customers/00000000-0000-7000-8000-000000000000/suspension-impact", "customers:read", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown: %d", rec.Code)
	}
}

func TestACustomerWithAccountsIsNotDeleted(t *testing.T) {
	g := newGateway(t)
	c := g.createCustomer(t, `{"name": "Acme"}`)
	g.accounts(t, c.ID, "active:a")
	path := "/admin/customers/" + c.ID

	p := as[problem](t, g.send(t, http.MethodDelete, path, "customers:write", ""), http.StatusConflict)
	if p.Code != "conflict" {
		t.Fatalf("problem = %+v", p)
	}
	g.exec(t, `DELETE FROM control_plane.accounts`)
	if rec := g.send(t, http.MethodDelete, path, "customers:write", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if rec := g.send(t, http.MethodGet, path, "customers:read", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("read after delete: %d", rec.Code)
	}
	if rec := g.send(t, http.MethodDelete, path, "customers:write", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("delete again: %d", rec.Code)
	}
}
