package dashboard

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

// The states of an access link that is neither used nor expired.
const (
	LinkQueued = "queued"
	LinkSent   = "sent"
	LinkFailed = "failed"
)

var (
	// ErrNotFound is an operator or a role unknown, or an identifier that names none.
	ErrNotFound = errors.New("not found")
	// ErrEmailTaken is an email an operator already has, case-insensitively.
	ErrEmailTaken = errors.New("email taken")
	// ErrSelfLockout is a change that would lock its author out.
	ErrSelfLockout = errors.New("self lockout")
	// ErrOperatorDisabled is an access link asked for a disabled account.
	ErrOperatorDisabled = errors.New("operator disabled")
)

// InvalidField is a value of the request that names nothing known.
type InvalidField struct {
	Field string
}

func (e InvalidField) Error() string {
	return "invalid " + e.Field
}

// RoleRef names a role.
type RoleRef struct {
	ID   string
	Name string
}

// LinkState is the access link of an operator awaiting its sending or its use.
type LinkState struct {
	Kind  LinkKind
	State string
}

// OperatorRecord is an operator as the BFF lists it; Link is nil when no link awaits.
type OperatorRecord struct {
	ID           string
	Email        string
	DisplayName  string
	Active       bool
	Roles        []RoleRef
	SecondFactor bool
	Link         *LinkState
}

// Operators lists every operator, active and disabled, by email.
func (s Service) Operators(ctx context.Context) ([]OperatorRecord, error) {
	var all []OperatorRecord
	err := s.Store.Tx(ctx, func(tx Tx) (err error) {
		all, err = tx.Operators(ctx, "", maxDeliveries)
		return err
	})
	return all, err
}

func operator(ctx context.Context, tx Tx, id string) (OperatorRecord, error) {
	all, err := tx.Operators(ctx, id, maxDeliveries)
	switch {
	case err != nil:
		return OperatorRecord{}, err
	case len(all) == 0:
		return OperatorRecord{}, ErrNotFound
	}
	return all[0], nil
}

// CreateOperator creates an active operator without role nor password, and queues its activation link.
func (s Service) CreateOperator(ctx context.Context, actor, email, displayName string) (OperatorRecord, error) {
	var created OperatorRecord
	err := s.Store.Tx(ctx, func(tx Tx) error {
		id, ok, err := tx.InsertOperator(ctx, normalize(email), displayName)
		switch {
		case err != nil:
			return err
		case !ok:
			return ErrEmailTaken
		}
		if err := tx.QueueLink(ctx, id, Activation); err != nil {
			return err
		}
		for _, event := range []Event{EventOperatorCreated, EventAccessLinkRequested} {
			if err := tx.Log(ctx, Entry{Actor: actor, Event: event, Target: id, Success: true}); err != nil {
				return err
			}
		}
		created, err = operator(ctx, tx, id)
		return err
	})
	return created, err
}

// SetOperatorStatus disables or re-enables the operator id; disabling closes its sessions. Nobody disables himself.
func (s Service) SetOperatorStatus(ctx context.Context, actor, id string, active bool) (OperatorRecord, error) {
	if id == actor && !active {
		return OperatorRecord{}, ErrSelfLockout
	}
	var updated OperatorRecord
	err := s.Store.Tx(ctx, func(tx Tx) error {
		o, found, err := tx.OperatorByID(ctx, id)
		switch {
		case err != nil:
			return err
		case !found:
			return ErrNotFound
		}
		if o.Active != active {
			if err := tx.SetOperatorStatus(ctx, id, active); err != nil {
				return err
			}
			event := EventOperatorEnabled
			if !active {
				event = EventOperatorDisabled
				if err := tx.CloseSessions(ctx, id); err != nil {
					return err
				}
			}
			if err := tx.Log(ctx, Entry{Actor: actor, Event: event, Target: id, Success: true}); err != nil {
				return err
			}
		}
		updated, err = operator(ctx, tx, id)
		return err
	})
	return updated, err
}

// RequestAccessLink queues an activation link for an account without password, a reset link otherwise.
func (s Service) RequestAccessLink(ctx context.Context, actor, id string) error {
	return s.Store.Tx(ctx, func(tx Tx) error {
		o, found, err := tx.OperatorByID(ctx, id)
		switch {
		case err != nil:
			return err
		case !found:
			return ErrNotFound
		case !o.Active:
			return ErrOperatorDisabled
		}
		kind := Reset
		if o.PasswordHash == "" {
			kind = Activation
		}
		if err := tx.QueueLink(ctx, id, kind); err != nil {
			return err
		}
		return tx.Log(ctx, Entry{Actor: actor, Event: EventAccessLinkRequested, Target: id, Success: true})
	})
}

// SetOperatorRoles replaces the roles of the operator id; nobody loses operators:write or roles:write by his own hand.
func (s Service) SetOperatorRoles(ctx context.Context, actor, id string, roleIDs []string) (OperatorRecord, error) {
	var updated OperatorRecord
	err := s.Store.Tx(ctx, func(tx Tx) error {
		_, found, err := tx.OperatorByID(ctx, id)
		switch {
		case err != nil:
			return err
		case !found:
			return ErrNotFound
		}
		known, err := tx.ExistingRoles(ctx, roleIDs)
		if err != nil {
			return err
		}
		for i, r := range roleIDs {
			if !slices.Contains(known, r) {
				return InvalidField{Field: fmt.Sprintf("roleIds[%d]", i)}
			}
		}
		wanted := slices.Compact(slices.Sorted(slices.Values(roleIDs)))
		held, err := tx.RolesOf(ctx, id)
		if err != nil {
			return err
		}
		if !slices.Equal(held, wanted) {
			if err := guard(ctx, tx, actor, func() error { return tx.SetRoles(ctx, id, wanted) }); err != nil {
				return err
			}
			if err := tx.Log(ctx, Entry{Actor: actor, Event: EventRolesChanged, Target: id, Success: true}); err != nil {
				return err
			}
		}
		updated, err = operator(ctx, tx, id)
		return err
	})
	return updated, err
}

func guard(ctx context.Context, tx Tx, actor string, change func() error) error {
	before, err := tx.PermissionsOf(ctx, actor)
	if err != nil {
		return err
	}
	if err := change(); err != nil {
		return err
	}
	after, err := tx.PermissionsOf(ctx, actor)
	if err != nil {
		return err
	}
	for _, p := range []string{"operators:write", "roles:write"} {
		if slices.Contains(before, p) && !slices.Contains(after, p) {
			return ErrSelfLockout
		}
	}
	return nil
}
