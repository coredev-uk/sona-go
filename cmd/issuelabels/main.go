// Command issuelabels reads an issue's body on standard input and prints the
// labels its answers call for, one per line: the platform/* label of the
// "Platform" answer and the area/* label of the "Area" answer, as the bug
// report and feature request forms (.github/ISSUE_TEMPLATE) render them (each
// field is "### <label>", a blank line, then the answer).
//
//	gh issue view 12 --json body --jq .body | go run ./cmd/issuelabels -have bug,needs-triage
//
// A body without those headings (an issue not made from a form) prints
// nothing, as do answers that are not one of the forms' options ("Not sure",
// "All of them", _No response_). -have lists the labels the issue already
// carries: a kind that already has a label is skipped, so an edit never
// overrides what a maintainer set. The issue-labels workflow runs it and adds
// what it prints; it never removes a label. The option texts below must match
// the forms: the test reads both forms and fails when they drift.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// platforms maps the Platform dropdown's options to labels.
var platforms = map[string]string{
	"Linux":   "platform/linux",
	"macOS":   "platform/macos",
	"Windows": "platform/windows",
}

// areas maps the Area dropdown's options to labels. "Not sure" has none.
var areas = map[string]string{
	"Playback, the queue or audio output":           "area/playback",
	"My library: songs, albums, artists, playlists": "area/library",
	"Search": "area/search",
	"Pages, menus, windows or how Sona looks": "area/ui",
	"Themes":                      "area/themes",
	"Add-ons":                     "area/add-ons",
	"Installing or updating Sona": "area/updates",
	"Signing in":                  "area/sign-in",
	"Last.fm, Discord, media keys or the desktop":      "area/integrations",
	"Accessibility: screen reader, keyboard, contrast": "area/accessibility",
}

func main() {
	have := flag.String("have", "", "comma-separated labels the issue already has")
	flag.Parse()
	body, err := io.ReadAll(bufio.NewReader(os.Stdin))
	if err != nil {
		fmt.Fprintln(os.Stderr, "issuelabels:", err)
		os.Exit(1)
	}
	for _, l := range labels(string(body), strings.Split(*have, ",")) {
		fmt.Println(l)
	}
}

// labels returns the labels body's Platform and Area answers call for, leaving
// out a kind (platform/, area/) that have already holds one of.
func labels(body string, have []string) []string {
	fields := parse(body)
	var out []string
	for _, k := range []struct {
		heading, prefix string
		options         map[string]string
	}{
		{"Platform", "platform/", platforms},
		{"Area", "area/", areas},
	} {
		if hasPrefix(have, k.prefix) {
			continue
		}
		if l, ok := k.options[fields[k.heading]]; ok {
			out = append(out, l)
		}
	}
	return out
}

func hasPrefix(labels []string, prefix string) bool {
	for _, l := range labels {
		if strings.HasPrefix(strings.TrimSpace(l), prefix) {
			return true
		}
	}
	return false
}

// parse returns the answer under each "### heading" of body, trimmed; the
// first of a repeated heading wins (the forms put their dropdowns before any
// free text, so a heading typed into a text box comes later). A heading
// inside a code fence (a pasted log, the diagnostics bundle) is text.
func parse(body string) map[string]string {
	fields := map[string]string{}
	var heading string
	var answer []string
	inFence := false
	flush := func() {
		if heading == "" {
			return
		}
		if _, seen := fields[heading]; !seen {
			fields[heading] = strings.TrimSpace(strings.Join(answer, "\n"))
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
		}
		if h, ok := strings.CutPrefix(line, "### "); ok && !inFence {
			flush()
			heading, answer = strings.TrimSpace(h), nil
			continue
		}
		answer = append(answer, line)
	}
	flush()
	return fields
}
