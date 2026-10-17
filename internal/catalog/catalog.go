// Package catalog reads and checks the Kafka topic catalog, contracts/topics.yaml.
package catalog

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

const (
	maxClearRetention = 96 * time.Hour
	deadLetterSuffix  = ".dead-letter"
)

// Catalog is every topic and every consumer group of the platform.
type Catalog struct {
	Topics map[string]Topic `yaml:"topics"`
	Groups []Group          `yaml:"groups"`
}

// Topic is the contract of one Kafka topic.
type Topic struct {
	Key           string        `yaml:"key"`
	Order         string        `yaml:"order"`
	DuplicateUnit string        `yaml:"duplicate_unit"`
	Payload       string        `yaml:"payload"`
	Body          string        `yaml:"body"`
	Partitions    int           `yaml:"partitions"`
	Retention     time.Duration `yaml:"retention"`
	Compacted     bool          `yaml:"compacted"`
	Producers     []string      `yaml:"producers"`
}

// Dependency is how a lane reacts when one of its dependencies fails.
type Dependency struct {
	Mode          string `yaml:"mode"`
	Justification string `yaml:"justification"`
}

// Group is the contract of one consumer of one topic.
type Group struct {
	Topic              string                `yaml:"topic"`
	Service            string                `yaml:"service"`
	Assignment         string                `yaml:"assignment"`
	Tenant             string                `yaml:"tenant"`
	Dependencies       map[string]Dependency `yaml:"dependencies"`
	Concurrency        string                `yaml:"concurrency"`
	MaxInFlight        int                   `yaml:"max_in_flight"`
	MaxAge             string                `yaml:"max_age"`
	DeadLetter         string                `yaml:"dead_letter"`
	Start              string                `yaml:"start"`
	StartJustification string                `yaml:"start_justification"`
}

// Name is the group's identity: its service and its topic.
func (g Group) Name() string { return g.Service + "." + g.Topic }

// Parse decodes a catalog, refusing unknown fields and duplicate keys.
func Parse(data []byte) (Catalog, error) {
	var c Catalog
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return Catalog{}, fmt.Errorf("catalog: %w", err)
	}
	return c, nil
}

// Validate returns every breach of the catalog's static guards.
func (c Catalog) Validate() []error {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	for _, name := range sortedKeys(c.Topics) {
		t := c.Topics[name]
		if !slices.Contains([]string{"per_key", "per_partition", "none"}, t.Order) {
			fail("topic %s: order %q, want per_key, per_partition or none", name, t.Order)
		}
		if (t.Key == "manual") != (t.Order == "per_partition") {
			fail("topic %s: a manual key goes with per_partition order, and only with it", name)
		}
		if !slices.Contains([]string{"none", "clear_bounded"}, t.Body) {
			fail("topic %s: body %q, want none or clear_bounded", name, t.Body)
		}
		if t.Partitions < 1 {
			fail("topic %s: partitions must be positive", name)
		}
		if len(t.Producers) == 0 {
			fail("topic %s: no producers", name)
		}
		if t.Retention <= 0 && !t.Compacted {
			fail("topic %s: retention required unless compacted", name)
		}
		// debt: #80
		if t.Body == "clear_bounded" && (t.Compacted || t.Retention > maxClearRetention) {
			fail("topic %s: a clear_bounded body is kept at most 96h, got %s", name, t.Retention)
		}
	}

	seen := map[string]bool{}
	for _, g := range c.Groups {
		name := g.Name()
		if seen[name] {
			fail("group %s: duplicate", name)
		}
		seen[name] = true
		if _, ok := c.Topics[g.Topic]; !ok {
			fail("group %s: topic %s is not declared", name, g.Topic)
		}
		if g.DeadLetter != name+deadLetterSuffix {
			fail("group %s: dead_letter %q, want %q", name, g.DeadLetter, name+deadLetterSuffix)
		}
		if !slices.Contains([]string{"", "group", "broadcast", "manual"}, g.Assignment) {
			fail("group %s: assignment %q, want group, broadcast or manual", name, g.Assignment)
		}
		switch g.Start {
		case "earliest":
		case "latest":
			if g.StartJustification == "" {
				fail("group %s: start latest needs a start_justification", name)
			}
		default:
			fail("group %s: start %q, want earliest or latest", name, g.Start)
		}
		switch g.Concurrency {
		case "per_partition":
			if g.MaxInFlight != 0 {
				fail("group %s: max_in_flight only applies to per_tenant", name)
			}
		case "per_tenant":
			if g.MaxInFlight < 1 {
				fail("group %s: per_tenant needs a positive max_in_flight", name)
			}
		default:
			fail("group %s: concurrency %q, want per_partition or per_tenant", name, g.Concurrency)
		}
		if g.MaxAge != "" && g.MaxAge != "category" {
			if d, err := time.ParseDuration(g.MaxAge); err != nil || d <= 0 {
				fail("group %s: max_age %q, want category or a positive duration", name, g.MaxAge)
			}
		}
		for _, dep := range sortedKeys(g.Dependencies) {
			d := g.Dependencies[dep]
			switch d.Mode {
			case "blocking", "park_by_tenant", "park_global":
			case "fail_open":
				if d.Justification == "" {
					fail("group %s: fail_open dependency %s needs a justification", name, dep)
				}
			default:
				fail("group %s: dependency %s mode %q, want blocking, park_by_tenant, park_global or fail_open", name, dep, d.Mode)
			}
		}
	}
	return errs
}

