package postgres_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/martialanouman/sms-platform/internal/platform/snapshot"
)

type group struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	Status      string     `json:"status"`
	MemberCount *int       `json:"member_count"`
	CreatedBy   *string    `json:"created_by"`
	CreatedAt   *time.Time `json:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at"`
}

type problem struct {
	Code   string `json:"code"`
	Errors []struct {
		Field string `json:"field"`
	} `json:"errors"`
}

func (g gateway) send(t *testing.T, method, path, scope, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+g.k.Token(t, "v1", scope))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	g.h.ServeHTTP(rec, req)
	return rec
}

func as[T any](t *testing.T, rec *httptest.ResponseRecorder, status int) T {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status %d, want %d: %s", rec.Code, status, rec.Body)
	}
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("%v: %s", err, rec.Body)
	}
	return v
}

func (g gateway) createGroup(t *testing.T, body string) group {
	t.Helper()
	return as[group](t, g.send(t, http.MethodPost, "/admin/customer-groups", "customer-groups:write", body), http.StatusCreated)
}

func (g gateway) exec(t *testing.T, statements ...string) {
	t.Helper()
	for _, s := range statements {
		if _, err := g.migrator.Exec(context.Background(), s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func TestAGroupIsCreatedUnderItsOperatorAndReadBack(t *testing.T) {
	g := newGateway(t)
	before := time.Now()

	created := g.createGroup(t, `{"name": "Banques", "description": "Établissements financiers"}`)
	if created.Name != "Banques" || created.Description == nil || *created.Description != "Établissements financiers" ||
		created.Status != "active" || created.MemberCount == nil || *created.MemberCount != 0 ||
		created.CreatedBy == nil || *created.CreatedBy != operator ||
		created.CreatedAt == nil || created.CreatedAt.Before(before.Add(-time.Minute)) ||
		created.UpdatedAt == nil || created.UpdatedAt.Before(before.Add(-time.Minute)) {
		t.Fatalf("created = %+v", created)
	}
	lines := g.lines(t)
	if len(lines) != 1 || lines[0].OperationID != "create-customer-group" || lines[0].Status != 201 {
		t.Fatalf("lines = %+v", lines)
	}
	if n := g.count(t, `SELECT count(*) FROM control_plane.audit_changes WHERE table_name = 'customer_groups'
		AND op = 'insert' AND actor = '`+operator+`' AND request_id = '`+lines[0].RequestID+`'`); n != 1 {
		t.Fatalf("%d changes captured under the operator", n)
	}

	read := as[group](t, g.send(t, http.MethodGet, "/admin/customer-groups/"+created.ID, "customer-groups:read", ""), http.StatusOK)
	if read.ID != created.ID || read.Name != "Banques" || *read.Description != "Établissements financiers" || *read.CreatedBy != operator {
		t.Fatalf("read = %+v", read)
	}
	if rec := g.send(t, http.MethodGet, "/admin/customer-groups/00000000-0000-7000-8000-000000000000", "customer-groups:read", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown group: %d %s", rec.Code, rec.Body)
	}
}

func TestAGroupNameIsTakenOnce(t *testing.T) {
	g := newGateway(t)
	g.createGroup(t, `{"name": "Banques"}`)
	other := g.createGroup(t, `{"name": "Télécoms"}`)

	for _, rec := range []*httptest.ResponseRecorder{
		g.send(t, http.MethodPost, "/admin/customer-groups", "customer-groups:write", `{"name": "Banques"}`),
		g.send(t, http.MethodPatch, "/admin/customer-groups/"+other.ID, "customer-groups:write", `{"name": "Banques"}`),
	} {
		p := as[problem](t, rec, http.StatusConflict)
		if p.Code != "conflict" || len(p.Errors) != 1 || p.Errors[0].Field != "body.name" {
			t.Fatalf("problem = %+v", p)
		}
	}
	if lines := g.lines(t); len(lines) != 4 || lines[2].Status != 409 || lines[3].Status != 409 {
		t.Fatalf("lines = %+v", lines)
	}
}

func TestAPatchKeepsAnAbsentFieldAndClearsANullOne(t *testing.T) {
	g := newGateway(t)
	created := g.createGroup(t, `{"name": "Banques", "description": "d"}`)
	path := "/admin/customer-groups/" + created.ID

	archived := as[group](t, g.send(t, http.MethodPatch, path, "customer-groups:write", `{"status": "archived"}`), http.StatusOK)
	if archived.Status != "archived" || archived.Name != "Banques" || archived.Description == nil || *archived.Description != "d" ||
		!archived.UpdatedAt.After(*created.UpdatedAt) {
		t.Fatalf("archived = %+v", archived)
	}
	cleared := as[group](t, g.send(t, http.MethodPatch, path, "customer-groups:write", `{"description": null}`), http.StatusOK)
	if cleared.Description != nil || cleared.Status != "archived" || cleared.Name != "Banques" {
		t.Fatalf("cleared = %+v", cleared)
	}
	renamed := as[group](t, g.send(t, http.MethodPatch, path, "customer-groups:write", `{"name": "Assurances", "description": "e"}`), http.StatusOK)
	if renamed.Name != "Assurances" || renamed.Description == nil || *renamed.Description != "e" || renamed.Status != "archived" {
		t.Fatalf("renamed = %+v", renamed)
	}
	if rec := g.send(t, http.MethodPatch, "/admin/customer-groups/00000000-0000-7000-8000-000000000000", "customer-groups:write", `{"name": "x"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown group: %d %s", rec.Code, rec.Body)
	}
}

func TestAnUnreadableBodyIsAValidationError(t *testing.T) {
	g := newGateway(t)
	for name, body := range map[string]string{"truncated": `{"name": `, "empty": ``, "unknown field": `{"name": "x", "colour": "red"}`} {
		p := as[problem](t, g.send(t, http.MethodPost, "/admin/customer-groups", "customer-groups:write", body), http.StatusUnprocessableEntity)
		if p.Code != "validation_error" {
			t.Errorf("%s: %+v", name, p)
		}
	}
	if n := g.count(t, `SELECT count(*) FROM control_plane.customer_groups`); n != 0 {
		t.Fatalf("%d groups written", n)
	}
}

func TestGroupsArePagedFilteredAndCountTheirMembers(t *testing.T) {
	g := newGateway(t)
	first := g.createGroup(t, `{"name": "a"}`)
	second := g.createGroup(t, `{"name": "b"}`)
	third := g.createGroup(t, `{"name": "c"}`)
	g.send(t, http.MethodPatch, "/admin/customer-groups/"+second.ID, "customer-groups:write", `{"status": "archived"}`)
	g.exec(t, `SET app.actor = 'test'`,
		`INSERT INTO control_plane.customers (name, group_id) VALUES ('x', '`+third.ID+`'), ('y', '`+third.ID+`'), ('z', '`+first.ID+`'), ('w', NULL)`)

	type groupPage struct {
		Data       []group `json:"data"`
		NextCursor *string `json:"next_cursor"`
	}
	one := as[groupPage](t, g.send(t, http.MethodGet, "/admin/customer-groups?limit=2", "customer-groups:read", ""), http.StatusOK)
	if len(one.Data) != 2 || one.Data[0].ID != third.ID || one.Data[1].ID != second.ID || one.NextCursor == nil ||
		*one.Data[0].MemberCount != 2 || *one.Data[1].MemberCount != 0 {
		t.Fatalf("first page = %+v", one)
	}
	two := as[groupPage](t, g.send(t, http.MethodGet, "/admin/customer-groups?limit=2&cursor="+*one.NextCursor, "customer-groups:read", ""), http.StatusOK)
	if len(two.Data) != 1 || two.Data[0].ID != first.ID || *two.Data[0].MemberCount != 1 || two.NextCursor != nil {
		t.Fatalf("second page = %+v", two)
	}
	if read := as[group](t, g.send(t, http.MethodGet, "/admin/customer-groups/"+third.ID, "customer-groups:read", ""), http.StatusOK); *read.MemberCount != 2 {
		t.Fatalf("read member_count = %d", *read.MemberCount)
	}
	archived := as[groupPage](t, g.send(t, http.MethodGet, "/admin/customer-groups?status=archived", "customer-groups:read", ""), http.StatusOK)
	if len(archived.Data) != 1 || archived.Data[0].ID != second.ID {
		t.Fatalf("archived = %+v", archived)
	}
	if rec := g.send(t, http.MethodGet, "/admin/customer-groups?cursor=nope", "customer-groups:read", ""); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad cursor: %d", rec.Code)
	}
}

func TestDeletingAGroupDetachesItsCustomers(t *testing.T) {
	g := newGateway(t)
	created := g.createGroup(t, `{"name": "a"}`)
	g.exec(t, `SET app.actor = 'test'`, `INSERT INTO control_plane.customers (name, group_id) VALUES ('x', '`+created.ID+`')`)
	path := "/admin/customer-groups/" + created.ID

	if rec := g.send(t, http.MethodDelete, path, "customer-groups:write", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if n := g.count(t, `SELECT count(*) FROM control_plane.customers WHERE group_id IS NULL`); n != 1 {
		t.Fatalf("%d detached customers", n)
	}
	if rec := g.send(t, http.MethodGet, path, "customer-groups:read", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("read after delete: %d", rec.Code)
	}
	if rec := g.send(t, http.MethodDelete, path, "customer-groups:write", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("delete again: %d", rec.Code)
	}
	if lines := g.lines(t); len(lines) != 3 || lines[1].OperationID != "delete-customer-group" || lines[1].Status != 204 || lines[2].Status != 404 {
		t.Fatalf("lines = %+v", lines)
	}
}

func TestTheCustomersOfAGroupArePaged(t *testing.T) {
	g := newGateway(t)
	created := g.createGroup(t, `{"name": "a"}`)
	other := g.createGroup(t, `{"name": "b"}`)
	g.exec(t, `SET app.actor = 'test'`,
		`INSERT INTO control_plane.customers (name, group_id, max_age_otp, max_age_marketing) VALUES ('x', '`+created.ID+`', 60, 3600)`,
		`INSERT INTO control_plane.customers (name, group_id) VALUES ('y', '`+created.ID+`')`,
		`INSERT INTO control_plane.customers (name, group_id) VALUES ('z', '`+other.ID+`')`,
		`UPDATE control_plane.customers SET status = 'suspended' WHERE name = 'x'`)

	type customer struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Status         string `json:"status"`
		GroupID        string `json:"group_id"`
		MaxAgeOverride *struct {
			OTP           *int `json:"otp"`
			Transactional *int `json:"transactional"`
			Marketing     *int `json:"marketing"`
		} `json:"max_age_override"`
		CreatedAt *time.Time `json:"created_at"`
		UpdatedAt *time.Time `json:"updated_at"`
	}
	type customerPage struct {
		Data       []customer `json:"data"`
		NextCursor *string    `json:"next_cursor"`
	}
	path := "/admin/customer-groups/" + created.ID + "/customers"
	one := as[customerPage](t, g.send(t, http.MethodGet, path+"?limit=1", "customer-groups:read", ""), http.StatusOK)
	if len(one.Data) != 1 || one.Data[0].Name != "y" || one.NextCursor == nil {
		t.Fatalf("first page = %+v", one)
	}
	two := as[customerPage](t, g.send(t, http.MethodGet, path+"?limit=1&cursor="+*one.NextCursor, "customer-groups:read", ""), http.StatusOK)
	if len(two.Data) != 1 || two.NextCursor != nil {
		t.Fatalf("second page = %+v", two)
	}
	x := two.Data[0]
	if x.Name != "x" || x.Status != "suspended" || x.GroupID != created.ID || x.MaxAgeOverride == nil ||
		x.MaxAgeOverride.OTP == nil || *x.MaxAgeOverride.OTP != 60 || x.MaxAgeOverride.Transactional != nil ||
		x.MaxAgeOverride.Marketing == nil || *x.MaxAgeOverride.Marketing != 3600 ||
		x.CreatedAt == nil || x.CreatedAt.IsZero() || x.UpdatedAt == nil || !x.UpdatedAt.After(*x.CreatedAt) || x.ID == "" {
		t.Fatalf("customer = %+v", x)
	}
	if rec := g.send(t, http.MethodGet, "/admin/customer-groups/00000000-0000-7000-8000-000000000000/customers", "customer-groups:read", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown group: %d %s", rec.Code, rec.Body)
	}
}

func TestAGroupWrittenThroughTheGatewayReachesASnapshotReader(t *testing.T) {
	g := newGateway(t)
	g.exec(t, `CREATE ROLE reader LOGIN PASSWORD 'reader'`, `GRANT USAGE ON SCHEMA control_plane TO reader`,
		`GRANT SELECT ON control_plane.customer_groups TO reader`)
	ctx, cancel := context.WithCancel(context.Background())
	names := func(rows []json.RawMessage) (any, error) {
		var out []string
		for _, r := range rows {
			var row struct{ Name string }
			if err := json.Unmarshal(r, &row); err != nil {
				return nil, err
			}
			out = append(out, row.Name)
		}
		return out, nil
	}
	s, err := snapshot.Open(ctx, slog.New(slog.DiscardHandler), g.pg.As("reader"), snapshot.Table{Name: "customer_groups", Build: names})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	g.createGroup(t, `{"name": "Banques"}`)
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if got, _ := s.Get("customer_groups").([]string); len(got) == 1 && got[0] == "Banques" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("snapshot = %v", s.Get("customer_groups"))
		}
	}
}

func TestABodyBeyondTheLimitIsTooLarge(t *testing.T) {
	g := newGateway(t)
	p := as[problem](t, g.send(t, http.MethodPost, "/admin/customer-groups", "customer-groups:write",
		`{"name": "x", "description": "`+strings.Repeat("a", 2<<20)+`"}`), http.StatusRequestEntityTooLarge)
	if p.Code != "payload_too_large" {
		t.Fatalf("problem = %+v", p)
	}
}
