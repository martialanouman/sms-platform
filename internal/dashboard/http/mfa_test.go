package http_test

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"
)

func totpAt(t *testing.T, secret string, offset int64) string {
	t.Helper()
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		t.Fatal(err)
	}
	m := hmac.New(sha1.New, key)
	_ = binary.Write(m, binary.BigEndian, time.Now().Unix()/30+offset)
	sum := m.Sum(nil)
	o := sum[len(sum)-1] & 0x0f
	return fmt.Sprintf("%06d", (binary.BigEndian.Uint32(sum[o:])&0x7fffffff)%1_000_000)
}

type signedIn struct{ cookie, challenge string }

func (a *api) signIn(t *testing.T) signedIn {
	t.Helper()
	a.logins++
	got := a.do(t, call{method: "POST", path: "/api/auth/login", forwarded: "198.51.100." + strconv.Itoa(a.logins),
		body: `{"email":"owner@example.test","password":"Correct-horse-9"}`})
	if got.status != 200 || len(got.cookies) != 1 {
		t.Fatalf("login: %d %s", got.status, got.raw)
	}
	return signedIn{cookie: got.cookies[0].Value, challenge: got.body["challenge"].(string)}
}

func (a *api) verify(t *testing.T, s signedIn, method, code string) answer {
	t.Helper()
	return a.do(t, call{method: "POST", path: "/api/auth/mfa/verify", cookie: s.cookie,
		body: `{"challenge":"` + s.challenge + `","method":"` + method + `","code":"` + code + `"}`})
}

func factors(me answer) string {
	f, _ := me.body["secondFactors"].(map[string]any)
	return fmt.Sprintf("elevated=%v totp=%v codes=%v passkeys=%v", me.body["elevated"], f["totp"], f["recoveryCodesRemaining"], f["passkeys"])
}

func TestAnOperatorEnrollsAndElevatesByTheFirstCode(t *testing.T) {
	a := newAPI(t, netip.MustParsePrefix("127.0.0.0/8"))
	a.activate(t)
	s := a.signIn(t)
	enrolled := a.do(t, call{method: "POST", path: "/api/auth/mfa/totp/enroll", cookie: s.cookie, body: `{}`})
	secret, _ := enrolled.body["secret"].(string)
	codes, _ := enrolled.body["recoveryCodes"].([]any)
	if enrolled.status != 200 || secret == "" || len(codes) != 10 || !strings.HasPrefix(enrolled.body["otpauthUri"].(string), "otpauth://totp/") {
		t.Fatalf("enroll: %d %s", enrolled.status, enrolled.raw)
	}
	if enrolled.header.Get("Cache-Control") != "no-store" {
		t.Errorf("the enrolment may be cached: %q", enrolled.header.Get("Cache-Control"))
	}
	if got := factors(a.do(t, call{method: "GET", path: "/api/auth/me", cookie: s.cookie})); got != "elevated=false totp=false codes=0 passkeys=0" {
		t.Errorf("me before the first code: %s", got)
	}
	verified := a.verify(t, s, "totp", totpAt(t, secret, 0))
	if verified.status != 204 || len(verified.cookies) != 1 || verified.cookies[0].Value == s.cookie || !verified.cookies[0].HttpOnly ||
		!verified.cookies[0].Secure || verified.cookies[0].Name != "__Host-dashboard_session" {
		t.Fatalf("verify: %d %s %v", verified.status, verified.raw, verified.cookies)
	}
	if got := a.do(t, call{method: "GET", path: "/api/auth/me", cookie: s.cookie}); got.status != 401 {
		t.Errorf("the cookie before elevation: %d", got.status)
	}
	elevated := verified.cookies[0].Value
	if got := factors(a.do(t, call{method: "GET", path: "/api/auth/me", cookie: elevated})); got != "elevated=true totp=true codes=10 passkeys=0" {
		t.Errorf("me after the first code: %s", got)
	}
	if got := a.do(t, call{method: "POST", path: "/api/auth/mfa/totp/confirm", cookie: elevated, body: `{"code":"123456"}`}); got.status != 409 || got.code() != "mfa_nothing_to_confirm" {
		t.Errorf("confirm with nothing pending: %d %s", got.status, got.raw)
	}

	other := a.signIn(t)
	for name, tc := range map[string]struct {
		body, code string
		status     int
	}{
		"no proof":    {`{}`, "mfa_already_enrolled", 409},
		"wrong proof": {`{"method":"recovery_code","code":"AAAAAAAAAA"}`, "mfa_replacement_refused", 409},
		"half proof":  {`{"method":"totp"}`, "validation_error", 422},
	} {
		if got := a.do(t, call{method: "POST", path: "/api/auth/mfa/totp/enroll", cookie: other.cookie, body: tc.body}); got.status != tc.status || got.code() != tc.code {
			t.Errorf("replace with %s: %d %s", name, got.status, got.raw)
		}
	}
	if got := a.do(t, call{method: "POST", path: "/api/auth/mfa/totp/confirm", cookie: other.cookie, body: `{"code":"123456"}`}); got.status != 409 || got.code() != "mfa_elevation_required" {
		t.Errorf("confirm not elevated: %d %s", got.status, got.raw)
	}
	next := a.do(t, call{method: "POST", path: "/api/auth/mfa/totp/enroll", cookie: elevated, body: `{"method":"recovery_code","code":"` + codes[0].(string) + `"}`})
	if next.status != 200 {
		t.Fatalf("replace with a recovery code: %d %s", next.status, next.raw)
	}
	if got := a.do(t, call{method: "POST", path: "/api/auth/mfa/totp/confirm", cookie: elevated, body: `{"code":"` + totpAt(t, secret, 1) + `"}`}); got.status != 422 || got.code() != "mfa_code_refused" {
		t.Errorf("confirm with the old factor: %d %s", got.status, got.raw)
	}
	if got := a.do(t, call{method: "POST", path: "/api/auth/mfa/totp/confirm", cookie: elevated, body: `{"code":"` + totpAt(t, next.body["secret"].(string), 0) + `"}`}); got.status != 204 {
		t.Errorf("confirm: %d %s", got.status, got.raw)
	}
	if got := a.verify(t, other, "recovery_code", codes[1].(string)); got.status != 401 || got.code() != "unauthenticated" {
		t.Errorf("a recovery code of the replaced factor: %d %s", got.status, got.raw)
	}
}

