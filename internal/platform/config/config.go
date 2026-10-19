// Package config loads the infrastructure settings of a service from its environment; it is the only reader of the
// environment, and business configuration comes from the database (docs/design/07-distribution-de-config-et-redis.md).
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/martialanouman/sms-platform/internal/platform/mtls"
)

// Infra is the closed list of the environment variables a service reads.
type Infra struct {
	LogLevel          slog.Level
	OpsAddr           string
	DrainDelay        time.Duration
	DrainBudget       time.Duration
	ShutdownTimeout   time.Duration
	TLS               mtls.Files
	TLSAllowedClients []string
	TraceSampleRatio  float64
	PostgresURL       string
}

var defaults = map[string]string{
	"LOG_LEVEL":           "info",
	"OPS_ADDR":            ":9090",
	"DRAIN_DELAY":         "5s",
	"DRAIN_BUDGET":        "90s",
	"SHUTDOWN_TIMEOUT":    "30s",
	"TLS_CERT_FILE":       "",
	"TLS_KEY_FILE":        "",
	"TLS_CA_FILE":         "",
	"TLS_ALLOWED_CLIENTS": "",
	"TRACE_SAMPLE_RATIO":  "1",
	"POSTGRES_URL":        "",
}

// Variables is the sorted closed list of the environment variables Load reads.
func Variables() []string {
	return slices.Sorted(maps.Keys(defaults))
}

// Load reads Infra from the environment and reports every unreadable or inconsistent variable at once.
func Load() (Infra, error) {
	var errs []error
	unreadable := map[string]bool{}
	value := func(name string) string {
		if v := os.Getenv(name); v != "" {
			return v
		}
		return defaults[name]
	}
	duration := func(name string) time.Duration {
		d, err := time.ParseDuration(value(name))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			unreadable[name] = true
		}
		return d
	}
	c := Infra{
		OpsAddr:         value("OPS_ADDR"),
		DrainDelay:      duration("DRAIN_DELAY"),
		DrainBudget:     duration("DRAIN_BUDGET"),
		ShutdownTimeout: duration("SHUTDOWN_TIMEOUT"),
		TLS:             mtls.Files{Cert: value("TLS_CERT_FILE"), Key: value("TLS_KEY_FILE"), CA: value("TLS_CA_FILE")},
		PostgresURL:     value("POSTGRES_URL"),
	}
	if err := c.LogLevel.UnmarshalText([]byte(value("LOG_LEVEL"))); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL: %w", err))
	}
	for _, client := range strings.Split(value("TLS_ALLOWED_CLIENTS"), ",") {
		if client = strings.TrimSpace(client); client != "" {
			c.TLSAllowedClients = append(c.TLSAllowedClients, client)
		}
	}
	ratio, err := strconv.ParseFloat(value("TRACE_SAMPLE_RATIO"), 64)
	switch {
	case err != nil:
		errs = append(errs, fmt.Errorf("TRACE_SAMPLE_RATIO: %w", err))
	case !(ratio >= 0 && ratio <= 1):
		errs = append(errs, fmt.Errorf("TRACE_SAMPLE_RATIO: %v is outside [0, 1]", ratio))
	}
	c.TraceSampleRatio = ratio
	if c.DrainDelay < 0 {
		errs = append(errs, fmt.Errorf("DRAIN_DELAY: %s is negative", c.DrainDelay))
	}
	if c.ShutdownTimeout <= 0 && !unreadable["SHUTDOWN_TIMEOUT"] {
		errs = append(errs, fmt.Errorf("SHUTDOWN_TIMEOUT: %s is not positive", c.ShutdownTimeout))
	}
	if c.DrainBudget <= c.ShutdownTimeout && !unreadable["DRAIN_BUDGET"] && !unreadable["SHUTDOWN_TIMEOUT"] {
		errs = append(errs, fmt.Errorf("DRAIN_BUDGET: %s does not exceed SHUTDOWN_TIMEOUT %s", c.DrainBudget, c.ShutdownTimeout))
	}
	return c, errors.Join(errs...)
}
