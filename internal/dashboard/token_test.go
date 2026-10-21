package dashboard_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/martialanouman/sms-platform/internal/dashboard"
	"github.com/martialanouman/sms-platform/internal/platform/scopes"
)

type keySigner struct {
	keys map[string]*ecdsa.PrivateKey
	fail error
}

func newKeySigner(t *testing.T) *keySigner {
	t.Helper()
	k := &keySigner{keys: map[string]*ecdsa.PrivateKey{}}
	for _, kid := range []string{"test-v1", "test-v2"} {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		k.keys[kid] = key
	}
	return k
}

func (k *keySigner) Keys(context.Context) (string, []dashboard.PublicKey, error) {
	return "test-v2", []dashboard.PublicKey{{Kid: "test-v1", Key: &k.keys["test-v1"].PublicKey}, {Kid: "test-v2", Key: &k.keys["test-v2"].PublicKey}}, k.fail
}

func (k *keySigner) Sign(_ context.Context, kid string, input []byte) ([]byte, error) {
	key, ok := k.keys[kid]
	if k.fail != nil || !ok {
		return nil, errors.Join(k.fail, errors.New("unknown kid"))
	}
	digest := sha256.Sum256(input)
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	return append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...), err
}

type brokenStore struct{}

func (brokenStore) Tx(context.Context, func(dashboard.Tx) error) error {
	return errors.New("postgres down")
}

type claims struct {
	Iss, Aud, Sub, Scope string
	Iat, Exp             int64
}

