package postgres_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/martialanouman/sms-platform/internal/adminapi"
	"github.com/martialanouman/sms-platform/internal/adminapi/adminapitest"
	adminhuma "github.com/martialanouman/sms-platform/internal/adminapi/huma"
	"github.com/martialanouman/sms-platform/internal/adminapi/postgres"
	errs "github.com/martialanouman/sms-platform/internal/platform/errors"
	"github.com/martialanouman/sms-platform/internal/platform/pgtest"
	"github.com/martialanouman/sms-platform/migrations"
)

type gateway struct {
	h           http.Handler
	k           adminapitest.Keys
	uncommitted prometheus.Counter
	migrator    *pgx.Conn
}

type line struct {
	Operator, OperationID, Target, RequestID string
	Status                                   int
}

func newGateway(t *testing.T) gateway {
	t.Helper()
	ctx := context.Background()
	pg := pgtest.ControlPlane(t)
	migrator, err := pgx.Connect(ctx, pg.As("migrator"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = migrator.Close(ctx) })
	for _, s := range []string{string(migrations.Grants("control_plane")), `ALTER ROLE admin_api_svc PASSWORD 'admin_api_svc'`,
		`CREATE TABLE control_plane.probe (id int PRIMARY KEY)`,
		`CREATE TRIGGER audit_capture AFTER INSERT OR UPDATE OR DELETE ON control_plane.probe
			FOR EACH ROW EXECUTE FUNCTION control_plane.audit_capture()`,
		`GRANT INSERT ON control_plane.probe TO admin_api_svc`} {
		if _, err := migrator.Exec(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	pool, err := pgxpool.New(ctx, pg.As("admin_api_svc"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	k := adminapitest.NewKeys(t, "v1")
	uncommitted := prometheus.NewCounter(prometheus.CounterOpts{Name: "uncommitted"})
	h, api := adminhuma.New(adminhuma.Config{Store: postgres.New(pool), Verifier: k.Verifier(), Uncommitted: uncommitted})
	huma.Register(api, huma.Operation{OperationID: "create-probe", Method: http.MethodPost, Path: "/admin/probes/{id}",
		Security: []map[string][]string{{"OperatorBearer": {"routes:write"}}}},
		func(ctx context.Context, in *struct {
			ID int `path:"id"`
		}) (*struct{}, error) {
			if _, err := postgres.DB(adminapi.TxFrom(ctx)).Exec(ctx, `INSERT INTO control_plane.probe VALUES ($1)`, in.ID); err != nil {
				return nil, err
			}
			if in.ID < 0 {
				return nil, errs.ErrConflict
			}
			return nil, nil
		})
	huma.Register(api, huma.Operation{OperationID: "list-probes", Method: http.MethodGet, Path: "/admin/probes",
		Security: []map[string][]string{{"OperatorBearer": {"routes:read"}}}},
		func(context.Context, *struct{}) (*struct{}, error) { return nil, nil })
	huma.Register(api, huma.Operation{OperationID: "get-probe", Method: http.MethodGet, Path: "/admin/probes/{id}"},
		func(context.Context, *struct {
			ID int `path:"id"`
		}) (*struct{}, error) {
			return nil, nil
		})
	return gateway{h: h, k: k, migrator: migrator, uncommitted: uncommitted}
}

func (g gateway) call(t *testing.T, method, path, scope string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if scope != "" {
		req.Header.Set("Authorization", "Bearer "+g.k.Token(t, "v1", scope))
	}
	req.Header.Set("X-Request-Id", "forged")
	rec := httptest.NewRecorder()
	g.h.ServeHTTP(rec, req)
	return rec
}

func (g gateway) lines(t *testing.T) []line {
	t.Helper()
	rows, err := g.migrator.Query(context.Background(),
		`SELECT operator, operation_id, target, coalesce(request_id, ''), status FROM control_plane.audit_log ORDER BY at, id`)
	if err != nil {
		t.Fatal(err)
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (l line, err error) {
		err = r.Scan(&l.Operator, &l.OperationID, &l.Target, &l.RequestID, &l.Status)
		return l, err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func (g gateway) count(t *testing.T, query string) int {
	t.Helper()
	var n int
	if err := g.migrator.QueryRow(context.Background(), query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

type page struct {
	Data []struct {
		ID          string     `json:"id"`
		Operator    string     `json:"operator"`
		OperationID string     `json:"operation_id"`
		Target      string     `json:"target"`
		RequestID   *string    `json:"request_id"`
		Status      int        `json:"status"`
		Outcome     *string    `json:"outcome"`
		At          *time.Time `json:"at"`
	} `json:"data"`
	NextCursor *string `json:"next_cursor"`
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) page {
	t.Helper()
	var p page
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("%d %s: %v", rec.Code, rec.Body, err)
	}
	return p
}

func TestReadingTheAuditLogLeavesOneLineWrittenAfterTheRead(t *testing.T) {
	g := newGateway(t)

	start := time.Now()
	first := g.call(t, http.MethodGet, "/admin/audit-log", "audit:read")
	if first.Code != http.StatusOK || len(decode(t, first).Data) != 0 || strings.Contains(first.Body.String(), "$schema") ||
		first.Header().Get("Link") != "" {
		t.Fatalf("first read: %d %s", first.Code, first.Body)
	}
	id := first.Header().Get("X-Request-Id")
	if id == "" || id == "forged" {
		t.Fatalf("X-Request-Id = %q", id)
	}
	want := line{Operator: operator, OperationID: "list-audit-log", Target: "/admin/audit-log", RequestID: id, Status: 200}
	if got := g.lines(t); len(got) != 1 || got[0] != want {
		t.Fatalf("lines = %+v, want %+v", got, want)
	}

	second := g.call(t, http.MethodGet, "/admin/audit-log?limit=1", "audit:read")
	p := decode(t, second)
	if len(p.Data) != 1 || p.NextCursor != nil {
		t.Fatalf("second read: %s", second.Body)
	}
	e := p.Data[0]
	if e.Operator != operator || e.OperationID != "list-audit-log" || e.Target != "/admin/audit-log" || e.Status != 200 ||
		*e.RequestID != id || e.Outcome != nil || e.At == nil || e.At.Before(start.Add(-time.Second)) || e.At.After(time.Now()) || !strings.Contains(second.Body.String(), `"outcome":null`) {
		t.Fatalf("second read: %s", second.Body)
	}
	if got := g.lines(t); len(got) != 2 || got[1].Target != "/admin/audit-log" {
		t.Fatalf("the line of a query carries it: %+v", got)
	}
}

func TestTheAuditLogIsPagedAndFilteredOverHTTP(t *testing.T) {
	g := newGateway(t)
	for range 3 {
		g.call(t, http.MethodGet, "/admin/audit-log", "audit:read")
	}
	g.call(t, http.MethodGet, "/admin/audit-log", "routes:read")

	p := decode(t, g.call(t, http.MethodGet, "/admin/audit-log?limit=2&operator="+operator, "audit:read"))
	if len(p.Data) != 2 || p.NextCursor == nil {
		t.Fatalf("first page: %+v", p)
	}
	rest := decode(t, g.call(t, http.MethodGet, "/admin/audit-log?limit=10&operator="+operator+"&cursor="+*p.NextCursor, "audit:read"))
	if len(rest.Data) != 2 || rest.NextCursor != nil || rest.Data[0].ID == p.Data[1].ID {
		t.Fatalf("second page: %+v", rest)
	}
	if none := decode(t, g.call(t, http.MethodGet, "/admin/audit-log?operator=nobody", "audit:read")); len(none.Data) != 0 || none.Data == nil {
		t.Fatalf("nobody: %+v", none)
	}
	for query, want := range map[string]bool{"from_date=2099-01-01T00:00:00Z": false, "to_date=2000-01-01T00:00:00Z": false,
		"from_date=2000-01-01T00:00:00Z&to_date=2099-01-01T00:00:00Z": true} {
		if rec := g.call(t, http.MethodGet, "/admin/audit-log?"+query, "audit:read"); rec.Code != http.StatusOK || (len(decode(t, rec).Data) > 0) != want {
			t.Errorf("%s: %d %s", query, rec.Code, rec.Body)
		}
	}
	for _, query := range []string{"cursor=nope", "limit=0", "limit=501", "from_date=yesterday", "unknown=1"} {
		if rec := g.call(t, http.MethodGet, "/admin/audit-log?"+query, "audit:read"); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"validation_error"`) ||
			query != "cursor=nope" && !strings.Contains(rec.Body.String(), `"field":"query.`) {
			t.Errorf("%s: %d %s", query, rec.Code, rec.Body)
		}
	}
}

func TestAnUnauthenticatedCallLeavesNoLineAndAForbiddenOneLeavesItsRefusal(t *testing.T) {
	g := newGateway(t)

	for _, auth := range []string{"", "Bearer nope", "Basic " + g.k.Token(t, "v1", "audit:read"), g.k.Token(t, "v1", "audit:read")} {
		req := httptest.NewRequest(http.MethodGet, "/admin/audit-log", nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		rec := httptest.NewRecorder()
		g.h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), `"unauthenticated"`) {
			t.Errorf("%q: %d %s", auth, rec.Code, rec.Body)
		}
	}
	if got := g.lines(t); len(got) != 0 {
		t.Fatalf("lines after 401: %+v", got)
	}

	if rec := g.call(t, http.MethodGet, "/admin/audit-log", "routes:read"); rec.Code != http.StatusForbidden {
		t.Fatalf("without audit:read: %d", rec.Code)
	}
	if rec := g.call(t, http.MethodPost, "/admin/probes/1", "routes:read"); rec.Code != http.StatusForbidden {
		t.Fatalf("write without routes:write: %d", rec.Code)
	}
	got := g.lines(t)
	if len(got) != 2 || got[0].OperationID != "list-audit-log" || got[0].Status != 403 || got[1].OperationID != "create-probe" ||
		got[1].Status != 403 || got[1].Target != "/admin/probes/1" || got[1].Operator != operator {
		t.Fatalf("lines = %+v", got)
	}
	if n := g.count(t, `SELECT count(*) FROM control_plane.probe`); n != 0 {
		t.Fatalf("%d probes written without the scope", n)
	}
}

func TestAMutationCommitsWithItsLineUnderTheOperator(t *testing.T) {
	g := newGateway(t)

	rec := g.call(t, http.MethodPost, "/admin/probes/7", "routes:write")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	got := g.lines(t)
	if len(got) != 1 || got[0].Status != 204 || got[0].RequestID != rec.Header().Get("X-Request-Id") {
		t.Fatalf("lines = %+v", got)
	}
	if n := g.count(t, `SELECT count(*) FROM control_plane.audit_changes WHERE actor = '`+operator+`' AND request_id = '`+got[0].RequestID+`'`); n != 1 {
		t.Fatalf("%d captured changes under the operator and the request", n)
	}
	if n := testutil.ToFloat64(g.uncommitted); n != 0 {
		t.Fatalf("a committed call counted %v uncommitted", n)
	}
}

func TestARefusedMutationRollsBackAndLeavesItsRefusal(t *testing.T) {
	g := newGateway(t)

	rec := g.call(t, http.MethodPost, "/admin/probes/-1", "routes:write")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"conflict"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if n := g.count(t, `SELECT count(*) FROM control_plane.probe`) + g.count(t, `SELECT count(*) FROM control_plane.audit_changes`); n != 0 {
		t.Fatalf("%d rows survived the refusal", n)
	}
	if got := g.lines(t); len(got) != 1 || got[0].Status != 409 || got[0].OperationID != "create-probe" {
		t.Fatalf("lines = %+v", got)
	}
}

func TestTheChangesOfAnUnknownRequestAreNotFoundAndTheRefusalIsAudited(t *testing.T) {
	g := newGateway(t)
	write := g.call(t, http.MethodPost, "/admin/probes/7", "routes:write")

	rec := g.call(t, http.MethodGet, "/admin/audit-log/unknown/changes", "audit:read")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"not_found"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if got := g.lines(t); len(got) != 2 || got[1].Status != 404 || got[1].Target != "/admin/audit-log/unknown/changes" {
		t.Fatalf("lines = %+v", got)
	}

	rec = g.call(t, http.MethodGet, "/admin/audit-log/"+write.Header().Get("X-Request-Id")+"/changes", "audit:read")
	var changes struct {
		Data []struct {
			Actor  string          `json:"actor"`
			Table  string          `json:"table"`
			PK     string          `json:"pk"`
			Op     string          `json:"op"`
			Before json.RawMessage `json:"before"`
			After  json.RawMessage `json:"after"`
			At     *time.Time      `json:"at"`
		} `json:"data"`
		NextCursor *string `json:"next_cursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &changes); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	c := changes.Data
	if len(c) != 1 || c[0].Actor != operator || c[0].Table != "probe" || c[0].PK != `{"id": 7}` || c[0].Op != "insert" ||
		string(c[0].Before) != "null" || string(c[0].After) != `{"id":7}` || c[0].At == nil || c[0].At.Year() < 2026 || changes.NextCursor != nil {
		t.Fatalf("changes = %s", rec.Body)
	}
}

func TestAnAuditLineThatCannotBeWrittenFailsTheCall(t *testing.T) {
	g := newGateway(t)
	if _, err := g.migrator.Exec(context.Background(), `REVOKE INSERT ON control_plane.audit_log FROM admin_api_svc`); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct{ method, path, scope string }{
		{http.MethodGet, "/admin/audit-log", "audit:read"},
		{http.MethodPost, "/admin/probes/7", "routes:write"},
		{http.MethodPost, "/admin/probes/-1", "routes:write"},
		{http.MethodPost, "/admin/probes/7", "routes:read"},
	} {
		rec := g.call(t, c.method, c.path, c.scope)
		if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"service_unavailable"`) {
			t.Errorf("%s %s under %s: %d %s", c.method, c.path, c.scope, rec.Code, rec.Body)
		}
	}
	if n := g.count(t, `SELECT count(*) FROM control_plane.probe`); n != 0 {
		t.Fatalf("%d probes written without their line", n)
	}
	if n := testutil.ToFloat64(g.uncommitted); n != 4 {
		t.Fatalf("uncommitted = %v, want 4", n)
	}
}

func TestAnOrdinaryReadLeavesNoLine(t *testing.T) {
	g := newGateway(t)

	if rec := g.call(t, http.MethodGet, "/admin/probes", "routes:read"); rec.Code != http.StatusNoContent {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if got := g.lines(t); len(got) != 0 {
		t.Fatalf("lines = %+v", got)
	}
}

func TestAnOperationWithoutItsScopeIsRefusedToEveryToken(t *testing.T) {
	g := newGateway(t)

	rec := g.call(t, http.MethodGet, "/admin/probes/1", strings.Join([]string{"audit:read", "routes:read", "routes:write"}, " "))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestARefusalIsAuditedEvenWhenTheCallerHasGone(t *testing.T) {
	g := newGateway(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/admin/probes/1", nil)
	req.Header.Set("Authorization", "Bearer "+g.k.Token(t, "v1", "routes:read"))
	g.h.ServeHTTP(httptest.NewRecorder(), req)

	if got := g.lines(t); len(got) != 1 || got[0].Status != 403 {
		t.Fatalf("lines = %+v", got)
	}
}

func TestAnUnexpectedErrorIsAuditedAndTellsNothingOfItsCause(t *testing.T) {
	g := newGateway(t)
	g.call(t, http.MethodPost, "/admin/probes/7", "routes:write")

	rec := g.call(t, http.MethodPost, "/admin/probes/7", "routes:write")
	if rec.Code != http.StatusInternalServerError || rec.Header().Get("Content-Type") != "application/json" ||
		!strings.Contains(rec.Body.String(), `"internal_error"`) || strings.Contains(rec.Body.String(), "probe") {
		t.Fatalf("%d %s %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body)
	}
	if got := g.lines(t); len(got) != 2 || got[1].Status != 500 {
		t.Fatalf("lines = %+v", got)
	}
}
