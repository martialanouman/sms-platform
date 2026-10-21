package http_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	dashboardhttp "github.com/martialanouman/sms-platform/internal/dashboard/http"
	"github.com/martialanouman/sms-platform/internal/platform/mtls/mtlstest"
)

type relayed struct {
	method, path, query, body string
	header                    http.Header
}

type gateway struct {
	srv       *httptest.Server
	url       *url.URL
	transport http.RoundTripper
	mu        sync.Mutex
	received  []relayed
	status    int
	answer    string
}

func newGateway(t *testing.T) *gateway {
	t.Helper()
	ca := mtlstest.New(t)
	serverTLS, err := ca.Issue(t, t.TempDir(), "localhost").Server([]string{"dashboard"}, []string{"http/1.1"})
	if err != nil {
		t.Fatal(err)
	}
	clientTLS, err := ca.Issue(t, t.TempDir(), "dashboard").Client()
	if err != nil {
		t.Fatal(err)
	}
	g := &gateway{status: http.StatusOK, answer: `{"data":[],"next_cursor":null}`, transport: dashboardhttp.GatewayTransport(clientTLS)}
	g.srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		g.mu.Lock()
		g.received = append(g.received, relayed{r.Method, r.URL.Path, r.URL.RawQuery, string(body), r.Header.Clone()})
		status, answer := g.status, g.answer
		g.mu.Unlock()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Request-Id", "01a16026-a1b2-7c44-afc6-4791c68f65e8")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, answer)
	}))
	g.srv.TLS = serverTLS
	g.srv.StartTLS()
	t.Cleanup(g.srv.Close)
	_, port, _ := net.SplitHostPort(g.srv.Listener.Addr().String())
	g.url = &url.URL{Scheme: "https", Host: "localhost:" + port}
	return g
}

func (g *gateway) calls() []relayed {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]relayed(nil), g.received...)
}

func (g *gateway) answers(status int, body string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.status, g.answer = status, body
}

