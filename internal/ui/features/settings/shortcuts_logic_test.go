package settings

import (
	"maps"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
)

// The cases of v0.2.0's chord.test.ts and registry.test.ts, over the
// dispatcher's vocabulary (see the notes in shortcuts_logic.go).

func TestParseChordNormalisesModifierOrderCaseAndAliases(t *testing.T) {
	for in, want := range map[string]string{
		"shift+ctrl+k":            "Ctrl+Shift+K",
		"Control+ArrowRight":      "Ctrl+ArrowRight",
		"alt+option+k":            "", // the same modifier twice
		" space ":                 "Space",
		"cmd+,":                   "Meta+,",
		"super+win+k":             "", // likewise
		"Meta+Alt+Ctrl+Shift+f12": "Ctrl+Alt+Shift+Meta+F12",
		"escape":                  "Escape",
		"pagedown":                "PageDown",
		"ctrl+[":                  "Ctrl+[",
		"alt+0":                   "Alt+0",
	} {
		got, ok := parseChord(in)
		if (want == "") == ok || got != want {
			t.Errorf("parseChord(%q) = %q, %v, want %q", in, got, ok, want)
		}
	}
}

func TestParseChordReadsPlusAsAKey(t *testing.T) {
	for in, want := range map[string]string{"Ctrl++": "Ctrl+Plus", "+": "Plus", "Ctrl+Plus": "Ctrl+Plus", "++": "Plus"} {
		if got, ok := parseChord(in); !ok || got != want {
			t.Errorf("parseChord(%q) = %q, %v, want %q", in, got, ok, want)
		}
	}
}

// Shift is a modifier of every key but the two the dispatcher spells with
// it: "?" is Shift+/ and "Plus" is Shift+=. (v0.2.0 dropped it from every
// character: "Ctrl+Shift+/" was "Ctrl+/", which the dispatcher would not
// run for that key press.)
func TestParseChordFoldsShiftOnlyIntoQuestionMarkAndPlus(t *testing.T) {
	for in, want := range map[string]string{
		"Shift+?":         "?",
		"?":               "?",
		"Shift+/":         "?",
		"Ctrl+Shift+/":    "Ctrl+?",
		"Ctrl+?":          "Ctrl+?",
		"Ctrl+/":          "Ctrl+/",
		"/":               "/",
		"Shift+=":         "Plus",
		"Ctrl+Shift+=":    "Ctrl+Plus",
		"Ctrl+Shift+Plus": "Ctrl+Plus",
		"Shift+A":         "Shift+A",
		"Shift+ArrowLeft": "Shift+ArrowLeft",
		"Shift+1":         "Shift+1",
	} {
		if got, ok := parseChord(in); !ok || got != want {
			t.Errorf("parseChord(%q) = %q, %v, want %q", in, got, ok, want)
		}
	}
}

func TestParseChordRejectsWhatIsNotOneChord(t *testing.T) {
	for _, bad := range []string{
		"", " ", "Ctrl+", "Hyper+K", "Ctrl+Ctrl+K", "Ctrl+Nope", "K+Ctrl", "Ctrl+K+J", "Ctrl K",
		// MyGo has no F13 to F24.
		"F13", "F24", "Ä", "!",
	} {
		if got, ok := parseChord(bad); ok {
			t.Errorf("parseChord(%q) = %q, want none", bad, got)
		}
	}
}

// v0.2.0 read these lower-case spellings in a hand-edited file; so do the
// page and the dispatcher (both through kit.ParseChord), as canonical chords.
func TestParseChordReadsV020Aliases(t *testing.T) {
	for in, want := range map[string]string{
		"esc": "Escape", "Ctrl+Right": "Ctrl+ArrowRight", "return": "Enter", "Ctrl+comma": "Ctrl+,", "pgup": "PageUp",
		"shift+up": "Shift+ArrowUp", "del": "Delete", "Alt+period": "Alt+.", "slash": "/", "spacebar": "Space",
		"pgdn": "PageDown", "ins": "Insert", "Down": "ArrowDown", "left": "ArrowLeft",
	} {
		if got, ok := parseChord(in); !ok || got != want {
			t.Errorf("parseChord(%q) = %q, %v, want %q", in, got, ok, want)
		}
	}
}

func TestParseBindingParsesChordsOnceEachAndNone(t *testing.T) {
	if got, ok := parseBinding("/ ctrl+f Ctrl+F"); !ok || !slices.Equal(got, []string{"/", "Ctrl+F"}) {
		t.Errorf(`parseBinding("/ ctrl+f Ctrl+F") = %q, %v`, got, ok)
	}
	if got, ok := parseBinding("none"); !ok || len(got) != 0 {
		t.Errorf(`parseBinding("none") = %q, %v`, got, ok)
	}
	if got, ok := parseBinding(" None "); !ok || len(got) != 0 {
		t.Errorf(`parseBinding(" None ") = %q, %v`, got, ok)
	}
	for _, bad := range []string{"  ", "", "Ctrl+K Nope+X"} {
		if got, ok := parseBinding(bad); ok {
			t.Errorf("parseBinding(%q) = %q, want none", bad, got)
		}
	}
}

