package http_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/martialanouman/sms-platform/internal/dashboard"
	dashboardhttp "github.com/martialanouman/sms-platform/internal/dashboard/http"
)

type keySigner struct {
	key  *ecdsa.PrivateKey
	fail error
}

func newKeySigner(t *testing.T) *keySigner {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &keySigner{key: key}
}

func (k *keySigner) Keys(context.Context) (string, []dashboard.PublicKey, error) {
	return "test-v1", []dashboard.PublicKey{{Kid: "test-v1", Key: &k.key.PublicKey}}, k.fail
}

func (k *keySigner) Sign(_ context.Context, _ string, input []byte) ([]byte, error) {
	if k.fail != nil {
		return nil, k.fail
	}
	digest := sha256.Sum256(input)
	r, s, err := ecdsa.Sign(rand.Reader, k.key, digest[:])
	return append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...), err
}

func TestAnElevatedOwnerTakesAPersonalTokenThatTheJWKSVerifies(t *testing.T) {
	a := newAPI(t, netip.MustParsePrefix("127.0.0.0/8"))
	a.activate(t)
	c := a.elevated(t)
	got := a.do(t, call{method: "POST", path: "/api/gateway-token", cookie: c, body: `{"scopes":["routes:read","cdr:read"]}`})
	token, _ := got.body["token"].(string)
	expires, _ := time.Parse(time.RFC3339, fmt.Sprint(got.body["expiresAt"]))
	if got.status != 200 || fmt.Sprint(got.body["scopes"]) != "[cdr:read routes:read]" || time.Until(expires) < 59*time.Minute || time.Until(expires) > time.Hour ||
		got.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("issue: %d %s", got.status, got.raw)
	}
	jwks := httptest.NewRecorder()
	dashboardhttp.JWKS(a.svc).ServeHTTP(jwks, httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil))
	if jwks.Code != 200 || jwks.Header().Get("Content-Type") != "application/json" || jwks.Header().Get("Cache-Control") != "max-age=60" {
		t.Fatalf("jwks: %d %v", jwks.Code, jwks.Header())
	}
	if want, err := a.svc.JWKS(context.Background()); err != nil || jwks.Body.String() != string(want) {
		t.Fatalf("jwks %s, want %s: %v", jwks.Body, want, err)
	}
	parts := strings.Split(token, ".")
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	if !strings.Contains(string(payload), `"scope":"cdr:read routes:read"`) {
		t.Fatalf("claims %s", payload)
	}

	for name, tc := range map[string]struct {
		body, refused string
	}{
		"content": {`{"scopes":["routes:read","content:read"]}`, "content:read"},
	} {
		if got := a.do(t, call{method: "POST", path: "/api/gateway-token", cookie: c, body: tc.body}); got.status != 403 || got.code() != "forbidden_scope" ||
			!strings.Contains(got.body["message"].(string), tc.refused) {
			t.Errorf("%s: %d %s", name, got.status, got.raw)
		}
	}
	a.holdOnly(t, "Audit")
	if _, err := a.owner.Exec(context.Background(), `UPDATE dashboard.roles SET permissions = '{content:read,gateway:personal_token}' WHERE name = 'Audit'`); err != nil {
		t.Fatal(err)
	}
	if got := a.do(t, call{method: "POST", path: "/api/gateway-token", cookie: c, body: `{}`}); got.status != 403 || got.code() != "forbidden_scope" ||
		!strings.Contains(got.raw, "aucun scope") {
		t.Errorf("a token without scope: %d %s", got.status, got.raw)
	}
	a.holdOnly(t, "Exploitation")
	if got := a.do(t, call{method: "POST", path: "/api/gateway-token", cookie: c, body: `{}`}); got.status != 403 || got.code() != "forbidden_scope" ||
		!strings.Contains(got.raw, "gateway:personal_token") {
		t.Errorf("a token for an operator without gateway:personal_token: %d %s", got.status, got.raw)
	}
	a.holdOnly(t, "Propriétaire")

	a.signer.fail = errors.New("openbao down")
	if got := a.do(t, call{method: "POST", path: "/api/gateway-token", cookie: c, body: `{}`}); got.status != 503 || got.code() != "service_unavailable" {
		t.Errorf("a signer down: %d %s", got.status, got.raw)
	}
	down := httptest.NewRecorder()
	dashboardhttp.JWKS(a.svc).ServeHTTP(down, httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil))
	if down.Code != 503 || strings.Contains(down.Body.String(), "openbao") {
		t.Errorf("the JWKS with a signer down: %d %s", down.Code, down.Body)
	}
}

func TestTheJWKSIsServedOnItsOwnPathAndNeverByThePublicListener(t *testing.T) {
	a := newAPI(t)
	for _, path := range []string{"/.well-known/jwks.json", "/api/.well-known/jwks.json", "/jwks.json"} {
		if got := a.do(t, call{method: "GET", path: path}); strings.Contains(got.raw, `"keys"`) || strings.Contains(got.raw, "test-v1") {
			t.Errorf("the public listener serves %s: %d %s", path, got.status, got.raw)
		}
	}
	for _, r := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/jwks.json", nil),
		httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json/x", nil),
		httptest.NewRequest(http.MethodPost, "/.well-known/jwks.json", nil),
	} {
		w := httptest.NewRecorder()
		dashboardhttp.JWKS(a.svc).ServeHTTP(w, r)
		if w.Code != 404 || w.Body.Len() != 0 {
			t.Errorf("%s %s: %d %s", r.Method, r.URL.Path, w.Code, w.Body)
		}
	}
}
