package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestParseChordReadsV020Spelling(t *testing.T) {
	for _, c := range []struct {
		in   string
		mods ui.Modifiers
		key  ui.Key
		ok   bool
	}{
		{"Ctrl+K", ui.Ctrl, ui.KeyK, true},
		{"Shift+ArrowRight", ui.Shift, ui.KeyRight, true},
		{"Ctrl+Shift+E", ui.Ctrl | ui.Shift, ui.KeyE, true},
		{"?", ui.Shift, ui.KeySlash, true},
		{"Space", 0, ui.KeySpace, true},
		{"Ctrl++", ui.Ctrl | ui.Shift, ui.KeyEqual, true},
		{"+", ui.Shift, ui.KeyEqual, true},
		{"Ctrl+", 0, 0, false},
		{"Ctrl+Ctrl+K", 0, 0, false},
		{"Hyper+K", 0, 0, false},
		{"Ctrl+Nope", 0, 0, false},
	} {
		mods, key, ok := ParseChord(c.in)
		if ok != c.ok || (ok && (mods != c.mods || key != c.key)) {
			t.Errorf("ParseChord(%q) = %v %v %v, want %v %v %v", c.in, mods, key, ok, c.mods, c.key, c.ok)
		}
	}
}
