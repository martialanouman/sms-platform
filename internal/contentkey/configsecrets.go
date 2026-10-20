// Package contentkey holds the master key of the platform's sealed data: it seals and opens the replayed configuration
// secrets (ADR-0016) under a key that never leaves the KMS (docs/design/06-confiance-interne.md D5).
package contentkey

import (
	"context"
	"errors"
)

// ConfigSecretsKey is the KMS key of the configuration secrets, persisted as their *_kms_key_ref.
const ConfigSecretsKey = "config-secrets"

// The verdicts on a request itself; any other error is transient (ADR-0016 addendum).
var (
	ErrEmptySecret   = errors.New("contentkey: empty secret")
	ErrUnknownKey    = errors.New("contentkey: unknown kms key ref")
	ErrUndecryptable = errors.New("contentkey: sealed secret refused by the KMS")
)

// KMS encrypts and decrypts under a named key it holds; Decrypt returns ErrUndecryptable for a sealed value it refuses.
type KMS interface {
	Encrypt(ctx context.Context, key string, plaintext []byte) ([]byte, error)
	Decrypt(ctx context.Context, key string, sealed []byte) ([]byte, error)
}

// ConfigSecrets seals and opens the replayed configuration secrets.
type ConfigSecrets struct {
	KMS KMS
}

// Seal returns the sealed form of plaintext and the ref of the key that sealed it.
func (c ConfigSecrets) Seal(ctx context.Context, plaintext []byte) ([]byte, string, error) {
	if len(plaintext) == 0 {
		return nil, "", ErrEmptySecret
	}
	sealed, err := c.KMS.Encrypt(ctx, ConfigSecretsKey, plaintext)
	if err != nil {
		return nil, "", err
	}
	return sealed, ConfigSecretsKey, nil
}

// Open reverses Seal.
func (c ConfigSecrets) Open(ctx context.Context, keyRef string, sealed []byte) ([]byte, error) {
	if keyRef != ConfigSecretsKey {
		return nil, ErrUnknownKey
	}
	return c.KMS.Decrypt(ctx, ConfigSecretsKey, sealed)
}
