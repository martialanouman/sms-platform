package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/martialanouman/sms-platform/internal/adminapi/adminapitest"
	"github.com/martialanouman/sms-platform/internal/platform/config"
	"github.com/martialanouman/sms-platform/internal/platform/mtls/mtlstest"
	"github.com/martialanouman/sms-platform/internal/platform/observability"
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

func valid(t *testing.T, ca mtlstest.Authority) config.Infra {
	t.Helper()
	return config.Infra{
		LogLevel:          slog.LevelInfo,
		OpsAddr:           freeAddr(t),
		InternalAddr:      freeAddr(t),
		DrainBudget:       5 * time.Second,
		ShutdownTimeout:   2 * time.Second,
		TLS:               ca.Issue(t, t.TempDir(), "admin-api-svc"),
		TLSAllowedClients: []string{"dashboard"},
		JWKSURL:           "https://dashboard:1/.well-known/jwks.json",
		PublicURL:         adminapitest.Issuer + "/",
		PostgresURL:       "postgres://admin_api_svc@127.0.0.1:1/control_plane",
	}
}

func TestRunRefusesEachFaultBeforeListening(t *testing.T) {
	for name, c := range map[string]struct {
		change func(*config.Infra)
		want   string
	}{
		"no internal address":     {func(i *config.Infra) { i.InternalAddr = "" }, "INTERNAL_ADDR"},
		"allowlist beyond":        {func(i *config.Infra) { i.TLSAllowedClients = []string{"dashboard", "router-svc"} }, "TLS_ALLOWED_CLIENTS"},
		"allowlist short":         {func(i *config.Infra) { i.TLSAllowedClients = nil }, "TLS_ALLOWED_CLIENTS"},
		"allowlist of another":    {func(i *config.Infra) { i.TLSAllowedClients = []string{"router-svc"} }, "TLS_ALLOWED_CLIENTS"},
		"no JWKS":                 {func(i *config.Infra) { i.JWKSURL = "" }, "JWKS_URL"},
		"JWKS in clear":           {func(i *config.Infra) { i.JWKSURL = "http://dashboard:8443/.well-known/jwks.json" }, "JWKS_URL"},
		"no issuer":               {func(i *config.Infra) { i.PublicURL = "" }, "DASHBOARD_PUBLIC_URL"},
		"issuer with a path":      {func(i *config.Infra) { i.PublicURL = "https://dashboard.test/app" }, "DASHBOARD_PUBLIC_URL"},
		"no Postgres":             {func(i *config.Infra) { i.PostgresURL = "" }, "POSTGRES_URL"},
		"unreadable Postgres URL": {func(i *config.Infra) { i.PostgresURL = "postgres://[" }, "POSTGRES_URL"},
		"no certificate":          {func(i *config.Infra) { i.TLS.Cert = "" }, "mtls"},
	} {
		t.Run(name, func(t *testing.T) {
			infra := valid(t, mtlstest.New(t))
			c.change(&infra)
			err := run(t.Context(), infra, slog.New(slog.DiscardHandler), prometheus.NewRegistry(), prometheus.NewRegistry())
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("run = %v, want %q", err, c.want)
			}
			for _, addr := range []string{infra.OpsAddr, infra.InternalAddr} {
				ln, lerr := net.Listen("tcp", addr)
				if lerr != nil {
					t.Fatalf("run listened on %s before refusing: %v", addr, lerr)
				}
				_ = ln.Close()
			}
		})
	}
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

type gateway struct {
	infra    config.Infra
	ca       mtlstest.Authority
	keys     adminapitest.Keys
	logs     *lockedBuffer
	migrator *pgx.Conn
}

func start(t *testing.T, jwks func(ca mtlstest.Authority, keys adminapitest.Keys) string) gateway {
	t.Helper()
	ctx := context.Background()
	pg := pgtest.ControlPlane(t)
	migrator, err := pgx.Connect(ctx, pg.As("migrator"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = migrator.Close(ctx) })
	for _, s := range []string{string(migrations.Grants("control_plane")), `ALTER ROLE admin_api_svc PASSWORD 'admin_api_svc'`} {
		if _, err := migrator.Exec(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	ca := mtlstest.New(t)
	keys := adminapitest.NewKeys(t, "v1")
	infra := valid(t, ca)
	infra.PostgresURL = pg.As("admin_api_svc")
	infra.JWKSURL = jwks(ca, keys)
	logs := &lockedBuffer{}
	runCtx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	registry := prometheus.NewRegistry()
	go func() {
		done <- run(runCtx, infra, observability.NewLogger(logs, service, slog.LevelInfo), registry, registry)
	}()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("run = %v", err)
		}
	})
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		resp, err := http.Get("http://" + infra.OpsAddr + "/readyz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("/readyz never answered 200; logs: %s", logs.String())
		}
	}
	return gateway{infra: infra, ca: ca, keys: keys, logs: logs, migrator: migrator}
}

func servedJWKS(t *testing.T, san string, fetches *atomic.Int32) func(mtlstest.Authority, adminapitest.Keys) string {
	return func(ca mtlstest.Authority, keys adminapitest.Keys) string {
		cfg, err := ca.Issue(t, t.TempDir(), san).Server([]string{"admin-api-svc"}, []string{"http/1.1"})
		if err != nil {
			t.Fatal(err)
		}
		srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fetches.Add(1)
			if r.URL.Path != "/.well-known/jwks.json" {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(keys.JWKS(t))
		}))
		srv.TLS = cfg
		srv.StartTLS()
		t.Cleanup(srv.Close)
		_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
		return "https://localhost:" + port + "/.well-known/jwks.json"
	}
}

