// Package debt finds the debt marks of a repository and the ones no open debt issue backs.
package debt

import (
	"bytes"
	"context"
	"io/fs"
	"regexp"
	"strconv"
)

var markPattern = regexp.MustCompile(`debt:\s*#(\d+)`)

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

// Scan returns every mark of fsys, in path then line order, outside .git directories.
func Scan(fsys fs.FS) ([]Mark, error) {
	var marks []Mark
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		for i, line := range bytes.Split(data, []byte("\n")) {
			for _, m := range markPattern.FindAllSubmatch(line, -1) {
				issue, err := strconv.Atoi(string(m[1]))
				if err != nil {
					return err
				}
				marks = append(marks, Mark{Path: path, Line: i + 1, Issue: issue})
			}
		}
		return nil
	})
	return marks, err
}

// Orphans returns the marks of fsys that no open debt issue backs.
func Orphans(ctx context.Context, fsys fs.FS, issues Issues) ([]Mark, error) {
	marks, err := Scan(fsys)
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
