package argon2id_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/martialanouman/sms-platform/internal/platform/argon2id"
)

func TestAHashVerifiesItsSecretOnly(t *testing.T) {
	phc := argon2id.Hash("s3cret!")
	if !strings.HasPrefix(phc, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Fatalf("phc = %s", phc)
	}
	if ok, err := argon2id.Verify(phc, "s3cret!"); err != nil || !ok {
		t.Fatalf("own secret: %v %v", ok, err)
	}
	if ok, err := argon2id.Verify(phc, "s3cret?"); err != nil || ok {
		t.Fatalf("other secret: %v %v", ok, err)
	}
	if again := argon2id.Hash("s3cret!"); again == phc {
		t.Fatal("two hashes share their salt")
	}
}

func TestAHashThisPackageWouldNotWriteIsAnError(t *testing.T) {
	phc := argon2id.Hash("x")
	for _, bad := range []string{
		"",
		"x" + phc,
		phc[:strings.LastIndex(phc, "$")] + "$" + base64.RawStdEncoding.EncodeToString(make([]byte, 64)),
		strings.Replace(phc, "argon2id", "argon2i", 1),
		strings.Replace(phc, "v=19", "v=16", 1),
		strings.Replace(phc, "t=3", "t=1", 1),
		phc[:strings.LastIndex(phc, "$")] + "$!!",
		phc[:strings.LastIndex(phc, "$")] + "$" + strings.Repeat("A", 10),
		strings.Join(append(strings.Split(phc, "$")[:4], "AAAA", strings.Split(phc, "$")[5]), "$"),
	} {
		if ok, err := argon2id.Verify(bad, "x"); err == nil || ok {
			t.Errorf("%q: %v %v", bad, ok, err)
		}
	}
}