// Frozen returns an error for every per_key topic of base whose partition count head changes.
func Frozen(base, head Catalog) []error {
	var errs []error
	for _, name := range sortedKeys(base.Topics) {
		b := base.Topics[name]
		h, ok := head.Topics[name]
		if b.Order == "per_key" && ok && h.Partitions != b.Partitions {
			errs = append(errs, fmt.Errorf("topic %s: per_key partitions are frozen at %d, got %d", name, b.Partitions, h.Partitions))
		}
	}
	return errs
}

// DeadLetters returns the dead-letter topic of every group, derived from its source topic.
func (c Catalog) DeadLetters() map[string]Topic {
	dl := map[string]Topic{}
	for _, g := range c.Groups {
		src := c.Topics[g.Topic]
		dl[g.DeadLetter] = Topic{
			Key:           src.Key,
			Order:         "none",
			DuplicateUnit: "record",
			Payload:       src.Payload,
			Body:          src.Body,
			Partitions:    8,
			Retention:     maxClearRetention,
			Producers:     []string{g.Service},
		}
	}
	return dl
}

// Markdown renders the catalog as the generated documentation page.
func (c Catalog) Markdown() []byte {
	var b strings.Builder
	b.WriteString("# Catalogue des topics\n\nPage générée depuis `contracts/topics.yaml` par `go generate` : ne pas l'éditer.\n\n")
	b.WriteString("## Topics\n\n| Topic | Clé | Ordre | Corps | Partitions | Rétention | Payload | Producteurs | Doublon |\n|---|---|---|---|---|---|---|---|---|\n")
	for _, name := range sortedKeys(c.Topics) {
		t := c.Topics[name]
		fmt.Fprintf(&b, "| `%s` | `%s` | %s | %s | %d | %s | `%s` | %s | %s |\n",
			name, t.Key, t.Order, t.Body, t.Partitions, retention(t), t.Payload, strings.Join(t.Producers, ", "), t.DuplicateUnit)
	}
	b.WriteString("\n## Groupes\n\n| Groupe | Assignation | Tenant | Dépendances | Concurrence | Âge maximal | Départ |\n|---|---|---|---|---|---|---|\n")
	for _, g := range c.Groups {
		var deps []string
		for _, name := range sortedKeys(g.Dependencies) {
			deps = append(deps, name+" : "+g.Dependencies[name].Mode)
		}
		assignment := g.Assignment
		if assignment == "" {
			assignment = "group"
		}
		concurrency := g.Concurrency
		if g.MaxInFlight > 0 {
			concurrency = fmt.Sprintf("%s (%d en vol)", concurrency, g.MaxInFlight)
		}
		maxAge := g.MaxAge
		if maxAge == "" {
			maxAge = "—"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s | %s | %s |\n",
			g.Name(), assignment, g.Tenant, strings.Join(deps, ", "), concurrency, maxAge, g.Start)
	}
	dl := c.DeadLetters()
	b.WriteString("\n## Dead-letters\n\nDérivées de chaque groupe : clé et corps de la source, ordre `none`.\n\n| Topic | Corps | Partitions | Rétention |\n|---|---|---|---|\n")
	for _, name := range sortedKeys(dl) {
		t := dl[name]
		fmt.Fprintf(&b, "| `%s` | %s | %d | %s |\n", name, t.Body, t.Partitions, retention(t))
	}
	return []byte(b.String())
}

func retention(t Topic) string {
	if t.Compacted {
		return "compacté"
	}
	return fmt.Sprintf("%dh", int(t.Retention.Hours()))
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
