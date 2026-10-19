// Package schema reads the declarations that sit beside the database migrations.
package schema

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Audit is how a table is audited (design 05 §4.3).
type Audit string

// The audit classes of a table.
const (
	Diff   Audit = "diff"
	Event  Audit = "event"
	Exempt Audit = "exempt"
)

// Table is the audit class of one table and, unless it is diffed, why.
type Table struct {
	Audit  Audit  `yaml:"audit"`
	Reason string `yaml:"reason"`
}

// Classification maps every table of a database to its audit class.
type Classification map[string]Table

// ParseClassification decodes a classification and refuses an unknown class or a missing reason.
func ParseClassification(data []byte) (Classification, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var c Classification
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("classification: %w", err)
	}
	for name, t := range c {
		switch t.Audit {
		case Diff:
		case Event, Exempt:
			if strings.TrimSpace(t.Reason) == "" {
				return nil, fmt.Errorf("classification: %q: %s needs a reason", name, t.Audit)
			}
		default:
			return nil, fmt.Errorf("classification: %q: unknown audit class %q", name, t.Audit)
		}
	}
	return c, nil
}

// Observed is what the database says of one of its tables.
type Observed struct {
	Captured      bool
	HasPrimaryKey bool
}

// Check returns, sorted, every disagreement between the classification and the tables of the database.
func (c Classification) Check(tables map[string]Observed) []string {
	var problems []string
	for name, o := range tables {
		t, ok := c[name]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("%q: not classified", name))
		case t.Audit == Diff && !o.Captured:
			problems = append(problems, fmt.Sprintf("%q: diff without audit_capture", name))
		case t.Audit == Diff && !o.HasPrimaryKey:
			problems = append(problems, fmt.Sprintf("%q: diff without a primary key", name))
		case t.Audit != Diff && o.Captured:
			problems = append(problems, fmt.Sprintf("%q: %s but carries audit_capture", name, t.Audit))
		}
	}
	for name := range c {
		if _, ok := tables[name]; !ok {
			problems = append(problems, fmt.Sprintf("%q: classified but absent", name))
		}
	}
	slices.Sort(problems)
	return problems
}
