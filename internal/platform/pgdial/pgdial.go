// Package pgdial replays a Postgres connection while the network refuses it.
package pgdial

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"time"
)

const interval = time.Second

// Bound is how long a cmd/ waits for the network to admit it: a new pod's egress falls under default-deny until the
// policy controller admits its address, a matter of seconds; waiting for a store to start is the deployment's job.
const Bound = 60 * time.Second

// Retry calls attempt until it succeeds, fails on anything but the network, or ctx ends; then it returns the last error.
func Retry(ctx context.Context, log *slog.Logger, attempt func(context.Context) error) error {
	for {
		err := attempt(ctx)
		var netErr net.Error
		if err == nil || !errors.As(err, &netErr) {
			return err
		}
		log.Info("postgres unreachable, retrying", "error", err)
		select {
		case <-ctx.Done():
			return err
		case <-time.After(interval):
		}
	}
}
