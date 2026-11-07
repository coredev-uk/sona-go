package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestRenderGroupsAndCountsTheRest(t *testing.T) {
	got := render("v0.3.0-rc.3", []string{
		"docs(release): record the 0.3.0 rc results (#194)",
		"feat(lyrics): shape word-timed lyrics in their language (#193)",
		"fix(deps): update MyGo to v0.8.3 (#192)",
		"ci(secret-scan): allow Renovate's debug comment (#191)",
		"Merge pull request #12 from coredev-uk/branch",
		"perf: draw the queue once (#190)",
		"feat: add a thing (#189)",
		"refactor(album): stick the sleeve (#185)",
		"feat(ui)!: replace the settings file (#180)",
		"revert: playback: choose the output device (#179)",
		`Revert "feat: x (#5)"`,
		"Tidy the readme",
		"wip: not a known type",
		"",
	}, 30)
	want := `## Changes since v0.3.0-rc.3

### Breaking changes

- ui: replace the settings file (#180)

### Features

- lyrics: shape word-timed lyrics in their language (#193)
- add a thing (#189)

### Fixes

- deps: update MyGo to v0.8.3 (#192)

### Performance

- draw the queue once (#190)

### Reverts

- playback: choose the output device (#179)
- Revert "feat: x (#5)"

### Other changes

- Tidy the readme
- not a known type

Plus 3 internal changes (docs, CI, refactors).
`
	if got != want {
		t.Errorf("render:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderNoChanges(t *testing.T) {
	for name, subjects := range map[string][]string{
		"no commits":  {""},
		"only merges": {"Merge branch 'main' into x"},
	} {
		if got, want := render("v0.3.0-rc.4", subjects, 30), "## No changes since v0.3.0-rc.4\n"; got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
}

func TestRenderOnlyInternalChanges(t *testing.T) {
	got := render("v1", []string{"chore: bump (#2)", "test: more (#1)"}, 30)
	want := "## Changes since v1\n\nOnly 2 internal changes (tests, chores).\n"
	if got != want {
		t.Errorf("render = %q, want %q", got, want)
	}
	got = render("v1", []string{"ci: one (#2)"}, 30)
	if want := "## Changes since v1\n\nOnly 1 internal change (CI).\n"; got != want {
		t.Errorf("render = %q, want %q", got, want)
	}
}

func TestRenderCapsTheList(t *testing.T) {
	got := render("v1", []string{"feat: a", "feat: b", "fix: c", "fix: d", "docs: e"}, 3)
	want := `## Changes since v1

### Features

- a
- b

### Fixes

- c

And 1 more.

Plus 1 internal change (docs).
`
	if got != want {
		t.Errorf("render:\n%s\nwant:\n%s", got, want)
	}
}

func TestBreakingDocsChangeIsListed(t *testing.T) {
	got := render("v1", []string{"docs!: drop the old guide (#3)"}, 30)
	if !strings.Contains(got, "### Breaking changes\n\n- drop the old guide (#3)\n") || strings.Contains(got, "internal") {
		t.Errorf("render = %q", got)
	}
}

// repo is a throwaway git repository; commit adds an empty commit.
type repo struct {
	t   *testing.T
	dir string
}

func newRepo(t *testing.T) *repo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	r := &repo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", "main")
	return r
}

func (r *repo) git(args ...string) string {
	r.t.Helper()
	full := append([]string{"-C", r.dir, "-c", "user.name=t", "-c", "user.email=t@example.test", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)
	out, err := runGit(full...)
	if err != nil {
		r.t.Fatal(err)
	}
	return out
}

func (r *repo) commit(subject string) { r.git("commit", "-q", "--allow-empty", "-m", subject) }

func (r *repo) notes(args ...string) string {
	r.t.Helper()
	var out bytes.Buffer
	git := func(a ...string) (string, error) { return runGit(append([]string{"-C", r.dir}, a...)...) }
	if err := run(args, git, &out); err != nil {
		r.t.Fatal(err)
	}
	return out.String()
}

func TestNotesSinceTheNearestEarlierTag(t *testing.T) {
	r := newRepo(t)
	r.commit("feat: first (#1)")
	r.git("tag", "v0.2.0")
	r.commit("fix: second (#2)")
	r.git("tag", "v0.3.0-rc.1")
	r.commit("feat: third (#3)")
	r.git("tag", "v0.3.0-rc.2")
	r.commit("fix: fourth (#4)")

	if got, want := r.notes(), "## Changes since v0.3.0-rc.2\n\n### Fixes\n\n- fourth (#4)\n"; got != want {
		t.Errorf("notes = %q, want %q", got, want)
	}
	// The tag being built is never the previous one, so a re-run after it
	// was made lists the same changes.
	r.git("tag", "v0.3.0-rc.3")
	if got, want := r.notes("-exclude", "v0.3.0-rc.3"), "## Changes since v0.3.0-rc.2\n\n### Fixes\n\n- fourth (#4)\n"; got != want {
		t.Errorf("notes = %q, want %q", got, want)
	}
	// A second tag on the same commit is "no changes".
	if got, want := r.notes(), "## No changes since v0.3.0-rc.3\n"; got != want {
		t.Errorf("notes = %q, want %q", got, want)
	}
	if got, want := r.notes("-since", "v0.2.0", "-ref", "v0.3.0-rc.1"), "## Changes since v0.2.0\n\n### Fixes\n\n- second (#2)\n"; got != want {
		t.Errorf("notes = %q, want %q", got, want)
	}
}

func TestNotesFromABranchSeeOnlyTheirOwnHistory(t *testing.T) {
	r := newRepo(t)
	r.commit("feat: base (#1)")
	r.git("tag", "v0.3.0-rc.1")
	r.commit("fix: lands on main only (#2)")
	r.git("tag", "v0.3.0-rc.2")
	r.git("checkout", "-q", "-b", "side", "v0.3.0-rc.1")
	r.commit("feat: on the branch (#3)")
	if got, want := r.notes(), "## Changes since v0.3.0-rc.1\n\n### Features\n\n- on the branch (#3)\n"; got != want {
		t.Errorf("notes = %q, want %q", got, want)
	}
}

func TestNotesWithoutAnEarlierTagAreEmpty(t *testing.T) {
	r := newRepo(t)
	r.commit("feat: first (#1)")
	if got := r.notes(); got != "" {
		t.Errorf("notes = %q, want none", got)
	}
	r.git("tag", "v0.1.0")
	if got := r.notes("-exclude", "v0.1.0"); got != "" {
		t.Errorf("notes excluding the only tag = %q, want none", got)
	}
}
