package adminapi_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/martialanouman/sms-platform/internal/adminapi"
	errs "github.com/martialanouman/sms-platform/internal/platform/errors"
)

const (
	issuer   = "https://dashboard.test"
	operator = "0199f0a2-7c1e-7000-8000-000000000001"
)

var now = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

type keys map[string]*ecdsa.PrivateKey

func (k keys) Key(_ context.Context, kid string) (*ecdsa.PublicKey, error) {
	key, ok := k[kid]
	if !ok {
		return nil, errors.New("unknown kid")
	}
	return &key.PublicKey, nil
}

func newKeys(t *testing.T, kids ...string) keys {
	t.Helper()
	k := keys{}
	for _, kid := range kids {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		k[kid] = key
	}
	return k
}

func sign(t *testing.T, key *ecdsa.PrivateKey, header, claims map[string]any) string {
	t.Helper()
	h, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	c, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	unsigned := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(c)
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	signature := append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func header(kid string) map[string]any {
	return map[string]any{"alg": "ES256", "kid": kid, "typ": "JWT"}
}

func claims() map[string]any {
	return map[string]any{"iss": issuer, "aud": "gateway-admin", "sub": operator, "scope": "audit:read routes:write",
		"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(4 * time.Minute).Unix()}
}

func with(c map[string]any, key string, value any) map[string]any {
	c[key] = value
	return c
}

func verifier(k keys) adminapi.Verifier {
	return adminapi.Verifier{Keys: k, Issuer: issuer, Now: func() time.Time { return now }}
}

func TestATokenOfTheIssuerGivesItsOperatorAndTheScopesOfTheContract(t *testing.T) {
	k := newKeys(t, "v1", "v2")
	token := sign(t, k["v2"], header("v2"), with(claims(), "scope", "audit:read routes:write admin:read"))

	p, err := verifier(k).Verify(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if p.Subject != operator || !slices.Equal(p.Scopes, []string{"audit:read", "routes:write"}) {
		t.Fatalf("principal = %+v", p)
	}
}

func TestATokenIsRefusedUnlessEveryClaimIsTheIssuers(t *testing.T) {
	k := newKeys(t, "v1", "v2")
	other := newKeys(t, "v1")
	valid := sign(t, k["v1"], header("v1"), claims())
	tampered := valid[:len(valid)-4] + "AAAA"
	dot := strings.LastIndex(valid, ".")
	raw, err := base64.RawURLEncoding.DecodeString(valid[dot+1:])
	if err != nil {
		t.Fatal(err)
	}
	padded := valid[:dot+1] + base64.RawURLEncoding.EncodeToString(slices.Concat(raw[:32], []byte{0, 0}, raw[32:]))

	for name, token := range map[string]string{
		"signed by another key":     sign(t, other["v1"], header("v1"), claims()),
		"signed under another kid":  sign(t, k["v1"], header("v2"), claims()),
		"unknown kid":               sign(t, k["v1"], header("v3"), claims()),
		"tampered signature":        tampered,
		"alg none":                  sign(t, k["v1"], with(header("v1"), "alg", "none"), claims()),
		"alg ES384":                 sign(t, k["v1"], with(header("v1"), "alg", "ES384"), claims()),
		"another issuer":            sign(t, k["v1"], header("v1"), with(claims(), "iss", issuer+"/")),
		"another audience":          sign(t, k["v1"], header("v1"), with(claims(), "aud", "gateway")),
		"audience as a list":        sign(t, k["v1"], header("v1"), with(claims(), "aud", []string{"gateway-admin"})),
		"expired at this second":    sign(t, k["v1"], header("v1"), with(claims(), "exp", now.Unix())),
		"no exp":                    sign(t, k["v1"], header("v1"), with(claims(), "exp", nil)),
		"sub not an operator id":    sign(t, k["v1"], header("v1"), with(claims(), "sub", "service:hub")),
		"no sub":                    sign(t, k["v1"], header("v1"), with(claims(), "sub", "")),
		"two parts":                 valid[:len(valid)-87],
		"empty":                     "",
		"signature of wrong length": valid + "AA",
		"s padded with zeros":       padded,
		"four parts":                valid + ".e30",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := verifier(k).Verify(context.Background(), token)
			if !errors.Is(err, errs.ErrUnauthenticated) {
				t.Fatalf("err = %v, want unauthenticated", err)
			}
		})
	}
}

func TestAPrincipalHoldsOnlyItsScopes(t *testing.T) {
	p := adminapi.Principal{Subject: operator, Scopes: []string{"audit:read"}}
	if !p.Has("audit:read") || p.Has("audit:write") {
		t.Fatalf("Has is wrong on %+v", p)
	}
}
