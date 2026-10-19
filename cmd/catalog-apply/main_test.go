package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/martialanouman/sms-platform/internal/catalog"
	"github.com/martialanouman/sms-platform/internal/platform/config"
	"github.com/martialanouman/sms-platform/internal/platform/mtls/mtlstest"
)

func TestRunRefusesEmptyBrokers(t *testing.T) {
	err := run(context.Background(), config.Infra{}, slog.New(slog.DiscardHandler))
	if !errors.Is(err, errNoBrokers) {
		t.Fatalf("run = %v, want %v", err, errNoBrokers)
	}
}

func TestRunRefusesToDialWithoutMTLS(t *testing.T) {
	err := run(context.Background(), config.Infra{KafkaBrokers: []string{"127.0.0.1:1"}}, slog.New(slog.DiscardHandler))
	if err == nil || !strings.Contains(err.Error(), "kafka mTLS") {
		t.Fatalf("run = %v, want a refusal of the missing mTLS files", err)
	}
}

func TestRunDialsTheBrokersOverTLS(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	first := make(chan byte, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		b := make([]byte, 1)
		if _, err := io.ReadFull(conn, b); err == nil {
			first <- b[0]
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	infra := config.Infra{KafkaBrokers: []string{ln.Addr().String()}, TLS: mtlstest.New(t).Issue(t, t.TempDir(), "catalog-apply")}
	if err := run(ctx, infra, slog.New(slog.DiscardHandler)); err == nil {
		t.Fatal("run succeeded against a listener that speaks no Kafka")
	}
	select {
	case b := <-first:
		if b != 0x16 {
			t.Errorf("first byte sent to the broker = %#x, want a TLS handshake record (0x16)", b)
		}
	case <-time.After(5 * time.Second):
		t.Error("run never wrote to the broker")
	}
}

func messages(t *testing.T, p catalog.Plan) []string {
	t.Helper()
	var logs bytes.Buffer
	report(slog.New(slog.NewJSONHandler(&logs, nil)), p)
	var msgs []string
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var entry struct {
			Msg string `json:"msg"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		msgs = append(msgs, entry.Msg)
	}
	return msgs
}

func TestReportLogsEveryChange(t *testing.T) {
	a := catalog.ACL{Principal: "User:CN=router-svc", Resource: "TOPIC", Name: "mt.outcome", Pattern: "LITERAL", Operation: "WRITE", Permission: "ALLOW", Host: "*"}
	got := messages(t, catalog.Plan{
		Create:    map[string]catalog.Topic{"mt.outcome": {Partitions: 128}},
		Extend:    map[string]int{"mt.inbound.otp": 64},
		Configure: map[string]map[string]string{"dlr.events": {"retention.ms": "345600000"}},
		Add:       []catalog.ACL{a},
		Remove:    []catalog.ACL{a},
		Unmanaged: []string{"legacy"},
	})
	want := []string{"topic created", "topic extended", "topic configured", "ACL added", "ACL removed", "topic outside the catalog"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("report logged %q, want %q", got, want)
	}
	if got := messages(t, catalog.Plan{Unmanaged: []string{"legacy"}}); strings.Join(got, "|") != "topic outside the catalog|cluster already matches the catalog" {
		t.Errorf("report of an empty plan logged %q", got)
	}
}
