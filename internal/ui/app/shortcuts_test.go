package app

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/module"
)

// On macOS the Ctrl defaults are Command chords (a focused AppKit text field
// takes Ctrl+B/F/P/N/H/D/A/E/K itself); elsewhere they stay Ctrl.
func TestCtrlDefaultsAreCommandChordsOnMacOnly(t *testing.T) {
	list := []module.Shortcut{
		{ID: "a", Keys: []string{"Ctrl+K", "/", "Ctrl+Shift+M", "Alt+1", "Ctrl++", "Ctrl+ArrowLeft"}},
		{ID: "b"},
	}
	mac := platformKeys(list, ui.Super)
	want := []string{"Meta+K", "/", "Meta+Shift+M", "Alt+1", "Meta++", "Meta+ArrowLeft"}
	if !slices.Equal(mac[0].Keys, want) {
		t.Errorf("macOS keys = %q, want %q", mac[0].Keys, want)
	}
	if list[0].Keys[0] != "Ctrl+K" {
		t.Error("the registered shortcut was rewritten in place")
	}
	for _, k := range mac[0].Keys {
		if _, ok := parseChord(k); !ok {
			t.Errorf("%q is not a chord", k)
		}
	}
	if other := platformKeys(list, ui.Ctrl); other[0].Keys[0] != "Ctrl+K" {
		t.Errorf("non-macOS keys = %q", other[0].Keys)
	}
}

// A user's saved binding keeps what they saved, even a Ctrl chord on macOS.
func TestSavedBindingsAreNotMappedToCommand(t *testing.T) {
	list := platformKeys([]module.Shortcut{{ID: "a", Keys: []string{"Ctrl+K"}}, {ID: "b", Keys: []string{"Ctrl+B"}}}, ui.Super)
	d := newDispatcher(list, config.Shortcuts{"a": "Ctrl+J"})
	got := d.bindings()
	if !slices.Equal(got[0].Keys, []string{"Ctrl+J"}) {
		t.Errorf("overridden keys = %q", got[0].Keys)
	}
	if !slices.Equal(got[1].Keys, []string{"Meta+B"}) {
		t.Errorf("default keys = %q", got[1].Keys)
	}
	c, _ := parseChord("Ctrl+J")
	if !slices.Contains(d.list[0].chords, c) {
		t.Error("the saved Ctrl chord is not bound")
	}
}
