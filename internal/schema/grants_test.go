package schema_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/martialanouman/sms-platform/internal/schema"
)

var classified = schema.Classification{
	"audit_log": {Audit: schema.Exempt, Reason: "journal"},
	"customers": {Audit: schema.Diff},
}

func TestParseGrants(t *testing.T) {
	got, err := schema.ParseGrants("admin-api-svc", []byte(`
control_plane:
  audit_log: [select, insert]
  customers: [update]
`), classified)
	if err != nil {
		t.Fatal(err)
	}
	want := schema.Grants{Service: "admin-api-svc", Tables: map[string][]schema.Privilege{
		"audit_log": {schema.Select, schema.Insert},
		"customers": {schema.Update},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseGrantsRefuses(t *testing.T) {
	for name, tc := range map[string]struct{ service, yaml, want string }{
		"upper case service":  {"Admin-svc", `control_plane: {audit_log: [select]}`, `service "Admin-svc"`},
		"underscored service": {"admin_svc", `control_plane: {audit_log: [select]}`, `service "admin_svc"`},
		"other database":      {"svc", `message_state: {audit_log: [select]}`, `database "message_state"`},
		"unclassified table":  {"svc", `control_plane: {jobs: [select]}`, `table "jobs" is not classified`},
		"truncate":            {"svc", `control_plane: {audit_log: [truncate]}`, `privilege "truncate"`},
		"no privilege":        {"svc", `control_plane: {audit_log: []}`, `table "audit_log" grants nothing`},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := schema.ParseGrants(tc.service, []byte(tc.yaml), classified)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRole(t *testing.T) {
	if got := schema.Role("admin-api-svc"); got != "admin_api_svc" {
		t.Fatalf("got %q", got)
	}
}
