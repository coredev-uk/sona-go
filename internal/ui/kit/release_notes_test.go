package kit

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// releaseNotesGolden is longer than its dialog, as a real release's are, and
// has every block the changelogs carry: a heading with a linked version,
// groups, bullets with bold, italic and code, links and a wrapped line.
const releaseNotesGolden = `## [0.8.0](https://github.com/coredev-uk/sona-go/compare/v0.7.0...v0.8.0) (2026-11-26)

### Features

* **library:** one place for **favourite** songs ([#621](https://github.com/coredev-uk/sona-go/issues/621)) ([a627490](https://github.com/coredev-uk/sona-go/commit/a627490))
* **lyrics:** letters lift with the colour sweep, letter by letter, and a line that is long enough to wrap onto a second line in the dialog ([#617](https://github.com/coredev-uk/sona-go/issues/617))
* **cli:** ` + "`sona connect`" + `: the add-on protocol on stdio for *your own* programs ([#553](https://github.com/coredev-uk/sona-go/issues/553))
* **ui:** keep a submenu open while the pointer heads for it ([#625](https://github.com/coredev-uk/sona-go/issues/625))
* **macos:** a designed drag-to-install window in the .dmg ([#646](https://github.com/coredev-uk/sona-go/issues/646))

### Bug Fixes

* **playback:** a song that is playing never stays in the loading state ([#701](https://github.com/coredev-uk/sona-go/issues/701))
* **history:** the History page lists every play, and a failed fetch can be retried ([#708](https://github.com/coredev-uk/sona-go/issues/708))
* **playlists:** show a renamed playlist's generated cover at once ([#707](https://github.com/coredev-uk/sona-go/issues/707))

### Performance Improvements

* **ui:** slow ambient motion paints at most 60 times a second ([#702](https://github.com/coredev-uk/sona-go/issues/702))
* **library:** sync music videos into SQLite so the page works offline ([#516](https://github.com/coredev-uk/sona-go/issues/516))
`

// releaseNotesSample is a release's notes as release-please writes them: a
// heading with a linked version, groups, bullets with bold, a code span and
// links to the issue and the commit.
const releaseNotesSample = `## [0.8.0](https://github.com/coredev-uk/sona-go/compare/v0.7.0...v0.8.0) (2026-11-26)

### Features

* **library:** one place for **favourite** songs ([#621](https://github.com/coredev-uk/sona-go/issues/621)) ([a627490](https://github.com/coredev-uk/sona-go/commit/a627490))
* *italic* and ` + "`sona addons dev`" + ` in a list

### Bug Fixes

* **playback:** a song that is playing never stays loading ([#701](https://github.com/coredev-uk/sona-go/issues/701))
`

type notesApp struct {
	open      bool
	notes     *ReleaseNotes
	spec      ReleaseNotesSpec
	opened    []string
	behind    int
	primary   int
	secondary int
}

func (a *notesApp) view(c *ui.Context) {
	theme.Apply(c)
	ui.Column(c).Padding(20).Gap(12).AlignItems(ui.Start).Children(func() {
		if Button(c.Key("opener"), "Notes…", ButtonOpts{}).Clicked() {
			a.open = true
		}
		if Button(c.Key("behind"), "Behind", ButtonOpts{}).Clicked() {
			a.behind++
		}
	})
	a.notes.Show(c, &a.open, a.spec)
}

func newNotesApp(t *testing.T, notes string, w, h int) (*notesApp, *ui.Tester) {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	a := &notesApp{}
	a.notes = NewReleaseNotes(func(url string) { a.opened = append(a.opened, url) })
	a.spec = ReleaseNotesSpec{
		Version:   "0.8.0",
		Published: time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC),
		Notes:     notes,
		Secondary: ReleaseNotesAction{Label: "View release", Icon: "external-link", IconAfter: true, Run: func() { a.secondary++ }},
		Primary:   ReleaseNotesAction{Label: "Install and restart", Icon: "arrow-down", Run: func() { a.primary++ }},
	}
	return a, ui.NewTester(a.view, w, h)
}

