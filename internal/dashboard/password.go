// Package dashboard is the core of the dashboard BFF: its operators, access links, sessions and access log
// (spec 08 §8.3, §8.9). It does no I/O; the ports of store.go do.
package dashboard

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	slots      = 10
	memory     = 64 * 1024
	passes     = 3
	lanes      = 4
	saltLength = 16
	keyLength  = 32
)

var params = fmt.Sprintf("m=%d,t=%d,p=%d", memory, passes, lanes)

// ErrBusy is every hashing slot taken: the caller retries later, nothing was decided.
var ErrBusy = errors.New("every hashing slot is busy")

var errUnreadableHash = errors.New("unreadable password hash")

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
	salt := make([]byte, saltLength)
	_, _ = rand.Read(salt)
	var phc string
	err := h.hold(func() error {
		key := argon2.IDKey([]byte(password), salt, passes, memory, lanes, keyLength)
		phc = fmt.Sprintf("$argon2id$v=%d$%s$%s$%s", argon2.Version, params,
			base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
		return nil
	})
	return phc, err
}

// Verify tells whether password is the one phc was computed from. A hash this binary would not have written is an
// error, never a match.
func (h *Hasher) Verify(phc, password string) (bool, error) {
	fields := strings.Split(phc, "$")
	if len(fields) != 6 || fields[0] != "" || fields[1] != "argon2id" ||
		fields[2] != fmt.Sprintf("v=%d", argon2.Version) || fields[3] != params {
		return false, errUnreadableHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(fields[4])
	if err != nil || len(salt) < saltLength {
		return false, errUnreadableHash
	}
	want, err := base64.RawStdEncoding.DecodeString(fields[5])
	if err != nil || len(want) != keyLength {
		return false, errUnreadableHash
	}
	var match bool
	err = h.hold(func() error {
		match = subtle.ConstantTimeCompare(argon2.IDKey([]byte(password), salt, passes, memory, lanes, keyLength), want) == 1
		return nil
	})
	return match, err
}

// VerifyDummy costs what Verify costs, under the same slot, for an operator who does not exist or cannot log in: the
// time of a refusal tells nothing.
func (h *Hasher) VerifyDummy(password string) error {
	return h.hold(func() error {
		runtime.KeepAlive(argon2.IDKey([]byte(password), make([]byte, saltLength), passes, memory, lanes, keyLength))
		return nil
	})
}
