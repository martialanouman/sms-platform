package config_test

import (
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/martialanouman/sms-platform/internal/platform/config"
	"github.com/martialanouman/sms-platform/internal/platform/mtls"
)

func env(t *testing.T, values map[string]string) {
	t.Helper()
	for _, v := range config.Variables() {
		t.Setenv(v, "")
	}
	for k, v := range values {
		t.Setenv(k, v)
	}
}

func TestLoadAppliesTheDefaults(t *testing.T) {
	env(t, nil)
	got, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	want := config.Infra{
		LogLevel:         slog.LevelInfo,
		OpsAddr:          ":9090",
		DrainDelay:       5 * time.Second,
		DrainBudget:      90 * time.Second,
		ShutdownTimeout:  30 * time.Second,
		TraceSampleRatio: 1,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadReadsEveryVariable(t *testing.T) {
	env(t, map[string]string{
		"LOG_LEVEL":                 "debug",
		"OPS_ADDR":                  "127.0.0.1:9191",
		"DRAIN_DELAY":               "0s",
		"DRAIN_BUDGET":              "2m",
		"SHUTDOWN_TIMEOUT":          "1m",
		"TLS_CERT_FILE":             "/tls/tls.crt",
		"TLS_KEY_FILE":              "/tls/tls.key",
		"TLS_CA_FILE":               "/tls/ca.crt",
		"TLS_ALLOWED_CLIENTS":       "dashboard, admin-api-svc",
		"TRACE_SAMPLE_RATIO":        "0.25",
		"POSTGRES_URL":              "postgres://migrator@pg:5432/control_plane",
		"KAFKA_BROKERS":             "kafka-0:9093, kafka-1:9093",
		"POSTGRES_ROLE_SECRETS_DIR": "/secrets/postgres",
		"GRPC_ADDR":                 ":7002",
		"OPENBAO_ADDR":              "https://openbao.openbao.svc:8200",
	})
	got, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	want := config.Infra{
		LogLevel:               slog.LevelDebug,
		OpsAddr:                "127.0.0.1:9191",
		DrainBudget:            2 * time.Minute,
		ShutdownTimeout:        time.Minute,
		TLS:                    mtls.Files{Cert: "/tls/tls.crt", Key: "/tls/tls.key", CA: "/tls/ca.crt"},
		TLSAllowedClients:      []string{"dashboard", "admin-api-svc"},
		TraceSampleRatio:       0.25,
		PostgresURL:            "postgres://migrator@pg:5432/control_plane",
		KafkaBrokers:           []string{"kafka-0:9093", "kafka-1:9093"},
		PostgresRoleSecretsDir: "/secrets/postgres",
		GRPCAddr:               ":7002",
		OpenBaoAddr:            "https://openbao.openbao.svc:8200",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestVariablesIsTheClosedListLoadReads(t *testing.T) {
	want := []string{
		"DRAIN_BUDGET", "DRAIN_DELAY", "GRPC_ADDR", "KAFKA_BROKERS", "LOG_LEVEL", "OPENBAO_ADDR", "OPS_ADDR", "POSTGRES_ROLE_SECRETS_DIR", "POSTGRES_URL", "SHUTDOWN_TIMEOUT",
		"TLS_ALLOWED_CLIENTS", "TLS_CA_FILE", "TLS_CERT_FILE", "TLS_KEY_FILE", "TRACE_SAMPLE_RATIO",
	}
	if got := config.Variables(); !reflect.DeepEqual(got, want) {
		t.Errorf("Variables() = %v, want %v", got, want)
	}
}

func TestLoadRefusesAndReportsEveryFault(t *testing.T) {
	env(t, map[string]string{
		"LOG_LEVEL":          "loud",
		"DRAIN_DELAY":        "-1s",
		"SHUTDOWN_TIMEOUT":   "0s",
		"TRACE_SAMPLE_RATIO": "1.5",
		"DRAIN_BUDGET":       "soon",
	})
	_, err := config.Load()
	if err == nil {
		t.Fatal("Load accepted faulty variables")
	}
	for _, v := range []string{"LOG_LEVEL", "DRAIN_DELAY", "SHUTDOWN_TIMEOUT", "TRACE_SAMPLE_RATIO", "DRAIN_BUDGET"} {
		if !strings.Contains(err.Error(), v) {
			t.Errorf("error does not name %s: %v", v, err)
		}
	}
}

func TestLoadRefusesEachRule(t *testing.T) {
	for name, c := range map[string]struct {
		values map[string]string
		names  string
	}{
		"zero shutdown":            {map[string]string{"SHUTDOWN_TIMEOUT": "0s", "DRAIN_BUDGET": "1s"}, "SHUTDOWN_TIMEOUT"},
		"budget equal to shutdown": {map[string]string{"DRAIN_BUDGET": "30s"}, "DRAIN_BUDGET"},
		"negative ratio":           {map[string]string{"TRACE_SAMPLE_RATIO": "-0.1"}, "TRACE_SAMPLE_RATIO"},
		"NaN ratio":                {map[string]string{"TRACE_SAMPLE_RATIO": "NaN"}, "TRACE_SAMPLE_RATIO"},
		"unreadable ratio":         {map[string]string{"TRACE_SAMPLE_RATIO": "half"}, "TRACE_SAMPLE_RATIO"},
		"unreadable shutdown":      {map[string]string{"SHUTDOWN_TIMEOUT": "long"}, `SHUTDOWN_TIMEOUT: time: invalid duration "long"`},
		"unreadable delay":         {map[string]string{"DRAIN_DELAY": "later"}, "DRAIN_DELAY"},
	} {
		t.Run(name, func(t *testing.T) {
			env(t, c.values)
			if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), c.names) {
				t.Errorf("Load() error = %v, want one naming %s", err, c.names)
			}
		})
	}
}

func TestAnUnreadableDurationIsOneError(t *testing.T) {
	for _, name := range []string{"DRAIN_BUDGET", "SHUTDOWN_TIMEOUT"} {
		env(t, map[string]string{name: "90"})
		_, err := config.Load()
		if err == nil || strings.Count(err.Error(), name) != 1 {
			t.Errorf("Load() error = %v, want one error on %s", err, name)
		}
	}
}

func TestAnUnreadableShutdownIsNotComparedToTheBudget(t *testing.T) {
	env(t, map[string]string{"SHUTDOWN_TIMEOUT": "abc", "DRAIN_BUDGET": "0s"})
	_, err := config.Load()
	if err == nil || strings.Contains(err.Error(), "DRAIN_BUDGET") {
		t.Errorf("Load() error = %v, want only the unreadable SHUTDOWN_TIMEOUT", err)
	}
}

func TestLoadAcceptsTheRatioBounds(t *testing.T) {
	for _, ratio := range []string{"0", "1"} {
		env(t, map[string]string{"TRACE_SAMPLE_RATIO": ratio})
		if _, err := config.Load(); err != nil {
			t.Errorf("TRACE_SAMPLE_RATIO=%s: %v", ratio, err)
		}
	}
}
