package adminapi

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"regexp"
	"slices"
	"strings"
	"time"

	errs "github.com/martialanouman/sms-platform/internal/platform/errors"
	"github.com/martialanouman/sms-platform/internal/platform/scopes"
)

const audience = "gateway-admin"

var operatorID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Principal is the operator behind a request and the scopes of the contract its token carries.
type Principal struct {
	Subject string
	Scopes  []string
}

// Has is whether p holds scope.
func (p Principal) Has(scope string) bool {
	return slices.Contains(p.Scopes, scope)
}

// Keys is the key set of the token issuer: the public key of kid, or an error when it has none.
type Keys interface {
	Key(ctx context.Context, kid string) (*ecdsa.PublicKey, error)
}

// Verifier checks the tokens the dashboard issues (ADR-0019).
type Verifier struct {
	Keys   Keys
	Issuer string
	Now    func() time.Time
}

// Verify is the principal of token, or errs.ErrUnauthenticated.
func (v Verifier) Verify(ctx context.Context, token string) (Principal, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Principal{}, errs.ErrUnauthenticated
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	var claims struct {
		Iss   string `json:"iss"`
		Aud   string `json:"aud"`
		Sub   string `json:"sub"`
		Scope string `json:"scope"`
		Exp   *int64 `json:"exp"`
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(signature) != 64 || decode(parts[0], &header) != nil || header.Alg != "ES256" {
		return Principal{}, errs.ErrUnauthenticated
	}
	key, err := v.Keys.Key(ctx, header.Kid)
	if err != nil {
		return Principal{}, errs.ErrUnauthenticated
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r, s := new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])
	if !ecdsa.Verify(key, digest[:], r, s) || decode(parts[1], &claims) != nil {
		return Principal{}, errs.ErrUnauthenticated
	}
	if claims.Iss != v.Issuer || claims.Aud != audience || claims.Exp == nil || v.Now().Unix() >= *claims.Exp ||
		!operatorID.MatchString(claims.Sub) {
		return Principal{}, errs.ErrUnauthenticated
	}
	p := Principal{Subject: claims.Sub}
	for _, scope := range strings.Fields(claims.Scope) {
		if slices.Contains(scopes.All, scope) {
			p.Scopes = append(p.Scopes, scope)
		}
	}
	return p, nil
}

func decode(segment string, into any) error {
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, into)
}
