// Package release computes the next version of the repository from its conventional commits.
package release

import (
	"fmt"
	"regexp"
	"strconv"
)

var (
	tag      = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	title    = regexp.MustCompile(`^([a-z]+)(\([a-z0-9-]+\))?(!)?: `)
	breaking = regexp.MustCompile(`(?m)^BREAKING[ -]CHANGE: `)
	patches  = map[string]bool{"fix": true, "perf": true, "refactor": true, "build": true, "revert": true}
)

const (
	none = iota
	patch
	minor
	major
)

// Next is the version after current, empty for none yet, that commits since it call for; false when none ships a change.
func Next(current string, commits []string) (string, bool, error) {
	v := [3]int{0, 0, 0}
	if current != "" {
		m := tag.FindStringSubmatch(current)
		if m == nil {
			return "", false, fmt.Errorf("tag %q is not vMAJOR.MINOR.PATCH", current)
		}
		for i := range v {
			v[i], _ = strconv.Atoi(m[i+1])
		}
	}
	bump := none
	for _, c := range commits {
		m := title.FindStringSubmatch(c)
		if m == nil || m[1] != "feat" && !patches[m[1]] {
			continue
		}
		switch {
		case m[3] == "!" || breaking.MatchString(c):
			bump = major
		case m[1] == "feat":
			bump = max(bump, minor)
		default:
			bump = max(bump, patch)
		}
	}
	switch {
	case bump == none:
		return "", false, nil
	case current == "":
		return "v0.1.0", true, nil
	case bump == major && v[0] > 0:
		v = [3]int{v[0] + 1, 0, 0}
	case bump >= minor:
		v = [3]int{v[0], v[1] + 1, 0}
	default:
		v[2]++
	}
	return fmt.Sprintf("v%d.%d.%d", v[0], v[1], v[2]), true, nil
}
