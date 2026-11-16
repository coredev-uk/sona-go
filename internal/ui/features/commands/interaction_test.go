package commands

import (
	"context"
	"hash/fnv"
	"slices"
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
	r.songPage = "/song/" + songs.Songs[0].CatalogID
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
// on a library entry of each kind. Choosing opens the page; Shift+Enter plays
// a song that has one (the foot says so); a song with no page here (an
// upload) plays instead.
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
		row, kind string
		want      func(r *rig, g string) string
	}{
		{"Zed Artist", "artist", func(*rig, string) string { return "nav /artist/l.ar1" }},
		{"Zed Album", "album", func(*rig, string) string { return "nav /album/l.al1" }},
		{"Zed Playlist", "playlist", func(*rig, string) string { return "nav /playlist/p.pl1" }},
		{"Quartz Lantern", "song", func(r *rig, g string) string {
			if g == "Shift+Enter" {
				return "queue Quartz Lantern"
			}
			return "nav " + r.songPage
		}},
		{"Quartz Lantern", "song with no page", func(*rig, string) string { return "queue Quartz Lantern" }},
	}
	for _, tc := range cases {
		for _, g := range gestures {
			t.Run(tc.kind+"/"+g.name, func(t *testing.T) {
				r := ixPalette(t)
				if tc.kind == "song with no page" {
					r.routes = slices.DeleteFunc(r.routes, func(p string) bool { return p == "/song/" })
				}
				start := r.router.Location()
				g.do(r, tc.row)
				if got, want := r.outcome(start), tc.want(r, g.name); got != want {
					t.Errorf("%s on the %s entry: %s, want %s", g.name, tc.kind, got, want)
				}
			})
		}
	}
}

// TestInteractionPaletteShiftEnterIsHinted: the foot names Shift+Enter only
// while the active entry is a song with a page, the one entry it does
// something for.
func TestInteractionPaletteShiftEnterIsHinted(t *testing.T) {
	r := ixPalette(t)
	r.activate("Zed Album")
	r.tt.Frame()
	if slices.Contains(r.tt.Texts(), "Play") {
		t.Error("the foot hints Shift+Enter on an album entry, where it does nothing more than Enter")
	}
	r.activate("Quartz Lantern")
	r.tt.Frame()
	if !slices.Contains(r.tt.Texts(), "Play") {
		t.Errorf("the foot does not name Shift+Enter on a song entry; texts %q", r.tt.Texts())
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

// TestInteractionPaletteEntriesCueWhatEnterDoes: the active library entry
// ends in a glyph that says what Enter does, a chevron where it opens a page
// and a play triangle where it plays (an upload); an inactive row ends blank.
// The pixel hashes of the three states differ, and a command row draws its
// keys there instead.
func TestInteractionPaletteEntriesCueWhatEnterDoes(t *testing.T) {
	r := ixPalette(t)
	hash := func(row string) uint64 {
		r.activate(row)
		r.tt.Frame()
		return r.ixTailHash(row)
	}
	blank := func(row string) bool {
		r.activate("Zed Artist")
		if row == "Zed Artist" {
			r.activate("Zed Album")
		}
		r.tt.Frame()
		return r.ixBlankTail(row)
	}
	opens := hash("Zed Album")
	for _, row := range []string{"Zed Artist", "Zed Playlist", "Quartz Lantern", "Search Apple Music for “zed”"} {
		if got := hash(row); got != opens {
			t.Errorf("%s: the active cue differs from the open chevron", row)
		}
	}
	// With no song page routed the song entry plays, and says so.
	r.routes = slices.DeleteFunc(r.routes, func(p string) bool { return p == "/song/" })
	r.tt.Frame()
	plays := hash("Quartz Lantern")
	if plays == opens {
		t.Error("a song with no page has the open chevron, not a play cue")
	}
	if r.ixBlankTail("Quartz Lantern") {
		t.Error("the active song entry with no page draws no cue")
	}
	for _, row := range []string{"Zed Album", "Quartz Lantern"} {
		if !blank(row) {
			t.Errorf("%s draws a cue while not active", row)
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

// ixTailHash is a hash of the pixels at the right end of the named row.
func (r *rig) ixTailHash(name string) uint64 {
	r.t.Helper()
	info, ok := r.tt.Access(name)
	if !ok {
		r.t.Fatalf("no row %q", name)
	}
	img := r.tt.Image()
	scale := float32(img.Bounds().Dx()) / 1132
	b := info.Bounds
	cy := b.Y + b.H/2
	h := fnv.New64a()
	for y := int((cy - 8) * scale); y < int((cy+8)*scale); y++ {
		for x := int((b.X + b.W - 48) * scale); x < int((b.X+b.W-14)*scale); x++ {
			cr, cg, cb, ca := img.At(x, y).RGBA()
			_, _ = h.Write([]byte{byte(cr >> 8), byte(cg >> 8), byte(cb >> 8), byte(ca >> 8)})
		}
	}
	return h.Sum64()
}

// TestInteractionPaletteEnterBeforeArrowingRunsTheFirstEntry: the first entry
// is the active one until the user moves, so Enter right after typing runs
// what the list shows first (a library hit), not the Apple Music search that
// was alone in the list before the hits arrived.
func TestInteractionPaletteEnterBeforeArrowingRunsTheFirstEntry(t *testing.T) {
	r := ixPalette(t)
	start := r.router.Location()
	if info, ok := r.tt.Access("Zed Artist"); !ok || !info.Selected {
		t.Fatal("the first library hit is not the active entry")
	}
	if info, ok := r.tt.Access("Search Apple Music for “zed”"); !ok || info.Selected {
		t.Fatal("the Apple Music search is active before the user moved")
	}
	r.tt.Key(0, ui.KeyEnter)
	if got, want := r.outcome(start), "nav /artist/l.ar1"; got != want {
		t.Errorf("Enter after typing: %s, want %s", got, want)
	}
}

// TestInteractionPaletteArrowedChoiceSurvivesNewHits: once the user has moved
// the highlight, hits arriving for the same query do not take it back.
func TestInteractionPaletteArrowedChoiceSurvivesNewHits(t *testing.T) {
	r := ixPalette(t)
	r.tt.Key(0, ui.KeyDown)
	r.settle()
	r.tt.Frame()
	if info, ok := r.tt.Access("Zed Album"); !ok || !info.Selected {
		t.Fatal("Down did not move to the second entry")
	}
}
