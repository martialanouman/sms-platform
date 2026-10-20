package dashboard

import "context"

// UseAccessLink sets the password of the link's operator and spends the link. The link is judged first, then the
// password, then hashed: a refused password or a busy hasher leaves the link valid. A reset link also closes every
// session of its operator.
func (s Service) UseAccessLink(ctx context.Context, token, password string) error {
	digest := Digest(token)
	var usable bool
	if err := s.Store.Tx(ctx, func(tx Tx) (err error) {
		usable, err = tx.LinkUsable(ctx, digest)
		return err
	}); err != nil {
		return err
	}
	if !usable {
		return ErrLinkInvalid
	}
	if missing := CheckPassword(password); len(missing) > 0 {
		return PolicyError{Missing: missing}
	}
	phc, err := s.Hasher.Hash(ctx, password)
	if err != nil {
		return err
	}
	return s.Store.Tx(ctx, func(tx Tx) error {
		id, kind, ok, err := tx.ConsumeLink(ctx, digest)
		if err != nil {
			return err
		}
		if !ok {
			return ErrLinkInvalid
		}
		if err := tx.SetPassword(ctx, id, phc); err != nil {
			return err
		}
		if kind == Reset {
			if err := tx.CloseSessions(ctx, id); err != nil {
				return err
			}
		}
		return tx.Log(ctx, Entry{Actor: id, Event: EventAccessLinkUsed, Success: true})
	})
}
