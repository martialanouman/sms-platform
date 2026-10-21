// Command dashboard serves the operations dashboard BFF: authentication, sessions and the access log (spec 08).
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/martialanouman/sms-platform/internal/dashboard"
	dashboardhttp "github.com/martialanouman/sms-platform/internal/dashboard/http"
	dashboardopenbao "github.com/martialanouman/sms-platform/internal/dashboard/openbao"
	"github.com/martialanouman/sms-platform/internal/dashboard/postgres"
	"github.com/martialanouman/sms-platform/internal/dashboard/smtp"
	"github.com/martialanouman/sms-platform/internal/platform/config"
	"github.com/martialanouman/sms-platform/internal/platform/metrics"
	"github.com/martialanouman/sms-platform/internal/platform/observability"
	platformopenbao "github.com/martialanouman/sms-platform/internal/platform/openbao"
	"github.com/martialanouman/sms-platform/internal/platform/ops"
	"github.com/martialanouman/sms-platform/internal/platform/pgdial"
	"github.com/martialanouman/sms-platform/internal/platform/supervisor"
	"github.com/martialanouman/sms-platform/web"
)

const (
	service       = "dashboard"
	projectedJWT  = "/var/run/secrets/openbao/token"
	deliveryEvery = 5 * time.Second
	purgeEvery    = time.Minute
)

func main() {
	infra, err := config.Load()
	log := observability.NewLogger(os.Stdout, service, infra.LogLevel)
	if err == nil {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
		err = run(ctx, infra, log, prometheus.DefaultRegisterer, prometheus.DefaultGatherer)
		stop()
	}
	if err != nil {
		log.Error("dashboard failed", "error", err)
		os.Exit(1)
	}
}

// debt: #141
func totpKey(infra config.Infra) (dashboard.TotpKey, error) {
	held, err := os.ReadFile(infra.TotpKeyFile)
	if err != nil {
		return dashboard.TotpKey{}, err
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(held)))
	if err != nil {
		return dashboard.TotpKey{}, errors.New("TOTP_KEY_FILE is not hexadecimal")
	}
	return dashboard.NewTotpKey(key)
}

func validate(infra config.Infra) error {
	_, smtpPort, smtpErr := net.SplitHostPort(infra.SMTPAddr)
	switch {
	case infra.HTTPAddr == "":
		return errors.New("HTTP_ADDR is empty")
	case infra.PostgresURL == "":
		return errors.New("POSTGRES_URL is empty")
	case !dashboard.ValidEmail(infra.BootstrapEmail):
		return errors.New("DASHBOARD_BOOTSTRAP_EMAIL is not an email address")
	case smtpErr != nil || smtpPort == "":
		return fmt.Errorf("SMTP_ADDR %q is not host:port", infra.SMTPAddr)
	case infra.SMTPFrom == "":
		return errors.New("SMTP_FROM is empty")
	case (infra.SMTPUsername == "") != (infra.SMTPPasswordFile == ""):
		return errors.New("SMTP_USERNAME and SMTP_PASSWORD_FILE go together")
	case infra.CookieKeyFile == "":
		return errors.New("COOKIE_KEY_FILE is empty")
	case infra.TotpKeyFile == "":
		return errors.New("TOTP_KEY_FILE is empty")
	case infra.TLS.CA == "":
		return errors.New("TLS_CA_FILE is empty, and the relay is verified by it")
	case infra.InternalAddr == "":
		return errors.New("INTERNAL_ADDR is empty, and the JWKS is served there only")
	case !strings.HasPrefix(infra.OpenBaoAddr, "https://"):
		return fmt.Errorf("OPENBAO_ADDR %q is not https://", infra.OpenBaoAddr)
	case !strings.HasPrefix(infra.AdminAPIURL, "https://"):
		return fmt.Errorf("ADMIN_API_URL %q is not https://", infra.AdminAPIURL)
	case infra.TLS.Cert == "":
		return errors.New("TLS_CERT_FILE is empty")
	case infra.TLS.Key == "":
		return errors.New("TLS_KEY_FILE is empty")
	case !slices.Equal(slices.Sorted(slices.Values(infra.TLSAllowedClients)), dashboardhttp.JWKSCallers):
		return fmt.Errorf("TLS_ALLOWED_CLIENTS %v is not the callers of the JWKS %v", infra.TLSAllowedClients, dashboardhttp.JWKSCallers)
	}
	return nil
}

