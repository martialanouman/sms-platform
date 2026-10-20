package grpc_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/martialanouman/sms-platform/internal/contentkey"
	contentkeygrpc "github.com/martialanouman/sms-platform/internal/contentkey/grpc"
	contentkeyv1 "github.com/martialanouman/sms-platform/internal/gen/contentkey/v1"
	"github.com/martialanouman/sms-platform/internal/platform/mtls/mtlstest"
)

type kms struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (k *kms) Encrypt(_ context.Context, _ string, plaintext []byte) ([]byte, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.calls++
	return append([]byte("vault:v1:"), plaintext...), k.err
}

func (k *kms) Decrypt(_ context.Context, _ string, sealed []byte) ([]byte, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.calls++
	return []byte(strings.TrimPrefix(string(sealed), "vault:v1:")), k.err
}

func serve(t *testing.T, k *kms, opts ...grpc.ServerOption) string {
	t.Helper()
	s := grpc.NewServer(opts...)
	contentkeygrpc.Register(s, contentkey.ConfigSecrets{KMS: k})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.Serve(ln) }()
	t.Cleanup(s.Stop)
	return ln.Addr().String()
}

type authority struct {
	t  *testing.T
	ca mtlstest.Authority
}

func (a authority) creds(names ...string) credentials.TransportCredentials {
	a.t.Helper()
	cfg, err := a.ca.Issue(a.t, a.t.TempDir(), names...).Client()
	if err != nil {
		a.t.Fatal(err)
	}
	cfg.ServerName = "content-key-svc"
	return credentials.NewTLS(cfg)
}

func authorized(t *testing.T, k *kms, allowed ...string) (string, authority) {
	t.Helper()
	a := authority{t, mtlstest.New(t)}
	cfg, err := a.ca.Issue(t, t.TempDir(), "content-key-svc").Server(allowed, []string{"h2"})
	if err != nil {
		t.Fatal(err)
	}
	return serve(t, k, grpc.Creds(credentials.NewTLS(cfg)), grpc.UnaryInterceptor(contentkeygrpc.Authorize)), a
}

func client(t *testing.T, addr string, creds credentials.TransportCredentials) contentkeyv1.ConfigSecretsServiceClient {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(creds))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return contentkeyv1.NewConfigSecretsServiceClient(conn)
}

func code(err error) codes.Code {
	return status.Code(err)
}

func TestAdminAPISealsAndCannotOpen(t *testing.T) {
	k := &kms{}
	addr, a := authorized(t, k, "admin-api-svc")
	c := client(t, addr, a.creds("admin-api-svc"))
	resp, err := c.Seal(t.Context(), &contentkeyv1.SealRequest{Plaintext: []byte("bind-pw")})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.GetSealed()) != "vault:v1:bind-pw" || resp.GetKmsKeyRef() != "config-secrets" {
		t.Fatalf("Seal = %q, %q", resp.GetSealed(), resp.GetKmsKeyRef())
	}
	_, err = c.Open(t.Context(), &contentkeyv1.OpenRequest{KmsKeyRef: "config-secrets", Sealed: resp.GetSealed()})
	if code(err) != codes.PermissionDenied {
		t.Fatalf("Open err = %v, want PermissionDenied", err)
	}
	if k.calls != 1 {
		t.Fatalf("%d KMS calls, want only Seal's", k.calls)
	}
}

func TestACallerAdmittedByTLSButAbsentFromTheMapReachesNothing(t *testing.T) {
	k := &kms{}
	addr, a := authorized(t, k, "admin-api-svc", "router-svc")
	c := client(t, addr, a.creds("router-svc"))
	if _, err := c.Seal(t.Context(), &contentkeyv1.SealRequest{Plaintext: []byte("x")}); code(err) != codes.PermissionDenied {
		t.Fatalf("Seal err = %v, want PermissionDenied", err)
	}
	if _, err := c.Open(t.Context(), &contentkeyv1.OpenRequest{KmsKeyRef: "config-secrets", Sealed: []byte("x")}); code(err) != codes.PermissionDenied {
		t.Fatalf("Open err = %v, want PermissionDenied", err)
	}
	if k.calls != 0 {
		t.Fatalf("%d KMS calls", k.calls)
	}
}

