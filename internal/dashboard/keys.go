package dashboard

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/netip"
	"strings"
)

// Keys derives, from the one secret of the service, the seal of the session cookie and the subjects of the
// brute-force counters, each under its own domain.
type Keys struct {
	secret []byte
}

// NewKeys refuses a secret shorter than 32 bytes.
func NewKeys(secret []byte) (Keys, error) {
	if len(secret) < 32 {
		return Keys{}, errors.New("the key of the dashboard is shorter than 32 bytes")
	}
	return Keys{secret: secret}, nil
}

func (k Keys) mac(domain, value string) []byte {
	m := hmac.New(sha256.New, k.secret)
	m.Write([]byte(domain + ":" + value))
	return m.Sum(nil)
}

// Seal is the cookie value of a session token.
func (k Keys) Seal(token string) string {
	return token + "." + base64.RawURLEncoding.EncodeToString(k.mac("session", token))
}

// Open is the token of a cookie value whose seal is intact.
func (k Keys) Open(cookie string) (string, bool) {
	token, seal, found := strings.Cut(cookie, ".")
	mac, err := base64.RawURLEncoding.DecodeString(seal)
	if !found || err != nil || !hmac.Equal(mac, k.mac("session", token)) {
		return "", false
	}
	return token, true
}

// EmailSubject is the counter subject of a submitted email, never kept in clear.
func (k Keys) EmailSubject(email string) []byte {
	return k.mac("email", email)
}

// MfaSubject is the counter subject of an operator's second-factor attempts, across its sessions.
func (k Keys) MfaSubject(operatorID string) []byte {
	return k.mac("mfa", operatorID)
}

// SourceSubject is the counter subject of a source address: an IPv4 address, or the /64 of an IPv6 one.
func (k Keys) SourceSubject(addr netip.Addr) []byte {
	addr = addr.Unmap()
	if addr.Is6() {
		addr = netip.PrefixFrom(addr, 64).Masked().Addr()
	}
	return k.mac("source", addr.String())
}

// NewToken is 256 random bits, base64url.
func NewToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Digest is what the database keeps of a token.
func Digest(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
