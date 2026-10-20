package smtp_test

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/martialanouman/sms-platform/internal/dashboard/smtp"
	"github.com/martialanouman/sms-platform/internal/platform/mtls"
	"github.com/martialanouman/sms-platform/internal/platform/mtls/mtlstest"
)

type mailpit struct {
	smtp, api string
}

func start(t *testing.T, tlsFiles *mtls.Files, env map[string]string) mailpit {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	opts := []testcontainers.ContainerCustomizer{
		testcontainers.WithExposedPorts("1025/tcp", "8025/tcp"),
		testcontainers.WithEnv(env),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/livez").WithPort("8025/tcp")),
	}
	if tlsFiles != nil {
		opts = append(opts, testcontainers.WithFiles(
			testcontainers.ContainerFile{HostFilePath: tlsFiles.Cert, ContainerFilePath: "/tls/cert.pem", FileMode: 0o644},
			testcontainers.ContainerFile{HostFilePath: tlsFiles.Key, ContainerFilePath: "/tls/key.pem", FileMode: 0o644},
		))
		env["MP_SMTP_TLS_CERT"], env["MP_SMTP_TLS_KEY"] = "/tls/cert.pem", "/tls/key.pem"
	}
	c, err := testcontainers.Run(ctx, "axllent/mailpit:v1.31.4", opts...)
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatal(err)
	}
	smtpAddr, err := c.PortEndpoint(ctx, "1025/tcp", "")
	if err != nil {
		t.Fatal(err)
	}
	api, err := c.PortEndpoint(ctx, "8025/tcp", "http")
	if err != nil {
		t.Fatal(err)
	}
	return mailpit{smtp: strings.Replace(smtpAddr, "127.0.0.1", "localhost", 1), api: api}
}

type received struct {
	To      []struct{ Address string }
	Subject string
	Text    string
}

func (m mailpit) messages(t *testing.T) []received {
	t.Helper()
	resp, err := http.Get(m.api + "/api/v1/messages")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var list struct{ Messages []struct{ ID string } }
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	var all []received
	for _, msg := range list.Messages {
		r, err := http.Get(m.api + "/api/v1/message/" + msg.ID)
		if err != nil {
			t.Fatal(err)
		}
		var got received
		err = json.NewDecoder(r.Body).Decode(&got)
		_ = r.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, got)
	}
	return all
}

func roots(ca mtlstest.Authority) *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(ca.PEM)
	return pool
}

const body = "Pour activer votre compte, ouvrez https://dashboard.example.test/access#jeton\n"

func send(t *testing.T, m smtp.Mailer) error {
	t.Helper()
	return m.Send(context.Background(), "owner@example.test", "Activation de votre compte", body)
}

func delivered(t *testing.T, m mailpit) {
	t.Helper()
	got := m.messages(t)
	if len(got) != 1 || len(got[0].To) != 1 || got[0].To[0].Address != "owner@example.test" ||
		got[0].Subject != "Activation de votre compte" || strings.TrimSpace(got[0].Text) != strings.TrimSpace(body) {
		t.Fatalf("received %+v", got)
	}
}

func TestAMessageGoesInClearWhenTheServerOffersNoTLS(t *testing.T) {
	m := start(t, nil, map[string]string{})
	if err := send(t, smtp.Mailer{Addr: m.smtp, From: "dashboard@example.test"}); err != nil {
		t.Fatal(err)
	}
	delivered(t, m)
}

func TestAMessageGoesUnderSTARTTLSVerifiedByTheCA(t *testing.T) {
	ca := mtlstest.New(t)
	files := ca.Issue(t, t.TempDir(), "localhost")
	m := start(t, &files, map[string]string{"MP_SMTP_REQUIRE_STARTTLS": "true"})
	if err := send(t, smtp.Mailer{Addr: m.smtp, From: "dashboard@example.test", Roots: roots(mtlstest.New(t))}); err == nil {
		t.Fatal("a server of another CA was trusted")
	}
	if err := send(t, smtp.Mailer{Addr: m.smtp, From: "dashboard@example.test", Roots: roots(ca)}); err != nil {
		t.Fatal(err)
	}
	delivered(t, m)
}

func TestCredentialsGoOnlyUnderTLS(t *testing.T) {
	clear := start(t, nil, map[string]string{"MP_SMTP_AUTH_ACCEPT_ANY": "true", "MP_SMTP_AUTH_ALLOW_INSECURE": "true"})
	err := send(t, smtp.Mailer{Addr: clear.smtp, From: "dashboard@example.test", Username: "relay", Password: "relay secret"})
	if err == nil || strings.Contains(err.Error(), "relay secret") {
		t.Fatalf("credentials over a clear connection: %v, want a refusal", err)
	}
	if got := clear.messages(t); len(got) != 0 {
		t.Fatalf("delivered %+v", got)
	}
	ca := mtlstest.New(t)
	files := ca.Issue(t, t.TempDir(), "localhost")
	secured := start(t, &files, map[string]string{"MP_SMTP_AUTH": "relay:relay-secret"})
	for _, wrong := range []smtp.Mailer{
		{Addr: secured.smtp, From: "dashboard@example.test", Roots: roots(ca)},
		{Addr: secured.smtp, From: "dashboard@example.test", Username: "relay", Password: "other-secret", Roots: roots(ca)},
	} {
		if err := send(t, wrong); err == nil {
			t.Fatalf("the relay took a message with credentials %q:%q", wrong.Username, wrong.Password)
		}
	}
	if err := send(t, smtp.Mailer{Addr: secured.smtp, From: "dashboard@example.test", Username: "relay", Password: "relay-secret", Roots: roots(ca)}); err != nil {
		t.Fatal(err)
	}
	delivered(t, secured)
}

func TestARefusalCarriesTheCodeOnly(t *testing.T) {
	m := start(t, nil, map[string]string{"MP_SMTP_ALLOWED_RECIPIENTS": "@nowhere\\.test$"})
	err := send(t, smtp.Mailer{Addr: m.smtp, From: "dashboard@example.test"})
	if err == nil || err.Error() != fmt.Sprintf("smtp: %d", 550) {
		t.Fatalf("err = %v, want the code alone", err)
	}
}

func TestAHeaderInjectionIsRefused(t *testing.T) {
	m := start(t, nil, map[string]string{})
	mailer := smtp.Mailer{Addr: m.smtp, From: "dashboard@example.test"}
	for _, tc := range [][2]string{
		{"owner@example.test", "Activation\r\nBcc: intruder@example.test"},
		{"owner@example.test\r\nBcc: intruder@example.test", "Activation"},
	} {
		if err := mailer.Send(context.Background(), tc[0], tc[1], body); err == nil {
			t.Errorf("to %q, subject %q: sent", tc[0], tc[1])
		}
	}
	if got := m.messages(t); len(got) != 0 {
		t.Fatalf("delivered %+v", got)
	}
}

func TestASilentRelayIsGivenUpAtTheCallersDeadline(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = c.Close() })
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := (smtp.Mailer{Addr: ln.Addr().String(), From: "dashboard@example.test"}).Send(ctx, "owner@example.test", "Activation", body); err == nil {
		t.Fatal("a silent relay took the message")
	}
	if waited := time.Since(start); waited > 2*time.Second {
		t.Fatalf("gave up after %v, want the 200 ms of the caller", waited)
	}
}
