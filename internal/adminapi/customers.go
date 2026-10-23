package adminapi

import (
	"context"
	"errors"
	"time"

	errs "github.com/martialanouman/sms-platform/internal/platform/errors"
)

// Refused is a write the store refused with Code, on the body field Field when one is to blame.
type Refused struct {
	Code    errs.Code
	Field   string
	Message string
}

func (r Refused) Error() string { return r.Message }

// Unwrap is the code of the refusal.
func (r Refused) Unwrap() error { return r.Code }

// Nullable is a nullable field of a PATCH: Set when the field was sent, Value nil when it was sent null.
type Nullable[T any] struct {
	Set   bool
	Value *T
}

// CustomerGroup is a row of control_plane.customer_groups, with the count of its customers.
type CustomerGroup struct {
	ID          string
	Name        string
	Description *string
	Status      string
	CreatedBy   *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	MemberCount int64
}

// GroupPatch is the fields an update of a customer group sets.
type GroupPatch struct {
	Name        *string
	Description Nullable[string]
	Status      *string
}

// MaxAge is a customer's maximum age per traffic category, in seconds; nil keeps the platform's.
type MaxAge struct {
	OTP           *int
	Transactional *int
	Marketing     *int
}

// Customer is a row of control_plane.customers.
type Customer struct {
	ID        string
	Name      string
	Status    string
	GroupID   *string
	MaxAge    MaxAge
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Groups is the store of customer groups, within a transaction; a missing group is errs.ErrNotFound.
type Groups interface {
	Groups(ctx context.Context, status *string, after *Key, limit int) ([]CustomerGroup, error)
	Group(ctx context.Context, id string) (CustomerGroup, error)
	CreateGroup(ctx context.Context, name string, description *string) (CustomerGroup, error)
	UpdateGroup(ctx context.Context, id string, p GroupPatch) (CustomerGroup, error)
	DeleteGroup(ctx context.Context, id string) error
}

// CustomerFilter narrows the customers.
type CustomerFilter struct {
	GroupID *string
	Status  *string
}

// CustomerPatch is the fields an update of a customer sets; a nil MaxAge leaves the three ages.
type CustomerPatch struct {
	Name    *string
	Status  *string
	GroupID Nullable[string]
	MaxAge  *MaxAge
}

// AccountRef names an account and its status.
type AccountRef struct {
	ID     string
	Name   string
	Status string
}

// Customers is the store of customers, within a transaction; a missing customer is errs.ErrNotFound.
type Customers interface {
	Customers(ctx context.Context, f CustomerFilter, after *Key, limit int) ([]Customer, error)
	Customer(ctx context.Context, id string) (Customer, error)
	CreateCustomer(ctx context.Context, name string, groupID *string, age MaxAge) (Customer, error)
	UpdateCustomer(ctx context.Context, id string, p CustomerPatch) (Customer, error)
	SetAccountsStatus(ctx context.Context, customerID, status string) error
	DeleteCustomer(ctx context.Context, id string) error
	CustomerAccounts(ctx context.Context, id string) ([]AccountRef, error)
}

// GroupsPage is a page of customer groups, newest first, and the cursor of the next one.
func GroupsPage(ctx context.Context, tx Tx, status *string, cursor string, limit int) ([]CustomerGroup, string, error) {
	return page(cursor, limit, func(g CustomerGroup) Key { return Key{g.CreatedAt, g.ID} },
		func(after *Key, n int) ([]CustomerGroup, error) { return tx.Groups(ctx, status, after, n) })
}

// CustomersPage is a page of the customers f selects, newest first, and the cursor of the next one.
func CustomersPage(ctx context.Context, tx Tx, f CustomerFilter, cursor string, limit int) ([]Customer, string, error) {
	return page(cursor, limit, func(c Customer) Key { return Key{c.CreatedAt, c.ID} },
		func(after *Key, n int) ([]Customer, error) { return tx.Customers(ctx, f, after, n) })
}

// GroupCustomersPage is a page of the customers of group id, newest first, and the cursor of the next one.
func GroupCustomersPage(ctx context.Context, tx Tx, id, cursor string, limit int) ([]Customer, string, error) {
	if _, err := tx.Group(ctx, id); err != nil {
		return nil, "", err
	}
	return CustomersPage(ctx, tx, CustomerFilter{GroupID: &id}, cursor, limit)
}

// UpdateCustomer applies p; suspending or closing a customer does the same to its accounts not yet closed (spec 01
// §1.2, ADR-0006).
func UpdateCustomer(ctx context.Context, tx Tx, id string, p CustomerPatch) (Customer, error) {
	c, err := tx.UpdateCustomer(ctx, id, p)
	if err != nil || p.Status == nil || *p.Status == "active" {
		return c, err
	}
	return c, tx.SetAccountsStatus(ctx, id, *p.Status)
}

// SuspendCustomer suspends customer id and its accounts not yet closed; a closed customer is a conflict.
func SuspendCustomer(ctx context.Context, tx Tx, id string) (Customer, error) {
	suspended := "suspended"
	c, err := UpdateCustomer(ctx, tx, id, CustomerPatch{Status: &suspended})
	var r Refused
	if errors.As(err, &r) && r.Field == "status" {
		return c, Refused{Code: errs.ErrConflict, Message: r.Message}
	}
	return c, err
}

// SuspensionImpact is what suspending customer id would affect: its accounts to suspend, and its closed ones.
func SuspensionImpact(ctx context.Context, tx Tx, id string) (suspend, closed []AccountRef, err error) {
	if _, err := tx.Customer(ctx, id); err != nil {
		return nil, nil, err
	}
	accounts, err := tx.CustomerAccounts(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	suspend, closed = []AccountRef{}, []AccountRef{}
	for _, a := range accounts {
		if a.Status == "closed" {
			closed = append(closed, a)
		} else {
			suspend = append(suspend, a)
		}
	}
	return suspend, closed, nil
}
