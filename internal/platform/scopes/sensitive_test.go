package scopes_test

import (
	"slices"
	"testing"

	"github.com/martialanouman/sms-platform/internal/platform/scopes"
)

func TestSensitiveAreTheTenOfSpec07(t *testing.T) {
	want := []string{"audit:read", "billing:topup", "cdr:export_bulk", "content:erase", "content:read", "content:write",
		"gdpr:erase", "msisdn:reveal", "routing-scripts:publish", "suppressions:delete"}
	got := slices.Sorted(slices.Values(scopes.Sensitive))
	if !slices.Equal(got, want) {
		t.Fatalf("Sensitive = %v, want %v", got, want)
	}
	for _, s := range scopes.Sensitive {
		if !slices.Contains(scopes.All, s) {
			t.Errorf("%s is not a scope of the contract", s)
		}
	}
}
