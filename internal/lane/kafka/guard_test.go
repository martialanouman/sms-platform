package kafka_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/martialanouman/sms-platform/contracts"
	"github.com/martialanouman/sms-platform/internal/catalog"
)

const lanePkg = "github.com/martialanouman/sms-platform/internal/lane/kafka"

var categories = []string{"otp", "transactional", "marketing"}

// uses lists the groups consumed and the topics produced, per service, by the calls to NewLane and NewProducer under
// root's cmd/ and internal/.
type uses struct {
	groups   map[string]bool
	produced map[string]map[string]bool
}

func scan(root string) (uses, []error) {
	u := uses{groups: map[string]bool{}, produced: map[string]map[string]bool{}}
	var errs []error
	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			alias := ""
			for _, imp := range f.Imports {
				if p, _ := strconv.Unquote(imp.Path.Value); p == lanePkg {
					alias = "kafka"
					if imp.Name != nil {
						alias = imp.Name.Name
					}
				}
			}
			if alias == "" {
				return nil
			}
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if x, ok := sel.X.(*ast.Ident); !ok || x.Name != alias {
					return true
				}
				switch sel.Sel.Name {
				case "NewLane":
					names, err := literal(call.Args[1])
					if err != nil {
						errs = append(errs, fmt.Errorf("%s: NewLane: %w", path, err))
					}
					for _, g := range names {
						u.groups[g] = true
					}
				case "NewProducer":
					services, err := literal(call.Args[1])
					if err != nil || len(services) != 1 {
						errs = append(errs, fmt.Errorf("%s: NewProducer: the service must be a literal", path))
						return true
					}
					list, ok := call.Args[2].(*ast.CompositeLit)
					if !ok {
						errs = append(errs, fmt.Errorf("%s: NewProducer: the topics must be a literal list", path))
						return true
					}
					for _, e := range list.Elts {
						topics, err := literal(e)
						if err != nil {
							errs = append(errs, fmt.Errorf("%s: NewProducer: %w", path, err))
						}
						for _, t := range topics {
							if u.produced[services[0]] == nil {
								u.produced[services[0]] = map[string]bool{}
							}
							u.produced[services[0]][t] = true
						}
					}
				}
				return true
			})
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			errs = append(errs, err)
		}
	}
	return u, errs
}

// literal resolves a string literal, or a family "prefix." + category into its three names.
func literal(e ast.Expr) ([]string, error) {
	if b, ok := e.(*ast.BinaryExpr); ok && b.Op == token.ADD {
		if prefix, err := literal(b.X); err == nil && len(prefix) == 1 && strings.HasSuffix(prefix[0], ".") {
			if _, ok := b.Y.(*ast.Ident); ok {
				var out []string
				for _, c := range categories {
					out = append(out, prefix[0]+c)
				}
				return out, nil
			}
		}
	}
	if l, ok := e.(*ast.BasicLit); ok && l.Kind == token.STRING {
		s, err := strconv.Unquote(l.Value)
		return []string{s}, err
	}
	return nil, fmt.Errorf("%T is neither a string literal nor a category family", e)
}

// guard checks guards 1 and 2 of docs/spec/09-contrats.md §9.1, for every service with a cmd/ under root; it also
// returns the services of the catalog that have none yet.
func guard(root string, c catalog.Catalog) (errs []error, pending []string) {
	u, errs := scan(root)
	declared := map[string]catalog.Group{}
	for _, g := range c.Groups {
		declared[g.Name()] = g
	}
	for name := range u.groups {
		g, ok := declared[name]
		if !ok {
			errs = append(errs, fmt.Errorf("group %s is consumed but not declared", name))
		}
		for _, d := range g.Dependencies {
			if d.Park != "" {
				if u.produced[g.Service] == nil {
					u.produced[g.Service] = map[string]bool{}
				}
				u.produced[g.Service][d.Park] = true
			}
		}
	}
	for svc, topics := range u.produced {
		for t := range topics {
			if !slices.Contains(c.Topics[t].Producers, svc) {
				errs = append(errs, fmt.Errorf("%s produces %s without being one of its producers", svc, t))
			}
		}
	}
	services := map[string]bool{}
	for _, g := range c.Groups {
		services[g.Service] = true
	}
	for _, t := range c.Topics {
		for _, p := range t.Producers {
			services[p] = true
		}
	}
	for svc := range services {
		if _, err := os.Stat(filepath.Join(root, "cmd", svc)); err != nil {
			pending = append(pending, svc)
			continue
		}
		for name, g := range declared {
			if g.Service == svc && !u.groups[name] {
				errs = append(errs, fmt.Errorf("group %s is declared but %s never consumes it", name, svc))
			}
		}
		for name, t := range c.Topics {
			if slices.Contains(t.Producers, svc) && !u.produced[svc][name] {
				errs = append(errs, fmt.Errorf("%s is declared a producer of %s but never produces it", svc, name))
			}
		}
	}
	slices.Sort(pending)
	return errs, pending
}

