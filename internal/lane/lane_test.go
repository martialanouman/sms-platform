package lane_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/martialanouman/sms-platform/internal/catalog"
	"github.com/martialanouman/sms-platform/internal/lane"
)

func fixture(t *testing.T, groups string) catalog.Catalog {
	t.Helper()
	c, err := catalog.Parse([]byte(`
topics:
  mt.inbound.otp: {key: message_id, order: none, duplicate_unit: m, payload: sms.v1.A, body: clear_bounded, partitions: 4, retention: 96h, producers: [rest-api-svc]}
  mt.reroute-park: {key: message_id, order: none, duplicate_unit: m, payload: sms.v1.A, body: clear_bounded, partitions: 4, retention: 96h, producers: [connector-pool-svc]}
  mt.routed.otp: {key: manual, order: per_partition, duplicate_unit: s, payload: sms.v1.A, body: clear_bounded, partitions: 4, retention: 96h, producers: [router-svc]}
  dlr.routed: {key: account_id, order: per_key, duplicate_unit: e, payload: sms.v1.A, body: none, partitions: 4, retention: 96h, producers: [message-state-svc]}
groups:
` + groups))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func one(topic, deps, extra string) string {
	return "  - {topic: " + topic + ", service: s, tenant: account_id, dependencies: {kafka: {mode: blocking}" + deps +
		"}, concurrency: per_partition, " + extra + "dead_letter: s." + topic + ".dead-letter, start: earliest}\n"
}

func TestPolicyRefusesWhatThisStepCannotServe(t *testing.T) {
	cases := map[string]struct{ group, want string }{
		"broadcast":      {one("dlr.routed", "", "assignment: broadcast, "), "022"},
		"manual":         {one("mt.routed.otp", "", "assignment: manual, "), "023a"},
		"park_by_tenant": {one("dlr.routed", ", endpoint: {mode: park_by_tenant}", ""), "008b"},
		"per_tenant": {strings.Replace(one("dlr.routed", "", "max_in_flight: 8, "),
			"per_partition", "per_tenant", 1), "008b"},
		"category without a category topic": {one("mt.reroute-park", "", "max_age: category, "), "category"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := fixture(t, tc.group)
			if errs := c.Validate(); len(errs) > 0 {
				t.Fatal(errs)
			}
			_, err := lane.NewPolicy(c, c.Groups[0].Name(), nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewPolicy = %v, want an error naming %q", err, tc.want)
			}
		})
	}
}

func TestPolicyRefusesAnUndeclaredGroup(t *testing.T) {
	c := fixture(t, one("dlr.routed", "", ""))
	if _, err := lane.NewPolicy(c, "s.mt.inbound.otp", nil); err == nil {
		t.Fatal("NewPolicy accepted a group the catalog does not declare")
	}
}

func TestPolicyModes(t *testing.T) {
	c := fixture(t, one("dlr.routed", ", cancel: {mode: fail_open, justification: j}", ""))
	p, err := lane.NewPolicy(c, "s.dlr.routed", nil)
	if err != nil {
		t.Fatal(err)
	}
	for dep, want := range map[string]string{"kafka": "blocking", "cancel": "fail_open", "redis": ""} {
		if got := p.Mode(dep); got != want {
			t.Errorf("Mode(%q) = %q, want %q", dep, got, want)
		}
	}
	if p.Topic() != "dlr.routed" || p.DeadLetter() != "s.dlr.routed.dead-letter" || p.Name() != "s.dlr.routed" {
		t.Errorf("Topic, DeadLetter, Name = %q, %q, %q", p.Topic(), p.DeadLetter(), p.Name())
	}
}

