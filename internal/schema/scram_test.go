package schema_test

import (
	"strings"
	"testing"

	"github.com/martialanouman/sms-platform/internal/schema"
)

func TestSCRAMVerifierRefusesASecretPostgresWouldNormalise(t *testing.T) {
	for _, secret := range []string{"", "mot-de-passe-é", "tab\tinside", "new\nline"} {
		if v, err := schema.SCRAMVerifier(secret, make([]byte, 16)); err == nil {
			t.Errorf("SCRAMVerifier(%q) = %s, want a refusal", secret, v)
		}
	}
}

func TestSCRAMVerifierHasThePostgresShape(t *testing.T) {
	v, err := schema.SCRAMVerifier("s3cret!~ ok", []byte("0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(v, "SCRAM-SHA-256$4096:MDEyMzQ1Njc4OWFiY2RlZg==$") || strings.Contains(v, "s3cret") {
		t.Errorf("verifier %s", v)
	}
}
