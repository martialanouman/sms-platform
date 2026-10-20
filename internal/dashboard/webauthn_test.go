package dashboard_test

import (
	"context"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/descope/virtualwebauthn"

	"github.com/martialanouman/sms-platform/internal/dashboard"
)

var relyingParty = virtualwebauthn.RelyingParty{ID: "dashboard.example.test", Name: "Tableau de bord SMS", Origin: publicURL}

type device struct {
	auth virtualwebauthn.Authenticator
	cred virtualwebauthn.Credential
	rp   virtualwebauthn.RelyingParty
}

func newDevice() *device {
	d := &device{auth: virtualwebauthn.NewAuthenticator(), cred: virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2), rp: relyingParty}
	d.auth.AddCredential(d.cred)
	return d
}

func (d *device) attest(t *testing.T, options []byte) []byte {
	t.Helper()
	o, err := virtualwebauthn.ParseAttestationOptions(string(options))
	if err != nil {
		t.Fatal(err)
	}
	return []byte(virtualwebauthn.CreateAttestationResponse(d.rp, d.auth, d.cred, *o))
}

func (d *device) assert(t *testing.T, options []byte) []byte {
	t.Helper()
	o, err := virtualwebauthn.ParseAssertionOptions(string(options))
	if err != nil {
		t.Fatal(err)
	}
	return []byte(virtualwebauthn.CreateAssertionResponse(d.rp, d.auth, d.cred, *o))
}

func passkey(assertion []byte) dashboard.Proof {
	return dashboard.Proof{Method: dashboard.MethodWebauthn, Assertion: assertion}
}

func (h *harness) begin(t *testing.T, cookie string) []byte {
	t.Helper()
	options, err := h.svc.BeginRegistration(context.Background(), cookie)
	if err != nil {
		t.Fatal(err)
	}
	return options
}

func (h *harness) register(t *testing.T, cookie, name string, d *device) string {
	t.Helper()
	id, err := h.svc.FinishRegistration(context.Background(), cookie, name, d.attest(t, h.begin(t, cookie)))
	if err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
	return id
}

func (h *harness) assertion(t *testing.T, cookie string, d *device) []byte {
	t.Helper()
	options, err := h.svc.BeginAssertion(context.Background(), cookie)
	if err != nil {
		t.Fatal(err)
	}
	return d.assert(t, options)
}

func (h *harness) elevate(t *testing.T, l dashboard.Login, d *device) string {
	t.Helper()
	return h.verify(t, l, passkey(h.assertion(t, l.Cookie, d)))
}

func (h *harness) withPasskey(t *testing.T) (string, *device) {
	t.Helper()
	h.activate(t, "owner@example.test")
	d := newDevice()
	h.register(t, h.login(t, "owner@example.test").Cookie, "Clé USB", d)
	return h.elevate(t, h.login(t, "owner@example.test"), d), d
}

func TestAPasskeyRegisteredWithoutAFactorElevatesThroughVerify(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.activate(t, "owner@example.test")
	l := h.login(t, "owner@example.test")
	d := newDevice()
	id := h.register(t, l.Cookie, "Clé USB", d)
	if c := h.current(t, l.Cookie); c.Elevated || c.Passkeys != 1 {
		t.Fatalf("after the registration: %+v", c)
	}
	keys, err := h.svc.ListPasskeys(ctx, l.Cookie)
	if err != nil || len(keys) != 1 || keys[0].ID != id || keys[0].Name != "Clé USB" || keys[0].CreatedAt.IsZero() {
		t.Fatalf("passkeys %+v (%v)", keys, err)
	}
	elevated := h.elevate(t, l, d)
	if elevated == l.Cookie {
		t.Fatal("the session token was not regenerated")
	}
	if c := h.current(t, elevated); !c.Elevated || c.Passkeys != 1 || c.Totp {
		t.Fatalf("after the assertion: %+v", c)
	}
	if got := h.events(t); !slices.Equal(got[len(got)-2:], []string{"factor_enrolled:success", "mfa:success"}) {
		t.Fatalf("log %q", got)
	}
}

