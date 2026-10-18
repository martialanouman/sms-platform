// Package ops serves the ops port of a service: /healthz, /readyz and /metrics, in clear, bounded by the
// NetworkPolicy to the kubelet and Prometheus (docs/design/06-confiance-interne.md §4).
package ops

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const probeTimeout = 2 * time.Second

// Check is a vital dependency: /readyz answers 503 while its probe fails.
type Check struct {
	Name  string
	Probe func(context.Context) error
}

// Server is the ops server of a service.
type Server struct {
	ln       net.Listener
	srv      *http.Server
	shutdown time.Duration
	draining atomic.Bool
	checks   []Check
}

// New listens on addr; shutdown bounds the stop of Run.
func New(addr string, shutdown time.Duration, g prometheus.Gatherer, checks ...Check) (*Server, error) {
	names := map[string]bool{}
	for _, c := range checks {
		if c.Name == "" || names[c.Name] {
			return nil, fmt.Errorf("ops: check name %q is empty or repeated", c.Name)
		}
		names[c.Name] = true
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("ops: %w", err)
	}
	s := &Server{ln: ln, shutdown: shutdown, checks: checks}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.Handle("GET /metrics", promhttp.HandlerFor(g, promhttp.HandlerOpts{}))
	s.srv = &http.Server{Handler: mux, ReadHeaderTimeout: probeTimeout}
	return s, nil
}

// Addr is the address the server listens on.
func (s *Server) Addr() string {
	return s.ln.Addr().String()
}

// Drain turns /readyz to 503 for good; it is a drain hook of the supervisor.
func (s *Server) Drain() {
	s.draining.Store(true)
}

// Run serves until ctx ends, then stops within the shutdown timeout.
func (s *Server) Run(ctx context.Context) error {
	served := make(chan error, 1)
	go func() { served <- s.srv.Serve(s.ln) }()
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	stop, cancel := context.WithTimeout(context.Background(), s.shutdown)
	defer cancel()
	return s.srv.Shutdown(stop)
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	if s.draining.Load() {
		http.Error(w, "draining", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), probeTimeout)
	defer cancel()
	failed := make([]string, len(s.checks))
	var wg sync.WaitGroup
	for i, c := range s.checks {
		wg.Go(func() {
			if c.Probe(ctx) != nil {
				failed[i] = c.Name
			}
		})
	}
	wg.Wait()
	if down := strings.Join(slices.DeleteFunc(failed, func(n string) bool { return n == "" }), ", "); down != "" {
		http.Error(w, "not ready: "+down, http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ok\n"))
}
