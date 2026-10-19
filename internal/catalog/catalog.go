// Package catalog reads and checks the Kafka topic catalog, contracts/topics.yaml.
package catalog

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
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
	Partitions    int32         `yaml:"partitions"`
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
	Lane               string                `yaml:"lane"`
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

func (g Group) name() string {
	if g.Lane != "" {
		return g.Service + "." + g.Lane + "." + g.Topic
	}
	return g.Service + "." + g.Topic
}

// Parse decodes a catalog, refusing unknown fields and duplicate keys.
func Parse(data []byte) (Catalog, error) {
	var c Catalog
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return Catalog{}, fmt.Errorf("catalog: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Catalog{}, errors.New("catalog: a single YAML document is expected")
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
		if t.Key == "" || t.Payload == "" || t.DuplicateUnit == "" {
			fail("topic %s: key, payload and duplicate_unit are required", name)
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
		name := g.name()
		if seen[name] {
			fail("group %s: duplicate", name)
		}
		seen[name] = true
		topic, ok := c.Topics[g.Topic]
		if !ok {
			fail("group %s: topic %s is not declared", name, g.Topic)
		}
		if g.Service == "" {
			fail("group %s: service is required", name)
		}
		if g.Assignment == "manual" && topic.Key != "manual" {
			fail("group %s: a manual assignment needs a topic with a manual key", name)
		}
		if !slices.Contains([]string{"account_id", "customer_id", "connector_account_id", "none"}, g.Tenant) {
			fail("group %s: tenant %q, want account_id, customer_id, connector_account_id or none", name, g.Tenant)
		}
		if len(g.Dependencies) == 0 {
			fail("group %s: no dependencies", name)
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
			if g.Tenant == "none" {
				fail("group %s: per_tenant needs a tenant", name)
			}
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
			case "park_by_tenant":
				if g.Tenant == "none" {
					fail("group %s: park_by_tenant dependency %s needs a tenant", name, dep)
				}
			case "blocking", "park_global":
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

// Frozen returns an error for every per_key topic of base whose key, order or partition count head changes.
func Frozen(base, head Catalog) []error {
	var errs []error
	for _, name := range sortedKeys(base.Topics) {
		b := base.Topics[name]
		h, ok := head.Topics[name]
		if b.Order == "per_key" && ok && (h.Key != b.Key || h.Order != b.Order || h.Partitions != b.Partitions) {
			errs = append(errs, fmt.Errorf("topic %s: per_key key, order and partitions are frozen at %s, %s, %d", name, b.Key, b.Order, b.Partitions))
		}
	}
	return errs
}

// MissingPayloads returns an error for every topic whose payload message known does not recognise.
func (c Catalog) MissingPayloads(known func(string) bool) []error {
	var errs []error
	for _, name := range sortedKeys(c.Topics) {
		if p := c.Topics[name].Payload; !known(p) {
			errs = append(errs, fmt.Errorf("topic %s: payload %s does not exist", name, p))
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
			Key:        src.Key,
			Order:      "none",
			Body:       src.Body,
			Partitions: 8,
			Retention:  maxClearRetention,
		}
	}
	return dl
}

// ACL is one Kafka access control entry, in the names of the Kafka protocol.
type ACL struct {
	Principal  string
	Resource   string
	Name       string
	Pattern    string
	Operation  string
	Permission string
	Host       string
}

// ACLs is every right the catalog grants, sorted and without duplicates.
func (c Catalog) ACLs() []ACL {
	var acls []ACL
	grant := func(service, resource, name, operation string) {
		acls = append(acls, ACL{Principal: "User:CN=" + service, Resource: resource, Name: name, Pattern: "LITERAL", Operation: operation, Permission: "ALLOW", Host: "*"})
	}
	for name, t := range c.Topics {
		for _, p := range t.Producers {
			grant(p, "TOPIC", name, "WRITE")
		}
	}
	for _, g := range c.Groups {
		grant(g.Service, "TOPIC", g.Topic, "READ")
		grant(g.Service, "GROUP", g.name(), "READ")
		grant(g.Service, "TOPIC", g.DeadLetter, "WRITE")
	}
	slices.SortFunc(acls, func(a, b ACL) int {
		return strings.Compare(a.Principal+" "+a.Resource+" "+a.Name+" "+a.Operation, b.Principal+" "+b.Resource+" "+b.Name+" "+b.Operation)
	})
	return slices.Compact(acls)
}

// Configs is the topic configuration the catalog owns; everything else is left to the broker and its operator.
func (t Topic) Configs() map[string]string {
	if t.Compacted {
		return map[string]string{"cleanup.policy": "compact"}
	}
	return map[string]string{"cleanup.policy": "delete", "retention.ms": strconv.FormatInt(t.Retention.Milliseconds(), 10)}
}

// Live is a topic as the cluster holds it.
type Live struct {
	Partitions int
	Configs    map[string]string
}

// State is what the cluster holds: its topics, internal ones excluded, and every ACL.
type State struct {
	Topics map[string]Live
	ACLs   []ACL
}

// Plan is what brings a cluster to the catalog; ACLs are added before others are removed.
type Plan struct {
	Create    map[string]Topic
	Extend    map[string]int
	Configure map[string]map[string]string
	Add       []ACL
	Remove    []ACL
	Unmanaged []string
}

// Empty reports whether the plan writes nothing; an unmanaged topic is only reported.
func (p Plan) Empty() bool {
	return len(p.Create)+len(p.Extend)+len(p.Configure)+len(p.Add)+len(p.Remove) == 0
}

// Plan compares the cluster with the catalog and its dead-letters; a refusal returns every fault and no plan.
func (c Catalog) Plan(s State) (Plan, error) {
	var p Plan
	var errs []error
	want := maps.Clone(c.Topics)
	maps.Copy(want, c.DeadLetters())
	for _, name := range sortedKeys(want) {
		t := want[name]
		live, ok := s.Topics[name]
		switch {
		case !ok:
			if p.Create == nil {
				p.Create = map[string]Topic{}
			}
			p.Create[name] = t
			continue
		case t.Order == "per_key" && live.Partitions != int(t.Partitions):
			errs = append(errs, fmt.Errorf("topic %s: %d partitions, per_key frozen at %d", name, live.Partitions, t.Partitions))
		case live.Partitions > int(t.Partitions) && t.Order != "per_partition":
			errs = append(errs, fmt.Errorf("topic %s: %d partitions, the catalog says %d and Kafka cannot remove any", name, live.Partitions, t.Partitions))
		case live.Partitions < int(t.Partitions):
			if p.Extend == nil {
				p.Extend = map[string]int{}
			}
			p.Extend[name] = int(t.Partitions)
		}
		for k, v := range t.Configs() {
			if live.Configs[k] != v {
				if p.Configure == nil {
					p.Configure = map[string]map[string]string{}
				}
				if p.Configure[name] == nil {
					p.Configure[name] = map[string]string{}
				}
				p.Configure[name][k] = v
			}
		}
	}
	if len(errs) > 0 {
		return Plan{}, errors.Join(errs...)
	}
	for _, name := range sortedKeys(s.Topics) {
		if _, ok := want[name]; !ok {
			p.Unmanaged = append(p.Unmanaged, name)
		}
	}
	acls := c.ACLs()
	for _, a := range acls {
		if !slices.Contains(s.ACLs, a) {
			p.Add = append(p.Add, a)
		}
	}
	for _, a := range s.ACLs {
		if !slices.Contains(acls, a) {
			p.Remove = append(p.Remove, a)
		}
	}
	return p, nil
}

// Markdown renders the catalog as the generated documentation page.
func (c Catalog) Markdown() []byte {
	var b strings.Builder
	b.WriteString("# Catalogue des topics\n\nPage générée depuis `contracts/topics.yaml` par `go generate` : ne pas l'éditer.\n\n")
	b.WriteString("## Topics\n\n| Topic | Clé | Ordre | Corps | Partitions | Rétention | Payload | Producteurs | Doublon |\n|---|---|---|---|---|---|---|---|---|\n")
	for _, name := range sortedKeys(c.Topics) {
		t := c.Topics[name]
		partitions := strconv.Itoa(int(t.Partitions))
		if t.Order == "per_partition" {
			partitions += " (initial)"
		}
		fmt.Fprintf(&b, "| `%s` | `%s` | %s | %s | %s | %s | `%s` | %s | %s |\n",
			name, t.Key, t.Order, t.Body, partitions, retention(t), t.Payload, strings.Join(t.Producers, ", "), t.DuplicateUnit)
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
			g.name(), assignment, g.Tenant, strings.Join(deps, ", "), concurrency, maxAge, g.Start)
	}
	dl := c.DeadLetters()
	b.WriteString("\n## Dead-letters\n\nDérivées de chaque groupe : clé et corps de la source, ordre `none`.\n\n| Topic | Corps | Partitions | Rétention |\n|---|---|---|---|\n")
	for _, name := range sortedKeys(dl) {
		t := dl[name]
		fmt.Fprintf(&b, "| `%s` | %s | %d | %s |\n", name, t.Body, t.Partitions, retention(t))
	}
	b.WriteString("\n## ACL\n\nDérivées des producteurs et des groupes, appliquées telles quelles par `catalog-apply` : toute autre ACL du cluster est retirée.\n\n| Principal | Ressource | Nom | Opération |\n|---|---|---|---|\n")
	for _, a := range c.ACLs() {
		fmt.Fprintf(&b, "| `%s` | %s | `%s` | %s |\n", a.Principal, a.Resource, a.Name, a.Operation)
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
