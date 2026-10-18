// Command openapi checks the static rules of the OpenAPI contracts, or writes the Admin API scope list.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

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
	errs := run(".", *gen)
	for _, err := range errs {
		fmt.Fprintln(os.Stderr, err)
	}
	if len(errs) > 0 {
		os.Exit(1)
	}
}

func run(root, gen string) []error {
	admin, err := load(filepath.Join(root, adminPath))
	if err != nil {
		return []error{err}
	}
	scopes, errs := admin.Scopes()
	errs = prefix(adminPath, errs)

	if gen != "" {
		if errs != nil {
			return errs
		}
		if err := os.WriteFile(gen, openapi.Go(scopes), 0o644); err != nil {
			return []error{err}
		}
		return nil
	}

	public, err := load(filepath.Join(root, publicPath))
	if err != nil {
		return []error{err}
	}
	spec, err := os.ReadFile(filepath.Join(root, specPath))
	if err != nil {
		return []error{err}
	}
	codes := openapi.SpecCodes(spec)
	errs = append(errs, prefix(adminPath, admin.UnknownCodes(codes))...)
	return append(errs, prefix(publicPath, append(public.PublicSecurity(), public.UnknownCodes(codes)...))...)
}

func load(path string) (openapi.Contract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return openapi.Contract{}, err
	}
	c, err := openapi.Parse(data)
	if err != nil {
		return openapi.Contract{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

func prefix(path string, errs []error) []error {
	for i, err := range errs {
		errs[i] = fmt.Errorf("%s: %w", path, err)
	}
	return errs
}
