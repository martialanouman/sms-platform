// Package adminapitest signs operator tokens for the tests of admin-api-svc.
package adminapitest

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/martialanouman/sms-platform/internal/adminapi"
)

// Issuer is the iss of the tokens Keys signs.
const Issuer = "https://dashboard.test"

// Operator is the sub of the tokens Keys signs.
const Operator = "0199f0a2-7c1e-7000-8000-000000000001"

// Keys is a key set of the dashboard, private keys by kid.
type Keys map[string]*ecdsa.PrivateKey

// NewKeys is a key set of a fresh P-256 key per kid.
func NewKeys(t *testing.T, kids ...string) Keys {
	t.Helper()
	k := Keys{}
	for _, kid := range kids {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		k[kid] = key
	}
	return k
}

// Key is the public key of kid.
func (k Keys) Key(_ context.Context, kid string) (*ecdsa.PublicKey, error) {
	key, ok := k[kid]
	if !ok {
		return nil, errors.New("unknown kid")
	}
	return &key.PublicKey, nil
}

// Verifier is the verifier of the tokens k signs.
func (k Keys) Verifier() adminapi.Verifier {
	return adminapi.Verifier{Keys: k, Issuer: Issuer, Now: time.Now}
}

// Token is a token of five minutes for Operator, signed under kid, carrying scope.
func (k Keys) Token(t *testing.T, kid, scope string) string {
	t.Helper()
	h, err := json.Marshal(map[string]string{"alg": "ES256", "kid": kid, "typ": "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := json.Marshal(map[string]any{"iss": Issuer, "aud": "gateway-admin", "sub": Operator, "scope": scope,
		"iat": time.Now().Unix(), "exp": time.Now().Add(5 * time.Minute).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	unsigned := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(c)
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, k[kid], digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...))
}
