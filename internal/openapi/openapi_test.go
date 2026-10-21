package openapi_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/martialanouman/sms-platform/internal/openapi"
)

const admin = `
openapi: 3.1.0
tags:
  - name: routing-scripts
  - name: audit
  - name: routes
paths:
  /admin/routing-scripts:
    parameters:
      - name: limit
        in: query
    get:
      operationId: list-routing-scripts
      tags: [routing-scripts]
      security: [{ OperatorBearer: ["routing-scripts:read"] }]
    post:
      operationId: create-routing-script
      tags: [routing-scripts]
      security: [{ OperatorBearer: ["routing-scripts:write"] }]
  /admin/routing-scripts/{id}:
    get:
      operationId: get-routing-script
      tags: [routing-scripts]
      security: [{ OperatorBearer: ["routing-scripts:read"] }]
    patch:
      operationId: update-routing-script
      tags: [routing-scripts]
      security: [{ OperatorBearer: ["routing-scripts:write"] }]
    delete:
      operationId: delete-routing-script
      tags: [routing-scripts]
      security: [{ OperatorBearer: ["routing-scripts:write"] }]
  /admin/routing-scripts/{id}/assignment:
    put:
      operationId: assign-routing-script
      tags: [routing-scripts]
      security: [{ OperatorBearer: ["routing-scripts:publish"] }]
  /admin/routing-scripts/{id}/publish:
    post:
      operationId: publish-routing-script
      tags: [routing-scripts]
      security: [{ OperatorBearer: ["routing-scripts:publish"] }]
  /admin/audit-log:
    get:
      operationId: list-audit-log
      tags: [audit]
      security: [{ OperatorBearer: ["audit:read"] }]
`

