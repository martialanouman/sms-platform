package dashboard

import (
	"context"
	"errors"
	"math"
	"net/netip"
	"time"
)

// Login is an opened first-factor session: its sealed cookie and its single-use second-factor challenge.
type Login struct {
	Cookie             string
	Challenge          string
	ChallengeExpiresAt time.Time
}

// Login checks email and password from source. Each attempt is counted on the source and on the email before any
// hashing; an unknown email, a wrong password, an account without password and a disabled one are refused alike, in
// the same time.
func (s Service) Login(ctx context.Context, email, password string, source netip.Addr) (Login, error) {
	if !source.IsValid() {
		return Login{}, errors.New("login without a source address")
	}
	email = normalize(email)
	emailSubject := s.Keys.EmailSubject(email)
	var operator Operator
	var known bool
	var retry time.Duration
	err := s.Store.Tx(ctx, func(tx Tx) (err error) {
		for _, subject := range [][]byte{s.Keys.SourceSubject(source), emailSubject} {
			if retry, err = tx.Reserve(ctx, subject, lockWindow, maxFailures); err != nil || retry > 0 {
				return err
			}
		}
		operator, known, err = tx.OperatorByEmail(ctx, email)
		return err
	})
	if err != nil {
		return Login{}, err
	}
	if retry > 0 {
		return Login{}, RateLimited{RetryAfter: time.Duration(math.Ceil(retry.Seconds())) * time.Second}
	}
	var match bool
	if known && operator.Active && operator.PasswordHash != "" {
		match, err = s.Hasher.Verify(operator.PasswordHash, password)
	} else {
		err = s.Hasher.VerifyDummy(password)
	}
	if err != nil {
		return Login{}, err
	}
	if !match {
		return Login{}, s.refuse(ctx, operator.ID)
	}
	token, challenge := NewToken(), NewToken()
	var expires time.Time
	err = s.Store.Tx(ctx, func(tx Tx) error {
		if err := tx.Forget(ctx, emailSubject); err != nil {
			return err
		}
		var err error
		if expires, err = tx.OpenSession(ctx, operator.ID, Digest(token), Digest(challenge), Lifetime, challengeTTL); err != nil {
			return err
		}
		return tx.Log(ctx, Entry{Actor: operator.ID, Event: EventLogin, Success: true})
	})
	if err != nil {
		return Login{}, err
	}
	return Login{Cookie: s.Keys.Seal(token), Challenge: challenge, ChallengeExpiresAt: expires}, nil
}

func (s Service) refuse(ctx context.Context, operatorID string) error {
	if err := s.Store.Tx(ctx, func(tx Tx) error {
		return tx.Log(ctx, Entry{Actor: operatorID, Event: EventLogin})
	}); err != nil {
		return err
	}
	return ErrUnauthenticated
}
