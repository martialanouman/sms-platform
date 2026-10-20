package dashboard

import (
	"context"
	"fmt"
	"time"
)

// DeliverNext sends the oldest access link due, if any. A failed send counts against the link and is retried later;
// its error is returned once the attempt is recorded.
func (s Service) DeliverNext(ctx context.Context) (bool, error) {
	var found bool
	var sendErr error
	err := s.Store.Tx(ctx, func(tx Tx) error {
		due, ok, err := tx.NextDue(ctx, maxDeliveries)
		if err != nil || !ok {
			return err
		}
		found = true
		token := NewToken()
		subject, body, ttl := s.message(due.Kind, token)
		if sendErr = s.Mailer.Send(ctx, due.Email, subject, body); sendErr != nil {
			return tx.MarkFailed(ctx, due.OperatorID, backoffBase, backoffCeiling)
		}
		return tx.MarkSent(ctx, due.OperatorID, Digest(token), ttl)
	})
	if err != nil {
		return found, err
	}
	return found, sendErr
}

func (s Service) message(kind LinkKind, token string) (subject, body string, ttl time.Duration) {
	link := s.PublicURL + "/access#" + token
	if kind == Reset {
		return "Réinitialisation de votre mot de passe",
			fmt.Sprintf("Pour choisir un nouveau mot de passe, ouvrez ce lien dans l'heure :\n\n%s\n\nIl ne sert qu'une fois.\n", link), resetTTL
	}
	return "Activation de votre compte",
		fmt.Sprintf("Pour activer votre compte et choisir votre mot de passe, ouvrez ce lien dans les 72 heures :\n\n%s\n\nIl ne sert qu'une fois.\n", link), activationTTL
}