func TestRefusedSecondFactorsShareOneBodyThenLock(t *testing.T) {
	a := newAPI(t, netip.MustParsePrefix("127.0.0.0/8"))
	a.activate(t)
	s := a.signIn(t)
	enrolled := a.do(t, call{method: "POST", path: "/api/auth/mfa/totp/enroll", cookie: s.cookie, body: `{}`})
	secret := enrolled.body["secret"].(string)
	first := a.verify(t, s, "totp", totpAt(t, secret, -1))
	if first.status != 204 {
		t.Fatalf("verify: %d %s", first.status, first.raw)
	}
	elevated := first.cookies[0].Value
	bodies := map[string]bool{}
	for _, method := range []string{"totp", "recovery_code", "webauthn", "totp", "recovery_code"} {
		got := a.verify(t, a.signIn(t), method, "99999999")
		if got.status != 401 || got.code() != "unauthenticated" {
			t.Fatalf("%s: %d %s", method, got.status, got.raw)
		}
		bodies[got.raw] = true
	}
	if got := a.do(t, call{method: "POST", path: "/api/auth/mfa/verify", cookie: s.cookie, body: `{"challenge":"` + strings.Repeat("a", 43) + `","method":"totp","code":"1"}`}); got.status != 401 {
		t.Fatalf("an unknown challenge: %d %s", got.status, got.raw)
	} else {
		bodies[got.raw] = true
	}
	if len(bodies) != 1 {
		t.Errorf("refusals tell their cause apart: %v", bodies)
	}
	got := a.verify(t, a.signIn(t), "totp", totpAt(t, secret, 0))
	retry, _ := strconv.Atoi(got.header.Get("Retry-After"))
	if got.status != 429 || got.code() != "rate_limited" || retry <= 0 || retry > 900 {
		t.Fatalf("the sixth attempt: %d %s retry %q", got.status, got.raw, got.header.Get("Retry-After"))
	}
	for path, body := range map[string]string{
		"/api/auth/mfa/totp/enroll":  `{"method":"totp","code":"` + totpAt(t, secret, 1) + `"}`,
		"/api/auth/mfa/totp/confirm": `{"code":"123456"}`,
	} {
		if got := a.do(t, call{method: "POST", path: path, cookie: elevated, body: body}); got.status != 429 || got.header.Get("Retry-After") == "" {
			t.Errorf("%s while locked: %d %s", path, got.status, got.raw)
		}
	}
	if got := a.do(t, call{method: "POST", path: "/api/auth/mfa/verify", body: `{"challenge":"` + strings.Repeat("a", 43) + `","method":"totp","code":"1"}`}); got.status != 401 {
		t.Errorf("without a session: %d %s", got.status, got.raw)
	}
}
