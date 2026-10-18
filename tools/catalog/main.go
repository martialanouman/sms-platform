// Command catalog validates contracts/topics.yaml, compares it with the base branch, or writes its page.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/martialanouman/sms-platform/internal/catalog"
	_ "github.com/martialanouman/sms-platform/internal/gen/sms/v1"
)

const path = "contracts/topics.yaml"

func main() {
	doc := flag.String("doc", "", "write the documentation page to this file instead of comparing with the base")
	flag.Parse()

	data, err := os.ReadFile(path)
	if err != nil {
		fail(err)
	}
	head, err := catalog.Parse(data)
	if err != nil {
		fail(err)
	}

	if *doc != "" {
		if err := os.WriteFile(*doc, head.Markdown(), 0o600); err != nil {
			fail(err)
		}
		return
	}

	base, err := baseCatalog()
	if err != nil {
		fail(err)
	}
	errs := append(head.Validate(), catalog.Frozen(base, head)...)
	report(append(errs, head.MissingPayloads(known)...))
}

func baseCatalog() (catalog.Catalog, error) {
	ref := os.Getenv("CATALOG_BASE")
	if ref == "" {
		ref = "origin/main"
	}
	if err := exec.Command("git", "rev-parse", "--verify", "--quiet", ref+"^{commit}").Run(); err != nil {
		return catalog.Catalog{}, fmt.Errorf("base %s not found: %w", ref, err)
	}
	listed, err := exec.Command("git", "ls-tree", "--name-only", ref, "--", path).Output()
	if err != nil {
		return catalog.Catalog{}, err
	}
	if len(listed) == 0 {
		return catalog.Catalog{}, nil
	}
	data, err := exec.Command("git", "show", ref+":"+path).Output()
	if err != nil {
		return catalog.Catalog{}, err
	}
	return catalog.Parse(data)
}

func known(name string) bool {
	_, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(name))
	return err == nil
}

func report(errs []error) {
	for _, err := range errs {
		fmt.Fprintln(os.Stderr, path+":", err)
	}
	if len(errs) > 0 {
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "catalog:", err)
	os.Exit(2)
}
