package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"
)

type sealer struct {
	err   error
	calls int
}

func (s *sealer) Seal(_ context.Context, plaintext []byte) ([]byte, string, error) {
	if s.err != nil {
		return nil, "", s.err
	}
	s.calls++
	return append([]byte("vault:v1:"), plaintext...), fmt.Sprintf("config-secrets-%d", s.calls), nil
}

type webhookBody struct {
	ID        string     `json:"id"`
	AccountID string     `json:"account_id"`
	EventType string     `json:"event_type"`
	URL       string     `json:"url"`
	Status    string     `json:"status"`
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
	Secret    string     `json:"secret"`
}

var webhookSecret = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (g gateway) createWebhook(t *testing.T, account, body string) webhookBody {
	t.Helper()
	rec := g.send(t, http.MethodPost, "/admin/accounts/"+account+"/webhooks", "accounts:write", body)
	if got := rec.Header().Get("Cache-Control"); rec.Code == http.StatusCreated && got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	return as[webhookBody](t, rec, http.StatusCreated)
}

func (g gateway) sealed(t *testing.T, id string) (sealed, ref string) {
	t.Helper()
	var raw []byte
	if err := g.migrator.QueryRow(context.Background(),
		`SELECT secret_sealed, secret_kms_key_ref FROM control_plane.webhooks WHERE id = $1`, id).Scan(&raw, &ref); err != nil {
		t.Fatal(err)
	}
	return string(raw), ref
}

