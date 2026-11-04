package settings

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// scRig is Settings → Shortcuts over the full Env and the settings the
// harness's core saves to, with a window-level dispatcher in the shell's
// way: every frame it asks MyGo for each chord a shortcut answers to now
// (the keys of the saved settings over the defaults), and a key it gets
// runs the first shortcut bound to it. A test sees that a rebinding takes
// effect, and that the recorder gets keys the dispatcher would have taken.
type scRig struct {
	t      *testing.T
	h      *uitest.Harness
	router *ui.Router
	s      *settings
	tt     *uitest.Session
	list   []module.Shortcut
	// runs counts the runs of each shortcut by ID.
	runs map[string]int
}

// scShortcuts are the shortcuts of the rig's fake modules: a few of every
// group, chords the recorder must take from the dispatcher (Space, Escape,
// Backspace, Tab, an arrow, F5, a Ctrl chord), two in conflict, one with
// two chords, one in a context and one with no key.
func scShortcuts(runs map[string]int) []module.Shortcut {
	mk := func(id string, group module.ShortcutGroup, scope string, keys ...string) module.Shortcut {
		return module.Shortcut{
			ID: id, Label: func() string { return id }, Group: group, Scope: scope, Keys: keys,
			Run: func() { runs[id]++ },
		}
	}
	return []module.Shortcut{
		mk("player.toggle", "playback", "", "Space"),
		mk("player.next", "playback", "", "Ctrl+ArrowRight"),
		mk("player.favourite", "playback", "", "F"),
		mk("view.sidebar", "view", "global", "Ctrl+B"),
		mk("view.refresh", "view", "", "F5"),
		mk("view.twinA", "view", "", "Ctrl+Y"),
		mk("view.twinB", "view", "", "Ctrl+Y"),
		mk("nav.search", "pages", "global", "/", "Ctrl+F"),
		mk("nav.palette", "pages", "global", "Ctrl+K"),
		mk("nav.back", "navigation", "", "Backspace"),
		mk("nav.tab", "navigation", "", "Tab"),
		mk("test.close", "general", "now-playing", "Escape"),
		mk("test.free", "general", ""),
		mk("test.arrow", "general", "", "ArrowRight"),
	}
}

// newScRig builds the section at a window size. before runs once the rig is
// wired and before its first frame (nil for nothing).
func newScRig(t *testing.T, w, h int, before func(*scRig), opts ...uitest.Option) *scRig {
	t.Helper()
	hn := uitest.New(t, opts...)
	r := &scRig{t: t, h: hn, router: ui.NewRouter(settingsPath), runs: map[string]int{}}
	r.list = scShortcuts(r.runs)
	hn.Env.Nav = r.router
	hn.Env.Shortcuts = r.bound
	r.s = newSettings(hn.Env,
		services.Service[services.SystemService](hn.Core), services.Service[services.PlatformService](hn.Core),
		fakeScrobbler{}, services.Service[services.IntegrationsService](hn.Core),
		services.Service[services.CDMService](hn.Core), services.Service[services.UpdatesService](hn.Core))
	r.s.init()
	r.router.Replace(settingsPath + "?section=shortcuts")
	t.Cleanup(kit.CancelConfirm)
	if before != nil {
		before(r)
	}
	r.tt = hn.Tester(t, func(c *ui.Context) {
		r.dispatch(c)
		ui.Column(c).Fill().Children(func() {
			r.router.View(c, func(rt *ui.Route) {
				if rt.Match(settingsPath) {
					r.s.page(c, rt)
				}
			})
		})
		kit.ConfirmHost(c)
	}, w, h)
	return r
}

// tallRig is the section in a window tall enough to show every row.
func tallRig(t *testing.T) *scRig {
	t.Helper()
	r := newScRig(t, 1132, 1500, nil)
	r.show("test.free not set")
	r.tt.Advance(10 * time.Millisecond) // the measured width settles
	return r
}

// bound is Env.Shortcuts: each shortcut with the keys it answers to now,
// the saved binding when it parses ("none" is no keys), else its defaults.
func (r *scRig) bound() []module.BoundShortcut {
	overrides := r.h.Settings.Get().Shortcuts
	out := make([]module.BoundShortcut, len(r.list))
	for i, sh := range r.list {
		keys := sh.Keys
		if b, ok := overrides[sh.ID]; ok {
			if chords, ok := parseBinding(b); ok {
				keys = strings.Fields(b)
				if len(chords) == 0 {
					keys = nil
				}
			}
		}
		out[i] = module.BoundShortcut{Shortcut: sh, Keys: keys}
	}
	return out
}

