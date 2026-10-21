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
	if errs := run(tree(t, "", "", ""), "", ""); errs != nil {
		t.Fatal(errs)
	}
}

func TestRunFailsOnAFaultyContract(t *testing.T) {
	for name, c := range map[string]struct{ path, old, new string }{
		"admin scope":     {adminPath, "OperatorBearer: ['customer-groups:read']", "OperatorBearer: ['customers:read']"},
		"admin code":      {adminPath, "            - unauthenticated\n", "            - unauthenticated\n            - made_up\n"},
		"public security": {publicPath, "      security:\n        - BearerApiKey: []\n", ""},
		"public code":     {publicPath, "            - unauthenticated\n", "            - unauthenticated\n            - made_up\n"},
		"spec code":       {specPath, "| `payload_too_large` |", "| |"},
		"bff permission":  {bffPath, "session: ['operators:read']", "session: ['operators:admin']"},
		"bff proxy path":  {bffPath, "  /sessions:\n", "  /gw/sessions:\n"},
		"bff code":        {bffPath, "            - unauthenticated\n", "            - unauthenticated\n            - made_up\n"},
	} {
		t.Run(name, func(t *testing.T) {
			errs := run(tree(t, c.path, c.old, c.new), "", "")
			if len(errs) == 0 || !strings.HasPrefix(errs[0].Error(), strings.Replace(c.path, specPath, adminPath, 1)) {
				t.Fatalf("errs = %v, want an error on %s", errs, c.path)
			}
		})
	}
}

func TestRunFailsOnAMalformedSpecRow(t *testing.T) {
	errs := run(tree(t, specPath, "| 404, 409, 403 |", "| 404, 409 |"), "", "")
	if len(errs) != 1 || !strings.HasPrefix(errs[0].Error(), specPath) {
		t.Fatalf("errs = %v, want one error on %s", errs, specPath)
	}
}

func TestRunDoesNotGenerateFromAFaultyContract(t *testing.T) {
	for name, c := range map[string]struct{ path, old, new string }{
		"admin": {adminPath, "OperatorBearer: ['customer-groups:read']", "OperatorBearer: ['customers:read']"},
		"bff":   {bffPath, "session: ['operators:read']", "session: ['operators:admin']"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			out, ts := filepath.Join(dir, "scopes.gen.go"), filepath.Join(dir, "scopes.gen.ts")
			if errs := run(tree(t, c.path, c.old, c.new), out, ts); len(errs) == 0 {
				t.Fatal("run accepted a faulty contract")
			}
			for _, f := range []string{out, ts} {
				if _, err := os.Stat(f); !os.IsNotExist(err) {
					t.Errorf("%s written despite the fault: %v", f, err)
				}
			}
		})
	}
}

func TestRunWritesTheTypeScriptList(t *testing.T) {
	ts := filepath.Join(t.TempDir(), "scopes.gen.ts")
	if errs := run(tree(t, "", "", ""), "", ts); errs != nil {
		t.Fatal(errs)
	}
	data, err := os.ReadFile(ts)
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"'audit:read'", "'gateway:personal_token'"} {
		if !strings.Contains(string(data), scope) {
			t.Errorf("scopes.gen.ts lacks %s", scope)
		}
	}
}
