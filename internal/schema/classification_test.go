package schema_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/martialanouman/sms-platform/internal/schema"
)

func TestParseClassification(t *testing.T) {
	got, err := schema.ParseClassification([]byte(`
customers: {audit: diff}
exact_routes: {audit: event, reason: import de masse}
audit_log: {audit: exempt, reason: le journal lui-même}
`))
	if err != nil {
		t.Fatal(err)
	}
	want := schema.Classification{
		"customers":    {Audit: schema.Diff},
		"exact_routes": {Audit: schema.Event, Reason: "import de masse"},
		"audit_log":    {Audit: schema.Exempt, Reason: "le journal lui-même"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseClassificationRefuses(t *testing.T) {
	for name, tc := range map[string]struct{ yaml, want string }{
		"unknown class":    {`t: {audit: maybe}`, `"t": unknown audit class "maybe"`},
		"missing class":    {`t: {reason: x}`, `"t": unknown audit class ""`},
		"event no reason":  {`t: {audit: event}`, `"t": event needs a reason`},
		"exempt no reason": {`t: {audit: exempt, reason: "  "}`, `"t": exempt needs a reason`},
		"unknown field":    {`t: {audit: diff, why: x}`, `field why not found`},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := schema.ParseClassification([]byte(tc.yaml))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