func parse(t *testing.T, doc string) openapi.Contract {
	t.Helper()
	c, err := openapi.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestScopes(t *testing.T) {
	s, errs := parse(t, admin).Scopes()
	if errs != nil {
		t.Fatal(errs)
	}
	want := []string{"audit:read", "msisdn:reveal", "routing-scripts:publish", "routing-scripts:read", "routing-scripts:write"}
	if !reflect.DeepEqual(s, want) {
		t.Errorf("Scopes = %v, want %v", s, want)
	}
}

func TestScopesReadsEveryMethod(t *testing.T) {
	for _, method := range []string{"get", "put", "post", "delete", "patch", "head", "options", "trace"} {
		t.Run(method, func(t *testing.T) {
			doc := admin + "  /admin/routes:\n    " + method + ":\n      operationId: x\n      tags: [routes]\n      security: [{ OperatorBearer: [\"routes:admin\"] }]\n"
			if _, errs := parse(t, doc).Scopes(); len(errs) != 1 {
				t.Fatalf("errs = %v, want one", errs)
			}
		})
	}
}

func TestScopesAcceptsSensitiveActions(t *testing.T) {
	for _, scope := range []string{"routing-scripts:publish", "suppressions:delete", "billing:topup", "content:erase", "gdpr:erase", "cdr:export_bulk"} {
		t.Run(scope, func(t *testing.T) {
			tag, _, _ := strings.Cut(scope, ":")
			doc := "tags:\n  - name: " + tag + "\npaths:\n  /x:\n    post:\n      operationId: x\n      tags: [" + tag + "]\n      security: [{ OperatorBearer: [\"" + scope + "\"] }]\n"
			if _, errs := parse(t, doc).Scopes(); errs != nil {
				t.Fatal(errs)
			}
		})
	}
}

func TestScopesRefuses(t *testing.T) {
	for name, op := range map[string]string{
		"no tag":            `security: [{ OperatorBearer: ["routes:read"] }]`,
		"two tags":          "tags: [routes, connectors]\n      security: [{ OperatorBearer: [\"routes:read\"] }]",
		"no security":       "tags: [routes]",
		"empty security":    "tags: [routes]\n      security: []",
		"two requirements":  "tags: [routes]\n      security: [{ OperatorBearer: [\"routes:read\"] }, { OperatorBearer: [\"routes:write\"] }]",
		"two schemes":       "tags: [routes]\n      security: [{ OperatorBearer: [\"routes:read\"], Other: [] }]",
		"two scopes":        "tags: [routes]\n      security: [{ OperatorBearer: [\"routes:read\", \"routes:write\"] }]",
		"no scope":          "tags: [routes]\n      security: [{ OperatorBearer: [] }]",
		"other tag":         "tags: [routes]\n      security: [{ OperatorBearer: [\"connectors:read\"] }]",
		"unknown action":    "tags: [routes]\n      security: [{ OperatorBearer: [\"routes:admin\"] }]",
		"action of another": "tags: [routes]\n      security: [{ OperatorBearer: [\"routes:topup\"] }]",
		"no action":         "tags: [routes]\n      security: [{ OperatorBearer: [\"routes\"] }]",
		"undeclared tag":    "tags: [route]\n      security: [{ OperatorBearer: [\"route:read\"] }]",
	} {
		t.Run(name, func(t *testing.T) {
			doc := admin + "  /admin/routes:\n    get:\n      operationId: list-routes\n      " + op + "\n"
			s, errs := parse(t, doc).Scopes()
			if len(errs) != 1 || !strings.Contains(errs[0].Error(), "list-routes") {
				t.Fatalf("errs = %v, want one error naming list-routes", errs)
			}
			if s != nil {
				t.Errorf("Scopes = %v, want nil on error", s)
			}
		})
	}
}

const public = `
openapi: 3.1.0
paths:
  /messages:
    post:
      operationId: submit-message
      security: [{ BearerApiKey: [] }]
  /messages/{id}:
    get:
      operationId: get-message
      security: [{ BearerApiKey: [] }]
webhooks:
  mo:
    post:
      operationId: on-mo
`

func TestScopesRefusesMissingOperationID(t *testing.T) {
	doc := admin + "  /admin/routes:\n    get:\n      tags: [routes]\n      security: [{ OperatorBearer: [\"routes:read\"] }]\n"
	if _, errs := parse(t, doc).Scopes(); len(errs) != 1 || !strings.Contains(errs[0].Error(), "/admin/routes") {
		t.Fatalf("errs = %v, want one error naming /admin/routes", errs)
	}
}

func TestGuardsRefuseAContractWithoutOperations(t *testing.T) {
	c := parse(t, "openapi: 3.1.0\ntags:\n  - name: routes\npaths: {}\n")
	if _, errs := c.Scopes(); len(errs) != 1 {
		t.Errorf("Scopes errs = %v, want one", errs)
	}
	if errs := c.PublicSecurity(); len(errs) != 1 {
		t.Errorf("PublicSecurity errs = %v, want one", errs)
	}
}

func TestPublicSecurity(t *testing.T) {
	if errs := parse(t, public).PublicSecurity(); errs != nil {
		t.Fatal(errs)
	}
	for name, sec := range map[string]string{
		"none":             "",
		"empty":            "\n      security: []",
		"scope":            "\n      security: [{ BearerApiKey: [\"messages:read\"] }]",
		"other scheme":     "\n      security: [{ Other: [] }]",
		"two requirements": "\n      security: [{ BearerApiKey: [] }, {}]",
		"anonymous":        "\n      security: [{}]",
	} {
		t.Run(name, func(t *testing.T) {
			doc := strings.Replace(public, "      operationId: get-message\n      security: [{ BearerApiKey: [] }]", "      operationId: get-message"+sec, 1)
			errs := parse(t, doc).PublicSecurity()
			if len(errs) != 1 || !strings.Contains(errs[0].Error(), "get-message") {
				t.Fatalf("errs = %v, want one error naming get-message", errs)
			}
		})
	}
}

const bff = `
openapi: 3.1.0
paths:
  /health:
    get:
      operationId: health
      security: []
  /auth/me:
    get:
      operationId: me
      security: [{ session: [] }]
  /operators:
    get:
      operationId: listOperators
      security: [{ session: ["operators:read"] }]
    post:
      operationId: createOperator
      security: [{ session: ["operators:write"] }]
  /operators/{operatorId}/access-link:
    post:
      operationId: requestOperatorAccessLink
      security: [{ session: ["operators:write"] }]
  /roles:
    get:
      operationId: listRoles
      security: [{ session: ["roles:read"] }]
    post:
      operationId: createRole
      security: [{ session: ["roles:write"] }]
  /sessions:
    get:
      operationId: listSessions
      security: [{ session: ["sessions:read"] }]
    delete:
      operationId: deleteSession
      security: [{ session: ["sessions:write"] }]
  /access-log:
    get:
      operationId: listAccessLog
      security: [{ session: ["audit:read"] }]
  /gateway-token:
    post:
      operationId: issueGatewayToken
      security: [{ session: ["gateway:personal_token"] }]
`

var adminScopes = []string{"audit:read", "msisdn:reveal"}

func TestPermissions(t *testing.T) {
	p, errs := parse(t, bff).Permissions(adminScopes)
	if errs != nil {
		t.Fatal(errs)
	}
	want := []string{"gateway:personal_token", "operators:read", "operators:write", "roles:read", "roles:write", "sessions:read", "sessions:write"}
	if !reflect.DeepEqual(p, want) {
		t.Errorf("Permissions = %v, want %v", p, want)
	}
}

func TestPermissionsAcceptsEveryPublicOperation(t *testing.T) {
	for _, id := range []string{"health", "login", "logout", "setPasswordFromAccessLink"} {
		t.Run(id, func(t *testing.T) {
			doc := strings.Replace(bff, "operationId: health", "operationId: "+id, 1)
			if _, errs := parse(t, doc).Permissions(adminScopes); errs != nil {
				t.Fatal(errs)
			}
		})
	}
}

func TestPermissionsRefusesAPublicOperationWithoutSecurity(t *testing.T) {
	doc := strings.Replace(bff, "operationId: health\n      security: []", "operationId: health", 1)
	if _, errs := parse(t, doc).Permissions(adminScopes); len(errs) != 1 || !strings.Contains(errs[0].Error(), "health") {
		t.Fatalf("errs = %v, want one error naming health", errs)
	}
}

func TestPermissionsRefusesAnOrphanPermission(t *testing.T) {
	doc := strings.Replace(bff, `session: ["roles:write"]`, `session: ["roles:read"]`, 1)
	if _, errs := parse(t, doc).Permissions(adminScopes); len(errs) != 1 || !strings.Contains(errs[0].Error(), "roles:write") {
		t.Fatalf("errs = %v, want one error naming roles:write", errs)
	}
}

func TestPermissionsRefuses(t *testing.T) {
	for name, sec := range map[string]string{
		"no security":        "",
		"public":             "\n      security: []",
		"two requirements":   "\n      security: [{ session: [] }, { session: [] }]",
		"two schemes":        "\n      security: [{ session: [], Other: [] }]",
		"other scheme":       "\n      security: [{ Other: [] }]",
		"two permissions":    "\n      security: [{ session: [\"operators:read\", \"roles:read\"] }]",
		"unknown permission": "\n      security: [{ session: [\"operators:admin\"] }]",
		"unknown scope":      "\n      security: [{ session: [\"routes:read\"] }]",
	} {
		t.Run(name, func(t *testing.T) {
			doc := bff + "  /roles/{roleId}:\n    get:\n      operationId: getRole" + sec + "\n"
			p, errs := parse(t, doc).Permissions(adminScopes)
			if len(errs) != 1 || !strings.Contains(errs[0].Error(), "getRole") {
				t.Fatalf("errs = %v, want one error naming getRole", errs)
			}
			if p != nil {
				t.Errorf("Permissions = %v, want nil on error", p)
			}
		})
	}
}

func TestPermissionsRefusesAPathUnderTheProxy(t *testing.T) {
	for _, path := range []string{"/gw", "/gw/admin/routes"} {
		t.Run(path, func(t *testing.T) {
			doc := bff + "  " + path + ":\n    get:\n      operationId: x\n      security: [{ session: [] }]\n"
			if _, errs := parse(t, doc).Permissions(adminScopes); len(errs) != 1 || !strings.Contains(errs[0].Error(), path) {
				t.Fatalf("errs = %v, want one error naming %s", errs, path)
			}
		})
	}
	doc := bff + "  /gwx:\n    get:\n      operationId: x\n      security: [{ session: [] }]\n"
	if _, errs := parse(t, doc).Permissions(adminScopes); errs != nil {
		t.Fatal(errs)
	}
}

func TestPermissionsRefusesAPermissionThatIsAlsoAScope(t *testing.T) {
	_, errs := parse(t, bff).Permissions(append(adminScopes, "sessions:read"))
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "sessions:read") {
		t.Fatalf("errs = %v, want one error naming sessions:read", errs)
	}
}

