package openbao_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/martialanouman/sms-platform/internal/contentkey"
	"github.com/martialanouman/sms-platform/internal/contentkey/openbao"
	"github.com/martialanouman/sms-platform/internal/platform/mtls/mtlstest"
)

const (
	root = "root"
	role = "content-key-svc"
)

type bao struct {
	addr    string
	jwt     string
	reviews atomic.Int32
}

func start(t *testing.T, ttl string) *bao {
	t.Helper()
	b := &bao{jwt: filepath.Join(t.TempDir(), "token")}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	review := http.NewServeMux()
	review.HandleFunc("POST /apis/authentication.k8s.io/v1/tokenreviews", func(w http.ResponseWriter, r *http.Request) {
		b.reviews.Add(1)
		var in struct {
			Spec struct {
				Token string `json:"token"`
			} `json:"spec"`
		}
		current, err := os.ReadFile(b.jwt)
		if json.NewDecoder(r.Body).Decode(&in) != nil || err != nil || in.Spec.Token != string(current) {
			_, _ = fmt.Fprint(w, `{"apiVersion":"authentication.k8s.io/v1","kind":"TokenReview","status":{"authenticated":false}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"apiVersion":"authentication.k8s.io/v1","kind":"TokenReview","status":{"authenticated":true,`+
			`"user":{"username":"system:serviceaccount:sms:content-key-svc","uid":"sa-uid"},"audiences":["openbao"]}}`)
	})
	srv := &http.Server{Handler: review, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	ctr, err := testcontainers.Run(t.Context(), "openbao/openbao:2.7.1",
		testcontainers.WithCmd("server", "-dev", "-dev-root-token-id="+root, "-dev-listen-address=0.0.0.0:8200"),
		testcontainers.WithExposedPorts("8200/tcp"),
		testcontainers.WithHostPortAccess(port),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/v1/sys/health").WithPort("8200/tcp")),
	)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := ctr.PortEndpoint(t.Context(), "8200/tcp", "http")
	if err != nil {
		t.Fatal(err)
	}
	b.addr = endpoint
	t.Cleanup(func() {
		if t.Failed() {
			r, _ := ctr.Logs(context.Background())
			data, _ := io.ReadAll(r)
			t.Logf("openbao logs:\n%s", data)
		}
	})
	for _, step := range []struct {
		path string
		body any
	}{
		{"sys/mounts/transit", map[string]any{"type": "transit"}},
		{"transit/keys/config-secrets", map[string]any{}},
		{"transit/keys/other", map[string]any{}},
		{"sys/policies/acl/content-key-svc", map[string]any{"policy": `path "transit/encrypt/config-secrets" { capabilities = ["update"] }
path "transit/decrypt/config-secrets" { capabilities = ["update"] }`}},
		{"sys/auth/kubernetes", map[string]any{"type": "kubernetes"}},
		{"auth/kubernetes/config", map[string]any{"kubernetes_host": fmt.Sprintf("http://%s:%d", testcontainers.HostInternal, port),
			"disable_local_ca_jwt": true, "kubernetes_ca_cert": string(mtlstest.New(t).PEM)}},
		{"auth/kubernetes/role/" + role, map[string]any{"bound_service_account_names": "content-key-svc",
			"bound_service_account_namespaces": "sms", "audience": "openbao", "token_ttl": ttl, "token_max_ttl": ttl,
			"token_policies": "content-key-svc", "token_no_default_policy": true}},
	} {
		b.admin(t, step.path, step.body)
	}
	b.rotate(t)
	return b
}

func (b *bao) rotate(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(b.jwt, []byte(serviceAccountJWT(t)), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (b *bao) admin(t *testing.T, path string, body any) []byte {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, b.addr+"/v1/"+path, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Vault-Token", root)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s: status %d: %s", path, resp.StatusCode, msg)
	}
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func serviceAccountJWT(t *testing.T) string {
	t.Helper()
	enc := func(v any) string {
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(data)
	}
	now := time.Now().Unix()
	unsigned := enc(map[string]string{"alg": "ES256", "typ": "JWT"}) + "." + enc(map[string]any{
		"iss": "https://kubernetes.default.svc", "sub": "system:serviceaccount:sms:content-key-svc", "aud": []string{"openbao"},
		"iat": now, "nbf": now, "exp": now + 3600,
		"kubernetes.io": map[string]any{"namespace": "sms", "serviceaccount": map[string]string{"name": "content-key-svc", "uid": "sa-uid"}},
	})
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...))
}

func (b *bao) transit() *openbao.Transit {
	return openbao.New(b.addr, &http.Client{Timeout: 10 * time.Second}, role, b.jwt)
}

func TestASealedSecretOpensUnderTheSameKey(t *testing.T) {
	b := start(t, "1h")
	tr := b.transit()
	secret := []byte("bind-pw-" + t.Name())
	sealed, err := tr.Encrypt(t.Context(), "config-secrets", secret)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(sealed, []byte("vault:v1:")) || bytes.Contains(sealed, secret) {
		t.Fatalf("sealed %q", sealed)
	}
	plain, err := tr.Decrypt(t.Context(), "config-secrets", sealed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain, secret) {
		t.Fatalf("opened %q, want %q", plain, secret)
	}
	var byOpenBao struct {
		Data struct {
			Ciphertext string `json:"ciphertext"`
		} `json:"data"`
	}
	resp := b.admin(t, "transit/encrypt/config-secrets", map[string]string{"plaintext": base64.StdEncoding.EncodeToString(secret)})
	if err := json.Unmarshal(resp, &byOpenBao); err != nil {
		t.Fatal(err)
	}
	plain, err = tr.Decrypt(t.Context(), "config-secrets", []byte(byOpenBao.Data.Ciphertext))
	if err != nil || !bytes.Equal(plain, secret) {
		t.Fatalf("opened OpenBao's own ciphertext as %q, %v; want %q", plain, err, secret)
	}
}

func TestATamperedSealedSecretIsUndecryptable(t *testing.T) {
	b := start(t, "1h")
	tr := b.transit()
	sealed, err := tr.Encrypt(t.Context(), "config-secrets", []byte("bind-pw"))
	if err != nil {
		t.Fatal(err)
	}
	tampered := bytes.Clone(sealed)
	tampered[len("vault:v1:")+4] ^= 'A' ^ 'B'
	if bytes.Equal(tampered, sealed) {
		t.Fatal("not tampered")
	}
	for _, s := range [][]byte{tampered, []byte("vault:v1:" + base64.StdEncoding.EncodeToString([]byte("garbage-garbage-garbage")))} {
		if _, err := tr.Decrypt(t.Context(), "config-secrets", s); !errors.Is(err, contentkey.ErrUndecryptable) {
			t.Fatalf("Decrypt(%q) err = %v, want ErrUndecryptable", s, err)
		}
	}
	if n := b.reviews.Load(); n != 1 {
		t.Fatalf("%d logins, want 1: a refused ciphertext keeps the token", n)
	}
}

func TestARefusedLoginIsTransient(t *testing.T) {
	b := start(t, "1h")
	sealed, err := b.transit().Encrypt(t.Context(), "config-secrets", []byte("bind-pw"))
	if err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, tr := range map[string]*openbao.Transit{
		"unknown role": openbao.New(b.addr, &http.Client{Timeout: 10 * time.Second}, "no-such-role", b.jwt),
		"empty token":  openbao.New(b.addr, &http.Client{Timeout: 10 * time.Second}, role, empty),
	} {
		if _, err := tr.Decrypt(t.Context(), "config-secrets", sealed); err == nil || errors.Is(err, contentkey.ErrUndecryptable) {
			t.Fatalf("%s: err = %v, want a transient error", name, err)
		}
	}
}

func TestAKeyOutsideThePolicyIsRefusedAsTransient(t *testing.T) {
	b := start(t, "1h")
	tr := b.transit()
	if _, err := tr.Encrypt(t.Context(), "config-secrets", []byte("x")); err != nil {
		t.Fatal(err)
	}
	_, err := tr.Decrypt(t.Context(), "other", []byte("vault:v1:AAAA"))
	if err == nil || errors.Is(err, contentkey.ErrUndecryptable) {
		t.Fatalf("err = %v, want a transient error", err)
	}
	if !strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("err = %q, want the status and not the body", err)
	}
}

