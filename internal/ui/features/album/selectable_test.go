package album

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The fine print's entries (the label, the day, the genres) and the ℗ line
// are selectable: a triple click takes the line and Ctrl/Cmd+C copies it.
// The muted names over the entries are not part of the selection.
func TestFinePrintEntriesAreSelectableAndTheRestIsNot(t *testing.T) {
	r := newRig(t, "/album/100", 1500, 1000)
	r.addCatalog(saltFlats())
	r.shows("Released", "Quiet Records", "℗ 2019 Mia Vale")
	triple := func(text string) {
		t.Helper()
		at, ok := r.tt.Find(text)
		if !ok {
			t.Fatalf("no %q among %q", text, r.tt.Texts())
		}
		x, y := at.X+at.W/2, at.Y+at.H/2
		r.tt.ClickAt(x, y)
		r.tt.ClickAt(x, y)
		r.tt.ClickAt(x, y)
		r.tt.Key(ui.Cmd, ui.KeyC)
	}
	for _, v := range []string{"Quiet Records", "March 8, 2019", "Indie Pop, Pop", "℗ 2019 Mia Vale"} {
		r.tt.SetClipboard("kept")
		triple(v)
		if got := r.tt.Clipboard(); got != v {
			t.Errorf("a triple click on %q copied %q", v, got)
		}
	}
	for _, muted := range []string{"Label"} {
		r.tt.SetClipboard("kept")
		triple(muted)
		if got := r.tt.Clipboard(); got != "kept" {
			t.Errorf("the muted %q copied %q", muted, got)
		}
	}
	if r.path() != "/album/100" {
		t.Errorf("selecting moved the page to %q", r.path())
	}
}
