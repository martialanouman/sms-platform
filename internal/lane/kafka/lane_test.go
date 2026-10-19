package kafka_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"maps"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	smsv1 "github.com/martialanouman/sms-platform/internal/gen/sms/v1"
	"github.com/martialanouman/sms-platform/internal/lane"
	"github.com/martialanouman/sms-platform/internal/lane/kafka"
	perrors "github.com/martialanouman/sms-platform/internal/platform/errors"
)

const secret = "BODY-7f3a"

func send(t *testing.T, topic string, recs ...*kgo.Record) {
	t.Helper()
	for _, r := range recs {
		r.Topic = topic
	}
	if err := client(t).ProduceSync(context.Background(), recs...).FirstErr(); err != nil {
		t.Fatal(err)
	}
}

func rec(value string, origin time.Time) *kgo.Record {
	return &kgo.Record{Key: []byte("k"), Value: []byte(value), Headers: toKafka(lane.Headers{OriginTS: origin, TraceID: "tr", ReplayCount: 3}.Raw())}
}

func toKafka(raw []lane.Header) []kgo.RecordHeader {
	var out []kgo.RecordHeader
	for _, h := range raw {
		out = append(out, kgo.RecordHeader{Key: h.Key, Value: h.Value})
	}
	return out
}

// seen collects what the handlers saw, in order.
type seen struct {
	mu      sync.Mutex
	handled []string
	expired []string
	more    chan struct{}
}

func newSeen() *seen { return &seen{more: make(chan struct{}, 1000)} }

func (s *seen) add(to *[]string, v string) {
	s.mu.Lock()
	*to = append(*to, v)
	s.mu.Unlock()
	s.more <- struct{}{}
}

func (s *seen) wait(t *testing.T, n int) {
	t.Helper()
	for i := range n {
		select {
		case <-s.more:
		case <-time.After(30 * time.Second):
			t.Fatalf("saw %d of %d records", i, n)
		}
	}
}

func (s *seen) snapshot() (handled, expired []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.handled...), append([]string(nil), s.expired...)
}

type running struct {
	cancel context.CancelFunc
	done   chan error
	log    *bytes.Buffer
}

