package adminapi

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	errs "github.com/martialanouman/sms-platform/internal/platform/errors"
	"github.com/martialanouman/sms-platform/internal/platform/keyset"
	"github.com/martialanouman/sms-platform/internal/platform/scopes"
)

// Entry is a line of control_plane.audit_log.
type Entry struct {
	ID          string
	Operator    string
	OperationID string
	Target      string
	RequestID   *string
	Status      int
	Outcome     *string
	At          time.Time
}

// Change is a line of control_plane.audit_changes.
type Change struct {
	ID     string
	Actor  string
	Table  string
	PK     string
	Op     string
	Before json.RawMessage
	After  json.RawMessage
	At     time.Time
}

// Filter narrows the audit log.
type Filter struct {
	Operator *string
	From     *time.Time
	To       *time.Time
}

// Key is the row a page starts after.
type Key struct {
	At time.Time
	ID string
}

// Tx is a transaction of the control plane, under its actor and request.
type Tx interface {
	Record(ctx context.Context, e Entry) error
	AuditLog(ctx context.Context, f Filter, after *Key, limit int) ([]Entry, error)
	Changes(ctx context.Context, requestID string, after *Key, limit int) ([]Change, error)
	Requested(ctx context.Context, requestID string) (bool, error)
	Groups
	Customers
	Accounts
	Credentials
}

// Store opens the transactions of the control plane.
type Store interface {
	Tx(ctx context.Context, actor, requestID string, fn func(Tx) error) error
	Record(ctx context.Context, e Entry) error
}

// Audited is whether a call of method under scope enters the audit log: every write, and every read under a
// sensitive scope (spec 07 §7.5).
func Audited(method, scope string) bool {
	// debt: #162
	return method != "GET" || slices.Contains(scopes.Sensitive, scope)
}

// AuditLogPage is a page of the audit log, newest first, and the cursor of the next one, empty on the last page.
func AuditLogPage(ctx context.Context, tx Tx, f Filter, cursor string, limit int) ([]Entry, string, error) {
	return page(cursor, limit, func(e Entry) Key { return Key{e.At, e.ID} }, func(after *Key, n int) ([]Entry, error) {
		return tx.AuditLog(ctx, f, after, n)
	})
}

// ChangesPage is a page of the row changes of requestID, oldest first, and the cursor of the next one.
func ChangesPage(ctx context.Context, tx Tx, requestID, cursor string, limit int) ([]Change, string, error) {
	return page(cursor, limit, func(c Change) Key { return Key{c.At, c.ID} }, func(after *Key, n int) ([]Change, error) {
		requested, err := tx.Requested(ctx, requestID)
		if err != nil {
			return nil, err
		}
		if !requested {
			return nil, errs.ErrNotFound
		}
		return tx.Changes(ctx, requestID, after, n)
	})
}

func page[T any](cursor string, limit int, key func(T) Key, fetch func(after *Key, n int) ([]T, error)) ([]T, string, error) {
	var after *Key
	if cursor != "" {
		at, id, ok := keyset.Parse(cursor)
		if !ok {
			return nil, "", errs.ErrValidation
		}
		after = &Key{At: at, ID: id}
	}
	rows, err := fetch(after, limit+1)
	if err != nil || len(rows) <= limit {
		return rows, "", err
	}
	last := key(rows[limit-1])
	return rows[:limit], keyset.Encode(last.At, last.ID), nil
}

type txKey struct{}

// WithTx is ctx carrying the transaction of its request.
func WithTx(ctx context.Context, tx Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// TxFrom is the transaction of the request of ctx, nil outside one.
func TxFrom(ctx context.Context) Tx {
	tx, _ := ctx.Value(txKey{}).(Tx)
	return tx
}
