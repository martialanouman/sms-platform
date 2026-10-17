// Command debtguard fails when a debt mark of the repository names no open issue labelled debt.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/martialanouman/sms-platform/internal/debt"
	"github.com/martialanouman/sms-platform/internal/debt/github"
)

func main() {
	tracked, err := exec.Command("git", "ls-files", "-z").Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "debtguard: git ls-files:", err)
		os.Exit(2)
	}
	paths := strings.Split(strings.TrimSuffix(string(tracked), "\x00"), "\x00")
	issues := github.Client{
		BaseURL: "https://api.github.com",
		Repo:    os.Getenv("GITHUB_REPOSITORY"),
		Token:   os.Getenv("GH_TOKEN"),
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
	orphans, err := debt.Orphans(context.Background(), os.DirFS("."), paths, issues)
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