func run(ctx context.Context, infra config.Infra, log *slog.Logger, registry prometheus.Registerer, gatherer prometheus.Gatherer) error {
	if err := validate(infra); err != nil {
		return err
	}
	expected, err := config.Origin(infra.PublicURL)
	if err != nil {
		return err
	}
	passkeys, err := dashboard.NewPasskeys(expected)
	if err != nil {
		return fmt.Errorf("DASHBOARD_PUBLIC_URL: %w", err)
	}
	secret, err := os.ReadFile(infra.CookieKeyFile)
	if err != nil {
		return err
	}
	keys, err := dashboard.NewKeys(secret)
	if err != nil {
		return err
	}
	totp, err := totpKey(infra)
	if err != nil {
		return err
	}
	mailer := smtp.Mailer{Addr: infra.SMTPAddr, From: infra.SMTPFrom, Username: infra.SMTPUsername, Roots: x509.NewCertPool()}
	if infra.SMTPPasswordFile != "" {
		password, err := os.ReadFile(infra.SMTPPasswordFile)
		if err != nil {
			return err
		}
		mailer.Password = strings.TrimSpace(string(password))
	}
	ca, err := os.ReadFile(infra.TLS.CA)
	if err != nil {
		return err
	}
	if !mailer.Roots.AppendCertsFromPEM(ca) {
		return errors.New("the internal CA holds no certificate")
	}
	internalTLS, err := infra.TLS.Server(infra.TLSAllowedClients, []string{"http/1.1"})
	if err != nil {
		return err
	}
	baoClient, err := platformopenbao.HTTPClient(ca)
	if err != nil {
		return err
	}
	undeliverable := prometheus.NewGauge(prometheus.GaugeOpts{Name: "dashboard_access_links_undeliverable",
		Help: "Access links of active accounts that ran out of attempts without being sent."})
	if err := metrics.Guard(registry).Register(undeliverable); err != nil {
		return err
	}

	pool, err := pgxpool.New(ctx, infra.PostgresURL)
	if err != nil {
		return errors.New("POSTGRES_URL is unreadable")
	}
	defer pool.Close()
	svc := dashboard.Service{Store: postgres.New(pool), Hasher: dashboard.NewHasher(), Keys: keys, Totp: totp, Passkeys: passkeys, Mailer: mailer, PublicURL: expected,
		Signer: dashboardopenbao.New(infra.OpenBaoAddr, baoClient, service, projectedJWT)}
	if err := pgdial.Retry(ctx, log, func(ctx context.Context) error { return svc.Bootstrap(ctx, infra.BootstrapEmail) }); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	gateway, err := url.Parse(infra.AdminAPIURL)
	if err != nil {
		return fmt.Errorf("ADMIN_API_URL: %w", err)
	}
	clientTLS, err := infra.TLS.Client()
	if err != nil {
		return err
	}
	handler, err := dashboardhttp.New(dashboardhttp.Config{Service: svc, Origin: expected, TrustedProxies: infra.TrustedProxies, Log: log, Assets: web.FS(),
		Gateway: gateway, GatewayTransport: &http.Transport{TLSClientConfig: clientTLS, ResponseHeaderTimeout: 30 * time.Second}})
	if err != nil {
		return err
	}
	opsServer, err := ops.New(infra.OpsAddr, infra.ShutdownTimeout, gatherer)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", infra.HTTPAddr)
	if err != nil {
		return err
	}
	internalLn, err := net.Listen("tcp", infra.InternalAddr)
	if err != nil {
		_ = ln.Close()
		return err
	}
	sup := supervisor.Ordered{DrainDelay: infra.DrainDelay, DrainBudget: infra.DrainBudget}
	sup.Add("ops", opsServer.Run)
	sup.Add("delivery", every(deliveryEvery, func(ctx context.Context) {
		for {
			sent, err := svc.DeliverNext(ctx)
			if err != nil {
				log.WarnContext(ctx, "access link not sent", "error", err)
			}
			if !sent || err != nil {
				break
			}
		}
		n, err := svc.Undeliverable(ctx)
		if err != nil {
			log.WarnContext(ctx, "undeliverable links not counted", "error", err)
			return
		}
		undeliverable.Set(float64(n))
	}))
	sup.Add("purge", every(purgeEvery, func(ctx context.Context) {
		if err := svc.Purge(ctx); err != nil {
			log.WarnContext(ctx, "purge failed", "error", err)
		}
	}))
	sup.Add("http", serve(&http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second,
		ErrorLog: slog.NewLogLogger(log.Handler(), slog.LevelWarn)}, ln, infra.ShutdownTimeout))
	sup.Add("internal", serve(&http.Server{Handler: dashboardhttp.JWKS(svc), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		IdleTimeout: 60 * time.Second, ErrorLog: slog.NewLogLogger(log.Handler(), slog.LevelWarn)}, tls.NewListener(internalLn, internalTLS), infra.ShutdownTimeout))
	sup.OnDrain(opsServer.Drain)
	return sup.Run(ctx, log)
}

func every(period time.Duration, do func(context.Context)) func(context.Context) error {
	return func(ctx context.Context) error {
		t := time.NewTicker(period)
		defer t.Stop()
		for {
			do(ctx)
			select {
			case <-ctx.Done():
				return nil
			case <-t.C:
			}
		}
	}
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
