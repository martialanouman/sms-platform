package supervisor_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/martialanouman/sms-platform/internal/platform/supervisor"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

type journal struct {
	mu     sync.Mutex
	events []string
	at     map[string]time.Time
}

func (j *journal) note(e string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.events = append(j.events, e)
	if j.at == nil {
		j.at = map[string]time.Time{}
	}
	j.at[e] = time.Now()
}

func (j *journal) read() ([]string, map[string]time.Time) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return slices.Clone(j.events), j.at
}

func until(j *journal, name string) func(context.Context) error {
	return func(ctx context.Context) error {
		<-ctx.Done()
		j.note("stop " + name)
		return nil
	}
}

func TestRunDrainsInReverseOrderAfterTheHooksAndTheDelay(t *testing.T) {
	var j journal
	o := supervisor.Ordered{DrainDelay: 50 * time.Millisecond, DrainBudget: time.Second}
	for _, name := range []string{"ops", "kafka", "listener"} {
		o.Add(name, until(&j, name))
	}
	o.OnDrain(func() { j.note("hook 1") })
	o.OnDrain(func() { j.note("hook 2") })
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	if err := o.Run(ctx, quiet); err != nil {
		t.Fatal(err)
	}
	events, at := j.read()
	if want := []string{"hook 1", "hook 2", "stop listener", "stop kafka", "stop ops"}; !slices.Equal(events, want) {
		t.Errorf("events = %v, want %v", events, want)
	}
	if gap := at["stop listener"].Sub(at["hook 2"]); gap < 50*time.Millisecond {
		t.Errorf("the first component stopped %s after the hooks, before DrainDelay", gap)
	}
}

func TestAFailedComponentDrainsTheOthers(t *testing.T) {
	var j journal
	o := supervisor.Ordered{DrainBudget: time.Second}
	o.Add("ops", until(&j, "ops"))
	o.Add("consumer", func(context.Context) error { return errors.New("broker gone") })
	o.Add("listener", until(&j, "listener"))
	o.OnDrain(func() { j.note("hook") })
	err := o.Run(context.Background(), quiet)
	if err == nil || !strings.Contains(err.Error(), "consumer: broker gone") {
		t.Errorf("Run = %v, want the consumer's error", err)
	}
	if events, _ := j.read(); !slices.Equal(events, []string{"hook", "stop listener", "stop ops"}) {
		t.Errorf("events = %v", events)
	}
}

func TestAComponentStoppingEarlyIsAFailure(t *testing.T) {
	var j journal
	o := supervisor.Ordered{DrainBudget: time.Second}
	o.Add("ops", until(&j, "ops"))
	o.Add("consumer", func(context.Context) error { return nil })
	err := o.Run(context.Background(), quiet)
	if err == nil || !strings.Contains(err.Error(), "consumer: stopped before drain") {
		t.Errorf("Run = %v", err)
	}
	if events, _ := j.read(); !slices.Equal(events, []string{"stop ops"}) {
		t.Errorf("events = %v", events)
	}
}

func TestTheBudgetNamesTheStuckComponentAndCancelsTheRest(t *testing.T) {
	var j journal
	release := make(chan struct{})
	defer close(release)
	o := supervisor.Ordered{DrainBudget: 100 * time.Millisecond}
	o.Add("ops", until(&j, "ops"))
	o.Add("stuck too", func(context.Context) error { <-release; return nil })
	o.Add("stuck", func(context.Context) error { <-release; return nil })
	o.Add("listener", until(&j, "listener"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	err := o.Run(ctx, quiet)
	if !errors.Is(err, supervisor.ErrDrainBudgetExceeded) || !strings.Contains(err.Error(), ": stuck still running") {
		t.Errorf("Run = %v, want the budget exceeded on stuck", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("Run took %s past a 100ms budget", elapsed)
	}
	deadline := time.Now().Add(time.Second)
	for {
		if events, _ := j.read(); slices.Contains(events, "stop ops") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("ops was not cancelled after the budget ran out")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestErrorsWhileStopping(t *testing.T) {
	for name, c := range map[string]struct {
		err  error
		want string
	}{
		"failure":   {errors.New("flush failed"), "exporter: flush failed"},
		"cancelled": {context.Canceled, ""},
	} {
		t.Run(name, func(t *testing.T) {
			o := supervisor.Ordered{DrainBudget: time.Second}
			o.Add("exporter", func(ctx context.Context) error { <-ctx.Done(); return c.err })
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err := o.Run(ctx, quiet)
			if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
				t.Errorf("Run = %v, want %q", err, c.want)
			}
		})
	}
}
