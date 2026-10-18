// Package openapi reads the OpenAPI contracts, checks their static rules and derives the Admin API scopes and
// the BFF permissions.
package openapi

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

var sensitive = map[string][]string{
	"routing-scripts": {"publish"},
	"suppressions":    {"delete"},
	"billing":         {"topup"},
	"content":         {"erase"},
	"gdpr":            {"erase"},
	"cdr":             {"export_bulk"},
}

const reveal = "msisdn:reveal"

var permissions = []string{"gateway:personal_token", "operators:read", "operators:write", "roles:read", "roles:write", "sessions:read", "sessions:write"}

var public = []string{"health", "login", "logout", "setPasswordFromAccessLink"}

// Contract is the part of an OpenAPI contract the static rules read.
type Contract struct {
	Tags       []Tag               `yaml:"tags"`
	Paths      map[string]PathItem `yaml:"paths"`
	Components struct {
		Schemas struct {
			Error struct {
				Properties struct {
					Code struct {
						Enum []string `yaml:"enum"`
					} `yaml:"code"`
				} `yaml:"properties"`
			} `yaml:"Error"`
			ErrorCode struct {
				Enum []string `yaml:"enum"`
			} `yaml:"ErrorCode"`
		} `yaml:"schemas"`
	} `yaml:"components"`
}

// Tag is a tag declared by a contract.
type Tag struct {
	Name string `yaml:"name"`
}

// PathItem holds the operations of one path.
type PathItem struct {
	Get     *Operation `yaml:"get"`
	Put     *Operation `yaml:"put"`
	Post    *Operation `yaml:"post"`
	Delete  *Operation `yaml:"delete"`
	Patch   *Operation `yaml:"patch"`
	Head    *Operation `yaml:"head"`
	Options *Operation `yaml:"options"`
	Trace   *Operation `yaml:"trace"`
}

// Operation is one operation of a contract.
type Operation struct {
	ID       string                `yaml:"operationId"`
	Tags     []string              `yaml:"tags"`
	Security []map[string][]string `yaml:"security"`
}

// Parse decodes a contract.
func Parse(data []byte) (Contract, error) {
	var c Contract
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Contract{}, err
	}
	return c, nil
}

func (c Contract) check(rule func(Operation) error) []error {
	if len(c.Paths) == 0 {
		return []error{errors.New("no operation")}
	}
	var errs []error
	for path, p := range c.Paths {
		for _, op := range []*Operation{p.Get, p.Put, p.Post, p.Delete, p.Patch, p.Head, p.Options, p.Trace} {
			if op == nil {
				continue
			}
			err := rule(*op)
			if op.ID == "" {
				err = errors.New("missing operationId")
			}
			if err != nil {
				errs = append(errs, fmt.Errorf("%s %s: %w", path, op.ID, err))
			}
		}
	}
	return errs
}

// Scopes checks that every operation declares exactly one scope of its only, declared tag, and lists the
// scopes, sorted.
func (c Contract) Scopes() ([]string, []error) {
	all := []string{reveal}
	errs := c.check(func(op Operation) error {
		scope, err := c.scopeOf(op)
		if err == nil && !slices.Contains(all, scope) {
			all = append(all, scope)
		}
		return err
	})
	if errs != nil {
		return nil, errs
	}
	slices.Sort(all)
	return all, nil
}

func (c Contract) scopeOf(op Operation) (string, error) {
	if len(op.Tags) != 1 {
		return "", fmt.Errorf("%d tags, want exactly one", len(op.Tags))
	}
	if !slices.Contains(c.Tags, Tag{Name: op.Tags[0]}) {
		return "", fmt.Errorf("tag %q is not declared in tags", op.Tags[0])
	}
	if len(op.Security) != 1 || len(op.Security[0]) != 1 || len(op.Security[0]["OperatorBearer"]) != 1 {
		return "", fmt.Errorf("security %v, want exactly one OperatorBearer scope", op.Security)
	}
	scope := op.Security[0]["OperatorBearer"][0]
	tag, action, _ := strings.Cut(scope, ":")
	if tag != op.Tags[0] || !slices.Contains(append([]string{"read", "write"}, sensitive[tag]...), action) {
		return "", fmt.Errorf("scope %q is not an action of tag %q", scope, op.Tags[0])
	}
	return scope, nil
}

// PublicSecurity checks that every operation declares the API key alone, without scope.
func (c Contract) PublicSecurity() []error {
	return c.check(func(op Operation) error {
		s := op.Security
		if len(s) != 1 || len(s[0]) != 1 || s[0]["BearerApiKey"] == nil || len(s[0]["BearerApiKey"]) != 0 {
			return fmt.Errorf("security %v, want [{BearerApiKey: []}]", s)
		}
		return nil
	})
}

