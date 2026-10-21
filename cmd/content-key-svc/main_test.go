package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/martialanouman/sms-platform/internal/contentkey"
	contentkeygrpc "github.com/martialanouman/sms-platform/internal/contentkey/grpc"
	contentkeyv1 "github.com/martialanouman/sms-platform/internal/gen/contentkey/v1"
	"github.com/martialanouman/sms-platform/internal/platform/config"
	"github.com/martialanouman/sms-platform/internal/platform/mtls/mtlstest"
	"github.com/martialanouman/sms-platform/internal/platform/observability"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().String()
}

func complete(t *testing.T, ca mtlstest.Authority) config.Infra {
	t.Helper()
	return config.Infra{
		LogLevel:          slog.LevelInfo,
		OpsAddr:           freeAddr(t),
		GRPCAddr:          freeAddr(t),
		OpenBaoAddr:       "https://127.0.0.1:1",
		TLS:               ca.Issue(t, t.TempDir(), "content-key-svc"),
		TLSAllowedClients: []string{"admin-api-svc"},
		DrainBudget:       5 * time.Second,
		ShutdownTimeout:   2 * time.Second,
	}
}

func TestRunRefusesEachFaultBeforeListening(t *testing.T) {
	for name, c := range map[string]struct {
		change func(*config.Infra)
		want   string
	}{
		"no gRPC address":            {func(i *config.Infra) { i.GRPCAddr = "" }, "GRPC_ADDR"},
		"no OpenBao":                 {func(i *config.Infra) { i.OpenBaoAddr = "" }, "OPENBAO_ADDR"},
		"OpenBao in clear":           {func(i *config.Infra) { i.OpenBaoAddr = "http://openbao.openbao.svc:8200" }, "OPENBAO_ADDR"},
		"allowlist beyond the map":   {func(i *config.Infra) { i.TLSAllowedClients = []string{"admin-api-svc", "router-svc"} }, "TLS_ALLOWED_CLIENTS"},
		"allowlist short of the map": {func(i *config.Infra) { i.TLSAllowedClients = nil }, "TLS_ALLOWED_CLIENTS"},
		"allowlist of another":       {func(i *config.Infra) { i.TLSAllowedClients = []string{"router-svc"} }, "TLS_ALLOWED_CLIENTS"},
	} {
		t.Run(name, func(t *testing.T) {
			infra := complete(t, mtlstest.New(t))
			c.change(&infra)
			err := run(t.Context(), infra, slog.New(slog.DiscardHandler))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("run = %v, want %q", err, c.want)
			}
			for _, addr := range []string{infra.OpsAddr, infra.GRPCAddr} {
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

func TestTheServiceServesBehindItsInterceptorAndDrains(t *testing.T) {
	ca := mtlstest.New(t)
	infra := complete(t, ca)
	infra.DrainDelay, infra.DrainBudget, infra.ShutdownTimeout = 500*time.Millisecond, 2*time.Second, 300*time.Millisecond
	var logs lockedBuffer
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- run(ctx, infra, observability.NewLogger(&logs, "content-key-svc", slog.LevelInfo)) }()

	ready := false
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		resp, err := http.Get("http://" + infra.OpsAddr + "/readyz")
		if err == nil {
			_ = resp.Body.Close()
			if ready = resp.StatusCode == http.StatusOK; ready {
				break
			}
		}
	}
	if !ready {
		t.Fatalf("/readyz never answered 200 without OpenBao; logs: %s", logs.String())
	}

	cfg, err := ca.Issue(t, t.TempDir(), "admin-api-svc").Client()
	if err != nil {
		t.Fatal(err)
	}
	cfg.ServerName = "content-key-svc"
	conn, err := grpc.NewClient(infra.GRPCAddr, grpc.WithTransportCredentials(credentials.NewTLS(cfg)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	c := contentkeyv1.NewConfigSecretsServiceClient(conn)
	if _, err := c.Open(t.Context(), &contentkeyv1.OpenRequest{KmsKeyRef: "config-secrets", Sealed: []byte("x")}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("Open err = %v, want PermissionDenied from the interceptor", err)
	}
	if _, err := c.Seal(t.Context(), &contentkeyv1.SealRequest{Plaintext: []byte("x")}); status.Code(err) != codes.Unavailable {
		t.Fatalf("Seal err = %v, want Unavailable from the unreachable KMS", err)
	}

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(ca.PEM)
	anonymous, err := tls.Dial("tcp", infra.GRPCAddr, &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, ServerName: "content-key-svc", NextProtos: []string{"h2"}})
	if err == nil {
		_, err = anonymous.Read(make([]byte, 1))
		_ = anonymous.Close()
	}
	if err == nil {
		t.Fatal("a client without certificate completed its handshake")
	}
	refused := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && !refused; time.Sleep(50 * time.Millisecond) {
		refused = handshakeRefusedInJSON(t, logs.String())
	}
	if !refused {
		t.Fatalf("no JSON line of grpc for the refused handshake; logs: %s", logs.String())
	}

	mute, err := net.Dial("tcp", infra.GRPCAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mute.Close() }()
	cancel()
	draining := false
	for deadline := time.Now().Add(infra.DrainDelay); time.Now().Before(deadline) && !draining; time.Sleep(20 * time.Millisecond) {
		if resp, err := http.Get("http://" + infra.OpsAddr + "/readyz"); err == nil {
			_ = resp.Body.Close()
			draining = resp.StatusCode == http.StatusServiceUnavailable
		}
	}
	if !draining {
		t.Error("/readyz never answered 503 during the drain delay")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run = %v after the drain, behind a connection that never handshakes", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run did not drain")
	}
}

func handshakeRefusedInJSON(t *testing.T, logs string) bool {
	t.Helper()
	for line := range strings.SplitSeq(strings.TrimSpace(logs), "\n") {
		var rec map[string]any
		if json.Unmarshal([]byte(line), &rec) == nil && rec["logger"] == "grpc" && strings.Contains(fmt.Sprint(rec["msg"]), "ServerTransport") {
			return true
		}
	}
	return false
}

type blockingKMS struct {
	entered, release chan struct{}
}

func (k blockingKMS) Encrypt(ctx context.Context, _ string, _ []byte) ([]byte, error) {
	k.entered <- struct{}{}
	select {
	case <-k.release:
		return []byte("vault:v1:x"), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (k blockingKMS) Decrypt(context.Context, string, []byte) ([]byte, error) {
	return nil, errors.New("unused")
}

func TestServeWaitsForTheCallsInFlight(t *testing.T) {
	kms := blockingKMS{make(chan struct{}), make(chan struct{})}
	s := grpc.NewServer()
	contentkeygrpc.Register(s, contentkey.ConfigSecrets{KMS: kms})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	served := make(chan error, 1)
	go func() { served <- serve(s, ln, 10*time.Second)(ctx) }()
	conn, err := grpc.NewClient(ln.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	called := make(chan error, 1)
	go func() {
		_, err := contentkeyv1.NewConfigSecretsServiceClient(conn).Seal(context.WithoutCancel(t.Context()), &contentkeyv1.SealRequest{Plaintext: []byte("x")})
		called <- err
	}()
	select {
	case <-kms.entered:
	case err := <-called:
		t.Fatalf("the call never reached the KMS: %v", err)
	}
	cancel()
	select {
	case err := <-served:
		t.Fatalf("serve returned %v with a call in flight", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(kms.release)
	if err := <-called; err != nil {
		t.Fatalf("the call in flight failed: %v", err)
	}
	if err := <-served; err != nil {
		t.Fatalf("serve = %v", err)
	}
}

func TestServeReportsAFailedListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = ln.Close()
	done := make(chan error, 1)
	go func() { done <- serve(grpc.NewServer(), ln, time.Second)(t.Context()) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("serve = nil on a closed listener")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve hung on a closed listener")
	}
}

func TestServeStopsACallPastTheShutdownTimeout(t *testing.T) {
	kms := blockingKMS{make(chan struct{}, 1), make(chan struct{})}
	t.Cleanup(func() { close(kms.release) })
	s := grpc.NewServer()
	contentkeygrpc.Register(s, contentkey.ConfigSecrets{KMS: kms})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	served := make(chan error, 1)
	go func() { served <- serve(s, ln, 200*time.Millisecond)(ctx) }()
	conn, err := grpc.NewClient(ln.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	called := make(chan error, 1)
	go func() {
		_, err := contentkeyv1.NewConfigSecretsServiceClient(conn).Seal(context.WithoutCancel(t.Context()), &contentkeyv1.SealRequest{Plaintext: []byte("x")})
		called <- err
	}()
	<-kms.entered
	cancel()
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("serve = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("serve outlived its shutdown timeout of 200ms by a second")
	}
	select {
	case err := <-called:
		if err == nil {
			t.Fatal("the call held past the shutdown timeout succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the call held past the shutdown timeout never returned")
	}
}
