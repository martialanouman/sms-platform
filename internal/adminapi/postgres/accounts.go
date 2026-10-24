package postgres

import (
	"context"

	"github.com/martialanouman/sms-platform/internal/adminapi"
	"github.com/martialanouman/sms-platform/internal/adminapi/postgres/sqlc"
	errs "github.com/martialanouman/sms-platform/internal/platform/errors"
)

func (t tx) CountAccounts(ctx context.Context, f adminapi.AccountFilter) (int, error) {
	n, err := queries.CountAccounts(ctx, t.db, sqlc.CountAccountsParams{CustomerID: f.CustomerID, GroupID: f.GroupID, Status: f.Status})
	return int(n), err
}

func (t tx) Accounts(ctx context.Context, f adminapi.AccountFilter, after *adminapi.Key, limit int) ([]adminapi.Account, error) {
	p := sqlc.AccountsParams{CustomerID: f.CustomerID, GroupID: f.GroupID, Status: f.Status, Lim: int64(limit)}
	if after != nil {
		p.AfterAt, p.AfterID = &after.At, &after.ID
	}
	rows, err := queries.Accounts(ctx, t.db, p)
	if err != nil {
		return nil, err
	}
	out := make([]adminapi.Account, len(rows))
	for i, r := range rows {
		out[i] = adminapi.Account(r)
	}
	return out, nil
}

func (t tx) Account(ctx context.Context, id string) (adminapi.Account, error) {
	r, err := queries.Account(ctx, t.db, id)
	return adminapi.Account(r), refused(err)
}

func (t tx) CreateAccount(ctx context.Context, a adminapi.Account) (adminapi.Account, error) {
	id, err := queries.CreateAccount(ctx, t.db, sqlc.CreateAccountParams{CustomerID: a.CustomerID, Name: a.Name,
		SmppEnabled: a.SmppEnabled, RestEnabled: a.RestEnabled, QuerySmEnabled: a.QuerySmEnabled,
		CancelSmEnabled: a.CancelSmEnabled, AllowedBindTypes: a.AllowedBindTypes, MaxSessions: a.MaxSessions})
	if err != nil {
		return adminapi.Account{}, refused(err)
	}
	return t.Account(ctx, id)
}

func (t tx) UpdateAccount(ctx context.Context, id string, p adminapi.AccountPatch) (adminapi.Account, error) {
	id, err := queries.UpdateAccount(ctx, t.db, sqlc.UpdateAccountParams{ID: id, Name: p.Name, Status: p.Status,
		SmppEnabled: p.SmppEnabled, RestEnabled: p.RestEnabled, QuerySmEnabled: p.QuerySmEnabled,
		CancelSmEnabled: p.CancelSmEnabled, AllowedBindTypes: p.AllowedBindTypes, MaxSessions: p.MaxSessions})
	if err != nil {
		return adminapi.Account{}, refused(err)
	}
	return t.Account(ctx, id)
}

func (t tx) DeleteAccount(ctx context.Context, id string) error {
	n, err := queries.DeleteAccount(ctx, t.db, id)
	if err == nil && n == 0 {
		return errs.ErrNotFound
	}
	return err
}
