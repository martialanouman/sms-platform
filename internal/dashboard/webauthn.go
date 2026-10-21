package dashboard

import (
	"cmp"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// MethodWebauthn is a passkey assertion.
const MethodWebauthn Method = "webauthn"

// Purpose is what a WebAuthn ceremony opens: one live challenge per session and purpose.
type Purpose string

// The two ceremonies.
const (
	PurposeRegistration Purpose = "registration"
	PurposeAssertion    Purpose = "assertion"
)

const ceremonyTTL = 5 * time.Minute

var (
	// ErrCeremonyRefused is a registration refused, whatever the cause.
	ErrCeremonyRefused = errors.New("the WebAuthn ceremony was refused")
	// ErrNoPasskey is an assertion opened on an account without passkey.
	ErrNoPasskey = errors.New("no passkey is registered")
	// ErrPasskeyUnknown is a passkey the operator does not hold.
	ErrPasskeyUnknown = errors.New("not a passkey of this operator")
	// ErrLastFactor is a removal that would leave the operator without a factor.
	ErrLastFactor = errors.New("the last factor cannot be removed")
)

// Passkey is what an operator sees of a passkey: nothing of the key.
type Passkey struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

// Passkeys is the relying party, its identifier and origin taken from the public URL, never from a request.
type Passkeys struct {
	rp *webauthn.WebAuthn
}

// NewPasskeys is the relying party of publicURL; an identifier WebAuthn refuses, such as an IP address, is refused.
func NewPasskeys(publicURL string) (Passkeys, error) {
	u, err := url.Parse(publicURL)
	if err != nil {
		return Passkeys{}, err
	}
	timeout := webauthn.TimeoutConfig{Enforce: true, Timeout: ceremonyTTL, TimeoutUVD: ceremonyTTL}
	rp, err := webauthn.New(&webauthn.Config{
		RPID:          u.Hostname(),
		RPDisplayName: totpIssuer,
		RPOrigins:     []string{publicURL},
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementPreferred,
			UserVerification: protocol.VerificationPreferred,
		},
		Timeouts: webauthn.TimeoutsConfig{Login: timeout, Registration: timeout},
	})
	return Passkeys{rp: rp}, err
}

type holder struct {
	Current
	keys []StoredPasskey
}

func (h holder) WebAuthnID() []byte {
	id, _ := hex.DecodeString(strings.ReplaceAll(h.OperatorID, "-", ""))
	return id
}

func (h holder) WebAuthnName() string        { return h.Email }
func (h holder) WebAuthnDisplayName() string { return h.DisplayName }

func (h holder) WebAuthnCredentials() []webauthn.Credential {
	creds := make([]webauthn.Credential, 0, len(h.keys))
	for _, k := range h.keys {
		var c webauthn.Credential
		if json.Unmarshal(k.Credential, &c) == nil {
			creds = append(creds, c)
		}
	}
	return creds
}

// BeginRegistration opens a passkey registration, replacing the one open; a guarded account wants an elevated
// session. It returns the options of navigator.credentials.create(), as JSON.
func (s Service) BeginRegistration(ctx context.Context, cookie string) ([]byte, error) {
	c, err := s.Resolve(ctx, cookie)
	if err != nil {
		return nil, err
	}
	var options []byte
	err = s.Store.Tx(ctx, func(tx Tx) error {
		h, err := s.guard(ctx, tx, c)
		if err != nil {
			return err
		}
		creation, session, err := s.Passkeys.rp.BeginRegistration(h, webauthn.WithExclusions(webauthn.Credentials(h.WebAuthnCredentials()).CredentialDescriptors()))
		if err != nil {
			return err
		}
		if options, err = json.Marshal(creation); err != nil {
			return err
		}
		return s.open(ctx, tx, c.SessionID, PurposeRegistration, session)
	})
	return options, err
}