// dispatch is the shell's key handler: a key pressed that no focused
// element took, for a chord some shortcut answers to, runs that shortcut.
func (r *scRig) dispatch(c *ui.Context) {
	for _, b := range r.bound() {
		for _, k := range b.Keys {
			if mods, key, ok := splitChord(k); ok && c.Shortcut(mods, key) {
				b.Run()
			}
		}
	}
}

func (r *scRig) show(texts ...string) {
	r.t.Helper()
	r.tt.Until(strings.Join(texts, ", "), r.tt.Shows(texts...))
}

func (r *scRig) hidden(texts ...string) {
	r.t.Helper()
	r.tt.Until("no "+strings.Join(texts, ", "), func() bool {
		for _, w := range texts {
			if r.tt.HasText(w) {
				return false
			}
		}
		return true
	})
}

func (r *scRig) click(label string) {
	r.t.Helper()
	if err := r.tt.Click(label); err != nil {
		r.t.Fatal(err)
	}
}

func (r *scRig) saved() config.Shortcuts { return r.h.Settings.Get().Shortcuts }

// awaitSaved waits for the saved bindings to be exactly want.
func (r *scRig) awaitSaved(want config.Shortcuts) {
	r.t.Helper()
	r.tt.Until(formatOverrides(want), func() bool { return maps.Equal(r.saved(), want) })
}

func formatOverrides(o config.Shortcuts) string {
	var parts []string
	for _, id := range slices.Sorted(maps.Keys(o)) {
		parts = append(parts, id+"="+o[id])
	}
	return "saved {" + strings.Join(parts, ", ") + "}"
}

// record clicks the recorder named name and waits until it is recording.
func (r *scRig) record(id, keys string) {
	r.t.Helper()
	r.click(id + " " + keys)
	r.show(id + " Press keys…")
}

func (r *scRig) press(mods ui.Modifiers, key ui.Key) {
	r.tt.Key(mods, key)
}

func (r *scRig) ran() map[string]int { return maps.Clone(r.runs) }

func (r *scRig) x(label string) float32 {
	r.t.Helper()
	rect, ok := r.tt.Find(label)
	if !ok {
		r.t.Fatalf("no %q; texts %q", label, r.tt.Texts())
	}
	return rect.X
}

func (r *scRig) y(label string) float32 {
	r.t.Helper()
	rect, ok := r.tt.Find(label)
	if !ok {
		r.t.Fatalf("no %q; texts %q", label, r.tt.Texts())
	}
	return rect.Y
}

// The section's rows are named by their action and the keycaps they show
// (Shortcuts.test.tsx: Label in Name), "not set" with no key, and sit in the
// groups in order.
func TestShortcutsRecordersAreNamedByTheirActionAndKeys(t *testing.T) {
	r := tallRig(t)
	for _, name := range []string{
		"view.sidebar Ctrl B", "test.close Esc", "test.free not set", "player.toggle Space", "player.next Ctrl →",
		"nav.search / or Ctrl F", "test.arrow →", "view.refresh F5", "nav.back Backspace",
	} {
		if _, ok := r.tt.Find(name); !ok {
			t.Errorf("no recorder named %q; texts %q", name, r.tt.Texts())
		}
	}
	// Each group has its heading, and "Not set" shows for the shortcut with no key.
	for _, h := range []string{"View", "Pages", "Navigation", "Not set"} {
		if !r.tt.HasText(h) {
			t.Errorf("no %q", h)
		}
	}
}

func TestShortcutsListGroupsInOrderInOneColumnWhereNarrow(t *testing.T) {
	// A 1000 DIP window leaves the section 636 DIP: below 46 rem.
	r := newScRig(t, 1000, 1800, nil)
	r.show("test.free not set")
	r.tt.Advance(10 * time.Millisecond) // the measured width settles
	order := []string{
		"player.toggle Space", "player.favourite F", "View", "view.sidebar Ctrl B", "view.twinB Ctrl Y", "Pages",
		"nav.search / or Ctrl F", "Navigation", "nav.back Backspace", "test.close Esc", "test.arrow →",
	}
	for i := 1; i < len(order); i++ {
		if r.y(order[i-1]) >= r.y(order[i]) {
			t.Errorf("%q is not above %q", order[i-1], order[i])
		}
	}
	// The rows' names start at one edge.
	x := r.x("player.toggle")
	for _, label := range []string{"view.sidebar", "nav.search", "nav.back", "test.close", "View", "Pages", "Navigation"} {
		if r.x(label) != x {
			t.Errorf("%q starts at %v, not at the column's %v", label, r.x(label), x)
		}
	}
}

