package schema_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/martialanouman/sms-platform/internal/schema"
)

var classified = map[string]schema.Classification{
	"control_plane": {
		"audit_log": {Audit: schema.Exempt, Reason: "journal"},
		"customers": {Audit: schema.Diff},
	},
	"dashboard": {"sessions": {Audit: schema.Exempt, Reason: "état"}},
}

func TestParseGrants(t *testing.T) {
	got, err := schema.ParseGrants("admin-api-svc", []byte(`
control_plane:
  audit_log: [select, insert]
  customers: [update]
dashboard:
  sessions: [delete]
`), classified)
	if err != nil {
		t.Fatal(err)
	}
	want := schema.Grants{Service: "admin-api-svc", Databases: map[string]map[string][]string{
		"control_plane": {"audit_log": {"select", "insert"}, "customers": {"update"}},
		"dashboard":     {"sessions": {"delete"}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseGrantsRefuses(t *testing.T) {
	for name, tc := range map[string]struct{ service, yaml, want string }{
		"upper case service":  {"Admin-svc", `control_plane: {audit_log: [select]}`, `service "Admin-svc"`},
		"underscored service": {"admin_svc", `control_plane: {audit_log: [select]}`, `service "admin_svc"`},
		"unknown database":    {"svc", `message_state: {audit_log: [select]}`, `database "message_state" is unknown`},
		"other database's":    {"svc", `control_plane: {sessions: [select]}`, `table "sessions" is not classified in control_plane`},
		"too long service":    {strings.Repeat("a", 64), `control_plane: {audit_log: [select]}`, `is not a service name`},
		"built-in role":       {"pg-signal-backend", `control_plane: {audit_log: [select]}`, `is not a service name`},
		"twice":               {"svc", `control_plane: {audit_log: [select, select]}`, `"select" on "audit_log" is declared twice`},
		"unclassified table":  {"svc", `dashboard: {jobs: [select]}`, `table "jobs" is not classified in dashboard`},
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

func TestGrantsSQLIsStable(t *testing.T) {
	g := []schema.Grants{{Service: "svc", Databases: map[string]map[string][]string{"dashboard": {
		"a": {"select"}, "b": {"select"}, "c": {"select"}, "d": {"select"},
	}}}}
	first := string(schema.GrantsSQL("dashboard", g))
	for range 20 {
		if got := string(schema.GrantsSQL("dashboard", g)); got != first {
			t.Fatalf("two renderings differ:\n%s\n%s", first, got)
		}
	}
}

func TestGrantsSQLOfADatabaseTouchesItAloneButKeepsEveryRole(t *testing.T) {
	got := string(schema.GrantsSQL("dashboard", []schema.Grants{
		{Service: "gateway-svc", Databases: map[string]map[string][]string{"control_plane": {"audit_log": {"select"}}}},
		{Service: "dashboard", Databases: map[string]map[string][]string{"dashboard": {"sessions": {"select", "delete"}}}},
	}))
	for _, want := range []string{
		"CREATE ROLE gateway_svc NOLOGIN;", "ALTER ROLE gateway_svc LOGIN;",
		"CREATE ROLE dashboard NOLOGIN;", "ALTER ROLE dashboard LOGIN;",
		"REVOKE ALL ON SCHEMA dashboard FROM gateway_svc;", "REVOKE ALL ON DATABASE dashboard FROM gateway_svc;",
		"GRANT CONNECT ON DATABASE dashboard TO dashboard;", "GRANT USAGE ON SCHEMA dashboard TO dashboard;",
		"GRANT select, delete ON dashboard.sessions TO dashboard;",
		"ARRAY['gateway_svc', 'dashboard']",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, refused := range []string{"control_plane", "TO gateway_svc"} {
		if strings.Contains(got, refused) {
			t.Errorf("holds %q", refused)
		}
	}
}