func TestExpiry(t *testing.T) {
	origin := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	rec := func(topic string, age time.Duration) (lane.Record, time.Time) {
		return lane.Record{Topic: topic, Headers: lane.Headers{OriginTS: origin}}, origin.Add(age)
	}
	byTopic := func(r lane.Record) (string, error) { return "marketing", nil }
	cases := map[string]struct {
		topic, extra string
		category     func(lane.Record) (string, error)
		lower        time.Duration
		age          time.Duration
		want         bool
	}{
		"otp suffix, under 10 min":       {"mt.inbound.otp", "max_age: category, ", nil, 0, 10 * time.Minute, false},
		"otp suffix, over 10 min":        {"mt.inbound.otp", "max_age: category, ", nil, 0, 10*time.Minute + 1, true},
		"category from the lane":         {"mt.reroute-park", "max_age: category, ", byTopic, 0, 72 * time.Hour, false},
		"category from the lane, over":   {"mt.reroute-park", "max_age: category, ", byTopic, 0, 72*time.Hour + 1, true},
		"fixed duration":                 {"dlr.routed", "max_age: 24h, ", nil, 0, 24*time.Hour + 1, true},
		"no max_age never expires":       {"dlr.routed", "", nil, 0, 1000 * time.Hour, false},
		"a lower limit applies":          {"dlr.routed", "max_age: 24h, ", nil, time.Hour, time.Hour + 1, true},
		"a higher limit is ignored":      {"dlr.routed", "max_age: 24h, ", nil, 48 * time.Hour, 24*time.Hour + 1, true},
		"a limit without max_age counts": {"dlr.routed", "", nil, time.Hour, time.Hour + 1, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := fixture(t, one(tc.topic, "", tc.extra))
			p, err := lane.NewPolicy(c, c.Groups[0].Name(), tc.category)
			if err != nil {
				t.Fatal(err)
			}
			r, now := rec(tc.topic, tc.age)
			got, err := p.Expired(r, now, tc.lower)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("Expired = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHeadersRoundTripAndRefuseAMissingOrigin(t *testing.T) {
	h := lane.Headers{
		OriginTS:    time.Date(2026, 10, 19, 12, 0, 0, 123456789, time.UTC),
		TraceID:     "4bf92f3577b34da6a3ce929d0e0e4736",
		ReplayCount: 2,
		ReplayedAt:  time.Date(2026, 10, 19, 13, 0, 0, 0, time.UTC),
	}
	got, err := lane.ParseHeaders(append(h.Raw(), lane.Header{Key: "other", Value: []byte("x")}))
	if err != nil {
		t.Fatal(err)
	}
	if got != h {
		t.Errorf("ParseHeaders(Raw()) = %+v, want %+v", got, h)
	}
	for name, raw := range map[string][]lane.Header{
		"absent":     {{Key: "trace_id", Value: []byte("t")}},
		"unreadable": {{Key: "origin_ts", Value: []byte("yesterday")}},
		"zero":       {{Key: "origin_ts", Value: []byte(time.Time{}.Format(time.RFC3339Nano))}},
	} {
		if _, err := lane.ParseHeaders(raw); err == nil {
			t.Errorf("%s origin_ts: ParseHeaders accepted it", name)
		}
	}
	if _, err := lane.ParseHeaders([]lane.Header{{Key: "origin_ts", Value: []byte(h.OriginTS.Format(time.RFC3339Nano))},
		{Key: "replay_count", Value: []byte("two")}}); err == nil {
		t.Error("an unreadable replay_count was accepted")
	}
}

func TestExpiryRefusesAnUnknownCategory(t *testing.T) {
	c := fixture(t, one("mt.reroute-park", "", "max_age: category, "))
	p, err := lane.NewPolicy(c, c.Groups[0].Name(), func(lane.Record) (string, error) { return "promo", nil })
	if err != nil {
		t.Fatal(err)
	}
	r := lane.Record{Topic: "mt.reroute-park", Headers: lane.Headers{OriginTS: time.Now()}}
	if _, err := p.Expired(r, time.Now(), 0); err == nil {
		t.Fatal("Expired accepted an unknown category")
	}
	p, err = lane.NewPolicy(c, c.Groups[0].Name(), func(lane.Record) (string, error) { return "", errors.New("unreadable") })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Expired(r, time.Now(), 0); err == nil {
		t.Fatal("Expired hid the failure to read the category")
	}
}
