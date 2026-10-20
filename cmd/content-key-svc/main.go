// Command content-key-svc seals and opens the replayed configuration secrets under the OpenBao Transit key, for the
// callers its method map names (ADR-0011, ADR-0016).
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"

	"github.com/martialanouman/sms-platform/internal/contentkey"
	contentkeygrpc "github.com/martialanouman/sms-platform/internal/contentkey/grpc"
	"github.com/martialanouman/sms-platform/internal/contentkey/openbao"
	"github.com/martialanouman/sms-platform/internal/platform/config"
	"github.com/martialanouman/sms-platform/internal/platform/observability"
	"github.com/martialanouman/sms-platform/internal/platform/ops"
	"github.com/martialanouman/sms-platform/internal/platform/supervisor"
)

const (
	service      = "content-key-svc"
	projectedJWT = "/var/run/secrets/openbao/token"
)

func main() {
	infra, err := config.Load()
	log := observability.NewLogger(os.Stdout, service, infra.LogLevel)
	if err == nil {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
		err = run(ctx, infra, log)
		stop()
	}
	if err != nil {
		log.Error("content-key-svc failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, infra config.Infra, log *slog.Logger) error {
	callers := slices.Sorted(maps.Keys(contentkeygrpc.Callers))
	switch {
	case infra.GRPCAddr == "":
		return errors.New("GRPC_ADDR is empty")
	case !strings.HasPrefix(infra.OpenBaoAddr, "https://"):
		return fmt.Errorf("OPENBAO_ADDR %q is not https://", infra.OpenBaoAddr)
	case !slices.Equal(slices.Sorted(slices.Values(infra.TLSAllowedClients)), callers):
		return fmt.Errorf("TLS_ALLOWED_CLIENTS %v is not the callers of the method map %v", infra.TLSAllowedClients, callers)
	}
	serverTLS, err := infra.TLS.Server(infra.TLSAllowedClients, []string{"h2"})
	if err != nil {
		return err
	}
	ca, err := os.ReadFile(infra.TLS.CA)
	if err != nil {
		return err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return fmt.Errorf("%s holds no certificate", infra.TLS.CA)
	}
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}}}

	grpclog.SetLoggerV2(observability.GRPCLogger(log))
	s := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS)), grpc.UnaryInterceptor(contentkeygrpc.Authorize))
	contentkeygrpc.Register(s, contentkey.ConfigSecrets{KMS: openbao.New(infra.OpenBaoAddr, client, service, projectedJWT)})
	opsServer, err := ops.New(infra.OpsAddr, infra.ShutdownTimeout, prometheus.DefaultGatherer)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", infra.GRPCAddr)
	if err != nil {
		return err
	}
	sup := supervisor.Ordered{DrainDelay: infra.DrainDelay, DrainBudget: infra.DrainBudget}
	sup.Add("ops", opsServer.Run)
	sup.Add("grpc", serve(s, ln, infra.ShutdownTimeout))
	sup.OnDrain(opsServer.Drain)
	return sup.Run(ctx, log)
}

func serve(s *grpc.Server, ln net.Listener, shutdown time.Duration) func(context.Context) error {
	return func(ctx context.Context) error {
		go func() {
			<-ctx.Done()
			timer := time.AfterFunc(shutdown, s.Stop)
			defer timer.Stop()
			s.GracefulStop()
		}()
		return s.Serve(ln)
	}
}