// Permissions checks that every BFF operation declares the session with at most one permission or Admin API
// scope, or no security at all for the public ones, that every BFF permission is used, that no path falls
// under the proxy's /gw, and lists the BFF permissions, sorted.
func (c Contract) Permissions(scopes []string) ([]string, []error) {
	var used []string
	errs := c.check(func(op Operation) error {
		perm, err := permissionOf(op, scopes)
		used = append(used, perm)
		return err
	})
	for path := range c.Paths {
		if path == "/gw" || strings.HasPrefix(path, "/gw/") {
			errs = append(errs, fmt.Errorf("%s: under /gw, the proxy to the Admin API", path))
		}
	}
	for _, perm := range permissions {
		if slices.Contains(scopes, perm) {
			errs = append(errs, fmt.Errorf("permission %q is also an Admin API scope", perm))
		}
		if !slices.Contains(used, perm) {
			errs = append(errs, fmt.Errorf("permission %q is declared by no operation", perm))
		}
	}
	if errs != nil {
		return nil, errs
	}
	return slices.Clone(permissions), nil
}

func permissionOf(op Operation, scopes []string) (string, error) {
	s := op.Security
	if s != nil && len(s) == 0 {
		if !slices.Contains(public, op.ID) {
			return "", errors.New("security [] on an operation that is not public")
		}
		return "", nil
	}
	if len(s) != 1 || len(s[0]) != 1 || s[0]["session"] == nil || len(s[0]["session"]) > 1 {
		return "", fmt.Errorf("security %v, want [{session: []}] or [{session: [<permission>]}]", s)
	}
	if len(s[0]["session"]) == 0 {
		return "", nil
	}
	perm := s[0]["session"][0]
	if !slices.Contains(permissions, perm) && !slices.Contains(scopes, perm) {
		return "", fmt.Errorf("%q is neither a BFF permission nor an Admin API scope", perm)
	}
	return perm, nil
}

// UnknownCodes lists the error codes of the contract missing from known; a contract without codes is an error.
func (c Contract) UnknownCodes(known []string) []error {
	s := c.Components.Schemas
	codes := append(s.Error.Properties.Code.Enum, s.ErrorCode.Enum...)
	if len(codes) == 0 {
		return []error{errors.New("no error code: Error.code and ErrorCode have no enum")}
	}
	var errs []error
	for _, code := range codes {
		if !slices.Contains(known, code) {
			errs = append(errs, fmt.Errorf("error code %q is not in docs/spec/09-contrats.md §9.3", code))
		}
	}
	return errs
}

var backquoted = regexp.MustCompile("`([^`]+)`")

// SpecError is a code of the §9.3 table with its HTTP status and SMPP command_status names, empty when it has none.
type SpecError struct {
	Code, HTTP, SMPP string
}

// SpecErrors reads the rows of the §9.3 table of docs/spec/09-contrats.md, one per code.
func SpecErrors(md []byte) ([]SpecError, error) {
	var rows []SpecError
	in := false
	for line := range strings.SplitSeq(string(md), "\n") {
		if strings.HasPrefix(line, "## ") {
			in = strings.HasPrefix(line, "## 9.3 ")
			continue
		}
		cells := strings.Split(line, "|")
		if !in || len(cells) < 4 {
			continue
		}
		var codes []string
		for _, m := range backquoted.FindAllStringSubmatch(cells[1], -1) {
			codes = append(codes, m[1])
		}
		http, smpp := statuses(cells[2], len(codes)), statuses(cells[3], len(codes))
		if http == nil || smpp == nil {
			return nil, fmt.Errorf("§9.3: %d codes but a status count that matches neither one nor them: %s", len(codes), line)
		}
		for i, code := range codes {
			rows = append(rows, SpecError{Code: code, HTTP: http[i], SMPP: smpp[i]})
		}
	}
	return rows, nil
}

func statuses(cell string, n int) []string {
	cell = strings.TrimSpace(strings.ReplaceAll(cell, "`", ""))
	if cell == "—" {
		cell = ""
	}
	values := strings.Split(cell, ", ")
	if len(values) == 1 {
		return slices.Repeat(values, n)
	}
	if len(values) != n {
		return nil
	}
	return values
}

// Go renders the scope and permission lists as a gofmt-formatted Go source file.
func Go(scopes, permissions []string) []byte {
	var b bytes.Buffer
	b.WriteString("// Code generated by tools/openapi from contracts/openapi-admin.yaml and contracts/openapi-bff.yaml. DO NOT EDIT.\n\n")
	b.WriteString("// Package scopes lists the scopes of the Admin API and the permissions of the dashboard BFF, generated from their contracts.\npackage scopes\n\n")
	list(&b, "// All is every scope, sorted.\nvar All", scopes)
	b.WriteString("\n")
	list(&b, "// Permissions is every permission of the dashboard BFF, sorted.\nvar Permissions", permissions)
	return b.Bytes()
}

func list(b *bytes.Buffer, decl string, values []string) {
	b.WriteString(decl + " = []string{\n")
	for _, v := range values {
		fmt.Fprintf(b, "\t%q,\n", v)
	}
	b.WriteString("}\n")
}
