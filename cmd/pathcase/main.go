// Command pathcase fails when two tracked paths differ only by letter case.
// They are distinct on Linux but collide on case-insensitive filesystems
// (macOS, Windows): one file shadows the other and a checkout there breaks.
//
// Usage: go run ./cmd/pathcase (from anywhere in the repository).
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pathcase:", err)
		os.Exit(1)
	}
}

func run() error {
	root, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	out, err := git("-C", strings.TrimSpace(string(root)), "ls-files", "-z")
	if err != nil {
		return err
	}
	var files []string
	for _, f := range bytes.Split(out, []byte{0}) {
		if len(f) > 0 {
			files = append(files, string(f))
		}
	}

	collisions := collide(files)
	for _, c := range collisions {
		fmt.Fprintf(os.Stderr, "case collision: %s\n", strings.Join(c, "  <->  "))
	}
	if len(collisions) > 0 {
		fmt.Fprintf(os.Stderr, "\n%d name(s) differ only by case; rename one side so the repo works on case-insensitive filesystems.\n", len(collisions))
		os.Exit(1)
	}
	fmt.Printf("No case collisions among %d tracked files.\n", len(files))
	return nil
}

func git(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

// collide returns each group of distinct names, among the files and every
// directory above them, that fold to the same key. Groups and their members
// are sorted.
func collide(files []string) [][]string {
	folded := map[string]map[string]struct{}{}
	for _, file := range files {
		parts := strings.Split(file, "/")
		for i := 1; i <= len(parts); i++ {
			name := strings.Join(parts[:i], "/")
			key := strings.ToLower(norm.NFC.String(name))
			if folded[key] == nil {
				folded[key] = map[string]struct{}{}
			}
			folded[key][name] = struct{}{}
		}
	}
	var groups [][]string
	for _, names := range folded {
		if len(names) < 2 {
			continue
		}
		group := make([]string, 0, len(names))
		for n := range names {
			group = append(group, n)
		}
		sort.Strings(group)
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i][0] < groups[j][0] })
	return groups
}
