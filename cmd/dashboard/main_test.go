package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/martialanouman/sms-platform/internal/platform/config"
	"github.com/martialanouman/sms-platform/internal/platform/mtls/mtlstest"
	"github.com/martialanouman/sms-platform/internal/platform/pgtest"
	"github.com/martialanouman/sms-platform/migrations"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func file(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func valid(t *testing.T) config.Infra {
	t.Helper()
	httpAddr := freeAddr(t)
	return config.Infra{
		OpsAddr:           freeAddr(t),
		DrainDelay:        0,
		DrainBudget:       10 * time.Second,
		ShutdownTimeout:   5 * time.Second,
		HTTPAddr:          httpAddr,
		PostgresURL:       "postgres://dashboard@127.0.0.1:1/dashboard",
		PublicURL:         "http://" + strings.Replace(httpAddr, "127.0.0.1", "localhost", 1),
		BootstrapEmail:    "owner@example.test",
		SMTPAddr:          "localhost:1025",
		SMTPFrom:          "dashboard@example.test",
		CookieKeyFile:     file(t, strings.Repeat("k", 64)),
		TotpKeyFile:       file(t, strings.Repeat("ab", 32)+"\n"),
		TLS:               mtlstest.New(t).Issue(t, t.TempDir(), "localhost"),
		InternalAddr:      freeAddr(t),
		OpenBaoAddr:       "https://127.0.0.1:1",
		TLSAllowedClients: []string{"admin-api-svc"},
	}
}

func TestRunRefusesEachFaultBeforeConnecting(t *testing.T) {
	for name, tc := range map[string]struct {
		spoil func(*config.Infra)
		want  string
	}{
		"no listener":           {func(i *config.Infra) { i.HTTPAddr = "" }, "HTTP_ADDR"},
		"no database":           {func(i *config.Infra) { i.PostgresURL = "" }, "POSTGRES_URL"},
		"no bootstrap":          {func(i *config.Infra) { i.BootstrapEmail = "owner" }, "DASHBOARD_BOOTSTRAP_EMAIL"},
		"bootstrap two ats":     {func(i *config.Infra) { i.BootstrapEmail = "a@b@example.test" }, "DASHBOARD_BOOTSTRAP_EMAIL"},
		"bootstrap no local":    {func(i *config.Infra) { i.BootstrapEmail = " @example.test" }, "DASHBOARD_BOOTSTRAP_EMAIL"},
		"relay without port":    {func(i *config.Infra) { i.SMTPAddr = "mailpit" }, "SMTP_ADDR"},
		"no sender":             {func(i *config.Infra) { i.SMTPFrom = "" }, "SMTP_FROM"},
		"user without secret":   {func(i *config.Infra) { i.SMTPUsername = "relay" }, "SMTP_PASSWORD_FILE"},
		"secret without user":   {func(i *config.Infra) { i.SMTPPasswordFile = "/x" }, "SMTP_USERNAME"},
		"no cookie key":         {func(i *config.Infra) { i.CookieKeyFile = "" }, "COOKIE_KEY_FILE"},
		"short cookie key":      {func(i *config.Infra) { i.CookieKeyFile = file(t, "short") }, "32 bytes"},
		"no TOTP key":           {func(i *config.Infra) { i.TotpKeyFile = "" }, "TOTP_KEY_FILE"},
		"TOTP key not hex":      {func(i *config.Infra) { i.TotpKeyFile = file(t, strings.Repeat("k", 64)) }, "TOTP_KEY_FILE"},
		"short TOTP key":        {func(i *config.Infra) { i.TotpKeyFile = file(t, strings.Repeat("ab", 31)) }, "32 bytes"},
		"unreadable TOTP key":   {func(i *config.Infra) { i.TotpKeyFile = "/nonexistent/totp-key" }, "totp-key"},
		"no CA":                 {func(i *config.Infra) { i.TLS.CA = "" }, "TLS_CA_FILE"},
		"empty CA":              {func(i *config.Infra) { i.TLS.CA = file(t, "") }, "CA"},
		"public URL with path":  {func(i *config.Infra) { i.PublicURL += "/app" }, "DASHBOARD_PUBLIC_URL"},
		"public URL relative":   {func(i *config.Infra) { i.PublicURL = "localhost:8080" }, "DASHBOARD_PUBLIC_URL"},
		"public URL query":      {func(i *config.Infra) { i.PublicURL += "?a=1" }, "DASHBOARD_PUBLIC_URL"},
		"public URL fragment":   {func(i *config.Infra) { i.PublicURL += "#a" }, "DASHBOARD_PUBLIC_URL"},
		"public URL user":       {func(i *config.Infra) { i.PublicURL = "http://u@localhost:8080" }, "DASHBOARD_PUBLIC_URL"},
		"public URL ftp":        {func(i *config.Infra) { i.PublicURL = "ftp://localhost:8080" }, "DASHBOARD_PUBLIC_URL"},
		"relay without number":  {func(i *config.Infra) { i.SMTPAddr = "mailpit:" }, "SMTP_ADDR"},
		"public URL by address": {func(i *config.Infra) { i.PublicURL = "http://127.0.0.1:8080" }, "RPID"},
		"no internal listener":  {func(i *config.Infra) { i.InternalAddr = "" }, "INTERNAL_ADDR"},
		"OpenBao in clear":      {func(i *config.Infra) { i.OpenBaoAddr = "http://openbao.openbao.svc:8200" }, "OPENBAO_ADDR"},
		"no certificate":        {func(i *config.Infra) { i.TLS.Cert = "" }, "TLS_CERT_FILE"},
		"no key":                {func(i *config.Infra) { i.TLS.Key = "" }, "TLS_KEY_FILE"},
		"other callers":         {func(i *config.Infra) { i.TLSAllowedClients = []string{"admin-api-svc", "router-svc"} }, "TLS_ALLOWED_CLIENTS"},
		"no callers":            {func(i *config.Infra) { i.TLSAllowedClients = nil }, "TLS_ALLOWED_CLIENTS"},
	} {
		infra := valid(t)
		tc.spoil(&infra)
		err := run(context.Background(), infra, slog.New(slog.DiscardHandler), prometheus.NewRegistry(), prometheus.NewRegistry())
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want a refusal naming %s", name, err, tc.want)
		}
	}
}

