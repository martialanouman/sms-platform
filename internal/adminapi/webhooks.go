package adminapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	errs "github.com/martialanouman/sms-platform/internal/platform/errors"
)

// Webhook is a row of control_plane.webhooks, without its sealed secret.
type Webhook struct {
	ID        string
	AccountID string
	EventType string
	URL       string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// WebhookPatch is the fields an update of a webhook sets; nil leaves a field.
type WebhookPatch struct {
	URL    *string
	Status *string
}

// Webhooks is the store of webhooks, within a transaction; a webhook is reached through its account, and a missing one
// is errs.ErrNotFound.
type Webhooks interface {
	AccountWebhooks(ctx context.Context, accountID string) ([]Webhook, error)
	Webhook(ctx context.Context, accountID, id string) (Webhook, error)
	CreateWebhook(ctx context.Context, accountID, eventType, url string, sealed []byte, keyRef string) (Webhook, error)
	UpdateWebhook(ctx context.Context, accountID, id string, p WebhookPatch) (Webhook, error)
	SealWebhook(ctx context.Context, accountID, id string, sealed []byte, keyRef string) (Webhook, error)
	DeleteWebhook(ctx context.Context, accountID, id string) error
}

// Sealer seals a secret the platform replays to a third party (ADR-0016); the caller answers any failure with
// errs.ErrServiceUnavailable, nothing written.
type Sealer interface {
	Seal(ctx context.Context, plaintext []byte) (sealed []byte, keyRef string, err error)
}

func sealNew(ctx context.Context, sealer Sealer) (secret string, sealed []byte, keyRef string, err error) {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	secret = hex.EncodeToString(raw)
	sealed, keyRef, err = sealer.Seal(ctx, []byte(secret))
	if err != nil {
		return "", nil, "", errs.ErrServiceUnavailable
	}
	return secret, sealed, keyRef, nil
}

// AccountWebhooks is the webhooks of account accountID, which must exist.
func AccountWebhooks(ctx context.Context, tx Tx, accountID string) ([]Webhook, error) {
	if _, err := tx.Account(ctx, accountID); err != nil {
		return nil, err
	}
	return tx.AccountWebhooks(ctx, accountID)
}

// CreateWebhook creates the eventType webhook of account accountID, its secret sealed before anything is written.
func CreateWebhook(ctx context.Context, tx Tx, sealer Sealer, accountID, eventType, url string) (Webhook, string, error) {
	if _, err := tx.Account(ctx, accountID); err != nil {
		return Webhook{}, "", err
	}
	secret, sealed, keyRef, err := sealNew(ctx, sealer)
	if err != nil {
		return Webhook{}, "", err
	}
	w, err := tx.CreateWebhook(ctx, accountID, eventType, url, sealed, keyRef)
	return w, secret, err
}

// UpdateWebhook applies p; enabling a closed account's webhook is a conflict.
func UpdateWebhook(ctx context.Context, tx Tx, accountID, id string, p WebhookPatch) (Webhook, error) {
	w, err := tx.UpdateWebhook(ctx, accountID, id, p)
	return w, closedConflict(err)
}

// RotateWebhookSecret gives webhook id a new secret, sealed before anything is written; the old one stops at once. A
// closed account's webhook is a conflict.
func RotateWebhookSecret(ctx context.Context, tx Tx, sealer Sealer, accountID, id string) (Webhook, string, error) {
	if _, err := tx.Webhook(ctx, accountID, id); err != nil {
		return Webhook{}, "", err
	}
	secret, sealed, keyRef, err := sealNew(ctx, sealer)
	if err != nil {
		return Webhook{}, "", err
	}
	w, err := tx.SealWebhook(ctx, accountID, id, sealed, keyRef)
	return w, secret, closedConflict(err)
}
