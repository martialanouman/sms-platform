package dashboard

import (
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/martialanouman/sms-platform/internal/platform/scopes"
)

const (
	audience         = "gateway-admin"
	personalLifetime = time.Hour
)

var personalExcluded = []string{"content:read", "content:erase", "gdpr:erase"}

// ErrSignerUnavailable is a signer that did not answer, or refused.
var ErrSignerUnavailable = errors.New("signer unavailable")

// ForbiddenScope is a scope a token may not carry; Scope is empty when the token would carry none.
type ForbiddenScope struct {
	Scope string
}

func (e ForbiddenScope) Error() string {
	return "forbidden scope " + e.Scope
}

// PublicKey is a verification key of the tokens, named by Kid.
type PublicKey struct {
	Kid string
	Key *ecdsa.PublicKey
}

// Signer signs tokens with a key it never discloses (spec 08 §8.4).
type Signer interface {
	Keys(ctx context.Context) (latest string, keys []PublicKey, err error)
	// Sign returns the ES256 signature, r and s on 32 bytes each, of input under the key kid.
	Sign(ctx context.Context, kid string, input []byte) ([]byte, error)
}

// PersonalToken is a token issued for a script run by hand, shown once.
type PersonalToken struct {
	Token     string
	Scopes    []string
	ExpiresAt time.Time
}

func personalScopes(held, requested []string) ([]string, error) {
	allowed := slices.DeleteFunc(slices.Clone(held), func(p string) bool {
		return !slices.Contains(scopes.All, p) || slices.Contains(personalExcluded, p)
	})
	if requested == nil {
		requested = allowed
	}
	for _, p := range requested {
		if !slices.Contains(allowed, p) {
			return nil, ForbiddenScope{Scope: p}
		}
	}
	if len(requested) == 0 {
		return nil, ForbiddenScope{}
	}
	return permissions(requested), nil
}

// IssuePersonalToken signs, for the operator of c, a token of one hour carrying requested, or every scope it may when
// requested is nil.
func (s Service) IssuePersonalToken(ctx context.Context, c Current, requested []string) (PersonalToken, error) {
	carried, err := personalScopes(c.Permissions, requested)
	if err != nil {
		return PersonalToken{}, err
	}
	now := time.Now()
	exp := now.Add(personalLifetime).Unix()
	token, err := s.sign(ctx, struct {
		Iss   string `json:"iss"`
		Aud   string `json:"aud"`
		Sub   string `json:"sub"`
		Scope string `json:"scope"`
		Iat   int64  `json:"iat"`
		Exp   int64  `json:"exp"`
	}{s.PublicURL, audience, c.OperatorID, strings.Join(carried, " "), now.Unix(), exp})
	if err != nil {
		return PersonalToken{}, err
	}
	if err := s.Store.Tx(ctx, func(tx Tx) error {
		return tx.Log(ctx, Entry{Actor: c.OperatorID, Event: EventPersonalTokenIssued, Success: true})
	}); err != nil {
		return PersonalToken{}, err
	}
	return PersonalToken{Token: token, Scopes: carried, ExpiresAt: time.Unix(exp, 0)}, nil
}

func (s Service) keys(ctx context.Context) (string, []PublicKey, error) {
	if s.Signer == nil {
		return "", nil, ErrSignerUnavailable
	}
	latest, keys, err := s.Signer.Keys(ctx)
	if err != nil {
		return "", nil, errors.Join(ErrSignerUnavailable, err)
	}
	return latest, keys, nil
}

func (s Service) sign(ctx context.Context, claims any) (string, error) {
	kid, _, err := s.keys(ctx)
	if err != nil {
		return "", err
	}
	header, err := json.Marshal(map[string]string{"alg": "ES256", "kid": kid, "typ": "JWT"})
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	signature, err := s.Signer.Sign(ctx, kid, []byte(unsigned))
	if err != nil {
		return "", errors.Join(ErrSignerUnavailable, err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// JWKS renders every public key of the signer as a JSON Web Key Set.
func (s Service) JWKS(ctx context.Context) ([]byte, error) {
	_, keys, err := s.keys(ctx)
	if err != nil {
		return nil, err
	}
	type jwk struct {
		Kty string `json:"kty"`
		Crv string `json:"crv"`
		Alg string `json:"alg"`
		Use string `json:"use"`
		Kid string `json:"kid"`
		X   string `json:"x"`
		Y   string `json:"y"`
	}
	set := struct {
		Keys []jwk `json:"keys"`
	}{Keys: make([]jwk, len(keys))}
	for i, k := range keys {
		point, err := k.Key.Bytes()
		if err != nil {
			return nil, err
		}
		set.Keys[i] = jwk{Kty: "EC", Crv: "P-256", Alg: "ES256", Use: "sig", Kid: k.Kid,
			X: base64.RawURLEncoding.EncodeToString(point[1:33]), Y: base64.RawURLEncoding.EncodeToString(point[33:])}
	}
	return json.Marshal(set)
}
