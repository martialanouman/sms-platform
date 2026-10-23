package adminapi_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/martialanouman/sms-platform/internal/adminapi"
	errs "github.com/martialanouman/sms-platform/internal/platform/errors"
)

type pageTx struct {
	adminapi.Tx
	entries   []adminapi.Entry
	changes   []adminapi.Change
	filter    adminapi.Filter
	after     *adminapi.Key
	limit     int
	requested []string
}

func (p *pageTx) Record(context.Context, adminapi.Entry) error { return nil }

func (p *pageTx) AuditLog(_ context.Context, f adminapi.Filter, after *adminapi.Key, limit int) ([]adminapi.Entry, error) {
	p.filter, p.after, p.limit = f, after, limit
	return p.entries[:min(limit, len(p.entries))], nil
}

func (p *pageTx) Changes(_ context.Context, _ string, after *adminapi.Key, limit int) ([]adminapi.Change, error) {
	p.after, p.limit = after, limit
	return p.changes[:min(limit, len(p.changes))], nil
}

func (p *pageTx) Requested(_ context.Context, requestID string) (bool, error) {
	return slices.Contains(p.requested, requestID), nil
}

func entries(n int) []adminapi.Entry {
	out := make([]adminapi.Entry, n)
	for i := range out {
		out[i] = adminapi.Entry{ID: fmt.Sprintf("0199f0a2-7c1e-7000-8000-%012d", n-i), At: now.Add(-time.Duration(i) * time.Second)}
	}
	return out
}

func TestAnAuditLogPageAsksOneMoreRowToKnowItIsNotTheLast(t *testing.T) {
	tx := &pageTx{entries: entries(4)}
	f := adminapi.Filter{Operator: new(operator)}

	page, next, err := adminapi.AuditLogPage(context.Background(), tx, f, "", 3)
	if err != nil {
		t.Fatal(err)
	}
	if tx.limit != 4 || tx.after != nil || tx.filter.Operator != f.Operator {
		t.Fatalf("asked limit %d after %v filter %+v", tx.limit, tx.after, tx.filter)
	}
	if len(page) != 3 || next == "" {
		t.Fatalf("page of %d, next %q", len(page), next)
	}

	tx.entries = tx.entries[3:]
	page, last, err := adminapi.AuditLogPage(context.Background(), tx, f, next, 3)
	if err != nil {
		t.Fatal(err)
	}
	want := adminapi.Key{At: entries(4)[2].At, ID: entries(4)[2].ID}
	if tx.after == nil || !tx.after.At.Equal(want.At) || tx.after.ID != want.ID {
		t.Fatalf("after = %+v, want %+v", tx.after, want)
	}
	if len(page) != 1 || last != "" {
		t.Fatalf("last page of %d, next %q", len(page), last)
	}
}

func TestAPageRefusesACursorItDidNotIssue(t *testing.T) {
	tx := &pageTx{requested: []string{"r1"}}
	if _, _, err := adminapi.AuditLogPage(context.Background(), tx, adminapi.Filter{}, "nope", 10); !errors.Is(err, errs.ErrValidation) {
		t.Fatalf("log: err = %v", err)
	}
	if _, _, err := adminapi.ChangesPage(context.Background(), tx, "r1", "nope", 10); !errors.Is(err, errs.ErrValidation) {
		t.Fatalf("changes: err = %v", err)
	}
}

func TestTheChangesOfARequestTheLogNeverSawAreNotFound(t *testing.T) {
	tx := &pageTx{requested: []string{"r1"}, changes: []adminapi.Change{{ID: entries(1)[0].ID, At: now}}}

	if _, _, err := adminapi.ChangesPage(context.Background(), tx, "r2", "", 10); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("err = %v, want not_found", err)
	}
	page, next, err := adminapi.ChangesPage(context.Background(), tx, "r1", "", 1)
	if err != nil || len(page) != 1 || next != "" || tx.limit != 2 {
		t.Fatalf("page %v next %q err %v limit %d", page, next, err, tx.limit)
	}
}

func TestEveryWriteAndEverySensitiveReadIsAudited(t *testing.T) {
	for _, c := range []struct {
		method, scope string
		audited       bool
	}{
		{"GET", "routes:read", false},
		{"GET", "stream:read", false},
		{"GET", "audit:read", true},
		{"GET", "msisdn:reveal", true},
		{"POST", "routes:write", true},
		{"POST", "routes:read", true},
		{"PATCH", "customers:write", true},
		{"PUT", "content:write", true},
		{"DELETE", "suppressions:delete", true},
	} {
		if got := adminapi.Audited(c.method, c.scope); got != c.audited {
			t.Errorf("Audited(%s, %s) = %v", c.method, c.scope, got)
		}
	}
}
