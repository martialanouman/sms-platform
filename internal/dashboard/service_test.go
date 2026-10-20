package dashboard_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/martialanouman/sms-platform/internal/dashboard"
	"github.com/martialanouman/sms-platform/internal/dashboard/postgres"
	"github.com/martialanouman/sms-platform/internal/platform/pgtest"
	"github.com/martialanouman/sms-platform/migrations"
)

const publicURL = "https://dashboard.example.test"

type message struct{ to, subject, body string }

type mailbox struct {
	mu   sync.Mutex
	sent []message
	fail error
}

func (m *mailbox) Send(_ context.Context, to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return m.fail
	}
	m.sent = append(m.sent, message{to, subject, body})
	return nil
}

func (m *mailbox) last(t *testing.T) message {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sent) == 0 {
		t.Fatal("no message sent")
	}
	return m.sent[len(m.sent)-1]
}

var linkToken = regexp.MustCompile(`/access#([A-Za-z0-9_-]{43})\b`)

func (m *mailbox) token(t *testing.T) string {
	t.Helper()
	got := linkToken.FindStringSubmatch(m.last(t).body)
	if got == nil {
		t.Fatalf("no link in %q", m.last(t).body)
	}
	return got[1]
}

type harness struct {
	svc   dashboard.Service
	mail  *mailbox
	owner *pgx.Conn
	ip    byte
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	pg := pgtest.ControlPlane(t).In("dashboard")
	migrator, err := pgx.Connect(context.Background(), pg.As("migrator"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = migrator.Close(context.Background()) })
	if _, err := migrator.Exec(context.Background(), string(migrations.Grants("dashboard"))); err != nil {
		t.Fatal(err)
	}
	if _, err := migrator.Exec(context.Background(), `ALTER ROLE dashboard PASSWORD 'dashboard'`); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(context.Background(), pg.As("dashboard"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	k, err := dashboard.NewKeys(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	totp, err := dashboard.NewTotpKey(bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	mail := &mailbox{}
	return &harness{
		svc: dashboard.Service{Store: postgres.New(pool), Hasher: dashboard.NewHasher(), Keys: k, Totp: totp, Mailer: mail,
			PublicURL: publicURL},
		mail:  mail,
		owner: migrator,
	}
}

func (h *harness) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := h.owner.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func (h *harness) source() netip.Addr {
	h.ip++
	return netip.AddrFrom4([4]byte{198, 51, 100, h.ip})
}

func (h *harness) log(t *testing.T) []dashboard.Entry {
	t.Helper()
	var all []dashboard.Entry
	cursor := ""
	for {
		page, next, err := h.svc.AccessLog(context.Background(), cursor)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, page...)
		if next == "" {
			return all
		}
		cursor = next
	}
}

func (h *harness) operatorID(t *testing.T, email string) string {
	t.Helper()
	var id string
	if err := h.owner.QueryRow(context.Background(), `SELECT id::text FROM dashboard.operators WHERE lower(email) = lower($1)`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (h *harness) deliver(t *testing.T) {
	t.Helper()
	if sent, err := h.svc.DeliverNext(context.Background()); err != nil || !sent {
		t.Fatalf("deliver = %v, %v; want a message sent", sent, err)
	}
}

const password = "Correct-horse-9"

func (h *harness) activate(t *testing.T, email string) {
	t.Helper()
	ctx := context.Background()
	if err := h.svc.Bootstrap(ctx, email); err != nil {
		t.Fatal(err)
	}
	h.deliver(t)
	if err := h.svc.UseAccessLink(ctx, h.mail.token(t), password); err != nil {
		t.Fatal(err)
	}
}

func (h *harness) login(t *testing.T, email string) dashboard.Login {
	t.Helper()
	l, err := h.svc.Login(context.Background(), email, password, h.source())
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestBootstrapCreatesTheOperatorOnceAndQueuesOneActivation(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for _, email := range []string{"Owner@Example.test", " owner@example.test", "OWNER@example.test "} {
		wg.Go(func() {
			if err := h.svc.Bootstrap(ctx, email); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	var operators, links int
	var email, name, kind string
	if err := h.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM dashboard.operators), (SELECT count(*) FROM dashboard.access_links),
		o.email, o.display_name, l.kind FROM dashboard.operators o JOIN dashboard.access_links l ON l.operator_id = o.id`).
		Scan(&operators, &links, &email, &name, &kind); err != nil {
		t.Fatal(err)
	}
	if operators != 1 || links != 1 || email != "owner@example.test" || name != email || kind != "activation" {
		t.Fatalf("%d operators, %d links, %q named %q, link %q", operators, links, email, name, kind)
	}
	id := h.operatorID(t, email)
	if got := h.log(t); len(got) != 1 || got[0].Event != dashboard.EventAccessLinkRequested || got[0].Actor != "" || got[0].Target != id || !got[0].Success {
		t.Fatalf("log %+v, want one access_link_requested targeting %s", got, id)
	}
	if err := h.svc.Bootstrap(ctx, "owner@example.test"); err != nil {
		t.Fatal(err)
	}
	h.deliver(t)
	if sent, err := h.svc.DeliverNext(ctx); err != nil || sent {
		t.Fatalf("a second delivery: %v, %v; want nothing left", sent, err)
	}
}

func TestDeliveryMailsTheLinkThenBacksOffOnFailure(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.svc.Bootstrap(ctx, "owner@example.test"); err != nil {
		t.Fatal(err)
	}
	refused := errors.New("relay refused")
	h.mail.fail = refused
	for attempt, wait := range []int{30, 60, 120} {
		if sent, err := h.svc.DeliverNext(ctx); !sent || !errors.Is(err, refused) {
			t.Fatalf("attempt %d: %v, %v; want the failure", attempt, sent, err)
		}
		var attempts int
		var delay float64
		if err := h.owner.QueryRow(ctx, `SELECT attempts, extract(epoch FROM next_attempt_at - now()) FROM dashboard.access_links`).Scan(&attempts, &delay); err != nil {
			t.Fatal(err)
		}
		if attempts != attempt+1 || delay < float64(wait)-5 || delay > float64(wait) {
			t.Fatalf("after failure %d: %d attempts, next in %.0f s; want %d s", attempt+1, attempts, delay, wait)
		}
		if sent, err := h.svc.DeliverNext(ctx); sent || err != nil {
			t.Fatalf("retried before its time: %v, %v", sent, err)
		}
		h.exec(t, `UPDATE dashboard.access_links SET next_attempt_at = now()`)
	}
	h.exec(t, `UPDATE dashboard.access_links SET attempts = 5, next_attempt_at = now()`)
	_, _ = h.svc.DeliverNext(ctx)
	var ceiling float64
	if err := h.owner.QueryRow(ctx, `SELECT extract(epoch FROM next_attempt_at - now()) FROM dashboard.access_links`).Scan(&ceiling); err != nil || ceiling < 595 || ceiling > 600 {
		t.Fatalf("sixth failure retried in %.0f s (%v), want the 10 min ceiling", ceiling, err)
	}
	h.exec(t, `UPDATE dashboard.access_links SET attempts = 9, next_attempt_at = now()`)
	if sent, _ := h.svc.DeliverNext(ctx); !sent {
		t.Fatal("the tenth attempt was not made")
	}
	h.exec(t, `UPDATE dashboard.access_links SET next_attempt_at = now()`)
	if sent, err := h.svc.DeliverNext(ctx); sent || err != nil {
		t.Fatalf("an eleventh attempt: %v, %v", sent, err)
	}
	h.exec(t, `UPDATE dashboard.access_links SET attempts = 9, next_attempt_at = now()`)
	h.mail.fail = nil
	h.deliver(t)
	m := h.mail.last(t)
	token := h.mail.token(t)
	if m.to != "owner@example.test" || m.subject != "Activation de votre compte" || !strings.Contains(m.body, publicURL+"/access#"+token) {
		t.Fatalf("message %+v", m)
	}
	var stored []byte
	var ttl float64
	if err := h.owner.QueryRow(ctx, `SELECT token_hash, extract(epoch FROM expires_at - sent_at) FROM dashboard.access_links`).Scan(&stored, &ttl); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, dashboard.Digest(token)) || ttl != 72*3600 {
		t.Fatalf("stored %x for a token of digest %x, valid %.0f s; want 72 h", stored, dashboard.Digest(token), ttl)
	}
}

func TestAnAccessLinkSetsThePasswordOnceAndOnlyWhileValid(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.svc.Bootstrap(ctx, "owner@example.test"); err != nil {
		t.Fatal(err)
	}
	h.deliver(t)
	token := h.mail.token(t)
	var policy dashboard.PolicyError
	if err := h.svc.UseAccessLink(ctx, token, "short"); !errors.As(err, &policy) || len(policy.Missing) != 4 {
		t.Fatalf("weak password: %v", err)
	}
	if err := h.svc.UseAccessLink(ctx, "unknown"+token[7:], "short"); !errors.Is(err, dashboard.ErrLinkInvalid) {
		t.Fatalf("unknown link judged after the policy: %v", err)
	}
	busy := h.svc
	busy.Hasher = dashboard.NewHasher()
	release := dashboard.HoldEverySlot(busy.Hasher)
	err := busy.UseAccessLink(ctx, token, password)
	release()
	if !errors.Is(err, dashboard.ErrBusy) {
		t.Fatalf("every slot held: %v, want ErrBusy", err)
	}
	if err := h.svc.UseAccessLink(ctx, token, password); err != nil {
		t.Fatalf("the link did not survive a refused password and a busy hasher: %v", err)
	}
	if err := h.svc.UseAccessLink(ctx, token, password); !errors.Is(err, dashboard.ErrLinkInvalid) {
		t.Fatalf("a spent link: %v", err)
	}
	id := h.operatorID(t, "owner@example.test")
	if got := h.log(t); got[0].Event != dashboard.EventAccessLinkUsed || got[0].Actor != id || !got[0].Success {
		t.Fatalf("last log line %+v", got[0])
	}
	if _, err := h.svc.Login(ctx, "owner@example.test", password, h.source()); err != nil {
		t.Fatalf("login with the password set: %v", err)
	}

	for name, spoil := range map[string]string{
		"expired":  `UPDATE dashboard.access_links SET expires_at = now() - interval '1 second'`,
		"replaced": `UPDATE dashboard.access_links SET token_hash = NULL, sent_at = NULL, attempts = 0, next_attempt_at = now()`,
		"disabled": `UPDATE dashboard.operators SET status = 'disabled'`,
	} {
		t.Run(name, func(t *testing.T) {
			h.exec(t, `UPDATE dashboard.operators SET status = 'active'`)
			h.exec(t, `UPDATE dashboard.access_links SET token_hash = NULL, sent_at = NULL, attempts = 0, next_attempt_at = now()`)
			h.deliver(t)
			stale := h.mail.token(t)
			h.exec(t, spoil)
			if name == "replaced" {
				h.deliver(t)
			}
			if err := h.svc.UseAccessLink(ctx, stale, "short"); !errors.Is(err, dashboard.ErrLinkInvalid) {
				t.Fatalf("%s link judged after the policy: %v", name, err)
			}
			if err := h.svc.UseAccessLink(ctx, stale, password); !errors.Is(err, dashboard.ErrLinkInvalid) {
				t.Fatalf("%s link: %v, want ErrLinkInvalid", name, err)
			}
		})
	}
}

func TestAResetLinkClosesEverySession(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.activate(t, "owner@example.test")
	l := h.login(t, "owner@example.test")
	h.exec(t, `UPDATE dashboard.access_links SET kind = 'reset', token_hash = NULL, sent_at = NULL, next_attempt_at = now()`)
	h.deliver(t)
	if !strings.Contains(h.mail.last(t).subject, "Réinitialisation") {
		t.Errorf("subject %q", h.mail.last(t).subject)
	}
	var ttl float64
	if err := h.owner.QueryRow(ctx, `SELECT extract(epoch FROM expires_at - sent_at) FROM dashboard.access_links`).Scan(&ttl); err != nil || ttl != 3600 {
		t.Fatalf("reset link valid %.0f s (%v), want 1 h", ttl, err)
	}
	if err := h.svc.UseAccessLink(ctx, h.mail.token(t), "Another-horse-8"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Resolve(ctx, l.Cookie); !errors.Is(err, dashboard.ErrUnauthenticated) {
		t.Fatalf("a session survived the reset: %v", err)
	}
	for _, e := range h.log(t) {
		if e.Event == dashboard.EventFactorRemoved {
			t.Fatal("a reset without factor logged a removal")
		}
	}
}

func TestLoginRefusesAlikeAndLogsWhoItCan(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.activate(t, "owner@example.test")
	h.exec(t, `INSERT INTO dashboard.operators (email, display_name) VALUES ('pending@example.test', 'p')`)
	h.exec(t, `INSERT INTO dashboard.operators (email, display_name, password_hash, status)
		SELECT 'gone@example.test', 'g', password_hash, 'disabled' FROM dashboard.operators WHERE email = 'owner@example.test'`)
	owner := h.operatorID(t, "owner@example.test")
	for email, actor := range map[string]string{
		"nobody@example.test":  "",
		"pending@example.test": h.operatorID(t, "pending@example.test"),
		"gone@example.test":    h.operatorID(t, "gone@example.test"),
	} {
		if _, err := h.svc.Login(ctx, email, password, h.source()); !errors.Is(err, dashboard.ErrUnauthenticated) {
			t.Errorf("%s: %v, want ErrUnauthenticated", email, err)
		}
		if got := h.log(t)[0]; got.Event != dashboard.EventLogin || got.Success || got.Actor != actor {
			t.Errorf("%s: logged %+v, want a failed login by %q", email, got, actor)
		}
	}
	if _, err := h.svc.Login(ctx, "Owner@Example.test ", "Correct-horse-8", h.source()); !errors.Is(err, dashboard.ErrUnauthenticated) {
		t.Errorf("wrong password: %v", err)
	}
	if got := h.log(t)[0]; got.Event != dashboard.EventLogin || got.Success || got.Actor != owner {
		t.Errorf("wrong password logged %+v", got)
	}
	l, err := h.svc.Login(ctx, "Owner@Example.test ", password, h.source())
	if err != nil {
		t.Fatal(err)
	}
	if got := h.log(t)[0]; got.Event != dashboard.EventLogin || !got.Success || got.Actor != owner {
		t.Errorf("login logged %+v", got)
	}
	if len(l.Challenge) < 43 || time.Until(l.ChallengeExpiresAt) < 4*time.Minute || time.Until(l.ChallengeExpiresAt) > 5*time.Minute {
		t.Errorf("challenge %q until %v", l.Challenge, l.ChallengeExpiresAt)
	}
	var challenge []byte
	if err := h.owner.QueryRow(ctx, `SELECT challenge_hash FROM dashboard.sessions`).Scan(&challenge); err != nil || !bytes.Equal(challenge, dashboard.Digest(l.Challenge)) {
		t.Errorf("challenge stored %x (%v), want its digest", challenge, err)
	}
	var sessionID string
	if err := h.owner.QueryRow(ctx, `SELECT id::text FROM dashboard.sessions`).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	s, err := h.svc.Resolve(ctx, l.Cookie)
	if err != nil || s.SessionID != sessionID || s.OperatorID != owner || s.Email != "owner@example.test" || s.DisplayName != "owner@example.test" || time.Until(s.ExpiresAt) < 12*time.Hour-time.Minute || time.Until(s.ExpiresAt) > 12*time.Hour {
		t.Fatalf("session %+v, %v", s, err)
	}
}

func TestFiveFailuresLockAnEmailOrASourceForFifteenMinutes(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.activate(t, "owner@example.test")
	locked := func(err error) time.Duration {
		var limited dashboard.RateLimited
		if !errors.As(err, &limited) {
			return 0
		}
		return limited.RetryAfter
	}
	for _, email := range []string{"owner@example.test", "Owner@example.test", "OWNER@EXAMPLE.TEST", " owner@example.test", "owner@Example.test"} {
		if _, err := h.svc.Login(ctx, email, "wrong", h.source()); locked(err) != 0 {
			t.Fatalf("locked before five failures: %v", err)
		}
	}
	retry := locked(func() error { _, err := h.svc.Login(ctx, "owner@example.test", password, h.source()); return err }())
	if retry < 14*time.Minute || retry > 15*time.Minute {
		t.Fatalf("sixth attempt with the right password: retry after %v, want under 15 min", retry)
	}
	before := len(h.log(t))
	h.exec(t, `UPDATE dashboard.login_attempt_counters SET last_failure_at = now() - interval '14 minutes 59 seconds'`)
	if retry := locked(func() error { _, err := h.svc.Login(ctx, "owner@example.test", password, h.source()); return err }()); retry < time.Second || retry > 2*time.Second {
		t.Fatalf("one second left: retry after %v, want 1 s", retry)
	}
	if len(h.log(t)) != before {
		t.Error("a refused attempt was logged")
	}
	h.exec(t, `UPDATE dashboard.login_attempt_counters SET last_failure_at = now() - interval '15 minutes'`)
	for range 2 {
		if _, err := h.svc.Login(ctx, "owner@example.test", "wrong", h.source()); locked(err) != 0 {
			t.Fatalf("a window passed did not start a new count: %v", err)
		}
	}
	h.login(t, "owner@example.test")

	source := h.source()
	for i := range 5 {
		_, _ = h.svc.Login(ctx, "other"+string(rune('a'+i))+"@example.test", "wrong", source)
	}
	if _, err := h.svc.Login(ctx, "owner@example.test", password, source); locked(err) == 0 {
		t.Fatalf("a source after five attempts on five emails: %v, want locked", err)
	}
}

func TestASuccessForgetsTheFailuresOfItsEmail(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.activate(t, "owner@example.test")
	for range 2 {
		for range 4 {
			_, _ = h.svc.Login(ctx, "owner@example.test", "wrong", h.source())
		}
		h.login(t, "owner@example.test")
	}
}

func TestABusyHasherRefusesButCountsTheAttempt(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.activate(t, "owner@example.test")
	busy := dashboard.NewHasher()
	release := dashboard.HoldEverySlot(busy)
	h.svc.Hasher = busy
	for range 5 {
		if _, err := h.svc.Login(ctx, "owner@example.test", password, h.source()); !errors.Is(err, dashboard.ErrBusy) {
			t.Fatalf("login with every slot held: %v, want ErrBusy", err)
		}
	}
	if _, err := h.svc.Login(ctx, "nobody@example.test", password, h.source()); !errors.Is(err, dashboard.ErrBusy) {
		t.Fatalf("an unknown email with every slot held: %v, want ErrBusy, as long as a known one", err)
	}
	release()
	var limited dashboard.RateLimited
	if _, err := h.svc.Login(ctx, "owner@example.test", password, h.source()); !errors.As(err, &limited) {
		t.Fatalf("five busy attempts did not count: %v", err)
	}
}

func TestASessionEndsAfterFifteenIdleMinutesOrTwelveHours(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.activate(t, "owner@example.test")
	owner := h.operatorID(t, "owner@example.test")
	for name, tc := range map[string]struct {
		spoil string
		live  bool
	}{
		"14 idle minutes": {`UPDATE dashboard.sessions SET last_seen_at = now() - interval '14 minutes 50 seconds'`, true},
		"15 idle minutes": {`UPDATE dashboard.sessions SET last_seen_at = now() - interval '15 minutes'`, false},
		"lifetime spent": {`UPDATE dashboard.sessions SET created_at = now() - interval '13 hours',
			expires_at = now() - interval '1 second'`, false},
		"account disabled": {`UPDATE dashboard.operators SET status = 'disabled'`, false},
	} {
		t.Run(name, func(t *testing.T) {
			h.exec(t, `UPDATE dashboard.operators SET status = 'active'`)
			l := h.login(t, "owner@example.test")
			h.exec(t, tc.spoil+` WHERE true`)
			_, err := h.svc.Resolve(ctx, l.Cookie)
			if tc.live != (err == nil) || (!tc.live && !errors.Is(err, dashboard.ErrUnauthenticated)) {
				t.Fatalf("resolve = %v, want live %v", err, tc.live)
			}
			if tc.live {
				return
			}
			if got := h.log(t)[0]; got.Event != dashboard.EventSessionRefused || got.Actor != owner || got.Success {
				t.Errorf("logged %+v, want session_refused", got)
			}
			if err := h.svc.Activity(ctx, l.Cookie); !errors.Is(err, dashboard.ErrUnauthenticated) {
				t.Errorf("activity on an ended session = %v", err)
			}
		})
	}
}

func TestOnlyAnActivityExtendsASession(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.activate(t, "owner@example.test")
	l := h.login(t, "owner@example.test")
	h.exec(t, `UPDATE dashboard.sessions SET last_seen_at = now() - interval '10 minutes'`)
	idle := func() float64 {
		var s float64
		if err := h.owner.QueryRow(ctx, `SELECT extract(epoch FROM now() - last_seen_at) FROM dashboard.sessions`).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if _, err := h.svc.Resolve(ctx, l.Cookie); err != nil || idle() < 600 {
		t.Fatalf("resolve extended the session (idle %.0f s, %v)", idle(), err)
	}
	if err := h.svc.Activity(ctx, l.Cookie); err != nil || idle() > 5 {
		t.Fatalf("activity did not extend the session (idle %.0f s, %v)", idle(), err)
	}
}

func TestACookieWithoutItsSealIsUnknownAndUnlogged(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.activate(t, "owner@example.test")
	l := h.login(t, "owner@example.test")
	before := len(h.log(t))
	other, err := dashboard.NewKeys(bytes.Repeat([]byte{8}, 32))
	if err != nil {
		t.Fatal(err)
	}
	token, _ := h.svc.Keys.Open(l.Cookie)
	for _, cookie := range []string{"", token, other.Seal(token), h.svc.Keys.Seal(dashboard.NewToken())} {
		if _, err := h.svc.Resolve(ctx, cookie); !errors.Is(err, dashboard.ErrUnauthenticated) {
			t.Errorf("%q: %v", cookie, err)
		}
		if err := h.svc.Logout(ctx, cookie); err != nil {
			t.Errorf("logout %q: %v", cookie, err)
		}
	}
	if len(h.log(t)) != before {
		t.Errorf("an unknown cookie was logged")
	}
}

func TestLogoutClosesTheSessionAndLogsIt(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.activate(t, "owner@example.test")
	l := h.login(t, "owner@example.test")
	if err := h.svc.Logout(ctx, l.Cookie); err != nil {
		t.Fatal(err)
	}
	if got := h.log(t)[0]; got.Event != dashboard.EventLogout || got.Actor != h.operatorID(t, "owner@example.test") || !got.Success {
		t.Errorf("logged %+v", got)
	}
	if _, err := h.svc.Resolve(ctx, l.Cookie); !errors.Is(err, dashboard.ErrUnauthenticated) {
		t.Errorf("resolve after logout: %v", err)
	}
	if got := h.log(t)[0]; got.Event != dashboard.EventLogout {
		t.Errorf("a closed session was refused and logged: %+v", got)
	}
}

func TestTheAccessLogPagesNewestFirst(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.activate(t, "owner@example.test")
	for range 120 {
		_, _ = h.svc.Login(ctx, "nobody@example.test", "x", h.source())
		h.exec(t, `DELETE FROM dashboard.login_attempt_counters`)
	}
	page, next, err := h.svc.AccessLog(ctx, "")
	if err != nil || len(page) != 50 || next == "" {
		t.Fatalf("first page: %d entries, next %q, %v", len(page), next, err)
	}
	all := h.log(t)
	if len(all) != 122 {
		t.Fatalf("%d entries, want 122", len(all))
	}
	if !slices.IsSortedFunc(all, func(a, b dashboard.Entry) int {
		if c := b.At.Compare(a.At); c != 0 {
			return c
		}
		return strings.Compare(b.ID, a.ID)
	}) || all[121].Event != dashboard.EventAccessLinkRequested {
		t.Fatal("the log is not newest first")
	}
	encode := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	for _, bad := range []string{"x", "eyJ9", strings.Repeat("A", 300), encode("abc " + all[0].ID), encode("123 " + strings.Repeat("z", 36)),
		encode("123 " + strings.ReplaceAll(all[0].ID, "-", "0")), encode("123 " + all[0].ID + "0"), encode("123 0" + all[0].ID)} {
		if _, _, err := h.svc.AccessLog(ctx, bad); !errors.Is(err, dashboard.ErrBadCursor) {
			t.Errorf("cursor %q: %v", bad, err)
		}
	}
}

func TestPurgeDropsEndedSessionsAndIdleCounters(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.activate(t, "owner@example.test")
	for range 3 {
		h.login(t, "owner@example.test")
	}
	_, _ = h.svc.Login(ctx, "a@example.test", "x", h.source())
	_, _ = h.svc.Login(ctx, "b@example.test", "x", h.source())
	h.exec(t, `UPDATE dashboard.sessions SET last_seen_at = now() - interval '15 minutes'
		WHERE id = (SELECT id FROM dashboard.sessions ORDER BY id LIMIT 1)`)
	h.exec(t, `UPDATE dashboard.sessions SET created_at = now() - interval '13 hours', expires_at = now()
		WHERE id = (SELECT id FROM dashboard.sessions ORDER BY id DESC LIMIT 1)`)
	h.exec(t, `UPDATE dashboard.login_attempt_counters SET last_failure_at = now() - interval '15 minutes'
		WHERE subject = (SELECT subject FROM dashboard.login_attempt_counters ORDER BY subject LIMIT 1)`)
	var sessions, counters int
	if err := h.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM dashboard.sessions), (SELECT count(*) FROM dashboard.login_attempt_counters)`).Scan(&sessions, &counters); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.Purge(ctx); err != nil {
		t.Fatal(err)
	}
	var leftSessions, leftCounters int
	if err := h.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM dashboard.sessions), (SELECT count(*) FROM dashboard.login_attempt_counters)`).Scan(&leftSessions, &leftCounters); err != nil {
		t.Fatal(err)
	}
	if leftSessions != sessions-2 || leftCounters != counters-1 {
		t.Fatalf("sessions %d → %d, counters %d → %d; want two and one fewer", sessions, leftSessions, counters, leftCounters)
	}
}

func TestNoLinkGoesToADisabledAccount(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.svc.Bootstrap(ctx, "owner@example.test"); err != nil {
		t.Fatal(err)
	}
	h.exec(t, `UPDATE dashboard.operators SET status = 'disabled'`)
	if sent, err := h.svc.DeliverNext(ctx); sent || err != nil {
		t.Fatalf("deliver = %v, %v; want nothing sent", sent, err)
	}
}

func TestEveryWriteKeepsToItsOwnRows(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.activate(t, "witness@example.test")
	w := h.login(t, "witness@example.test")
	h.exec(t, `UPDATE dashboard.sessions SET last_seen_at = now() - interval '5 minutes'`)
	h.exec(t, `UPDATE dashboard.access_links SET token_hash = 'w', sent_at = now(), attempts = 3, next_attempt_at = now() + interval '1 hour'`)
	snapshot := func() string {
		var s string
		if err := h.owner.QueryRow(ctx, `SELECT o.password_hash || l.token_hash::text || l.attempts || l.next_attempt_at || l.sent_at
			|| (SELECT string_agg(last_seen_at::text, ',') FROM dashboard.sessions WHERE operator_id = o.id)
			FROM dashboard.operators o JOIN dashboard.access_links l ON l.operator_id = o.id WHERE o.email = 'witness@example.test'`).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	before := snapshot()

	if err := h.svc.Bootstrap(ctx, "owner@example.test"); err != nil {
		t.Fatal(err)
	}
	h.mail.fail = errors.New("relay refused")
	_, _ = h.svc.DeliverNext(ctx)
	h.mail.fail = nil
	h.exec(t, `UPDATE dashboard.access_links SET next_attempt_at = now() WHERE kind = 'activation' AND sent_at IS NULL`)
	h.deliver(t)
	if err := h.svc.UseAccessLink(ctx, h.mail.token(t), password); err != nil {
		t.Fatal(err)
	}
	first, second := h.login(t, "owner@example.test"), h.login(t, "owner@example.test")
	if err := h.svc.Activity(ctx, first.Cookie); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.Logout(ctx, first.Cookie); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Resolve(ctx, second.Cookie); err != nil {
		t.Errorf("logout closed another session of its operator: %v", err)
	}
	h.exec(t, `UPDATE dashboard.access_links SET kind = 'reset', token_hash = NULL, sent_at = NULL, next_attempt_at = now()
		WHERE operator_id = (SELECT id FROM dashboard.operators WHERE email = 'owner@example.test')`)
	h.deliver(t)
	if err := h.svc.UseAccessLink(ctx, h.mail.token(t), "Another-horse-8"); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(); after != before {
		t.Fatalf("the witness changed:\nbefore %s\nafter  %s", before, after)
	}
	if _, err := h.svc.Resolve(ctx, w.Cookie); err != nil {
		t.Fatalf("the witness lost its session: %v", err)
	}
}

type gate struct {
	mailbox
	entered chan string
	release chan struct{}
	once    sync.Once
}

func (g *gate) Send(ctx context.Context, to, subject, body string) error {
	first := false
	g.once.Do(func() { first = true })
	if first {
		g.entered <- to
		<-g.release
	}
	return g.mailbox.Send(ctx, to, subject, body)
}

func TestTwoDeliveriesSendTwoLinksOldestFirst(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	for _, email := range []string{"newer@example.test", "older@example.test"} {
		if err := h.svc.Bootstrap(ctx, email); err != nil {
			t.Fatal(err)
		}
	}
	h.exec(t, `UPDATE dashboard.access_links l SET next_attempt_at = now() - CASE o.email WHEN 'older@example.test' THEN interval '2 minutes' ELSE interval '1 minute' END
		FROM dashboard.operators o WHERE o.id = l.operator_id`)
	g := &gate{entered: make(chan string, 1), release: make(chan struct{})}
	h.svc.Mailer = g
	done := make(chan error, 1)
	go func() {
		_, err := h.svc.DeliverNext(ctx)
		done <- err
	}()
	if first := <-g.entered; first != "older@example.test" {
		t.Fatalf("first delivery to %s, want the oldest due", first)
	}
	short, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	sent, err := h.svc.DeliverNext(short)
	close(g.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err != nil || !sent {
		t.Fatalf("second delivery while the first is sending: %v, %v; want the other link", sent, err)
	}
	var to []string
	for _, m := range g.sent {
		to = append(to, m.to)
	}
	if slices.Sort(to); !slices.Equal(to, []string{"newer@example.test", "older@example.test"}) {
		t.Fatalf("sent to %q, want each link once", to)
	}
}

func TestA429OnTheEmailStillCountsTheSource(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.activate(t, "owner@example.test")
	for range 5 {
		_, _ = h.svc.Login(ctx, "owner@example.test", "wrong", h.source())
	}
	source := h.source()
	var limited dashboard.RateLimited
	if _, err := h.svc.Login(ctx, "owner@example.test", "wrong", source); !errors.As(err, &limited) {
		t.Fatalf("locked email: %v", err)
	}
	for i := range 4 {
		_, _ = h.svc.Login(ctx, "other"+string(rune('a'+i))+"@example.test", "wrong", source)
	}
	if _, err := h.svc.Login(ctx, "fresh@example.test", "wrong", source); !errors.As(err, &limited) {
		t.Fatalf("a source after five attempts, one of them refused on its email: %v, want locked", err)
	}
}

func TestAnInvalidSourceIsRefused(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.Login(context.Background(), "owner@example.test", "wrong", netip.Addr{}); err == nil || errors.Is(err, dashboard.ErrUnauthenticated) {
		t.Fatalf("login from no address: %v, want a refusal of the source", err)
	}
	var counters int
	if err := h.owner.QueryRow(context.Background(), `SELECT count(*) FROM dashboard.login_attempt_counters`).Scan(&counters); err != nil || counters != 0 {
		t.Fatalf("%d counters (%v), want none", counters, err)
	}
}

func TestAnEmailStoredInUpperCaseLogsIn(t *testing.T) {
	h := newHarness(t)
	h.activate(t, "owner@example.test")
	h.exec(t, `UPDATE dashboard.operators SET email = 'Owner@Example.TEST'`)
	h.login(t, "owner@example.test")
}

func TestUndeliverableCountsTheLinksOfActiveAccountsThatGaveUp(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.svc.Bootstrap(ctx, "owner@example.test"); err != nil {
		t.Fatal(err)
	}
	for spoil, want := range map[string]int{
		`UPDATE dashboard.access_links SET attempts = 9`:                   0,
		`UPDATE dashboard.access_links SET attempts = 10`:                  1,
		`UPDATE dashboard.access_links SET attempts = 10, sent_at = now()`: 0,
		`UPDATE dashboard.operators SET status = 'disabled'`:               0,
	} {
		h.exec(t, `UPDATE dashboard.operators SET status = 'active'`)
		h.exec(t, `UPDATE dashboard.access_links SET attempts = 10, sent_at = NULL`)
		h.exec(t, spoil)
		if got, err := h.svc.Undeliverable(ctx); err != nil || got != want {
			t.Errorf("after %s: %d, %v; want %d", spoil, got, err, want)
		}
	}
}