func verify(t *testing.T, jwks []byte, token string) (header map[string]string, c claims) {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token %q", token)
	}
	decode := func(s string, v any) {
		t.Helper()
		data, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		if v != nil {
			if err := json.Unmarshal(data, v); err != nil {
				t.Fatal(err)
			}
		}
	}
	decode(parts[0], &header)
	decode(parts[1], &c)
	var set struct {
		Keys []map[string]string `json:"keys"`
	}
	if err := json.Unmarshal(jwks, &set); err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(set.Keys, func(k map[string]string) bool { return k["kid"] == header["kid"] })
	if i < 0 {
		t.Fatalf("kid %q absent from %s", header["kid"], jwks)
	}
	jwk := set.Keys[i]
	if jwk["kty"] != "EC" || jwk["crv"] != "P-256" || jwk["alg"] != "ES256" || jwk["use"] != "sig" {
		t.Fatalf("jwk %v", jwk)
	}
	x, _ := base64.RawURLEncoding.DecodeString(jwk["x"])
	y, _ := base64.RawURLEncoding.DecodeString(jwk["y"])
	key, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), slices.Concat([]byte{4}, x, y))
	if err != nil {
		t.Fatal(err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		t.Fatalf("signature %q: %v", parts[2], err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(key, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("the signature does not verify under the JWKS")
	}
	return header, c
}

func TestAPersonalTokenCarriesTheScopesItMayAndVerifiesUnderTheJWKS(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	actor := h.actor(t)
	svc := h.svc
	svc.Signer = newKeySigner(t)
	owner := dashboard.Current{OperatorID: actor, Permissions: slices.Concat(scopes.All, scopes.Permissions)}
	before := len(h.log(t))

	issued, err := svc.IssuePersonalToken(ctx, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := slices.DeleteFunc(slices.Clone(scopes.All), func(s string) bool {
		return s == "content:read" || s == "content:erase" || s == "gdpr:erase"
	})
	if !slices.Equal(issued.Scopes, want) {
		t.Fatalf("scopes %v, want %v", issued.Scopes, want)
	}
	jwks, err := svc.JWKS(ctx)
	if err != nil {
		t.Fatal(err)
	}
	header, c := verify(t, jwks, issued.Token)
	if header["alg"] != "ES256" || header["kid"] != "test-v2" || header["typ"] != "JWT" {
		t.Fatalf("header %v", header)
	}
	if c.Iss != publicURL || c.Aud != "gateway-admin" || c.Sub != actor || c.Scope != strings.Join(want, " ") || c.Exp-c.Iat != 3600 ||
		time.Since(time.Unix(c.Iat, 0)).Abs() > time.Minute || !issued.ExpiresAt.Equal(time.Unix(c.Exp, 0)) {
		t.Fatalf("claims %+v, expires %v", c, issued.ExpiresAt)
	}
	if log := h.since(t, before); len(log) != 1 || log[0].Event != dashboard.EventPersonalTokenIssued || log[0].Actor != actor || log[0].Target != "" || !log[0].Success {
		t.Fatalf("logged %+v", log)
	}

	narrowed, err := svc.IssuePersonalToken(ctx, owner, []string{"routes:read", "cdr:read", "routes:read"})
	if err != nil || !slices.Equal(narrowed.Scopes, []string{"cdr:read", "routes:read"}) {
		t.Fatalf("narrowed %v, %v", narrowed.Scopes, err)
	}
	if _, c := verify(t, jwks, narrowed.Token); c.Scope != "cdr:read routes:read" {
		t.Fatalf("narrowed claim %q", c.Scope)
	}
}

func TestAPersonalTokenRefusesAScopeItMayNotCarry(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	actor := h.actor(t)
	svc := h.svc
	svc.Signer = newKeySigner(t)
	owner := dashboard.Current{OperatorID: actor, Permissions: slices.Concat(scopes.All, scopes.Permissions)}
	before := len(h.log(t))
	for _, tc := range []struct {
		c         dashboard.Current
		requested []string
		refused   string
	}{
		{owner, []string{"routes:read", "content:read"}, "content:read"},
		{owner, []string{"content:erase"}, "content:erase"},
		{owner, []string{"gdpr:erase"}, "gdpr:erase"},
		{owner, []string{"operators:read"}, "operators:read"},
		{owner, []string{"nope:read", "routes:read"}, "nope:read"},
		{owner, []string{"content:read", "gdpr:erase"}, "content:read"},
		{dashboard.Current{OperatorID: actor, Permissions: []string{"routes:read"}}, []string{"cdr:read"}, "cdr:read"},
		{dashboard.Current{OperatorID: actor, Permissions: []string{"content:read", "gateway:personal_token"}}, nil, ""},
	} {
		var refused dashboard.ForbiddenScope
		if _, err := svc.IssuePersonalToken(ctx, tc.c, tc.requested); !errors.As(err, &refused) || refused.Scope != tc.refused {
			t.Errorf("%v asking %v: %v, want %q refused", tc.c.Permissions, tc.requested, err, tc.refused)
		}
	}
	broken := svc
	broken.Store = brokenStore{}
	if issued, err := broken.IssuePersonalToken(ctx, owner, nil); err == nil || issued.Token != "" {
		t.Fatalf("a token whose issuance could not be logged: %+v, %v", issued, err)
	}
	svc.Signer.(*keySigner).fail = errors.New("openbao down")
	if _, err := svc.IssuePersonalToken(ctx, owner, nil); !errors.Is(err, dashboard.ErrSignerUnavailable) {
		t.Fatalf("a signer down: %v", err)
	}
	if _, err := svc.JWKS(ctx); !errors.Is(err, dashboard.ErrSignerUnavailable) {
		t.Fatalf("the JWKS with a signer down: %v", err)
	}
	svc.Signer = nil
	if _, err := svc.IssuePersonalToken(ctx, owner, nil); !errors.Is(err, dashboard.ErrSignerUnavailable) {
		t.Fatalf("no signer: %v", err)
	}
	if _, err := svc.JWKS(ctx); !errors.Is(err, dashboard.ErrSignerUnavailable) {
		t.Fatalf("the JWKS without signer: %v", err)
	}
	if log := h.since(t, before); len(log) != 0 {
		t.Fatalf("a refused token was logged: %+v", log)
	}
}