func start(t *testing.T, w world, h kafka.Handlers) running {
	t.Helper()
	log := &bytes.Buffer{}
	l, err := kafka.NewLane(w.cat, w.group, h, slog.New(slog.NewJSONHandler(&syncWriter{w: log}, nil)),
		kgo.SeedBrokers(brokers...), kgo.FetchMaxWait(100*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := running{cancel: cancel, done: make(chan error, 1), log: log}
	go func() { r.done <- l.Run(ctx); l.Close() }()
	t.Cleanup(func() {
		cancel()
		if err := <-r.done; err != nil {
			t.Error(err)
		}
	})
	return r
}

type syncWriter struct {
	mu sync.Mutex
	w  *bytes.Buffer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

func (r running) stop(t *testing.T) error {
	t.Helper()
	r.cancel()
	select {
	case err := <-r.done:
		r.done <- err
		return err
	case <-time.After(30 * time.Second):
		t.Fatal("the lane did not stop")
		return nil
	}
}

func committed(t *testing.T, w world) int64 {
	t.Helper()
	offs, err := kadm.NewClient(client(t)).FetchOffsets(context.Background(), w.group)
	if err != nil {
		t.Fatal(err)
	}
	o, ok := offs.Lookup(w.topic, 0)
	if !ok {
		return -1
	}
	return o.At
}

func TestAPoisonGoesToTheDeadLetterWithoutBlockingItsPartition(t *testing.T) {
	w := newWorld(t, "dependencies: {kafka: {mode: blocking}}")
	origin := time.Now().UTC().Truncate(time.Microsecond)
	send(t, w.topic, rec("a", origin), rec("poison", origin), rec("c", origin),
		&kgo.Record{Key: []byte("k"), Value: []byte("no-origin")})
	s := newSeen()
	r := start(t, w, kafka.Handlers{
		Handle: func(_ context.Context, rc lane.Record) lane.Outcome {
			s.add(&s.handled, string(rc.Value))
			if string(rc.Value) == "poison" {
				return lane.Poison("unreadable_payload")
			}
			return lane.Ok()
		},
		Expired: func(context.Context, lane.Record) lane.Outcome { return lane.Ok() },
	})
	s.wait(t, 3)
	dls := readAll(t, w.dl, 2)
	if err := r.stop(t); err != nil {
		t.Fatal(err)
	}
	if handled, _ := s.snapshot(); strings.Join(handled, ",") != "a,poison,c" {
		t.Errorf("handled %v, want a, poison, c", handled)
	}
	if got := committed(t, w); got != 4 {
		t.Errorf("committed offset %d, want 4", got)
	}
	var d smsv1.DeadLetter
	if err := proto.Unmarshal(dls[0].Value, &d); err != nil {
		t.Fatal(err)
	}
	if string(d.GetPayload()) != "poison" || d.GetReason() != "unreadable_payload" || d.GetTopic() != w.topic ||
		d.GetPartition() != 0 || d.GetOffset() != 1 || d.GetAt() == nil || len(d.GetHeaders()) != 3 {
		t.Errorf("dead-letter = %v", &d)
	}
	var raw []lane.Header
	for _, x := range dls[0].Headers {
		raw = append(raw, lane.Header{Key: x.Key, Value: x.Value})
	}
	h, err := lane.ParseHeaders(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !h.OriginTS.Equal(origin) || h.TraceID != "tr" || h.ReplayCount != 3 || string(dls[0].Key) != "k" {
		t.Errorf("dead-letter headers %+v and key %q, want the source's", h, dls[0].Key)
	}
	if err := proto.Unmarshal(dls[1].Value, &d); err != nil || d.GetReason() != "unreadable_headers" {
		t.Errorf("record without origin_ts: dead-letter %v, %v", &d, err)
	}
}

func TestAnExpiredRecordIsNeverHandled(t *testing.T) {
	w := newWorld(t, "dependencies: {kafka: {mode: blocking}}, max_age: category")
	now := time.Now().UTC()
	send(t, w.topic, rec("old", now.Add(-11*time.Minute)), rec("young", now), rec("lowered", now.Add(-2*time.Minute)))
	s := newSeen()
	start(t, w, kafka.Handlers{
		Handle: func(_ context.Context, rc lane.Record) lane.Outcome {
			s.add(&s.handled, string(rc.Value))
			return lane.Ok()
		},
		Expired: func(_ context.Context, rc lane.Record) lane.Outcome {
			s.add(&s.expired, string(rc.Value))
			return lane.Ok()
		},
		Limit: func(rc lane.Record) time.Duration {
			if string(rc.Value) == "lowered" {
				return time.Minute
			}
			return 0
		},
	})
	s.wait(t, 3)
	handled, expired := s.snapshot()
	if strings.Join(handled, ",") != "young" || strings.Join(expired, ",") != "old,lowered" {
		t.Errorf("handled %v, expired %v; want young, and old and lowered", handled, expired)
	}
}

func TestABlockingFailureReplaysInPlaceWithoutCommitting(t *testing.T) {
	w := newWorld(t, "dependencies: {kafka: {mode: blocking}, store: {mode: blocking}}")
	origin := time.Now().UTC()
	send(t, w.topic, rec("a", origin), rec("b", origin), rec("c", origin))
	s := newSeen()
	var mu sync.Mutex
	failures := 2
	var tries []time.Time
	release := make(chan struct{})
	r := start(t, w, kafka.Handlers{
		Handle: func(_ context.Context, rc lane.Record) lane.Outcome {
			s.add(&s.handled, string(rc.Value))
			mu.Lock()
			defer mu.Unlock()
			if string(rc.Value) == "b" {
				tries = append(tries, time.Now())
			}
			if string(rc.Value) == "b" && failures > 0 {
				failures--
				if failures == 0 {
					<-release
				}
				return lane.Transient("store", errors.New("down "+secret))
			}
			return lane.Ok()
		},
		Expired: func(context.Context, lane.Record) lane.Outcome { return lane.Ok() },
	})
	s.wait(t, 3)
	if got := committed(t, w); got != 1 {
		t.Errorf("committed offset %d while b fails, want 1", got)
	}
	close(release)
	s.wait(t, 2)
	if err := r.stop(t); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	gap := tries[2].Sub(tries[1])
	mu.Unlock()
	if gap < 800*time.Millisecond {
		t.Errorf("replayed %v after an attempt that made no progress, want a backoff of about a second", gap)
	}
	if handled, _ := s.snapshot(); strings.Join(handled, ",") != "a,b,b,b,c" {
		t.Errorf("handled %v, want a, b three times, c", handled)
	}
	if got := committed(t, w); got != 3 {
		t.Errorf("committed offset %d, want 3", got)
	}
	if strings.Contains(r.log.String(), secret) {
		t.Errorf("the log carries the text of the handler's error: %s", r.log)
	}
	if !strings.Contains(r.log.String(), `"dependency":"store","code":"internal_error"`) {
		t.Errorf("the log does not name the failing dependency and its code: %s", r.log)
	}
}

func TestADrainCommitsWhatIsDone(t *testing.T) {
	w := newWorld(t, "dependencies: {kafka: {mode: blocking}}")
	origin := time.Now().UTC()
	send(t, w.topic, rec("a", origin), rec("b", origin))
	s := newSeen()
	block := make(chan struct{})
	r := start(t, w, kafka.Handlers{
		Handle: func(ctx context.Context, rc lane.Record) lane.Outcome {
			s.add(&s.handled, string(rc.Value))
			if string(rc.Value) == "b" {
				<-block
				return lane.Transient("kafka", ctx.Err())
			}
			return lane.Ok()
		},
		Expired: func(context.Context, lane.Record) lane.Outcome { return lane.Ok() },
	})
	s.wait(t, 2)
	time.AfterFunc(100*time.Millisecond, func() { close(block) })
	if err := r.stop(t); err != nil {
		t.Fatal(err)
	}
	if got := committed(t, w); got != 1 {
		t.Errorf("committed offset %d after the drain, want 1", got)
	}
}

func TestABreachOfContractStopsTheLaneWithoutCommitting(t *testing.T) {
	cases := map[string]lane.Outcome{
		"transient on an undeclared dependency": lane.Transient("redis", errors.New(secret)),
		"degraded on a blocking dependency":     lane.Degraded("kafka"),
		"degraded on an undeclared dependency":  lane.Degraded("redis"),
	}
	for name, out := range cases {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t, "dependencies: {kafka: {mode: blocking}}")
			send(t, w.topic, rec("done", time.Now().UTC()), rec("breach", time.Now().UTC()))
			l, err := kafka.NewLane(w.cat, w.group, kafka.Handlers{
				Handle: func(_ context.Context, rc lane.Record) lane.Outcome {
					if string(rc.Value) == "done" {
						return lane.Ok()
					}
					return out
				},
				Expired: func(context.Context, lane.Record) lane.Outcome { return lane.Ok() },
			}, slog.New(slog.DiscardHandler), kgo.SeedBrokers(brokers...))
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			err = l.Run(ctx)
			if err == nil || ctx.Err() != nil {
				t.Fatalf("Run = %v, want a breach of contract before the deadline", err)
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("the error carries the handler's text: %v", err)
			}
			if got := committed(t, w); got != 1 {
				t.Errorf("committed offset %d, want 1: what was done before the breach, nothing after", got)
			}
		})
	}
}

func TestOutcomesThatMoveOn(t *testing.T) {
	w := newWorld(t, "dependencies: {kafka: {mode: blocking}, cancel: {mode: fail_open, justification: j}}")
	origin := time.Now().UTC()
	send(t, w.topic, rec("refused", origin), rec("degraded", origin))
	s := newSeen()
	r := start(t, w, kafka.Handlers{
		Handle: func(_ context.Context, rc lane.Record) lane.Outcome {
			s.add(&s.handled, string(rc.Value))
			if string(rc.Value) == "refused" {
				return lane.Refuse(perrors.Code("no_route"))
			}
			return lane.Degraded("cancel")
		},
		Expired: func(context.Context, lane.Record) lane.Outcome { return lane.Ok() },
	})
	s.wait(t, 2)
	if err := r.stop(t); err != nil {
		t.Fatal(err)
	}
	if got := committed(t, w); got != 2 {
		t.Errorf("committed offset %d, want 2", got)
	}
}

func TestNewLaneNeedsBothHandlers(t *testing.T) {
	w := newWorld(t, "dependencies: {kafka: {mode: blocking}}")
	ok := func(context.Context, lane.Record) lane.Outcome { return lane.Ok() }
	for name, h := range map[string]kafka.Handlers{"no Handle": {Expired: ok}, "no Expired": {Handle: ok}} {
		if l, err := kafka.NewLane(w.cat, w.group, h, slog.New(slog.DiscardHandler), kgo.SeedBrokers(brokers...)); err == nil {
			l.Close()
			t.Errorf("%s: NewLane accepted it", name)
		}
	}
}

func TestNewLaneRefusesWhatThePolicyRefuses(t *testing.T) {
	w := newWorld(t, "dependencies: {kafka: {mode: blocking}, billing-svc: {mode: park_global}}")
	ok := func(context.Context, lane.Record) lane.Outcome { return lane.Ok() }
	_, err := kafka.NewLane(w.cat, w.group, kafka.Handlers{Handle: ok, Expired: ok}, slog.New(slog.DiscardHandler), kgo.SeedBrokers(brokers...))
	if err == nil || !strings.Contains(err.Error(), "008b") {
		t.Fatalf("NewLane = %v, want the refusal of park_global", err)
	}
}

func TestAGroupStartingLatestSkipsWhatPrecedesIt(t *testing.T) {
	for from, wantBefore := range map[string]bool{"earliest": true, "latest": false} {
		t.Run(from, func(t *testing.T) {
			w := newWorld(t, "dependencies: {kafka: {mode: blocking}}")
			w.cat.Groups[0].Start = from
			send(t, w.topic, rec("before", time.Now().UTC()))
			s := newSeen()
			start(t, w, kafka.Handlers{
				Handle: func(_ context.Context, rc lane.Record) lane.Outcome {
					s.add(&s.handled, string(rc.Value))
					return lane.Ok()
				},
				Expired: func(context.Context, lane.Record) lane.Outcome { return lane.Ok() },
			})
			for {
				send(t, w.topic, rec("after", time.Now().UTC()))
				if handled, _ := s.snapshot(); len(handled) > 0 {
					if (handled[0] == "before") != wantBefore {
						t.Fatalf("first record handled %q", handled[0])
					}
					return
				}
				time.Sleep(200 * time.Millisecond)
			}
		})
	}
}

func TestAPoisonIsNeverCommittedBeforeItsDeadLetterIsWritten(t *testing.T) {
	w := newWorld(t, "dependencies: {kafka: {mode: blocking}}")
	adm := kadm.NewClient(client(t))
	if _, err := adm.DeleteTopics(context.Background(), w.dl); err != nil {
		t.Fatal(err)
	}
	send(t, w.topic, rec("poison", time.Now().UTC()))
	s := newSeen()
	r := start(t, w, kafka.Handlers{
		Handle: func(_ context.Context, rc lane.Record) lane.Outcome {
			s.add(&s.handled, string(rc.Value))
			return lane.Poison("unreadable_payload")
		},
		Expired: func(context.Context, lane.Record) lane.Outcome { return lane.Ok() },
	})
	s.wait(t, 2)
	if got := committed(t, w); got != -1 {
		t.Fatalf("committed offset %d before the dead-letter exists, want none", got)
	}
	if !strings.Contains(r.log.String(), `"dependency":"kafka"`) {
		t.Errorf("the failed dead-letter write is not replayed as a kafka failure: %s", r.log)
	}
	createTopics(t, 1, w.dl)
	readAll(t, w.dl, 1)
	deadline := time.Now().Add(30 * time.Second)
	for committed(t, w) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("the poison was never committed once its dead-letter was written")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestEveryOutcomeIsCountedUnderItsGroup(t *testing.T) {
	reg := prometheus.NewRegistry()
	if err := kafka.Register(reg); err != nil {
		t.Fatal(err)
	}
	w := newWorld(t, "dependencies: {kafka: {mode: blocking}, cancel: {mode: fail_open, justification: j}}, max_age: category")
	now := time.Now().UTC()
	send(t, w.topic, rec("ok", now), rec("refused", now), rec("degraded", now), rec("poison", now), rec("old", now.Add(-time.Hour)),
		rec("rotten", now.Add(-time.Hour)))
	s := newSeen()
	r := start(t, w, kafka.Handlers{
		Handle: func(_ context.Context, rc lane.Record) lane.Outcome {
			s.add(&s.handled, string(rc.Value))
			switch string(rc.Value) {
			case "refused":
				return lane.Refuse(perrors.Code("no_route"))
			case "degraded":
				return lane.Degraded("cancel")
			case "poison":
				return lane.Poison("unreadable_payload")
			}
			return lane.Ok()
		},
		Expired: func(_ context.Context, rc lane.Record) lane.Outcome {
			s.add(&s.expired, string(rc.Value))
			if string(rc.Value) == "rotten" {
				return lane.Poison("unreadable_payload")
			}
			return lane.Ok()
		},
	})
	s.wait(t, 6)
	readAll(t, w.dl, 2)
	if err := r.stop(t); err != nil {
		t.Fatal(err)
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, f := range families {
		for _, m := range f.GetMetric() {
			labels := map[string]string{}
			for _, l := range m.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}
			if f.GetName() == "lane_records_total" && labels["group"] == w.group {
				got[labels["outcome"]] = m.GetCounter().GetValue()
			}
		}
	}
	want := map[string]float64{"ok": 1, "refused": 1, "degraded": 1, "poison": 2, "expired": 1}
	if !maps.Equal(got, want) {
		t.Errorf("lane_records_total = %v, want %v", got, want)
	}
}
