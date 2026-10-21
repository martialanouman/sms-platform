package http_test

import (
	"net/netip"
	"strings"
	"testing"

	"github.com/descope/virtualwebauthn"
)

type device struct {
	auth virtualwebauthn.Authenticator
	cred virtualwebauthn.Credential
}

func newDevice() *device {
	d := &device{auth: virtualwebauthn.NewAuthenticator(), cred: virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)}
	d.auth.AddCredential(d.cred)
	return d
}

var relyingParty = virtualwebauthn.RelyingParty{ID: "localhost", Name: "Tableau de bord SMS", Origin: origin}

func (a *api) registerPasskey(t *testing.T, cookie, name string, d *device) answer {
	t.Helper()
	begun := a.do(t, call{method: "POST", path: "/api/auth/mfa/webauthn/register/begin", cookie: cookie})
	if begun.status != 200 {
		return begun
	}
	o, err := virtualwebauthn.ParseAttestationOptions(begun.raw)
	if err != nil {
		t.Fatal(err)
	}
	attestation := virtualwebauthn.CreateAttestationResponse(relyingParty, d.auth, d.cred, *o)
	return a.do(t, call{method: "POST", path: "/api/auth/mfa/webauthn/register/finish", cookie: cookie,
		body: `{"name":"` + name + `","attestation":` + attestation + `}`})
}

func (a *api) assertPasskey(t *testing.T, s signedIn, d *device) answer {
	t.Helper()
	begun := a.do(t, call{method: "POST", path: "/api/auth/mfa/webauthn/assert/begin", cookie: s.cookie})
	if begun.status != 200 {
		t.Fatalf("assert begin: %d %s", begun.status, begun.raw)
	}
	o, err := virtualwebauthn.ParseAssertionOptions(begun.raw)
	if err != nil {
		t.Fatal(err)
	}
	assertion := virtualwebauthn.CreateAssertionResponse(relyingParty, d.auth, d.cred, *o)
	return a.do(t, call{method: "POST", path: "/api/auth/mfa/verify", cookie: s.cookie,
		body: `{"challenge":"` + s.challenge + `","method":"webauthn","assertion":` + assertion + `}`})
}

func TestAPasskeyIsRegisteredUsedListedAndRemoved(t *testing.T) {
	a := newAPI(t, netip.MustParsePrefix("127.0.0.0/8"))
	a.activate(t)
	s := a.signIn(t)
	d := newDevice()
	registered := a.registerPasskey(t, s.cookie, "Clé USB", d)
	first, _ := registered.body["id"].(string)
	if registered.status != 200 || first == "" {
		t.Fatalf("register finish: %d %s", registered.status, registered.raw)
	}
	if got := factors(a.do(t, call{method: "GET", path: "/api/auth/me", cookie: s.cookie})); got != "elevated=false totp=false codes=0 passkeys=1" {
		t.Errorf("me after the registration: %s", got)
	}
	if got := a.registerPasskey(t, a.signIn(t).cookie, "Autre", newDevice()); got.status != 409 || got.code() != "mfa_elevation_required" {
		t.Errorf("a registration from a session not elevated: %d %s", got.status, got.raw)
	}
	if got := a.do(t, call{method: "POST", path: "/api/auth/mfa/totp/enroll", cookie: s.cookie, body: `{}`}); got.status != 409 || got.code() != "mfa_elevation_required" {
		t.Errorf("an app added from a session not elevated: %d %s", got.status, got.raw)
	}
	verified := a.assertPasskey(t, s, d)
	if verified.status != 204 || len(verified.cookies) != 1 || verified.cookies[0].Value == s.cookie {
		t.Fatalf("verify by passkey: %d %s", verified.status, verified.raw)
	}
	elevated := verified.cookies[0].Value
	if got := factors(a.do(t, call{method: "GET", path: "/api/auth/me", cookie: elevated})); got != "elevated=true totp=false codes=0 passkeys=1" {
		t.Errorf("me after the assertion: %s", got)
	}
	if got := a.do(t, call{method: "DELETE", path: "/api/auth/mfa/webauthn/passkeys/" + first, cookie: elevated}); got.status != 409 || got.code() != "mfa_last_factor" {
		t.Errorf("remove the last factor: %d %s", got.status, got.raw)
	}
	if got := a.registerPasskey(t, elevated, "Seconde", newDevice()); got.status != 200 {
		t.Fatalf("a second passkey: %d %s", got.status, got.raw)
	}
	listed := a.do(t, call{method: "GET", path: "/api/auth/mfa/webauthn/passkeys", cookie: elevated})
	if listed.status != 200 || listed.raw[0] != '[' || !strings.Contains(listed.raw, `"name":"Clé USB"`) || !strings.Contains(listed.raw, `"name":"Seconde"`) || !strings.Contains(listed.raw, `"createdAt"`) {
		t.Errorf("list: %d %s", listed.status, listed.raw)
	}
	if got := a.do(t, call{method: "DELETE", path: "/api/auth/mfa/webauthn/passkeys/01a159b0-0000-7000-8000-000000000009", cookie: elevated}); got.status != 404 || got.code() != "not_found" {
		t.Errorf("remove an unknown passkey: %d %s", got.status, got.raw)
	}
	if got := a.do(t, call{method: "DELETE", path: "/api/auth/mfa/webauthn/passkeys/" + first, cookie: a.signIn(t).cookie}); got.status != 409 || got.code() != "mfa_elevation_required" {
		t.Errorf("remove from a session not elevated: %d %s", got.status, got.raw)
	}
	if got := a.do(t, call{method: "DELETE", path: "/api/auth/mfa/webauthn/passkeys/" + first, cookie: elevated}); got.status != 204 {
		t.Errorf("remove: %d %s", got.status, got.raw)
	}
	if got := a.assertPasskey(t, a.signIn(t), d); got.status != 401 || got.code() != "unauthenticated" {
		t.Errorf("the removed passkey: %d %s", got.status, got.raw)
	}
}

func TestARefusedRegistrationAndAnAccountWithoutPasskey(t *testing.T) {
	a := newAPI(t, netip.MustParsePrefix("127.0.0.0/8"))
	a.activate(t)
	s := a.signIn(t)
	if got := a.do(t, call{method: "POST", path: "/api/auth/mfa/webauthn/assert/begin", cookie: s.cookie}); got.status != 404 || got.code() != "not_found" {
		t.Errorf("an assertion without passkey: %d %s", got.status, got.raw)
	}
	if got := a.do(t, call{method: "POST", path: "/api/auth/mfa/webauthn/register/finish", cookie: s.cookie,
		body: `{"name":"Clé","attestation":{"id":"x"}}`}); got.status != 422 || got.code() != "webauthn_ceremony_refused" {
		t.Errorf("a finish without ceremony: %d %s", got.status, got.raw)
	}
	if got := a.do(t, call{method: "GET", path: "/api/auth/mfa/webauthn/passkeys", cookie: s.cookie}); got.status != 200 || got.raw != "[]\n" {
		t.Errorf("an empty list: %d %q", got.status, got.raw)
	}
	if got := a.do(t, call{method: "GET", path: "/api/auth/mfa/webauthn/passkeys"}); got.status != 401 {
		t.Errorf("a list without session: %d %s", got.status, got.raw)
	}
}