func mailpit(t *testing.T) (smtpAddr, api string, files config.Infra, ca mtlstest.Authority) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	ca = mtlstest.New(t)
	tlsFiles := ca.Issue(t, t.TempDir(), "localhost")
	c, err := testcontainers.Run(ctx, "axllent/mailpit:v1.31.4",
		testcontainers.WithExposedPorts("1025/tcp", "8025/tcp"),
		testcontainers.WithEnv(map[string]string{"MP_SMTP_TLS_CERT": "/tls/cert.pem", "MP_SMTP_TLS_KEY": "/tls/key.pem", "MP_SMTP_REQUIRE_STARTTLS": "true",
			"MP_SMTP_AUTH": "relay:relay-secret"}),
		testcontainers.WithFiles(
			testcontainers.ContainerFile{HostFilePath: tlsFiles.Cert, ContainerFilePath: "/tls/cert.pem", FileMode: 0o644},
			testcontainers.ContainerFile{HostFilePath: tlsFiles.Key, ContainerFilePath: "/tls/key.pem", FileMode: 0o644},
		),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/livez").WithPort("8025/tcp")),
	)
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatal(err)
	}
	smtpAddr, err = c.PortEndpoint(ctx, "1025/tcp", "")
	if err != nil {
		t.Fatal(err)
	}
	api, err = c.PortEndpoint(ctx, "8025/tcp", "http")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Replace(smtpAddr, "127.0.0.1", "localhost", 1), api, config.Infra{TLS: tlsFiles}, ca
}

var linkToken = regexp.MustCompile(`/access#([A-Za-z0-9_-]{43})\b`)

func getJSON(t *testing.T, url string, into any) {
	t.Helper()
	r, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		t.Fatal(err)
	}
}

