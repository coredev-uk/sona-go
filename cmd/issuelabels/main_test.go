package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// A bug report as GitHub renders the form: "### label", a blank line, the answer.
const bugBody = `### Platform

macOS

### OS version

macOS 15.1

### Desktop and session

_No response_

### How you installed

macOS .dmg

### Area

Signing in

### How often

Every time

### Sona version

0.3.0

### What happened

Sign-in loops.

### Steps to reproduce

1. Open Sona

### Diagnostics bundle

` + "```text\n### Platform\nLinux\n### Area\nSearch\n```" + `

### Other logs or screenshots

_No response_

### Before you send

- [x] I searched the existing issues and this is not already reported.
- [x] I am on the latest release or release candidate.
- [x] I removed personal data (account names, tokens, library contents) from everything I pasted.
`

const featureBody = `### Platform

All of them

### Area

Themes

### Problem

Too bright.

### Proposal

A dimmer one.

### Alternatives considered

_No response_

### Before you send

- [x] I searched the existing issues and this is not already requested.
`

func TestLabels(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		have []string
		want []string
	}{
		{"bug form", bugBody, nil, []string{"platform/macos", "area/sign-in"}},
		{"bug form, CRLF", strings.ReplaceAll(bugBody, "\n", "\r\n"), nil, []string{"platform/macos", "area/sign-in"}},
		{"feature form, every platform", featureBody, nil, []string{"area/themes"}},
		{"labels the form itself adds don't count", bugBody, []string{"bug", "needs-triage"}, []string{"platform/macos", "area/sign-in"}},
		{"a maintainer's area stays", bugBody, []string{"bug", "area/playback"}, []string{"platform/macos"}},
		{"a maintainer's platform stays", bugBody, []string{"platform/linux"}, []string{"area/sign-in"}},
		{"both set already", bugBody, []string{"platform/linux", "area/ui"}, nil},
		{"Not sure and no response", "### Platform\n\n_No response_\n\n### Area\n\nNot sure\n", nil, nil},
		{"an issue not from a form", "Sona crashes when I press play.\n\nPlatform: Linux\n", nil, nil},
		{"empty", "", nil, nil},
		{"unknown answers", "### Platform\n\nBeOS\n\n### Area\n\nVibes\n", nil, nil},
		{"the first heading wins", "### Area\n\nSearch\n\n### What happened\n\nx\n\n### Area\n\nThemes\n", nil, []string{"area/search"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := labels(tc.body, tc.have); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("labels = %v, want %v", got, tc.want)
			}
		})
	}
}

// formOptions returns the options of the dropdown labelled label in a form,
// read line by line (the repository has no YAML dependency).
func formOptions(t *testing.T, path, label string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(b), "\n")
	var opts []string
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "label: "+label {
			continue
		}
		for i++; i < len(lines); i++ {
			s := strings.TrimSpace(lines[i])
			if s == "options:" {
				indent := len(lines[i]) - len(strings.TrimLeft(lines[i], " "))
				for i++; i < len(lines) && strings.HasPrefix(lines[i], strings.Repeat(" ", indent+2)+"- "); i++ {
					o := strings.TrimPrefix(strings.TrimSpace(lines[i]), "- ")
					opts = append(opts, strings.Trim(o, `"`))
				}
				return opts
			}
			if strings.HasPrefix(s, "- type:") {
				break
			}
		}
	}
	t.Fatalf("%s: no dropdown %q with options", path, label)
	return nil
}

// The forms' option texts and the tables above must agree: every option maps
// to a label (or is an explicit "no label" answer), and every table entry is
// an option of each form that has the dropdown.
func TestFormsMatchTables(t *testing.T) {
	const dir = "../../.github/ISSUE_TEMPLATE/"
	noLabel := map[string]bool{"Not sure": true, "All of them": true}
	for _, tc := range []struct {
		form, heading string
		table         map[string]string
	}{
		{"bug_report.yml", "Platform", platforms},
		{"bug_report.yml", "Area", areas},
		{"feature_request.yml", "Platform", platforms},
		{"feature_request.yml", "Area", areas},
	} {
		t.Run(tc.form+" "+tc.heading, func(t *testing.T) {
			opts := formOptions(t, dir+tc.form, tc.heading)
			in := map[string]bool{}
			for _, o := range opts {
				in[o] = true
				if _, ok := tc.table[o]; !ok && !noLabel[o] {
					t.Errorf("option %q has no label in the table", o)
				}
			}
			for o := range tc.table {
				if !in[o] {
					t.Errorf("table entry %q is not an option of the form", o)
				}
			}
		})
	}
}

// Both forms' Area dropdowns list the same options in the same order, and the
// labels they set are exactly the user-facing area labels: area/ci-release is
// maintainer-facing (CI, the release workflow and its secrets) and is never
// an option.
func TestAreaDropdownsAgree(t *testing.T) {
	const dir = "../../.github/ISSUE_TEMPLATE/"
	bug := formOptions(t, dir+"bug_report.yml", "Area")
	feature := formOptions(t, dir+"feature_request.yml", "Area")
	if !reflect.DeepEqual(bug, feature) {
		t.Errorf("Area options differ:\nbug     %q\nfeature %q", bug, feature)
	}
	const maintainerOnly = "area/ci-release"
	want := map[string]bool{
		"area/playback": true, "area/library": true, "area/search": true, "area/ui": true,
		"area/themes": true, "area/add-ons": true, "area/updates": true, "area/sign-in": true,
		"area/integrations": true, "area/accessibility": true,
	}
	for _, l := range areas {
		if l == maintainerOnly {
			t.Errorf("%s is maintainer-facing and must not be a form option", l)
		}
		if !want[l] {
			t.Errorf("%q is not a user-facing area label", l)
		}
		delete(want, l)
	}
	for l := range want {
		t.Errorf("no Area option sets %s", l)
	}
}
