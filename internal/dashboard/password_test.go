package dashboard_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/martialanouman/sms-platform/internal/dashboard"
)

func TestCheckPasswordNamesEachMissingRule(t *testing.T) {
	for password, want := range map[string][]string{
		"Correct-horse-9":  nil,
		"Short-9a":         {"douze caractères au moins"},
		"lower-case-only9": {"une majuscule"},
		"UPPER-CASE-ONLY9": {"une minuscule"},
		"No-digits-at-all": {"un chiffre"},
		"NoSpecials12345":  {"un caractère spécial"},
		"Éléphant-rosé-9":  nil,
		"Eleven-cha9":      {"douze caractères au moins"},
		"Twelve-chr9x":     nil,
		"Correct+horse9":   nil,
		"Correct horse9":   nil,
		"":                 {"douze caractères au moins", "une majuscule", "une minuscule", "un chiffre", "un caractère spécial"},
	} {
		if got := dashboard.CheckPassword(password); !slices.Equal(got, want) {
			t.Errorf("%q: missing %q, want %q", password, got, want)
		}
	}
}

func TestTheHasherVerifiesWhatItHashed(t *testing.T) {
	h := dashboard.NewHasher()
	ctx := context.Background()
	phc, err := h.Hash(ctx, "Correct-horse-9")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(phc, "$argon2id$v=19$m=65536,t=3,p=4$") || strings.Contains(phc, "Correct-horse-9") {
		t.Fatalf("phc = %q", phc)
	}
	if again, _ := h.Hash(ctx, "Correct-horse-9"); again == phc {
		t.Error("two hashes of one password are equal: no salt")
	}
	for password, want := range map[string]bool{"Correct-horse-9": true, "Correct-horse-8": false, "": false} {
		if ok, err := h.Verify(ctx, phc, password); err != nil || ok != want {
			t.Errorf("verify %q = %v, %v; want %v", password, ok, err, want)
		}
	}
}

func TestTheHasherRefusesAHashItCannotTrust(t *testing.T) {
	h := dashboard.NewHasher()
	valid, err := h.Hash(context.Background(), "Correct-horse-9")
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Split(valid, "$")
	for name, phc := range map[string]string{
		"empty":           "",
		"bcrypt":          "$2a$10$abcdefghijklmnopqrstuuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ012",
		"argon2i":         strings.Replace(valid, "$argon2id$", "$argon2i$", 1),
		"other version":   strings.Replace(valid, "$v=19$", "$v=16$", 1),
		"huge memory":     strings.Join([]string{"", fields[1], fields[2], "m=4194304,t=3,p=4", fields[4], fields[5]}, "$"),
		"many passes":     strings.Join([]string{"", fields[1], fields[2], "m=65536,t=99,p=4", fields[4], fields[5]}, "$"),
		"no lanes":        strings.Join([]string{"", fields[1], fields[2], "m=65536,t=3,p=0", fields[4], fields[5]}, "$"),
		"trailing params": strings.Join([]string{"", fields[1], fields[2], "m=65536,t=3,p=4,x=1", fields[4], fields[5]}, "$"),
		"short salt":      strings.Join([]string{"", fields[1], fields[2], fields[3], "c2FsdA", fields[5]}, "$"),
		"no key":          strings.Join([]string{"", fields[1], fields[2], fields[3], fields[4], ""}, "$"),
		"salt not base64": strings.Join([]string{"", fields[1], fields[2], fields[3], "!!!!!!!!!!!!", fields[5]}, "$"),
	} {
		if ok, err := h.Verify(context.Background(), phc, "Correct-horse-9"); err == nil || ok {
			t.Errorf("%s: verify = %v, %v; want a refusal", name, ok, err)
		} else if errors.Is(err, dashboard.ErrBusy) || strings.Contains(err.Error(), "Correct-horse-9") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
