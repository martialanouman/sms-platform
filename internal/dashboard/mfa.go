package dashboard

import (
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"strings"
)

// Method is how a second factor is presented.
type Method string

// The methods of the BFF contract that a code proves.
const (
	MethodTotp         Method = "totp"
	MethodRecoveryCode Method = "recovery_code"
)

// Proof is a second factor presented: a code, or a passkey assertion as navigator.credentials.get() produced it.
type Proof struct {
	Method    Method
	Code      string
	Assertion []byte
}

// TotpEnrollment is shown once, at enrolment (I2).
type TotpEnrollment struct {
	Secret        string
	URI           string
	RecoveryCodes []string
}

var (
	// ErrAlreadyEnrolled is a factor in place and no proof of it.
	ErrAlreadyEnrolled = errors.New("a factor is in place")
	// ErrReplacementRefused is a proof of the factor in place refused.
	ErrReplacementRefused = errors.New("the proof of the factor in place was refused")
	// ErrElevationRequired is a session whose second factor was not verified.
	ErrElevationRequired = errors.New("the session is not elevated")
	// ErrNothingToConfirm is no enrolment pending.
	ErrNothingToConfirm = errors.New("no enrolment is pending")
	// ErrCodeRefused is a code of the pending authenticator app refused.
	ErrCodeRefused = errors.New("the code was refused")
)

const (
	totpSecretBytes   = 20
	recoveryCodeCount = 10
	recoveryCodeChars = 10
)

var codeNoise = strings.NewReplacer(" ", "", "-", "")

// EnrollTotp draws an authenticator app and its recovery codes, pending until their first use (verify) or, on a
// guarded account, their confirmation; a guarded account wants a proof of its factor.
func (s Service) EnrollTotp(ctx context.Context, cookie string, proof *Proof) (TotpEnrollment, error) {
	c, err := s.Resolve(ctx, cookie)
	if err != nil {
		return TotpEnrollment{}, err
	}
	if proof != nil {
		if err := s.reserve(ctx, c.OperatorID); err != nil {
			return TotpEnrollment{}, err
		}
	}
	secret := make([]byte, totpSecretBytes)
	_, _ = rand.Read(secret)
	codes, digests := newRecoveryCodes()
	err = s.Store.Tx(ctx, func(tx Tx) error {
		factors, err := tx.Factors(ctx, c.OperatorID)
		if err != nil {
			return err
		}
		confirmed, _ := split(factors)
		if confirmed == nil && c.Passkeys > 0 && !c.Elevated {
			return ErrElevationRequired
		}
		if confirmed != nil {
			if proof == nil {
				return ErrAlreadyEnrolled
			}
			ok, err := s.prove(ctx, tx, c.OperatorID, confirmed, *proof)
			if err != nil || !ok {
				return cmp.Or(err, ErrReplacementRefused)
			}
		}
		return tx.AddPending(ctx, c.OperatorID, s.Totp.seal(c.OperatorID, secret), digests)
	})
	if errors.Is(err, ErrReplacementRefused) {
		return TotpEnrollment{}, cmp.Or(s.logMfaFailure(ctx, c.OperatorID), err)
	}
	if err != nil {
		return TotpEnrollment{}, err
	}
	return TotpEnrollment{Secret: totpEncoding.EncodeToString(secret), URI: otpauthURI(secret, c.Email), RecoveryCodes: codes}, nil
}

// ConfirmTotp makes the pending authenticator app the only one, with its codes; it wants an elevated session.
func (s Service) ConfirmTotp(ctx context.Context, cookie, code string) error {
	c, err := s.Resolve(ctx, cookie)
	if err != nil {
		return err
	}
	if !c.Elevated {
		return ErrElevationRequired
	}
	if err := s.reserve(ctx, c.OperatorID); err != nil {
		return err
	}
	return s.Store.Tx(ctx, func(tx Tx) error {
		factors, err := tx.Factors(ctx, c.OperatorID)
		if err != nil {
			return err
		}
		_, pending := split(factors)
		if pending == nil {
			return ErrNothingToConfirm
		}
		ok, err := s.prove(ctx, tx, c.OperatorID, pending, Proof{Method: MethodTotp, Code: code})
		if err != nil || !ok {
			return cmp.Or(err, ErrCodeRefused)
		}
		return s.confirm(ctx, tx, c.OperatorID, pending)
	})
}

