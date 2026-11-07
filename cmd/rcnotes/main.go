// Command rcnotes writes the changelog a prerelease carries: the changes since
// the previous release or prerelease, as Markdown, from the first-parent
// commit subjects (the squash merges' PR titles, in Conventional Commit form).
//
//	go run ./cmd/rcnotes -exclude v0.3.0-rc.5 >rc-changes.md
//	go run ./cmd/rcnotes -since v0.3.0-rc.3 -ref v0.3.0-rc.4
//
// The previous tag is the nearest v* tag in the history of -ref (fewest
// first-parent commits away, the tag named by -exclude never): for a nightly
// that is the last nightly or release on main, for an rc dispatched from a
// release PR's branch the last rc cut from main or the branch. -since
// overrides it. With no earlier tag nothing is written (the caller keeps its
// own text); with no commits since it, one line says so.
//
// Features, fixes, performance changes, reverts and breaking changes are
// listed, newest first; docs, CI, tests, build, refactor, chore and style
// commits are one count at the end, since they change nothing a tester can
// see. At most -max entries are listed. The publish job of release.yml runs
// it (it reads git only: no network).
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// gitFunc runs git with args and returns its standard output.
type gitFunc func(args ...string) (string, error)

func main() {
	if err := run(os.Args[1:], runGit, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "rcnotes:", err)
		os.Exit(1)
	}
}

func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func run(args []string, git gitFunc, stdout io.Writer) error {
	fs := flag.NewFlagSet("rcnotes", flag.ContinueOnError)
	since := fs.String("since", "", "the previous tag (default: the nearest v* tag in -ref's history)")
	ref := fs.String("ref", "HEAD", "the commit the notes go up to")
	exclude := fs.String("exclude", "", "a tag that is never the previous one (the tag being built)")
	limit := fs.Int("max", 30, "most entries to list; the rest is counted")
	out := fs.String("out", "", "file to write (default: standard output)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *limit < 1 {
		return errors.New("-max must be at least 1")
	}
	prev := *since
	if prev == "" {
		var err error
		if prev, err = previousTag(git, *ref, *exclude); err != nil {
			return err
		}
		if prev == "" {
			fmt.Fprintf(os.Stderr, "rcnotes: no earlier v* tag in the history of %s: nothing to compare with\n", *ref)
			return write(*out, stdout, "")
		}
	}
	log, err := git("log", "--first-parent", "--format=%s", "refs/tags/"+prev+".."+*ref)
	if err != nil {
		return err
	}
	return write(*out, stdout, render(prev, strings.Split(strings.TrimSpace(log), "\n"), *limit))
}

