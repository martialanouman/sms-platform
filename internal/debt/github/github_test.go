package github_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/martialanouman/sms-platform/internal/debt/github"
)

var pages = map[string]string{
	"1": `[{"number":1},{"number":2,"pull_request":{}}]`,
	"2": `[{"number":3}]`,
	"3": `[]`,
}

func server(t *testing.T, auth *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/repos/acme/sms/issues" || q.Get("labels") != "debt" || q.Get("state") != "open" || q.Get("per_page") != "100" {
			http.Error(w, "unexpected request "+r.URL.String(), http.StatusBadRequest)
			return
		}
		*auth = r.Header.Get("Authorization")
		page, ok := pages[q.Get("page")]
		if !ok {
			http.Error(w, "unexpected page", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(page))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestOpenDebtReadsEveryPageAndSkipsPullRequests(t *testing.T) {
	var auth string
	srv := server(t, &auth)
	got, err := github.Client{BaseURL: srv.URL, Repo: "acme/sms", Token: "tok", HTTP: srv.Client()}.OpenDebt(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := map[int]bool{1: true, 3: true}; !reflect.DeepEqual(got, want) {
		t.Errorf("OpenDebt = %v, want %v", got, want)
	}
	if auth != "Bearer tok" {
		t.Errorf("Authorization = %q, want the token", auth)
	}
}

func TestOpenDebtSendsNoAuthorizationWithoutToken(t *testing.T) {
	auth := "unset"
	srv := server(t, &auth)
	if _, err := (github.Client{BaseURL: srv.URL, Repo: "acme/sms", HTTP: srv.Client()}).OpenDebt(context.Background()); err != nil {
		t.Fatal(err)
	}
	if auth != "" {
		t.Errorf("Authorization = %q, want none", auth)
	}
}

func TestOpenDebtFailsOnAnErrorStatus(t *testing.T) {
	var auth string
	srv := server(t, &auth)
	_, err := github.Client{BaseURL: srv.URL, Repo: "other/repo", HTTP: srv.Client()}.OpenDebt(context.Background())
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Errorf("err = %v, want the 400 status", err)
	}
}
