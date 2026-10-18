// Package schema reads the declarations that sit beside the database migrations.
package schema

import (
	"bytes"
	"fmt"
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
