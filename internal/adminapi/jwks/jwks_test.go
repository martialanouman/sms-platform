package jwks_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/martialanouman/sms-platform/internal/adminapi/jwks"
)

type issuer struct {
	mu      sync.Mutex
	keys    map[string]*ecdsa.PrivateKey
	extra   []map[string]string
	fetches atomic.Int32
	down    atomic.Bool
	status  int
	body    string
}

func (i *issuer) add(t *testing.T, kid string) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.keys[kid] = key
	return key
}

func (i *issuer) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	i.fetches.Add(1)
	if i.down.Load() {
		http.Error(w, "down", http.StatusServiceUnavailable)
		return
	}
	if i.body != "" {
		if i.status != 0 {
			w.WriteHeader(i.status)
		}
		_, _ = w.Write([]byte(i.body))
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	set := struct {
		Keys []map[string]string `json:"keys"`
	}{Keys: append([]map[string]string(nil), i.extra...)}
	for kid, key := range i.keys {
		point, _ := key.PublicKey.Bytes()
		set.Keys = append(set.Keys, map[string]string{"kty": "EC", "crv": "P-256", "alg": "ES256", "use": "sig",
			"kid": kid, "x": base64.RawURLEncoding.EncodeToString(point[1:33]),
			"y": base64.RawURLEncoding.EncodeToString(point[33:])})
	}
	_ = json.NewEncoder(w).Encode(set)
}

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func setup(t *testing.T) (*issuer, *clock, *jwks.Keys) {
	t.Helper()
	iss := &issuer{keys: map[string]*ecdsa.PrivateKey{}}
	srv := httptest.NewServer(iss)
	t.Cleanup(srv.Close)
	c := &clock{now: time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)}
	return iss, c, &jwks.Keys{URL: srv.URL, Client: srv.Client(), Now: c.Now, Log: slog.New(slog.DiscardHandler)}
}

func TestAKidIsLoadedOnFirstUseAndServedFromCacheAfter(t *testing.T) {
	iss, _, k := setup(t)
	want := iss.add(t, "v1")

	for range 3 {
		got, err := k.Key(context.Background(), "v1")
		if err != nil {
			t.Fatal(err)
		}
		if !got.Equal(&want.PublicKey) {
			t.Fatal("not the issuer's key")
		}
	}
	if n := iss.fetches.Load(); n != 1 {
		t.Fatalf("fetches = %d, want 1", n)
	}
}

func TestAnUnknownKidReloadsAtMostOncePerMinute(t *testing.T) {
	iss, c, k := setup(t)
	iss.add(t, "v1")

	for range 5 {
		if _, err := k.Key(context.Background(), "v2"); err == nil {
			t.Fatal("unknown kid accepted")
		}
	}
	if n := iss.fetches.Load(); n != 1 {
		t.Fatalf("fetches = %d, want 1", n)
	}
	iss.add(t, "v2")
	c.advance(59 * time.Second)
	if _, err := k.Key(context.Background(), "v2"); err == nil {
		t.Fatal("reloaded within the minute")
	}
	c.advance(time.Second)
	if _, err := k.Key(context.Background(), "v2"); err != nil {
		t.Fatalf("rotated kid after a minute: %v", err)
	}
	if n := iss.fetches.Load(); n != 2 {
		t.Fatalf("fetches = %d, want 2", n)
	}
}

func TestConcurrentUnknownKidsShareOneReload(t *testing.T) {
	iss, _, k := setup(t)
	iss.add(t, "v1")

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() { _, _ = k.Key(context.Background(), "v9") })
	}
	wg.Wait()
	if n := iss.fetches.Load(); n != 1 {
		t.Fatalf("fetches = %d, want 1", n)
	}
}

func TestAnUnreachableIssuerKeepsTheCachedKeys(t *testing.T) {
	iss, c, k := setup(t)
	iss.add(t, "v1")
	if _, err := k.Key(context.Background(), "v1"); err != nil {
		t.Fatal(err)
	}

	iss.down.Store(true)
	c.advance(time.Minute)
	if _, err := k.Key(context.Background(), "v2"); err == nil {
		t.Fatal("unknown kid accepted while the issuer is down")
	}
	if _, err := k.Key(context.Background(), "v1"); err != nil {
		t.Fatalf("cached key lost: %v", err)
	}
}

func TestOnlyP256SigningKeysAreKept(t *testing.T) {
	iss, _, k := setup(t)
	iss.add(t, "v1")
	point := strings.Repeat("A", 43)
	iss.extra = []map[string]string{
		{"kty": "RSA", "kid": "rsa", "n": "AQAB", "e": "AQAB"},
		{"kty": "EC", "crv": "P-384", "kid": "p384", "x": point, "y": point},
		{"kty": "EC", "crv": "P-256", "kid": "off-curve", "x": point, "y": point},
	}

	if _, err := k.Key(context.Background(), "v1"); err != nil {
		t.Fatalf("a bad entry spoiled the set: %v", err)
	}
	for _, kid := range []string{"rsa", "p384", "off-curve"} {
		if _, err := k.Key(context.Background(), kid); err == nil {
			t.Errorf("%s kept", kid)
		}
	}
}

func TestAnOversizedKeySetIsRefused(t *testing.T) {
	iss, _, k := setup(t)
	iss.add(t, "v1")
	rec := httptest.NewRecorder()
	iss.ServeHTTP(rec, nil)
	iss.body = `{"pad":"` + strings.Repeat("x", 1<<20) + `",` + strings.TrimPrefix(rec.Body.String(), "{")

	if _, err := k.Key(context.Background(), "v1"); err == nil {
		t.Fatal("oversized key set read")
	}
}

func TestAnErrorStatusOrAnEmptySetKeepsTheCachedKeys(t *testing.T) {
	for status, body := range map[int]string{http.StatusBadGateway: `{"error":"bad gateway"}`, http.StatusOK: `{"keys":[]}`} {
		iss, c, k := setup(t)
		iss.add(t, "v1")
		if _, err := k.Key(context.Background(), "v1"); err != nil {
			t.Fatal(err)
		}
		iss.status, iss.body = status, body
		c.advance(time.Minute)
		if _, err := k.Key(context.Background(), "v2"); err == nil {
			t.Fatalf("%d %s: unknown kid accepted", status, body)
		}
		if _, err := k.Key(context.Background(), "v1"); err != nil {
			t.Errorf("%d %s: cached key lost: %v", status, body, err)
		}
	}
}

func TestACallerThatHasGoneDoesNotSpoilTheReload(t *testing.T) {
	iss, _, k := setup(t)
	iss.add(t, "v1")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := k.Key(ctx, "v1"); err != nil {
		t.Fatalf("reload under a cancelled caller: %v", err)
	}
}
