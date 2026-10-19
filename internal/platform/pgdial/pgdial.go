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

// A new pod's egress falls under default-deny until the policy controller admits its address, a matter of seconds;
// waiting for a store to start is the deployment's job, not this bound's.
const bound = 60 * time.Second

// Retry calls attempt until it succeeds, fails on anything but reaching the server, or ctx or a minute ends; then it
// returns the last error. A refusal, a dial timeout or an unresolved name is replayed; a TLS alert, a read or a write
// cut is not.
func Retry(ctx context.Context, log *slog.Logger, attempt func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, bound)
	defer cancel()
	for {
		err := attempt(ctx)
		var opErr *net.OpError
		var dnsErr *net.DNSError
		dial := errors.As(err, &opErr) && opErr.Op == "dial"
		if err == nil || !dial && !errors.As(err, &dnsErr) {
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