func TestAWebhookIsCreatedWithASecretShownOnceAndStoredSealed(t *testing.T) {
	g := newGateway(t)
	a := g.account(t)

	mo := g.createWebhook(t, a.ID, `{"event_type": "mo", "url": "https://hooks.example.com/mo"}`)
	if mo.AccountID != a.ID || mo.EventType != "mo" || mo.URL != "https://hooks.example.com/mo" || mo.Status != "active" ||
		mo.CreatedAt == nil || mo.UpdatedAt == nil || !webhookSecret.MatchString(mo.Secret) {
		t.Fatalf("mo = %+v", mo)
	}
	if sealed, ref := g.sealed(t, mo.ID); sealed != "vault:v1:"+mo.Secret || ref != "config-secrets-1" {
		t.Fatalf("sealed = %q %q", sealed, ref)
	}
	dlr := g.createWebhook(t, a.ID, `{"event_type": "dlr", "url": "https://hooks.example.com/dlr"}`)
	if dlr.Secret == mo.Secret {
		t.Fatal("two webhooks share their secret")
	}

	g.createWebhook(t, g.account(t).ID, `{"event_type": "mo", "url": "https://hooks.example.com/other"}`)
	rec := g.send(t, http.MethodGet, "/admin/accounts/"+a.ID+"/webhooks", "accounts:read", "")
	var listed []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil || rec.Code != http.StatusOK || len(listed) != 2 {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	for _, w := range listed {
		for k := range w {
			if k == "secret" || strings.HasSuffix(k, "_sealed") || strings.HasSuffix(k, "_kms_key_ref") {
				t.Fatalf("listed webhook carries %q", k)
			}
		}
	}
	if listed[0]["event_type"] != "dlr" || listed[1]["event_type"] != "mo" {
		t.Fatalf("order: %v", listed)
	}
	if rec := g.send(t, http.MethodGet, "/admin/accounts/00000000-0000-7000-8000-000000000000/webhooks", "accounts:read", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown account: %d", rec.Code)
	}
}

func TestAWebhookIsRefusedATakenEventABadURLAndAClosedAccount(t *testing.T) {
	g := newGateway(t)
	a := g.account(t)
	g.createWebhook(t, a.ID, `{"event_type": "mo", "url": "https://hooks.example.com/mo"}`)
	path := "/admin/accounts/" + a.ID + "/webhooks"

	p := as[problem](t, g.send(t, http.MethodPost, path, "accounts:write", `{"event_type": "mo", "url": "https://other.example.com"}`),
		http.StatusConflict)
	if len(p.Errors) != 1 || p.Errors[0].Field != "body.event_type" {
		t.Fatalf("taken event: %+v", p)
	}
	for _, url := range []string{"http://hooks.example.com", "https://user@hooks.example.com", "https://" + strings.Repeat("a", 2048)} {
		if rec := g.send(t, http.MethodPost, path, "accounts:write", `{"event_type": "dlr", "url": "`+url+`"}`); rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: %d", url[:20], rec.Code)
		}
	}

	suspended := g.account(t)
	g.send(t, http.MethodPost, "/admin/accounts/"+suspended.ID+"/suspend", "accounts:write", "")
	g.createWebhook(t, suspended.ID, `{"event_type": "mo", "url": "https://hooks.example.com/mo"}`)

	g.send(t, http.MethodPatch, "/admin/accounts/"+a.ID, "accounts:write", `{"status": "closed"}`)
	p = as[problem](t, g.send(t, http.MethodPost, path, "accounts:write", `{"event_type": "dlr", "url": "https://hooks.example.com"}`),
		http.StatusUnprocessableEntity)
	if p.Code != "validation_error" || len(p.Errors) != 0 {
		t.Fatalf("closed account: %+v", p)
	}
	if rec := g.send(t, http.MethodPost, "/admin/accounts/00000000-0000-7000-8000-000000000000/webhooks", "accounts:write",
		`{"event_type": "mo", "url": "https://hooks.example.com"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown account: %d", rec.Code)
	}
}

func TestAWebhookIsUpdatedRotatedAndDeleted(t *testing.T) {
	g := newGateway(t)
	a := g.account(t)
	w := g.createWebhook(t, a.ID, `{"event_type": "dlr", "url": "https://hooks.example.com/dlr"}`)
	path := "/admin/accounts/" + a.ID + "/webhooks/" + w.ID

	updated := as[webhookBody](t, g.send(t, http.MethodPatch, path, "accounts:write",
		`{"url": "https://hooks.example.com/v2", "status": "disabled"}`), http.StatusOK)
	if updated.URL != "https://hooks.example.com/v2" || updated.Status != "disabled" || updated.Secret != "" ||
		!updated.UpdatedAt.After(*w.UpdatedAt) {
		t.Fatalf("updated = %+v", updated)
	}
	if same := as[webhookBody](t, g.send(t, http.MethodPatch, path, "accounts:write", `{}`), http.StatusOK); same.URL != updated.URL || same.Status != "disabled" {
		t.Fatalf("empty patch = %+v", same)
	}
	for _, body := range []string{`{"url": null}`, `{"status": null}`, `{"url": "http://x"}`} {
		if rec := g.send(t, http.MethodPatch, path, "accounts:write", body); rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: %d", body, rec.Code)
		}
	}

	rec := g.send(t, http.MethodPost, path+"/rotate-secret", "accounts:write", "")
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	rotated := as[webhookBody](t, rec, http.StatusOK)
	if !webhookSecret.MatchString(rotated.Secret) || rotated.Secret == w.Secret || rotated.Status != "disabled" {
		t.Fatalf("rotated = %+v", rotated)
	}
	if sealed, ref := g.sealed(t, w.ID); sealed != "vault:v1:"+rotated.Secret || ref != "config-secrets-2" {
		t.Fatalf("sealed after rotation = %q", sealed)
	}

	elsewhere := "/admin/accounts/" + g.account(t).ID + "/webhooks/" + w.ID
	for _, call := range []struct{ method, path, body string }{
		{http.MethodPatch, elsewhere, `{"status": "active"}`},
		{http.MethodPost, elsewhere + "/rotate-secret", ""},
		{http.MethodDelete, elsewhere, ""},
	} {
		if rec := g.send(t, call.method, call.path, "accounts:write", call.body); rec.Code != http.StatusNotFound {
			t.Fatalf("%s through another account: %d", call.method, rec.Code)
		}
	}

	if rec := g.send(t, http.MethodDelete, path, "accounts:write", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec := g.send(t, http.MethodDelete, path, "accounts:write", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("delete again: %d", rec.Code)
	}
}

func TestAClosedAccountRevivesNoWebhookButStillEditsAndDeletes(t *testing.T) {
	g := newGateway(t)
	a := g.account(t)
	w := g.createWebhook(t, a.ID, `{"event_type": "mo", "url": "https://hooks.example.com/mo"}`)
	path := "/admin/accounts/" + a.ID + "/webhooks/" + w.ID
	g.send(t, http.MethodPatch, "/admin/accounts/"+a.ID, "accounts:write", `{"status": "closed"}`)

	if c := as[webhookBody](t, g.send(t, http.MethodPatch, path, "accounts:write", `{"url": "https://hooks.example.com/v2"}`), http.StatusOK); c.URL != "https://hooks.example.com/v2" {
		t.Fatalf("url on a closed account = %+v", c)
	}
	if c := as[webhookBody](t, g.send(t, http.MethodPatch, path, "accounts:write", `{"status": "disabled"}`), http.StatusOK); c.Status != "disabled" {
		t.Fatalf("disable on a closed account = %+v", c)
	}
	for _, call := range []struct{ method, path, body string }{
		{http.MethodPatch, path, `{"status": "active"}`},
		{http.MethodPost, path + "/rotate-secret", ""},
	} {
		p := as[problem](t, g.send(t, call.method, call.path, "accounts:write", call.body), http.StatusConflict)
		if p.Code != "conflict" {
			t.Fatalf("%s %s: %+v", call.method, call.path, p)
		}
	}
	if rec := g.send(t, http.MethodDelete, path, "accounts:write", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete on a closed account: %d", rec.Code)
	}
}

func TestASealThatFailsIsUnavailableAndWritesNothing(t *testing.T) {
	g := newGateway(t)
	a := g.account(t)
	w := g.createWebhook(t, a.ID, `{"event_type": "mo", "url": "https://hooks.example.com/mo"}`)
	before, _ := g.sealed(t, w.ID)
	g.sealer.err = errors.New("kms down")

	p := as[problem](t, g.send(t, http.MethodPost, "/admin/accounts/"+a.ID+"/webhooks", "accounts:write",
		`{"event_type": "dlr", "url": "https://hooks.example.com/dlr"}`), http.StatusServiceUnavailable)
	if p.Code != "service_unavailable" || strings.Contains(p.Message, "kms") {
		t.Fatalf("create: %+v", p)
	}
	as[problem](t, g.send(t, http.MethodPost, "/admin/accounts/"+a.ID+"/webhooks/"+w.ID+"/rotate-secret", "accounts:write", ""),
		http.StatusServiceUnavailable)
	if rec := g.send(t, http.MethodPost, "/admin/accounts/00000000-0000-7000-8000-000000000000/webhooks", "accounts:write",
		`{"event_type": "dlr", "url": "https://hooks.example.com/dlr"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown account while the KMS is down: %d", rec.Code)
	}
	if rec := g.send(t, http.MethodPost, "/admin/accounts/"+g.account(t).ID+"/webhooks/"+w.ID+"/rotate-secret", "accounts:write", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("another account's webhook while the KMS is down: %d", rec.Code)
	}
	if after, _ := g.sealed(t, w.ID); after != before {
		t.Fatal("a failed rotation changed the secret")
	}
	if n := g.count(t, `SELECT count(*) FROM control_plane.webhooks`); n != 1 {
		t.Fatalf("%d webhooks after a failed creation", n)
	}
	if n := g.count(t, `SELECT count(*) FROM control_plane.audit_log WHERE operation_id IN ('create-webhook', 'rotate-webhook-secret')
		AND status = 503`); n != 2 {
		t.Fatalf("%d audit lines in 503", n)
	}
}

func TestNoSealedSecretReachesTheAuditAndWebhooksGoWithTheirAccount(t *testing.T) {
	g := newGateway(t)
	a := g.account(t)
	w := g.createWebhook(t, a.ID, `{"event_type": "mo", "url": "https://hooks.example.com/mo"}`)
	g.send(t, http.MethodPost, "/admin/accounts/"+a.ID+"/webhooks/"+w.ID+"/rotate-secret", "accounts:write", "")

	if n := g.count(t, `SELECT count(*) FROM control_plane.audit_changes WHERE table_name = 'webhooks' AND actor = '`+operator+`'`); n != 2 {
		t.Fatalf("%d webhook changes under the operator", n)
	}
	if n := g.count(t, `SELECT count(*) FROM control_plane.audit_changes WHERE table_name = 'webhooks'
		AND (before ?| array['secret_sealed', 'secret_kms_key_ref'] OR after ?| array['secret_sealed', 'secret_kms_key_ref'])`); n != 0 {
		t.Fatalf("%d changes carry the sealed secret", n)
	}
	if rec := g.send(t, http.MethodDelete, "/admin/accounts/"+a.ID, "accounts:write", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete account: %d", rec.Code)
	}
	if n := g.count(t, `SELECT count(*) FROM control_plane.webhooks`); n != 0 {
		t.Fatalf("%d webhooks outlive their account", n)
	}
}

func TestTheBaseRefusesAWebhookURLTheContractRefuses(t *testing.T) {
	g := newGateway(t)
	a := g.account(t)
	for _, url := range []string{"http://hooks.example.com", "https://u@hooks.example.com", "https://" + strings.Repeat("a", 2048)} {
		_, err := g.migrator.Exec(context.Background(), `INSERT INTO control_plane.webhooks
			(account_id, event_type, url, secret_sealed, secret_kms_key_ref) VALUES ($1, 'mo', $2, 'x', 'k')`, a.ID, url)
		if err == nil || !strings.Contains(err.Error(), "webhooks_url_check") {
			t.Errorf("%.24s: %v", url, err)
		}
	}
}
