package dashboard

import (
	"context"
	"time"
)

// Current is the live session of a request and its operator.
type Current struct {
	SessionID              string
	OperatorID             string
	Email                  string
	DisplayName            string
	ExpiresAt              time.Time
	Elevated               bool
	Fresh                  bool
	Totp                   bool
	RecoveryCodesRemaining int
	Passkeys               int
}

// Resolve is the live session of cookie; it never extends it (spec 08 §8.3). A sealed cookie whose session has
// ended is logged as refused; an unsealed or unknown one is not.
func (s Service) Resolve(ctx context.Context, cookie string) (Current, error) {
	token, ok := s.Keys.Open(cookie)
	if !ok {
		return Current{}, ErrUnauthenticated
	}
	var v SessionView
	var found bool
	err := s.Store.Tx(ctx, func(tx Tx) (err error) {
		if v, found, err = tx.SessionOf(ctx, Digest(token), IdleTimeout, freshness); err != nil || !found || v.Live {
			return err
		}
		return tx.Log(ctx, Entry{Actor: v.OperatorID, Event: EventSessionRefused})
	})
	switch {
	case err != nil:
		return Current{}, err
	case !found || !v.Live:
		return Current{}, ErrUnauthenticated
	}
	return v.Current, nil
}

// Activity is an operator's gesture: the only extension of a live session.
func (s Service) Activity(ctx context.Context, cookie string) error {
	token, ok := s.Keys.Open(cookie)
	if !ok {
		return ErrUnauthenticated
	}
	return s.Store.Tx(ctx, func(tx Tx) error {
		touched, err := tx.Touch(ctx, Digest(token), IdleTimeout)
		if err == nil && !touched {
			return ErrUnauthenticated
		}
		return err
	})
}

// Logout closes the session of cookie, whether it existed or not.
func (s Service) Logout(ctx context.Context, cookie string) error {
	token, ok := s.Keys.Open(cookie)
	if !ok {
		return nil
	}
	return s.Store.Tx(ctx, func(tx Tx) error {
		id, closed, err := tx.CloseSession(ctx, Digest(token))
		if err != nil || !closed {
			return err
		}
		return tx.Log(ctx, Entry{Actor: id, Event: EventLogout, Success: true})
	})
}
