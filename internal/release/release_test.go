package release_test

import (
	"testing"

	"github.com/martialanouman/sms-platform/internal/release"
)

func TestNext(t *testing.T) {
	for _, tc := range []struct {
		name, current string
		commits       []string
		want          string
	}{
		{"first release", "", []string{"feat(deploy): images (#104)"}, "v0.1.0"},
		{"first release from a fix", "", []string{"fix: x"}, "v0.1.0"},
		{"feat bumps the minor", "v0.1.3", []string{"feat: x"}, "v0.2.0"},
		{"fix bumps the patch", "v0.1.3", []string{"fix(deploy): x"}, "v0.1.4"},
		{"perf bumps the patch", "v0.1.3", []string{"perf: x"}, "v0.1.4"},
		{"refactor bumps the patch", "v0.1.3", []string{"refactor: x"}, "v0.1.4"},
		{"build bumps the patch", "v0.1.3", []string{"build(deps): bump x"}, "v0.1.4"},
		{"revert bumps the patch", "v0.1.3", []string{`revert: "feat: x"`}, "v0.1.4"},
		{"bang breaks, minor in 0.x", "v0.4.2", []string{"feat(api)!: x"}, "v0.5.0"},
		{"footer breaks, minor in 0.x", "v0.4.2", []string{"fix: x\n\nBREAKING CHANGE: y"}, "v0.5.0"},
		{"hyphenated footer breaks", "v1.4.2", []string{"fix: x\n\nBREAKING-CHANGE: y"}, "v2.0.0"},
		{"bang breaks, major from 1.x", "v1.4.2", []string{"refactor!: x"}, "v2.0.0"},
		{"strongest bump wins", "v1.4.2", []string{"fix: a", "feat: b", "docs: c"}, "v1.5.0"},
		{"breaking beats feat", "v1.4.2", []string{"feat: a", "fix!: b"}, "v2.0.0"},
		{"breaking stays when a feat follows", "v1.4.2", []string{"fix!: a", "feat: b"}, "v2.0.0"},
		{"feat stays when a fix follows", "v1.4.2", []string{"feat: a", "fix: b"}, "v1.5.0"},
		{"breaking change named in a title is a fix", "v0.1.3", []string{"fix: parse the BREAKING CHANGE: footer"}, "v0.1.4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok, err := release.Next(tc.current, tc.commits); err != nil || !ok || got != tc.want {
				t.Errorf("Next(%q, %q) = %q, %v, %v; want %q", tc.current, tc.commits, got, ok, err, tc.want)
			}
		})
	}
}

func TestNextPublishesNothingForWhatLeavesTheBinaryAlone(t *testing.T) {
	for _, commits := range [][]string{
		nil,
		{"docs(steps): x", "chore: y", "ci: z", "test: t", "style: s"},
		{"Merge branch 'x'", "feature: not conventional", "fixup! fix: x"},
		{"docs: x\n\nBREAKING CHANGE: a docs commit breaks nothing it ships"},
	} {
		if got, ok, err := release.Next("v0.3.1", commits); err != nil || ok {
			t.Errorf("Next(%q) = %q, %v, %v; want nothing to publish", commits, got, ok, err)
		}
	}
}

func TestNextPublishesNothingYetWithoutAChangeToShip(t *testing.T) {
	if got, ok, err := release.Next("", []string{"docs: x", "ci: y"}); err != nil || ok {
		t.Errorf("Next of an untagged repository with docs only = %q, %v, %v; want nothing", got, ok, err)
	}
}

func TestNextRefusesATagItCannotRead(t *testing.T) {
	for _, current := range []string{"0.1.0", "v1.2", "v1.2.3-rc.1", "vx.1.2", "release-v1.2.3", "v01.2.3"} {
		if got, _, err := release.Next(current, []string{"feat: x"}); err == nil {
			t.Errorf("Next(%q) = %q, want a refusal", current, got)
		}
	}
}
