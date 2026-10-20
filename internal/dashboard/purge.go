package dashboard

import "context"

// Purge deletes the sessions that have ended and the counters whose window has passed.
func (s Service) Purge(ctx context.Context) error {
	return s.Store.Tx(ctx, func(tx Tx) error {
		return tx.Purge(ctx, IdleTimeout, lockWindow)
	})
}
