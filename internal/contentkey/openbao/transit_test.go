package openbao_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/martialanouman/sms-platform/internal/contentkey"
	"github.com/martialanouman/sms-platform/internal/contentkey/openbao"
	"github.com/martialanouman/sms-platform/internal/platform/openbao/openbaotest"
)

const role = "content-key-svc"

func start(t *testing.T, ttl string) *openbaotest.Bao {
	t.Helper()
	return openbaotest.Start(t, role, ttl,
		openbaotest.Step{Path: "transit/keys/config-secrets", Body: map[string]any{}},
		openbaotest.Step{Path: "transit/keys/other", Body: map[string]any{}},
		openbaotest.Step{Path: "sys/policies/acl/content-key-svc", Body: map[string]any{"policy": `path "transit/encrypt/config-secrets" { capabilities = ["update"] }
path "transit/decrypt/config-secrets" { capabilities = ["update"] }`}},
	)
}

func transit(b *openbaotest.Bao) *openbao.Transit {
	return openbao.New(b.Addr, &http.Client{Timeout: 10 * time.Second}, role, b.JWT)
}

func TestASealedSecretOpensUnderTheSameKey(t *testing.T) {
	b := start(t, "1h")
	tr := transit(b)
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
	resp := b.Admin(t, "transit/encrypt/config-secrets", map[string]string{"plaintext": base64.StdEncoding.EncodeToString(secret)})
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
	tr := transit(b)
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
	if n := b.Reviews.Load(); n != 1 {
		t.Fatalf("%d logins, want 1: a refused ciphertext keeps the token", n)
	}
}

func TestARefusedLoginIsTransient(t *testing.T) {
	b := start(t, "1h")
	sealed, err := transit(b).Encrypt(t.Context(), "config-secrets", []byte("bind-pw"))
	if err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, tr := range map[string]*openbao.Transit{
		"unknown role": openbao.New(b.Addr, &http.Client{Timeout: 10 * time.Second}, "no-such-role", b.JWT),
		"empty token":  openbao.New(b.Addr, &http.Client{Timeout: 10 * time.Second}, role, empty),
	} {
		if _, err := tr.Decrypt(t.Context(), "config-secrets", sealed); err == nil || errors.Is(err, contentkey.ErrUndecryptable) {
			t.Fatalf("%s: err = %v, want a transient error", name, err)
		}
	}
}

func TestAKeyOutsideThePolicyIsRefusedAsTransient(t *testing.T) {
	b := start(t, "1h")
	tr := transit(b)
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
	tr := transit(b)
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
	if n := b.Reviews.Load(); n != 1 {
		t.Fatalf("%d logins, want 1", n)
	}
}

func TestTheTokenIsRenewedPastHalfItsLease(t *testing.T) {
	b := start(t, "6s")
	tr := transit(b)
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
	if n := b.Reviews.Load(); n != 1 {
		t.Fatalf("%d logins before half the lease, want 1", n)
	}
	time.Sleep(time.Until(loggedIn.Add(3600 * time.Millisecond)))
	call()
	if n := b.Reviews.Load(); n != 2 {
		t.Fatalf("%d logins past half the lease, want 2", n)
	}
}

func TestARevokedTokenIsForgotten(t *testing.T) {
	b := start(t, "1h")
	tr := transit(b)
	if _, err := tr.Encrypt(t.Context(), "config-secrets", []byte("x")); err != nil {
		t.Fatal(err)
	}
	b.Admin(t, "sys/leases/revoke-prefix/auth/kubernetes/login", map[string]any{})
	if _, err := tr.Encrypt(t.Context(), "config-secrets", []byte("x")); err == nil {
		t.Fatal("call under a revoked token succeeded")
	}
	if _, err := tr.Encrypt(t.Context(), "config-secrets", []byte("x")); err != nil {
		t.Fatalf("call after a revoked token: %v", err)
	}
	if n := b.Reviews.Load(); n != 2 {
		t.Fatalf("%d logins, want 2", n)
	}
}

func TestEachLoginReadsTheRotatedToken(t *testing.T) {
	b := start(t, "1h")
	tr := transit(b)
	if _, err := tr.Encrypt(t.Context(), "config-secrets", []byte("x")); err != nil {
		t.Fatal(err)
	}
	b.Rotate(t)
	b.Admin(t, "sys/leases/revoke-prefix/auth/kubernetes/login", map[string]any{})
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

func TestARedirectIsNotFollowed(t *testing.T) {
	var reached atomic.Int32
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Add(1)
		_, _ = fmt.Fprint(w, `{"auth":{"client_token":"t","lease_duration":3600},"data":{"ciphertext":"vault:v1:x"}}`)
	}))
	t.Cleanup(elsewhere.Close)
	redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		http.Redirect(w, r, elsewhere.URL+"/v1/auth/kubernetes/login", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirecting.Close)
	jwt := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(jwt, []byte("jwt"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	tr := openbao.New(redirecting.URL, client, role, jwt)
	if _, err := tr.Encrypt(t.Context(), "config-secrets", []byte("x")); err == nil {
		t.Fatal("Encrypt followed a redirect")
	}
	if n := reached.Load(); n != 0 {
		t.Fatalf("the redirect target received %d requests", n)
	}
	if client.CheckRedirect != nil {
		t.Fatal("New changed the caller's client")
	}
}
