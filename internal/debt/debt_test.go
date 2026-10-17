package debt_test

import (
	"context"
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/martialanouman/sms-platform/internal/debt"
)

const d = "debt"

var repo = fstest.MapFS{
	"a.go":      {Data: []byte("package a\n\n// " + d + ": #12\n")},
	"c.md":      {Data: []byte("la marque s'écrit debt: #N\n")},
	"d.go":      {Data: []byte("// " + d + ":#7\n")},
	"dir/x":     {Data: []byte(d + ": #98\n")},
	"docs/b.md": {Data: []byte("voir " + d + ": #3 et " + d + ": #12\n")},
	"e.md":      {Data: []byte(d + " : #8\n" + d + ":\u00a0#9\n" + strings.ToUpper(d) + ": #10\n")},
	"ln":        {Data: []byte("a.go"), Mode: fs.ModeSymlink},
}

var paths = []string{"a.go", "c.md", "d.go", "dir", "docs/b.md", "e.md", "ln"}

func TestScanReadsEveryMarkWithItsLine(t *testing.T) {
	got, err := debt.Scan(repo, paths)
	if err != nil {
		t.Fatal(err)
	}
	want := []debt.Mark{
		{Path: "a.go", Line: 3, Issue: 12},
		{Path: "d.go", Line: 1, Issue: 7},
		{Path: "docs/b.md", Line: 1, Issue: 3},
		{Path: "docs/b.md", Line: 1, Issue: 12},
		{Path: "e.md", Line: 1, Issue: 8},
		{Path: "e.md", Line: 2, Issue: 9},
		{Path: "e.md", Line: 3, Issue: 10},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Scan = %+v, want %+v", got, want)
	}
}

func TestScanLocatesAnIssueNumberOutOfRange(t *testing.T) {
	fsys := fstest.MapFS{"big.md": {Data: []byte("\n" + d + ": #99999999999999999999\n")}}
	if _, err := debt.Scan(fsys, []string{"big.md"}); err == nil || !strings.Contains(err.Error(), "big.md:2") {
		t.Errorf("err = %v, want it located at big.md:2", err)
	}
}

type issues map[int]bool

func (i issues) OpenDebt(context.Context) (map[int]bool, error) { return i, nil }

type failing struct{ err error }

func (f failing) OpenDebt(context.Context) (map[int]bool, error) { return nil, f.err }

func TestOrphansAreMarksWithoutAnOpenDebtIssue(t *testing.T) {
	got, err := debt.Orphans(context.Background(), repo, []string{"d.go", "docs/b.md"}, issues{12: true})
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
	if _, err := debt.Orphans(context.Background(), repo, paths, failing{boom}); !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}
