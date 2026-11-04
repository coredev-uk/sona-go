package settings

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/module"
)

// The rules of shortcut bindings, ported from v0.2.0's shared/shortcuts
// (chord.ts, registry.ts): chords as canonical strings, a user's binding
// over a shortcut's defaults, conflicts, rebinding.
//
// A chord is a key with exact modifiers, spelled as v0.2.0 spelled it:
// modifiers in the order Ctrl, Alt, Shift, Meta, then one key: "Ctrl+K",
// "Shift+ArrowRight", "Alt+1", "Space", "?", "Plus". Letters are upper case.
// A binding is chords separated by spaces, or "none" (config.Shortcuts).
//
// The vocabulary is the shell's dispatcher's (app/shortcuts.go parseChord
// and namedKeys), not v0.2.0's browser one: the same keys, the same
// spellings, and no others, so what the page shows bound is what the
// dispatcher runs. Two differences follow from the keys being MyGo's:
// Shift is a modifier of every key but the two the dispatcher folds into a
// spelling ("?" is Shift+/ and "Plus" is Shift+=: "Ctrl+Shift+/" is
// "Ctrl+?"), and the lower-case aliases v0.2.0 read in a hand-edited file
// ("esc", "Right", "return") are not chords here, as they are not to the
// dispatcher.

// noKeys is the binding that means "no key" (chord.ts NO_KEYS).
const noKeys = "none"

// keySpec is a key a chord can name and the MyGo key it is.
type keySpec struct {
	name string
	key  ui.Key
}

// keySpecs are the keys the dispatcher's namedKeys has, by their canonical
// names, in the order the recorder asks for them.
var keySpecs = func() []keySpec {
	k := []keySpec{
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
		k = append(k, keySpec{string(rune('0' + i)), ui.Key0 + ui.Key(i)})
	}
	for i := range 26 {
		k = append(k, keySpec{string(rune('A' + i)), ui.KeyA + ui.Key(i)})
	}
	for i := range 12 {
		k = append(k, keySpec{"F" + strconv.Itoa(i+1), ui.KeyF1 + ui.Key(i)})
	}
	return k
}()

var (
	// keyNames are the canonical names by key; keysByName the keys by
	// lower-cased name.
	keyNames   = map[ui.Key]string{}
	keysByName = map[string]ui.Key{}
	// recordable are the keys a recorder listens for.
	recordable []ui.Key
)

func init() {
	for _, k := range keySpecs {
		keyNames[k.key] = k.name
		keysByName[strings.ToLower(k.name)] = k.key
		recordable = append(recordable, k.key)
	}
}

// modifierOrder is how a chord names its modifiers.
var modifierOrder = []struct {
	name string
	mod  ui.Modifiers
}{{"Ctrl", ui.Ctrl}, {"Alt", ui.Alt}, {"Shift", ui.Shift}, {"Meta", ui.Super}}

// modifierAliases are the spellings of a modifier the dispatcher reads.
var modifierAliases = map[string]ui.Modifiers{
	"ctrl": ui.Ctrl, "control": ui.Ctrl, "alt": ui.Alt, "option": ui.Alt, "shift": ui.Shift,
	"meta": ui.Super, "super": ui.Super, "cmd": ui.Super, "win": ui.Super,
}

// modifierCombos are the sixteen sets of modifiers a key can be pressed
// with.
var modifierCombos = func() (out [16]ui.Modifiers) {
	for i := range out {
		for bit, m := range []ui.Modifiers{ui.Ctrl, ui.Alt, ui.Shift, ui.Super} {
			if i&(1<<bit) != 0 {
				out[i] |= m
			}
		}
	}
	return out
}()

// spell is the canonical chord for key pressed with exactly mods (chord.ts
// compose), which is what the recorder makes of a key press (eventChord);
// false for a key no chord can name. The dispatcher spells Shift+/ "?" and
// Shift+= "Plus", so the Shift goes into the spelling.
func spell(mods ui.Modifiers, key ui.Key) (string, bool) {
	name, ok := keyNames[key]
	if !ok {
		return "", false
	}
	switch {
	case key == ui.KeySlash && mods&ui.Shift != 0:
		name, mods = "?", mods&^ui.Shift
	case key == ui.KeyEqual && mods&ui.Shift != 0:
		name, mods = "Plus", mods&^ui.Shift
	}
	var b strings.Builder
	for _, m := range modifierOrder {
		if mods&m.mod != 0 {
			b.WriteString(m.name)
			b.WriteByte('+')
		}
	}
	b.WriteString(name)
	return b.String(), true
}

// splitChord reads a written chord ("ctrl+k", "Shift+ArrowRight",
// "Ctrl++") into the modifiers held and the key, which is false when the
// text is not exactly one chord.
func splitChord(text string) (mods ui.Modifiers, key ui.Key, ok bool) {
	t := strings.TrimSpace(text)
	if t == "" {
		return 0, 0, false
	}
	var keyText, modText string
	switch {
	case t == "+":
		keyText = "+"
	case strings.HasSuffix(t, "++"):
		keyText, modText = "+", t[:len(t)-2]
	default:
		at := strings.LastIndex(t, "+")
		keyText = t[at+1:]
		if at >= 0 {
			modText = t[:at]
		}
	}
	switch name := strings.ToLower(strings.TrimSpace(keyText)); name {
	case "?":
		key, mods = ui.KeySlash, ui.Shift
	case "plus", "+":
		key, mods = ui.KeyEqual, ui.Shift
	default:
		k, known := keysByName[name]
		if !known {
			return 0, 0, false
		}
		key = k
	}
	if modText != "" {
		var held ui.Modifiers
		for part := range strings.SplitSeq(modText, "+") {
			m, known := modifierAliases[strings.ToLower(strings.TrimSpace(part))]
			if !known || held&m != 0 {
				return 0, 0, false
			}
			held |= m
		}
		mods |= held
	}
	return mods, key, true
}

