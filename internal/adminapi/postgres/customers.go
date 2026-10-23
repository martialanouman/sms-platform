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
	"customer_groups_name_key": "name",
}

func refused(err error) error {
	var pg *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return errs.ErrNotFound
	case errors.As(err, &pg) && pg.Code == "23505":
		return adminapi.Refused{Code: errs.ErrConflict, Field: fields[pg.ConstraintName], Message: "already taken"}
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

func (t tx) GroupCustomers(ctx context.Context, id string, after *adminapi.Key, limit int) ([]adminapi.Customer, error) {
	p := sqlc.GroupCustomersParams{GroupID: &id, Lim: int64(limit)}
	if after != nil {
		p.AfterAt, p.AfterID = &after.At, &after.ID
	}
	rows, err := queries.GroupCustomers(ctx, t.db, p)
	if err != nil {
		return nil, err
	}
	out := make([]adminapi.Customer, len(rows))
	for i, r := range rows {
		out[i] = customer(r)
	}
	return out, nil
}

func customer(r sqlc.ControlPlaneCustomer) adminapi.Customer {
	return adminapi.Customer{ID: r.ID, Name: r.Name, Status: r.Status, GroupID: r.GroupID, CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt, MaxAge: adminapi.MaxAge{OTP: r.MaxAgeOtp, Transactional: r.MaxAgeTransactional,
			Marketing: r.MaxAgeMarketing}}
}
