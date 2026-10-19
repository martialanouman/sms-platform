package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/martialanouman/sms-platform/internal/catalog"
	"github.com/martialanouman/sms-platform/internal/platform/config"
	"github.com/martialanouman/sms-platform/internal/platform/mtls"
	"github.com/martialanouman/sms-platform/internal/platform/mtls/mtlstest"
	"github.com/martialanouman/sms-platform/internal/platform/pgtest"
)

func TestRunRefusesAnyMissingSettingBeforeConnecting(t *testing.T) {
	complete := func() config.Infra {
		return config.Infra{
			KafkaBrokers:           []string{"127.0.0.1:1"},
			PostgresURL:            "postgres://migrator@127.0.0.1:1/control_plane",
			PostgresRoleSecretsDir: t.TempDir(),
			TLS:                    mtlstest.New(t).Issue(t, t.TempDir(), "catalog-apply"),
		}
	}
	for name, c := range map[string]struct {
		drop func(*config.Infra)
		want string
	}{
		"brokers":     {func(i *config.Infra) { i.KafkaBrokers = nil }, errNoBrokers.Error()},
		"postgres":    {func(i *config.Infra) { i.PostgresURL = "" }, errNoPostgresURL.Error()},
		"secrets dir": {func(i *config.Infra) { i.PostgresRoleSecretsDir = "" }, errNoSecretsDir.Error()},
		"mTLS":        {func(i *config.Infra) { i.TLS = mtls.Files{} }, "kafka mTLS"},
	} {
		t.Run(name, func(t *testing.T) {
			infra := complete()
			c.drop(&infra)
			if err := run(context.Background(), infra, slog.New(slog.DiscardHandler)); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("run = %v, want %q", err, c.want)
			}
		})
	}
}

func TestRunGrantsPostgresThenDialsKafkaOverTLS(t *testing.T) {
	pg := pgtest.ControlPlane(t)
	dir := t.TempDir()
	secret := pgtest.Secret()
	if err := os.WriteFile(filepath.Join(dir, "admin_api_svc"), []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
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
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()
	infra := config.Infra{
		KafkaBrokers:           []string{ln.Addr().String()},
		PostgresURL:            pg.Late(t, 2*time.Second).As("migrator"),
		PostgresRoleSecretsDir: dir,
		TLS:                    mtlstest.New(t).Issue(t, t.TempDir(), "catalog-apply"),
	}
	var logs bytes.Buffer
	if err := run(ctx, infra, slog.New(slog.NewJSONHandler(&logs, nil))); err == nil {
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
	svc, err := pgx.Connect(context.Background(), pg.AsWith("admin_api_svc", secret))
	if err != nil {
		t.Fatalf("admin_api_svc cannot log in with its secret: %v", err)
	}
	_ = svc.Close(context.Background())
	if !strings.Contains(logs.String(), `"msg":"role secret set","role":"admin_api_svc"`) || strings.Contains(logs.String(), secret) ||
		strings.Contains(logs.String(), "migrator:migrator") {
		t.Errorf("log = %s, want the role named, without its secret nor the URL", logs.String())
	}
}

func TestRunFailsOnAMissingServiceSecret(t *testing.T) {
	infra := config.Infra{
		KafkaBrokers:           []string{"127.0.0.1:1"},
		PostgresURL:            pgtest.ControlPlane(t).As("migrator"),
		PostgresRoleSecretsDir: t.TempDir(),
		TLS:                    mtlstest.New(t).Issue(t, t.TempDir(), "catalog-apply"),
	}
	if err := run(context.Background(), infra, slog.New(slog.DiscardHandler)); err == nil || !strings.Contains(err.Error(), "admin_api_svc") {
		t.Fatalf("run = %v, want a refusal naming admin_api_svc", err)
	}
}

func messages(t *testing.T, p catalog.Plan) []string {
	t.Helper()
	var logs bytes.Buffer
	report(slog.New(slog.NewJSONHandler(&logs, nil)), p)
	var msgs []string
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		msg := fmt.Sprint(entry["msg"])
		if topic, ok := entry["topic"]; ok {
			msg += " " + fmt.Sprint(topic)
		}
		if partitions, ok := entry["partitions"]; ok {
			msg += " " + fmt.Sprint(partitions)
		}
		if acl, ok := entry["acl"].(map[string]any); ok {
			msg += " " + fmt.Sprint(acl["Principal"], " ", acl["Name"], " ", acl["Operation"])
		}
		msgs = append(msgs, msg)
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
	want := []string{"topic created mt.outcome 128", "topic extended mt.inbound.otp 64", "topic configured dlr.events", "ACL added User:CN=router-svc mt.outcome WRITE",
		"ACL removed User:CN=router-svc mt.outcome WRITE", "topic outside the catalog legacy"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("report logged %q, want %q", got, want)
	}
	if got := messages(t, catalog.Plan{Unmanaged: []string{"legacy"}}); strings.Join(got, "|") != "topic outside the catalog legacy|cluster already matches the catalog" {
		t.Errorf("report of an empty plan logged %q", got)
	}
}
