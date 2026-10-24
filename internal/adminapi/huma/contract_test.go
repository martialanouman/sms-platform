package huma_test

import (
	"context"
	"io/fs"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/martialanouman/sms-platform/contracts"
	"github.com/martialanouman/sms-platform/internal/adminapi"
	"github.com/martialanouman/sms-platform/internal/adminapi/adminapitest"
	adminhuma "github.com/martialanouman/sms-platform/internal/adminapi/huma"
	"github.com/martialanouman/sms-platform/internal/platform/scopes"
)

const scheme = "OperatorBearer"

// complete turns true in step 032, when every operation of the contract is served.
const complete = false

type served struct {
	method, path string
	op           *openapi3.Operation
}

func contract(t *testing.T) *openapi3.T {
	t.Helper()
	raw, err := fs.ReadFile(contracts.OpenAPI(), "openapi-admin.yaml")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := openapi3.NewLoader().LoadFromData(raw)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func servedOperations(t *testing.T) []served {
	t.Helper()
	_, api := adminhuma.New(adminhuma.Config{})
	raw, err := api.OpenAPI().MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	doc, err := openapi3.NewLoader().LoadFromData(raw)
	if err != nil {
		t.Fatal(err)
	}
	var out []served
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			out = append(out, served{method, path, op})
		}
	}
	if len(out) == 0 {
		t.Fatal("no operation served")
	}
	return out
}

func codes(op *openapi3.Operation) []string {
	return slices.Sorted(maps.Keys(op.Responses.Map()))
}

func parameters(op *openapi3.Operation) []string {
	var names []string
	for _, p := range op.Parameters {
		names = append(names, p.Value.In+":"+p.Value.Name)
	}
	slices.Sort(names)
	return names
}

func parametersOf(item *openapi3.PathItem, op *openapi3.Operation) []string {
	all := &openapi3.Operation{Parameters: append(slices.Clone(item.Parameters), op.Parameters...)}
	return parameters(all)
}

func security(op *openapi3.Operation) []map[string][]string {
	var out []map[string][]string
	if op.Security == nil {
		return out
	}
	for _, r := range *op.Security {
		out = append(out, map[string][]string(r))
	}
	return out
}

func TestTheServedContractIsASubsetOfTheFile(t *testing.T) {
	file := contract(t)
	for _, s := range servedOperations(t) {
		item := file.Paths.Find(s.path)
		if item == nil || item.GetOperation(s.method) == nil {
			t.Errorf("%s %s (%s) is not in the contract", s.method, s.path, s.op.OperationID)
			continue
		}
		want := item.GetOperation(s.method)
		if s.op.OperationID != want.OperationID {
			t.Errorf("%s %s: operationId %q, contract %q", s.method, s.path, s.op.OperationID, want.OperationID)
		}
		if got, w := security(s.op), security(want); !reflect.DeepEqual(got, w) {
			t.Errorf("%s: security %v, contract %v", want.OperationID, got, w)
		}
		if got, w := codes(s.op), codes(want); !slices.Equal(got, w) {
			t.Errorf("%s: responses %v, contract %v", want.OperationID, got, w)
		}
		if got, w := parameters(s.op), parametersOf(item, want); !slices.Equal(got, w) {
			t.Errorf("%s: parameters %v, contract %v", want.OperationID, got, w)
		}
	}
	if complete {
		var missing []string
		ids := map[string]bool{}
		for _, s := range servedOperations(t) {
			ids[s.op.OperationID] = true
		}
		for _, item := range file.Paths.Map() {
			for _, op := range item.Operations() {
				if !ids[op.OperationID] {
					missing = append(missing, op.OperationID)
				}
			}
		}
		if len(missing) > 0 {
			t.Errorf("not served: %v", missing)
		}
	}
}

func TestTheCountedPagesAreServedWithTheirTotal(t *testing.T) {
	want := map[string]bool{"list-customer-groups": true, "list-customers": true, "list-group-customers": true, "list-accounts": true}
	for _, s := range servedOperations(t) {
		if !want[s.op.OperationID] {
			continue
		}
		delete(want, s.op.OperationID)
		ok := s.op.Responses.Status(http.StatusOK)
		body := ok.Value.Content.Get("application/json").Schema.Value
		if !slices.Contains(body.Required, "total") || body.Properties["total"] == nil {
			t.Errorf("%s: required %v", s.op.OperationID, body.Required)
		}
	}
	if len(want) != 0 {
		t.Errorf("not served: %v", want)
	}
}

type memory struct {
	mu      sync.Mutex
	entries []adminapi.Entry
}

func (m *memory) Tx(_ context.Context, _, _ string, fn func(adminapi.Tx) error) error {
	return fn(memoryTx{m: m})
}

func (m *memory) Record(_ context.Context, e adminapi.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, e)
	return nil
}

type memoryTx struct {
	adminapi.Tx
	m *memory
}

func (t memoryTx) Record(ctx context.Context, e adminapi.Entry) error { return t.m.Record(ctx, e) }

func (memoryTx) AuditLog(context.Context, adminapi.Filter, *adminapi.Key, int) ([]adminapi.Entry, error) {
	return nil, nil
}

func (memoryTx) Changes(context.Context, string, *adminapi.Key, int) ([]adminapi.Change, error) {
	return nil, nil
}

func (memoryTx) Requested(context.Context, string) (bool, error) { return true, nil }

func TestEachOperationCalledWithEveryScopeButItsOwnIsForbidden(t *testing.T) {
	k := adminapitest.NewKeys(t, "v1")
	h, _ := adminhuma.New(adminhuma.Config{Store: &memory{}, Verifier: k.Verifier()})
	for _, s := range servedOperations(t) {
		if len(security(s.op)) != 1 || len(security(s.op)[0][scheme]) != 1 {
			t.Errorf("%s declares %v, not exactly one scope", s.op.OperationID, security(s.op))
			continue
		}
		own := security(s.op)[0][scheme][0]
		others := slices.DeleteFunc(slices.Clone(scopes.All), func(scope string) bool { return scope == own })
		path := s.path
		for _, p := range s.op.Parameters {
			if p.Value.In == "path" {
				path = strings.ReplaceAll(path, "{"+p.Value.Name+"}", "x")
			}
		}

		req := httptest.NewRequest(s.method, path, nil)
		req.Header.Set("Authorization", "Bearer "+k.Token(t, "v1", strings.Join(others, " ")))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"forbidden_scope"`) {
			t.Errorf("%s with every scope but %s: %d %s", s.op.OperationID, own, rec.Code, rec.Body)
		}
	}
}

func TestNothingOutsideTheContractIsRouted(t *testing.T) {
	h, _ := adminhuma.New(adminhuma.Config{Store: &memory{}, Verifier: adminapitest.NewKeys(t, "v1").Verifier()})
	for _, path := range []string{"/openapi.json", "/openapi.yaml", "/docs", "/schemas/Error.json", "/admin/nope"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d", path, rec.Code)
		}
	}
}
