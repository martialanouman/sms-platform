package kafka_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	smsv1 "github.com/martialanouman/sms-platform/internal/gen/sms/v1"
	"github.com/martialanouman/sms-platform/internal/lane"
	"github.com/martialanouman/sms-platform/internal/lane/kafka"
	perrors "github.com/martialanouman/sms-platform/internal/platform/errors"
)

func keyed(account, value string) *kgo.Record {
	r := rec(value, time.Now().UTC())
	r.Key = []byte(account)
	return r
}

func byKey(rc lane.Record) (string, error) {
	if len(rc.Key) == 0 {
		return "", errors.New("no account")
	}
	return string(rc.Key), nil
}

func expiredOk(context.Context, lane.Record) lane.Outcome { return lane.Ok() }

func waitCommitted(t *testing.T, w world, want int64) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for committed(t, w) != want {
		if time.Now().After(deadline) {
			t.Fatalf("committed offset %d, want %d", committed(t, w), want)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Decision 4 §8: 24 accounts without a receiver and one slow endpoint beside a normal account, on one partition.
func TestASlowTenantDelaysOnlyItself(t *testing.T) {
	w := newTenantWorld(t, "dependencies: {kafka: {mode: blocking}}")
	var mu sync.Mutex
	var latencies []time.Duration
	normal := make(chan struct{}, 100)
	r := start(t, w, kafka.Handlers{
		Handle: func(ctx context.Context, rc lane.Record) lane.Outcome {
			switch account := string(rc.Key); {
			case account == "slow":
				select {
				case <-time.After(300 * time.Millisecond):
				case <-ctx.Done():
				}
			case strings.HasPrefix(account, "dead"):
				return lane.Refuse(perrors.Code("no_receiver"))
			case account == "normal" && string(rc.Value) != "warm":
				mu.Lock()
				latencies = append(latencies, time.Since(rc.Headers.OriginTS))
				mu.Unlock()
				normal <- struct{}{}
			}
			return lane.Ok()
		},
		Expired: expiredOk,
		Tenant:  byKey,
	})
	send(t, w.topic, keyed("normal", "warm"))
	waitCommitted(t, w, 1)
	const rounds = 40
	for i := range rounds {
		send(t, w.topic, keyed("slow", "s"), keyed(fmt.Sprintf("dead%d", i%24), "d"), keyed("normal", "n"))
		time.Sleep(20 * time.Millisecond)
	}
	for i := range rounds {
		select {
		case <-normal:
		case <-time.After(30 * time.Second):
			t.Fatalf("normal account: %d of %d records handled", i, rounds)
		}
	}
	if err := r.stop(t); err != nil {
		t.Fatal(err)
	}
	slices.Sort(latencies)
	if p99 := latencies[len(latencies)*99/100]; p99 >= time.Second {
		t.Errorf("p99 of the normal account = %s, want < 1s", p99)
	}
}

func TestATenantHasAtMostMaxInFlightRecordsHandledAtOnce(t *testing.T) {
	w := newTenantWorld(t, "dependencies: {kafka: {mode: blocking}}")
	var now, peak atomic.Int32
	release := make(chan struct{})
	entered := make(chan struct{}, 100)
	start(t, w, kafka.Handlers{
		Handle: func(context.Context, lane.Record) lane.Outcome {
			n := now.Add(1)
			for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
			}
			entered <- struct{}{}
			<-release
			now.Add(-1)
			return lane.Ok()
		},
		Expired: expiredOk,
		Tenant:  byKey,
	})
	for range 20 {
		send(t, w.topic, keyed("a", "x"))
	}
	for i := range 8 {
		select {
		case <-entered:
		case <-time.After(30 * time.Second):
			t.Fatalf("%d of 8 records in flight", i)
		}
	}
	select {
	case <-entered:
		t.Error("a ninth record of the tenant entered while eight were in flight")
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	waitCommitted(t, w, 20)
	if p := peak.Load(); p != 8 {
		t.Errorf("peak in flight = %d, want 8", p)
	}
}

func TestACommitNeverPassesTheOldestUnfinishedRecordAndADrainKeepsIt(t *testing.T) {
	w := newTenantWorld(t, "dependencies: {kafka: {mode: blocking}}")
	send(t, w.topic, keyed("b", "b0"), keyed("held", "h"), keyed("b", "b1"), keyed("b", "b2"))
	s := newSeen()
	h := kafka.Handlers{
		Handle: func(ctx context.Context, rc lane.Record) lane.Outcome {
			s.add(&s.handled, string(rc.Value))
			if string(rc.Key) == "held" {
				<-ctx.Done()
				return lane.Transient("kafka", ctx.Err())
			}
			return lane.Ok()
		},
		Expired: expiredOk,
		Tenant:  byKey,
	}
	r := start(t, w, h)
	s.wait(t, 4)
	time.Sleep(300 * time.Millisecond)
	if got := committed(t, w); got > 1 {
		t.Fatalf("committed offset %d past the unfinished record at 1", got)
	}
	if err := r.stop(t); err != nil {
		t.Fatal(err)
	}
	if got := committed(t, w); got != 1 {
		t.Fatalf("committed offset %d after the drain, want 1: what was done, nothing past the unfinished record", got)
	}
	again := newSeen()
	h.Handle = func(_ context.Context, rc lane.Record) lane.Outcome {
		again.add(&again.handled, string(rc.Value))
		return lane.Ok()
	}
	start(t, w, h)
	again.wait(t, 3)
	waitCommitted(t, w, 4)
}

func TestABlockingFailureUnderPerTenantReplaysInPlace(t *testing.T) {
	w := newTenantWorld(t, "dependencies: {kafka: {mode: blocking}, store: {mode: blocking}}")
	send(t, w.topic, keyed("a", "fails"+secret), keyed("b", "b"))
	var healthy atomic.Bool
	var attempts atomic.Int32
	r := start(t, w, kafka.Handlers{
		Handle: func(_ context.Context, rc lane.Record) lane.Outcome {
			if string(rc.Value) == "fails"+secret {
				attempts.Add(1)
				if !healthy.Load() {
					return lane.Transient("store", errors.New("down"))
				}
			}
			return lane.Ok()
		},
		Expired: expiredOk,
		Tenant:  byKey,
	})
	deadline := time.Now().Add(30 * time.Second)
	for attempts.Load() < 2 {
		if time.Now().After(deadline) {
			t.Fatal("the failed record was not replayed")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := committed(t, w); got > 0 {
		t.Fatalf("committed offset %d past the failing record", got)
	}
	healthy.Store(true)
	waitCommitted(t, w, 2)
	if !strings.Contains(r.log.String(), `"dependency":"store"`) {
		t.Errorf("the replay is not logged under its dependency: %s", r.log)
	}
	if strings.Contains(r.log.String(), secret) {
		t.Errorf("the log carries the body (I1): %s", r.log)
	}
}

func TestARecordWithoutATenantGoesToTheDeadLetter(t *testing.T) {
	w := newTenantWorld(t, "dependencies: {kafka: {mode: blocking}}")
	send(t, w.topic, keyed("", "orphan"), keyed("a", "a"))
	start(t, w, kafka.Handlers{
		Handle:  func(context.Context, lane.Record) lane.Outcome { return lane.Ok() },
		Expired: expiredOk,
		Tenant:  byKey,
	})
	dls := readAll(t, w.dl, 1)
	var d smsv1.DeadLetter
	if err := proto.Unmarshal(dls[0].Value, &d); err != nil || d.GetReason() != "unreadable_tenant" || string(d.GetPayload()) != "orphan" {
		t.Errorf("dead-letter %v, %v, want orphan under unreadable_tenant", &d, err)
	}
	waitCommitted(t, w, 2)
}

func TestAPerTenantLaneStopsReadingOnceItsWindowIsFull(t *testing.T) {
	w := newTenantWorld(t, "dependencies: {kafka: {mode: blocking}}")
	recs := []*kgo.Record{keyed("held", "h")}
	for i := range 3000 {
		recs = append(recs, keyed(fmt.Sprintf("t%d", i%50), "x"))
	}
	send(t, w.topic, recs...)
	var handled atomic.Int32
	release := make(chan struct{})
	start(t, w, kafka.Handlers{
		Handle: func(_ context.Context, rc lane.Record) lane.Outcome {
			if string(rc.Key) == "held" {
				<-release
			}
			handled.Add(1)
			return lane.Ok()
		},
		Expired: expiredOk,
		Tenant:  byKey,
	})
	deadline := time.Now().Add(30 * time.Second)
	for handled.Load() < 1000 {
		if time.Now().After(deadline) {
			t.Fatalf("handled %d records, want the window", handled.Load())
		}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
	if n := handled.Load(); n > 1024 {
		t.Errorf("handled %d records behind an unfinished one, want at most a window of 1024", n)
	}
	close(release)
	waitCommitted(t, w, 3001)
}

func TestABreachStopsAPerTenantLaneWhileAnotherTenantReplays(t *testing.T) {
	w := newTenantWorld(t, "dependencies: {kafka: {mode: blocking}}")
	send(t, w.topic, keyed("a", "replays"), keyed("b", "breach"))
	breached := make(chan struct{})
	l, err := kafka.NewLane(w.cat, w.group, kafka.Handlers{
		Handle: func(_ context.Context, rc lane.Record) lane.Outcome {
			if string(rc.Key) == "a" {
				return lane.Transient("kafka", errors.New("down"))
			}
			close(breached)
			return lane.Degraded("kafka")
		},
		Expired: expiredOk,
		Tenant:  byKey,
	}, slog.New(slog.DiscardHandler), kgo.SeedBrokers(brokers...))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := l.Run(ctx); err == nil || ctx.Err() != nil {
		t.Fatalf("Run = %v, want the breach before the deadline", err)
	}
	<-breached
	if got := committed(t, w); got > 0 {
		t.Errorf("committed offset %d past the replaying record", got)
	}
}
