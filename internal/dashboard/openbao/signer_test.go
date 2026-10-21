package openbao_test

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/martialanouman/sms-platform/internal/dashboard"
	"github.com/martialanouman/sms-platform/internal/dashboard/openbao"
	"github.com/martialanouman/sms-platform/internal/platform/openbao/openbaotest"
)

func start(t *testing.T) (*openbaotest.Bao, *openbao.Signer) {
	t.Helper()
	b := openbaotest.Start(t, "dashboard", "1h",
		openbaotest.Step{Path: "transit/keys/dashboard-token", Body: map[string]any{"type": "ecdsa-p256"}},
		openbaotest.Step{Path: "transit/keys/other", Body: map[string]any{"type": "ecdsa-p256"}},
		openbaotest.Step{Path: "sys/policies/acl/dashboard", Body: map[string]any{"policy": `path "transit/sign/dashboard-token" { capabilities = ["update"] }
path "transit/keys/dashboard-token" { capabilities = ["read"] }`}},
	)
	return b, openbao.New(b.Addr, &http.Client{Timeout: 10 * time.Second}, "dashboard", b.JWT)
}

func verifies(t *testing.T, keys []dashboard.PublicKey, kid string, input, sig []byte) bool {
	t.Helper()
	i := slices.IndexFunc(keys, func(k dashboard.PublicKey) bool { return k.Kid == kid })
	if i < 0 || len(sig) != 64 {
		t.Fatalf("kid %q among %v, signature of %d bytes", kid, keys, len(sig))
	}
	digest := sha256.Sum256(input)
	return ecdsa.Verify(keys[i].Key, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:]))
}

func TestTransitSignsUnderTheLatestKeyAndPublishesEveryVersion(t *testing.T) {
	b, s := start(t)
	input := []byte("header.payload")
	latest, keys, err := s.Keys(t.Context())
	if err != nil || latest != "dashboard-token-v1" || len(keys) != 1 {
		t.Fatalf("keys: %q %v %v", latest, keys, err)
	}
	sig, err := s.Sign(t.Context(), latest, input)
	if err != nil {
		t.Fatal(err)
	}
	if !verifies(t, keys, latest, input, sig) {
		t.Fatal("the signature does not verify under its key")
	}

	b.Admin(t, "transit/keys/dashboard-token/rotate", map[string]any{})
	latest, keys, err = s.Keys(t.Context())
	if err != nil || latest != "dashboard-token-v2" || len(keys) != 2 {
		t.Fatalf("after rotation: %q %v %v", latest, keys, err)
	}
	for _, kid := range []string{"dashboard-token-v1", "dashboard-token-v2"} {
		sig, err := s.Sign(t.Context(), kid, input)
		if err != nil {
			t.Fatal(err)
		}
		if !verifies(t, keys, kid, input, sig) {
			t.Fatalf("%s does not verify under its key", kid)
		}
		other := "dashboard-token-v1"
		if kid == other {
			other = "dashboard-token-v2"
		}
		if verifies(t, keys, other, input, sig) {
			t.Fatalf("%s verifies under %s", kid, other)
		}
	}
	if _, err := s.Sign(t.Context(), "other-v1", input); err == nil {
		t.Fatal("a kid of another key signed")
	}
	if _, err := s.Sign(t.Context(), "dashboard-token-v9", input); err == nil {
		t.Fatal("an absent version signed")
	}
}

func TestASignerOutsideItsPolicyOrUnreachableFails(t *testing.T) {
	b, _ := start(t)
	for name, s := range map[string]*openbao.Signer{
		"other role":  openbao.New(b.Addr, &http.Client{Timeout: 10 * time.Second}, "no-such-role", b.JWT),
		"unreachable": openbao.New("http://127.0.0.1:1", &http.Client{Timeout: time.Second}, "dashboard", b.JWT),
	} {
		if _, _, err := s.Keys(t.Context()); err == nil {
			t.Errorf("%s: keys read", name)
		}
		if _, err := s.Sign(t.Context(), "dashboard-token-v1", []byte("x")); err == nil {
			t.Errorf("%s: signed", name)
		}
	}
	if _, err := openbao.New(b.Addr, &http.Client{}, "dashboard", b.JWT).Sign(t.Context(), "dashboard-token", []byte("x")); err == nil {
		t.Fatalf("a kid without version: %v", err)
	}
}

func publicPEM(t *testing.T, key any) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), "\n", `\n`)
}

func answering(t *testing.T, transit string) *openbao.Signer {
	t.Helper()
	jwt := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(jwt, []byte("jwt"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/auth/kubernetes/login" {
			_, _ = fmt.Fprint(w, `{"auth":{"client_token":"t","lease_duration":3600}}`)
			return
		}
		_, _ = fmt.Fprint(w, transit)
	}))
	t.Cleanup(srv.Close)
	return openbao.New(srv.URL, &http.Client{Timeout: 5 * time.Second}, "dashboard", jwt)
}

func TestAnAnswerWithoutAnES256SignatureOrAP256KeyIsAnError(t *testing.T) {
	if sig, err := answering(t, `{"data":{"signature":"vault:v1:"}}`).Sign(t.Context(), "dashboard-token-v1", []byte("x")); err == nil {
		t.Fatalf("signed %x without a signature", sig)
	}
	p256, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p384, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	edwards, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keys := func(latest int, pem string) string {
		return fmt.Sprintf(`{"data":{"latest_version":%d,"keys":{"1":{"public_key":"%s"}}}}`, latest, pem)
	}
	if _, _, err := answering(t, keys(1, publicPEM(t, &p256.PublicKey))).Keys(t.Context()); err != nil {
		t.Fatalf("a P-256 key: %v", err)
	}
	for name, transit := range map[string]string{
		"no latest version": keys(0, publicPEM(t, &p256.PublicKey)),
		"no key":            `{"data":{"latest_version":1,"keys":{}}}`,
		"not PEM":           keys(1, "x"),
		"P-384":             keys(1, publicPEM(t, &p384.PublicKey)),
		"Ed25519":           keys(1, publicPEM(t, edwards)),
	} {
		if _, keys, err := answering(t, transit).Keys(t.Context()); err == nil || strings.Contains(err.Error(), "%!") {
			t.Errorf("%s: %v, %v", name, keys, err)
		}
	}
}
