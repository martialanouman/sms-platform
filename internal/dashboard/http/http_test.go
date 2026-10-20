package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/martialanouman/sms-platform/internal/dashboard"
	dashboardhttp "github.com/martialanouman/sms-platform/internal/dashboard/http"
	"github.com/martialanouman/sms-platform/internal/dashboard/postgres"
	"github.com/martialanouman/sms-platform/internal/platform/pgtest"
	"github.com/martialanouman/sms-platform/migrations"
)

const origin = "http://localhost:8080"

var spa = fstest.MapFS{
	"index.html":           {Data: []byte(`<!doctype html><html lang="fr"><title>Tableau de bord</title></html>`)},
	"assets/index-Ab12.js": {Data: []byte(`console.log(1)`)},
	"assets/sub/x.js":      {Data: []byte(`x`)},
}

type mailbox struct {
	mu   sync.Mutex
	body string
}

func (m *mailbox) Send(_ context.Context, _, _, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.body = body
	return nil
}

var linkToken = regexp.MustCompile(`/access#([A-Za-z0-9_-]{43})\b`)

type api struct {
	svc     dashboard.Service
	mail    *mailbox
	handler http.Handler
	logins  int
}

func newAPI(t *testing.T, trusted ...netip.Prefix) *api {
	t.Helper()
	ctx := context.Background()
	pg := pgtest.ControlPlane(t).In("dashboard")
	migrator, err := pgx.Connect(ctx, pg.As("migrator"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = migrator.Close(ctx) })
	for _, s := range []string{string(migrations.Grants("dashboard")), `ALTER ROLE dashboard PASSWORD 'dashboard'`} {
		if _, err := migrator.Exec(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	pool, err := pgxpool.New(ctx, pg.As("dashboard"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	keys, err := dashboard.NewKeys(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	mail := &mailbox{}
	totp, err := dashboard.NewTotpKey(bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	svc := dashboard.Service{Store: postgres.New(pool), Hasher: dashboard.NewHasher(), Keys: keys, Totp: totp, Mailer: mail, PublicURL: origin}
	h, err := dashboardhttp.New(dashboardhttp.Config{Service: svc, Origin: origin, TrustedProxies: trusted, Log: slog.New(slog.DiscardHandler), Assets: spa})
	if err != nil {
		t.Fatal(err)
	}
	return &api{svc: svc, mail: mail, handler: h}
}

type call struct {
	method, path, body, contentType, origin, cookie, forwarded string
	size                                                       int
}

type answer struct {
	status  int
	body    map[string]any
	raw     string
	cookies []*http.Cookie
	header  http.Header
}

func (a *api) do(t *testing.T, c call) answer {
	t.Helper()
	var body io.Reader = strings.NewReader(c.body)
	if c.size > 0 {
		body = bytes.NewReader(bytes.Repeat([]byte(" "), c.size))
	}
	r := httptest.NewRequest(c.method, c.path, body)
	r.RemoteAddr = "127.0.0.1:41000"
	if c.method != http.MethodGet {
		if c.origin == "" {
			c.origin = origin
		}
		if c.origin != "-" {
			r.Header.Set("Origin", c.origin)
		}
		if c.contentType == "" {
			c.contentType = "application/json"
		}
		if c.contentType != "-" {
			r.Header.Set("Content-Type", c.contentType)
		}
	}
	if c.cookie != "" {
		r.Header.Set("Cookie", "__Host-dashboard_session="+c.cookie)
	}
	if c.forwarded != "" {
		r.Header.Set("X-Forwarded-For", c.forwarded)
	}
	w := httptest.NewRecorder()
	a.handler.ServeHTTP(w, r)
	got := answer{status: w.Code, raw: w.Body.String(), cookies: w.Result().Cookies(), header: w.Header()}
	_ = json.Unmarshal(w.Body.Bytes(), &got.body)
	return got
}

func (a answer) code() string {
	s, _ := a.body["code"].(string)
	return s
}

func (a *api) activate(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if err := a.svc.Bootstrap(ctx, "owner@example.test"); err != nil {
		t.Fatal(err)
	}
	if sent, err := a.svc.DeliverNext(ctx); err != nil || !sent {
		t.Fatalf("deliver: %v, %v", sent, err)
	}
	token := linkToken.FindStringSubmatch(a.mail.body)[1]
	if got := a.do(t, call{method: "POST", path: "/api/auth/access-link", body: `{"token":"` + token + `","password":"Correct-horse-9"}`}); got.status != 204 {
		t.Fatalf("access link: %d %s", got.status, got.raw)
	}
}

func TestHealthAnswersWithoutSession(t *testing.T) {
	a := newAPI(t)
	if got := a.do(t, call{method: "GET", path: "/api/health"}); got.status != 200 || got.body["status"] != "ok" {
		t.Fatalf("%d %s", got.status, got.raw)
	}
}

func TestEveryUnsafeRequestIsCheckedBeforeTheOperation(t *testing.T) {
	a := newAPI(t)
	login := `{"email":"owner@example.test","password":"x"}`
	for name, tc := range map[string]struct {
		c      call
		status int
		code   string
	}{
		"no origin":          {call{method: "POST", path: "/api/auth/login", body: login, origin: "-"}, 403, "forbidden_origin"},
		"other origin":       {call{method: "POST", path: "/api/auth/login", body: login, origin: "https://evil.example"}, 403, "forbidden_origin"},
		"logout cross-site":  {call{method: "POST", path: "/api/auth/logout", origin: "https://evil.example"}, 403, "forbidden_origin"},
		"text body":          {call{method: "POST", path: "/api/auth/login", body: login, contentType: "text/plain"}, 415, "unsupported_media_type"},
		"activity untyped":   {call{method: "POST", path: "/api/auth/activity", contentType: "-"}, 415, "unsupported_media_type"},
		"body too large":     {call{method: "POST", path: "/api/auth/login", size: 17 << 10}, 413, "payload_too_large"},
		"missing password":   {call{method: "POST", path: "/api/auth/login", body: `{"email":"owner@example.test"}`}, 422, "validation_error"},
		"unknown field":      {call{method: "POST", path: "/api/auth/login", body: `{"email":"o@example.test","password":"x","extra":1}`}, 422, "validation_error"},
		"json with charset":  {call{method: "POST", path: "/api/auth/login", body: login, contentType: "application/json; charset=utf-8"}, 401, "unauthenticated"},
		"not served":         {call{method: "GET", path: "/api/operators"}, 404, "not_found"},
		"wrong method":       {call{method: "GET", path: "/api/auth/login"}, 404, "not_found"},
		"me without session": {call{method: "GET", path: "/api/auth/me"}, 401, "unauthenticated"},
		"me with a forgery":  {call{method: "GET", path: "/api/auth/me", cookie: "x.y"}, 401, "unauthenticated"},
	} {
		got := a.do(t, tc.c)
		if got.status != tc.status || got.code() != tc.code || got.body["message"] == "" {
			t.Errorf("%s: %d %s, want %d %s", name, got.status, got.raw, tc.status, tc.code)
		}
	}
}

func TestAValidationErrorPlacesItsFieldsButNeverEchoesThePassword(t *testing.T) {
	a := newAPI(t)
	secret := "Secret-" + strings.Repeat("p", 4100)
	got := a.do(t, call{method: "POST", path: "/api/auth/login", body: `{"email":"owner@example.test","password":"` + secret + `"}`})
	if got.status != 422 || got.code() != "validation_error" || strings.Contains(got.raw, "Secret-ppp") {
		t.Fatalf("%d %s", got.status, got.raw)
	}
	errs, _ := got.body["errors"].([]any)
	first, _ := errs[0].(map[string]any)
	if len(errs) != 1 || first["field"] != "password" || !strings.HasPrefix(first["message"].(string), "maxLength") {
		t.Fatalf("errors[] in %s, want the password field and the reason alone", got.raw)
	}
}

func TestAnOperatorActivatesLogsInAndOut(t *testing.T) {
	a := newAPI(t)
	ctx := context.Background()
	if err := a.svc.Bootstrap(ctx, "owner@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.svc.DeliverNext(ctx); err != nil {
		t.Fatal(err)
	}
	token := linkToken.FindStringSubmatch(a.mail.body)[1]
	link := func(token, password string) answer {
		return a.do(t, call{method: "POST", path: "/api/auth/access-link", body: `{"token":"` + token + `","password":"` + password + `"}`})
	}
	if got := link(token, "short"); got.status != 422 || got.code() != "password_policy" || !strings.Contains(got.raw, "majuscule") {
		t.Fatalf("weak password: %d %s", got.status, got.raw)
	}
	if got := link(token, "Correct-horse-9"); got.status != 204 {
		t.Fatalf("access link: %d %s", got.status, got.raw)
	}
	if got := link(token, "Correct-horse-9"); got.status != 410 || got.code() != "access_link_invalid" {
		t.Fatalf("spent link: %d %s", got.status, got.raw)
	}

	got := a.do(t, call{method: "POST", path: "/api/auth/login", body: `{"email":"Owner@example.test","password":"Correct-horse-9"}`})
	if got.status != 200 || len(got.cookies) != 1 {
		t.Fatalf("login: %d %s", got.status, got.raw)
	}
	c := got.cookies[0]
	if c.Name != "__Host-dashboard_session" || c.Path != "/" || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.MaxAge != 0 || c.Domain != "" {
		t.Errorf("cookie %+v", c)
	}
	if challenge, _ := got.body["challenge"].(string); len(challenge) < 43 || got.body["expiresAt"] == nil {
		t.Errorf("challenge %s", got.raw)
	}

	me := a.do(t, call{method: "GET", path: "/api/auth/me", cookie: c.Value})
	operator, _ := me.body["operator"].(map[string]any)
	permissions, _ := me.body["permissions"].([]any)
	if me.status != 200 || operator["email"] != "owner@example.test" || operator["displayName"] != "owner@example.test" || operator["id"] == "" ||
		permissions == nil || len(permissions) != 0 || me.body["elevated"] != false || me.body["idleTimeoutSeconds"] != float64(900) ||
		!within12h(me.body["absoluteExpiresAt"]) || me.body["secondFactors"] == nil {
		t.Errorf("me: %d %s", me.status, me.raw)
	}
	if got := a.do(t, call{method: "POST", path: "/api/auth/activity", cookie: c.Value}); got.status != 204 {
		t.Errorf("activity: %d %s", got.status, got.raw)
	}
	if got := a.do(t, call{method: "GET", path: "/api/access-log", cookie: c.Value}); got.status != 403 || got.code() != "forbidden_scope" || !strings.Contains(got.raw, "audit:read") {
		t.Errorf("access log without audit:read: %d %s", got.status, got.raw)
	}
	out := a.do(t, call{method: "POST", path: "/api/auth/logout", cookie: c.Value})
	if out.status != 204 || len(out.cookies) != 1 || out.cookies[0].MaxAge >= 0 || out.cookies[0].Name != "__Host-dashboard_session" {
		t.Errorf("logout: %d %v", out.status, out.cookies)
	}
	if got := a.do(t, call{method: "GET", path: "/api/auth/me", cookie: c.Value}); got.status != 401 {
		t.Errorf("me after logout: %d", got.status)
	}
	if got := a.do(t, call{method: "POST", path: "/api/auth/activity", cookie: c.Value}); got.status != 401 || got.code() != "unauthenticated" {
		t.Errorf("activity after logout: %d %s", got.status, got.raw)
	}
	if got := a.do(t, call{method: "POST", path: "/api/auth/logout"}); got.status != 204 {
		t.Errorf("logout without session: %d", got.status)
	}
}

func TestLoginRefusals(t *testing.T) {
	a := newAPI(t)
	a.activate(t)
	wrong := call{method: "POST", path: "/api/auth/login", body: `{"email":"owner@example.test","password":"Wrong-horse-9"}`}
	if got := a.do(t, wrong); got.status != 401 || got.code() != "unauthenticated" {
		t.Fatalf("wrong password: %d %s", got.status, got.raw)
	}
	for range 4 {
		a.do(t, wrong)
	}
	got := a.do(t, wrong)
	retry, err := strconv.Atoi(got.header.Get("Retry-After"))
	if got.status != 429 || got.code() != "rate_limited" || err != nil || retry < 1 || retry > 900 {
		t.Fatalf("sixth attempt: %d %s Retry-After %q", got.status, got.raw, got.header.Get("Retry-After"))
	}
}

func TestTheSourceIsForwardedOnlyByATrustedProxy(t *testing.T) {
	for name, tc := range map[string]struct {
		trusted []netip.Prefix
		locked  bool
	}{
		"trusted":   {[]netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}, false},
		"untrusted": {nil, true},
	} {
		t.Run(name, func(t *testing.T) {
			a := newAPI(t, tc.trusted...)
			for i := range 5 {
				a.do(t, call{method: "POST", path: "/api/auth/login", forwarded: "198.51.100.1",
					body: `{"email":"x` + strconv.Itoa(i) + `@example.test","password":"x"}`})
			}
			got := a.do(t, call{method: "POST", path: "/api/auth/login", forwarded: "198.51.100.1, 198.51.100.2",
				body: `{"email":"y@example.test","password":"x"}`})
			if (got.status == 429) != tc.locked {
				t.Fatalf("another forwarded source: %d %s, want locked %v", got.status, got.raw, tc.locked)
			}
		})
	}
}

func within12h(v any) bool {
	s, _ := v.(string)
	at, err := time.Parse(time.RFC3339, s)
	return err == nil && time.Until(at) > 11*time.Hour && time.Until(at) <= 12*time.Hour
}

func TestABusyHasherAnswers503(t *testing.T) {
	a := newAPI(t, netip.MustParsePrefix("127.0.0.0/8"))
	start := make(chan struct{})
	statuses := make(chan answer, 40)
	var wg sync.WaitGroup
	for i := range 40 {
		wg.Go(func() {
			<-start
			statuses <- a.do(t, call{method: "POST", path: "/api/auth/login", forwarded: "198.51.100." + strconv.Itoa(i+1),
				body: `{"email":"nobody` + strconv.Itoa(i) + `@example.test","password":"x"}`})
		})
	}
	close(start)
	wg.Wait()
	close(statuses)
	busy := 0
	for got := range statuses {
		switch {
		case got.status == 503 && got.code() == "service_unavailable":
			busy++
		case got.status != 401:
			t.Errorf("%d %s", got.status, got.raw)
		}
	}
	if busy == 0 {
		t.Fatal("forty simultaneous logins on ten hashing slots: none refused")
	}
}

func TestA422NamesTheParameterOrTheBody(t *testing.T) {
	a := newAPI(t)
	for name, tc := range map[string]struct {
		c     call
		field string
	}{
		"cursor too long": {call{method: "GET", path: "/api/access-log?cursor=" + strings.Repeat("a", 257)}, "cursor"},
		"body not JSON":   {call{method: "POST", path: "/api/auth/login", body: `{"email":`}, "body"},
	} {
		got := a.do(t, tc.c)
		errs, _ := got.body["errors"].([]any)
		first := map[string]any{}
		if len(errs) == 1 {
			first, _ = errs[0].(map[string]any)
		}
		if got.status != 422 || got.code() != "validation_error" || first["field"] != tc.field {
			t.Errorf("%s: %d %s, want 422 on %s", name, got.status, got.raw, tc.field)
		}
	}
}

func TestTheSourceSkipsEveryTrustedHopAndNeverVanishes(t *testing.T) {
	a := newAPI(t, netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8"))
	for i := range 5 {
		a.do(t, call{method: "POST", path: "/api/auth/login", forwarded: "198.51.100.1, 10.0.0.2",
			body: `{"email":"x` + strconv.Itoa(i) + `@example.test","password":"x"}`})
	}
	if got := a.do(t, call{method: "POST", path: "/api/auth/login", forwarded: "198.51.100.1, 10.0.0.3",
		body: `{"email":"y@example.test","password":"x"}`}); got.status != 429 {
		t.Errorf("the client behind two proxies: %d %s, want its own count locked", got.status, got.raw)
	}
	if got := a.do(t, call{method: "POST", path: "/api/auth/login", forwarded: "198.51.100.9, 10.0.0.3",
		body: `{"email":"y@example.test","password":"x"}`}); got.status != 401 {
		t.Errorf("another client behind the same proxies: %d %s, want its own count", got.status, got.raw)
	}
	if got := a.do(t, call{method: "POST", path: "/api/auth/login", body: `{"email":"z@example.test","password":"x"}`}); got.status != 401 {
		t.Errorf("a trusted proxy without X-Forwarded-For: %d %s, want the proxy as the source", got.status, got.raw)
	}
}

func TestTheSPAIsServedWithItsCachePolicy(t *testing.T) {
	a := newAPI(t)
	for name, tc := range map[string]struct {
		c           call
		status      int
		body, cache string
	}{
		"root":              {call{method: "GET", path: "/"}, 200, "Tableau de bord", "no-cache"},
		"a route":           {call{method: "GET", path: "/login"}, 200, "Tableau de bord", "no-cache"},
		"a deep route":      {call{method: "GET", path: "/access/x"}, 200, "Tableau de bord", "no-cache"},
		"a hashed asset":    {call{method: "GET", path: "/assets/index-Ab12.js"}, 200, "console.log(1)", "public, max-age=31536000, immutable"},
		"a missing asset":   {call{method: "GET", path: "/assets/gone.js"}, 404, "", ""},
		"an asset folder":   {call{method: "GET", path: "/assets/sub"}, 404, "", ""},
		"head":              {call{method: "HEAD", path: "/"}, 200, "", "no-cache"},
		"the shell by name": {call{method: "GET", path: "/index.html"}, 301, "", "no-cache"},
		"post":              {call{method: "POST", path: "/"}, 405, "", ""},
	} {
		got := a.do(t, tc.c)
		if got.status != tc.status || !strings.Contains(got.raw, tc.body) || got.header.Get("Cache-Control") != tc.cache ||
			(tc.status == 405) != (got.header.Get("Allow") == "GET, HEAD") || (tc.status == 404 && strings.Contains(got.raw, "Tableau de bord")) {
			t.Errorf("%s: %d %q cache %q, want %d %q cache %q", name, got.status, got.raw, got.header.Get("Cache-Control"), tc.status, tc.body, tc.cache)
		}
	}
}

func TestEveryResponseCarriesTheSecurityHeaders(t *testing.T) {
	a := newAPI(t)
	for _, c := range []call{{method: "GET", path: "/"}, {method: "GET", path: "/assets/index-Ab12.js"}, {method: "GET", path: "/api/health"},
		{method: "GET", path: "/api/auth/me"}, {method: "POST", path: "/"}} {
		h := a.do(t, c).header
		for name, want := range map[string]string{
			"X-Content-Type-Options":  "nosniff",
			"X-Frame-Options":         "DENY",
			"Referrer-Policy":         "same-origin",
			"Content-Security-Policy": "default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'",
		} {
			if got := h.Get(name); got != want {
				t.Errorf("%s %s: %s %q, want %q", c.method, c.path, name, got, want)
			}
		}
	}
}
