package keyset_test

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/martialanouman/sms-platform/internal/platform/keyset"
)

const id = "0199f0a2-7c1e-7000-8000-000000000001"

func TestACursorKeepsTheMicrosecondAndTheID(t *testing.T) {
	at := time.Date(2026, 10, 21, 12, 0, 0, 123456789, time.UTC)

	gotAt, gotID, ok := keyset.Parse(keyset.Encode(at, id))
	if !ok || !gotAt.Equal(at.Truncate(time.Microsecond)) || gotID != id {
		t.Fatalf("Parse = %v %q %v", gotAt, gotID, ok)
	}
}

func TestACursorNotOfThisFormIsRefused(t *testing.T) {
	for name, cursor := range map[string]string{
		"not base64":   "!!",
		"no id":        base64.RawURLEncoding.EncodeToString([]byte("1761048000000000")),
		"id not uuid":  base64.RawURLEncoding.EncodeToString([]byte("1761048000000000 abc")),
		"at not micro": base64.RawURLEncoding.EncodeToString([]byte("noon " + id)),
	} {
		if _, _, ok := keyset.Parse(cursor); ok {
			t.Errorf("%s accepted", name)
		}
	}
}
