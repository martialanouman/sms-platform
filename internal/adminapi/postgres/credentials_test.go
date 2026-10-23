package postgres_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/martialanouman/sms-platform/internal/platform/argon2id"
)

type credentialBody struct {
	ID             string     `json:"id"`
	AccountID      string     `json:"account_id"`
	Type           string     `json:"type"`
	SystemID       *string    `json:"system_id"`
	Status         string     `json:"status"`
	LastUsedAt     *time.Time `json:"last_used_at"`
	GraceExpiresAt *time.Time `json:"grace_expires_at"`
	CreatedAt      *time.Time `json:"created_at"`
	RotatedAt      *time.Time `json:"rotated_at"`
	Secret         string     `json:"secret"`
}

func (g gateway) account(t *testing.T) accountBody {
	t.Helper()
	c := g.createCustomer(t, `{"name": "Acme"}`)
	return g.createAccount(t, `{"customer_id": "`+c.ID+`", "name": "api"}`)
}

func (g gateway) createCredential(t *testing.T, account, body string) credentialBody {
	t.Helper()
	rec := g.send(t, http.MethodPost, "/admin/accounts/"+account+"/credentials", "credentials:write", body)
	if got := rec.Header().Get("Cache-Control"); rec.Code == http.StatusCreated && got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	return as[credentialBody](t, rec, http.StatusCreated)
}

func (g gateway) hashes(t *testing.T, id string) (current, previous *string) {
	t.Helper()
	if err := g.migrator.QueryRow(context.Background(),
		`SELECT secret_hash, previous_secret_hash FROM control_plane.credentials WHERE id = $1`, id).Scan(&current, &previous); err != nil {
		t.Fatal(err)
	}
	return current, previous
}