// Verify elevates the session of cookie by its login challenge and a second factor, and regenerates its token. On an
// account without a confirmed factor, a code of the pending authenticator app confirms it (arbitrage 84).
func (s Service) Verify(ctx context.Context, cookie, challenge string, p Proof) (string, error) {
	c, err := s.Resolve(ctx, cookie)
	if err != nil {
		return "", err
	}
	if err := s.reserve(ctx, c.OperatorID); err != nil {
		return "", err
	}
	token, _ := s.Keys.Open(cookie)
	next := NewToken()
	err = s.Store.Tx(ctx, func(tx Tx) error {
		elevated, err := tx.Elevate(ctx, Digest(token), Digest(challenge), Digest(next), IdleTimeout)
		if err != nil || !elevated {
			return cmp.Or(err, ErrUnauthenticated)
		}
		factors, err := tx.Factors(ctx, c.OperatorID)
		if err != nil {
			return err
		}
		confirmed, pending := split(factors)
		if c.Passkeys > 0 {
			pending = nil
		}
		factor := cmp.Or(confirmed, pending)
		var ok bool
		if p.Method == MethodWebauthn {
			if ok, err = s.assert(ctx, tx, c, p.Assertion); err == nil && ok {
				err = tx.Forget(ctx, s.Keys.MfaSubject(c.OperatorID))
			}
		} else {
			ok, err = s.prove(ctx, tx, c.OperatorID, factor, p)
		}
		if err != nil || !ok {
			return cmp.Or(err, ErrUnauthenticated)
		}
		if factor != nil && !factor.Confirmed {
			if err := s.confirm(ctx, tx, c.OperatorID, factor); err != nil {
				return err
			}
		}
		return tx.Log(ctx, Entry{Actor: c.OperatorID, Event: EventMFA, Success: true})
	})
	if errors.Is(err, ErrUnauthenticated) {
		return "", cmp.Or(s.logMfaFailure(ctx, c.OperatorID), ErrUnauthenticated)
	}
	if err != nil {
		return "", err
	}
	return s.Keys.Seal(next), nil
}

func (s Service) prove(ctx context.Context, tx Tx, operatorID string, f *TotpFactor, p Proof) (bool, error) {
	ok, err := s.check(ctx, tx, operatorID, f, p)
	if err != nil || !ok {
		return false, err
	}
	return true, tx.Forget(ctx, s.Keys.MfaSubject(operatorID))
}

func (s Service) check(ctx context.Context, tx Tx, operatorID string, f *TotpFactor, p Proof) (bool, error) {
	switch {
	case f == nil:
		return false, nil
	case p.Method == MethodTotp:
		secret, err := s.Totp.open(operatorID, f.Sealed)
		if err != nil {
			return false, err
		}
		now, err := tx.CurrentStep(ctx)
		if err != nil {
			return false, err
		}
		step, ok := totpMatch(secret, p.Code, now, f.LastStep)
		if !ok {
			return false, nil
		}
		return true, tx.ConsumeStep(ctx, f.ID, step)
	case p.Method == MethodRecoveryCode && f.Confirmed:
		return tx.UseRecoveryCode(ctx, f.ID, Digest(strings.ToUpper(codeNoise.Replace(p.Code))))
	}
	return false, nil
}

func (s Service) confirm(ctx context.Context, tx Tx, operatorID string, f *TotpFactor) error {
	replaced, err := tx.Confirm(ctx, operatorID, f.ID)
	if err != nil {
		return err
	}
	if replaced {
		if err := tx.Log(ctx, Entry{Actor: operatorID, Event: EventFactorRemoved, Success: true}); err != nil {
			return err
		}
	}
	return tx.Log(ctx, Entry{Actor: operatorID, Event: EventFactorEnrolled, Success: true})
}

func (s Service) reserve(ctx context.Context, operatorID string) error {
	return s.Store.Tx(ctx, func(tx Tx) error {
		retry, err := tx.Reserve(ctx, s.Keys.MfaSubject(operatorID), lockWindow, maxFailures)
		if err != nil || retry <= 0 {
			return err
		}
		return limited(retry)
	})
}

func (s Service) logMfaFailure(ctx context.Context, operatorID string) error {
	return s.Store.Tx(ctx, func(tx Tx) error {
		return tx.Log(ctx, Entry{Actor: operatorID, Event: EventMFA})
	})
}

func split(factors []TotpFactor) (confirmed, pending *TotpFactor) {
	for i := range factors {
		if factors[i].Confirmed {
			confirmed = &factors[i]
		} else {
			pending = &factors[i]
		}
	}
	return confirmed, pending
}

func newRecoveryCodes() (codes []string, digests [][]byte) {
	for range recoveryCodeCount {
		b := make([]byte, 7)
		_, _ = rand.Read(b)
		code := totpEncoding.EncodeToString(b)[:recoveryCodeChars]
		codes = append(codes, code)
		digests = append(digests, Digest(code))
	}
	return codes, digests
}
