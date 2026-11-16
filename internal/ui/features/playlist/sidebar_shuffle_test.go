package playlist

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// shuffled waits for one play and reports it.
func (e *sbEnv) shuffled(want string) {
	e.t.Helper()
	e.tt.Until("the shuffled play", func() bool { return len(e.play.played()) == 1 })
	// Shuffle starts from a random song: compare what plays, not where.
	got := e.play.played()[0]
	if i := strings.Index(got, " start="); i >= 0 {
		j := strings.Index(got[i+1:], " ")
		got = got[:i] + got[i+1+j:]
	}
	if got != want {
		e.t.Errorf("played %q, want %q", got, want)
	}
	e.play.mu.Lock()
	on := e.play.shuffle
	e.play.mu.Unlock()
	if !on {
		e.t.Error("shuffle was not turned on")
	}
}

// Over a row the artwork's shuffle button shows, and clicking it shuffles
// that playlist without opening it.
func TestHoveringARowShowsItsShuffleButtonAndClickingShufflesWithoutNavigating(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.api.pinTracks["p.own"] = []services.Song{song("s1", "One", "A"), song("s2", "Two", "A")}
	e.shows("Road Trip", "Morning Mix")

	r, ok := e.tt.Find("Shuffle Road Trip")
	if !ok || r.W != rowArt {
		t.Fatalf("the button over the artwork: %v, %v", r, ok)
	}
	if got := e.router.Path(); got != "/library/albums" {
		t.Fatalf("started at %q", got)
	}

	e.hover("Road Trip")
	if err := e.tt.Click("Shuffle Road Trip"); err != nil {
		t.Fatal(err)
	}
	e.shuffled("[s1 s2] from=playlist:p.own")
	if got := e.router.Path(); got != "/library/albums" {
		t.Errorf("shuffling navigated to %q", got)
	}
}

// Every row of the sections has its button, and it is named for its playlist.
func TestEveryPlaylistRowHasAShuffleButton(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.shows("Morning Mix", "Quiet Hours", "Road Trip")
	tr := access.Read(t, e.tt.Tester)
	for _, name := range []string{"Morning Mix", "Discovery Mix", "Quiet Hours", "Road Trip"} {
		if _, ok := tr.Find(ui.RoleButton, "Shuffle "+name); !ok {
			t.Errorf("no button %q:\n%s", "Shuffle "+name, tr.Dump())
		}
	}
}

// A playlist pin has one too, and so does an album pin (both shuffle on
// Shift+Enter); a song pin (one track) and an artist pin (no player) do not.
func TestPlaylistAndAlbumPinsHaveAShuffleButtonAndOtherPinsDoNot(t *testing.T) {
	e := sbSetup(t, sbLibrary(), []services.Pin{
		{Kind: "playlist", ID: "p.a", Name: "Alpha", Action: "drillIn"},
		{Kind: "album", ID: "l.b", Name: "Beta", Action: "play"},
		{Kind: "song", ID: "i.c", Name: "Gamma", Action: "play", AlbumID: "l.cc"},
		{Kind: "artist", ID: "r.d", Name: "Delta", Action: "drillIn"},
	})
	e.api.pinTracks["p.a"] = []services.Song{song("s1", "One", "A")}
	e.shows("Alpha", "Beta, plays", "Gamma, plays", "Delta")
	tr := access.Read(t, e.tt.Tester)
	for _, name := range []string{"Alpha", "Beta"} {
		if _, ok := tr.Find(ui.RoleButton, "Shuffle "+name); !ok {
			t.Errorf("no shuffle button for %s:\n%s", name, tr.Dump())
		}
	}
	for _, name := range []string{"Gamma", "Delta"} {
		if _, ok := tr.Find(ui.RoleButton, "Shuffle "+name); ok {
			t.Errorf("the pin %s has a shuffle button", name)
		}
	}
	e.hover("Alpha")
	if err := e.tt.Click("Shuffle Alpha"); err != nil {
		t.Fatal(err)
	}
	e.shuffled("[s1] from=playlist:p.a")
	if got := e.router.Path(); got != "/library/albums" {
		t.Errorf("shuffling a pin navigated to %q", got)
	}
}

// The button is a sibling of the row's link, never inside it, and is no Tab
// stop of its own.
func TestShuffleButtonIsNotNestedAndAddsNoTabStopPerRow(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.shows("Road Trip")
	e.tt.Settle()
	access.NoNestedControls(t, e.tt.Tester)
	for _, s := range stopLabels(access.TabWalk(t, e.tt.Tester, 120)) {
		if strings.HasPrefix(s, "Shuffle ") {
			t.Errorf("Tab stops at %q", s)
		}
	}
}