type deployment struct {
	ca       mtlstest.Authority
	infra    config.Infra
	api      string
	migrator *pgx.Conn
	expected string
}

func prepare(t *testing.T, seeds ...string) deployment {
	t.Helper()
	ctx := context.Background()
	pg := pgtest.ControlPlane(t).In("dashboard")
	migrator, err := pgx.Connect(ctx, pg.As("migrator"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = migrator.Close(ctx) })
	for _, s := range append([]string{string(migrations.Grants("dashboard")), `ALTER ROLE dashboard PASSWORD 'dashboard'`}, seeds...) {
		if _, err := migrator.Exec(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	smtpAddr, api, relay, ca := mailpit(t)
	infra := valid(t)
	infra.PostgresURL, infra.SMTPAddr, infra.TLS = pg.As("dashboard"), smtpAddr, relay.TLS
	infra.SMTPUsername, infra.SMTPPasswordFile = "relay", file(t, "relay-secret\n")
	expected := infra.PublicURL
	infra.PublicURL += "/"
	infra.DrainDelay = time.Second
	return deployment{ca: ca, infra: infra, api: api, migrator: migrator, expected: expected}
}

func (d deployment) start(t *testing.T, registry *prometheus.Registry) (stop func(), done chan error) {
	t.Helper()
	runCtx, stop := context.WithCancel(context.Background())
	done = make(chan error, 1)
	go func() { done <- run(runCtx, d.infra, slog.New(slog.DiscardHandler), registry, registry) }()
	t.Cleanup(stop)
	return stop, done
}

type message struct {
	To   []struct{ Address string }
	Text string
}

func (d deployment) messages(t *testing.T) []message {
	t.Helper()
	var list struct{ Messages []struct{ ID string } }
	getJSON(t, d.api+"/api/v1/messages", &list)
	all := make([]message, len(list.Messages))
	for i, m := range list.Messages {
		getJSON(t, d.api+"/api/v1/message/"+m.ID, &all[i])
	}
	return all
}

func (d deployment) count(t *testing.T, query string) int {
	t.Helper()
	var n int
	if err := d.migrator.QueryRow(context.Background(), query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

const pending = `WITH o AS (INSERT INTO dashboard.operators (email, display_name) VALUES ('second@example.test', 's') RETURNING id)
	INSERT INTO dashboard.access_links (operator_id, kind) SELECT id, 'activation' FROM o`

func TestTheDashboardServesAnOperatorFromActivationToLogout(t *testing.T) {
	d := prepare(t,
		`WITH o AS (INSERT INTO dashboard.operators (email, display_name) VALUES ('stuck@example.test', 's') RETURNING id)
			INSERT INTO dashboard.access_links (operator_id, kind, attempts) SELECT id, 'activation', 10 FROM o`,
		pending,
		`INSERT INTO dashboard.sessions (operator_id, token_hash, created_at, expires_at)
			SELECT id, 'old', now() - interval '13 hours', now() - interval '1 hour' FROM dashboard.operators WHERE email = 'stuck@example.test'`)
	stop, done := d.start(t, prometheus.NewRegistry())

	base := "http://" + d.infra.HTTPAddr + "/api"
	var token string
	var mails []message
	for deadline := time.Now().Add(20 * time.Second); len(mails) == 0 && time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		mails = d.messages(t)
	}
	time.Sleep(time.Second)
	mails = d.messages(t)
	{
		for _, m := range mails {
			if got := linkToken.FindStringSubmatch(m.Text); got != nil && m.To[0].Address == "owner@example.test" && strings.Contains(m.Text, d.expected+"/access#"+got[1]) {
				token = got[1]
			}
		}
	}
	if token == "" || len(mails) != 2 {
		t.Fatalf("%d messages, token %v: want the owner's and the second operator's links from the first tick", len(mails), token != "")
	}
	if n := d.count(t, `SELECT count(*) FROM dashboard.sessions`); n != 0 {
		t.Errorf("%d sessions left, want the ended one purged at start", n)
	}
	post := func(path, body, cookie string) (*http.Response, []byte) {
		r, err := http.NewRequest("POST", base+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Origin", d.expected)
		r.Header.Set("Content-Type", "application/json; charset=utf-8")
		if cookie != "" {
			r.Header.Set("Cookie", "__Host-dashboard_session="+cookie)
		}
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		answer, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return resp, answer
	}
	if resp, _ := post("/auth/access-link", `{"token":"`+token+`","password":"Correct-horse-9"}`, ""); resp.StatusCode != 204 {
		t.Fatalf("access link: %d", resp.StatusCode)
	}
	login, _ := post("/auth/login", `{"email":"owner@example.test","password":"Correct-horse-9"}`, "")
	if login.StatusCode != 200 || len(login.Cookies()) != 1 {
		t.Fatalf("login: %d", login.StatusCode)
	}
	cookie := login.Cookies()[0].Value
	if resp, answer := post("/auth/mfa/totp/enroll", `{}`, cookie); resp.StatusCode != 200 || !strings.Contains(string(answer), `"otpauthUri":"otpauth://totp/`) {
		t.Fatalf("enroll: %d %.80q, want the TOTP key wired", resp.StatusCode, answer)
	}
	shell, err := http.Get("http://" + d.infra.HTTPAddr + "/login")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(shell.Body)
	_ = shell.Body.Close()
	if shell.StatusCode != 200 || !strings.Contains(string(page), `<html lang="fr">`) {
		t.Fatalf("GET /login: %d %.80q, want the embedded SPA (make web builds it)", shell.StatusCode, page)
	}
	r, err := http.NewRequest("GET", base+"/auth/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Cookie", "__Host-dashboard_session="+cookie)
	me, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	_ = me.Body.Close()
	if me.StatusCode != 200 {
		t.Fatalf("me: %d", me.StatusCode)
	}
	jwks, err := http.Get("http://" + d.infra.HTTPAddr + "/.well-known/jwks.json")
	if err != nil {
		t.Fatal(err)
	}
	public, _ := io.ReadAll(jwks.Body)
	_ = jwks.Body.Close()
	if strings.Contains(string(public), `"keys"`) {
		t.Fatalf("the public listener serves the JWKS: %s", public)
	}
	internal := func(names ...string) (*http.Response, error) {
		cfg := &tls.Config{MinVersion: tls.VersionTLS13, ServerName: "localhost", RootCAs: x509.NewCertPool()}
		cfg.RootCAs.AppendCertsFromPEM(d.ca.PEM)
		if len(names) > 0 {
			files := d.ca.Issue(t, t.TempDir(), names...)
			cert, err := tls.LoadX509KeyPair(files.Cert, files.Key)
			if err != nil {
				t.Fatal(err)
			}
			cfg.Certificates = []tls.Certificate{cert}
		}
		client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: cfg}}
		return client.Get("https://" + d.infra.InternalAddr + "/.well-known/jwks.json")
	}
	if resp, err := internal("admin-api-svc"); err != nil || resp.StatusCode != 503 {
		t.Fatalf("the JWKS to admin-api-svc, OpenBao unreachable: %v, %v", resp, err)
	}
	slow := func() {
		files := d.ca.Issue(t, t.TempDir(), "admin-api-svc")
		cert, err := tls.LoadX509KeyPair(files.Cert, files.Key)
		if err != nil {
			t.Fatal(err)
		}
		roots := x509.NewCertPool()
		roots.AppendCertsFromPEM(d.ca.PEM)
		conn, err := tls.Dial("tcp", d.infra.InternalAddr, &tls.Config{MinVersion: tls.VersionTLS13, ServerName: "localhost", RootCAs: roots,
			Certificates: []tls.Certificate{cert}, NextProtos: []string{"h2", "http/1.1"}})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Close() }()
		if p := conn.ConnectionState().NegotiatedProtocol; p != "http/1.1" {
			t.Fatalf("the internal listener negotiated %q, want http/1.1 alone", p)
		}
		if _, err := conn.Write([]byte("GET /.well-known/jwks.json HTTP/1.1\r\n")); err != nil {
			t.Fatal(err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
		started := time.Now()
		_, _ = io.ReadAll(conn)
		if time.Since(started) > 10*time.Second {
			t.Fatal("the internal listener held a half-sent request past its header timeout")
		}
	}
	slow()
	for _, names := range [][]string{{"router-svc"}, nil} {
		if resp, err := internal(names...); err == nil {
			_ = resp.Body.Close()
			t.Fatalf("the internal listener answered %v: %d", names, resp.StatusCode)
		}
	}
	if resp, _ := post("/auth/logout", "", cookie); resp.StatusCode != 204 {
		t.Fatalf("logout: %d", resp.StatusCode)
	}
	metrics, err := http.Get("http://" + d.infra.OpsAddr + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	exposed, _ := io.ReadAll(metrics.Body)
	_ = metrics.Body.Close()
	if !strings.Contains(string(exposed), "dashboard_access_links_undeliverable 1") {
		t.Errorf("metrics do not expose the undeliverable links:\n%s", exposed)
	}

	stop()
	draining := false
	for deadline := time.Now().Add(time.Second); !draining && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if ready, err := http.Get("http://" + d.infra.OpsAddr + "/readyz"); err == nil {
			draining = ready.StatusCode == 503
			_ = ready.Body.Close()
		}
	}
	if !draining {
		t.Error("readyz never answered 503 during the drain delay")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run = %v after the drain", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("run outlived its drain budget")
	}
	open := -1
	for deadline := time.Now().Add(5 * time.Second); open != 0 && time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		open = d.count(t, `SELECT count(*) FROM pg_stat_activity WHERE usename = 'dashboard'`)
	}
	if open != 0 {
		t.Fatalf("%d connections of the dashboard left after the drain", open)
	}
}

func TestAnUnrecordedDeliveryWaitsItsTickAndASlowBodyIsCut(t *testing.T) {
	d := prepare(t,
		`WITH o AS (INSERT INTO dashboard.operators (email, display_name) VALUES ('owner@example.test', 'o') RETURNING id)
			INSERT INTO dashboard.access_links (operator_id, kind) SELECT id, 'activation' FROM o`,
		`REVOKE UPDATE ON dashboard.access_links FROM dashboard`,
		`GRANT UPDATE (kind, expires_at, sent_at, attempts, next_attempt_at) ON dashboard.access_links TO dashboard`)
	d.start(t, prometheus.NewRegistry())
	for deadline := time.Now().Add(20 * time.Second); len(d.messages(t)) == 0 && time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
	}
	time.Sleep(2 * time.Second)
	if n := len(d.messages(t)); n != 1 {
		t.Fatalf("%d messages in the first tick, want one: a sending it cannot record must wait for the next tick", n)
	}
	conn, err := net.Dial("tcp", d.infra.HTTPAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := io.WriteString(conn, "POST /api/auth/login HTTP/1.1\r\nHost: x\r\nOrigin: "+d.expected+
		"\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_ = conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if held := time.Since(start); err != nil || resp.StatusCode != http.StatusUnprocessableEntity || held > 15*time.Second {
		t.Fatalf("a body sent slowly: %v after %v, want a 422 once the read timeout has cut the body", err, held)
	}
	_ = resp.Body.Close()
}

func TestServeReturnsOnlyOnceTheRequestsInFlightAreAnswered(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	arrived, answered := make(chan struct{}), make(chan struct{})
	srv := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(arrived)
		time.Sleep(time.Second)
		close(answered)
	})}
	ctx, cancel := context.WithCancel(context.Background())
	returned := make(chan error, 1)
	go func() { returned <- serve(srv, ln, 5*time.Second)(ctx) }()
	go func() {
		if resp, err := http.Get("http://" + ln.Addr().String()); err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-arrived
	cancel()
	select {
	case err := <-returned:
		if err != nil {
			t.Fatalf("serve = %v", err)
		}
		select {
		case <-answered:
		default:
			t.Fatal("serve returned while a request was in flight")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve never returned")
	}
}
