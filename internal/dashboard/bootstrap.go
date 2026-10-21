package dashboard

import "context"

// Bootstrap seeds the default roles, creates the operator of email, named after it and without password, and queues
// its activation link; an existing operator is left as it is. The operator of email receives the Owner role when no
// active operator holds it.
func (s Service) Bootstrap(ctx context.Context, email string) error {
	email = normalize(email)
	return s.Store.Tx(ctx, func(tx Tx) error {
		for _, r := range DefaultRoles() {
			if err := tx.SeedRole(ctx, r); err != nil {
				return err
			}
		}
		id, created, err := tx.CreateOperator(ctx, email)
		if err != nil {
			return err
		}
		if created {
			if err := tx.QueueLink(ctx, id, Activation); err != nil {
				return err
			}
			if err := tx.Log(ctx, Entry{Event: EventAccessLinkRequested, Target: id, Success: true}); err != nil {
				return err
			}
		}
		owner, granted, err := tx.EnsureOwner(ctx, email)
		if err != nil || !granted {
			return err
		}
		return tx.Log(ctx, Entry{Event: EventRolesChanged, Target: owner, Success: true})
	})
}
