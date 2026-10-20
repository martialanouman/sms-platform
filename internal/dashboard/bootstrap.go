package dashboard

import "context"

// Bootstrap creates the operator of email, without password, and queues its activation link; an existing operator is
// left as it is.
func (s Service) Bootstrap(ctx context.Context, email string) error {
	email = normalize(email)
	return s.Store.Tx(ctx, func(tx Tx) error {
		id, created, err := tx.CreateOperator(ctx, email, email)
		if err != nil || !created {
			return err
		}
		if err := tx.QueueLink(ctx, id, Activation); err != nil {
			return err
		}
		return tx.Log(ctx, Entry{Event: EventAccessLinkRequested, Target: id, Success: true})
	})
}