func claimsOf(t *testing.T, header http.Header) map[string]any {
	t.Helper()
	token, found := strings.CutPrefix(header.Get("Authorization"), "Bearer ")
	parts := strings.Split(token, ".")
	if !found || len(parts) != 3 {
		t.Fatalf("Authorization %q", header.Get("Authorization"))
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var c map[string]any
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestTheProxyRelaysTheAdminAPIUnderTheOperatorsToken(t *testing.T) {
	a := newAPI(t, netip.MustParsePrefix("127.0.0.0/8"))
	a.activate(t)
	c := a.elevated(t)
	a.gateway.answers(http.StatusForbidden, `{"code":"forbidden_scope","message":"the operator token lacks audit:read"}`)

	got := a.do(t, call{method: "GET", path: "/api/gw/admin/audit-log?limit=2&operator=x", cookie: c, forwarded: "203.0.113.9",
		authorization: "Bearer forged"})
	if got.status != 403 || got.raw != `{"code":"forbidden_scope","message":"the operator token lacks audit:read"}` ||
		got.header.Get("X-Request-Id") != "01a16026-a1b2-7c44-afc6-4791c68f65e8" || got.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %s %v", got.status, got.raw, got.header)
	}
	calls := a.gateway.calls()
	if len(calls) != 1 {
		t.Fatalf("%d calls", len(calls))
	}
	r := calls[0]
	if len(r.header.Values("Authorization")) != 1 || r.header.Get("Authorization") == "Bearer forged" {
		t.Fatalf("Authorization relayed %v", r.header.Values("Authorization"))
	}
	if r.method != "GET" || r.path != "/admin/audit-log" || r.query != "limit=2&operator=x" || r.header.Get("Cookie") != "" ||
		r.header.Get("X-Forwarded-For") != "" || r.header.Get("X-Forwarded-Host") != "" || r.header.Get("X-Forwarded-Proto") != "" {
		t.Fatalf("relayed %+v", r)
	}
	claims := claimsOf(t, r.header)
	var operator string
	if err := a.owner.QueryRow(t.Context(), `SELECT id FROM dashboard.operators`).Scan(&operator); err != nil {
		t.Fatal(err)
	}
	if claims["sub"] != operator || claims["exp"].(float64)-claims["iat"].(float64) != 300 || !strings.Contains(claims["scope"].(string), "audit:read") ||
		!strings.Contains(claims["scope"].(string), "content:read") || strings.Contains(claims["scope"].(string), "operators:") {
		t.Fatalf("claims %v", claims)
	}
}

func TestTheProxyRefusesBeforeRelaying(t *testing.T) {
	a := newAPI(t, netip.MustParsePrefix("127.0.0.0/8"))
	a.activate(t)
	elevated := a.elevated(t)
	notElevated := a.signIn(t).cookie
	signed := a.signer.signs.Load()
	for name, tc := range map[string]struct {
		c      call
		status int
		code   string
	}{
		"no session":                       {call{method: "GET", path: "/api/gw/admin/audit-log"}, 401, "unauthenticated"},
		"off the contract without session": {call{method: "GET", path: "/api/gw/admin/nope"}, 404, "not_found"},
		"no origin without session":        {call{method: "POST", path: "/api/gw/admin/routes", body: `{}`, origin: "-"}, 403, "forbidden_origin"},
		"text body without session":        {call{method: "POST", path: "/api/gw/admin/routes", body: `{}`, contentType: "text/plain"}, 415, "unsupported_media_type"},
		"a dot segment":                    {call{method: "GET", path: "/api/gw/admin/customers/%2e%2e", cookie: elevated}, 404, "not_found"},
		"not elevated":                     {call{method: "GET", path: "/api/gw/admin/audit-log", cookie: notElevated}, 403, "mfa_required"},
		"no origin":                        {call{method: "POST", path: "/api/gw/admin/routes", cookie: elevated, body: `{}`, origin: "-"}, 403, "forbidden_origin"},
		"other origin":                     {call{method: "POST", path: "/api/gw/admin/routes", cookie: elevated, body: `{}`, origin: "https://evil.example"}, 403, "forbidden_origin"},
		"text body":                        {call{method: "POST", path: "/api/gw/admin/routes", cookie: elevated, body: `{}`, contentType: "text/plain"}, 415, "unsupported_media_type"},
		"json to a csv import":             {call{method: "POST", path: "/api/gw/admin/exact-routes/import", cookie: elevated, body: `{}`}, 415, "unsupported_media_type"},
		"bodiless untyped":                 {call{method: "POST", path: "/api/gw/admin/customers/0199f0a2-7c1e-7000-8000-000000000001/suspend", cookie: elevated, contentType: "-"}, 415, "unsupported_media_type"},
		"path off the contract":            {call{method: "GET", path: "/api/gw/admin/nope", cookie: elevated}, 404, "not_found"},
		"method off the contract":          {call{method: "DELETE", path: "/api/gw/admin/audit-log", cookie: elevated}, 404, "not_found"},
		"a stream":                         {call{method: "GET", path: "/api/gw/admin/stream/metrics", cookie: elevated}, 404, "not_found"},
		"outside /admin":                   {call{method: "GET", path: "/api/gw/openapi.json", cookie: elevated}, 404, "not_found"},
	} {
		got := a.do(t, tc.c)
		if got.status != tc.status || got.code() != tc.code {
			t.Errorf("%s: %d %s", name, got.status, got.raw)
		}
	}
	if calls := a.gateway.calls(); len(calls) != 0 {
		t.Fatalf("relayed %d refused calls: %+v", len(calls), calls)
	}
	if n := a.signer.signs.Load(); n != signed {
		t.Fatalf("%d tokens signed for refused calls", n-signed)
	}
}

func TestTheProxyRelaysTheMediaOfTheOperationUncapped(t *testing.T) {
	a := newAPI(t, netip.MustParsePrefix("127.0.0.0/8"))
	a.activate(t)
	c := a.elevated(t)
	csv := strings.Repeat("33600000000,route,0199f0a2-7c1e-7000-8000-000000000001\n", 400)

	got := a.do(t, call{method: "POST", path: "/api/gw/admin/exact-routes/import", cookie: c, body: csv, contentType: "text/csv"})
	calls := a.gateway.calls()
	if got.raw != `{"data":[],"next_cursor":null}` || got.header.Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("answer %q %v", got.raw, got.header)
	}
	if got.status != 200 || len(calls) != 1 || calls[0].body != csv || calls[0].header.Get("Content-Type") != "text/csv" {
		t.Fatalf("%d %s, relayed %d", got.status, got.raw, len(calls))
	}
	if strings.Contains(a.logs.String(), "33600000000") {
		t.Fatalf("the relayed body reached the log: %s", a.logs.String())
	}
}

