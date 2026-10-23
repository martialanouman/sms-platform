package contentkey_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"

	"github.com/martialanouman/sms-platform/internal/adminapi/contentkey"
	keys "github.com/martialanouman/sms-platform/internal/contentkey"
	contentkeygrpc "github.com/martialanouman/sms-platform/internal/contentkey/grpc"
	"github.com/martialanouman/sms-platform/internal/platform/mtls/mtlstest"
)

type kms struct{}

func (kms) Encrypt(_ context.Context, _ string, plaintext []byte) ([]byte, error) {
	return append([]byte("vault:v1:"), plaintext...), nil
}

func (kms) Decrypt(context.Context, string, []byte) ([]byte, error) { return nil, errors.New("unused") }

func serve(t *testing.T, ca mtlstest.Authority, san string) string {
	t.Helper()
	cfg, err := ca.Issue(t, t.TempDir(), san).Server([]string{"admin-api-svc"}, []string{"h2"})
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer(grpc.Creds(credentials.NewTLS(cfg)), grpc.UnaryInterceptor(contentkeygrpc.Authorize))
	contentkeygrpc.Register(s, keys.ConfigSecrets{KMS: kms{}})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.Serve(ln) }()
	t.Cleanup(s.Stop)
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	return "content-key-svc:" + port
}

func dial(t *testing.T, ca mtlstest.Authority, addr string, log *slog.Logger) *contentkey.Client {
	t.Helper()
	cfg, err := ca.Issue(t, t.TempDir(), "admin-api-svc").Client()
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(addr)
	dns := manual.NewBuilderWithScheme("dns")
	dns.InitialState(resolver.State{Addresses: []resolver.Address{{Addr: "127.0.0.1:" + port}}})
	c, err := contentkey.New(addr, cfg, log, grpc.WithResolvers(dns))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestTheAdminAPISealsThroughContentKey(t *testing.T) {
	ca := mtlstest.New(t)
	c := dial(t, ca, serve(t, ca, "content-key-svc"), slog.New(slog.DiscardHandler))
	sealed, ref, err := c.Seal(t.Context(), []byte("secret"))
	if err != nil || string(sealed) != "vault:v1:secret" || ref != "config-secrets" {
		t.Fatalf("Seal = %q %q %v", sealed, ref, err)
	}
}

func TestAServerUnderAnotherNameIsRefusedAndTheLogTellsNoSecret(t *testing.T) {
	ca := mtlstest.New(t)
	var out bytes.Buffer
	c := dial(t, ca, serve(t, ca, "router-svc"), slog.New(slog.NewJSONHandler(&out, nil)))
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if _, _, err := c.Seal(ctx, []byte("s3cret-value")); err == nil {
		t.Fatal("sealed through a server that is not content-key-svc")
	}
	if !strings.Contains(out.String(), `"level":"ERROR"`) || strings.Contains(out.String(), "s3cret-value") {
		t.Fatalf("log = %s", out.String())
	}
}

func TestAServerThatIsDownFailsWithinTheDeadline(t *testing.T) {
	ca := mtlstest.New(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	_ = ln.Close()
	c := dial(t, ca, "content-key-svc:"+port, slog.New(slog.DiscardHandler))
	start := time.Now()
	if _, _, err := c.Seal(t.Context(), []byte("x")); err == nil {
		t.Fatal("sealed with no server")
	}
	if took := time.Since(start); took > 7*time.Second {
		t.Fatalf("took %v", took)
	}
}

func TestASealWaitsForAServerThatStartsLate(t *testing.T) {
	ca := mtlstest.New(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	_ = ln.Close()
	c := dial(t, ca, "content-key-svc:"+port, slog.New(slog.DiscardHandler))
	go func() {
		time.Sleep(500 * time.Millisecond)
		cfg, err := ca.Issue(t, t.TempDir(), "content-key-svc").Server([]string{"admin-api-svc"}, []string{"h2"})
		if err != nil {
			return
		}
		s := grpc.NewServer(grpc.Creds(credentials.NewTLS(cfg)), grpc.UnaryInterceptor(contentkeygrpc.Authorize))
		contentkeygrpc.Register(s, keys.ConfigSecrets{KMS: kms{}})
		late, err := net.Listen("tcp", "127.0.0.1:"+port)
		if err != nil {
			return
		}
		t.Cleanup(s.Stop)
		_ = s.Serve(late)
	}()
	if _, _, err := c.Seal(t.Context(), []byte("x")); err != nil {
		t.Fatalf("Seal before the server was up: %v", err)
	}
}
