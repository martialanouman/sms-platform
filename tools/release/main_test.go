package main

import (
	"os/exec"
	"testing"
)

func repo(t *testing.T) func(args ...string) {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid",
			"-c", "commit.gpgSign=false", "-c", "tag.gpgSign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	t.Chdir(dir)
	return git
}

func TestNextReadsTheTagsAndTheCommitsSinceTheLast(t *testing.T) {
	git := repo(t)
	commit := func(msg string) { git("commit", "-q", "--allow-empty", "-m", msg) }
	expect := func(want string) {
		t.Helper()
		if got, err := next(); err != nil || got != want {
			t.Errorf("next() = %q, %v; want %q", got, err, want)
		}
	}
	commit("feat: a")
	expect("v0.1.0")
	git("tag", "v0.1.0")
	git("tag", "v0.2.0")
	git("tag", "v0.9.0")
	commit("fix: b")
	git("tag", "v0.11.0")
	commit("docs: c\n\nBREAKING CHANGE: a docs commit ships nothing")
	expect("")
	commit("fix: d")
	expect("v0.11.1")
	commit("fix: e\n\nBREAKING CHANGE: f")
	expect("v0.12.0")
	git("switch", "-q", "-c", "elsewhere")
	commit("feat: g")
	git("tag", "v0.20.0")
	git("switch", "-q", "main")
	expect("v0.12.0")
}