func TestShortcutsRunInTwoColumnsWhereTheSectionIsWideEnough(t *testing.T) {
	r := tallRig(t)
	r.tt.Advance(10 * time.Millisecond) // the measured width settles
	// The groups run on from the first column to the second, in order.
	left, right := r.x("View"), r.x("Pages")
	if left >= right {
		t.Fatalf("two columns: View at %v, Pages at %v", left, right)
	}
	for _, label := range []string{"player.toggle", "view.sidebar", "view.twinA"} {
		if r.x(label) != left {
			t.Errorf("%q is not in the first column", label)
		}
	}
	for _, label := range []string{"Navigation", "nav.search", "nav.back", "test.close"} {
		if r.x(label) != right {
			t.Errorf("%q is not in the second column", label)
		}
	}
	// Each column starts at the top.
	if r.y("Pages") > r.y("view.sidebar") {
		t.Errorf("the second column starts below the first one's rows: Pages at %v", r.y("Pages"))
	}
}

// What is recorded for each class of key: a letter with Ctrl, with Ctrl and
// Shift, a function key, a digit with Alt, an arrow with Shift, Enter,
// Home, Insert, Page Down, "?" ("Shift+/"), "+", punctuation, Super, a bare
// letter. The binding is saved (and nothing the keys mean to the shell
// runs), then reset to no key.
func TestShortcutsRecordingEachKeyClass(t *testing.T) {
	r := tallRig(t)
	for _, tc := range []struct {
		name string
		mods ui.Modifiers
		key  ui.Key
		want string
		caps string // the recorder's name once it is set
	}{
		{"Ctrl+letter", ui.Ctrl, ui.KeyJ, "Ctrl+J", "Ctrl J"},
		{"Ctrl+Shift+letter", ui.Ctrl | ui.Shift, ui.KeyK, "Ctrl+Shift+K", "Ctrl Shift K"},
		{"function key", 0, ui.KeyF6, "F6", "F6"},
		{"Alt+digit", ui.Alt, ui.Key1, "Alt+1", "Alt 1"},
		{"Shift+arrow", ui.Shift, ui.KeyLeft, "Shift+ArrowLeft", "Shift ←"},
		{"Enter", 0, ui.KeyEnter, "Enter", "Enter"},
		{"Home", 0, ui.KeyHome, "Home", "Home"},
		{"Insert", 0, ui.KeyInsert, "Insert", "Insert"},
		{"Page Down", 0, ui.KeyPageDown, "PageDown", "Page Down"},
		{"question mark", ui.Shift, ui.KeySlash, "?", "?"},
		{"plus", ui.Shift, ui.KeyEqual, "Plus", "+"},
		{"Ctrl+plus", ui.Ctrl | ui.Shift, ui.KeyEqual, "Ctrl+Plus", "Ctrl +"},
		{"punctuation", 0, ui.KeyComma, ",", ","},
		{"backquote", 0, ui.KeyBackquote, "`", "`"},
		{"Super", ui.Super, ui.KeyK, "Meta+K", "Super K"},
		{"bare letter", 0, ui.KeyJ, "J", "J"},
	} {
		r.record("test.free", "not set")
		r.press(tc.mods, tc.key)
		r.awaitSaved(config.Shortcuts{"test.free": tc.want})
		r.show("test.free " + tc.caps)
		if r.tt.HasText("Press keys…") {
			t.Errorf("%s: still recording", tc.name)
		}
		// The reset button is there for the changed keys, and takes it back.
		r.click("Reset test.free to no key")
		r.awaitSaved(config.Shortcuts{})
		r.show("test.free not set")
	}
	if got := r.ran(); len(got) != 0 {
		t.Errorf("recording ran shortcuts: %v", got)
	}
}

