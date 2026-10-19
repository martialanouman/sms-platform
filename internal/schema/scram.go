package schema

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
)

const scramIterations = 4096

// SCRAMVerifier is the SCRAM-SHA-256 verifier Postgres stores for password, so that the clear text never reaches the
// server; a secret outside printable ASCII is refused, SASLprep would change it.
func SCRAMVerifier(password string, salt []byte) (string, error) {
	if password == "" {
		return "", errors.New("empty secret")
	}
	for _, c := range []byte(password) {
		if c < 0x20 || c > 0x7e {
			return "", errors.New("secret outside printable ASCII")
		}
	}
	salted, err := pbkdf2.Key(sha256.New, password, salt, scramIterations, sha256.Size)
	if err != nil {
		return "", err
	}
	mac := func(msg string) []byte {
		h := hmac.New(sha256.New, salted)
		h.Write([]byte(msg))
		return h.Sum(nil)
	}
	stored := sha256.Sum256(mac("Client Key"))
	b64 := base64.StdEncoding.EncodeToString
	return fmt.Sprintf("SCRAM-SHA-256$%d:%s$%s:%s", scramIterations, b64(salt), b64(stored[:]), b64(mac("Server Key"))), nil
}
