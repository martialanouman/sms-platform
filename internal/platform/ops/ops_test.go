package ops_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/martialanouman/sms-platform/internal/platform/metrics"
	"github.com/martialanouman/sms-platform/internal/platform/ops"
)

func serve(t *testing.T, g prometheus.Gatherer, checks ...ops.Check) *ops.Server {
	t.Helper()
	s, err := ops.New("127.0.0.1:0", time.Second, g, checks...)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	return s
}

func get(t *testing.T, s *ops.Server, path string) (int, string) {
	t.Helper()
	resp, err := http.Get("http://" + s.Addr() + path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(body)
}

func ok(context.Context) error { return nil }

func TestNewRefusesBadChecks(t *testing.T) {
	for name, checks := range map[string][]ops.Check{
		"empty name": {{Name: "", Probe: ok}},
		"duplicate":  {{Name: "postgres", Probe: ok}, {Name: "postgres", Probe: ok}},
	} {
		if _, err := ops.New("127.0.0.1:0", time.Second, prometheus.NewRegistry(), checks...); err == nil {
			t.Errorf("%s: New accepted the checks", name)
		}
	}
	if _, err := ops.New("256.0.0.1:0", time.Second, prometheus.NewRegistry()); err == nil {
		t.Error("New listened on an invalid address")
	}
}

func TestHealthz(t *testing.T) {
	s := serve(t, prometheus.NewRegistry(), ops.Check{Name: "postgres", Probe: func(context.Context) error { return errors.New("down") }})
	if code, _ := get(t, s, "/healthz"); code != http.StatusOK {
		t.Errorf("/healthz = %d, want 200 whatever the dependencies", code)
	}
}

func TestReadyz(t *testing.T) {
	if code, _ := get(t, serve(t, prometheus.NewRegistry(), ops.Check{Name: "kafka", Probe: ok}), "/readyz"); code != http.StatusOK {
		t.Errorf("all probes up: /readyz = %d", code)
	}
	s := serve(t, prometheus.NewRegistry(),
		ops.Check{Name: "kafka", Probe: ok},
		ops.Check{Name: "postgres", Probe: func(context.Context) error { return errors.New("dial host=db") }},
	)
	code, body := get(t, s, "/readyz")
	if code != http.StatusServiceUnavailable || body != "not ready: postgres\n" {
		t.Errorf("one probe down: /readyz = %d %q, want 503 naming postgres only, without its error", code, body)
	}
}

func TestReadyzBoundsAndParallelisesTheProbes(t *testing.T) {
	slow := func(ctx context.Context) error {
		select {
		case <-time.After(300 * time.Millisecond):
		case <-ctx.Done():
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 2*time.Second {
			return errors.New("no 2s deadline")
		}
		return nil
	}
	s := serve(t, prometheus.NewRegistry(), ops.Check{Name: "a", Probe: slow}, ops.Check{Name: "b", Probe: slow})
	start := time.Now()
	code, body := get(t, s, "/readyz")
	if code != http.StatusOK {
		t.Errorf("/readyz = %d %q", code, body)
	}
	if elapsed := time.Since(start); elapsed > 550*time.Millisecond {
		t.Errorf("two 300ms probes took %s: they ran one after the other", elapsed)
	}
}

func TestReadyzIsDrainingWithoutProbing(t *testing.T) {
	var probed atomic.Bool
	s := serve(t, prometheus.NewRegistry(), ops.Check{Name: "kafka", Probe: func(context.Context) error { probed.Store(true); return nil }})
	s.Drain()
	if code, body := get(t, s, "/readyz"); code != http.StatusServiceUnavailable || !strings.Contains(body, "draining") {
		t.Errorf("/readyz = %d %q, want 503 draining", code, body)
	}
	if probed.Load() {
		t.Error("a draining server still probed its dependencies")
	}
}

func TestMetricsServesTheGuardedRegistry(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "sms_accepted_total", Help: "h"}, []string{"route_id"})
	metrics.Guard(reg).MustRegister(c)
	c.WithLabelValues("r1").Inc()
	if code, body := get(t, serve(t, reg), "/metrics"); code != http.StatusOK || !strings.Contains(body, `sms_accepted_total{route_id="r1"} 1`) {
		t.Errorf("/metrics = %d %q", code, body)
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	s, err := ops.New("127.0.0.1:0", time.Second, prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	if code, _ := get(t, s, "/healthz"); code != http.StatusOK {
		t.Fatalf("/healthz = %d", code)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	if _, err := http.Get("http://" + s.Addr() + "/healthz"); err == nil {
		t.Error("the server still answers after Run returned")
	}
}

func TestRunFinishesTheRequestsInFlight(t *testing.T) {
	started := make(chan struct{})
	s, err := ops.New("127.0.0.1:0", time.Second, prometheus.NewRegistry(), ops.Check{Name: "slow", Probe: func(context.Context) error {
		close(started)
		time.Sleep(300 * time.Millisecond)
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	codes := make(chan int, 1)
	go func() {
		resp, err := http.Get("http://" + s.Addr() + "/readyz")
		if err != nil {
			codes <- 0
			return
		}
		_ = resp.Body.Close()
		codes <- resp.StatusCode
	}()
	<-started
	cancel()
	if err := <-done; err != nil {
		t.Errorf("Run = %v, want a graceful stop", err)
	}
	if code := <-codes; code != http.StatusOK {
		t.Errorf("the request in flight got %d", code)
	}
}
