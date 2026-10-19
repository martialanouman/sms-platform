// Command release prints the version that the commits since the last release call for, or nothing.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/martialanouman/sms-platform/internal/release"
)

func main() {
	tags, err := exec.Command("git", "tag", "--list", "v*", "--merged", "HEAD", "--sort=-v:refname").Output()
	if err != nil {
		fail(err)
	}
	current, _, _ := strings.Cut(strings.TrimSpace(string(tags)), "\n")
	since := "HEAD"
	if current != "" {
		since = current + "..HEAD"
	}
	log, err := exec.Command("git", "log", "--format=%B%x00", since).Output()
	if err != nil {
		fail(err)
	}
	var commits []string
	for _, c := range strings.Split(string(log), "\x00") {
		if c = strings.TrimSpace(c); c != "" {
			commits = append(commits, c)
		}
	}
	next, ok, err := release.Next(current, commits)
	if err != nil {
		fail(err)
	}
	if ok {
		fmt.Println(next)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "release:", err)
	os.Exit(1)
}
