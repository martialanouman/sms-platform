// Package pgdial replays a Postgres connection while the network refuses it.
package pgdial

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/url"
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

var errNotADatabaseURL = errors.New("not a postgres URL naming a database")

// Database is the URL of database on the server of url, its credentials and options kept. Its error never quotes url,
// which holds a password.
func Database(rawURL, database string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || len(u.Path) < 2 {
		return "", errNotADatabaseURL
	}
	u.Path = "/" + database
	return u.String(), nil
}
