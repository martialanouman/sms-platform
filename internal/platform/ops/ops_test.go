package ops_test

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
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
	down := func(context.Context) error { return errors.New("dial host=db") }
	s := serve(t, prometheus.NewRegistry(),
		ops.Check{Name: "kafka", Probe: ok},
		ops.Check{Name: "postgres", Probe: down},
		ops.Check{Name: "redis", Probe: down},
	)
	code, body := get(t, s, "/readyz")
	if code != http.StatusServiceUnavailable || body != "not ready: postgres, redis\n" {
		t.Errorf("one probe down: /readyz = %d %q, want 503 naming postgres and redis, without their error", code, body)
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

func TestReadyzRunsTheProbesTogether(t *testing.T) {
	var barrier sync.WaitGroup
	barrier.Add(2)
	meet := func(ctx context.Context) error {
		barrier.Done()
		met := make(chan struct{})
		go func() { barrier.Wait(); close(met) }()
		select {
		case <-met:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s := serve(t, prometheus.NewRegistry(), ops.Check{Name: "a", Probe: meet}, ops.Check{Name: "b", Probe: meet})
	if code, body := get(t, s, "/readyz"); code != http.StatusOK {
		t.Errorf("/readyz = %d %q: the probes did not run together", code, body)
	}
}

func TestReadyzAnswersWithinTwoSeconds(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	ignores := func(d time.Duration) func(context.Context) error {
		return func(context.Context) error {
			select {
			case <-time.After(d):
			case <-release:
			}
			return nil
		}
	}
	s := serve(t, prometheus.NewRegistry(), ops.Check{Name: "slow", Probe: ignores(1500 * time.Millisecond)}, ops.Check{Name: "stuck", Probe: ignores(time.Hour)})
	start := time.Now()
	code, body := get(t, s, "/readyz")
	if code != http.StatusServiceUnavailable || body != "not ready: stuck\n" {
		t.Errorf("/readyz = %d %q, want 503 naming the stuck probe only", code, body)
	}
	if elapsed := time.Since(start); elapsed > 2500*time.Millisecond {
		t.Errorf("/readyz answered after %s", elapsed)
	}
}

func TestRunIsBoundedByTheShutdownTimeout(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	s, err := ops.New("127.0.0.1:0", 200*time.Millisecond, prometheus.NewRegistry(), ops.Check{Name: "slow", Probe: func(context.Context) error {
		close(started)
		<-release
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	go func() {
		if resp, err := http.Get("http://" + s.Addr() + "/readyz"); err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Run = %v, want the shutdown timeout", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run outlived its 200ms shutdown timeout")
	}
}

func TestAnIdleConnectionIsClosed(t *testing.T) {
	s := serve(t, prometheus.NewRegistry())
	conn, err := net.Dial("tcp", s.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write([]byte("GET /healthz HTTP/1.1\r\nHost: ops\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(conn)
	resp, err := http.ReadResponse(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := r.ReadByte(); !errors.Is(err, io.EOF) {
		t.Errorf("idle connection read = %v, want the server to close it", err)
	}
}

type failing struct{}

func (failing) Describe(ch chan<- *prometheus.Desc) {
	ch <- prometheus.NewDesc("pool_up", "h", nil, nil)
}

func (failing) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.NewInvalidMetric(prometheus.NewDesc("pool_up", "h", nil, nil), errors.New("dial tcp db.internal:5432: refused"))
}

func TestMetricsHidesACollectorError(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(failing{})
	if _, body := get(t, serve(t, reg), "/metrics"); strings.Contains(body, "db.internal") {
		t.Errorf("/metrics leaks a collector error: %q", body)
	}
}
