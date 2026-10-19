package schema_test

import (
	"reflect"
	"testing"

	"github.com/martialanouman/sms-platform/internal/schema"
)

func TestCheckAgreesWithAClassifiedDatabase(t *testing.T) {
	c := schema.Classification{
		"customers": {Audit: schema.Diff},
		"jobs":      {Audit: schema.Exempt, Reason: "état d'exécution"},
	}
	got := c.Check(map[string]schema.Observed{
		"customers": {Captured: true, HasPrimaryKey: true},
		"jobs":      {HasPrimaryKey: true},
	})
	if len(got) != 0 {
		t.Fatalf("got %q", got)
	}
}

func TestCheckReportsEveryDisagreement(t *testing.T) {
	c := schema.Classification{
		"gone":      {Audit: schema.Diff},
		"uncaught":  {Audit: schema.Diff},
		"keyless":   {Audit: schema.Diff},
		"exact":     {Audit: schema.Event, Reason: "volume"},
		"suspended": {Audit: schema.Exempt, Reason: "journal"},
	}
	got := c.Check(map[string]schema.Observed{
		"new":       {HasPrimaryKey: true},
		"uncaught":  {HasPrimaryKey: true},
		"keyless":   {Captured: true},
		"exact":     {Captured: true, HasPrimaryKey: true},
		"suspended": {Captured: true},
	})
	want := []string{
		`"exact": event but carries audit_capture`,
		`"gone": classified but absent`,
		`"keyless": diff without a primary key`,
		`"new": not classified`,
		`"suspended": exempt but carries audit_capture`,
		`"uncaught": diff without audit_capture`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}