const codes = `
openapi: 3.1.0
components:
  schemas:
    Error:
      properties:
        code:
          enum: [unauthenticated, not_found]
    ErrorCode:
      enum: [no_route, delivery_failed]
`

func TestUnknownCodesRefusesAContractWithoutCodes(t *testing.T) {
	if errs := parse(t, "openapi: 3.1.0\n").UnknownCodes([]string{"not_found"}); len(errs) != 1 {
		t.Fatalf("errs = %v, want one", errs)
	}
}

func TestUnknownCodes(t *testing.T) {
	known := []string{"unauthenticated", "not_found", "no_route", "delivery_failed"}
	if errs := parse(t, codes).UnknownCodes(known); errs != nil {
		t.Fatal(errs)
	}
	for _, missing := range []string{"not_found", "delivery_failed"} {
		t.Run(missing, func(t *testing.T) {
			errs := parse(t, codes).UnknownCodes(slicesWithout(known, missing))
			if len(errs) != 1 || !strings.Contains(errs[0].Error(), missing) {
				t.Fatalf("errs = %v, want one error naming %s", errs, missing)
			}
		})
	}
}

func slicesWithout(s []string, v string) []string {
	var out []string
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

func TestSpecErrors(t *testing.T) {
	md := "## 9.2 gRPC\n\n| `billing-svc` | x | y |\n\n## 9.3 Codes d'erreur\n\n" +
		"| Code | HTTP | SMPP | Quand |\n|---|---|---|---|\n" +
		"| `unauthenticated` | 401 | `ESME_RINVPASWD` | Clé (jamais `ESME_RINVSYSID`) |\n" +
		"| `queue_full`, `service_unavailable` | 503 | `ESME_RMSGQFUL`, `ESME_RSYSERR` | |\n" +
		"| `not_found`, `conflict` | 404, 409 | — | |\n" +
		"| `no_route` | — | — | |\n" +
		"| `a`, `b` | 404, — | — | |\n\nVoir `later` | ou | ailleurs.\n" +
		"Les codes après l'ACK…\n\n## 9.4 Suite\n\n| `later` | — | — | |\n"
	want := []openapi.SpecError{
		{Code: "unauthenticated", HTTP: "401", SMPP: "ESME_RINVPASWD"},
		{Code: "queue_full", HTTP: "503", SMPP: "ESME_RMSGQFUL"},
		{Code: "service_unavailable", HTTP: "503", SMPP: "ESME_RSYSERR"},
		{Code: "not_found", HTTP: "404"},
		{Code: "conflict", HTTP: "409"},
		{Code: "no_route"},
		{Code: "a", HTTP: "404"},
		{Code: "b"},
	}
	got, err := openapi.SpecErrors([]byte(md))
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("SpecErrors = %v, %v, want %v", got, err, want)
	}
}

