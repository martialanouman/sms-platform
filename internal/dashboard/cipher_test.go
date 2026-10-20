package dashboard_test

import (
	"bytes"
	"testing"

	"github.com/martialanouman/sms-platform/internal/dashboard"
)

func totpKey(t *testing.T, seed byte) dashboard.TotpKey {
	t.Helper()
	k, err := dashboard.NewTotpKey(bytes.Repeat([]byte{seed}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestNewTotpKeyWantsExactly32Bytes(t *testing.T) {
	for _, n := range []int{0, 16, 24, 31, 33, 64} {
		if _, err := dashboard.NewTotpKey(make([]byte, n)); err == nil {
			t.Errorf("a %d-byte key was accepted", n)
		}
	}
}

func TestASealedSecretOpensOnlyForItsOperatorUnderItsKey(t *testing.T) {
	k := totpKey(t, 1)
	secret := []byte("12345678901234567890")
	sealed := dashboard.SealTotp(k, "op-a", secret)
	if len(sealed) != 1+12+len(secret)+16 || sealed[0] != 1 || bytes.Contains(sealed, secret) {
		t.Fatalf("sealed = %x", sealed)
	}
	if bytes.Equal(sealed, dashboard.SealTotp(k, "op-a", secret)) {
		t.Error("two seals of the same secret are equal: the nonce repeats")
	}
	if got, err := dashboard.OpenTotp(k, "op-a", sealed); err != nil || !bytes.Equal(got, secret) {
		t.Fatalf("open = %q, %v", got, err)
	}
	version := bytes.Clone(sealed)
	version[0] = 2
	tampered := bytes.Clone(sealed)
	tampered[len(tampered)-1] ^= 1
	for name, open := range map[string]func() ([]byte, error){
		"other operator": func() ([]byte, error) { return dashboard.OpenTotp(k, "op-b", sealed) },
		"other key":      func() ([]byte, error) { return dashboard.OpenTotp(totpKey(t, 2), "op-a", sealed) },
		"other version":  func() ([]byte, error) { return dashboard.OpenTotp(k, "op-a", version) },
		"tampered":       func() ([]byte, error) { return dashboard.OpenTotp(k, "op-a", tampered) },
		"truncated":      func() ([]byte, error) { return dashboard.OpenTotp(k, "op-a", sealed[:12]) },
		"empty":          func() ([]byte, error) { return dashboard.OpenTotp(k, "op-a", nil) },
	} {
		if got, err := open(); err == nil {
			t.Errorf("%s: opened %q", name, got)
		}
	}
}
