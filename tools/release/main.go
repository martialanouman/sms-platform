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
	v, err := next()
	if err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
	if v != "" {
		fmt.Println(v)
	}
}

func next() (string, error) {
	tags, err := exec.Command("git", "tag", "--list", "v*", "--merged", "HEAD", "--sort=-v:refname").Output()
	if err != nil {
		return "", err
	}
	current, _, _ := strings.Cut(strings.TrimSpace(string(tags)), "\n")
	since := "HEAD"
	if current != "" {
		since = current + "..HEAD"
	}
	log, err := exec.Command("git", "log", "--format=%B%x00", since).Output()
	if err != nil {
		return "", err
	}
	var commits []string
	for _, c := range strings.Split(string(log), "\x00") {
		if c = strings.TrimSpace(c); c != "" {
			commits = append(commits, c)
		}
	}
	v, _, err := release.Next(current, commits)
	return v, err
}
