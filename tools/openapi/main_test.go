package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tree(t *testing.T, path, old, new string) string {
	t.Helper()
	root := t.TempDir()
	for _, p := range []string{adminPath, publicPath, bffPath, specPath} {
		data, err := os.ReadFile(filepath.Join("..", "..", p))
		if err != nil {
			t.Fatal(err)
		}
		if p == path {
			if !strings.Contains(string(data), old) {
				t.Fatalf("%s does not contain %q", p, old)
			}
			data = []byte(strings.Replace(string(data), old, new, 1))
		}
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(p)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestRunPassesOnTheRealContracts(t *testing.T) {
	if errs := run(tree(t, "", "", ""), ""); errs != nil {
		t.Fatal(errs)
	}
}

func TestRunFailsOnAFaultyContract(t *testing.T) {
	for name, c := range map[string]struct{ path, old, new string }{
		"admin scope":     {adminPath, "OperatorBearer: ['customer-groups:read']", "OperatorBearer: ['customers:read']"},
		"admin code":      {adminPath, "            - unauthenticated\n", "            - unauthenticated\n            - made_up\n"},
		"public security": {publicPath, "      security:\n        - BearerApiKey: []\n", ""},
		"public code":     {publicPath, "            - unauthenticated\n", "            - unauthenticated\n            - made_up\n"},
		"spec code":       {specPath, "| `not_found`, ", "| "},
		"bff permission":  {bffPath, "session: ['operators:read']", "session: ['operators:admin']"},
		"bff proxy path":  {bffPath, "  /sessions:\n", "  /gw/sessions:\n"},
		"bff code":        {bffPath, "            - unauthenticated\n", "            - unauthenticated\n            - made_up\n"},
	} {
		t.Run(name, func(t *testing.T) {
			errs := run(tree(t, c.path, c.old, c.new), "")
			if len(errs) == 0 || !strings.HasPrefix(errs[0].Error(), strings.Replace(c.path, specPath, adminPath, 1)) {
				t.Fatalf("errs = %v, want an error on %s", errs, c.path)
			}
		})
	}
}

func TestRunDoesNotGenerateFromAFaultyContract(t *testing.T) {
	for name, c := range map[string]struct{ path, old, new string }{
		"admin": {adminPath, "OperatorBearer: ['customer-groups:read']", "OperatorBearer: ['customers:read']"},
		"bff":   {bffPath, "session: ['operators:read']", "session: ['operators:admin']"},
	} {
		t.Run(name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "scopes.gen.go")
			if errs := run(tree(t, c.path, c.old, c.new), out); len(errs) == 0 {
				t.Fatal("run accepted a faulty contract")
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Errorf("scope list written despite the fault: %v", err)
			}
		})
	}
}
