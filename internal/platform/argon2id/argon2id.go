// Package argon2id hashes and verifies short secrets (passwords, SMPP bind passwords) as PHC strings, under one set of
// parameters for the whole platform.
package argon2id

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	memory     = 64 * 1024
	passes     = 3
	lanes      = 4
	saltLength = 16
	keyLength  = 32
)

var params = fmt.Sprintf("m=%d,t=%d,p=%d", memory, passes, lanes)

var errUnreadable = errors.New("unreadable argon2id hash")

// Hash is the PHC string of secret under a fresh salt.
func Hash(secret string) string {
	salt := make([]byte, saltLength)
	_, _ = rand.Read(salt)
	key := argon2.IDKey([]byte(secret), salt, passes, memory, lanes, keyLength)
	return fmt.Sprintf("$argon2id$v=%d$%s$%s$%s", argon2.Version, params,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

// Verify tells whether secret is the one phc was computed from. A hash this package would not have written is an
// error, never a match.
func Verify(phc, secret string) (bool, error) {
	fields := strings.Split(phc, "$")
	if len(fields) != 6 || fields[0] != "" || fields[1] != "argon2id" ||
		fields[2] != fmt.Sprintf("v=%d", argon2.Version) || fields[3] != params {
		return false, errUnreadable
	}
	salt, err := base64.RawStdEncoding.DecodeString(fields[4])
	if err != nil || len(salt) < saltLength {
		return false, errUnreadable
	}
	want, err := base64.RawStdEncoding.DecodeString(fields[5])
	if err != nil || len(want) != keyLength {
		return false, errUnreadable
	}
	return subtle.ConstantTimeCompare(argon2.IDKey([]byte(secret), salt, passes, memory, lanes, keyLength), want) == 1, nil
}

// Dummy costs what Verify costs, for a caller who must refuse without telling why by its timing.
func Dummy(secret string) {
	runtime.KeepAlive(argon2.IDKey([]byte(secret), make([]byte, saltLength), passes, memory, lanes, keyLength))
}
