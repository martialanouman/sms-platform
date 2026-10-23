package adminapi

import (
	"context"
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
	GroupCustomers(ctx context.Context, id string, after *Key, limit int) ([]Customer, error)
}

// GroupsPage is a page of customer groups, newest first, and the cursor of the next one.
func GroupsPage(ctx context.Context, tx Tx, status *string, cursor string, limit int) ([]CustomerGroup, string, error) {
	return page(cursor, limit, func(g CustomerGroup) Key { return Key{g.CreatedAt, g.ID} },
		func(after *Key, n int) ([]CustomerGroup, error) { return tx.Groups(ctx, status, after, n) })
}

// GroupCustomersPage is a page of the customers of group id, newest first, and the cursor of the next one.
func GroupCustomersPage(ctx context.Context, tx Tx, id, cursor string, limit int) ([]Customer, string, error) {
	return page(cursor, limit, func(c Customer) Key { return Key{c.CreatedAt, c.ID} },
		func(after *Key, n int) ([]Customer, error) {
			if _, err := tx.Group(ctx, id); err != nil {
				return nil, err
			}
			return tx.GroupCustomers(ctx, id, after, n)
		})
}
