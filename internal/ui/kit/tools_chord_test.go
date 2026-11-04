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

func TestChordKeysNameEveryKeyTheParserReads(t *testing.T) {
	for _, key := range ChordKeys() {
		name, ok := ChordKeyName(key)
		if !ok {
			t.Fatalf("key %v has no name", key)
		}
		if _, got, ok := ParseChord(name); !ok || got != key {
			t.Errorf("ParseChord(%q) = %v, %v, want %v", name, got, ok, key)
		}
	}
	if _, _, ok := ParseChord("++"); !ok {
		t.Error("++ is the plus key")
	}
}

func TestParseChordReadsTheAliasesOfV020(t *testing.T) {
	for _, c := range []struct {
		in   string
		mods ui.Modifiers
		key  ui.Key
	}{
		{"esc", 0, ui.KeyEscape}, {"Return", 0, ui.KeyEnter}, {"Ctrl+Right", ui.Ctrl, ui.KeyRight},
		{"shift+up", ui.Shift, ui.KeyUp}, {"Alt+comma", ui.Alt, ui.KeyComma}, {"period", 0, ui.KeyPeriod},
		{"slash", 0, ui.KeySlash}, {"del", 0, ui.KeyDelete}, {"pgup", 0, ui.KeyPageUp}, {"pgdn", 0, ui.KeyPageDown},
		{"ins", 0, ui.KeyInsert}, {"spacebar", 0, ui.KeySpace}, {"Ctrl+Down", ui.Ctrl, ui.KeyDown}, {"left", 0, ui.KeyLeft},
	} {
		mods, key, ok := ParseChord(c.in)
		if !ok || mods != c.mods || key != c.key {
			t.Errorf("ParseChord(%q) = %v %v %v, want %v %v", c.in, mods, key, ok, c.mods, c.key)
		}
	}
	for _, bad := range []string{"F13", "Ctrl+Nope", "esc+K"} {
		if _, _, ok := ParseChord(bad); ok {
			t.Errorf("ParseChord(%q) read", bad)
		}
	}
}