func TestConcurrentFirstCallsLogInOnce(t *testing.T) {
	b := start(t, "1h")
	tr := b.transit()
	var wg sync.WaitGroup
	gate := make(chan struct{})
	errs := make(chan error, 20)
	for range 20 {
		wg.Go(func() {
			<-gate
			_, err := tr.Encrypt(t.Context(), "config-secrets", []byte("x"))
			errs <- err
		})
	}
	close(gate)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tr.Encrypt(t.Context(), "config-secrets", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if n := b.reviews.Load(); n != 1 {
		t.Fatalf("%d logins, want 1", n)
	}
}

func TestTheTokenIsRenewedPastHalfItsLease(t *testing.T) {
	b := start(t, "6s")
	tr := b.transit()
	call := func() {
		t.Helper()
		if _, err := tr.Encrypt(t.Context(), "config-secrets", []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	loggedIn := time.Now()
	call()
	time.Sleep(time.Until(loggedIn.Add(2400 * time.Millisecond)))
	call()
	if n := b.reviews.Load(); n != 1 {
		t.Fatalf("%d logins before half the lease, want 1", n)
	}
	time.Sleep(time.Until(loggedIn.Add(3600 * time.Millisecond)))
	call()
	if n := b.reviews.Load(); n != 2 {
		t.Fatalf("%d logins past half the lease, want 2", n)
	}
}

func TestARevokedTokenIsForgotten(t *testing.T) {
	b := start(t, "1h")
	tr := b.transit()
	if _, err := tr.Encrypt(t.Context(), "config-secrets", []byte("x")); err != nil {
		t.Fatal(err)
	}
	b.admin(t, "sys/leases/revoke-prefix/auth/kubernetes/login", map[string]any{})
	if _, err := tr.Encrypt(t.Context(), "config-secrets", []byte("x")); err == nil {
		t.Fatal("call under a revoked token succeeded")
	}
	if _, err := tr.Encrypt(t.Context(), "config-secrets", []byte("x")); err != nil {
		t.Fatalf("call after a revoked token: %v", err)
	}
	if n := b.reviews.Load(); n != 2 {
		t.Fatalf("%d logins, want 2", n)
	}
}

func TestEachLoginReadsTheRotatedToken(t *testing.T) {
	b := start(t, "1h")
	tr := b.transit()
	if _, err := tr.Encrypt(t.Context(), "config-secrets", []byte("x")); err != nil {
		t.Fatal(err)
	}
	b.rotate(t)
	b.admin(t, "sys/leases/revoke-prefix/auth/kubernetes/login", map[string]any{})
	_, _ = tr.Encrypt(t.Context(), "config-secrets", []byte("x"))
	if _, err := tr.Encrypt(t.Context(), "config-secrets", []byte("x")); err != nil {
		t.Fatalf("login after the token rotated: %v", err)
	}
}

func stuckLogin(t *testing.T) (string, <-chan struct{}) {
	t.Helper()
	arrived := make(chan struct{}, 10)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		arrived <- struct{}{}
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	return srv.URL, arrived
}

func answering(t *testing.T, transit string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/auth/kubernetes/login" {
			_, _ = fmt.Fprint(w, `{"auth":{"client_token":"t","lease_duration":3600}}`)
			return
		}
		_, _ = fmt.Fprint(w, transit)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestTheCallersDeadlineBoundsTheLogin(t *testing.T) {
	jwt := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(jwt, []byte("jwt"), 0o600); err != nil {
		t.Fatal(err)
	}
	addr, arrived := stuckLogin(t)
	tr := openbao.New(addr, &http.Client{}, role, jwt)
	deadline := func() error {
		ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, err := tr.Encrypt(ctx, "config-secrets", []byte("x"))
			done <- err
		}()
		select {
		case err := <-done:
			return err
		case <-time.After(5 * time.Second):
			t.Fatal("the call outlived its deadline")
			return nil
		}
	}
	if err := deadline(); err == nil {
		t.Fatal("a stuck login succeeded")
	}
	<-arrived
	holder, cancel := context.WithCancel(context.WithoutCancel(t.Context()))
	t.Cleanup(cancel)
	go func() { _, _ = tr.Encrypt(holder, "config-secrets", []byte("x")) }()
	<-arrived
	if err := deadline(); err == nil {
		t.Fatal("a call behind a stuck login succeeded")
	}
	if len(arrived) != 0 {
		t.Fatal("the call behind the stuck login logged in itself")
	}
}

func TestAResponseWithoutItsFieldIsAnError(t *testing.T) {
	jwt := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(jwt, []byte("jwt"), 0o600); err != nil {
		t.Fatal(err)
	}
	tr := openbao.New(answering(t, `{}`), &http.Client{Timeout: 5 * time.Second}, role, jwt)
	if sealed, err := tr.Encrypt(t.Context(), "config-secrets", []byte("x")); err == nil {
		t.Fatalf("Encrypt = %q without error", sealed)
	}
	if plain, err := tr.Decrypt(t.Context(), "config-secrets", []byte("vault:v1:x")); err == nil || errors.Is(err, contentkey.ErrUndecryptable) {
		t.Fatalf("Decrypt = %q, %v; want a transient error", plain, err)
	}
}