// The keys the shell's dispatcher runs go to the recorder instead: Space,
// Escape, Backspace, Tab, an arrow, F5 and a Ctrl chord all reach it, and
// no shortcut runs. (MyGo's HandleInput never sees a key a window shortcut
// claims: the recorder asks for every chord as a shortcut of its own, which
// a key goes to first.)
func TestShortcutsRecorderTakesTheKeysTheShellRuns(t *testing.T) {
	r := tallRig(t)
	for _, tc := range []struct {
		name  string
		mods  ui.Modifiers
		key   ui.Key
		other string // the shortcut that has the chord
		chord string
	}{
		{"Space", 0, ui.KeySpace, "player.toggle", "Space"},
		{"arrow", 0, ui.KeyRight, "test.arrow", "→"},
		{"F5", 0, ui.KeyF5, "view.refresh", "F5"},
		{"Ctrl chord", ui.Ctrl, ui.KeyB, "view.sidebar", "Ctrl B"},
		{"Ctrl chord with two keys", ui.Ctrl, ui.KeyF, "nav.search", "Ctrl F"},
		{"Ctrl+K, the palette's", ui.Ctrl, ui.KeyK, "nav.palette", "Ctrl K"},
	} {
		r.record("test.free", "not set")
		r.press(tc.mods, tc.key)
		// It is a chord another shortcut has: the recorder asks, no key is
		// saved, and the other shortcut did not run.
		r.show("is used by “" + tc.other + "”.")
		if got := r.ran(); len(got) != 0 {
			t.Fatalf("%s: shortcuts ran: %v", tc.name, got)
		}
		if got := r.saved(); len(got) != 0 {
			t.Fatalf("%s: saved %v before the user chose", tc.name, got)
		}
		r.click("Cancel")
		r.hidden("is used by")
	}
	// Escape ends the recording, Backspace removes the keys, and neither runs
	// the shortcut that has the key.
	r.record("test.free", "not set")
	r.press(0, ui.KeyEscape)
	r.hidden("Press keys…")
	r.record("player.favourite", "F")
	r.press(0, ui.KeyBackspace)
	r.awaitSaved(config.Shortcuts{"player.favourite": "none"})
	r.record("test.free", "not set")
	r.press(0, ui.KeyDelete)
	r.hidden("Press keys…")
	if got := r.ran(); len(got) != 0 {
		t.Errorf("shortcuts ran: %v", got)
	}
	// The dispatcher is live: out of the recorder the same keys run.
	r.press(ui.Ctrl, ui.KeyB)
	r.press(0, ui.KeyF5)
	if got := r.ran(); got["view.sidebar"] != 1 || got["view.refresh"] != 1 || len(got) != 2 {
		t.Errorf("the dispatcher ran %v, want one Ctrl+B and one F5", got)
	}
}

// Tab and Shift+Tab move the focus, so they are refused with a reason, and
// the recording goes on.
func TestShortcutsTabIsRefusedAndTheRecordingGoesOn(t *testing.T) {
	r := tallRig(t)
	r.record("test.free", "not set")
	r.show("Press the new keys. Esc cancels, Backspace removes the shortcut.")
	r.press(0, ui.KeyTab)
	r.show("Tab moves between controls. Choose another key.")
	if !r.tt.Focused("test.free Press keys…") {
		t.Error("Tab moved the focus")
	}
	r.press(ui.Shift, ui.KeyTab)
	r.show("Shift+Tab moves between controls. Choose another key.")
	if !r.tt.Focused("test.free Press keys…") {
		t.Error("Shift+Tab moved the focus")
	}
	if got, ann := r.saved(), r.tt.Announcements(); len(got) != 0 || !slices.Contains(ann, "Shift+Tab moves between controls. Choose another key.") {
		t.Errorf("saved %v, announced %q", got, ann)
	}
	if got := r.ran(); len(got) != 0 {
		t.Errorf("shortcuts ran: %v", got)
	}
	// Then a key is taken as ever.
	r.press(ui.Ctrl, ui.KeyJ)
	r.awaitSaved(config.Shortcuts{"test.free": "Ctrl+J"})
	r.show("test.free Ctrl J")
	r.hidden("moves between controls")
}

