// Package config loads the infrastructure settings of a service from its environment; it is the only reader of the
// environment, and business configuration comes from the database (docs/design/07-distribution-de-config-et-redis.md).
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
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
}

// Load reads Infra from the environment and reports every unreadable or inconsistent variable at once.
func Load() (Infra, error) {
	var errs []error
	value := func(name, fallback string) string {
		if v := os.Getenv(name); v != "" {
			return v
		}
		return fallback
	}
	duration := func(name, fallback string) time.Duration {
		d, err := time.ParseDuration(value(name, fallback))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
		return d
	}
	c := Infra{
		OpsAddr:         value("OPS_ADDR", ":9090"),
		DrainDelay:      duration("DRAIN_DELAY", "5s"),
		DrainBudget:     duration("DRAIN_BUDGET", "90s"),
		ShutdownTimeout: duration("SHUTDOWN_TIMEOUT", "30s"),
		TLS:             mtls.Files{Cert: value("TLS_CERT_FILE", ""), Key: value("TLS_KEY_FILE", ""), CA: value("TLS_CA_FILE", "")},
	}
	if err := c.LogLevel.UnmarshalText([]byte(value("LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL: %w", err))
	}
	for _, client := range strings.Split(value("TLS_ALLOWED_CLIENTS", ""), ",") {
		if client = strings.TrimSpace(client); client != "" {
			c.TLSAllowedClients = append(c.TLSAllowedClients, client)
		}
	}
	ratio, err := strconv.ParseFloat(value("TRACE_SAMPLE_RATIO", "1"), 64)
	switch {
	case err != nil:
		errs = append(errs, fmt.Errorf("TRACE_SAMPLE_RATIO: %w", err))
	case ratio < 0 || ratio > 1:
		errs = append(errs, fmt.Errorf("TRACE_SAMPLE_RATIO: %v is outside [0, 1]", ratio))
	}
	c.TraceSampleRatio = ratio
	if c.DrainDelay < 0 {
		errs = append(errs, fmt.Errorf("DRAIN_DELAY: %s is negative", c.DrainDelay))
	}
	if c.ShutdownTimeout <= 0 {
		errs = append(errs, fmt.Errorf("SHUTDOWN_TIMEOUT: %s is not positive", c.ShutdownTimeout))
	}
	if c.DrainBudget <= c.ShutdownTimeout {
		errs = append(errs, fmt.Errorf("DRAIN_BUDGET: %s does not exceed SHUTDOWN_TIMEOUT %s", c.DrainBudget, c.ShutdownTimeout))
	}
	return c, errors.Join(errs...)
}
