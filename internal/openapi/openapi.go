// Package openapi reads the OpenAPI contracts, checks their static rules and derives the Admin API scopes.
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

// SpecCodes reads the codes of the §9.3 table of docs/spec/09-contrats.md.
func SpecCodes(md []byte) []string {
	var codes []string
	in := false
	for line := range strings.SplitSeq(string(md), "\n") {
		if strings.HasPrefix(line, "## ") {
			in = strings.HasPrefix(line, "## 9.3 ")
			continue
		}
		cells := strings.Split(line, "|")
		if !in || len(cells) < 3 {
			continue
		}
		for _, m := range backquoted.FindAllStringSubmatch(cells[1], -1) {
			codes = append(codes, m[1])
		}
	}
	return codes
}

// Go renders the scope list as a gofmt-formatted Go source file.
func Go(scopes []string) []byte {
	var b bytes.Buffer
	b.WriteString("// Code generated by tools/openapi from contracts/openapi-admin.yaml. DO NOT EDIT.\n\n")
	b.WriteString("// Package scopes lists the scopes of the Admin API, generated from its contract.\npackage scopes\n\n")
	b.WriteString("// All is every scope, sorted.\nvar All = []string{\n")
	for _, scope := range scopes {
		fmt.Fprintf(&b, "\t%q,\n", scope)
	}
	b.WriteString("}\n")
	return b.Bytes()
}
