package metrics

import (
	"reflect"
	"testing"
)

func TestLabels(t *testing.T) {
	for desc, want := range map[string][]string{
		`Desc{fqName: "x", help: "y, constLabels: {to=\"1\"}", unit: "", constLabels: {a="1,b=\"2\"",c="3"}, variableLabels: {d,c(e)}}`: {"a", "c", "d", "e"},
		`Desc{fqName: "x", help: "", unit: "", constLabels: {a="1"}, variableLabels: {}}`:                                               {"a"},
	} {
		if got, err := labels(desc); err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("labels(%s) = %v, %v, want %v", desc, got, err, want)
		}
	}
}

func TestLabelsRefusesAnUnknownRendering(t *testing.T) {
	for _, desc := range []string{
		`Desc{fqName: "x", variableLabels: {a}}`,
		`Desc{fqName: "_a="1", variableLabels: {b}}`,
		`Desc{fqName: "x"}`,
		`Desc{fqName: "x", help: "", unit: "", constLabels: {}, variable: {b}}`,
		`Desc{fqName: "x", help: "", unit: "", constLabels: {}, variableLabels: {b}`,
		`Desc{fqName: "x", help: "", unit: ", constLabels: {}, variableLabels: {b}}`,
		`Desc{fqName: "x", help: "", unit: "", constLabels: {a="1"`,
		`Desc{fqName: "x", constLabels: {a="1"}}`,
		`Desc{fqName: "x", constLabels: {a}, variableLabels: {b}}`,
		`Desc{fqName: "x", constLabels: {a=1}, variableLabels: {b}}`,
		`Desc{fqName: "x", constLabels: {}, variableLabels: {b}`,
	} {
		if got, err := labels(desc); err == nil {
			t.Errorf("labels(%s) = %v, want an error", desc, got)
		}
	}
}
