package pgdial_test

import (
	"context"
	"crypto/x509"
	"errors"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/martialanouman/sms-platform/internal/platform/pgdial"
)

func closedPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

func refused(t *testing.T, addr string) error {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err == nil {
		_ = conn.Close()
		t.Fatal("the closed port accepted")
	}
	return err
}

func TestRetryReplaysARefusedDialUntilItSucceeds(t *testing.T) {
	addr := closedPort(t)
	attempts := 0
	start := time.Now()
	err := pgdial.Retry(context.Background(), slog.New(slog.DiscardHandler), func(context.Context) error {
		if attempts++; attempts < 3 {
			return refused(t, addr)
		}
		return nil
	})
	if err != nil || attempts != 3 {
		t.Fatalf("Retry = %v after %d attempts, want success on the third", err, attempts)
	}
	if elapsed := time.Since(start); elapsed < 2*time.Second {
		t.Errorf("three attempts in %s, want a second between them", elapsed)
	}
}

func TestRetryReturnsAnythingButTheNetworkAtOnce(t *testing.T) {
	for _, fault := range []error{&pgconn.PgError{Code: "28P01"}, x509.UnknownAuthorityError{},
		&net.OpError{Op: "remote error", Net: "tcp", Err: errors.New("tls: bad certificate")}} {
		attempts := 0
		err := pgdial.Retry(context.Background(), slog.New(slog.DiscardHandler), func(context.Context) error {
			attempts++
			return fault
		})
		if !errors.Is(err, fault) || attempts != 1 {
			t.Errorf("Retry of %T = %v after %d attempts, want it at once", fault, err, attempts)
		}
	}
}

func TestRetryStopsWithTheContextOnTheLastNetworkError(t *testing.T) {
	addr := closedPort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	attempts := 0
	err := pgdial.Retry(ctx, slog.New(slog.DiscardHandler), func(context.Context) error {
		attempts++
		return refused(t, addr)
	})
	var netErr net.Error
	if !errors.As(err, &netErr) || attempts != 2 {
		t.Errorf("Retry = %v after %d attempts, want the network error after two", err, attempts)
	}
}

func TestRetryReplaysAnUnresolvedName(t *testing.T) {
	attempts := 0
	err := pgdial.Retry(context.Background(), slog.New(slog.DiscardHandler), func(context.Context) error {
		if attempts++; attempts < 2 {
			return &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "no such host", Name: "postgres-rw.postgres.svc", IsNotFound: true}}
		}
		return nil
	})
	if err != nil || attempts != 2 {
		t.Errorf("Retry = %v after %d attempts, want success on the second", err, attempts)
	}
}