func TestFormatBinding(t *testing.T) {
	if got := formatBinding(nil); got != "none" {
		t.Errorf("no chords: %q", got)
	}
	if got := formatBinding([]string{"/", "Ctrl+F"}); got != "/ Ctrl+F" {
		t.Errorf("two chords: %q", got)
	}
}

// What the recorder makes of a key press (chord.test.ts eventChord).
func TestSpellMakesTheChordOfAKeyPress(t *testing.T) {
	for _, tc := range []struct {
		mods ui.Modifiers
		key  ui.Key
		want string
	}{
		{ui.Ctrl, ui.KeyK, "Ctrl+K"},
		{ui.Ctrl | ui.Shift, ui.KeyK, "Ctrl+Shift+K"},
		{ui.Shift | ui.Ctrl | ui.Alt | ui.Super, ui.KeyK, "Ctrl+Alt+Shift+Meta+K"},
		{0, ui.KeyK, "K"},
		{0, ui.KeySpace, "Space"},
		{ui.Shift, ui.KeyRight, "Shift+ArrowRight"},
		{0, ui.KeyEnter, "Enter"},
		{0, ui.KeyEscape, "Escape"},
		{0, ui.KeyTab, "Tab"},
		{ui.Shift, ui.KeyTab, "Shift+Tab"},
		{0, ui.KeyBackspace, "Backspace"},
		{0, ui.KeyDelete, "Delete"},
		{0, ui.KeyF5, "F5"},
		{ui.Alt, ui.KeyF12, "Alt+F12"},
		{ui.Alt, ui.Key1, "Alt+1"},
		{ui.Shift, ui.Key1, "Shift+1"},
		{0, ui.KeySlash, "/"},
		{ui.Shift, ui.KeySlash, "?"}, // however the layout reaches the question mark
		{ui.Ctrl | ui.Shift, ui.KeySlash, "Ctrl+?"},
		{ui.Shift, ui.KeyEqual, "Plus"},
		{ui.Ctrl | ui.Shift, ui.KeyEqual, "Ctrl+Plus"},
		{0, ui.KeyEqual, "="},
		{0, ui.KeyBackslash, `\`},
		{ui.Super, ui.KeyComma, "Meta+,"},
	} {
		got, ok := spell(tc.mods, tc.key)
		if !ok || got != tc.want {
			t.Errorf("spell(%v, %v) = %q, %v, want %q", tc.mods, tc.key, got, ok, tc.want)
		}
	}
}

// A key no chord can name is no chord: a key outside the dispatcher's
// vocabulary, and what is no key at all (a lone modifier comes as no key
// press).
func TestSpellRefusesKeysNoChordCanName(t *testing.T) {
	for _, k := range []ui.Key{ui.KeyUnknown, ui.KeyContextMenu, ui.KeyBack, ui.KeyForward} {
		if got, ok := spell(ui.Ctrl, k); ok {
			t.Errorf("spell(Ctrl, %v) = %q, want none", k, got)
		}
	}
}

// Every key with every set of modifiers spells a chord that reads back as
// the same key and modifiers: the recorder never produces what the
// dispatcher would read as another key press.
func TestEveryKeyAndModifiersRoundTrip(t *testing.T) {
	if len(recordable) != 26+10+12+15+11 {
		t.Errorf("%d keys in the vocabulary", len(recordable))
	}
	seen := map[string]ui.Key{}
	for _, key := range recordable {
		name, _ := kit.ChordKeyName(key)
		for _, mods := range modifierCombos {
			chord, ok := spell(mods, key)
			if !ok {
				t.Fatalf("no chord for %s with %v", name, mods)
			}
			if other, dup := seen[chord]; dup && other != key {
				t.Errorf("%q spells two keys", chord)
			}
			seen[chord] = key
			m, k, ok := splitChord(chord)
			if !ok || m != mods || k != key {
				t.Errorf("%q reads back as %v, %v, %v; want %v, %v", chord, m, k, ok, mods, key)
			}
			if again, ok := parseChord(chord); !ok || again != chord {
				t.Errorf("%q is not its own canonical form: %q", chord, again)
			}
		}
	}
	if len(seen) != len(recordable)*len(modifierCombos) {
		t.Errorf("%d distinct chords, want %d", len(seen), len(recordable)*len(modifierCombos))
	}
}

func shOf(id string, scope string, keys ...string) module.Shortcut {
	return module.Shortcut{ID: id, Label: func() string { return id }, Group: "general", Keys: keys, Scope: scope}
}

var shRegistry = []module.Shortcut{
	shOf("player.toggle", "", "Space"),
	shOf("player.next", "", "Ctrl+ArrowRight"),
	shOf("search.focus", "global", "/", "Ctrl+F"),
	shOf("player.close", "now-playing", "Escape"),
	shOf("lyrics.close", "lyrics-room", "Escape"),
	shOf("player.favourite", "", "F"),
}

func shIDs(list []module.Shortcut) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = s.ID
	}
	return out
}

func shIDSet(m map[string]bool) []string { return slices.Sorted(maps.Keys(m)) }

func TestResolveBindingsUsesAnOverrideOverTheDefaults(t *testing.T) {
	b := resolveBindings(shRegistry, config.Shortcuts{"player.toggle": "K", "search.focus": "Ctrl+Shift+F"})
	if got := b["player.toggle"]; !slices.Equal(got, []string{"K"}) {
		t.Errorf("player.toggle = %q", got)
	}
	if got := b["search.focus"]; !slices.Equal(got, []string{"Ctrl+Shift+F"}) {
		t.Errorf("search.focus = %q", got)
	}
	if got := b["player.next"]; !slices.Equal(got, []string{"Ctrl+ArrowRight"}) {
		t.Errorf("player.next = %q", got)
	}
}

func TestResolveBindingsUnbindsWithNoneAndKeepsDefaultsForBlankOrInvalid(t *testing.T) {
	b := resolveBindings(shRegistry, config.Shortcuts{"player.toggle": "none", "player.next": "", "search.focus": "Hyper+F"})
	if got, ok := b["player.toggle"]; !ok || len(got) != 0 {
		t.Errorf("none = %q, %v", got, ok)
	}
	if got := b["player.next"]; !slices.Equal(got, []string{"Ctrl+ArrowRight"}) {
		t.Errorf("blank = %q", got)
	}
	if got := b["search.focus"]; !slices.Equal(got, []string{"/", "Ctrl+F"}) {
		t.Errorf("invalid = %q", got)
	}
}

func TestResolveBindingsIgnoresOverridesForShortcutsThatAreNotRegistered(t *testing.T) {
	if _, ok := resolveBindings(shRegistry, config.Shortcuts{"gone.action": "Ctrl+K"})["gone.action"]; ok {
		t.Error("an override for an unknown shortcut is resolved")
	}
}

func TestDefaultChordsAreCanonical(t *testing.T) {
	got := defaultChords(shOf("x", "", "ctrl+f", "Nope", "shift+/"))
	if !slices.Equal(got, []string{"Ctrl+F", "?"}) {
		t.Errorf("defaults = %q", got)
	}
}

func TestConflictsFindsAChordUsedTwiceInOverlappingScopes(t *testing.T) {
	b := resolveBindings(shRegistry, config.Shortcuts{"player.favourite": "/"})
	if got := shIDs(conflictsFor(shRegistry, b, "player.favourite", "/")); !slices.Equal(got, []string{"search.focus"}) {
		t.Errorf("conflicts = %q", got)
	}
	if got := shIDSet(conflictingIds(shRegistry, b)); !slices.Equal(got, []string{"player.favourite", "search.focus"}) {
		t.Errorf("conflicting = %q", got)
	}
}

func TestConflictsAllowsOneChordInDistinctContexts(t *testing.T) {
	b := resolveBindings(shRegistry, config.Shortcuts{})
	if got := conflictsFor(shRegistry, b, "player.close", "Escape"); len(got) != 0 {
		t.Errorf("conflicts = %q", shIDs(got))
	}
	if n := len(conflictingIds(shRegistry, b)); n != 0 {
		t.Errorf("%d conflicting", n)
	}
}

func TestConflictsCountsAContextAgainstPageWideKeys(t *testing.T) {
	b := resolveBindings(shRegistry, config.Shortcuts{"player.close": "Space"})
	if got := shIDs(conflictsFor(shRegistry, b, "player.close", "Space")); !slices.Equal(got, []string{"player.toggle"}) {
		t.Errorf("conflicts = %q", got)
	}
}

func TestRebindStoresOnlyBindingsThatDifferFromTheDefaults(t *testing.T) {
	for name, tc := range map[string]struct {
		from   config.Shortcuts
		id     string
		chords []string
		want   config.Shortcuts
	}{
		"a new key":       {config.Shortcuts{}, "player.toggle", []string{"K"}, config.Shortcuts{"player.toggle": "K"}},
		"back to default": {config.Shortcuts{"player.toggle": "K"}, "player.toggle", []string{"Space"}, config.Shortcuts{}},
		"no key":          {config.Shortcuts{}, "player.toggle", nil, config.Shortcuts{"player.toggle": "none"}},
		"two keys":        {nil, "player.toggle", []string{"K", "Ctrl+K"}, config.Shortcuts{"player.toggle": "K Ctrl+K"}},
		"the others stay": {
			config.Shortcuts{"player.next": "N"},
			"player.toggle",
			[]string{"K"},
			config.Shortcuts{"player.next": "N", "player.toggle": "K"},
		},
	} {
		if got := rebind(shRegistry, tc.from, tc.id, tc.chords, false); !maps.Equal(got, tc.want) {
			t.Errorf("%s: %v, want %v", name, got, tc.want)
		}
	}
}

func TestRebindAShortcutWithoutDefaultKeys(t *testing.T) {
	list := append(slices.Clone(shRegistry), shOf("x", ""))
	if got := rebind(list, nil, "x", []string{"K"}, false); !maps.Equal(got, config.Shortcuts{"x": "K"}) {
		t.Errorf("a key: %v", got)
	}
	// No key is its default: nothing to store.
	if got := rebind(list, config.Shortcuts{"x": "K"}, "x", nil, false); len(got) != 0 {
		t.Errorf("no key: %v", got)
	}
}

func TestRebindDoesNotChangeItsInput(t *testing.T) {
	from := config.Shortcuts{"player.next": "N"}
	rebind(shRegistry, from, "player.toggle", []string{"K"}, true)
	if !maps.Equal(from, config.Shortcuts{"player.next": "N"}) {
		t.Errorf("the overrides changed: %v", from)
	}
}

func TestRebindMovesAStolenChordLeavingTheOtherShortcutsOtherKeys(t *testing.T) {
	next := rebind(shRegistry, config.Shortcuts{}, "player.favourite", []string{"Ctrl+F"}, true)
	if want := (config.Shortcuts{"player.favourite": "Ctrl+F", "search.focus": "/"}); !maps.Equal(next, want) {
		t.Errorf("rebound = %v, want %v", next, want)
	}
	if n := len(conflictingIds(shRegistry, resolveBindings(shRegistry, next))); n != 0 {
		t.Errorf("%d conflicting after stealing", n)
	}
}

func TestRebindStealingTheOnlyKeyLeavesNoKey(t *testing.T) {
	next := rebind(shRegistry, config.Shortcuts{}, "player.favourite", []string{"Space"}, true)
	if want := (config.Shortcuts{"player.favourite": "Space", "player.toggle": "none"}); !maps.Equal(next, want) {
		t.Errorf("rebound = %v, want %v", next, want)
	}
}

func TestRebindLeavesConflictsInPlaceWithoutSteal(t *testing.T) {
	next := rebind(shRegistry, config.Shortcuts{}, "player.favourite", []string{"Space"}, false)
	got := shIDSet(conflictingIds(shRegistry, resolveBindings(shRegistry, next)))
	if !slices.Equal(got, []string{"player.favourite", "player.toggle"}) {
		t.Errorf("conflicting = %q", got)
	}
}

// What the summary counts: the keys each shortcut answers to now, as the
// shell lists them.
func TestShortcutConflictsCountsTheShortcutsSharingAChord(t *testing.T) {
	bound := func(keys map[string][]string) []module.BoundShortcut {
		out := make([]module.BoundShortcut, len(shRegistry))
		for i, s := range shRegistry {
			out[i] = module.BoundShortcut{Shortcut: s, Keys: s.Keys}
			if k, ok := keys[s.ID]; ok {
				out[i].Keys = k
			}
		}
		return out
	}
	if n := shortcutConflicts(bound(nil)); n != 0 {
		t.Errorf("defaults: %d conflicts", n)
	}
	// "ctrl+f" is the same chord as "Ctrl+F" however the shell spells it.
	if n := shortcutConflicts(bound(map[string][]string{"player.favourite": {"ctrl+f"}})); n != 2 {
		t.Errorf("a shared chord: %d conflicts, want 2", n)
	}
	if n := shortcutConflicts(bound(map[string][]string{"player.favourite": nil})); n != 0 {
		t.Errorf("no keys: %d conflicts", n)
	}
	if n := shortcutConflicts(nil); n != 0 {
		t.Errorf("no shortcuts: %d conflicts", n)
	}
}

// The vocabulary is kit's, the dispatcher's too: every key the recorder
// listens for spells a chord that kit.ParseChord reads back as that key.
func TestVocabularyIsKits(t *testing.T) {
	for _, key := range recordable {
		name, ok := kit.ChordKeyName(key)
		if !ok {
			t.Fatalf("key %v has no name", key)
		}
		if _, k, ok := kit.ParseChord(name); !ok || k != key {
			t.Errorf("%q reads as %v, %v; want %v", name, k, ok, key)
		}
	}
}
