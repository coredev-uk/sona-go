package commands

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
)

// Interaction review (#311): what Enter, a click and Shift+Enter do to each
// entry of the command palette's library section and to its "Search Apple
// Music for" entry, pinned as today's code behaves. A "DEPARTS" comment marks
// an assertion where that departs from the rule: choosing a result opens it,
// Play plays, and a modifier never does what no visible cue hints at.

// ixLib answers the palette's library lookup: one artist, album and playlist
// of invented names (their pages open), and the library's real songs for
// the term (a song plays).
type ixLib struct{ songs []services.Song }

func (l ixLib) Library(_ context.Context, term string, _ int) (services.LibrarySearch, error) {
	r := services.LibrarySearch{
		Artists:   []services.Artist{{ID: "l.ar1", Source: services.SourceLibrary, Name: "Zed Artist", Genre: "Rock"}},
		Albums:    []services.Album{{ID: "l.al1", Source: services.SourceLibrary, Name: "Zed Album", Artist: "Mia"}},
		Playlists: []services.Playlist{{ID: "p.pl1", Source: services.SourceLibrary, Name: "Zed Playlist", Curator: "Mia"}},
	}
	if strings.HasPrefix(term, "zed") {
		r.Songs = l.songs
	}
	return r, nil
}

// ixPalette opens the palette on "zed", a term that matches no command and
// finds the four library kinds.
func ixPalette(t *testing.T) *rig {
	t.Helper()
	r := newRig(t)
	songs, err := r.m.lib.Library(context.Background(), "quartz", 4)
	if err != nil || len(songs.Songs) == 0 {
		t.Fatalf("the fixture library has no Quartz Lantern: %v", err)
	}
	r.m.lib = ixLib{songs: songs.Songs}
	r.tt.Key(ui.Ctrl, ui.KeyK)
	r.tt.Type("zed")
	r.tt.Advance(libraryDelay + 10*time.Millisecond)
	r.tt.Until("the library section", r.tt.Shows("In Your Library", "Zed Artist", "Zed Album", "Zed Playlist", "Quartz Lantern", "Search Apple Music for “zed”"))
	r.settle() // the list takes the height of its rows a frame after they arrive
	r.tt.Frame()
	return r
}