func TestTheCallerIsTheSANFoundInTheMapNotTheFirst(t *testing.T) {
	k := &kms{}
	addr, a := authorized(t, k, "admin-api-svc")
	c := client(t, addr, a.creds("router-svc", "admin-api-svc"))
	if _, err := c.Seal(t.Context(), &contentkeyv1.SealRequest{Plaintext: []byte("x")}); err != nil {
		t.Fatal(err)
	}
}

func TestACallWithoutCertificateIsDenied(t *testing.T) {
	k := &kms{}
	addr := serve(t, k, grpc.UnaryInterceptor(contentkeygrpc.Authorize))
	c := client(t, addr, insecure.NewCredentials())
	if _, err := c.Seal(t.Context(), &contentkeyv1.SealRequest{Plaintext: []byte("x")}); code(err) != codes.PermissionDenied {
		t.Fatalf("Seal err = %v, want PermissionDenied", err)
	}
}

func TestATLSPeerWithoutVerifiedCertificateIsDenied(t *testing.T) {
	ca := mtlstest.New(t)
	files := ca.Issue(t, t.TempDir(), "content-key-svc")
	cert, err := tls.LoadX509KeyPair(files.Cert, files.Key)
	if err != nil {
		t.Fatal(err)
	}
	creds := credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}})
	addr := serve(t, &kms{}, grpc.Creds(creds), grpc.UnaryInterceptor(contentkeygrpc.Authorize))
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(ca.PEM)
	c := client(t, addr, credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, ServerName: "content-key-svc"}))
	if _, err := c.Seal(t.Context(), &contentkeyv1.SealRequest{Plaintext: []byte("x")}); code(err) != codes.PermissionDenied {
		t.Fatalf("Seal err = %v, want PermissionDenied", err)
	}
}

func TestEachVerdictHasItsCode(t *testing.T) {
	secret := "secret-" + t.Name()
	for _, tc := range []struct {
		name string
		err  error
		call func(contentkeyv1.ConfigSecretsServiceClient) error
		want codes.Code
	}{
		{"empty secret", nil, func(c contentkeyv1.ConfigSecretsServiceClient) error {
			_, err := c.Seal(t.Context(), &contentkeyv1.SealRequest{})
			return err
		}, codes.InvalidArgument},
		{"unknown key", nil, func(c contentkeyv1.ConfigSecretsServiceClient) error {
			_, err := c.Open(t.Context(), &contentkeyv1.OpenRequest{KmsKeyRef: "content", Sealed: []byte("x")})
			return err
		}, codes.InvalidArgument},
		{"undecryptable", contentkey.ErrUndecryptable, func(c contentkeyv1.ConfigSecretsServiceClient) error {
			_, err := c.Open(t.Context(), &contentkeyv1.OpenRequest{KmsKeyRef: "config-secrets", Sealed: []byte(secret)})
			return err
		}, codes.DataLoss},
		{"transient open", errors.New("openbao down: " + secret), func(c contentkeyv1.ConfigSecretsServiceClient) error {
			_, err := c.Open(t.Context(), &contentkeyv1.OpenRequest{KmsKeyRef: "config-secrets", Sealed: []byte(secret)})
			return err
		}, codes.Unavailable},
		{"transient seal", errors.New("openbao down: " + secret), func(c contentkeyv1.ConfigSecretsServiceClient) error {
			_, err := c.Seal(t.Context(), &contentkeyv1.SealRequest{Plaintext: []byte(secret)})
			return err
		}, codes.Unavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := client(t, serve(t, &kms{err: tc.err}), insecure.NewCredentials())
			err := tc.call(c)
			if code(err) != tc.want {
				t.Fatalf("err = %v, want %s", err, tc.want)
			}
			if strings.Contains(status.Convert(err).Message(), "secret-") {
				t.Fatalf("status message %q carries the secret", status.Convert(err).Message())
			}
		})
	}
}