func TestReleaseNotesDialogShowsTheVersionDateAndRenderedNotes(t *testing.T) {
	a, tt := newNotesApp(t, releaseNotesSample, 900, 700)
	if tt.HasText("What's New in Sona 0.8.0") {
		t.Fatal("the dialog is built while closed")
	}
	if err := tt.Click("Notes…"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"What's New in Sona 0.8.0", "Released", "Features", "Bug Fixes", "favourite", "italic", "sona addons dev",
		"#621", "Close", "View release", "Install and restart",
	} {
		if !tt.HasText(want) {
			t.Errorf("the dialog lacks %q: %q", want, tt.Texts())
		}
	}
	// The Markdown is rendered, not shown: no syntax characters survive.
	for _, tx := range tt.Texts() {
		if strings.Contains(tx, "**") || strings.Contains(tx, "](") || strings.HasPrefix(strings.TrimSpace(tx), "###") {
			t.Errorf("unrendered Markdown in %q", tx)
		}
	}
	if !a.open {
		t.Fatal("the dialog closed itself")
	}
}

func TestReleaseNotesDialogIsANamedDialogWithTheHeadingAndAGroupForTheNotes(t *testing.T) {
	_, tt := newNotesApp(t, releaseNotesSample, 900, 700)
	if err := tt.Click("Notes…"); err != nil {
		t.Fatal(err)
	}
	d, ok := tt.AccessByRole(ui.RoleDialog, "What's New in Sona 0.8.0")
	if !ok {
		t.Fatalf("no dialog named by its title: %v", tt.AccessAll())
	}
	if !strings.Contains(d.Description, "Released") {
		t.Errorf("the dialog's description is %q, want the release date", d.Description)
	}
	if h, ok := tt.AccessByRole(ui.RoleHeading, "What's New in Sona 0.8.0"); !ok || h.Level != 2 {
		t.Errorf("the title is not a level 2 heading: %+v, %v", h, ok)
	}
	if _, ok := tt.AccessByRole(ui.RoleGroup, "Release notes"); !ok {
		t.Error("the notes' scroll area is not a group named Release notes")
	}
	// The Markdown's own headings are headings.
	if _, ok := tt.AccessByRole(ui.RoleHeading, "Bug Fixes"); !ok {
		t.Error("a Markdown heading is not a heading")
	}
}

func TestReleaseNotesLinksOpenInTheBrowserAndOthersDoNot(t *testing.T) {
	a, tt := newNotesApp(t, "A [web](https://example.test/a) link, a [file](file:///etc/passwd), a [script](javascript:alert(1)), a [mail](mailto:a@example.test) and a [jump](#top).", 900, 700)
	if err := tt.Click("Notes…"); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"web", "file", "script", "mail", "jump"} {
		r, ok := tt.Find(text)
		if !ok {
			t.Fatalf("no %q link on screen: %q", text, tt.Texts())
		}
		tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	}
	if got, want := strings.Join(a.opened, " "), "https://example.test/a mailto:a@example.test"; got != want {
		t.Fatalf("opened %q, want %q", got, want)
	}
}

func TestReleaseNotesLinksTakeTheKeyboard(t *testing.T) {
	a, tt := newNotesApp(t, "A [web](https://example.test/a) link.", 900, 700)
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter)
	// The dialog opens on the notes; Tab goes on to the link, and Enter opens it.
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("web") || tt.Focused("Close") {
		t.Fatal("Tab did not move from the notes to the link")
	}
	tt.Key(0, ui.KeyEnter)
	if len(a.opened) != 1 || a.opened[0] != "https://example.test/a" {
		t.Fatalf("keyboard opened %v", a.opened)
	}
}