func TestARegistrationChallengeServesOnceWhateverTheOutcome(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.activate(t, "owner@example.test")
	cookie := h.login(t, "owner@example.test").Cookie
	d := newDevice()
	attestation := d.attest(t, h.begin(t, cookie))
	if _, err := h.svc.FinishRegistration(ctx, cookie, "Clé", []byte(`{"id":"broken"}`)); !errors.Is(err, dashboard.ErrCeremonyRefused) {
		t.Fatalf("a malformed attestation: %v", err)
	}
	if _, err := h.svc.FinishRegistration(ctx, cookie, "Clé", attestation); !errors.Is(err, dashboard.ErrCeremonyRefused) {
		t.Fatalf("the challenge spent by a refusal served again: %v", err)
	}
	attestation = d.attest(t, h.begin(t, cookie))
	if _, err := h.svc.FinishRegistration(ctx, cookie, "Clé", attestation); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.FinishRegistration(ctx, cookie, "Clé", attestation); !errors.Is(err, dashboard.ErrCeremonyRefused) {
		t.Fatalf("a replayed attestation: %v", err)
	}
}

func TestOpeningACeremonyReplacesTheChallengeOfItsPurpose(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.activate(t, "owner@example.test")
	cookie := h.login(t, "owner@example.test").Cookie
	d := newDevice()
	first := h.begin(t, cookie)
	second := h.begin(t, cookie)
	if _, err := h.svc.FinishRegistration(ctx, cookie, "Clé", d.attest(t, first)); !errors.Is(err, dashboard.ErrCeremonyRefused) {
		t.Fatalf("the replaced challenge served: %v", err)
	}
	if _, err := h.svc.FinishRegistration(ctx, cookie, "Clé", d.attest(t, second)); !errors.Is(err, dashboard.ErrCeremonyRefused) {
		t.Fatalf("the refusal did not spend the challenge: %v", err)
	}
	h.register(t, cookie, "Clé", d)
	var rows int
	if err := h.owner.QueryRow(ctx, `SELECT count(*) FROM dashboard.webauthn_challenges`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("%d challenges left (%v)", rows, err)
	}
}

func TestAChallengeServesOnlyItsPurposeItsSessionAndItsOrigin(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.activate(t, "fresh@example.test")
	stray := newDevice()
	opened := h.begin(t, h.login(t, "fresh@example.test").Cookie)
	if _, err := h.svc.FinishRegistration(ctx, h.login(t, "fresh@example.test").Cookie, "Clé", stray.attest(t, opened)); !errors.Is(err, dashboard.ErrCeremonyRefused) {
		t.Fatalf("the registration challenge of another session: %v", err)
	}
	_, d := h.withPasskey(t)
	l := h.login(t, "owner@example.test")
	options, err := h.svc.BeginAssertion(ctx, h.login(t, "owner@example.test").Cookie)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Verify(ctx, l.Cookie, l.Challenge, passkey(d.assert(t, options))); !errors.Is(err, dashboard.ErrUnauthenticated) {
		t.Fatalf("the assertion challenge of another session: %v", err)
	}
	away := *d
	away.rp.Origin = "https://ailleurs.example.test"
	if _, err := h.svc.Verify(ctx, l.Cookie, l.Challenge, passkey(away.assert(t, mustOptions(t, h, l.Cookie)))); !errors.Is(err, dashboard.ErrUnauthenticated) {
		t.Fatalf("an assertion from another origin: %v", err)
	}
	elevated := h.elevate(t, h.login(t, "owner@example.test"), d)
	strayAway := newDevice()
	strayAway.rp.Origin = "https://ailleurs.example.test"
	if _, err := h.svc.FinishRegistration(ctx, elevated, "Clé", strayAway.attest(t, h.begin(t, elevated))); !errors.Is(err, dashboard.ErrCeremonyRefused) {
		t.Fatalf("a registration from another origin: %v", err)
	}
	if _, err := h.svc.FinishRegistration(ctx, elevated, "Clé", newDevice().attest(t, asCreation(mustOptions(t, h, elevated)))); !errors.Is(err, dashboard.ErrCeremonyRefused) {
		t.Fatalf("an assertion challenge finished a registration: %v", err)
	}
}

func mustOptions(t *testing.T, h *harness, cookie string) []byte {
	t.Helper()
	options, err := h.svc.BeginAssertion(context.Background(), cookie)
	if err != nil {
		t.Fatal(err)
	}
	return options
}

func asCreation(assertion []byte) []byte {
	return []byte(strings.Replace(string(assertion), `"publicKey":{`, `"publicKey":{"rp":{"id":"dashboard.example.test","name":"x"},"user":{"id":"AA","name":"x","displayName":"x"},"pubKeyCredParams":[{"type":"public-key","alg":-7}],`, 1))
}