func (g gateway) client(t *testing.T, name string) *http.Client {
	t.Helper()
	cfg, err := g.ca.Issue(t, t.TempDir(), name).Client()
	if err != nil {
		t.Fatal(err)
	}
	cfg.ServerName = "admin-api-svc"
	return &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}, Timeout: 5 * time.Second}
}

func (g gateway) get(t *testing.T, c *http.Client, scope string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "https://"+g.infra.InternalAddr+"/admin/audit-log", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+g.keys.Token(t, "v1", scope))
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestTheGatewayVerifiesAgainstTheDashboardJWKSAndAuditsUnderItsOperator(t *testing.T) {
	var fetches atomic.Int32
	g := start(t, servedJWKS(t, "localhost", &fetches))
	c := g.client(t, "dashboard")
	if n := fetches.Load(); n != 0 {
		t.Fatalf("the JWKS was read %d times before any token", n)
	}

	if status, body := g.get(t, c, "audit:read"); status != http.StatusOK {
		t.Fatalf("first read: %d %s; logs: %s", status, body, g.logs.String())
	}
	status, body := g.get(t, c, "audit:read")
	var page struct {
		Data []struct {
			Operator    string `json:"operator"`
			OperationID string `json:"operation_id"`
			Status      int    `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &page); err != nil || status != http.StatusOK || len(page.Data) != 1 ||
		page.Data[0].Operator != adminapitest.Operator || page.Data[0].OperationID != "list-audit-log" || page.Data[0].Status != 200 {
		t.Fatalf("second read: %d %s", status, body)
	}
	if status, _ := g.get(t, c, "routes:read"); status != http.StatusForbidden {
		t.Fatalf("without audit:read: %d", status)
	}
	if _, err := g.client(t, "router-svc").Get("https://" + g.infra.InternalAddr + "/admin/audit-log"); err == nil {
		t.Fatal("router-svc passed the handshake")
	}
	if !jsonLine(t, g.logs, "TLS handshake error", "WARN") {
		t.Fatalf("no JSON line for the refused handshake; logs: %s", g.logs.String())
	}

	if _, err := g.migrator.Exec(context.Background(), `REVOKE INSERT ON control_plane.audit_log FROM admin_api_svc`); err != nil {
		t.Fatal(err)
	}
	if status, _ := g.get(t, c, "audit:read"); status != http.StatusServiceUnavailable {
		t.Fatalf("without its audit line: %d", status)
	}
	if !jsonLine(t, g.logs, "admin call not committed", "ERROR") {
		t.Fatalf("no JSON line for the uncommitted call; logs: %s", g.logs.String())
	}
	if m := metricsOf(t, g); !strings.Contains(m, "\nadmin_api_uncommitted_calls_total 1\n") {
		t.Fatalf("metrics: %s", m)
	}
}

func jsonLine(t *testing.T, logs *lockedBuffer, msg, level string) bool {
	t.Helper()
	for line := range strings.Lines(logs.String()) {
		var entry struct {
			Msg   string `json:"msg"`
			Level string `json:"level"`
		}
		if json.Unmarshal([]byte(line), &entry) == nil && strings.Contains(entry.Msg, msg) && entry.Level == level {
			return true
		}
	}
	return false
}

func TestABootWithAWrongPasswordFails(t *testing.T) {
	pg := pgtest.ControlPlane(t)
	infra := valid(t, mtlstest.New(t))
	infra.PostgresURL = pg.AsWith("admin_api_svc", "wrong")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	err := run(ctx, infra, slog.New(slog.DiscardHandler), prometheus.NewRegistry(), prometheus.NewRegistry())
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "28P01" {
		t.Fatalf("run = %v, want the refusal of the password", err)
	}
}

func TestAJWKSServedUnderAnotherNameIsNotTrusted(t *testing.T) {
	var fetches atomic.Int32
	g := start(t, servedJWKS(t, "other", &fetches))

	if status, body := g.get(t, g.client(t, "dashboard"), "audit:read"); status != http.StatusUnauthorized {
		t.Fatalf("%d %s", status, body)
	}
	if m := metricsOf(t, g); !strings.Contains(m, "\nadmin_api_jwks_reload_failures_total 1\n") || fetches.Load() != 0 {
		t.Fatalf("fetches %d, metrics: %s", fetches.Load(), m)
	}
}

func TestNothingIsReadFromAnUnreachableJWKSAtBootAndItsCallsAreRefused(t *testing.T) {
	g := start(t, func(mtlstest.Authority, adminapitest.Keys) string { return "https://dashboard:1/.well-known/jwks.json" })

	if status, body := g.get(t, g.client(t, "dashboard"), "audit:read"); status != http.StatusUnauthorized {
		t.Fatalf("%d %s", status, body)
	}
	if !jsonLine(t, g.logs, "jwks not reloaded", "WARN") {
		t.Fatalf("no JSON line for the reload; logs: %s", g.logs.String())
	}
	if m := metricsOf(t, g); !strings.Contains(m, "\nadmin_api_jwks_reload_failures_total 1\n") ||
		!strings.Contains(m, "\nadmin_api_uncommitted_calls_total 0\n") {
		t.Fatalf("metrics: %s", m)
	}
}

func metricsOf(t *testing.T, g gateway) string {
	t.Helper()
	resp, err := http.Get("http://" + g.infra.OpsAddr + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	m, _ := io.ReadAll(resp.Body)
	return string(m)
}