func TestEveryTopicAndGroupOfTheCodeIsDeclaredAndUsed(t *testing.T) {
	c, err := catalog.Parse(contracts.Topics())
	if err != nil {
		t.Fatal(err)
	}
	errs, pending := guard("../../..", c)
	for _, err := range errs {
		t.Error(err)
	}
	t.Logf("services of the catalog without a cmd/ yet: %v", pending)
}

func TestTheGuardOfTopicsAndGroups(t *testing.T) {
	c, err := catalog.Parse([]byte(`
topics:
  mt.inbound.otp: {key: message_id, order: none, duplicate_unit: m, payload: sms.v1.A, body: none, partitions: 1, retention: 96h, producers: [front]}
  mt.billing-wait.otp: {key: message_id, order: none, duplicate_unit: m, payload: sms.v1.A, body: none, partitions: 1, retention: 96h, producers: [back]}
  mt.inbound.transactional: {key: message_id, order: none, duplicate_unit: m, payload: sms.v1.A, body: none, partitions: 1, retention: 96h, producers: [front]}
  mt.billing-wait.transactional: {key: message_id, order: none, duplicate_unit: m, payload: sms.v1.A, body: none, partitions: 1, retention: 96h, producers: [back]}
  mt.inbound.marketing: {key: message_id, order: none, duplicate_unit: m, payload: sms.v1.A, body: none, partitions: 1, retention: 96h, producers: [front]}
  mt.billing-wait.marketing: {key: message_id, order: none, duplicate_unit: m, payload: sms.v1.A, body: none, partitions: 1, retention: 96h, producers: [back]}
groups:
  - {topic: mt.inbound.otp, service: back, tenant: none, dependencies: {kafka: {mode: blocking}, billing: {mode: park_global, park: mt.billing-wait.otp}}, concurrency: per_partition, dead_letter: back.mt.inbound.otp.dead-letter, start: earliest}
  - {topic: mt.inbound.transactional, service: back, tenant: none, dependencies: {kafka: {mode: blocking}, billing: {mode: park_global, park: mt.billing-wait.transactional}}, concurrency: per_partition, dead_letter: back.mt.inbound.transactional.dead-letter, start: earliest}
  - {topic: mt.inbound.marketing, service: back, tenant: none, dependencies: {kafka: {mode: blocking}, billing: {mode: park_global, park: mt.billing-wait.marketing}}, concurrency: per_partition, dead_letter: back.mt.inbound.marketing.dead-letter, start: earliest}
`))
	if err != nil {
		t.Fatal(err)
	}
	const head = "package x\n\nimport lk \"" + lanePkg + "\"\n\n"
	good := map[string]string{
		"cmd/front/main.go":  head + `func f() { lk.NewProducer(c, "front", []string{"mt.inbound." + cat}) }`,
		"cmd/back/main.go":   head + `func f() { lk.NewLane(c, "back.mt.inbound." + cat, h, l) }`,
		"internal/x/x.go":    "package x\n\nfunc NewLane() {}\n",
		"cmd/back/x_test.go": head + `func f() { lk.NewLane(c, "undeclared", h, l) }`,
	}
	cases := map[string]struct {
		files map[string]string
		want  string
	}{
		"the real wiring":         {good, ""},
		"an undeclared group":     {map[string]string{"internal/x/x.go": head + `func f() { lk.NewLane(c, "back.nowhere", h, l) }`}, "back.nowhere is consumed but not declared"},
		"a producer not declared": {map[string]string{"internal/x/x.go": head + `func f() { lk.NewProducer(c, "back", []string{"mt.inbound.otp"}) }`}, "back produces mt.inbound.otp"},
		"a group never consumed":  {map[string]string{"cmd/front/main.go": good["cmd/front/main.go"], "cmd/back/main.go": head + `func f() { lk.NewLane(c, "back.mt.inbound.otp", h, l) }`}, "back.mt.inbound.marketing is declared but back never consumes it"},
		"a park never produced":   {map[string]string{"cmd/front/main.go": good["cmd/front/main.go"], "cmd/back/main.go": head + `func f() { lk.NewLane(c, "back.mt.inbound.otp", h, l) }`}, "back is declared a producer of mt.billing-wait.marketing but never produces it"},
		"a topic never produced":  {map[string]string{"cmd/back/main.go": good["cmd/back/main.go"], "cmd/front/main.go": head + `func f() { lk.NewProducer(c, "front", []string{"mt.inbound.otp"}) }`}, "front is declared a producer of mt.inbound.marketing but never produces it"},
		"a group in a variable":   {map[string]string{"internal/x/x.go": head + `func f() { lk.NewLane(c, name, h, l) }`}, "neither a string literal"},
		"topics in a variable":    {map[string]string{"internal/x/x.go": head + `func f() { lk.NewProducer(c, "front", topics) }`}, "topics must be a literal list"},
		"a service in a variable": {map[string]string{"internal/x/x.go": head + `func f() { lk.NewProducer(c, svc, []string{}) }`}, "service must be a literal"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			for path, src := range tc.files {
				if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, path), []byte(src), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			errs, _ := guard(root, c)
			got := fmt.Sprint(errs)
			if tc.want == "" && len(errs) > 0 || tc.want != "" && !strings.Contains(got, tc.want) {
				t.Errorf("guard = %v, want %q", errs, tc.want)
			}
		})
	}
}