// Without the pointer: Shift+Enter on the focused row shuffles it (Enter
// still opens it), and the row's menu lists Shuffle.
func TestShuffleFromTheKeyboard(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.api.pinTracks["p.own"] = []services.Song{song("s1", "One", "A")}
	e.shows("Road Trip")
	for range 60 {
		e.tt.Key(0, ui.KeyTab)
		e.tt.Frame()
		if tr := access.Read(t, e.tt.Tester); tr.Focus != nil && tr.Focus.Label == "Road Trip" {
			break
		}
	}
	if tr := access.Read(t, e.tt.Tester); tr.Focus == nil || tr.Focus.Label != "Road Trip" {
		t.Fatal("Tab never reached Road Trip")
	}
	e.tt.Key(ui.Shift, ui.KeyEnter)
	e.shuffled("[s1] from=playlist:p.own")
	if got := e.router.Path(); got != "/library/albums" {
		t.Errorf("Shift+Enter navigated to %q", got)
	}
}

func TestPlaylistMenuShufflesFromTheSidebar(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.api.pinTracks["p.own"] = []services.Song{song("s1", "One", "A")}
	e.shows("Road Trip")
	e.more("Road Trip", "More for Road Trip")
	e.choose("Shuffle")
	e.shuffled("[s1] from=playlist:p.own")
}

// A playlist with no songs, or one the user cannot play now, has its button
// off with the reason, and nothing plays.
func TestShuffleButtonSaysWhyItCannotPlay(t *testing.T) {
	inBubble(t, shuffleButtonSaysWhyItCannotPlay)
}

func shuffleButtonSaysWhyItCannotPlay(t *testing.T) {
	tree := sbLibrary()
	tree.Playlists[0].Playlist.TrackCount = 0 // Road Trip
	e := sbSetup(t, tree, nil)
	e.api.pinTracks["p.own"] = []services.Song{song("s1", "One", "A")}
	e.shows("Road Trip")
	tr := access.Read(t, e.tt.Tester)
	n, ok := tr.Find(ui.RoleButton, "Shuffle Road Trip")
	if !ok || n.Description != "No songs to play" || !n.Has(access.Disabled) {
		t.Fatalf("an empty playlist's button: %+v, %v", n, ok)
	}
	e.hover("Road Trip")
	_ = e.tt.Click("Shuffle Road Trip")
	e.quiet()
	if got := e.play.played(); len(got) != 0 {
		t.Errorf("an empty playlist played %q", got)
	}
}

func TestShuffleButtonIsOffWhileSignedOut(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.h.Env.Play.Patch(func(s *services.PlaybackState) { s.AuthRequired = true })
	e.shows("Morning Mix")
	n, ok := access.Read(t, e.tt.Tester).Find(ui.RoleButton, "Shuffle Morning Mix")
	if !ok || n.Description != "Sign in again to play" {
		t.Fatalf("the button while signed out: %+v, %v", n, ok)
	}
}

// A row hovered, as the sidebar draws it, in both themes: the shuffle glyph
// (not the play triangle) on its wash over the artwork, in ink.
func TestPlaylistSidebarHoverGolden(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{{"playlist-sidebar-hover-light", theme.ModeLight}, {"playlist-sidebar-hover-dark", theme.ModeDark}} {
		t.Run(tc.name, func(t *testing.T) {
			goldenFont(t)
			e := sbSetup(t, sbLibrary(), sbPlaylistPins("p.own"))
			theme.SetMode(tc.mode)
			e.shows("Pinned", "Apple Music", "Playlists", "Running")
			e.hover("Quiet Hours")
			e.tt.Settle()
			uitest.Golden(t, e.tt.Image(), tc.name)
		})
	}
}

// The pins of each kind and setting, in both themes: a pin that opens is a
// bare link; one that plays carries the play triangle at its end, one that
// shuffles the shuffle glyph (so neither reads as a link). The pin hovered
// shows the shuffle glyph over its art and gives its end to the More button.
func TestPlaylistSidebarPinsGolden(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{{"playlist-sidebar-pins-light", theme.ModeLight}, {"playlist-sidebar-pins-dark", theme.ModeDark}} {
		t.Run(tc.name, func(t *testing.T) {
			goldenFont(t)
			pins := []services.Pin{
				{Kind: "playlist", ID: "p.a", Name: "Opens", Action: "drillIn", Art: fixtureArt(1)},
				{Kind: "album", ID: "l.a", Name: "Plays", Action: "play", Art: fixtureArt(2)},
				{Kind: "playlist", ID: "p.b", Name: "Shuffles", Action: "shuffle", Art: fixtureArt(3)},
				{Kind: "artist", ID: "r.a", Name: "Artist opens", Action: "play", Art: fixtureArt(4)},
				{Kind: "song", ID: "i.a", Name: "Song plays", Action: "play", Art: fixtureArt(5)},
			}
			e := sbSetupWith(t, sbLibrary(), pins, nil)
			theme.SetMode(tc.mode)
			e.shows("Pinned", "Opens", "Artist opens")
			e.hover("Opens")
			e.tt.Settle()
			uitest.Golden(t, e.tt.Image(), tc.name)
		})
	}
}
