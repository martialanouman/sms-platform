package debt_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/martialanouman/sms-platform/internal/debt"
)

func repo() fstest.MapFS {
	return fstest.MapFS{
		"a.go":       {Data: []byte("package a\n\n// debt: #12\n")},
		"docs/b.md":  {Data: []byte("voir debt: #3 et debt: #12\n")},
		"c.md":       {Data: []byte("la marque s'écrit debt: #N\n")},
		"d.go":       {Data: []byte("// debt:#7\n")},
		".git/HEAD":  {Data: []byte("debt: #99\n")},
		"sub/.git/x": {Data: []byte("debt: #98\n")},
	}
}

func TestScanReadsEveryMarkWithItsLine(t *testing.T) {
	got, err := debt.Scan(repo())
	if err != nil {
		t.Fatal(err)
	}
	want := []debt.Mark{
		{Path: "a.go", Line: 3, Issue: 12},
		{Path: "d.go", Line: 1, Issue: 7},
		{Path: "docs/b.md", Line: 1, Issue: 3},
		{Path: "docs/b.md", Line: 1, Issue: 12},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Scan = %+v, want %+v", got, want)
	}
}

type issues map[int]bool

func (i issues) OpenDebt(context.Context) (map[int]bool, error) { return i, nil }

type failing struct{ err error }

func (f failing) OpenDebt(context.Context) (map[int]bool, error) { return nil, f.err }

func TestOrphansAreMarksWithoutAnOpenDebtIssue(t *testing.T) {
	got, err := debt.Orphans(context.Background(), repo(), issues{12: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []debt.Mark{{Path: "d.go", Line: 1, Issue: 7}, {Path: "docs/b.md", Line: 1, Issue: 3}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Orphans = %+v, want %+v", got, want)
	}
}

func TestOrphansFailsWhenIssuesCannotBeRead(t *testing.T) {
	boom := errors.New("api down")
	if _, err := debt.Orphans(context.Background(), repo(), failing{boom}); !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}
