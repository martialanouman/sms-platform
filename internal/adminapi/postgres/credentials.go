package postgres

import (
	"context"
	"time"

	"github.com/martialanouman/sms-platform/internal/adminapi"
	"github.com/martialanouman/sms-platform/internal/adminapi/postgres/sqlc"
	errs "github.com/martialanouman/sms-platform/internal/platform/errors"
)

func (t tx) AccountCredentials(ctx context.Context, accountID string) ([]adminapi.Credential, error) {
	rows, err := queries.AccountCredentials(ctx, t.db, accountID)
	if err != nil {
		return nil, err
	}
	out := make([]adminapi.Credential, len(rows))
	for i, r := range rows {
		out[i] = adminapi.Credential(r)
	}
	return out, nil
}

func (t tx) Credential(ctx context.Context, accountID, id string) (adminapi.Credential, error) {
	r, err := queries.Credential(ctx, t.db, sqlc.CredentialParams{ID: id, AccountID: accountID})
	return adminapi.Credential(r), refused(err)
}

func (t tx) CreateCredential(ctx context.Context, accountID, kind string, systemID *string, hash string) (adminapi.Credential, error) {
	id, err := queries.CreateCredential(ctx, t.db, sqlc.CreateCredentialParams{AccountID: accountID, Type: kind,
		SystemID: systemID, SecretHash: &hash})
	if err != nil {
		return adminapi.Credential{}, refused(err)
	}
	return t.Credential(ctx, accountID, id)
}

func (t tx) SetCredentialStatus(ctx context.Context, accountID, id, status string) (adminapi.Credential, error) {
	id, err := queries.SetCredentialStatus(ctx, t.db, sqlc.SetCredentialStatusParams{ID: id, AccountID: accountID, Status: status})
	if err != nil {
		return adminapi.Credential{}, refused(err)
	}
	return t.Credential(ctx, accountID, id)
}

func (t tx) RotateCredential(ctx context.Context, accountID, id, hash string, grace time.Duration) (adminapi.Credential, error) {
	id, err := queries.RotateCredential(ctx, t.db, sqlc.RotateCredentialParams{ID: id, AccountID: accountID,
		SecretHash: &hash, GraceSec: int(grace / time.Second)})
	if err != nil {
		return adminapi.Credential{}, refused(err)
	}
	return t.Credential(ctx, accountID, id)
}

func (t tx) RevokeCredential(ctx context.Context, accountID, id string) error {
	n, err := queries.RevokeCredential(ctx, t.db, sqlc.RevokeCredentialParams{ID: id, AccountID: accountID})
	if err == nil && n == 0 {
		return errs.ErrNotFound
	}
	return err
}