func TestAKeyHeldIsExcludedAndRegisteredOnceAcrossOperators(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	elevated, d := h.withPasskey(t)
	options := string(h.begin(t, elevated))
	if !strings.Contains(options, base64.RawURLEncoding.EncodeToString(d.cred.ID)) {
		t.Fatalf("the key held is not excluded: %s", options)
	}
	if _, err := h.svc.FinishRegistration(ctx, elevated, "Encore", d.attest(t, h.begin(t, elevated))); !errors.Is(err, dashboard.ErrCeremonyRefused) {
		t.Fatalf("the same key registered twice: %v", err)
	}
	h.activate(t, "other@example.test")
	other := h.login(t, "other@example.test").Cookie
	if _, err := h.svc.FinishRegistration(ctx, other, "Volée", d.attest(t, h.begin(t, other))); !errors.Is(err, dashboard.ErrCeremonyRefused) {
		t.Fatalf("the key of another operator registered: %v", err)
	}
	if c := h.current(t, other); c.Passkeys != 0 {
		t.Fatalf("the other operator holds %d passkeys", c.Passkeys)
	}
}

func TestAGuardedAccountOpensARegistrationOnlyWhenElevated(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.guarded(t)
	l := h.login(t, "owner@example.test")
	if _, err := h.svc.BeginRegistration(ctx, l.Cookie); !errors.Is(err, dashboard.ErrElevationRequired) {
		t.Fatalf("an authenticator app guards the account: %v", err)
	}
	h.activate(t, "keyed@example.test")
	d := newDevice()
	h.register(t, h.login(t, "keyed@example.test").Cookie, "Clé", d)
	keyed := h.login(t, "keyed@example.test")
	if _, err := h.svc.BeginRegistration(ctx, keyed.Cookie); !errors.Is(err, dashboard.ErrElevationRequired) {
		t.Fatalf("a passkey guards the account: %v", err)
	}
	h.register(t, h.elevate(t, keyed, d), "Seconde", newDevice())
}

func TestTheSignCounterOnlyMovesForwardButMayStayAtZero(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	_, d := h.withPasskey(t)
	h.elevate(t, h.login(t, "owner@example.test"), d)
	d.cred.Counter = 5
	h.elevate(t, h.login(t, "owner@example.test"), d)
	for _, count := range []uint32{5, 3} {
		d.cred.Counter = count
		l := h.login(t, "owner@example.test")
		if _, err := h.svc.Verify(ctx, l.Cookie, l.Challenge, passkey(h.assertion(t, l.Cookie, d))); !errors.Is(err, dashboard.ErrUnauthenticated) {
			t.Fatalf("a counter of %d after 5: %v", count, err)
		}
	}
	d.cred.Counter = 6
	h.elevate(t, h.login(t, "owner@example.test"), d)
}

func TestAnAssertionWithoutPasskeyIsRefusedAndWrongOnesCountTowardTheLock(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.guarded(t)
	if _, err := h.svc.BeginAssertion(ctx, h.login(t, "owner@example.test").Cookie); !errors.Is(err, dashboard.ErrNoPasskey) {
		t.Fatalf("an assertion without passkey: %v", err)
	}
	h2 := newHarness(t)
	_, d := h2.withPasskey(t)
	stranger := newDevice()
	for range 5 {
		l := h2.login(t, "owner@example.test")
		if _, err := h2.svc.Verify(ctx, l.Cookie, l.Challenge, passkey(h2.assertion(t, l.Cookie, stranger))); !errors.Is(err, dashboard.ErrUnauthenticated) {
			t.Fatalf("an unknown key: %v", err)
		}
	}
	l := h2.login(t, "owner@example.test")
	var limited dashboard.RateLimited
	if _, err := h2.svc.Verify(ctx, l.Cookie, l.Challenge, passkey(h2.assertion(t, l.Cookie, d))); !errors.As(err, &limited) {
		t.Fatalf("the sixth attempt: %v", err)
	}
	if got := h2.events(t); !slices.Equal(got[len(got)-2:], []string{"mfa:failure", "login:success"}) {
		t.Fatalf("log %q", got)
	}
}

