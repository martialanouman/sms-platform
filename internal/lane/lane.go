// Package lane is the single runtime of the Kafka consumers, under the contract of contracts/topics.yaml
// (docs/design/04-contrats-de-topics-et-lanes.md §4).
package lane

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/martialanouman/sms-platform/internal/catalog"
	perrors "github.com/martialanouman/sms-platform/internal/platform/errors"
)

// Kind is what a record became.
type Kind int

// The outcomes of a record, docs/design/04-contrats-de-topics-et-lanes.md §4.1.
const (
	KindOk Kind = iota
	KindRefuse
	KindTransient
	KindPoison
	KindDegraded
)

// Outcome is the verdict of a handler on one record.
type Outcome struct {
	kind   Kind
	dep    string
	reason string
	err    error
}

// Ok is a record fully handled.
func Ok() Outcome { return Outcome{kind: KindOk} }

// Refuse is a definitive business refusal the handler has already published.
func Refuse(_ perrors.Code) Outcome { return Outcome{kind: KindRefuse} }

// Transient is a failure of dep; the runtime applies the mode the catalog declares for it.
func Transient(dep string, err error) Outcome {
	return Outcome{kind: KindTransient, dep: dep, err: err}
}

// Poison is a record that cannot be handled, sent to the dead-letter under reason, a short code.
func Poison(reason string) Outcome { return Outcome{kind: KindPoison, reason: reason} }

// Degraded is a record handled without dep, a fail_open dependency.
func Degraded(dep string) Outcome { return Outcome{kind: KindDegraded, dep: dep} }

// Kind returns what the record became.
func (o Outcome) Kind() Kind { return o.kind }

// Dependency returns the dependency of a transient or degraded outcome.
func (o Outcome) Dependency() string { return o.dep }

// Reason returns the dead-letter reason of a poison.
func (o Outcome) Reason() string { return o.reason }

// Err returns the failure of a transient outcome.
func (o Outcome) Err() error { return o.err }

// The common headers, docs/spec/09-contrats.md §9.1.
const (
	HeaderOriginTS    = "origin_ts"
	HeaderTraceID     = "trace_id"
	HeaderReplayCount = "replay_count"
	HeaderReplayedAt  = "replayed_at"
)

// Header is one raw Kafka header.
type Header struct {
	Key   string
	Value []byte
}

// Headers are the common headers of a record.
type Headers struct {
	OriginTS    time.Time
	TraceID     string
	ReplayCount int
	ReplayedAt  time.Time
}

// ParseHeaders reads the common headers, refusing a record without a readable origin_ts.
func ParseHeaders(raw []Header) (Headers, error) {
	var h Headers
	var origin bool
	for _, r := range raw {
		v := string(r.Value)
		var err error
		switch r.Key {
		case HeaderOriginTS:
			h.OriginTS, err = time.Parse(time.RFC3339Nano, v)
			origin = err == nil && !h.OriginTS.IsZero()
		case HeaderTraceID:
			h.TraceID = v
		case HeaderReplayCount:
			h.ReplayCount, err = strconv.Atoi(v)
		case HeaderReplayedAt:
			h.ReplayedAt, err = time.Parse(time.RFC3339Nano, v)
		}
		if err != nil {
			return Headers{}, fmt.Errorf("lane: header %s unreadable", r.Key)
		}
	}
	if !origin {
		return Headers{}, errors.New("lane: no origin_ts")
	}
	return h, nil
}

// Raw renders h as Kafka headers, omitting the empty ones.
func (h Headers) Raw() []Header {
	raw := []Header{{Key: HeaderOriginTS, Value: []byte(h.OriginTS.UTC().Format(time.RFC3339Nano))}}
	if h.TraceID != "" {
		raw = append(raw, Header{Key: HeaderTraceID, Value: []byte(h.TraceID)})
	}
	if h.ReplayCount != 0 {
		raw = append(raw, Header{Key: HeaderReplayCount, Value: []byte(strconv.Itoa(h.ReplayCount))})
	}
	if !h.ReplayedAt.IsZero() {
		raw = append(raw, Header{Key: HeaderReplayedAt, Value: []byte(h.ReplayedAt.UTC().Format(time.RFC3339Nano))})
	}
	return raw
}

