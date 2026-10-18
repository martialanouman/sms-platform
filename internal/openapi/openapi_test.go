package openapi_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/martialanouman/sms-platform/internal/openapi"
)

const admin = `
openapi: 3.1.0
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
	wantAll := []string{"audit:read", "msisdn:reveal", "routing-scripts:publish", "routing-scripts:read", "routing-scripts:write"}
	if !reflect.DeepEqual(s.All, wantAll) {
		t.Errorf("All = %v, want %v", s.All, wantAll)
	}
	wantBy := map[string]string{
		"list-routing-scripts":   "routing-scripts:read",
		"get-routing-script":     "routing-scripts:read",
		"create-routing-script":  "routing-scripts:write",
		"update-routing-script":  "routing-scripts:write",
		"delete-routing-script":  "routing-scripts:write",
		"assign-routing-script":  "routing-scripts:publish",
		"publish-routing-script": "routing-scripts:publish",
		"list-audit-log":         "audit:read",
	}
	if !reflect.DeepEqual(s.ByOperation, wantBy) {
		t.Errorf("ByOperation = %v, want %v", s.ByOperation, wantBy)
	}
}

func TestScopesAcceptsSensitiveActions(t *testing.T) {
	for _, scope := range []string{"routing-scripts:publish", "suppressions:delete", "billing:topup", "content:erase", "gdpr:erase", "cdr:export_bulk"} {
		t.Run(scope, func(t *testing.T) {
			tag, _, _ := strings.Cut(scope, ":")
			doc := "paths:\n  /x:\n    post:\n      operationId: x\n      tags: [" + tag + "]\n      security: [{ OperatorBearer: [\"" + scope + "\"] }]\n"
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
	} {
		t.Run(name, func(t *testing.T) {
			doc := admin + "  /admin/routes:\n    get:\n      operationId: list-routes\n      " + op + "\n"
			s, errs := parse(t, doc).Scopes()
			if len(errs) != 1 || !strings.Contains(errs[0].Error(), "list-routes") {
				t.Fatalf("errs = %v, want one error naming list-routes", errs)
			}
			if s.ByOperation != nil {
				t.Errorf("ByOperation = %v, want nil on error", s.ByOperation)
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

func TestSpecCodes(t *testing.T) {
	md := "## 9.2 gRPC\n\n| `billing-svc` | x | y |\n\n## 9.3 Codes d'erreur\n\n" +
		"| Code | HTTP | SMPP | Quand |\n|---|---|---|---|\n" +
		"| `unauthenticated` | 401 | `ESME_RINVPASWD` | Clé (jamais `ESME_RINVSYSID`) |\n" +
		"| `queue_full`, `service_unavailable` | 503 | `ESME_RMSGQFUL` | |\n\n" +
		"Les codes après l'ACK…\n\n## 9.4 Suite\n\n| `later` | — | — | |\n"
	want := []string{"unauthenticated", "queue_full", "service_unavailable"}
	if got := openapi.SpecCodes([]byte(md)); !reflect.DeepEqual(got, want) {
		t.Errorf("SpecCodes = %v, want %v", got, want)
	}
}

func TestGo(t *testing.T) {
	s := openapi.Scopes{
		All:         []string{"audit:read", "msisdn:reveal"},
		ByOperation: map[string]string{"list-audit-log": "audit:read", "get-audit": "audit:read"},
	}
	want := `// Code generated by tools/openapi from contracts/openapi-admin.yaml. DO NOT EDIT.

// Package scopes lists the scopes of the Admin API, generated from its contract.
package scopes

// Scope grants the operations that declare it.
type Scope string

// All is every scope, sorted.
var All = []Scope{
	"audit:read",
	"msisdn:reveal",
}

// ByOperation is the scope each operation declares.
var ByOperation = map[string]Scope{
	"get-audit":      "audit:read",
	"list-audit-log": "audit:read",
}
`
	got, err := s.Go()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("Go() =\n%s\nwant\n%s", got, want)
	}
}

func TestParseRefusesInvalidYAML(t *testing.T) {
	if _, err := openapi.Parse([]byte("paths: [")); err == nil {
		t.Fatal("Parse accepted invalid YAML")
	}
}
