package adminapi

import (
	"context"
	"errors"
	"time"

	errs "github.com/martialanouman/sms-platform/internal/platform/errors"
)

// Account is a row of control_plane.accounts, with the name and status of its customer.
type Account struct {
	ID               string
	CustomerID       string
	CustomerName     string
	CustomerStatus   string
	Name             string
	Status           string
	SmppEnabled      bool
	RestEnabled      bool
	QuerySmEnabled   bool
	CancelSmEnabled  bool
	AllowedBindTypes []string
	MaxSessions      int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// AccountFilter narrows the accounts.
type AccountFilter struct {
	CustomerID *string
	GroupID    *string
	Status     *string
}

// AccountPatch is the fields an update of an account sets; nil leaves a field.
type AccountPatch struct {
	Name             *string
	Status           *string
	SmppEnabled      *bool
	RestEnabled      *bool
	QuerySmEnabled   *bool
	CancelSmEnabled  *bool
	AllowedBindTypes []string
	MaxSessions      *int
}

// Accounts is the store of accounts, within a transaction; a missing account is errs.ErrNotFound.
type Accounts interface {
	Accounts(ctx context.Context, f AccountFilter, after *Key, limit int) ([]Account, error)
	CountAccounts(ctx context.Context, f AccountFilter) (int, error)
	Account(ctx context.Context, id string) (Account, error)
	CreateAccount(ctx context.Context, a Account) (Account, error)
	UpdateAccount(ctx context.Context, id string, p AccountPatch) (Account, error)
	DeleteAccount(ctx context.Context, id string) error
}

// AccountsPage is a page of the accounts f selects, newest first, the cursor of the next one and how many f selects.
func AccountsPage(ctx context.Context, tx Tx, f AccountFilter, cursor string, limit int) ([]Account, string, int, error) {
	accounts, next, err := page(cursor, limit, func(a Account) Key { return Key{a.CreatedAt, a.ID} },
		func(after *Key, n int) ([]Account, error) { return tx.Accounts(ctx, f, after, n) })
	if err != nil {
		return nil, "", 0, err
	}
	total, err := tx.CountAccounts(ctx, f)
	return accounts, next, total, err
}

// SuspendAccount suspends account id; a closed account is a conflict.
func SuspendAccount(ctx context.Context, tx Tx, id string) (Account, error) {
	suspended := "suspended"
	a, err := tx.UpdateAccount(ctx, id, AccountPatch{Status: &suspended})
	return a, closedConflict(err)
}

func closedConflict(err error) error {
	var r Refused
	if errors.As(err, &r) {
		return Refused{Code: errs.ErrConflict, Message: r.Message}
	}
	return err
}