func sha(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func bindVerifies(t *testing.T, phc *string, secret string) bool {
	t.Helper()
	if phc == nil {
		return false
	}
	ok, err := argon2id.Verify(*phc, secret)
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

var (
	apiKey       = regexp.MustCompile(`^smsgw_[A-Za-z0-9_-]{43}$`)
	bindPassword = regexp.MustCompile(`^[A-Za-z0-9]{8}$`)
)

func TestCredentialsAreCreatedWithASecretShownOnceAndStoredHashed(t *testing.T) {
	g := newGateway(t)
	a := g.account(t)

	bind := g.createCredential(t, a.ID, `{"type": "smpp_bind", "system_id": "acme01"}`)
	key := g.createCredential(t, a.ID, `{"type": "api_key"}`)
	if key.AccountID != a.ID || key.Type != "api_key" || key.SystemID != nil || key.Status != "active" ||
		key.LastUsedAt != nil || key.GraceExpiresAt != nil || key.RotatedAt != nil || key.CreatedAt == nil ||
		!apiKey.MatchString(key.Secret) {
		t.Fatalf("key = %+v", key)
	}
	if current, _ := g.hashes(t, key.ID); current == nil || *current != sha(key.Secret) {
		t.Fatalf("api key hash = %v", current)
	}
	if bind.Type != "smpp_bind" || bind.SystemID == nil || *bind.SystemID != "acme01" || !bindPassword.MatchString(bind.Secret) {
		t.Fatalf("bind = %+v", bind)
	}
	if current, _ := g.hashes(t, bind.ID); !bindVerifies(t, current, bind.Secret) || bindVerifies(t, current, bind.Secret+"x") {
		t.Fatalf("bind hash does not verify its secret alone: %v", current)
	}

	other := g.account(t)
	if again := g.createCredential(t, other.ID, `{"type": "api_key"}`); again.Secret == key.Secret {
		t.Fatal("two keys share their secret")
	}
	if again := g.createCredential(t, other.ID, `{"type": "smpp_bind", "system_id": "acme02"}`); again.Secret == bind.Secret {
		t.Fatal("two binds share their password")
	}

	rec := g.send(t, http.MethodGet, "/admin/accounts/"+a.ID+"/credentials", "credentials:read", "")
	var listed []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil || rec.Code != http.StatusOK || len(listed) != 2 {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	for _, c := range listed {
		for k := range c {
			if k == "secret" || strings.HasSuffix(k, "_hash") || strings.HasSuffix(k, "_sealed") {
				t.Fatalf("listed credential carries %q: %v", k, c)
			}
		}
	}
	if listed[0]["type"] != "api_key" || listed[1]["type"] != "smpp_bind" {
		t.Fatalf("list order: %v", listed)
	}
	if strings.Contains(rec.Body.String(), key.Secret) || strings.Contains(rec.Body.String(), bind.Secret) {
		t.Fatal("a secret is listed")
	}
	if rec := g.send(t, http.MethodGet, "/admin/accounts/00000000-0000-7000-8000-000000000000/credentials", "credentials:read", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown account: %d", rec.Code)
	}
}

func TestACredentialIsRefusedATakenTypeATakenSystemIDAMisplacedOneAndAClosedAccount(t *testing.T) {
	g := newGateway(t)
	a := g.account(t)
	g.createCredential(t, a.ID, `{"type": "api_key"}`)
	g.createCredential(t, a.ID, `{"type": "smpp_bind", "system_id": "acme01"}`)
	other := g.account(t)
	path := "/admin/accounts/" + other.ID + "/credentials"

	for body, want := range map[string]struct {
		status int
		field  string
	}{
		`{"type": "smpp_bind", "system_id": "acme01"}`: {http.StatusConflict, "body.system_id"},
		`{"type": "smpp_bind"}`:                        {http.StatusUnprocessableEntity, "body.system_id"},
		`{"type": "api_key", "system_id": "x"}`:        {http.StatusUnprocessableEntity, "body.system_id"},
		`{"type": "smpp_bind", "system_id": "a b"}`:    {http.StatusUnprocessableEntity, "body.system_id"},
	} {
		p := as[problem](t, g.send(t, http.MethodPost, path, "credentials:write", body), want.status)
		if len(p.Errors) != 1 || p.Errors[0].Field != want.field {
			t.Errorf("%s: %+v", body, p)
		}
	}
	p := as[problem](t, g.send(t, http.MethodPost, "/admin/accounts/"+a.ID+"/credentials", "credentials:write", `{"type": "api_key"}`),
		http.StatusConflict)
	if len(p.Errors) != 1 || p.Errors[0].Field != "body.type" {
		t.Fatalf("taken type: %+v", p)
	}

	g.send(t, http.MethodPatch, "/admin/accounts/"+other.ID, "accounts:write", `{"status": "closed"}`)
	p = as[problem](t, g.send(t, http.MethodPost, path, "credentials:write", `{"type": "api_key"}`), http.StatusUnprocessableEntity)
	if p.Code != "validation_error" || len(p.Errors) != 0 {
		t.Fatalf("closed account: %+v", p)
	}
	suspended := g.account(t)
	g.send(t, http.MethodPost, "/admin/accounts/"+suspended.ID+"/suspend", "accounts:write", "")
	g.createCredential(t, suspended.ID, `{"type": "api_key"}`)
	if rec := g.send(t, http.MethodPost, "/admin/accounts/00000000-0000-7000-8000-000000000000/credentials", "credentials:write",
		`{"type": "api_key"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown account: %d %s", rec.Code, rec.Body)
	}
}

func TestARotationKeepsTheOldSecretForItsGraceOnly(t *testing.T) {
	g := newGateway(t)
	a := g.account(t)
	key := g.createCredential(t, a.ID, `{"type": "api_key"}`)
	path := "/admin/accounts/" + a.ID + "/credentials/" + key.ID

	before := time.Now()
	rec := g.send(t, http.MethodPost, path+"/rotate", "credentials:write", `{"grace_period_sec": 3600}`)
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	graced := as[credentialBody](t, rec, http.StatusOK)
	current, previous := g.hashes(t, key.ID)
	if !apiKey.MatchString(graced.Secret) || graced.Secret == key.Secret || *current != sha(graced.Secret) ||
		previous == nil || *previous != sha(key.Secret) || graced.GraceExpiresAt == nil ||
		graced.GraceExpiresAt.Sub(before) < 59*time.Minute || graced.GraceExpiresAt.Sub(before) > 61*time.Minute ||
		graced.RotatedAt == nil {
		t.Fatalf("graced = %+v", graced)
	}

	for _, body := range []string{``, `{}`, `{"grace_period_sec": null}`, `{"grace_period_sec": 0}`} {
		at := as[credentialBody](t, g.send(t, http.MethodPost, path+"/rotate", "credentials:write", body), http.StatusOK)
		current, previous := g.hashes(t, key.ID)
		if at.GraceExpiresAt != nil || previous != nil || *current != sha(at.Secret) {
			t.Fatalf("%q: %+v previous=%v", body, at, previous)
		}
	}
	for _, body := range []string{`{"grace_period_sec": 604801}`, `{"grace_period_sec": -1}`} {
		if rec := g.send(t, http.MethodPost, path+"/rotate", "credentials:write", body); rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: %d", body, rec.Code)
		}
	}
	if rec := g.send(t, http.MethodPost, "/admin/accounts/"+g.account(t).ID+"/credentials/"+key.ID+"/rotate", "credentials:write", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("another account's credential: %d", rec.Code)
	}
}

func TestADisabledCredentialStaysDisabledAndARevokedOneOnlyARotationRevives(t *testing.T) {
	g := newGateway(t)
	a := g.account(t)
	bind := g.createCredential(t, a.ID, `{"type": "smpp_bind", "system_id": "acme01"}`)
	path := "/admin/accounts/" + a.ID + "/credentials/" + bind.ID

	for range 2 {
		if c := as[credentialBody](t, g.send(t, http.MethodPatch, path, "credentials:write", `{"status": "disabled"}`), http.StatusOK); c.Status != "disabled" || c.Secret != "" {
			t.Fatalf("disabled = %+v", c)
		}
	}
	if rotated := as[credentialBody](t, g.send(t, http.MethodPost, path+"/rotate", "credentials:write", `{"grace_period_sec": 60}`), http.StatusOK); rotated.Status != "disabled" {
		t.Fatalf("a rotation enabled it: %+v", rotated)
	}
	if c := as[credentialBody](t, g.send(t, http.MethodPatch, path, "credentials:write", `{"status": "active"}`), http.StatusOK); c.Status != "active" {
		t.Fatalf("active = %+v", c)
	}
	if rec := g.send(t, http.MethodPatch, path, "credentials:write", `{"status": "revoked"}`); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("revoked through the status: %d %s", rec.Code, rec.Body)
	}

	elsewhere := "/admin/accounts/" + g.account(t).ID + "/credentials/" + bind.ID
	if rec := g.send(t, http.MethodPatch, elsewhere, "credentials:write", `{"status": "disabled"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("patch through another account: %d", rec.Code)
	}
	if rec := g.send(t, http.MethodDelete, elsewhere, "credentials:write", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("revoke through another account: %d", rec.Code)
	}
	if current, _ := g.hashes(t, bind.ID); current == nil {
		t.Fatal("revoked through another account")
	}
	for range 2 {
		if rec := g.send(t, http.MethodDelete, path, "credentials:write", ""); rec.Code != http.StatusNoContent {
			t.Fatalf("revoke: %d %s", rec.Code, rec.Body)
		}
	}
	current, previous := g.hashes(t, bind.ID)
	var grace *time.Time
	if err := g.migrator.QueryRow(context.Background(), `SELECT grace_expires_at FROM control_plane.credentials WHERE id = $1`, bind.ID).Scan(&grace); err != nil {
		t.Fatal(err)
	}
	if current != nil || previous != nil || grace != nil {
		t.Fatalf("revoked keeps %v %v %v", current, previous, grace)
	}
	p := as[problem](t, g.send(t, http.MethodPatch, path, "credentials:write", `{"status": "active"}`), http.StatusConflict)
	if p.Code != "conflict" {
		t.Fatalf("enable revoked: %+v", p)
	}
	revived := as[credentialBody](t, g.send(t, http.MethodPost, path+"/rotate", "credentials:write", `{"grace_period_sec": 3600}`), http.StatusOK)
	if current, previous := g.hashes(t, bind.ID); revived.Status != "active" || revived.GraceExpiresAt != nil || previous != nil ||
		!bindVerifies(t, current, revived.Secret) || revived.Secret == bind.Secret {
		t.Fatalf("revived = %+v", revived)
	}
	if rec := g.send(t, http.MethodDelete, "/admin/accounts/"+a.ID+"/credentials/00000000-0000-7000-8000-000000000000", "credentials:write", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown credential: %d", rec.Code)
	}
}

func TestNoHashReachesTheAuditAndCredentialsGoWithTheirAccount(t *testing.T) {
	g := newGateway(t)
	a := g.account(t)
	key := g.createCredential(t, a.ID, `{"type": "api_key"}`)
	g.send(t, http.MethodPost, "/admin/accounts/"+a.ID+"/credentials/"+key.ID+"/rotate", "credentials:write", `{"grace_period_sec": 60}`)
	g.send(t, http.MethodDelete, "/admin/accounts/"+a.ID+"/credentials/"+key.ID, "credentials:write", "")

	if n := g.count(t, `SELECT count(*) FROM control_plane.audit_changes WHERE table_name = 'credentials' AND actor = '`+operator+`'`); n != 3 {
		t.Fatalf("%d credential changes under the operator", n)
	}
	if n := g.count(t, `SELECT count(*) FROM control_plane.audit_changes WHERE table_name = 'credentials'
		AND (before ?| array['secret_hash', 'previous_secret_hash'] OR after ?| array['secret_hash', 'previous_secret_hash'])`); n != 0 {
		t.Fatalf("%d changes carry a hash", n)
	}

	if rec := g.send(t, http.MethodDelete, "/admin/accounts/"+a.ID, "accounts:write", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete account: %d %s", rec.Code, rec.Body)
	}
	if n := g.count(t, `SELECT count(*) FROM control_plane.credentials`); n != 0 {
		t.Fatalf("%d credentials outlive their account", n)
	}
}

func TestAClosedAccountRevivesNoCredentialButStillRevokesAndDisables(t *testing.T) {
	g := newGateway(t)
	a := g.account(t)
	key := g.createCredential(t, a.ID, `{"type": "api_key"}`)
	bind := g.createCredential(t, a.ID, `{"type": "smpp_bind", "system_id": "acme01"}`)
	keyPath := "/admin/accounts/" + a.ID + "/credentials/" + key.ID
	bindPath := "/admin/accounts/" + a.ID + "/credentials/" + bind.ID
	g.send(t, http.MethodDelete, keyPath, "credentials:write", "")
	g.send(t, http.MethodPatch, "/admin/accounts/"+a.ID, "accounts:write", `{"status": "closed"}`)

	if c := as[credentialBody](t, g.send(t, http.MethodPatch, bindPath, "credentials:write", `{"status": "disabled"}`), http.StatusOK); c.Status != "disabled" {
		t.Fatalf("disable on a closed account = %+v", c)
	}
	for _, call := range []struct{ method, path, body string }{
		{http.MethodPost, keyPath + "/rotate", ""},
		{http.MethodPost, bindPath + "/rotate", `{"grace_period_sec": 60}`},
		{http.MethodPatch, bindPath, `{"status": "active"}`},
	} {
		p := as[problem](t, g.send(t, call.method, call.path, "credentials:write", call.body), http.StatusConflict)
		if p.Code != "conflict" {
			t.Fatalf("%s %s: %+v", call.method, call.path, p)
		}
	}
	if rec := g.send(t, http.MethodDelete, bindPath, "credentials:write", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke on a closed account: %d", rec.Code)
	}
}
