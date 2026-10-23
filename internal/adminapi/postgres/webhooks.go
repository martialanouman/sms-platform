package postgres

import (
	"context"

	"github.com/martialanouman/sms-platform/internal/adminapi"
	"github.com/martialanouman/sms-platform/internal/adminapi/postgres/sqlc"
	errs "github.com/martialanouman/sms-platform/internal/platform/errors"
)

func webhook(r sqlc.WebhookRow) adminapi.Webhook {
	return adminapi.Webhook{ID: r.ID, AccountID: r.AccountID, EventType: r.EventType, URL: r.Url, Status: r.Status,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func (t tx) AccountWebhooks(ctx context.Context, accountID string) ([]adminapi.Webhook, error) {
	rows, err := queries.AccountWebhooks(ctx, t.db, accountID)
	if err != nil {
		return nil, err
	}
	out := make([]adminapi.Webhook, len(rows))
	for i, r := range rows {
		out[i] = webhook(sqlc.WebhookRow(r))
	}
	return out, nil
}

func (t tx) Webhook(ctx context.Context, accountID, id string) (adminapi.Webhook, error) {
	r, err := queries.Webhook(ctx, t.db, sqlc.WebhookParams{ID: id, AccountID: accountID})
	return webhook(r), refused(err)
}

func (t tx) CreateWebhook(ctx context.Context, accountID, eventType, url string, sealed []byte, keyRef string) (adminapi.Webhook, error) {
	r, err := queries.CreateWebhook(ctx, t.db, sqlc.CreateWebhookParams{AccountID: accountID, EventType: eventType, Url: url,
		SecretSealed: sealed, SecretKmsKeyRef: keyRef})
	return webhook(sqlc.WebhookRow(r)), refused(err)
}

func (t tx) UpdateWebhook(ctx context.Context, accountID, id string, p adminapi.WebhookPatch) (adminapi.Webhook, error) {
	r, err := queries.UpdateWebhook(ctx, t.db, sqlc.UpdateWebhookParams{ID: id, AccountID: accountID, Url: p.URL, Status: p.Status})
	return webhook(sqlc.WebhookRow(r)), refused(err)
}

func (t tx) SealWebhook(ctx context.Context, accountID, id string, sealed []byte, keyRef string) (adminapi.Webhook, error) {
	r, err := queries.SealWebhook(ctx, t.db, sqlc.SealWebhookParams{ID: id, AccountID: accountID, SecretSealed: sealed,
		SecretKmsKeyRef: keyRef})
	return webhook(sqlc.WebhookRow(r)), refused(err)
}

func (t tx) DeleteWebhook(ctx context.Context, accountID, id string) error {
	n, err := queries.DeleteWebhook(ctx, t.db, sqlc.DeleteWebhookParams{ID: id, AccountID: accountID})
	if err == nil && n == 0 {
		return errs.ErrNotFound
	}
	return err
}