// A modifier alone is no chord; blur stops the recording.
func TestShortcutsLoneModifiersAreIgnoredAndBlurStopsRecording(t *testing.T) {
	r := tallRig(t)
	r.record("test.free", "not set")
	r.tt.HoldModifiers(ui.Ctrl)
	r.tt.HoldModifiers(ui.Ctrl | ui.Shift)
	r.tt.HoldModifiers(0)
	if !r.tt.HasText("Press keys…") || len(r.saved()) != 0 {
		t.Fatalf("a modifier on its own ended the recording or saved %v", r.saved())
	}
	// Another window takes the keyboard.
	r.tt.SetFocused(false)
	r.hidden("Press keys…")
	r.tt.SetFocused(true)
	// A click elsewhere.
	r.record("test.free", "not set")
	r.click("Filter shortcuts")
	r.hidden("Press keys…")
	if got := r.saved(); len(got) != 0 {
		t.Errorf("saved %v", got)
	}
	// Clicking the recorder again ends the recording as well.
	r.record("test.free", "not set")
	r.click("test.free Press keys…")
	r.hidden("Press keys…")
}

// Enter and Space press the focused recorder, the keyboard's way to start;
// a key MyGo takes for another language's layout by position is the chord
// of the key at that place (Ctrl+J on a Cyrillic layout), and the key of
// no US key typed alone is no chord.
func TestShortcutsRecordFromTheKeyboardAndOnAnotherLayout(t *testing.T) {
	r := tallRig(t)
	r.record("test.free", "not set")
	r.press(0, ui.KeyEscape)
	r.hidden("Press keys…")
	if !r.tt.Focused("test.free not set") {
		t.Fatal("the recorder lost the focus")
	}
	r.press(0, ui.KeyEnter)
	r.show("test.free Press keys…")
	r.tt.KeyAt(ui.Ctrl, ui.KeyJ) // the key at J's place; the layout types no US key
	r.awaitSaved(config.Shortcuts{"test.free": "Ctrl+J"})
	r.press(0, ui.KeySpace)
	r.show("test.free Press keys…")
	r.tt.KeyAt(0, ui.KeyK) // plain: it types text, not a chord
	if !r.tt.HasText("Press keys…") || len(r.saved()) != 1 {
		t.Errorf("a key with no chord changed the recording: saved %v", r.saved())
	}
	r.press(0, ui.KeyEscape)
	if got := r.ran(); len(got) != 0 {
		t.Errorf("shortcuts ran: %v", got)
	}
}

// A chord another shortcut has asks first: Cancel, Escape and Use it here.
func TestShortcutsAConflictAsksCancelEscapeAndUseItHere(t *testing.T) {
	r := tallRig(t)
	r.record("test.free", "not set")
	r.press(0, ui.KeySpace)
	r.show("is used by “player.toggle”.", "Cancel", "Use it here")
	if !r.tt.Focused("Use it here") {
		t.Error("Use it here does not have the focus")
	}
	if ann := r.tt.Announcements(); !slices.Contains(ann, "Space is used by “player.toggle”.") {
		t.Errorf("the choice was not announced: %q", ann)
	}
	// Cancel: nothing changes and the recorder has the focus again.
	r.click("Cancel")
	r.hidden("Use it here")
	r.tt.Frame()
	if !r.tt.Focused("test.free not set") || len(r.saved()) != 0 {
		t.Errorf("after Cancel: recorder focused %v, saved %v", r.tt.Focused("test.free not set"), r.saved())
	}
	// Escape cancels too.
	r.record("test.free", "not set")
	r.press(0, ui.KeySpace)
	r.show("Use it here")
	r.press(0, ui.KeyEscape)
	r.hidden("Use it here")
	r.tt.Frame()
	if !r.tt.Focused("test.free not set") || len(r.saved()) != 0 {
		t.Errorf("after Escape: recorder focused %v, saved %v", r.tt.Focused("test.free not set"), r.saved())
	}
	// Use it here moves the key: the other shortcut has none left.
	r.record("test.free", "not set")
	r.press(0, ui.KeySpace)
	r.click("Use it here")
	r.awaitSaved(config.Shortcuts{"test.free": "Space", "player.toggle": "none"})
	r.show("test.free Space", "player.toggle not set")
	r.hidden("is used by")
	if got := r.ran(); len(got) != 0 {
		t.Errorf("shortcuts ran: %v", got)
	}
}

