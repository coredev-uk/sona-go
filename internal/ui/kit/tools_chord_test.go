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
		{"esc", 0, ui.KeyEscape},
		{"Return", 0, ui.KeyEnter},
		{"Ctrl+Right", ui.Ctrl, ui.KeyRight},
		{"shift+up", ui.Shift, ui.KeyUp},
		{"Alt+comma", ui.Alt, ui.KeyComma},
		{"period", 0, ui.KeyPeriod},
		{"slash", 0, ui.KeySlash},
		{"del", 0, ui.KeyDelete},
		{"pgup", 0, ui.KeyPageUp},
		{"pgdn", 0, ui.KeyPageDown},
		{"ins", 0, ui.KeyInsert},
		{"spacebar", 0, ui.KeySpace},
		{"Ctrl+Down", ui.Ctrl, ui.KeyDown},
		{"left", 0, ui.KeyLeft},
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

// IsShiftSlash is "?" with no Key alone: Windows' press, which has the Key,
// goes to the shortcut, and a held key's repeats are not presses.
func TestIsShiftSlashIsTheKeylessPressOnly(t *testing.T) {
	for _, c := range []struct {
		name string
		ev   ui.InputEvent
		want bool
	}{
		{"GTK and AppKit", ui.InputEvent{Kind: ui.InputKeyDown, Code: ui.CodeSlash, Mods: ui.Shift}, true},
		{"with a Key (Windows)", ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeySlash, Code: ui.CodeSlash, Mods: ui.Shift}, false},
		{"repeat", ui.InputEvent{Kind: ui.InputKeyDown, Code: ui.CodeSlash, Mods: ui.Shift, Repeat: true}, false},
		{"release", ui.InputEvent{Kind: ui.InputKeyUp, Code: ui.CodeSlash, Mods: ui.Shift}, false},
		{"no Shift", ui.InputEvent{Kind: ui.InputKeyDown, Code: ui.CodeSlash}, false},
		{"Ctrl too", ui.InputEvent{Kind: ui.InputKeyDown, Code: ui.CodeSlash, Mods: ui.Shift | ui.Ctrl}, false},
		{"another key", ui.InputEvent{Kind: ui.InputKeyDown, Code: ui.CodeMinus, Mods: ui.Shift}, false},
	} {
		if got := IsShiftSlash(c.ev); got != c.want {
			t.Errorf("%s: IsShiftSlash = %v, want %v", c.name, got, c.want)
		}
	}
	for chord, want := range map[string]bool{"?": true, "Shift+/": true, "Shift+slash": true, "/": false, "Ctrl+?": false, "F1": false} {
		if got := IsShiftSlashChord(chord); got != want {
			t.Errorf("IsShiftSlashChord(%q) = %v, want %v", chord, got, want)
		}
	}
}

// A ShiftSlash reports a press it observed once, in the frame after; not one
// that the focused element took, or that a text field typed.
func TestShiftSlashTakesWhatNoElementTook(t *testing.T) {
	var s ShiftSlash
	text := ""
	var took []bool
	takeIt := false
	tt := ui.NewTester(func(c *ui.Context) {
		took = append(took, s.Take(c))
		ui.Column(c).Fill().ObserveInput(s.Observe).Children(func() {
			b := ui.Button(c, "B")
			if takeIt {
				b.HandleInput(func(ev ui.InputEvent) bool {
					if IsShiftSlash(ev) {
						TakeShiftSlash()
						return true
					}
					return false
				})
			}
			ui.TextInput(c, &text).Label("F")
		})
	}, 400, 300)
	tt.Frame()
	press := func() bool {
		took = nil
		tt.CodeDown(ui.Shift, ui.CodeSlash)
		tt.CodeUp(ui.Shift, ui.CodeSlash)
		tt.Frame()
		n := 0
		for _, t := range took {
			if t {
				n++
			}
		}
		if n > 1 {
			t.Fatalf("one press taken %d times", n)
		}
		return n == 1
	}
	tt.Key(0, ui.KeyTab) // the button
	if !press() {
		t.Error("a press from the focused button was not taken")
	}
	takeIt = true
	tt.Frame()
	if press() {
		t.Error("a press the button took itself was taken")
	}
	tt.Key(0, ui.KeyTab) // the field
	tt.CodeDown(ui.Shift, ui.CodeSlash)
	tt.Type("?")
	tt.CodeUp(ui.Shift, ui.CodeSlash)
	took = nil
	tt.Frame()
	for _, t2 := range took {
		if t2 {
			t.Errorf("a ? typed in a field was taken (text %q)", text)
		}
	}
}
