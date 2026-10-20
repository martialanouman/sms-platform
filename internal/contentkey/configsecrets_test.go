package contentkey_test

import (
	"context"
	"errors"
	"testing"

	"github.com/martialanouman/sms-platform/internal/contentkey"
)

type call struct {
	op, key string
	data    []byte
}

type kms struct {
	calls []call
	out   []byte
	err   error
}

func (k *kms) Encrypt(_ context.Context, key string, plaintext []byte) ([]byte, error) {
	k.calls = append(k.calls, call{"encrypt", key, plaintext})
	return k.out, k.err
}

func (k *kms) Decrypt(_ context.Context, key string, sealed []byte) ([]byte, error) {
	k.calls = append(k.calls, call{"decrypt", key, sealed})
	return k.out, k.err
}

func TestSealEncryptsUnderTheConfigSecretsKey(t *testing.T) {
	k := &kms{out: []byte("vault:v1:sealed")}
	sealed, ref, err := contentkey.ConfigSecrets{KMS: k}.Seal(t.Context(), []byte("bind-pw"))
	if err != nil {
		t.Fatal(err)
	}
	if string(sealed) != "vault:v1:sealed" || ref != "config-secrets" {
		t.Fatalf("Seal = %q, %q", sealed, ref)
	}
	if c := k.calls[0]; len(k.calls) != 1 || c.op != "encrypt" || c.key != "config-secrets" || string(c.data) != "bind-pw" {
		t.Fatalf("call %+v", c)
	}
}

func TestSealRefusesAnEmptySecretWithoutTheKMS(t *testing.T) {
	k := &kms{}
	for _, empty := range [][]byte{nil, {}} {
		if _, _, err := (contentkey.ConfigSecrets{KMS: k}).Seal(t.Context(), empty); !errors.Is(err, contentkey.ErrEmptySecret) {
			t.Fatalf("Seal(%#v) err = %v, want ErrEmptySecret", empty, err)
		}
	}
	if len(k.calls) != 0 {
		t.Fatalf("calls %+v", k.calls)
	}
}

func TestOpenDecryptsUnderTheKeyItWasSealedWith(t *testing.T) {
	k := &kms{out: []byte("bind-pw")}
	plain, err := contentkey.ConfigSecrets{KMS: k}.Open(t.Context(), "config-secrets", []byte("vault:v1:sealed"))
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != "bind-pw" {
		t.Fatalf("Open = %q", plain)
	}
	if c := k.calls[0]; len(k.calls) != 1 || c.op != "decrypt" || c.key != "config-secrets" || string(c.data) != "vault:v1:sealed" {
		t.Fatalf("calls %+v", k.calls)
	}
}

func TestOpenRefusesAnotherKeyWithoutTheKMS(t *testing.T) {
	k := &kms{}
	for _, ref := range []string{"", "content", "config-secrets/"} {
		if _, err := (contentkey.ConfigSecrets{KMS: k}).Open(t.Context(), ref, []byte("vault:v1:x")); !errors.Is(err, contentkey.ErrUnknownKey) {
			t.Fatalf("Open(%q) err = %v, want ErrUnknownKey", ref, err)
		}
	}
	if len(k.calls) != 0 {
		t.Fatalf("calls %+v", k.calls)
	}
}

func TestTheKMSErrorReachesTheCaller(t *testing.T) {
	k := &kms{err: contentkey.ErrUndecryptable}
	if _, err := (contentkey.ConfigSecrets{KMS: k}).Open(t.Context(), "config-secrets", []byte("x")); !errors.Is(err, contentkey.ErrUndecryptable) {
		t.Fatalf("Open err = %v", err)
	}
	unavailable := errors.New("sealed")
	k.err = unavailable
	if _, _, err := (contentkey.ConfigSecrets{KMS: k}).Seal(t.Context(), []byte("x")); !errors.Is(err, unavailable) {
		t.Fatalf("Seal err = %v", err)
	}
}
