package dashboard

import (
	"context"
	"time"
)

// LinkKind is what an access link does: set the first password, or replace it.
type LinkKind string

// The kinds of access link.
const (
	Activation LinkKind = "activation"
	Reset      LinkKind = "reset"
)

// Event is a fact of the access log, as the BFF contract enumerates them.
type Event string

// The events 011a writes.
const (
	EventLogin               Event = "login"
	EventLogout              Event = "logout"
	EventAccessLinkRequested Event = "access_link_requested"
	EventAccessLinkUsed      Event = "access_link_used"
	EventSessionRefused      Event = "session_refused"
)

// Operator is an account as login reads it; PasswordHash is empty until the account is activated.
type Operator struct {
	ID           string
	PasswordHash string
	Active       bool
}

// SessionView is a session found by its token, Live when neither its idle timeout, its lifetime nor its
// operator's account has ended it.
type SessionView struct {
	ID          string
	OperatorID  string
	Email       string
	DisplayName string
	ExpiresAt   time.Time
	Live        bool
}

// Entry is a line of the access log; Actor and Target are empty when absent.
type Entry struct {
	ID      string
	At      time.Time
	Actor   string
	Event   Event
	Target  string
	Success bool
}

// Due is an access link waiting to be sent.
type Due struct {
	OperatorID string
	Email      string
	Kind       LinkKind
}

// Store runs fn in one transaction, committed if fn returns nil.
type Store interface {
	Tx(ctx context.Context, fn func(Tx) error) error
}

// Tx is what the core reads and writes; every comparison to the time is the database's.
type Tx interface {
	CreateOperator(ctx context.Context, email, displayName string) (id string, created bool, err error)
	OperatorByEmail(ctx context.Context, email string) (Operator, bool, error)
	SetPassword(ctx context.Context, operatorID, phc string) error

	QueueLink(ctx context.Context, operatorID string, kind LinkKind) error
	LinkUsable(ctx context.Context, digest []byte) (bool, error)
	ConsumeLink(ctx context.Context, digest []byte) (operatorID string, kind LinkKind, ok bool, err error)
	NextDue(ctx context.Context, maxAttempts int) (Due, bool, error)
	MarkSent(ctx context.Context, operatorID string, digest []byte, ttl time.Duration) error
	MarkFailed(ctx context.Context, operatorID string, base, ceiling time.Duration) error

	Reserve(ctx context.Context, subject []byte, window time.Duration, limit int) (retryAfter time.Duration, err error)
	Forget(ctx context.Context, subject []byte) error

	OpenSession(ctx context.Context, operatorID string, digest, challenge []byte, lifetime, challengeTTL time.Duration) (challengeExpiresAt time.Time, err error)
	SessionOf(ctx context.Context, digest []byte, idle time.Duration) (SessionView, bool, error)
	Touch(ctx context.Context, digest []byte, idle time.Duration) (bool, error)
	CloseSession(ctx context.Context, digest []byte) (operatorID string, ok bool, err error)
	CloseSessions(ctx context.Context, operatorID string) error
	Purge(ctx context.Context, idle, window time.Duration) error

	Log(ctx context.Context, e Entry) error
	AccessLog(ctx context.Context, before *Entry, limit int) ([]Entry, error)
}

// Mailer sends one message; its error never carries the body.
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}
