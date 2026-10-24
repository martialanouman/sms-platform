// Command openapi checks the static rules of the OpenAPI contracts, or writes the scope and permission lists.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/martialanouman/sms-platform/internal/dashboard/personal"
	"github.com/martialanouman/sms-platform/internal/openapi"
	platformscopes "github.com/martialanouman/sms-platform/internal/platform/scopes"
)

const (
	adminPath  = "contracts/openapi-admin.yaml"
	publicPath = "contracts/openapi-public.yaml"
	bffPath    = "contracts/openapi-bff.yaml"
	specPath   = "docs/spec/09-contrats.md"
)

func main() {
	gen := flag.String("gen", "", "write the scope and permission lists to this Go file")
	ts := flag.String("ts", "", "write the scope and permission lists to this TypeScript file")
	flag.Parse()
	errs := run(".", *gen, *ts)
	for _, err := range errs {
		fmt.Fprintln(os.Stderr, err)
	}
	if len(errs) > 0 {
		os.Exit(1)
	}
}

func run(root, gen, ts string) []error {
	admin, err := load(filepath.Join(root, adminPath))
	if err != nil {
		return []error{err}
	}
	bff, err := load(filepath.Join(root, bffPath))
	if err != nil {
		return []error{err}
	}
	scopes, errs := admin.Scopes()
	errs = prefix(adminPath, errs)
	permissions, permErrs := bff.Permissions(scopes)
	errs = append(errs, prefix(bffPath, permErrs)...)

	if gen != "" || ts != "" {
		if errs != nil {
			return errs
		}
		if gen != "" {
			if err := os.WriteFile(gen, openapi.Go(scopes, permissions), 0o600); err != nil {
				return []error{err}
			}
		}
		if ts != "" {
			if err := os.WriteFile(ts, openapi.TS(scopes, permissions, personal.Excluded, platformscopes.Sensitive), 0o600); err != nil {
				return []error{err}
			}
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
	rows, err := openapi.SpecErrors(spec)
	if err != nil {
		return []error{fmt.Errorf("%s: %w", specPath, err)}
	}
	var codes []string
	for _, r := range rows {
		codes = append(codes, r.Code)
	}
	errs = append(errs, prefix(adminPath, admin.UnknownCodes(codes))...)
	errs = append(errs, prefix(bffPath, bff.UnknownCodes(codes))...)
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
