package settings

import (
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/module"
)

// linuxCaps is a Linux desktop where everything works (v0.2.0's
// testing/capabilities linuxCapabilities).
func linuxCaps() services.Capabilities {
	return services.Capabilities{
		OS: "linux",
		Playback: services.PlaybackCapabilities{
			Available: true, Engine: "cdm", Volume: true, Equalizer: true, SoundCheck: true,
			Crossfade: true, OutputDevice: true, PrivateListening: true, Video: true,
		},
		MediaControls: true, Notifications: true, Tray: true, Autostart: true, SleepInhibit: true,
		PowerState: true, FileChooser: true, CLI: true, DiscordPresence: true, DeveloperTools: true,
		WindowControls: services.WindowControls{Native: "frame", Automatic: "desktop", Style: "auto"},
	}
}

// fakeShortcuts is one shortcut in the View group.
func fakeShortcuts() []module.BoundShortcut {
	return []module.BoundShortcut{{Shortcut: module.Shortcut{
		ID: "test.sidebar", Label: func() string { return "Show or hide the sidebar" }, Group: "view",
	}}}
}

func findIDs(query string, ctx findContext) []string {
	var ids []string
	for _, e := range findSettings(settingIndex(fakeShortcuts()), query, ctx, "en") {
		ids = append(ids, e.ID)
	}
	return ids
}

func testContext() findContext {
	caps := linuxCaps()
	st := config.Defaults()
	st.Desktop.Autostart = true
	st.Crossfade.Enabled = true
	return findContext{caps: &caps, settings: &st, languageRow: true}
}

func TestFindPutsTitlesThatStartWithTheQueryFirst(t *testing.T) {
	ctx := testContext()
	if got := findIDs("cross", ctx); !slices.Equal(got, []string{"crossfade", "crossfade-seconds"}) {
		t.Errorf("cross: %v", got)
	}
	if got := findIDs("lyrics", ctx); len(got) == 0 || got[0] != "lyrics-sources" {
		t.Errorf("lyrics: %v", got)
	}
}

func TestFindMatchesOtherWordsGroupAndSection(t *testing.T) {
	ctx := testContext()
	if got := findIDs("dark", ctx); !slices.Equal(got, []string{"theme"}) {
		t.Errorf("dark: %v", got)
	}
	if got := findIDs("tray", ctx); !slices.Contains(got, "close-action") {
		t.Errorf("tray: %v", got)
	}
	if got := findIDs("scrobbling", ctx); !slices.Equal(got, []string{"scrobble-lastfm", "scrobble-listenbrainz"}) {
		t.Errorf("scrobbling: %v", got)
	}
}

func TestFindNeedsEveryWordInAnyCaseWithoutAccents(t *testing.T) {
	ctx := testContext()
	if got := findIDs("CACHE artwork", ctx); !slices.Equal(got, []string{"art-cache"}) {
		t.Errorf("CACHE artwork: %v", got)
	}
	if got := findIDs("équaliser", ctx); !slices.Equal(got, []string{"equalizer"}) {
		t.Errorf("équaliser: %v", got)
	}
	if got := findIDs("nothing like this", ctx); len(got) != 0 {
		t.Errorf("nothing like this: %v", got)
	}
}

func TestFindLeavesOutRowsTheirSectionDoesNotShow(t *testing.T) {
	ctx := testContext()
	off := *ctx.settings
	off.Crossfade.Enabled = false
	noFade := ctx
	noFade.settings = &off
	if got := findIDs("crossfade", noFade); !slices.Equal(got, []string{"crossfade"}) {
		t.Errorf("crossfade off: %v", got)
	}
	noLang := ctx
	noLang.languageRow = false
	if got := findIDs("language", noLang); !slices.Equal(got, []string{"lyrics-translation"}) {
		t.Errorf("no language row: %v", got)
	}
	caps := linuxCaps()
	caps.Playback.Available = false
	noPlayback := ctx
	noPlayback.caps = &caps
	if got := findIDs("audio quality", noPlayback); len(got) != 0 {
		t.Errorf("no engine: %v", got)
	}
	// Start in the tray only with autostart on and a tray.
	noAuto := *ctx.settings
	noAuto.Desktop.Autostart = false
	c2 := ctx
	c2.settings = &noAuto
	if got := findIDs("start in the tray", c2); len(got) != 0 {
		t.Errorf("autostart off: %v", got)
	}
}

func TestFindListsTitleBarOnlyWhereTheWindowHasAFrame(t *testing.T) {
	ctx := testContext()
	if got := findIDs("title bar", ctx); !slices.Contains(got, "title-bar") {
		t.Errorf("frame: %v", got)
	}
	if got := findIDs("decorations", ctx); !slices.Equal(got, []string{"title-bar"}) {
		t.Errorf("decorations: %v", got)
	}
	for _, native := range []string{"inset", "none"} {
		caps := linuxCaps()
		caps.WindowControls.Native = native
		c := ctx
		c.caps = &caps
		if got := findIDs("title bar", c); slices.Contains(got, "title-bar") {
			t.Errorf("Native %q: %v", native, got)
		}
	}
	loading := ctx
	loading.caps = nil
	if got := findIDs("title bar", loading); slices.Contains(got, "title-bar") {
		t.Errorf("capabilities loading: %v", got)
	}
}

func TestFindOpenOnByItsKeywords(t *testing.T) {
	ctx := testContext()
	for _, q := range []string{"open on", "start page", "resume"} {
		if got := findIDs(q, ctx); !slices.Contains(got, "start-page") {
			t.Errorf("%q: %v", q, got)
		}
	}
}

func TestFindListsShortcutsByTheirAction(t *testing.T) {
	if got := findIDs("sidebar", testContext()); !slices.Equal(got, []string{"shortcut-test.sidebar"}) {
		t.Errorf("sidebar: %v", got)
	}
}

func TestSettingIndexIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range settingIndex(fakeShortcuts()) {
		if seen[e.ID] {
			t.Errorf("entry %q is listed twice", e.ID)
		}
		seen[e.ID] = true
	}
}
