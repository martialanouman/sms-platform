// Command debtguard fails when a debt mark of the repository names no open issue labelled debt.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/martialanouman/sms-platform/internal/debt"
	"github.com/martialanouman/sms-platform/internal/debt/github"
)

func main() {
	repo := os.Getenv("GITHUB_REPOSITORY")
	if repo == "" {
		fmt.Fprintln(os.Stderr, "debtguard: GITHUB_REPOSITORY manquant")
		os.Exit(2)
	}
	issues := github.Client{
		BaseURL: "https://api.github.com",
		Repo:    repo,
		Token:   os.Getenv("GH_TOKEN"),
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
	orphans, err := debt.Orphans(context.Background(), os.DirFS("."), issues)
	if err != nil {
		fmt.Fprintln(os.Stderr, "debtguard:", err)
		os.Exit(2)
	}
	for _, m := range orphans {
		fmt.Printf("%s:%d: debt: #%d ne désigne aucune issue ouverte étiquetée debt\n", m.Path, m.Line, m.Issue)
	}
	if len(orphans) > 0 {
		os.Exit(1)
	}
}