// outcome is what the gesture did: where it navigated, what it queued.
func (r *rig) outcome(start string) string {
	r.settle()
	var parts []string
	for range 200 {
		r.h.Main.Drain()
		if q := r.h.Env.Play.Queue(); len(q.Items) > 0 {
			break
		}
		if r.router.Location() != start {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	r.tt.Frame()
	if loc := r.router.Location(); loc != start {
		parts = append(parts, "nav "+loc)
	}
	for _, it := range r.h.Env.Play.Queue().Items {
		parts = append(parts, "queue "+it.Name)
	}
	if r.m.pal.IsOpen() {
		parts = append(parts, "palette still open")
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, "; ")
}

// activate makes the option named name the palette's active row, by the keys.
func (r *rig) activate(name string) {
	r.t.Helper()
	for range 20 {
		if info, ok := r.tt.Access(name); ok && info.Selected {
			return
		}
		r.tt.Key(0, ui.KeyDown)
	}
	r.t.Fatalf("could not make %q the active row; texts %q", name, r.tt.Texts())
}

// TestInteractionPaletteLibraryEntriesPerKind: Enter, a click and Shift+Enter
// on a library entry of each kind. The artist, album and playlist open their
// pages; a song plays at once.
func TestInteractionPaletteLibraryEntriesPerKind(t *testing.T) {
	type gesture struct {
		name string
		do   func(r *rig, row string)
	}
	gestures := []gesture{
		{"click", func(r *rig, row string) { r.tt.MustClick(row) }},
		{"Enter", func(r *rig, row string) { r.activate(row); r.tt.Key(0, ui.KeyEnter) }},
		{"Shift+Enter", func(r *rig, row string) { r.activate(row); r.tt.Key(ui.Shift, ui.KeyEnter) }},
	}
	cases := []struct {
		row, kind, want string
	}{
		{"Zed Artist", "artist", "nav /artist/l.ar1"},
		{"Zed Album", "album", "nav /album/l.al1"},
		{"Zed Playlist", "playlist", "nav /playlist/p.pl1"},
		// DEPARTS: an artist, album or playlist entry opens its page but a song entry plays at once, with the same look (cover, name, "Kind · artist") and no glyph to tell them apart.
		{"Quartz Lantern", "song", "queue Quartz Lantern"},
	}
	for _, tc := range cases {
		for _, g := range gestures {
			t.Run(tc.kind+"/"+g.name, func(t *testing.T) {
				r := ixPalette(t)
				start := r.router.Location()
				g.do(r, tc.row)
				// Shift+Enter is Enter: the palette has no modifier of its own.
				want := tc.want
				if got := r.outcome(start); got != want {
					t.Errorf("%s on the %s entry: %s, want %s", g.name, tc.kind, got, want)
				}
			})
		}
	}
}

// TestInteractionPaletteSearchAppleMusicEntry: the "Search Apple Music for"
// entry opens the search by click, Enter and Shift+Enter, and is the only one
// that does not name a result.
func TestInteractionPaletteSearchAppleMusicEntry(t *testing.T) {
	for _, name := range []string{"click", "Enter", "Shift+Enter"} {
		t.Run(name, func(t *testing.T) {
			r := ixPalette(t)
			start := r.router.Location()
			row := "Search Apple Music for “zed”"
			switch name {
			case "click":
				r.tt.MustClick(row)
			case "Enter":
				r.activate(row)
				r.tt.Key(0, ui.KeyEnter)
			default:
				r.activate(row)
				r.tt.Key(ui.Shift, ui.KeyEnter)
			}
			if got, want := r.outcome(start), "nav /search?q=zed"; got != want {
				t.Errorf("%s: %s, want %s", name, got, want)
			}
		})
	}
}

// TestInteractionPaletteSpaceIsText: Space in the palette's field is typed,
// it never runs the active entry.
func TestInteractionPaletteSpaceIsText(t *testing.T) {
	r := ixPalette(t)
	start := r.router.Location()
	r.activate("Zed Album")
	r.tt.Key(0, ui.KeySpace)
	// Nothing ran and the palette is still open: the Space went into the field.
	if got := r.outcome(start); got != "palette still open" {
		t.Errorf("Space on the active entry: %s, want only the open palette", got)
	}
}

// ixBlankTail reports whether the right end of the named row, where a trailing
// cue (a chevron, a play glyph, keycaps) would be drawn, is one flat colour.
func (r *rig) ixBlankTail(name string) bool {
	r.t.Helper()
	info, ok := r.tt.Access(name)
	if !ok {
		r.t.Fatalf("no row %q; texts %q", name, r.tt.Texts())
	}
	img := r.tt.Image()
	scale := float32(img.Bounds().Dx()) / 1132
	b := info.Bounds
	cy := b.Y + b.H/2
	x0, x1 := int((b.X+b.W-48)*scale), int((b.X+b.W-14)*scale)
	y0, y1 := int((cy-8)*scale), int((cy+8)*scale)
	first := img.At(x0, y0)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if img.At(x, y) != first {
				return false
			}
		}
	}
	return true
}

// TestInteractionPaletteEntriesHaveNoTrailingCue: no library entry draws a
// chevron or a play glyph at its end, active or not (commands draw their keys
// there), so the row does not say whether choosing it opens or plays.
func TestInteractionPaletteEntriesHaveNoTrailingCue(t *testing.T) {
	r := ixPalette(t)
	for _, row := range []string{"Zed Artist", "Zed Album", "Zed Playlist", "Quartz Lantern", "Search Apple Music for “zed”"} {
		r.activate(row)
		// DEPARTS: the song entry plays and the artist, album and playlist entries open, yet every row ends blank: no chevron, no play glyph, on the active row or off it.
		if !r.ixBlankTail(row) {
			t.Errorf("%s draws something at its end", row)
		}
	}
	// The check can tell: a command row draws its keys there.
	r.tt.Key(0, ui.KeyBackspace)
	r.tt.Key(0, ui.KeyBackspace)
	r.tt.Key(0, ui.KeyBackspace)
	r.tt.Type("next")
	r.tt.Until("the command", r.tt.Shows("Next track"))
	if r.ixBlankTail("Next track") {
		t.Error("the tail check cannot tell a row with keys from a blank one")
	}
}

// TestInteractionPaletteEnterBeforeArrowingRunsTheAppleMusicSearch: the entry
// that is active when the library's hits arrive is the one that was active
// before them, the Apple Music search, so Enter right after typing runs it
// and not the library hit shown above it.
func TestInteractionPaletteEnterBeforeArrowingRunsTheAppleMusicSearch(t *testing.T) {
	r := ixPalette(t)
	start := r.router.Location()
	// DEPARTS: the highlight stays on the last entry (the web search) when the library's hits arrive above it, so Enter does not open the first hit.
	if info, ok := r.tt.Access("Search Apple Music for “zed”"); !ok || !info.Selected {
		t.Fatal("the Apple Music search is not the active entry")
	}
	r.tt.Key(0, ui.KeyEnter)
	if got, want := r.outcome(start), "nav /search?q=zed"; got != want {
		t.Errorf("Enter after typing: %s, want %s", got, want)
	}
}
