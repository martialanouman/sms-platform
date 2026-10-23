// Package dashboard is the core of the dashboard BFF: its operators, access links, sessions and access log
// (spec 08 §8.3, §8.9). It does no I/O; the ports of store.go do.
package dashboard

import (
	"errors"
	"unicode"
	"unicode/utf8"

	"github.com/martialanouman/sms-platform/internal/platform/argon2id"
)

const slots = 10

// ErrBusy is every hashing slot taken: the caller retries later, nothing was decided.
var ErrBusy = errors.New("every hashing slot is busy")

// CheckPassword names, in French for the operator, each rule the password breaks; none when it complies.
func CheckPassword(password string) []string {
	var upper, lower, digit, special bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			digit = true
		case !unicode.IsLetter(r):
			special = true
		}
	}
	var missing []string
	for _, rule := range []struct {
		ok   bool
		name string
	}{
		{utf8.RuneCountInString(password) >= 12, "douze caractères au moins"},
		{upper, "une majuscule"},
		{lower, "une minuscule"},
		{digit, "un chiffre"},
		{special, "un caractère spécial"},
	} {
		if !rule.ok {
			missing = append(missing, rule.name)
		}
	}
	return missing
}

// Hasher hashes and verifies passwords with argon2id, ten at a time.
type Hasher struct {
	slots chan struct{}
}

// NewHasher is a hasher with its ten slots free.
func NewHasher() *Hasher {
	return &Hasher{slots: make(chan struct{}, slots)}
}

func (h *Hasher) hold(do func() error) error {
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
		return do()
	default:
		return ErrBusy
	}
}

// Hash is the PHC string of password under a fresh salt.
func (h *Hasher) Hash(password string) (string, error) {
	var phc string
	err := h.hold(func() error {
		phc = argon2id.Hash(password)
		return nil
	})
	return phc, err
}

// Verify tells whether password is the one phc was computed from. A hash this binary would not have written is an
// error, never a match.
func (h *Hasher) Verify(phc, password string) (bool, error) {
	var match bool
	err := h.hold(func() error {
		var err error
		match, err = argon2id.Verify(phc, password)
		return err
	})
	return match, err
}

// VerifyDummy costs what Verify costs, under the same slot, for an operator who does not exist or cannot log in: the
// time of a refusal tells nothing.
func (h *Hasher) VerifyDummy(password string) error {
	return h.hold(func() error {
		argon2id.Dummy(password)
		return nil
	})
}