// Record is one consumed record.
type Record struct {
	Topic     string
	Partition int32
	Offset    int64
	Key       []byte
	Value     []byte
	Headers   Headers
	Raw       []Header
}

var categoryMaxAge = map[string]time.Duration{
	"otp":           10 * time.Minute,
	"transactional": 24 * time.Hour,
	"marketing":     72 * time.Hour,
}

// Policy is the contract of one consumer group, as this runtime serves it.
type Policy struct {
	group    catalog.Group
	maxAge   time.Duration
	category func(Record) (string, error)
}

// NewPolicy reads group from c, refusing what this runtime cannot serve yet. category names the category of a
// record whose topic does not carry it; it is required for such a topic under max_age: category.
func NewPolicy(c catalog.Catalog, group string, category func(Record) (string, error)) (Policy, error) {
	i := slices.IndexFunc(c.Groups, func(g catalog.Group) bool { return g.Name() == group })
	if i < 0 {
		return Policy{}, fmt.Errorf("lane: group %s is not in the catalog", group)
	}
	g := c.Groups[i]
	switch g.Assignment {
	case "", "group":
	case "broadcast":
		return Policy{}, fmt.Errorf("lane: group %s: assignment broadcast arrives with step 022", group)
	default:
		return Policy{}, fmt.Errorf("lane: group %s: assignment %s arrives with step 023a", group, g.Assignment)
	}
	for dep, d := range g.Dependencies {
		if d.Mode == "park_by_tenant" {
			return Policy{}, fmt.Errorf("lane: group %s: dependency %s in park_by_tenant arrives with step 029c", group, dep)
		}
	}
	p := Policy{group: g}
	switch g.MaxAge {
	case "":
	case "category":
		if _, ok := categoryMaxAge[suffix(g.Topic)]; !ok {
			if category == nil {
				return Policy{}, fmt.Errorf("lane: group %s: max_age category on a topic without a category", group)
			}
			p.category = category
		}
	default:
		d, err := time.ParseDuration(g.MaxAge)
		if err != nil {
			return Policy{}, fmt.Errorf("lane: group %s: %w", group, err)
		}
		p.maxAge = d
	}
	return p, nil
}

func suffix(topic string) string { return topic[strings.LastIndex(topic, ".")+1:] }

// Name returns the consumer group id.
func (p Policy) Name() string { return p.group.Name() }

// Topic returns the consumed topic.
func (p Policy) Topic() string { return p.group.Topic }

// DeadLetter returns the dead-letter topic of the group.
func (p Policy) DeadLetter() string { return p.group.DeadLetter }

// StartsFromEnd reports whether a group with no committed offset begins at the end of its topic.
func (p Policy) StartsFromEnd() bool { return p.group.Start == "latest" }

// Park returns the park topic of a park_global dep.
func (p Policy) Park(dep string) string { return p.group.Dependencies[dep].Park }

// MaxInFlight returns the ceiling of records in flight per tenant of a per_tenant group, 0 otherwise.
func (p Policy) MaxInFlight() int { return p.group.MaxInFlight }

// Mode returns the declared mode of dep, empty when the group does not declare it.
func (p Policy) Mode(dep string) string { return p.group.Dependencies[dep].Mode }

// Expired reports whether rec is older at now than its limit: the max_age of the group, lowered by lower when
// lower is positive and smaller.
func (p Policy) Expired(rec Record, now time.Time, lower time.Duration) (bool, error) {
	limit := p.maxAge
	if p.group.MaxAge == "category" {
		cat := suffix(rec.Topic)
		if p.category != nil {
			var err error
			if cat, err = p.category(rec); err != nil {
				return false, err
			}
		}
		var ok bool
		if limit, ok = categoryMaxAge[cat]; !ok {
			return false, fmt.Errorf("lane: unknown category %q", cat)
		}
	}
	if lower > 0 && (limit == 0 || lower < limit) {
		limit = lower
	}
	return limit > 0 && now.Sub(rec.Headers.OriginTS) > limit, nil
}