// Stealing a chord leaves the other shortcut its other keys (registry.test.ts).
func TestShortcutsStealingAChordKeepsTheOthersOtherKeys(t *testing.T) {
	r := tallRig(t)
	r.record("player.favourite", "F")
	r.press(ui.Ctrl, ui.KeyF)
	r.show("is used by “nav.search”.")
	r.click("Use it here")
	r.awaitSaved(config.Shortcuts{"player.favourite": "Ctrl+F", "nav.search": "/"})
	r.show("player.favourite Ctrl F", "nav.search /")
	// The shell sees it: Ctrl+F runs the favourite, "/" still searches.
	keys := map[string][]string{}
	for _, b := range r.s.shortcuts() {
		keys[b.ID] = b.Keys
	}
	if !slices.Equal(keys["player.favourite"], []string{"Ctrl+F"}) || !slices.Equal(keys["nav.search"], []string{"/"}) {
		t.Errorf("the shell lists %v", keys)
	}
	r.press(ui.Ctrl, ui.KeyF)
	r.press(0, ui.KeySlash)
	if got := r.ran(); got["player.favourite"] != 1 || got["nav.search"] != 1 || len(got) != 2 {
		t.Errorf("ran %v, want the favourite once and the search once", got)
	}
}

// A rebinding takes effect: saved in Go's settings (what the shell's
// dispatcher reads on settings:changed), listed by Env.Shortcuts with the
// new keys, and the new chord runs the shortcut while the old one does not.
func TestShortcutsARebindingIsSavedAndTakesEffect(t *testing.T) {
	r := tallRig(t)
	r.record("player.favourite", "F")
	r.press(ui.Ctrl, ui.KeyJ)
	r.awaitSaved(config.Shortcuts{"player.favourite": "Ctrl+J"})
	r.show("player.favourite Ctrl J")
	if got := services.Service[services.SystemService](r.h.Core).Settings().Shortcuts; !maps.Equal(got, config.Shortcuts{"player.favourite": "Ctrl+J"}) {
		t.Errorf("Go's settings hold %v", got)
	}
	r.press(0, ui.KeyF) // the old chord, out of the recorder: nothing
	r.press(ui.Ctrl, ui.KeyJ)
	if got := r.ran(); got["player.favourite"] != 1 || len(got) != 1 {
		t.Errorf("ran %v, want the favourite once, by its new chord", got)
	}
	// And the summary in the list follows: one shortcut changed.
	r.show("1 changed")
}

// A key that is its default is stored as nothing: the override is removed.
func TestShortcutsAKeyBackOnItsDefaultIsNotStored(t *testing.T) {
	r := tallRig(t)
	r.record("player.favourite", "F")
	r.press(0, ui.KeyJ)
	r.awaitSaved(config.Shortcuts{"player.favourite": "J"})
	r.record("player.favourite", "J")
	r.press(0, ui.KeyF)
	r.awaitSaved(config.Shortcuts{})
	r.show("player.favourite F")
	r.hidden("Reset player.favourite")
}

func TestShortcutsEscapeCancelsAndBackspaceRemoves(t *testing.T) {
	r := tallRig(t)
	r.record("player.favourite", "F")
	r.press(0, ui.KeyEscape)
	r.show("player.favourite F")
	r.hidden("Press keys…")
	if got := r.saved(); len(got) != 0 {
		t.Errorf("Escape saved %v", got)
	}
	r.record("player.favourite", "F")
	r.press(0, ui.KeyBackspace)
	r.awaitSaved(config.Shortcuts{"player.favourite": "none"})
	r.show("player.favourite not set", "Reset player.favourite to F")
	// Delete as well, on a shortcut with two keys.
	r.record("nav.search", "/ or Ctrl F")
	r.press(0, ui.KeyDelete)
	r.awaitSaved(config.Shortcuts{"player.favourite": "none", "nav.search": "none"})
	r.show("nav.search not set")
	// The reset names the defaults as they are spoken.
	r.show("Reset nav.search to Slash or Ctrl+F")
	r.click("Reset nav.search to Slash or Ctrl+F")
	r.awaitSaved(config.Shortcuts{"player.favourite": "none"})
	r.show("nav.search / or Ctrl F")
}

