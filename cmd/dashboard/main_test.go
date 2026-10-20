package main

import (
	"context"
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
		OpsAddr:         freeAddr(t),
		DrainDelay:      0,
		DrainBudget:     10 * time.Second,
		ShutdownTimeout: 5 * time.Second,
		HTTPAddr:        httpAddr,
		PostgresURL:     "postgres://dashboard@127.0.0.1:1/dashboard",
		PublicURL:       "http://" + httpAddr,
		BootstrapEmail:  "owner@example.test",
		SMTPAddr:        "localhost:1025",
		SMTPFrom:        "dashboard@example.test",
		CookieKeyFile:   file(t, strings.Repeat("k", 64)),
		TLS:             mtlstest.New(t).Issue(t, t.TempDir(), "localhost"),
	}
}

func TestRunRefusesEachFaultBeforeConnecting(t *testing.T) {
	for name, tc := range map[string]struct {
		spoil func(*config.Infra)
		want  string
	}{
		"no listener":          {func(i *config.Infra) { i.HTTPAddr = "" }, "HTTP_ADDR"},
		"no database":          {func(i *config.Infra) { i.PostgresURL = "" }, "POSTGRES_URL"},
		"no bootstrap":         {func(i *config.Infra) { i.BootstrapEmail = "owner" }, "DASHBOARD_BOOTSTRAP_EMAIL"},
		"relay without port":   {func(i *config.Infra) { i.SMTPAddr = "mailpit" }, "SMTP_ADDR"},
		"no sender":            {func(i *config.Infra) { i.SMTPFrom = "" }, "SMTP_FROM"},
		"user without secret":  {func(i *config.Infra) { i.SMTPUsername = "relay" }, "SMTP_PASSWORD_FILE"},
		"secret without user":  {func(i *config.Infra) { i.SMTPPasswordFile = "/x" }, "SMTP_USERNAME"},
		"no cookie key":        {func(i *config.Infra) { i.CookieKeyFile = "" }, "COOKIE_KEY_FILE"},
		"short cookie key":     {func(i *config.Infra) { i.CookieKeyFile = file(t, "short") }, "32 bytes"},
		"no CA":                {func(i *config.Infra) { i.TLS.CA = "" }, "TLS_CA_FILE"},
		"empty CA":             {func(i *config.Infra) { i.TLS.CA = file(t, "") }, "CA"},
		"public URL with path": {func(i *config.Infra) { i.PublicURL += "/app" }, "DASHBOARD_PUBLIC_URL"},
		"public URL relative":  {func(i *config.Infra) { i.PublicURL = "localhost:8080" }, "DASHBOARD_PUBLIC_URL"},
	} {
		infra := valid(t)
		tc.spoil(&infra)
		err := run(context.Background(), infra, slog.New(slog.DiscardHandler), prometheus.NewRegistry(), prometheus.NewRegistry())
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want a refusal naming %s", name, err, tc.want)
		}
	}
}

func mailpit(t *testing.T) (smtpAddr, api string, files config.Infra) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	tlsFiles := mtlstest.New(t).Issue(t, t.TempDir(), "localhost")
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
	return strings.Replace(smtpAddr, "127.0.0.1", "localhost", 1), api, config.Infra{TLS: tlsFiles}
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

func TestTheDashboardServesAnOperatorFromActivationToLogout(t *testing.T) {
	ctx := context.Background()
	pg := pgtest.ControlPlane(t).In("dashboard")
	migrator, err := pgx.Connect(ctx, pg.As("migrator"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = migrator.Close(ctx) })
	for _, s := range []string{string(migrations.Grants("dashboard")), `ALTER ROLE dashboard PASSWORD 'dashboard'`,
		`WITH o AS (INSERT INTO dashboard.operators (email, display_name) VALUES ('stuck@example.test', 's') RETURNING id)
			INSERT INTO dashboard.access_links (operator_id, kind, attempts) SELECT id, 'activation', 10 FROM o`} {
		if _, err := migrator.Exec(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	smtpAddr, api, relay := mailpit(t)
	infra := valid(t)
	infra.PostgresURL, infra.SMTPAddr, infra.TLS = pg.As("dashboard"), smtpAddr, relay.TLS
	infra.SMTPUsername, infra.SMTPPasswordFile = "relay", file(t, "relay-secret\n")
	expected := infra.PublicURL
	infra.PublicURL += "/"
	infra.DrainDelay = time.Second
	registry := prometheus.NewRegistry()
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- run(runCtx, infra, slog.New(slog.DiscardHandler), registry, registry) }()
	t.Cleanup(stop)

	base := "http://" + infra.HTTPAddr + "/api"
	var token string
	for deadline := time.Now().Add(20 * time.Second); token == "" && time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		var list struct{ Messages []struct{ ID string } }
		getJSON(t, api+"/api/v1/messages", &list)
		if len(list.Messages) == 1 {
			var m struct{ Text string }
			getJSON(t, api+"/api/v1/message/"+list.Messages[0].ID, &m)
			if got := linkToken.FindStringSubmatch(m.Text); got != nil && strings.Contains(m.Text, expected+"/access#"+got[1]) {
				token = got[1]
			}
		}
	}
	if token == "" {
		t.Fatal("no activation link reached the relay")
	}
	post := func(path, body, cookie string) *http.Response {
		r, err := http.NewRequest("POST", base+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Origin", expected)
		r.Header.Set("Content-Type", "application/json")
		if cookie != "" {
			r.Header.Set("Cookie", "__Host-dashboard_session="+cookie)
		}
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return resp
	}
	if resp := post("/auth/access-link", `{"token":"`+token+`","password":"Correct-horse-9"}`, ""); resp.StatusCode != 204 {
		t.Fatalf("access link: %d", resp.StatusCode)
	}
	login := post("/auth/login", `{"email":"owner@example.test","password":"Correct-horse-9"}`, "")
	if login.StatusCode != 200 || len(login.Cookies()) != 1 {
		t.Fatalf("login: %d", login.StatusCode)
	}
	cookie := login.Cookies()[0].Value
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
	if resp := post("/auth/logout", "", cookie); resp.StatusCode != 204 {
		t.Fatalf("logout: %d", resp.StatusCode)
	}
	metrics, err := http.Get("http://" + infra.OpsAddr + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	exposed, _ := io.ReadAll(metrics.Body)
	_ = metrics.Body.Close()
	if !strings.Contains(string(exposed), "dashboard_access_links_undeliverable 1") {
		t.Errorf("metrics do not expose the undeliverable links:\n%s", exposed)
	}

	stop()
	time.Sleep(200 * time.Millisecond)
	if ready, err := http.Get("http://" + infra.OpsAddr + "/readyz"); err != nil || ready.StatusCode != 503 {
		t.Errorf("readyz during the drain delay: %v, %v; want 503", ready, err)
	} else {
		_ = ready.Body.Close()
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run = %v after the drain", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("run outlived its drain budget")
	}
	var open int
	if err := migrator.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE usename = 'dashboard'`).Scan(&open); err != nil || open != 0 {
		t.Fatalf("%d connections of the dashboard left after the drain (%v)", open, err)
	}
}
