package dashboard_test

import (
	"slices"
	"testing"

	"github.com/martialanouman/sms-platform/internal/dashboard"
	"github.com/martialanouman/sms-platform/internal/platform/scopes"
)

func rw(tags ...string) []string {
	var out []string
	for _, tag := range tags {
		out = append(out, tag+":read", tag+":write")
	}
	return out
}

func read(tags ...string) []string {
	var out []string
	for _, tag := range tags {
		out = append(out, tag+":read")
	}
	return out
}

func set(parts ...[]string) []string {
	out := slices.Concat(parts...)
	slices.Sort(out)
	return slices.Compact(out)
}

var supportReads = read("accounts", "customers", "customer-groups", "sender-ids", "routes", "exact-routes", "connectors",
	"smpp-sessions", "cdr", "billing", "traffic")

func spec85() map[string][]string {
	return map[string][]string{
		"Propriétaire": set(scopes.All, scopes.Permissions),
		"Exploitation": set(rw("routes", "exact-routes", "connectors", "smpp-sessions", "antispam", "routing-scripts",
			"sender-rewrite", "inbound-numbers", "suppressions", "opt-out-keywords", "dead-letters", "platform"),
			[]string{"routing-scripts:publish", "msisdn:reveal", "cdr:export_bulk", "stream:read"},
			read("accounts", "customers", "customer-groups", "sender-ids", "credentials", "billing", "cdr", "jobs", "traffic")),
		"Scripts":   set([]string{"routing-scripts:read", "routing-scripts:write", "traffic:read", "stream:read"}),
		"Support":   set(supportReads, []string{"msisdn:reveal", "stream:read"}),
		"Finance":   set(supportReads, []string{"billing:write", "billing:topup", "stream:read"}),
		"Reporting": set([]string{"billing:read", "traffic:read", "stream:read"}),
		"Clientèle": set(rw("customers", "accounts", "credentials", "customer-groups", "sender-ids"), []string{"billing:read", "billing:write"}),
		"Conformité": set(rw("suppressions", "opt-out-keywords"), read("inbound-numbers", "accounts", "customers", "customer-groups", "sender-ids", "cdr"),
			[]string{"suppressions:delete", "gdpr:erase", "content:erase", "msisdn:reveal", "cdr:export_bulk"}),
		"Audit": {"audit:read"},
		"noc": {"accounts:read", "antispam:read", "billing:read", "cdr:read", "connectors:read", "credentials:read", "customer-groups:read",
			"customers:read", "dead-letters:read", "exact-routes:read", "inbound-numbers:read", "jobs:read", "opt-out-keywords:read",
			"platform:read", "routes:read", "routing-scripts:read", "sender-ids:read", "sender-rewrite:read", "smpp-sessions:read",
			"stream:read", "suppressions:read", "traffic:read"},
	}
}

func TestDefaultRolesAreTheTableOfSpec85(t *testing.T) {
	want := spec85()
	roles := dashboard.DefaultRoles()
	if len(roles) != len(want) {
		t.Fatalf("%d default roles, want %d", len(roles), len(want))
	}
	for _, r := range roles {
		if !slices.Equal(r.Permissions, want[r.Name]) {
			t.Errorf("%s holds %v, want %v", r.Name, r.Permissions, want[r.Name])
		}
		if r.Description == "" {
			t.Errorf("%s has no description", r.Name)
		}
	}
}

func TestOnlyTheOwnerHoldsWhatSpec85KeepsOutOfEveryRole(t *testing.T) {
	ownerOnly := []string{"content:read", "content:write", "operators:read", "operators:write", "roles:read", "roles:write",
		"sessions:read", "sessions:write", "gateway:personal_token"}
	held := map[string]bool{}
	for _, r := range dashboard.DefaultRoles() {
		if r.Name == dashboard.Owner {
			continue
		}
		for _, p := range r.Permissions {
			held[p] = true
		}
	}
	for _, p := range slices.Concat(scopes.All, scopes.Permissions) {
		if held[p] == slices.Contains(ownerOnly, p) {
			t.Errorf("%s: held by a default role other than %s = %v", p, dashboard.Owner, held[p])
		}
	}
}

func TestDefaultRolesHoldOnlyGeneratedPermissions(t *testing.T) {
	known := slices.Concat(scopes.All, scopes.Permissions)
	for _, r := range dashboard.DefaultRoles() {
		for _, p := range r.Permissions {
			if !slices.Contains(known, p) {
				t.Errorf("%s holds %s, absent from the contracts", r.Name, p)
			}
		}
	}
}

func TestEveryDefaultRoleThatReadsTheTrafficReadsItsStream(t *testing.T) {
	for _, r := range dashboard.DefaultRoles() {
		if slices.Contains(r.Permissions, "traffic:read") && !slices.Contains(r.Permissions, "stream:read") {
			t.Errorf("%s reads the traffic without its stream", r.Name)
		}
	}
}