func TestAPasskeyIsRemovedFromAnElevatedSessionUnlessItIsTheLastFactor(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	elevated, d := h.withPasskey(t)
	keys, err := h.svc.ListPasskeys(ctx, elevated)
	if err != nil {
		t.Fatal(err)
	}
	first := keys[0].ID
	if err := h.svc.RemovePasskey(ctx, h.login(t, "owner@example.test").Cookie, first); !errors.Is(err, dashboard.ErrElevationRequired) {
		t.Fatalf("a session not elevated: %v", err)
	}
	if err := h.svc.RemovePasskey(ctx, elevated, first); !errors.Is(err, dashboard.ErrLastFactor) {
		t.Fatalf("the last factor: %v", err)
	}
	second := h.register(t, elevated, "Seconde", newDevice())
	h.activate(t, "other@example.test")
	otherDevice := newDevice()
	h.register(t, h.login(t, "other@example.test").Cookie, "Autre", otherDevice)
	otherElevated := h.elevate(t, h.login(t, "other@example.test"), otherDevice)
	if err := h.svc.RemovePasskey(ctx, otherElevated, first); !errors.Is(err, dashboard.ErrPasskeyUnknown) {
		t.Fatalf("the passkey of another operator: %v", err)
	}
	if err := h.svc.RemovePasskey(ctx, elevated, first); err != nil {
		t.Fatal(err)
	}
	if got := h.events(t); got[len(got)-1] != "factor_removed:success" {
		t.Fatalf("log %q", got)
	}
	if keys, err := h.svc.ListPasskeys(ctx, elevated); err != nil || len(keys) != 1 || keys[0].ID != second {
		t.Fatalf("passkeys %+v (%v)", keys, err)
	}
	l := h.login(t, "owner@example.test")
	if _, err := h.svc.Verify(ctx, l.Cookie, l.Challenge, passkey(h.assertion(t, l.Cookie, d))); !errors.Is(err, dashboard.ErrUnauthenticated) {
		t.Fatalf("the removed passkey elevated: %v", err)
	}
}

func TestAnAppBesideAPasskeyIsAddedFromAnElevatedSessionAndConfirmedOnlyThere(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	elevated, _ := h.withPasskey(t)
	if _, err := h.svc.EnrollTotp(ctx, h.login(t, "owner@example.test").Cookie, nil); !errors.Is(err, dashboard.ErrElevationRequired) {
		t.Fatalf("an app added from a session not elevated: %v", err)
	}
	e := h.enroll(t, elevated, nil)
	l := h.login(t, "owner@example.test")
	if _, err := h.svc.Verify(ctx, l.Cookie, l.Challenge, totp(code(t, e, h.step(t)))); !errors.Is(err, dashboard.ErrUnauthenticated) {
		t.Fatalf("the pending app of a passkey-guarded account elevated: %v", err)
	}
	if err := h.svc.ConfirmTotp(ctx, elevated, code(t, e, h.step(t))); err != nil {
		t.Fatal(err)
	}
	if c := h.current(t, elevated); !c.Totp || c.Passkeys != 1 {
		t.Fatalf("after the confirmation: %+v", c)
	}
	keys, err := h.svc.ListPasskeys(ctx, elevated)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.svc.RemovePasskey(ctx, elevated, keys[0].ID); err != nil {
		t.Fatalf("the passkey beside an app: %v", err)
	}
}

func TestAResetRemovesThePasskeysAndCeremoniesDieWithTheirSession(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	elevated, d := h.withPasskey(t)
	h.begin(t, elevated)
	if err := h.svc.Logout(ctx, elevated); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := h.owner.QueryRow(ctx, `SELECT count(*) FROM dashboard.webauthn_challenges`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("%d challenges outlive their session (%v)", rows, err)
	}
	h.exec(t, `UPDATE dashboard.access_links SET kind = 'reset', token_hash = NULL, sent_at = NULL, next_attempt_at = now()`)
	h.deliver(t)
	if err := h.svc.UseAccessLink(ctx, h.mail.token(t), password); err != nil {
		t.Fatal(err)
	}
	if got := h.events(t); got[len(got)-2] != "factor_removed:success" {
		t.Fatalf("log %q", got)
	}
	l := h.login(t, "owner@example.test")
	if c := h.current(t, l.Cookie); c.Passkeys != 0 {
		t.Fatalf("after the reset: %+v", c)
	}
	if _, err := h.svc.BeginAssertion(ctx, l.Cookie); !errors.Is(err, dashboard.ErrNoPasskey) {
		t.Fatalf("an assertion after the reset: %v", err)
	}
	h.register(t, l.Cookie, "Clé", d)
}

func TestASessionHoldsOneChallengePerPurpose(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	elevated, _ := h.withPasskey(t)
	if _, err := h.svc.BeginAssertion(ctx, elevated); err != nil {
		t.Fatal(err)
	}
	h.register(t, elevated, "Seconde", newDevice())
	var purposes []string
	rows, err := h.owner.Query(ctx, `SELECT purpose FROM dashboard.webauthn_challenges`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		purposes = append(purposes, p)
	}
	if !slices.Equal(purposes, []string{"assertion"}) {
		t.Fatalf("the registration took %q", purposes)
	}
}
