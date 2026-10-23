package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/martialanouman/sms-platform/internal/adminapi"
	"github.com/martialanouman/sms-platform/internal/adminapi/postgres/sqlc"
	errs "github.com/martialanouman/sms-platform/internal/platform/errors"
)

var fields = map[string]string{
	"customer_groups_name_key":        "name",
	"customers_group_id_fkey":         "group_id",
	"closed_is_final":                 "status",
	"accounts_customer_id_name_key":   "name",
	"accounts_customer_id_fkey":       "customer_id",
	"customer_is_closed":              "customer_id",
	"credentials_account_id_type_key": "type",
	"credentials_system_id_key":       "system_id",
	"credentials_system_id_check":     "system_id",
	"credentials_revoked_check":       "status",
}

func refused(err error) error {
	var pg *pgconn.PgError
	if errors.Is(err, pgx.ErrNoRows) {
		return errs.ErrNotFound
	}
	if !errors.As(err, &pg) {
		return err
	}
	switch pg.Code {
	case "23505":
		return adminapi.Refused{Code: errs.ErrConflict, Field: fields[pg.ConstraintName], Message: "already taken"}
	case "23503":
		return adminapi.Refused{Code: errs.ErrValidation, Field: fields[pg.ConstraintName], Message: "unknown reference"}
	case "23514":
		return adminapi.Refused{Code: errs.ErrValidation, Field: fields[pg.ConstraintName], Message: "not allowed"}
	case "23001":
		return adminapi.Refused{Code: errs.ErrConflict, Message: "still referenced"}
	}
	return err
}

func (t tx) Groups(ctx context.Context, status *string, after *adminapi.Key, limit int) ([]adminapi.CustomerGroup, error) {
	p := sqlc.GroupsParams{Status: status, Lim: int64(limit)}
	if after != nil {
		p.AfterAt, p.AfterID = &after.At, &after.ID
	}
	rows, err := queries.Groups(ctx, t.db, p)
	if err != nil {
		return nil, err
	}
	out := make([]adminapi.CustomerGroup, len(rows))
	for i, r := range rows {
		out[i] = adminapi.CustomerGroup(r)
	}
	return out, nil
}

func (t tx) Group(ctx context.Context, id string) (adminapi.CustomerGroup, error) {
	r, err := queries.Group(ctx, t.db, id)
	return adminapi.CustomerGroup(r), refused(err)
}

func (t tx) CreateGroup(ctx context.Context, name string, description *string) (adminapi.CustomerGroup, error) {
	id, err := queries.CreateGroup(ctx, t.db, sqlc.CreateGroupParams{Name: name, Description: description})
	if err != nil {
		return adminapi.CustomerGroup{}, refused(err)
	}
	return t.Group(ctx, id)
}

func (t tx) UpdateGroup(ctx context.Context, id string, p adminapi.GroupPatch) (adminapi.CustomerGroup, error) {
	id, err := queries.UpdateGroup(ctx, t.db, sqlc.UpdateGroupParams{ID: id, Name: p.Name, Status: p.Status,
		SetDescription: p.Description.Set, Description: p.Description.Value})
	if err != nil {
		return adminapi.CustomerGroup{}, refused(err)
	}
	return t.Group(ctx, id)
}

func (t tx) DeleteGroup(ctx context.Context, id string) error {
	n, err := queries.DeleteGroup(ctx, t.db, id)
	if err == nil && n == 0 {
		return errs.ErrNotFound
	}
	return err
}

func (t tx) Customers(ctx context.Context, f adminapi.CustomerFilter, after *adminapi.Key, limit int) ([]adminapi.Customer, error) {
	p := sqlc.CustomersParams{GroupID: f.GroupID, Status: f.Status, Lim: int64(limit)}
	if after != nil {
		p.AfterAt, p.AfterID = &after.At, &after.ID
	}
	rows, err := queries.Customers(ctx, t.db, p)
	if err != nil {
		return nil, err
	}
	out := make([]adminapi.Customer, len(rows))
	for i, r := range rows {
		out[i] = customer(r)
	}
	return out, nil
}

func (t tx) Customer(ctx context.Context, id string) (adminapi.Customer, error) {
	r, err := queries.Customer(ctx, t.db, id)
	return customer(r), refused(err)
}

func (t tx) CreateCustomer(ctx context.Context, name string, groupID *string, age adminapi.MaxAge) (adminapi.Customer, error) {
	id, err := queries.CreateCustomer(ctx, t.db, sqlc.CreateCustomerParams{Name: name, GroupID: groupID, MaxAgeOtp: age.OTP,
		MaxAgeTransactional: age.Transactional, MaxAgeMarketing: age.Marketing})
	if err != nil {
		return adminapi.Customer{}, refused(err)
	}
	return t.Customer(ctx, id)
}

func (t tx) UpdateCustomer(ctx context.Context, id string, p adminapi.CustomerPatch) (adminapi.Customer, error) {
	params := sqlc.UpdateCustomerParams{ID: id, Name: p.Name, Status: p.Status, SetGroup: p.GroupID.Set, GroupID: p.GroupID.Value}
	if p.MaxAge != nil {
		params.SetMaxAge, params.MaxAgeOtp, params.MaxAgeTransactional, params.MaxAgeMarketing =
			true, p.MaxAge.OTP, p.MaxAge.Transactional, p.MaxAge.Marketing
	}
	id, err := queries.UpdateCustomer(ctx, t.db, params)
	if err != nil {
		return adminapi.Customer{}, refused(err)
	}
	return t.Customer(ctx, id)
}

func (t tx) SetAccountsStatus(ctx context.Context, customerID, status string) error {
	return queries.SetAccountsStatus(ctx, t.db, sqlc.SetAccountsStatusParams{CustomerID: customerID, Status: status})
}

func (t tx) DeleteCustomer(ctx context.Context, id string) error {
	n, err := queries.DeleteCustomer(ctx, t.db, id)
	if err == nil && n == 0 {
		return errs.ErrNotFound
	}
	return refused(err)
}

func (t tx) CustomerAccounts(ctx context.Context, id string) ([]adminapi.AccountRef, error) {
	rows, err := queries.CustomerAccounts(ctx, t.db, id)
	if err != nil {
		return nil, err
	}
	out := make([]adminapi.AccountRef, len(rows))
	for i, r := range rows {
		out[i] = adminapi.AccountRef(r)
	}
	return out, nil
}

func customer(r sqlc.ControlPlaneCustomer) adminapi.Customer {
	return adminapi.Customer{ID: r.ID, Name: r.Name, Status: r.Status, GroupID: r.GroupID, CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt, MaxAge: adminapi.MaxAge{OTP: r.MaxAgeOtp, Transactional: r.MaxAgeTransactional,
			Marketing: r.MaxAgeMarketing}}
}
