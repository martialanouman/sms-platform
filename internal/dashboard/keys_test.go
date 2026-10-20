package dashboard_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"net/netip"
	"strings"
	"testing"

	"github.com/martialanouman/sms-platform/internal/dashboard"
)

func keys(t *testing.T, seed byte) dashboard.Keys {
	t.Helper()
	k, err := dashboard.NewKeys(bytes.Repeat([]byte{seed}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestNewKeysRefusesAShortSecret(t *testing.T) {
	if _, err := dashboard.NewKeys(make([]byte, 31)); err == nil {
		t.Fatal("a 31-byte secret was accepted")
	}
}

func TestASealOpensOnlyUntouchedAndUnderItsKey(t *testing.T) {
	k := keys(t, 1)
	token := dashboard.NewToken()
	cookie := k.Seal(token)
	if got, ok := k.Open(cookie); !ok || got != token {
		t.Fatalf("open = %q, %v; want %q", got, ok, token)
	}
	other := dashboard.NewToken()
	mac := cookie[strings.LastIndexByte(cookie, '.'):]
	for name, c := range map[string]string{
		"other key":      keys(t, 2).Seal(token),
		"other token":    other + mac,
		"mac cut":        cookie[:len(cookie)-1],
		"no mac":         token,
		"empty":          "",
		"mac of nothing": "." + mac[1:],
	} {
		if got, ok := k.Open(c); ok {
			t.Errorf("%s: opened as %q", name, got)
		}
	}
}

func TestTheDomainsOfTheKeyNeverMeet(t *testing.T) {
	k := keys(t, 1)
	value := "203.0.113.7"
	seal := k.Seal(value)
	mac, err := base64.RawURLEncoding.DecodeString(seal[strings.LastIndexByte(seal, '.')+1:])
	if err != nil {
		t.Fatal(err)
	}
	email, source := k.EmailSubject(value), k.SourceSubject(netip.MustParseAddr(value))
	if bytes.Equal(mac, email) || bytes.Equal(mac, source) || bytes.Equal(email, source) {
		t.Fatal("two domains share a subject")
	}
	if !bytes.Equal(email, k.EmailSubject(value)) || bytes.Equal(email, keys(t, 2).EmailSubject(value)) {
		t.Fatal("a subject is not stable under its key, or not bound to it")
	}
}

func TestASourceIsAnIPv4AddressOrAnIPv6Slash64(t *testing.T) {
	k := keys(t, 1)
	subject := func(s string) []byte { return k.SourceSubject(netip.MustParseAddr(s)) }
	if !bytes.Equal(subject("2001:db8:1:2::1"), subject("2001:db8:1:2:ffff::9")) {
		t.Error("two addresses of one /64 are two sources")
	}
	if bytes.Equal(subject("2001:db8:1:2::1"), subject("2001:db8:1:3::1")) {
		t.Error("two /64 are one source")
	}
	if bytes.Equal(subject("203.0.113.7"), subject("203.0.113.8")) {
		t.Error("two IPv4 addresses are one source")
	}
	if !bytes.Equal(subject("203.0.113.7"), subject("::ffff:203.0.113.7")) {
		t.Error("an IPv4-mapped address is another source")
	}
}

func TestATokenIs256RandomBitsAndItsDigestIsSHA256(t *testing.T) {
	a, b := dashboard.NewToken(), dashboard.NewToken()
	raw, err := base64.RawURLEncoding.DecodeString(a)
	if err != nil || len(raw) != 32 || a == b {
		t.Fatalf("tokens %q, %q (%v)", a, b, err)
	}
	if sum := sha256.Sum256([]byte(a)); !bytes.Equal(dashboard.Digest(a), sum[:]) {
		t.Fatal("digest is not the SHA-256 of the token")
	}
}