// FinishRegistration records the passkey the device produced under name; the challenge is spent whatever the outcome.
func (s Service) FinishRegistration(ctx context.Context, cookie, name string, attestation []byte) (string, error) {
	c, err := s.Resolve(ctx, cookie)
	if err != nil {
		return "", err
	}
	session, ok, err := s.take(ctx, c.SessionID, PurposeRegistration)
	if err != nil || !ok {
		return "", cmp.Or(err, ErrCeremonyRefused)
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(attestation)
	if err != nil {
		return "", ErrCeremonyRefused
	}
	var id string
	err = s.Store.Tx(ctx, func(tx Tx) error {
		h, err := s.guard(ctx, tx, c)
		if err != nil {
			return err
		}
		cred, err := s.Passkeys.rp.CreateCredential(h, session, parsed)
		if err != nil {
			return ErrCeremonyRefused
		}
		data, err := json.Marshal(cred)
		if err != nil {
			return err
		}
		var added bool
		if id, added, err = tx.AddPasskey(ctx, c.OperatorID, cred.ID, name, cred.Authenticator.SignCount, data); err != nil || !added {
			return cmp.Or(err, ErrCeremonyRefused)
		}
		return tx.Log(ctx, Entry{Actor: c.OperatorID, Event: EventFactorEnrolled, Success: true})
	})
	return id, err
}

// BeginAssertion opens a passkey assertion, ended by Verify. It returns the options of navigator.credentials.get(),
// as JSON.
func (s Service) BeginAssertion(ctx context.Context, cookie string) ([]byte, error) {
	c, err := s.Resolve(ctx, cookie)
	if err != nil {
		return nil, err
	}
	var options []byte
	err = s.Store.Tx(ctx, func(tx Tx) error {
		keys, err := tx.Passkeys(ctx, c.OperatorID)
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			return ErrNoPasskey
		}
		assertion, session, err := s.Passkeys.rp.BeginLogin(holder{Current: c, keys: keys})
		if err != nil {
			return err
		}
		if options, err = json.Marshal(assertion); err != nil {
			return err
		}
		return s.open(ctx, tx, c.SessionID, PurposeAssertion, session)
	})
	return options, err
}

// ListPasskeys is the passkeys of the operator, in registration order.
func (s Service) ListPasskeys(ctx context.Context, cookie string) ([]Passkey, error) {
	c, err := s.Resolve(ctx, cookie)
	if err != nil {
		return nil, err
	}
	var keys []Passkey
	err = s.Store.Tx(ctx, func(tx Tx) error {
		stored, err := tx.Passkeys(ctx, c.OperatorID)
		for _, k := range stored {
			keys = append(keys, k.Passkey)
		}
		return err
	})
	return keys, err
}

// RemovePasskey removes a passkey of the operator from an elevated session, unless it is the last factor.
func (s Service) RemovePasskey(ctx context.Context, cookie, id string) error {
	c, err := s.Resolve(ctx, cookie)
	if err != nil {
		return err
	}
	if !c.Fresh {
		return ErrElevationRequired
	}
	return s.Store.Tx(ctx, func(tx Tx) error {
		factors, err := tx.Factors(ctx, c.OperatorID)
		if err != nil {
			return err
		}
		keys, err := tx.Passkeys(ctx, c.OperatorID)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(keys, func(k StoredPasskey) bool { return k.ID == id }) {
			return ErrPasskeyUnknown
		}
		if confirmed, _ := split(factors); confirmed == nil && len(keys) == 1 {
			return ErrLastFactor
		}
		if err := tx.RemovePasskey(ctx, c.OperatorID, id); err != nil {
			return err
		}
		return tx.Log(ctx, Entry{Actor: c.OperatorID, Event: EventFactorRemoved, Success: true})
	})
}

func (s Service) assert(ctx context.Context, tx Tx, c Current, keys []StoredPasskey, session webauthn.SessionData, assertion []byte) (bool, error) {
	parsed, err := protocol.ParseCredentialRequestResponseBytes(assertion)
	if err != nil {
		return false, nil
	}
	cred, err := s.Passkeys.rp.ValidateLogin(holder{Current: c, keys: keys}, session, parsed)
	if err != nil {
		return false, nil
	}
	updated, err := json.Marshal(cred)
	if err != nil {
		return false, err
	}
	ok, err := tx.AdvancePasskey(ctx, c.OperatorID, cred.ID, cred.Authenticator.SignCount, updated)
	if err != nil || !ok {
		return false, err
	}
	return true, tx.Forget(ctx, s.Keys.MfaSubject(c.OperatorID))
}

func (s Service) guard(ctx context.Context, tx Tx, c Current) (holder, error) {
	factors, err := tx.Factors(ctx, c.OperatorID)
	if err != nil {
		return holder{}, err
	}
	keys, err := tx.Passkeys(ctx, c.OperatorID)
	if err != nil {
		return holder{}, err
	}
	if confirmed, _ := split(factors); (confirmed != nil || len(keys) > 0) && !c.Fresh {
		return holder{}, ErrElevationRequired
	}
	return holder{Current: c, keys: keys}, nil
}

func (s Service) open(ctx context.Context, tx Tx, sessionID string, purpose Purpose, session *webauthn.SessionData) error {
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return tx.OpenCeremony(ctx, sessionID, purpose, data)
}

func (s Service) take(ctx context.Context, sessionID string, purpose Purpose) (webauthn.SessionData, bool, error) {
	var session webauthn.SessionData
	var ok bool
	err := s.Store.Tx(ctx, func(tx Tx) error {
		data, found, err := tx.TakeCeremony(ctx, sessionID, purpose)
		if err != nil || !found {
			return err
		}
		ok = true
		return json.Unmarshal(data, &session)
	})
	return session, ok, err
}