// parseChord reads a written chord into its canonical form (chord.ts
// parseChord); false if it is not one chord.
func parseChord(text string) (string, bool) {
	mods, key, ok := splitChord(text)
	if !ok {
		return "", false
	}
	return spell(mods, key)
}

// parseBinding reads a binding: space-separated chords, each once, or
// "none" for no keys (chord.ts parseBinding). false if it is empty or any
// part is not a chord (the caller keeps the defaults then).
func parseBinding(text string) ([]string, bool) {
	t := strings.TrimSpace(text)
	if strings.EqualFold(t, noKeys) {
		return nil, true
	}
	if t == "" {
		return nil, false
	}
	var chords []string
	for part := range strings.FieldsSeq(t) {
		c, ok := parseChord(part)
		if !ok {
			return nil, false
		}
		if !slices.Contains(chords, c) {
			chords = append(chords, c)
		}
	}
	return chords, true
}

// formatBinding is the binding string for chords ("none" when empty).
func formatBinding(chords []string) string {
	if len(chords) == 0 {
		return noKeys
	}
	return strings.Join(chords, " ")
}

// scopesOverlap is whether two scopes can be active at the same time
// ("" is page; "global" and page are active wherever a context is); distinct
// contexts cannot (registry.ts scopesOverlap).
func scopesOverlap(a, b string) bool {
	base := func(s string) bool { return s == "" || s == "page" || s == "global" }
	return a == b || base(a) || base(b)
}

// bindings are the chords each shortcut answers to, by ID.
type bindings map[string][]string

// defaultChords is a shortcut's default chords, canonical.
func defaultChords(s module.Shortcut) []string {
	var out []string
	for _, k := range s.Keys {
		if c, ok := parseChord(k); ok {
			out = append(out, c)
		}
	}
	return out
}

// resolveBindings is each shortcut's chords: its override when that
// parses ("none" is no keys), else its defaults. A blank or invalid
// override keeps the defaults; overrides for IDs not registered are ignored
// (registry.ts resolveBindings).
func resolveBindings(list []module.Shortcut, overrides config.Shortcuts) bindings {
	out := make(bindings, len(list))
	for _, s := range list {
		if o, ok := overrides[s.ID]; ok {
			if chords, ok := parseBinding(o); ok {
				out[s.ID] = chords
				continue
			}
		}
		out[s.ID] = defaultChords(s)
	}
	return out
}

// conflictsFor is the other shortcuts that answer to chord where id would:
// the same chord in scopes that can be active at once.
func conflictsFor(list []module.Shortcut, b bindings, id, chord string) []module.Shortcut {
	scope := ""
	if i := slices.IndexFunc(list, func(s module.Shortcut) bool { return s.ID == id }); i >= 0 {
		scope = list[i].Scope
	}
	var out []module.Shortcut
	for _, s := range list {
		if s.ID != id && slices.Contains(b[s.ID], chord) && scopesOverlap(scope, s.Scope) {
			out = append(out, s)
		}
	}
	return out
}

// conflictingIds are the IDs involved in any conflict: two shortcuts on one
// chord in overlapping scopes.
func conflictingIds(list []module.Shortcut, b bindings) map[string]bool {
	out := map[string]bool{}
	for _, s := range list {
		for _, chord := range b[s.ID] {
			if len(conflictsFor(list, b, s.ID, chord)) > 0 {
				out[s.ID] = true
			}
		}
	}
	return out
}

// withBinding is overrides with id set to chords, the entry dropped when it
// equals the defaults.
func withBinding(list []module.Shortcut, overrides config.Shortcuts, id string, chords []string) config.Shortcuts {
	next := maps.Clone(overrides)
	if next == nil {
		next = config.Shortcuts{}
	}
	var defaults []string
	if i := slices.IndexFunc(list, func(s module.Shortcut) bool { return s.ID == id }); i >= 0 {
		defaults = defaultChords(list[i])
	}
	if slices.Equal(chords, defaults) {
		delete(next, id)
	} else {
		next[id] = formatBinding(chords)
	}
	return next
}

// rebind binds id to chords (replacing its keys). With steal, the shortcuts
// that conflict with any of them lose that chord, and keep their others, so
// the key does one thing again (registry.ts rebind).
func rebind(list []module.Shortcut, overrides config.Shortcuts, id string, chords []string, steal bool) config.Shortcuts {
	next := withBinding(list, overrides, id, chords)
	if !steal {
		return next
	}
	for _, chord := range chords {
		for _, other := range conflictsFor(list, resolveBindings(list, next), id, chord) {
			kept := slices.DeleteFunc(slices.Clone(resolveBindings(list, next)[other.ID]), func(c string) bool { return c == chord })
			next = withBinding(list, next, other.ID, kept)
		}
	}
	return next
}

// shortcutConflicts is how many shortcuts share a chord with another, by
// the keys each answers to now (summaries.ts: conflictingIds over the
// registered shortcuts).
func shortcutConflicts(list []module.BoundShortcut) int {
	all := make([]module.Shortcut, len(list))
	b := make(bindings, len(list))
	for i, s := range list {
		all[i] = s.Shortcut
		for _, k := range s.Keys {
			if c, ok := parseChord(k); ok && !slices.Contains(b[s.ID], c) {
				b[s.ID] = append(b[s.ID], c)
			}
		}
	}
	return len(conflictingIds(all, b))
}
