package dashboard

import "context"

// UseAccessLink sets the password of the link's operator, spends the link and closes every session of the operator,
// in one transaction. The link is judged first, then the password, then hashed: a refused password or a busy hasher
// rolls back and leaves the link valid.
func (s Service) UseAccessLink(ctx context.Context, token, password string) error {
	return s.Store.Tx(ctx, func(tx Tx) error {
		id, ok, err := tx.ConsumeLink(ctx, Digest(token))
		switch {
		case err != nil:
			return err
		case !ok:
			return ErrLinkInvalid
		}
		if missing := CheckPassword(password); len(missing) > 0 {
			return PolicyError{Missing: missing}
		}
		phc, err := s.Hasher.Hash(password)
		if err != nil {
			return err
		}
		if err := tx.SetPassword(ctx, id, phc); err != nil {
			return err
		}
		if err := tx.CloseSessions(ctx, id); err != nil {
			return err
		}
		return tx.Log(ctx, Entry{Actor: id, Event: EventAccessLinkUsed, Success: true})
	})
}
