package adminapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"math/big"
	"time"

	"github.com/martialanouman/sms-platform/internal/platform/argon2id"
)

// Credential is a row of control_plane.credentials, without its hashes.
type Credential struct {
	ID             string
	AccountID      string
	Type           string
	SystemID       *string
	Status         string
	LastUsedAt     *time.Time
	GraceExpiresAt *time.Time
	CreatedAt      time.Time
	RotatedAt      *time.Time
}

// Credentials is the store of credentials, within a transaction; a credential is reached through its account, and a
// missing one is errs.ErrNotFound.
type Credentials interface {
	AccountCredentials(ctx context.Context, accountID string) ([]Credential, error)
	Credential(ctx context.Context, accountID, id string) (Credential, error)
	CreateCredential(ctx context.Context, accountID, kind string, systemID *string, hash string) (Credential, error)
	SetCredentialStatus(ctx context.Context, accountID, id, status string) (Credential, error)
	RotateCredential(ctx context.Context, accountID, id, hash string, grace time.Duration) (Credential, error)
	RevokeCredential(ctx context.Context, accountID, id string) error
}

const alphanumeric = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// newSecret is a fresh secret for a credential of kind and the hash stored for it: an API key is 256 random bits,
// hashed with sha256 for a check per request; an SMPP password is eight characters, as SMPP 3.4 allows, hashed with
// argon2id.
func newSecret(kind string) (secret, hash string) {
	if kind == "api_key" {
		raw := make([]byte, 32)
		_, _ = rand.Read(raw)
		key := "smsgw_" + base64.RawURLEncoding.EncodeToString(raw)
		sum := sha256.Sum256([]byte(key))
		return key, hex.EncodeToString(sum[:])
	}
	password := make([]byte, 8)
	for i := range password {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphanumeric))))
		password[i] = alphanumeric[n.Int64()]
	}
	return string(password), argon2id.Hash(string(password))
}

// AccountCredentials is the credentials of account accountID, which must exist.
func AccountCredentials(ctx context.Context, tx Tx, accountID string) ([]Credential, error) {
	if _, err := tx.Account(ctx, accountID); err != nil {
		return nil, err
	}
	return tx.AccountCredentials(ctx, accountID)
}

// CreateCredential creates a credential of kind for account accountID; systemID is an SMPP bind's alone.
func CreateCredential(ctx context.Context, tx Tx, accountID, kind string, systemID *string) (Credential, string, error) {
	if _, err := tx.Account(ctx, accountID); err != nil {
		return Credential{}, "", err
	}
	secret, hash := newSecret(kind)
	c, err := tx.CreateCredential(ctx, accountID, kind, systemID, hash)
	return c, secret, err
}

// RotateCredential gives credential id a new secret; the old one stays valid for grace, unless the credential was
// revoked. A closed account's credential is a conflict.
func RotateCredential(ctx context.Context, tx Tx, accountID, id string, grace time.Duration) (Credential, string, error) {
	c, err := tx.Credential(ctx, accountID, id)
	if err != nil {
		return Credential{}, "", err
	}
	secret, hash := newSecret(c.Type)
	c, err = tx.RotateCredential(ctx, accountID, id, hash, grace)
	return c, secret, closedConflict(err)
}

// SetCredentialStatus enables or disables credential id; a revoked credential, or enabling one of a closed account, is
// a conflict.
func SetCredentialStatus(ctx context.Context, tx Tx, accountID, id, status string) (Credential, error) {
	c, err := tx.SetCredentialStatus(ctx, accountID, id, status)
	return c, closedConflict(err)
}