func TestReleaseNotesDialogIsCappedForReadingAndScrollsInTheWindow(t *testing.T) {
	var long strings.Builder
	for i := range 80 {
		fmt.Fprintf(&long, "* **fix:** change number %d, which has enough words to wrap on a narrow line, probably\n", i)
	}
	a, tt := newNotesApp(t, "### Fixes\n\n"+long.String(), 1400, 600)
	if err := tt.Click("Notes…"); err != nil {
		t.Fatal(err)
	}
	title, ok := tt.Find("What's New in Sona 0.8.0")
	if !ok {
		t.Fatal("no title")
	}
	closeBtn, _ := tt.Find("Close")
	// The panel is at most releaseNotesWidth wide: its footer's buttons stay in it.
	view, _ := tt.Find("Install and restart")
	if right := view.X + view.W; right-title.X > releaseNotesWidth {
		t.Errorf("content spans %v DIPs, want at most %d", right-title.X, releaseNotesWidth)
	}
	// The footer is in the window: the notes scroll, the buttons don't leave.
	if closeBtn.Y+closeBtn.H > 600 || view.Y+view.H > 600 {
		t.Errorf("footer at %v/%v is below the 600 DIP window", closeBtn.Y+closeBtn.H, view.Y+view.H)
	}
	// The list is longer than the window: its last change is outside the
	// scroll area's clip (a zero-size rect, below the footer) until the notes
	// scroll to it, and then it shows above the footer.
	const last = "fix: change number 79, which has enough words to wrap on a narrow line, probably"
	if r, ok := tt.Find(last); !ok || r.H != 0 || r.Y < closeBtn.Y {
		t.Fatalf("the last change is at %v before any scrolling: the notes are not clipped to a scroll area", r)
	}
	tt.Scroll(700, 300, 0, 100000)
	if r, ok := tt.Find(last); !ok || r.H == 0 || r.Y+r.H > closeBtn.Y {
		t.Errorf("the last change is at %v after scrolling, want it shown above the footer at %v", r, closeBtn.Y)
	}
	if a.opened != nil {
		t.Errorf("scrolling opened %v", a.opened)
	}
}

// Notes with no link have nothing to Tab to inside them: the scroll area is
// itself the first Tab stop, so Page Down reads a long list with no pointer.
func TestReleaseNotesScrollWithTheKeyboardWhenThereIsNoLink(t *testing.T) {
	var long strings.Builder
	for i := range 80 {
		fmt.Fprintf(&long, "* change number %d, which has enough words to wrap on a narrow line, probably\n", i)
	}
	a, tt := newNotesApp(t, long.String(), 1400, 600)
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter)
	if !a.open {
		t.Fatal("did not open from the keyboard")
	}
	if !tt.Focused("Release notes") {
		t.Fatalf("the focus is not on the notes: %q", tt.Texts())
	}
	const last = "change number 79, which has enough words to wrap on a narrow line, probably"
	for range 20 {
		tt.Key(0, ui.KeyPageDown)
	}
	if r, ok := tt.Find(last); !ok || r.H == 0 {
		t.Fatalf("Page Down did not reach the last change: %v", r)
	}
}

func TestReleaseNotesDialogActionsAndClose(t *testing.T) {
	a, tt := newNotesApp(t, releaseNotesSample, 900, 700)
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter)
	if !a.open {
		t.Fatal("did not open from the keyboard")
	}
	if err := tt.Click("View release"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Install and restart"); err != nil {
		t.Fatal(err)
	}
	if a.secondary != 1 || a.primary != 1 || !a.open {
		t.Fatalf("secondary %d, primary %d, open %v: an action closed the dialog or didn't run", a.secondary, a.primary, a.open)
	}
	if err := tt.Click("Close"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	if a.open || tt.HasText("What's New in Sona 0.8.0") {
		t.Fatal("Close did not close the dialog")
	}
	if !tt.Focused("Notes…") {
		t.Error("the focus did not return to the opener")
	}
	// Escape closes it too, and the page behind took no click while it was open.
	if err := tt.Click("Notes…"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(300 * time.Millisecond)
	if a.open || a.behind != 0 {
		t.Fatalf("open %v, behind %d after Escape", a.open, a.behind)
	}
}

func TestReleaseNotesWithoutNotesSaysSoAndKeepsTheActions(t *testing.T) {
	_, tt := newNotesApp(t, "", 900, 700)
	if err := tt.Click("Notes…"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"This release has no notes. The release page may say more.", "View release", "Close"} {
		if !tt.HasText(want) {
			t.Errorf("lacks %q: %q", want, tt.Texts())
		}
	}
	if _, ok := tt.AccessByRole(ui.RoleGroup, "Release notes"); ok {
		t.Error("an empty scroll area for no notes")
	}
}

func TestReleaseNotesWithoutADateHasNoDescription(t *testing.T) {
	a, tt := newNotesApp(t, "Notes.", 900, 700)
	a.spec.Published = time.Time{}
	if err := tt.Click("Notes…"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("Released") {
		t.Errorf("a date for no date: %q", tt.Texts())
	}
}