func write(path string, stdout io.Writer, text string) error {
	if path == "" {
		_, err := io.WriteString(stdout, text)
		return err
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

// previousTag is the v* tag merged into ref that is the fewest first-parent
// commits behind it, skipping exclude; "" when there is none. A tie (several
// tags on one commit) goes to the highest version name.
func previousTag(git gitFunc, ref, exclude string) (string, error) {
	list, err := git("tag", "--merged", ref, "--list", "v*", "--sort=-version:refname")
	if err != nil {
		return "", err
	}
	best, bestN := "", -1
	for tag := range strings.FieldsSeq(list) {
		if tag == exclude {
			continue
		}
		count, err := git("rev-list", "--count", "--first-parent", "refs/tags/"+tag+".."+ref)
		if err != nil {
			return "", err
		}
		n, err := strconv.Atoi(strings.TrimSpace(count))
		if err != nil {
			return "", fmt.Errorf("git rev-list --count %s: %q", tag, count)
		}
		if bestN < 0 || n < bestN {
			best, bestN = tag, n
		}
	}
	return best, nil
}

// change is one commit subject, taken apart.
type change struct {
	typ      string // lower-cased Conventional Commit type; "" when the subject has none
	text     string // the line to list: scope and subject without the type
	breaking bool   // a "!" after the type
}

// internalOrder names the types that are counted, not listed, in the order
// the count line names them.
var internalOrder = []struct {
	label string
	types []string
}{
	{"docs", []string{"docs"}},
	{"CI", []string{"ci"}},
	{"tests", []string{"test", "tests"}},
	{"build", []string{"build"}},
	{"refactors", []string{"refactor"}},
	{"chores", []string{"chore", "style"}},
}

func internalLabel(typ string) string {
	for _, g := range internalOrder {
		for _, t := range g.types {
			if t == typ {
				return g.label
			}
		}
	}
	return ""
}

// mergeSubject reports git's own merge commit subjects ("Merge pull request
// #1 from …", "Merge branch …"), which say nothing about the change.
func mergeSubject(s string) bool {
	for _, p := range []string{"Merge pull request ", "Merge branch ", "Merge remote-tracking branch ", "Merge tag "} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// parse takes a commit subject apart. It reports false for the subjects that
// are no change of their own: a merge commit and an empty line.
func parse(subject string) (change, bool) {
	subject = strings.TrimSpace(subject)
	if subject == "" || mergeSubject(subject) {
		return change{}, false
	}
	if strings.HasPrefix(subject, `Revert "`) { // git revert's own subject
		return change{typ: "revert", text: subject}, true
	}
	head, rest, ok := strings.Cut(subject, ": ")
	if !ok || strings.TrimSpace(rest) == "" {
		return change{text: subject}, true
	}
	c := change{}
	if strings.HasSuffix(head, "!") {
		c.breaking = true
		head = strings.TrimSuffix(head, "!")
	}
	typ, scope := head, ""
	if open := strings.IndexByte(head, '('); open > 0 && strings.HasSuffix(head, ")") {
		typ, scope = head[:open], head[open+1:len(head)-1]
	}
	if typ == "" || strings.ContainsAny(typ, " \t()!:") {
		return change{text: subject}, true
	}
	c.typ = strings.ToLower(typ)
	c.text = strings.TrimSpace(rest)
	if scope != "" {
		c.text = scope + ": " + c.text
	}
	return c, true
}

// sections are the headings and the types listed under each; the last
// collects every subject without a listed type.
var sections = []struct {
	title string
	types []string
}{
	{"Features", []string{"feat"}},
	{"Fixes", []string{"fix"}},
	{"Performance", []string{"perf"}},
	{"Reverts", []string{"revert"}},
	{"Other changes", nil},
}

// render writes the notes for subjects (newest first) since prev, listing at
// most limit entries.
func render(prev string, subjects []string, limit int) string {
	var breaking []string
	groups := make([][]string, len(sections))
	internal := map[string]int{}
	total := 0
	for _, s := range subjects {
		c, ok := parse(s)
		if !ok {
			continue
		}
		total++
		if c.breaking {
			breaking = append(breaking, c.text)
			continue
		}
		if label := internalLabel(c.typ); label != "" {
			internal[label]++
			continue
		}
		placed := false
		for i, sec := range sections {
			for _, t := range sec.types {
				if t == c.typ {
					groups[i] = append(groups[i], c.text)
					placed = true
				}
			}
		}
		if !placed {
			groups[len(groups)-1] = append(groups[len(groups)-1], c.text)
		}
	}
	if total == 0 {
		return "## No changes since " + prev + "\n"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## Changes since %s\n", prev)
	listed, left := 0, 0
	emit := func(title string, lines []string) {
		room := max(0, limit-listed)
		if len(lines) > room {
			left += len(lines) - room
			lines = lines[:room]
		}
		if len(lines) == 0 {
			return
		}
		fmt.Fprintf(&b, "\n### %s\n\n", title)
		for _, l := range lines {
			fmt.Fprintf(&b, "- %s\n", l)
		}
		listed += len(lines)
	}
	emit("Breaking changes", breaking)
	for i, sec := range sections {
		emit(sec.title, groups[i])
	}
	if left > 0 {
		fmt.Fprintf(&b, "\nAnd %d more.\n", left)
	}

	var names []string
	count := 0
	for _, g := range internalOrder {
		if n := internal[g.label]; n > 0 {
			names = append(names, g.label)
			count += n
		}
	}
	if count > 0 {
		noun := "internal changes"
		if count == 1 {
			noun = "internal change"
		}
		lead := "Plus"
		if listed+left == 0 {
			lead = "Only"
		}
		fmt.Fprintf(&b, "\n%s %d %s (%s).\n", lead, count, noun, strings.Join(names, ", "))
	}
	return b.String()
}
