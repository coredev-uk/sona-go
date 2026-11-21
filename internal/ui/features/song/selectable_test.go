package song

import (
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// tripleClick triple-clicks the middle of text.
func (r *rig) tripleClick(text string) {
	r.t.Helper()
	at, ok := r.tt.Find(text)
	if !ok {
		r.t.Fatalf("no %q on screen: %q", text, r.tt.Texts())
	}
	x, y := at.X+at.W/2, at.Y+at.H/2
	r.tt.ClickAt(x, y)
	r.tt.ClickAt(x, y)
	r.tt.ClickAt(x, y)
}

// doubleClick double-clicks the first word of text (its first 6 DIPs).
func (r *rig) doubleClick(text string) {
	r.t.Helper()
	at, ok := r.tt.Find(text)
	if !ok {
		r.t.Fatalf("no %q on screen: %q", text, r.tt.Texts())
	}
	x, y := at.X+6, at.Y+at.H/2
	r.tt.ClickAt(x, y)
	r.tt.ClickAt(x, y)
}

// The ISRC is selectable: a double click takes it whole and Ctrl/Cmd+C puts
// it on the clipboard; its name beside it, in muted type, is not.
func TestISRCIsSelectableAndCopies(t *testing.T) {
	r := newRig(t, 1132, 1200, airbag())
	r.open()
	r.tt.Until("the ledger", r.tt.Shows("ISRC", "GBAYE9700001"))
	r.tt.SetClipboard("kept")
	r.doubleClick("GBAYE9700001")
	r.tt.Key(ui.Cmd, ui.KeyC)
	if got := r.tt.Clipboard(); got != "GBAYE9700001" {
		t.Errorf("a double click on the ISRC copied %q", got)
	}
	// The other values copy the same way (a triple click takes the line).
	r.tt.SetClipboard("kept")
	at, _ := r.tt.Find("Thom Yorke & Jonny Greenwood")
	r.tt.ClickAt(at.X+at.W/2, at.Y+at.H/2)
	r.tt.ClickAt(at.X+at.W/2, at.Y+at.H/2)
	r.tt.ClickAt(at.X+at.W/2, at.Y+at.H/2)
	r.tt.Key(ui.Cmd, ui.KeyC)
	if got := r.tt.Clipboard(); got != "Thom Yorke & Jonny Greenwood" {
		t.Errorf("a triple click on the composers copied %q", got)
	}
	// The name of a fact is not in the selection.
	r.tt.SetClipboard("kept")
	r.doubleClick("ISRC")
	r.tt.Key(ui.Cmd, ui.KeyC)
	if got := r.tt.Clipboard(); got != "kept" {
		t.Errorf("the muted name ISRC copied %q", got)
	}
	if got := r.route(); got != "/song/1440" {
		t.Errorf("selecting moved the page to %q", got)
	}
}

// The Copy button puts the ISRC on the clipboard and says so; it is a named
// Tab stop beside the value.
func TestCopyISRCButton(t *testing.T) {
	r := newRig(t, 1132, 1200, airbag())
	r.open()
	r.tt.Until("the ledger", r.tt.Shows("ISRC", "GBAYE9700001"))
	r.walk("Radiohead", "OK Computer", "Play Airbag", "Copy ISRC", "View full lyrics")
	r.click("Copy ISRC")
	r.tt.Until("the toast", r.tt.Shows("ISRC copied"))
	if got := r.h.Copied(); !slices.Equal(got, []string{"GBAYE9700001"}) {
		t.Errorf("the clipboard got %q", got)
	}
	if _, ok := access.Read(t, r.tt.Tester).Find(ui.RoleButton, "Copy ISRC"); !ok {
		t.Error("Copy ISRC is not a button on the tree")
	}
}

// A song with no ISRC has no Copy button.
func TestNoISRCNoCopyButton(t *testing.T) {
	p := airbag()
	p.ISRC = ""
	r := newRig(t, 1132, 1200, p)
	r.open()
	r.tt.Until("the ledger", r.tt.Shows("Written by"))
	if err := r.tt.Click("Copy ISRC"); err == nil {
		t.Error("a Copy ISRC button without an ISRC")
	}
}

// The credited names and the roles under them are one selection across the
// people; the group titles and the initials of a person with no picture are
// not part of it.
func TestCreditNamesAndRolesSelect(t *testing.T) {
	r := newRig(t, 1132, 1200, airbag())
	r.open()
	r.tt.Until("the credits", r.tt.Shows("Radiohead", "Performer, Vocals", "Nigel Godrich", "Chris Blair"))

	r.tt.SetClipboard("kept")
	r.doubleClick("Nigel Godrich")
	r.tt.Key(ui.Cmd, ui.KeyC)
	if got := r.tt.Clipboard(); got != "Nigel" {
		t.Errorf("a double click on a name copied %q", got)
	}
	// Select all in the credits: names and roles, in order, no titles.
	r.tt.Key(ui.Cmd, ui.KeyA)
	r.tt.Key(ui.Cmd, ui.KeyC)
	got := r.tt.Clipboard()
	for _, want := range []string{"Radiohead", "Performer, Vocals", "Nigel Godrich", "Chris Blair"} {
		if !strings.Contains(got, want) {
			t.Errorf("select all in the credits copied %q, missing %q", got, want)
		}
	}
	if strings.Contains(got, "Producer\n") && strings.HasPrefix(got, "Producer") {
		t.Errorf("select all copied a group title first: %q", got)
	}

	// A triple click takes a person's roles line.
	r.tt.SetClipboard("kept")
	r.tripleClick("Performer, Vocals")
	r.tt.Key(ui.Cmd, ui.KeyC)
	if got := r.tt.Clipboard(); got != "Performer, Vocals" {
		t.Errorf("a triple click on a role line copied %q", got)
	}
}

// The song's lyrics stay unselectable: Apple's terms (§3.3.6 D of the
// Developer Program License Agreement) say music-related text from MusicKit
// is not for use apart from playback, and two of Sona's lyric sources are
// licensed by others (docs/design.md, "Selectable text").
func TestLyricsStayUnselectable(t *testing.T) {
	r := newRig(t, 1132, 1400, airbag())
	r.open()
	r.tt.Until("the lyrics", r.tt.Shows("In the next world war"))
	r.tt.SetClipboard("kept")
	r.doubleClick("In the next world war")
	r.tt.Key(ui.Cmd, ui.KeyC)
	if got := r.tt.Clipboard(); got != "kept" {
		t.Errorf("the lyrics copied %q", got)
	}
	at, _ := r.tt.Find("In the next world war")
	r.tt.RightClickAt(at.X+at.W/2, at.Y+at.H/2)
	if m := r.tt.Menu(); slices.Contains(m, "Copy") {
		t.Errorf("the lyrics have a Copy menu: %q", m)
	}
}

// The selection reads in light and dark: the ISRC with its Copy button, and
// the credited names (not their roles) in one selection.
func TestGoldenSongSelection(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []uitest.Option
		pick func(r *rig)
	}{
		{"song-isrc-selected-light", nil, func(r *rig) { r.doubleClick("GBAYE9700001") }},
		{"song-isrc-selected-dark", []uitest.Option{uitest.Dark()}, func(r *rig) { r.doubleClick("GBAYE9700001") }},
		{"song-credits-selected-light", nil, func(r *rig) {
			r.doubleClick("Nigel Godrich")
			r.tt.Key(ui.Cmd, ui.KeyA)
		}},
		{"song-credits-selected-dark", []uitest.Option{uitest.Dark()}, func(r *rig) {
			r.doubleClick("Nigel Godrich")
			r.tt.Key(ui.Cmd, ui.KeyA)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			i18n.SetRegional("en-GB")
			t.Cleanup(func() { i18n.SetRegional() })
			r := newRigOpts(t, 899, 1133, nil, tc.opts, airbag())
			r.open()
			r.tt.Until("the ledger", r.tt.Shows("ISRC", "GBAYE9700001", "Nigel Godrich"))
			r.tt.Settle()
			tc.pick(r)
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), tc.name)
		})
	}
}
