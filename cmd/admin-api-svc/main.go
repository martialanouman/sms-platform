// Command admin-api-svc serves the Admin API to the dashboard, under the scope each operation declares and with the
// audit of every mutation (spec 07 §7.3 to §7.5).
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/martialanouman/sms-platform/internal/adminapi"
	adminapihuma "github.com/martialanouman/sms-platform/internal/adminapi/huma"
	"github.com/martialanouman/sms-platform/internal/adminapi/jwks"
	"github.com/martialanouman/sms-platform/internal/adminapi/postgres"
	"github.com/martialanouman/sms-platform/internal/platform/config"
	"github.com/martialanouman/sms-platform/internal/platform/metrics"
	"github.com/martialanouman/sms-platform/internal/platform/observability"
	"github.com/martialanouman/sms-platform/internal/platform/ops"
	"github.com/martialanouman/sms-platform/internal/platform/pgdial"
	"github.com/martialanouman/sms-platform/internal/platform/supervisor"
)

const service = "admin-api-svc"

func main() {
	infra, err := config.Load()
	log := observability.NewLogger(os.Stdout, service, infra.LogLevel)
	if err == nil {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
		err = run(ctx, infra, log, prometheus.DefaultRegisterer, prometheus.DefaultGatherer)
		stop()
	}
	if err != nil {
		log.Error("admin-api-svc failed", "error", err)
		os.Exit(1)
	}
}

func validate(infra config.Infra) error {
	switch {
	case infra.InternalAddr == "":
		return errors.New("INTERNAL_ADDR is empty")
	case !slices.Equal(slices.Sorted(slices.Values(infra.TLSAllowedClients)), adminapihuma.Callers):
		return fmt.Errorf("TLS_ALLOWED_CLIENTS %v is not the callers of the Admin API %v", infra.TLSAllowedClients, adminapihuma.Callers)
	case !strings.HasPrefix(infra.JWKSURL, "https://"):
		return fmt.Errorf("JWKS_URL %q is not https://", infra.JWKSURL)
	case infra.PostgresURL == "":
		return errors.New("POSTGRES_URL is empty")
	}
	return nil
}

func run(ctx context.Context, infra config.Infra, log *slog.Logger, registry prometheus.Registerer, gatherer prometheus.Gatherer) error {
	slog.SetDefault(log)
	slog.SetLogLoggerLevel(slog.LevelWarn)
	if err := validate(infra); err != nil {
		return err
	}
	issuer, err := config.Origin(infra.PublicURL)
	if err != nil {
		return err
	}
	serverTLS, err := infra.TLS.Server(infra.TLSAllowedClients, []string{"http/1.1"})
	if err != nil {
		return err
	}
	clientTLS, err := infra.TLS.Client()
	if err != nil {
		return err
	}
	uncommitted := prometheus.NewCounter(prometheus.CounterOpts{Name: "admin_api_uncommitted_calls_total",
		Help: "Admin calls answered 503 because their transaction or their audit line did not commit."})
	reloadFailures := prometheus.NewCounter(prometheus.CounterOpts{Name: "admin_api_jwks_reload_failures_total",
		Help: "Reloads of the dashboard JWKS that brought no key set."})
	for _, c := range []prometheus.Collector{uncommitted, reloadFailures} {
		if err := metrics.Guard(registry).Register(c); err != nil {
			return err
		}
	}
	pool, err := pgxpool.New(ctx, infra.PostgresURL)
	if err != nil {
		return errors.New("POSTGRES_URL is unreadable")
	}
	defer pool.Close()
	if err := pgdial.Retry(ctx, log, pool.Ping); err != nil {
		return err
	}
	keys := &jwks.Keys{URL: infra.JWKSURL, Client: &http.Client{Transport: &http.Transport{TLSClientConfig: clientTLS}},
		Now: time.Now, Log: log, Failures: reloadFailures}
	handler, _ := adminapihuma.New(adminapihuma.Config{Store: postgres.New(pool),
		Verifier: adminapi.Verifier{Keys: keys, Issuer: issuer, Now: time.Now}, Uncommitted: uncommitted})
	opsServer, err := ops.New(infra.OpsAddr, infra.ShutdownTimeout, gatherer)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", infra.InternalAddr)
	if err != nil {
		return err
	}
	sup := supervisor.Ordered{DrainDelay: infra.DrainDelay, DrainBudget: infra.DrainBudget}
	sup.Add("ops", opsServer.Run)
	// debt: #167
	sup.Add("internal", serve(&http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		IdleTimeout: 60 * time.Second}, tls.NewListener(ln, serverTLS), infra.ShutdownTimeout))
	sup.OnDrain(opsServer.Drain)
	return sup.Run(ctx, log)
}

func serve(s *http.Server, ln net.Listener, shutdown time.Duration) func(context.Context) error {
	return func(ctx context.Context) error {
		shut := make(chan struct{})
		go func() {
			defer close(shut)
			<-ctx.Done()
			stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdown)
			defer cancel()
			_ = s.Shutdown(stop)
		}()
		if err := s.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		<-shut
		return nil
	}
}