func TestSpecErrorsRefusesAMismatchedRow(t *testing.T) {
	for _, row := range []string{
		"| `a`, `b`, `c` | 404, 409 | — | |",
		"| `a`, `b`, `c` | — | `X`, `Y` | |",
		"| `a`, `b` | 404, 409, 403 | — | |",
	} {
		if _, err := openapi.SpecErrors([]byte("## 9.3 Codes d'erreur\n\n" + row + "\n")); err == nil {
			t.Errorf("SpecErrors accepted a mismatched row: %s", row)
		}
	}
}

func TestGo(t *testing.T) {
	want := `// Code generated by tools/openapi from contracts/openapi-admin.yaml and contracts/openapi-bff.yaml. DO NOT EDIT.

// Package scopes lists the scopes of the Admin API and the permissions of the dashboard BFF, generated from their contracts.
package scopes

// All is every scope, sorted.
var All = []string{
	"audit:read",
	"msisdn:reveal",
}

// Permissions is every permission of the dashboard BFF, sorted.
var Permissions = []string{
	"operators:read",
}
`
	got := openapi.Go([]string{"audit:read", "msisdn:reveal"}, []string{"operators:read"})
	if string(got) != want {
		t.Errorf("Go() =\n%s\nwant\n%s", got, want)
	}
}

func TestTS(t *testing.T) {
	want := `// Code generated by tools/openapi from contracts/openapi-admin.yaml and contracts/openapi-bff.yaml. DO NOT EDIT.

export const scopes = ['audit:read', 'msisdn:reveal', 'operators:read'] as const

export type Scope = (typeof scopes)[number]
`
	got := openapi.TS([]string{"msisdn:reveal", "audit:read"}, []string{"operators:read"})
	if string(got) != want {
		t.Errorf("TS() =\n%s\nwant\n%s", got, want)
	}
}

func TestParseRefusesInvalidYAML(t *testing.T) {
	if _, err := openapi.Parse([]byte("paths: [")); err == nil {
		t.Fatal("Parse accepted invalid YAML")
	}
}
