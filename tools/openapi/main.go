// Command openapi checks the static rules of the OpenAPI contracts, or writes the Admin API scope list.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/martialanouman/sms-platform/internal/openapi"
)

const (
	adminPath  = "contracts/openapi-admin.yaml"
	publicPath = "contracts/openapi-public.yaml"
	specPath   = "docs/spec/09-contrats.md"
)

func main() {
	gen := flag.String("gen", "", "write the Admin API scope list to this file")
	flag.Parse()

	admin, public := load(adminPath), load(publicPath)
	scopes, errs := admin.Scopes()
	report(adminPath, errs)

	if *gen != "" {
		src, err := scopes.Go()
		if err != nil {
			fail(err)
		}
		if err := os.WriteFile(*gen, src, 0o644); err != nil {
			fail(err)
		}
		return
	}

	spec, err := os.ReadFile(specPath)
	if err != nil {
		fail(err)
	}
	codes := openapi.SpecCodes(spec)
	report(adminPath, admin.UnknownCodes(codes))
	report(publicPath, append(public.PublicSecurity(), public.UnknownCodes(codes)...))
}

func load(path string) openapi.Contract {
	data, err := os.ReadFile(path)
	if err != nil {
		fail(err)
	}
	c, err := openapi.Parse(data)
	if err != nil {
		fail(fmt.Errorf("%s: %w", path, err))
	}
	return c
}

func report(path string, errs []error) {
	for _, err := range errs {
		fmt.Fprintln(os.Stderr, path+":", err)
	}
	if len(errs) > 0 {
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "openapi:", err)
	os.Exit(2)
}