func TestAReportingOperatorIsRelayedAndTheGatewayDecides(t *testing.T) {
	a := newAPI(t, netip.MustParsePrefix("127.0.0.0/8"))
	a.activate(t)
	c := a.signIn(t).cookie
	a.holdOnly(t, "Reporting")

	if got := a.do(t, call{method: "GET", path: "/api/gw/admin/audit-log", cookie: c}); got.status != 200 {
		t.Fatalf("%d %s", got.status, got.raw)
	}
	calls := a.gateway.calls()
	if len(calls) != 1 || claimsOf(t, calls[0].header)["scope"] != "billing:read stream:read traffic:read" {
		t.Fatalf("relayed %+v", calls)
	}
}

func TestAGatewayOutOfReachIsServiceUnavailableAndLogsNoBody(t *testing.T) {
	a := newAPI(t, netip.MustParsePrefix("127.0.0.0/8"))
	a.activate(t)
	c := a.elevated(t)

	a.signer.fail = io.ErrUnexpectedEOF
	if got := a.do(t, call{method: "GET", path: "/api/gw/admin/audit-log", cookie: c}); got.status != 503 || got.code() != "service_unavailable" {
		t.Fatalf("signer down: %d %s", got.status, got.raw)
	}
	if calls := a.gateway.calls(); len(calls) != 0 {
		t.Fatalf("relayed without a token: %+v", calls)
	}
	a.signer.fail = nil

	a.gateway.srv.Close()
	got := a.do(t, call{method: "POST", path: "/api/gw/admin/routes", cookie: c, body: `{"name":"secret-route-body"}`})
	if got.status != 503 || got.code() != "service_unavailable" {
		t.Fatalf("%d %s", got.status, got.raw)
	}
	if strings.Contains(a.logs.String(), "secret-route-body") {
		t.Fatalf("the relayed body reached the log: %s", a.logs.String())
	}
}

func TestAGatewayThatNeverHandsShakesIsServiceUnavailableWithinItsBound(t *testing.T) {
	a := newAPI(t, netip.MustParsePrefix("127.0.0.0/8"))
	a.activate(t)
	c := a.elevated(t)
	silent, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = silent.Close() })
	go func() {
		for {
			conn, err := silent.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = conn.Close() })
		}
	}()
	a.gateway.url.Host = silent.Addr().String()

	start := time.Now()
	got := a.do(t, call{method: "GET", path: "/api/gw/admin/audit-log", cookie: c})
	if got.status != 503 || time.Since(start) > 15*time.Second {
		t.Fatalf("%d after %s", got.status, time.Since(start))
	}
}

func TestACallerThatHasGoneLeavesNoUnreachableLine(t *testing.T) {
	a := newAPI(t, netip.MustParsePrefix("127.0.0.0/8"))
	a.activate(t)
	c := a.elevated(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := httptest.NewRequestWithContext(ctx, "GET", "/api/gw/admin/audit-log", nil)
	r.Header.Set("Cookie", "__Host-dashboard_session="+c)
	a.handler.ServeHTTP(httptest.NewRecorder(), r)

	if strings.Contains(a.logs.String(), "admin api unreachable") {
		t.Fatalf("a caller gone was logged as the gateway out of reach: %s", a.logs.String())
	}
}
