// Command catalog validates contracts/topics.yaml, compares it with the base branch, or writes its page.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/martialanouman/sms-platform/internal/catalog"
)

const path = "contracts/topics.yaml"

func main() {
	doc := flag.String("doc", "", "write the documentation page to this file instead of comparing with the base")
	flag.Parse()

	data, err := os.ReadFile(path)
	if err != nil {
		fail(2, err)
	}
	head, err := catalog.Parse(data)
	if err != nil {
		fail(2, err)
	}
	errs := head.Validate()

	if *doc != "" {
		report(errs)
		if err := os.WriteFile(*doc, head.Markdown(), 0o644); err != nil {
			fail(2, err)
		}
		return
	}

	base, err := baseCatalog()
	if err != nil {
		fail(2, err)
	}
	report(append(errs, catalog.Frozen(base, head)...))
}

func baseCatalog() (catalog.Catalog, error) {
	ref := os.Getenv("CATALOG_BASE")
	if ref == "" {
		ref = "origin/main"
	}
	if err := exec.Command("git", "rev-parse", "--verify", "--quiet", ref+"^{commit}").Run(); err != nil {
		return catalog.Catalog{}, fmt.Errorf("base %s not found: %w", ref, err)
	}
	if exec.Command("git", "cat-file", "-e", ref+":"+path).Run() != nil {
		return catalog.Catalog{}, nil
	}
	data, err := exec.Command("git", "show", ref+":"+path).Output()
	if err != nil {
		return catalog.Catalog{}, err
	}
	return catalog.Parse(data)
}

func report(errs []error) {
	for _, err := range errs {
		fmt.Fprintln(os.Stderr, path+":", err)
	}
	if len(errs) > 0 {
		os.Exit(1)
	}
}

func fail(code int, err error) {
	fmt.Fprintln(os.Stderr, "catalog:", err)
	os.Exit(code)
}
