// Package debt finds the debt marks of a repository and the ones no open debt issue backs.
package debt

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"regexp"
	"strconv"
)

var markPattern = regexp.MustCompile(`(?i)debt[\s\p{Zs}]*:[\s\p{Zs}]*#(\d+)`)

// Mark is a `debt: #N` found in a file.
type Mark struct {
	Path  string
	Line  int
	Issue int
}

// Issues lists the open issues labelled debt.
type Issues interface {
	OpenDebt(ctx context.Context) (map[int]bool, error)
}

// Scan returns the marks of the regular files among paths, in the given order.
func Scan(fsys fs.FS, paths []string) ([]Mark, error) {
	var marks []Mark
	for _, path := range paths {
		info, err := fs.Lstat(fsys, path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil, err
		}
		for i, line := range bytes.Split(data, []byte("\n")) {
			for _, m := range markPattern.FindAllSubmatch(line, -1) {
				issue, err := strconv.Atoi(string(m[1]))
				if err != nil {
					return nil, fmt.Errorf("%s:%d: %w", path, i+1, err)
				}
				marks = append(marks, Mark{Path: path, Line: i + 1, Issue: issue})
			}
		}
	}
	return marks, nil
}

// Orphans returns the marks of paths that no open debt issue backs.
func Orphans(ctx context.Context, fsys fs.FS, paths []string, issues Issues) ([]Mark, error) {
	marks, err := Scan(fsys, paths)
	if err != nil {
		return nil, err
	}
	open, err := issues.OpenDebt(ctx)
	if err != nil {
		return nil, err
	}
	var orphans []Mark
	for _, m := range marks {
		if !open[m.Issue] {
			orphans = append(orphans, m)
		}
	}
	return orphans, nil
}