// A reset goes back to the default and returns the focus to the recorder,
// since the reset button goes with it.
func TestShortcutsResetOneReturnsTheFocusToTheRecorder(t *testing.T) {
	r := tallRig(t)
	r.record("player.favourite", "F")
	r.press(ui.Ctrl, ui.KeyJ)
	r.awaitSaved(config.Shortcuts{"player.favourite": "Ctrl+J"})
	r.click("Reset player.favourite to F")
	r.awaitSaved(config.Shortcuts{})
	r.show("player.favourite F")
	r.hidden("Reset player.favourite")
	r.tt.Frame()
	if !r.tt.Focused("player.favourite F") {
		t.Error("the focus did not return to the recorder")
	}
	// A shortcut with no default key resets to "no key".
	r.record("test.free", "not set")
	r.press(ui.Ctrl, ui.KeyU)
	r.awaitSaved(config.Shortcuts{"test.free": "Ctrl+U"})
	r.show("Reset test.free to no key")
}

// A conflict that exists already is marked and says what it conflicts with.
func TestShortcutsExistingConflictsAreMarkedAndCounted(t *testing.T) {
	r := tallRig(t)
	r.show("Also used by “view.twinB”. Only the first one runs.", "Also used by “view.twinA”. Only the first one runs.")
	r.show("2 conflicts") // the section list's summary
	// The recorder's description is the note.
	if _, ok := r.tt.Find("view.twinA Ctrl Y"); !ok {
		t.Fatalf("no recorder for the conflict; texts %q", r.tt.Texts())
	}
	// Moving one away ends the conflict and the notes.
	r.record("view.twinB", "Ctrl Y")
	r.press(ui.Ctrl, ui.KeyU)
	r.awaitSaved(config.Shortcuts{"view.twinB": "Ctrl+U"})
	r.hidden("Also used by")
	r.hidden("conflict")
	// Only the first of two on a chord runs.
	r.press(ui.Ctrl, ui.KeyY)
	if got := r.ran(); got["view.twinA"] != 1 || len(got) != 1 {
		t.Errorf("ran %v", got)
	}
}

func TestShortcutsResetAllAsksFirstAndGoesBackToDefaults(t *testing.T) {
	r := tallRig(t)
	// Nothing changed: the button is there, dimmed, and does nothing.
	r.click("Reset all")
	r.tt.Frame()
	if kit.ConfirmPending() {
		t.Fatal("Reset all asked with nothing to reset")
	}
	r.record("test.free", "not set")
	r.press(ui.Ctrl, ui.KeyJ)
	r.awaitSaved(config.Shortcuts{"test.free": "Ctrl+J"})
	r.record("player.favourite", "F")
	r.press(ui.Ctrl, ui.KeyL)
	r.awaitSaved(config.Shortcuts{"test.free": "Ctrl+J", "player.favourite": "Ctrl+L"})
	r.press(ui.Ctrl, ui.KeyJ)
	if r.ran()["test.free"] != 1 {
		t.Fatal("the new chord does not run before the reset")
	}
	// Cancel keeps them.
	r.click("Reset all")
	r.show("Reset every shortcut?", "All shortcuts go back to their default keys.")
	r.click("Cancel")
	r.hidden("Reset every shortcut?")
	if len(r.saved()) != 2 {
		t.Errorf("Cancel changed the bindings: %v", r.saved())
	}
	// Confirm (its button has the focus): every shortcut is back to its keys.
	r.click("Reset all")
	r.show("Reset every shortcut?")
	r.press(0, ui.KeyEnter)
	r.awaitSaved(config.Shortcuts{})
	r.show("test.free not set", "player.favourite F")
	r.hidden("Reset every shortcut?", "Reset test.free")
	r.press(ui.Ctrl, ui.KeyJ)
	r.press(0, ui.KeyF)
	if got := r.ran(); got["test.free"] != 1 || got["player.favourite"] != 1 || len(got) != 2 {
		t.Errorf("ran %v: Ctrl+J must be free again and F the favourite's", got)
	}
}

