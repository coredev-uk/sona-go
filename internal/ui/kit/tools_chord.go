package kit

import (
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// chordKeySpec is a key a chord can name and the MyGo key it is.
type chordKeySpec struct {
	name string
	key  ui.Key
}

// chordKeySpecs are the keys a chord can name, by their canonical spelling,
// in the order a recorder asks for them: the one table every chord parser
// and writer (the shell's dispatcher, Settings' bindings) reads.
var chordKeySpecs = func() []chordKeySpec {
	k := []chordKeySpec{
		{"Space", ui.KeySpace},
		{"Enter", ui.KeyEnter},
		{"Escape", ui.KeyEscape},
		{"Tab", ui.KeyTab},
		{"Backspace", ui.KeyBackspace},
		{"Delete", ui.KeyDelete},
		{"Insert", ui.KeyInsert},
		{"Home", ui.KeyHome},
		{"End", ui.KeyEnd},
		{"PageUp", ui.KeyPageUp},
		{"PageDown", ui.KeyPageDown},
		{"ArrowUp", ui.KeyUp},
		{"ArrowDown", ui.KeyDown},
		{"ArrowLeft", ui.KeyLeft},
		{"ArrowRight", ui.KeyRight},
		{"-", ui.KeyMinus},
		{"=", ui.KeyEqual},
		{",", ui.KeyComma},
		{".", ui.KeyPeriod},
		{"/", ui.KeySlash},
		{";", ui.KeySemicolon},
		{"'", ui.KeyQuote},
		{"[", ui.KeyBracketLeft},
		{"]", ui.KeyBracketRight},
		{`\`, ui.KeyBackslash},
		{"`", ui.KeyBackquote},
	}
	for i := range 10 {
		k = append(k, chordKeySpec{string(rune('0' + i)), ui.Key0 + ui.Key(i)})
	}
	for i := range 26 {
		k = append(k, chordKeySpec{string(rune('A' + i)), ui.KeyA + ui.Key(i)})
	}
	for i := range 12 {
		k = append(k, chordKeySpec{"F" + strconv.Itoa(i+1), ui.KeyF1 + ui.Key(i)})
	}
	return k
}()

var (
	// chordKeys are the keys by lower-cased name; chordNames the canonical
	// names by key.
	chordKeys  = map[string]ui.Key{}
	chordNames = map[ui.Key]string{}
)

func init() {
	for _, k := range chordKeySpecs {
		chordKeys[strings.ToLower(k.name)] = k.key
		chordNames[k.key] = k.name
	}
}

// ChordKeys are the keys a chord can name, in the order a recorder listens
// for them.
func ChordKeys() []ui.Key {
	out := make([]ui.Key, len(chordKeySpecs))
	for i, k := range chordKeySpecs {
		out[i] = k.key
	}
	return out
}

// ChordKeyName is a key's canonical spelling in a chord ("ArrowUp", "K");
// false for a key no chord can name.
func ChordKeyName(key ui.Key) (string, bool) {
	name, ok := chordNames[key]
	return name, ok
}

// chordAliases are the lower-case spellings people write by hand, to the
// canonical key they mean (v0.2.0's chord.ts keyAliases). F13 to F24 are not
// among them: MyGo has no such keys.
var chordAliases = map[string]string{
	"esc": "Escape", "return": "Enter", "del": "Delete", "ins": "Insert", "pgup": "PageUp", "pgdn": "PageDown",
	"up": "ArrowUp", "down": "ArrowDown", "left": "ArrowLeft", "right": "ArrowRight", "spacebar": "Space",
	"comma": ",", "period": ".", "slash": "/",
}

var chordMods = map[string]ui.Modifiers{
	"ctrl": ui.Ctrl, "control": ui.Ctrl, "alt": ui.Alt, "option": ui.Alt, "shift": ui.Shift,
	"meta": ui.Super, "super": ui.Super, "cmd": ui.Super, "win": ui.Super,
}

// ParseChord reads one chord in v0.2.0's spelling: modifiers and a key
// joined by "+". "?" is Shift+/ and "Plus" (a lone "+" or a trailing "++")
// Shift+=; a chord ending in a single "+" is not one. Keys may be written
// as v0.2.0 allowed ("esc", "Ctrl+Right", "return", "comma"…: chordAliases).
func ParseChord(s string) (mods ui.Modifiers, key ui.Key, ok bool) {
	parts := strings.Split(s, "+")
	switch {
	case s == "+" || s == "++":
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
	name := strings.ToLower(last)
	if canon, aliased := chordAliases[name]; aliased {
		name = strings.ToLower(canon)
	}
	key, ok = chordKeys[name]
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

// SlashByPosition is whether the platform gives "?" no Key, which is GTK's
// and AppKit's way in MyGo v0.9.0: Shift with a punctuation key comes by
// its position alone (InputEvent.Code), so Shortcut(ui.Shift, ui.KeySlash)
// never sees it there. Windows gives the key the layout types, and its
// shortcut takes it. ShiftSlash stands in where it is true; the "?"
// binding then means the key where US and UK layouts have "?", which on
// another layout is a key labelled otherwise (German has "?" on Shift+ß,
// and "-" where the US slash is), and the help's label says so.
var SlashByPosition = runtime.GOOS != "windows"

// IsShiftSlash reports whether ev is the press of Shift and the key at the
// slash position that comes by its position alone: "?" where MyGo gives it
// no Key (SlashByPosition). It is false where a Key comes with it, as on
// Windows, whose shortcut takes the press, so no press is run twice; and
// for a key held down, as the toggles it opens take one press.
func IsShiftSlash(ev ui.InputEvent) bool {
	return ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyUnknown && ev.Code == ui.CodeSlash && ev.Mods == ui.Shift && !ev.Repeat
}

// slashTakes counts the presses a focused element took itself
// (TakeShiftSlash). Main thread only, like every view.
var slashTakes int

// TakeShiftSlash tells that the focused element took an IsShiftSlash press
// itself, in its HandleInput, as the key recorder does: the ShiftSlash that
// noted it as it came (an ObserveInput sees it first) lets it go, as a
// window's shortcut never sees a key an element claims.
func TakeShiftSlash() { slashTakes++ }

// ShiftSlash is "?" as a chord where it comes with no Key: an element
// notes the presses that come to it or to an element inside it (Observe,
// its ObserveInput), and the view reads them as the chord "?" (Take,
// Pressed) while building, in the frame after.
type ShiftSlash struct {
	seen  bool
	takes int // slashTakes as the press came
}

// Observe notes an IsShiftSlash press. It is an ObserveInput.
func (s *ShiftSlash) Observe(ev ui.InputEvent) {
	if IsShiftSlash(ev) {
		s.seen, s.takes = true, slashTakes
	}
}

// Take reports whether a press came since the last Take, and forgets it.
// It is false for one that the focused element took (TakeShiftSlash) or
// typed: a "?" typed in a text field is text, as a single-key shortcut
// steps aside in one.
func (s *ShiftSlash) Take(c *ui.Context) bool {
	seen := s.seen
	s.seen = false
	return seen && s.takes == slashTakes && !c.FocusTakesText()
}

// Pressed is ChordPressed, true too for a press Take reports when chords
// hold "?" (Shift+/): a dialog's own "?" closes it on GTK and AppKit too.
func (s *ShiftSlash) Pressed(c *ui.Context, chords ...string) bool {
	slash := s.Take(c)
	if ChordPressed(c, chords...) {
		return true
	}
	return slash && slices.ContainsFunc(chords, IsShiftSlashChord)
}

// IsShiftSlashChord reports whether chord is "?" (Shift+/ by another
// spelling): the chord a ShiftSlash press stands for.
func IsShiftSlashChord(chord string) bool {
	mods, key, ok := ParseChord(chord)
	return ok && mods == ui.Shift && key == ui.KeySlash
}
