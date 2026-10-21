package dashboard

import (
	"errors"
	"strings"
	"time"
)

const (
	// IdleTimeout ends a session no gesture extended (spec 08 §8.3), noc sessions included. debt: #148
	IdleTimeout = 15 * time.Minute
	// Lifetime ends every session after its login.
	Lifetime       = 12 * time.Hour
	challengeTTL   = 5 * time.Minute
	freshness      = 5 * time.Minute
	activationTTL  = 72 * time.Hour
	resetTTL       = time.Hour
	lockWindow     = 15 * time.Minute
	maxFailures    = 5
	maxDeliveries  = 10
	backoffBase    = 30 * time.Second
	backoffCeiling = 10 * time.Minute
	accessLogPage  = 50
)

// Service runs the use cases of the dashboard over its ports.
type Service struct {
	Store     Store
	Hasher    *Hasher
	Keys      Keys
	Totp      TotpKey
	Passkeys  Passkeys
	Mailer    Mailer
	PublicURL string
}

var (
	// ErrUnauthenticated is no live session, or credentials refused, without telling which.
	ErrUnauthenticated = errors.New("unauthenticated")
	// ErrLinkInvalid is an access link used, expired, replaced or targeting a disabled account.
	ErrLinkInvalid = errors.New("access link invalid")
	// ErrBadCursor is a cursor of the access log this service did not issue.
	ErrBadCursor = errors.New("bad cursor")
)

// PolicyError names the rules a password breaks.
type PolicyError struct {
	Missing []string
}

func (e PolicyError) Error() string {
	return "le mot de passe doit contenir " + strings.Join(e.Missing, ", ")
}

// RateLimited is a login refused before any hashing; RetryAfter is the time left.
type RateLimited struct {
	RetryAfter time.Duration
}

func (e RateLimited) Error() string {
	return "too many attempts"
}

// ValidEmail is whether email, once normalized, has exactly one @ between two non-empty sides.
func ValidEmail(email string) bool {
	local, domain, ok := strings.Cut(normalize(email), "@")
	return ok && local != "" && domain != "" && !strings.Contains(domain, "@")
}

func normalize(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
