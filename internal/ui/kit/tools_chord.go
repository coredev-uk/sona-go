package kit

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// chordKeys are the key names of v0.2.0's chord spelling ("Ctrl+K",
// "Shift+ArrowRight", "?").
var chordKeys = map[string]ui.Key{
	"space": ui.KeySpace, "enter": ui.KeyEnter, "escape": ui.KeyEscape, "tab": ui.KeyTab,
	"backspace": ui.KeyBackspace, "delete": ui.KeyDelete, "insert": ui.KeyInsert,
	"home": ui.KeyHome, "end": ui.KeyEnd, "pageup": ui.KeyPageUp, "pagedown": ui.KeyPageDown,
	"arrowup": ui.KeyUp, "arrowdown": ui.KeyDown, "arrowleft": ui.KeyLeft, "arrowright": ui.KeyRight,
	"-": ui.KeyMinus, "=": ui.KeyEqual, ",": ui.KeyComma, ".": ui.KeyPeriod, "/": ui.KeySlash,
	";": ui.KeySemicolon, "'": ui.KeyQuote, "[": ui.KeyBracketLeft, "]": ui.KeyBracketRight,
	"\\": ui.KeyBackslash, "`": ui.KeyBackquote,
	"f1": ui.KeyF1, "f2": ui.KeyF2, "f3": ui.KeyF3, "f4": ui.KeyF4, "f5": ui.KeyF5, "f6": ui.KeyF6,
	"f7": ui.KeyF7, "f8": ui.KeyF8, "f9": ui.KeyF9, "f10": ui.KeyF10, "f11": ui.KeyF11, "f12": ui.KeyF12,
	"0": ui.Key0, "1": ui.Key1, "2": ui.Key2, "3": ui.Key3, "4": ui.Key4, "5": ui.Key5, "6": ui.Key6,
	"7": ui.Key7, "8": ui.Key8, "9": ui.Key9,
	"a": ui.KeyA, "b": ui.KeyB, "c": ui.KeyC, "d": ui.KeyD, "e": ui.KeyE, "f": ui.KeyF, "g": ui.KeyG,
	"h": ui.KeyH, "i": ui.KeyI, "j": ui.KeyJ, "k": ui.KeyK, "l": ui.KeyL, "m": ui.KeyM, "n": ui.KeyN,
	"o": ui.KeyO, "p": ui.KeyP, "q": ui.KeyQ, "r": ui.KeyR, "s": ui.KeyS, "t": ui.KeyT, "u": ui.KeyU,
	"v": ui.KeyV, "w": ui.KeyW, "x": ui.KeyX, "y": ui.KeyY, "z": ui.KeyZ,
}

var chordMods = map[string]ui.Modifiers{
	"ctrl": ui.Ctrl, "control": ui.Ctrl, "alt": ui.Alt, "option": ui.Alt, "shift": ui.Shift,
	"meta": ui.Super, "super": ui.Super, "cmd": ui.Super, "win": ui.Super,
}

// ParseChord reads one chord in v0.2.0's spelling: modifiers and a key
// joined by "+". "?" is Shift+/ and "Plus" (a lone "+" or a trailing "++")
// Shift+=; a chord ending in a single "+" is not one.
func ParseChord(s string) (mods ui.Modifiers, key ui.Key, ok bool) {
	parts := strings.Split(s, "+")
	switch {
	case s == "+":
		parts = []string{"Plus"}
	case strings.HasSuffix(s, "++"): // "Ctrl++": the plus key
		parts = append(strings.Split(strings.TrimSuffix(s, "++"), "+"), "Plus")
	}
	last := parts[len(parts)-1]
	for _, m := range parts[:len(parts)-1] {
		mod, known := chordMods[strings.ToLower(m)]
		if !known || mods&mod != 0 {
			return 0, 0, false
		}
		mods |= mod
	}
	switch {
	case last == "?":
		return mods | ui.Shift, ui.KeySlash, true
	case strings.EqualFold(last, "plus"):
		return mods | ui.Shift, ui.KeyEqual, true
	}
	key, ok = chordKeys[strings.ToLower(last)]
	return mods, key, ok
}

// ChordPressed reports whether any of chords was pressed this frame: for a
// dialog's own key that closes it again, which the window's shortcut
// handler cannot see while the dialog is open. Call it while building the
// dialog.
func ChordPressed(c *ui.Context, chords ...string) bool {
	for _, s := range chords {
		if mods, key, ok := ParseChord(s); ok && c.Shortcut(mods, key) {
			return true
		}
	}
	return false
}
