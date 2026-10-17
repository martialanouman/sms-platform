package probe

import "testing"

func TestExported(t *testing.T) {
	if Exported() != 1 {
		t.Fatal("want 1")
	}
}