func TestShortcutsFilterByActionOrKey(t *testing.T) {
	r := tallRig(t)
	r.click("Filter shortcuts")
	r.tt.Type("sidebar")
	r.show("view.sidebar Ctrl B")
	r.hidden("player.toggle Space", "nav.back Backspace", "test.free not set")
	if _, ok := r.tt.Find("View"); !ok {
		t.Error("the group of a shown shortcut has no heading")
	}
	if r.tt.HasText("Pages") {
		t.Error("a group with nothing shown has a heading")
	}
	// By the keys, as spoken: any case.
	r.click("Clear search")
	r.show("player.toggle Space")
	r.click("Filter shortcuts")
	r.tt.Type("CTRL+Right Arrow")
	r.show("player.next Ctrl →")
	r.hidden("test.arrow →", "view.sidebar Ctrl B")
	r.click("Clear search")
	r.click("Filter shortcuts")
	r.tt.Type("right arrow")
	r.show("player.next Ctrl →", "test.arrow →")
	r.hidden("player.toggle Space")
	// Nothing matches.
	r.click("Clear search")
	r.click("Filter shortcuts")
	r.tt.Type("zzz")
	r.show("No shortcut matches “zzz”.")
	r.hidden("player.toggle Space", "test.free not set")
	if ann := r.tt.Announcements(); !slices.Contains(ann, "No shortcut matches “zzz”.") {
		t.Errorf("announced %q", ann)
	}
	r.click("Clear search")
	r.hidden("No shortcut matches")
	r.show("player.toggle Space", "test.free not set")
}

// Find a setting lands on a shortcut's recorder; every shortcut has its
// row, by its find ID.
func TestShortcutsFindLandsOnTheRecorder(t *testing.T) {
	r := tallRig(t)
	r.click("Find a setting")
	r.tt.Type("favourite")
	r.show("player.favourite")
	r.click("player.favourite")
	r.tt.Until("the recorder has the focus", func() bool { return r.tt.Focused("player.favourite F") })
	if r.s.last != secShortcuts {
		t.Errorf("section %q", r.s.last)
	}
	// Every shortcut of the index has its row.
	n := 0
	for _, e := range settingIndex(r.s.shortcuts()) {
		if e.Section != secShortcuts {
			continue
		}
		n++
		if !r.s.seen[e.ID] {
			t.Errorf("no row for %s", e.ID)
		}
		if !strings.HasPrefix(e.ID, "shortcut-") {
			t.Errorf("find ID %q", e.ID)
		}
	}
	if n != len(r.list) {
		t.Errorf("%d shortcut entries for %d shortcuts", n, len(r.list))
	}
}

func TestShortcutsLoadSkeletonThenRows(t *testing.T) {
	r := newScRig(t, 1132, 1500, nil)
	r.tt.Frame()
	if _, ok := r.tt.Find("Loading"); !ok {
		t.Fatalf("no loading state; texts %q", r.tt.Texts())
	}
	if r.tt.HasText("player.toggle") {
		t.Error("rows before the settings")
	}
	r.show("player.toggle Space")
	if r.tt.HasText("Loading") {
		t.Error("still loading")
	}
}

// A settings read that fails shows the error with Try again, never the
// rows; the settings arriving (here by event) replace it.
func TestShortcutsReadErrorHasRetryAndRecovers(t *testing.T) {
	r := newScRig(t, 1132, 1500, func(r *scRig) { r.h.Env.Core = nil })
	r.show("Couldn't read Sona's settings", "Try again")
	if r.tt.HasText("player.toggle") || r.tt.HasText("Loading") {
		t.Errorf("an error shows rows or a skeleton: %q", r.tt.Texts())
	}
	r.click("Try again") // asks again; the service is still not there
	r.tt.Frame()
	r.show("Couldn't read Sona's settings")
	r.h.Env.Core = r.h.Core
	r.h.Bus.Emit(services.EventSettingsChanged, r.h.Settings.Get())
	r.show("player.toggle Space")
	r.hidden("Couldn't read Sona's settings")
}

// The settings come from Go's store: bindings saved earlier show at once,
// and a changed one arriving by event (another window, the file) shows.
func TestShortcutsShowSavedBindingsAndFollowSettingsChanged(t *testing.T) {
	r := newScRig(t, 1132, 1500, func(r *scRig) {
		if _, err := r.h.Settings.Update(func(s *config.Settings) {
			s.Shortcuts = config.Shortcuts{"player.favourite": "Ctrl+Shift+J", "test.close": "none", "view.refresh": "Hyper+F"}
		}); err != nil {
			t.Fatal(err)
		}
	})
	// An invalid binding keeps the defaults; the changed ones show.
	r.show("player.favourite Ctrl Shift J", "test.close not set", "view.refresh F5")
	r.show("Reset player.favourite to F", "Reset test.close to Escape", "Reset view.refresh to F5")
	next := r.h.Settings.Get()
	next.Shortcuts = config.Shortcuts{"player.toggle": "K"}
	r.h.Bus.Emit(services.EventSettingsChanged, next)
	r.show("player.toggle K", "player.favourite F", "test.close Esc")
}
