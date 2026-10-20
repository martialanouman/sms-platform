package dashboard_test

import (
	"context"
	"encoding/base32"
	"errors"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/martialanouman/sms-platform/internal/dashboard"
)

func (h *harness) step(t *testing.T) int64 {
	t.Helper()
	var step int64
	if err := h.owner.QueryRow(context.Background(), `SELECT floor(extract(epoch FROM now()) / 30)::bigint`).Scan(&step); err != nil {
		t.Fatal(err)
	}
	return step
}

func code(t *testing.T, e dashboard.TotpEnrollment, step int64) string {
	t.Helper()
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(e.Secret)
	if err != nil {
		t.Fatal(err)
	}
	return dashboard.TotpCode(secret, step)
}

func totp(c string) dashboard.Proof { return dashboard.Proof{Method: dashboard.MethodTotp, Code: c} }

func recovery(c string) dashboard.Proof {
	return dashboard.Proof{Method: dashboard.MethodRecoveryCode, Code: c}
}

func (h *harness) enroll(t *testing.T, cookie string, proof *dashboard.Proof) dashboard.TotpEnrollment {
	t.Helper()
	e, err := h.svc.EnrollTotp(context.Background(), cookie, proof)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func (h *harness) verify(t *testing.T, l dashboard.Login, p dashboard.Proof) string {
	t.Helper()
	cookie, err := h.svc.Verify(context.Background(), l.Cookie, l.Challenge, p)
	if err != nil {
		t.Fatalf("verify %s: %v", p.Method, err)
	}
	return cookie
}

func (h *harness) current(t *testing.T, cookie string) dashboard.Current {
	t.Helper()
	c, err := h.svc.Resolve(context.Background(), cookie)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (h *harness) events(t *testing.T) []string {
	t.Helper()
	var got []string
	for _, e := range h.log(t) {
		outcome := "failure"
		if e.Success {
			outcome = "success"
		}
		got = append(got, string(e.Event)+":"+outcome)
	}
	slices.Reverse(got)
	return got
}

func (h *harness) guarded(t *testing.T) dashboard.TotpEnrollment {
	t.Helper()
	h.activate(t, "owner@example.test")
	l := h.login(t, "owner@example.test")
	e := h.enroll(t, l.Cookie, nil)
	if _, err := h.svc.Verify(context.Background(), l.Cookie, l.Challenge, totp(code(t, e, h.step(t)-1))); err != nil {
		h.verify(t, l, totp(code(t, e, h.step(t))))
	}
	return e
}

func TestAFirstFactorIsConfirmedByItsFirstUse(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.activate(t, "owner@example.test")
	l := h.login(t, "owner@example.test")
	e := h.enroll(t, l.Cookie, nil)
	if secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(e.Secret); err != nil || len(secret) != 20 {
		t.Fatalf("secret of %d bytes (%v), want 20", len(secret), err)
	}
	u, err := url.Parse(e.URI)
	if err != nil || u.Query().Get("secret") != e.Secret || !strings.HasSuffix(u.Path, ":owner@example.test") {
		t.Fatalf("uri %q, secret %q", e.URI, e.Secret)
	}
	if len(e.RecoveryCodes) != 10 || len(slices.Compact(slices.Sorted(slices.Values(e.RecoveryCodes)))) != 10 {
		t.Fatalf("recovery codes %q", e.RecoveryCodes)
	}
	for _, c := range e.RecoveryCodes {
		if len(c) != 10 || strings.Trim(c, "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567") != "" {
			t.Fatalf("recovery code %q", c)
		}
	}
	if c := h.current(t, l.Cookie); c.Elevated || c.Totp || c.RecoveryCodesRemaining != 0 {
		t.Fatalf("before its first use: %+v", c)
	}
	elevated := h.verify(t, l, totp(code(t, e, h.step(t))))
	if elevated == l.Cookie {
		t.Fatal("the session token was not regenerated")
	}
	if _, err := h.svc.Resolve(ctx, l.Cookie); !errors.Is(err, dashboard.ErrUnauthenticated) {
		t.Fatalf("the cookie before elevation still opens the session: %v", err)
	}
	if c := h.current(t, elevated); !c.Elevated || !c.Totp || c.RecoveryCodesRemaining != 10 {
		t.Fatalf("after its first use: %+v", c)
	}
	if got := h.events(t); !slices.Equal(got[len(got)-2:], []string{"factor_enrolled:success", "mfa:success"}) {
		t.Fatalf("log %q", got)
	}
	for _, entry := range h.log(t) {
		for _, secret := range append([]string{e.Secret}, e.RecoveryCodes...) {
			if strings.Contains(entry.Target, secret) {
				t.Fatalf("the log carries a secret: %+v", entry)
			}
		}
	}
}

func TestAPendingFactorServesOnlyAnAccountWithoutAConfirmedOne(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	old := h.guarded(t)
	l := h.login(t, "owner@example.test")
	pending := h.enroll(t, l.Cookie, new(totp(code(t, old, h.step(t)))))
	l = h.login(t, "owner@example.test")
	if _, err := h.svc.Verify(ctx, l.Cookie, l.Challenge, totp(code(t, pending, h.step(t)))); !errors.Is(err, dashboard.ErrUnauthenticated) {
		t.Fatalf("the pending factor elevated a guarded account: %v", err)
	}
	h.verify(t, l, totp(code(t, old, h.step(t)+1)))
}

func TestATotpCodeIsSpentOnce(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	e := h.guarded(t)
	c := code(t, e, h.step(t))
	h.verify(t, h.login(t, "owner@example.test"), totp(c))
	l := h.login(t, "owner@example.test")
	if _, err := h.svc.Verify(ctx, l.Cookie, l.Challenge, totp(c)); !errors.Is(err, dashboard.ErrUnauthenticated) {
		t.Fatalf("a code was accepted twice: %v", err)
	}
	if got := h.events(t); got[len(got)-1] != "mfa:failure" {
		t.Fatalf("log %q", got)
	}
}

func TestARecoveryCodeElevatesOnceAndOnlyOnceItsFactorIsConfirmed(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.activate(t, "owner@example.test")
	l := h.login(t, "owner@example.test")
	e := h.enroll(t, l.Cookie, nil)
	if _, err := h.svc.Verify(ctx, l.Cookie, l.Challenge, recovery(e.RecoveryCodes[0])); !errors.Is(err, dashboard.ErrUnauthenticated) {
		t.Fatalf("a recovery code of a pending factor was accepted: %v", err)
	}
	h.verify(t, l, totp(code(t, e, h.step(t))))
	typed := strings.ToLower(e.RecoveryCodes[0][:5] + "-" + e.RecoveryCodes[0][5:] + " ")
	elevated := h.verify(t, h.login(t, "owner@example.test"), recovery(typed))
	if c := h.current(t, elevated); !c.Elevated || c.RecoveryCodesRemaining != 9 {
		t.Fatalf("after a recovery code: %+v", c)
	}
	l = h.login(t, "owner@example.test")
	if _, err := h.svc.Verify(ctx, l.Cookie, l.Challenge, recovery(e.RecoveryCodes[0])); !errors.Is(err, dashboard.ErrUnauthenticated) {
		t.Fatalf("a recovery code was accepted twice: %v", err)
	}
}

func TestTheChallengeIsTheSessionsOwnAndConsumedOnSuccessOnly(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	e := h.guarded(t)
	h.activate(t, "other@example.test")
	other := h.login(t, "other@example.test")
	l := h.login(t, "owner@example.test")
	for name, challenge := range map[string]string{"unknown": dashboard.NewToken(), "another operator's": other.Challenge, "empty": ""} {
		if _, err := h.svc.Verify(ctx, l.Cookie, challenge, totp(code(t, e, h.step(t)))); !errors.Is(err, dashboard.ErrUnauthenticated) {
			t.Errorf("%s challenge: %v", name, err)
		}
	}
	if _, err := h.svc.Verify(ctx, l.Cookie, l.Challenge, totp("0000000")); !errors.Is(err, dashboard.ErrUnauthenticated) {
		t.Fatalf("a wrong code: %v", err)
	}
	elevated := h.verify(t, l, totp(code(t, e, h.step(t))))
	if _, err := h.svc.Verify(ctx, elevated, l.Challenge, totp(code(t, e, h.step(t)+1))); !errors.Is(err, dashboard.ErrUnauthenticated) {
		t.Fatalf("a consumed challenge was accepted: %v", err)
	}
	l = h.login(t, "owner@example.test")
	h.exec(t, `UPDATE dashboard.sessions SET challenge_expires_at = now() - interval '1 second' WHERE challenge_hash IS NOT NULL`)
	if _, err := h.svc.Verify(ctx, l.Cookie, l.Challenge, totp(code(t, e, h.step(t)+1))); !errors.Is(err, dashboard.ErrUnauthenticated) {
		t.Fatalf("an expired challenge was accepted: %v", err)
	}
	if _, err := h.svc.Verify(ctx, "nope", l.Challenge, totp(code(t, e, h.step(t)+1))); !errors.Is(err, dashboard.ErrUnauthenticated) {
		t.Fatalf("no session: %v", err)
	}
	l = h.login(t, "owner@example.test")
	if _, err := h.svc.Verify(ctx, l.Cookie, l.Challenge, dashboard.Proof{Method: "webauthn", Code: code(t, e, h.step(t)+1)}); !errors.Is(err, dashboard.ErrUnauthenticated) {
		t.Fatalf("webauthn before 011b-3: %v", err)
	}
}

func TestFiveWrongCodesLockTheOperatorAcrossLogins(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	e := h.guarded(t)
	wrong := totp("9999999")
	for range 4 {
		l := h.login(t, "owner@example.test")
		if _, err := h.svc.Verify(ctx, l.Cookie, l.Challenge, wrong); !errors.Is(err, dashboard.ErrUnauthenticated) {
			t.Fatal(err)
		}
	}
	h.verify(t, h.login(t, "owner@example.test"), totp(code(t, e, h.step(t))))
	for range 5 {
		l := h.login(t, "owner@example.test")
		if _, err := h.svc.Verify(ctx, l.Cookie, l.Challenge, wrong); !errors.Is(err, dashboard.ErrUnauthenticated) {
			t.Fatalf("a success did not forget the failures: %v", err)
		}
	}
	l := h.login(t, "owner@example.test")
	var limited dashboard.RateLimited
	if _, err := h.svc.Verify(ctx, l.Cookie, l.Challenge, totp(code(t, e, h.step(t)+1))); !errors.As(err, &limited) || limited.RetryAfter <= 0 {
		t.Fatalf("the sixth attempt: %v", err)
	}
}

func TestReplacingAFactorWantsAProofOfTheOldOneThenAnElevatedConfirmation(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	old := h.guarded(t)
	l := h.login(t, "owner@example.test")
	if _, err := h.svc.EnrollTotp(ctx, l.Cookie, nil); !errors.Is(err, dashboard.ErrAlreadyEnrolled) {
		t.Fatalf("without a proof: %v", err)
	}
	wrong := totp("9999999")
	if _, err := h.svc.EnrollTotp(ctx, l.Cookie, &wrong); !errors.Is(err, dashboard.ErrReplacementRefused) {
		t.Fatalf("with a wrong proof: %v", err)
	}
	if got := h.events(t); got[len(got)-1] != "mfa:failure" {
		t.Fatalf("a refused proof is not logged: %q", got)
	}
	if err := h.svc.ConfirmTotp(ctx, l.Cookie, "123456"); !errors.Is(err, dashboard.ErrElevationRequired) {
		t.Fatalf("confirm on a session not elevated: %v", err)
	}
	next := h.enroll(t, l.Cookie, new(recovery(old.RecoveryCodes[0])))
	elevated := h.verify(t, l, totp(code(t, old, h.step(t))))
	if err := h.svc.ConfirmTotp(ctx, elevated, code(t, old, h.step(t)+1)); !errors.Is(err, dashboard.ErrCodeRefused) {
		t.Fatalf("confirm with a code of the old factor: %v", err)
	}
	if err := h.svc.ConfirmTotp(ctx, elevated, code(t, next, h.step(t))); err != nil {
		t.Fatal(err)
	}
	if got := h.events(t); !slices.Equal(got[len(got)-2:], []string{"factor_removed:success", "factor_enrolled:success"}) {
		t.Fatalf("log after the replacement %q", got)
	}
	if c := h.current(t, elevated); !c.Totp || c.RecoveryCodesRemaining != 10 {
		t.Fatalf("after the replacement: %+v", c)
	}
	if err := h.svc.ConfirmTotp(ctx, elevated, code(t, next, h.step(t)+1)); !errors.Is(err, dashboard.ErrNothingToConfirm) {
		t.Fatalf("nothing pending: %v", err)
	}
	l = h.login(t, "owner@example.test")
	for name, p := range map[string]dashboard.Proof{"old code": totp(code(t, old, h.step(t)+1)), "old recovery code": recovery(old.RecoveryCodes[1])} {
		if _, err := h.svc.Verify(ctx, l.Cookie, l.Challenge, p); !errors.Is(err, dashboard.ErrUnauthenticated) {
			t.Errorf("%s after the replacement: %v", name, err)
		}
	}
	h.verify(t, l, recovery(next.RecoveryCodes[0]))
}

func TestAResetRemovesEveryFactor(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	old := h.guarded(t)
	pending := h.enroll(t, h.login(t, "owner@example.test").Cookie, new(recovery(old.RecoveryCodes[0])))
	h.exec(t, `UPDATE dashboard.access_links SET kind = 'reset', token_hash = NULL, sent_at = NULL, next_attempt_at = now()`)
	h.deliver(t)
	if err := h.svc.UseAccessLink(ctx, h.mail.token(t), password); err != nil {
		t.Fatal(err)
	}
	if got := h.events(t); got[len(got)-1] != "access_link_used:success" || got[len(got)-2] != "factor_removed:success" {
		t.Fatalf("log %q", got)
	}
	l := h.login(t, "owner@example.test")
	if c := h.current(t, l.Cookie); c.Totp || c.RecoveryCodesRemaining != 0 {
		t.Fatalf("after the reset: %+v", c)
	}
	if _, err := h.svc.Verify(ctx, l.Cookie, l.Challenge, totp(code(t, pending, h.step(t)))); !errors.Is(err, dashboard.ErrUnauthenticated) {
		t.Fatalf("the application pending before the reset elevated: %v", err)
	}
	h.enroll(t, l.Cookie, nil)
}

func TestWrongProofsOfReplacementCountTowardTheLock(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	e := h.guarded(t)
	wrong := totp("9999999")
	l := h.login(t, "owner@example.test")
	for range 4 {
		if _, err := h.svc.EnrollTotp(ctx, l.Cookie, &wrong); !errors.Is(err, dashboard.ErrReplacementRefused) {
			t.Fatal(err)
		}
	}
	h.enroll(t, l.Cookie, new(recovery(e.RecoveryCodes[0])))
	for range 5 {
		if _, err := h.svc.EnrollTotp(ctx, l.Cookie, &wrong); !errors.Is(err, dashboard.ErrReplacementRefused) {
			t.Fatalf("a proof accepted did not forget the failures: %v", err)
		}
	}
	var limited dashboard.RateLimited
	if _, err := h.svc.EnrollTotp(ctx, l.Cookie, new(totp(code(t, e, h.step(t))))); !errors.As(err, &limited) {
		t.Fatalf("the sixth proof: %v", err)
	}
}

func TestOneCodeUsedByTwoSessionsAtOnceElevatesOne(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	e := h.guarded(t)
	sessions := []dashboard.Login{h.login(t, "owner@example.test"), h.login(t, "owner@example.test")}
	c := totp(code(t, e, h.step(t)))
	hold, err := h.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hold.Exec(ctx, `SELECT 1 FROM dashboard.totp_factors FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	errs := make([]error, len(sessions))
	var wg sync.WaitGroup
	for i, l := range sessions {
		wg.Go(func() {
			_, errs[i] = h.svc.Verify(ctx, l.Cookie, l.Challenge, c)
		})
	}
	for deadline := time.Now().Add(10 * time.Second); ; {
		var waiting int
		if err := hold.QueryRow(ctx, `SELECT count(DISTINCT pid) FROM pg_locks WHERE NOT granted`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting == len(sessions) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d verifications wait on the factor, want %d", waiting, len(sessions))
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := hold.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if accepted := len(errs) - len(slices.DeleteFunc(slices.Clone(errs), func(err error) bool { return err == nil })); accepted != 1 {
		t.Fatalf("accepted %d times: %v", accepted, errs)
	}
}

func TestASecondEnrolmentReplacesThePendingOneAndSurvivesANewLogin(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.activate(t, "owner@example.test")
	l := h.login(t, "owner@example.test")
	first := h.enroll(t, l.Cookie, nil)
	second := h.enroll(t, l.Cookie, nil)
	l = h.login(t, "owner@example.test")
	if _, err := h.svc.Verify(ctx, l.Cookie, l.Challenge, totp(code(t, first, h.step(t)))); !errors.Is(err, dashboard.ErrUnauthenticated) {
		t.Fatalf("the replaced enrolment elevated: %v", err)
	}
	if c := h.current(t, h.verify(t, l, totp(code(t, second, h.step(t))))); !c.Elevated || !c.Totp {
		t.Fatalf("the pending enrolment after a new login: %+v", c)
	}
}
