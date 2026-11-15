package search

import (
	"context"
	"crypto/sha1"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// Interaction review (#311): what a click, Enter, Shift+Enter, the Play
// control and a double-click do to each kind of result on the search
// surfaces, pinned as today's code behaves. A "DEPARTS" comment marks an
// assertion where that departs from the rule: choosing a result opens it,
// Play plays, a track list's row plays, and a modifier never does what no
// visible cue hints at.

// ixRec records what the shared players and the browser were asked to do.
type ixRec struct {
	mu sync.Mutex
	ev []string
}

func (r *ixRec) add(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ev = append(r.ev, s)
}

func (r *ixRec) events() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.ev)
}

// ixAPI is a synthetic SearchService: it answers from what the test sets and
// remembers what the feature writes.
type ixAPI struct {
	mu      sync.Mutex
	hints   services.SearchHints
	lib     services.LibrarySearch
	cat     services.SearchResults
	recent  []services.RecentSearch
	written []string // "term:scope:text", "item:kind:id", "forget:key", "clear"
}

func (a *ixAPI) Catalog(context.Context, string) (services.SearchResults, error) { return a.cat, nil }
func (a *ixAPI) Group(_ context.Context, _, group string, _ int) (services.SearchGroupPage, error) {
	for _, s := range a.cat.Sections {
		if s.Group == group {
			return services.SearchGroupPage{Group: group, Items: s.Items}, nil
		}
	}
	return services.SearchGroupPage{Group: group}, nil
}
func (a *ixAPI) Hints(context.Context, string) (services.SearchHints, error) { return a.hints, nil }
func (a *ixAPI) Browse(context.Context) ([]services.Item, error)             { return nil, nil }
func (a *ixAPI) Library(context.Context, string, int) (services.LibrarySearch, error) {
	return a.lib, nil
}
func (a *ixAPI) Profile(context.Context, string) (services.ProfileDetail, error) {
	return services.ProfileDetail{}, nil
}
func (a *ixAPI) Recent(context.Context) ([]services.RecentSearch, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.recent), nil
}
func (a *ixAPI) write(s string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.written = append(a.written, s)
}
func (a *ixAPI) RememberTerm(_ context.Context, term, scope string) error {
	a.write("term:" + scope + ":" + term)
	return nil
}
func (a *ixAPI) RememberItem(_ context.Context, it services.RecentItem) error {
	a.write("item:" + it.Kind + ":" + it.ID)
	return nil
}
func (a *ixAPI) ForgetRecent(_ context.Context, key string) error {
	a.write("forget:" + key)
	return nil
}
func (a *ixAPI) ClearRecent(context.Context) error {
	a.write("clear")
	return nil
}
func (a *ixAPI) writes() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	w := slices.Clone(a.written)
	sort.Strings(w)
	return w
}

// Synthetic results, one of every kind a search lists. Names are unique so a
// click finds its row.
func ixSong(id, name string, src services.Source, catalogID string) services.Item {
	s := &services.Song{ID: id, Source: src, CatalogID: catalogID, Name: name, Artist: "Ann", Playable: true}
	return services.Item{Kind: services.ItemSong, ID: id, Name: name, Subtitle: "Ann", Song: s}
}

func ixAlbum(id, name string) services.Item {
	return services.Item{Kind: services.ItemAlbum, ID: id, Name: name, Subtitle: "Ann",
		Album: &services.Album{ID: id, Source: services.SourceCatalog, Name: name, Artist: "Ann"}}
}

func ixArtist(id, name string) services.Item {
	return services.Item{Kind: services.ItemArtist, ID: id, Name: name,
		Artist: &services.Artist{ID: id, Source: services.SourceCatalog, Name: name}}
}

func ixPlaylist(id, name string) services.Item {
	return services.Item{Kind: services.ItemPlaylist, ID: id, Name: name, Subtitle: "Ann",
		Playlist: &services.Playlist{ID: id, Source: services.SourceCatalog, Name: name, Curator: "Ann"}}
}

func ixStation(id, name string, page bool) services.Item {
	return services.Item{Kind: services.ItemStation, ID: id, Name: name, Subtitle: "Radio Co",
		Station: &services.Station{ID: id, Name: name, Kind: services.StationLive, Playable: true, HasPage: page}}
}

func ixVideo(id, name string) services.Item {
	return services.Item{Kind: services.ItemMusicVideo, ID: id, Name: name, Subtitle: "Ann",
		MusicVideo: &services.MusicVideo{ID: id, Source: services.SourceCatalog, Name: name, Artist: "Ann"}}
}

func ixCurator(id, name string) services.Item {
	return services.Item{Kind: services.ItemCurator, ID: id, Name: name}
}

func ixLink(id, name string) services.Item {
	return services.Item{Kind: services.ItemLink, ID: id, Name: name, URL: "https://example.test/" + id}
}

// ixItems are the catalogue kinds the suggestions and the results list.
func ixItems() map[string]services.Item {
	return map[string]services.Item{
		"song":          ixSong("s1", "Song Alpha", services.SourceCatalog, ""),
		"album":         ixAlbum("a1", "Album Beta"),
		"artist":        ixArtist("ar1", "Artist Gamma"),
		"playlist":      ixPlaylist("pl.1", "Playlist Delta"),
		"station":       ixStation("ra.1", "Station Epsilon", true),
		"station-bare":  ixStation("ra.2", "Radio Zeta", false),
		"music-video":   ixVideo("mv1", "Video Eta"),
		"curator":       ixCurator("cu1", "Curator Theta"),
		"link":          ixLink("web1", "Web Iota"),
		"song-upload":   ixSong("u1", "Upload Kappa", services.SourceLibrary, ""),
		"song-libcat":   ixSong("i.2", "Library Lambda", services.SourceLibrary, "c.2"),
		"station-music": ixStation("ra.3", "Station Mu", true),
	}
}

// ixRegisterPlayers registers how the shared players play, as the discover
// and radio modules do (songs, albums, playlists, stations, music videos; no
// artist player), recording into rec.
func ixRegisterPlayers(env *module.Env, rec *ixRec) {
	for _, kind := range []string{services.ItemSong, services.ItemAlbum, services.ItemPlaylist, services.ItemStation} {
		env.Players.Register(kind, func(it services.Item) { rec.add("item " + it.Kind + ":" + it.ID) })
	}
	env.Players.RegisterVideo(func(v services.MusicVideo, _ *ui.Handle) { rec.add("video " + v.ID) })
	env.Players.Capabilities = func() *services.Capabilities {
		return &services.Capabilities{Playback: services.PlaybackCapabilities{Available: true, Video: true}}
	}
}

// ixAllRoutes is every page a result can open.
func ixAllRoutes(p string) bool {
	for _, pre := range []string{"/album/", "/artist/", "/playlist/", "/song/", "/station/", "/music-video/", "/curator/", "/search", "/profile/"} {
		if strings.HasPrefix(p, pre) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------
// The top bar's suggestion list.
// ---------------------------------------------------------------------

// ixField is the top bar's search field over the synthetic service, in a
// window tall enough for the whole suggestion list.
type ixField struct {
	t      *testing.T
	h      *uitest.Harness
	router *ui.Router
	s      *search
	api    *ixAPI
	play   *fieldPlay
	rec    *ixRec
	tt     *uitest.Session
	// start is where the test began; a change is what a gesture navigated to.
	start string
}

func newIxField(t *testing.T, start string, api *ixAPI) *ixField {
	t.Helper()
	x := &ixField{t: t, api: api, rec: &ixRec{}, play: &fieldPlay{}, start: start}
	x.h = uitest.New(t, uitest.Albums(fieldAlbums...))
	x.router = ui.NewRouter(start)
	x.h.Env.Nav = x.router
	x.h.Env.HasRoute = ixAllRoutes
	x.h.Env.Shortcuts = func() []module.BoundShortcut {
		return []module.BoundShortcut{{Shortcut: module.Shortcut{ID: "search.focus"}, Keys: []string{"/"}}}
	}
	useQueue(x.h.Env, x.play)
	ixRegisterPlayers(x.h.Env, x.rec)
	x.s = newSearch(x.h.Env, api)
	x.tt = x.h.Tester(t, func(c *ui.Context) {
		c.DisableAnimations() // a frozen test clock would redraw 20 frames per input
		ui.Column(c).Fill().Children(func() {
			ui.Row(c).Height(48).FillWidth().Padding(0, 16).AlignItems(ui.Center).Children(func() {
				ui.Row(c).MinWidth(240).Basis(520).Shrink(1).Grow(0).Children(func() { x.s.field.build(c) })
			})
			ui.Box(c).Grow(1)
		})
	}, 900, 1200)
	return x
}

// focusField clicks into the field.
func (x *ixField) focusField() {
	x.t.Helper()
	if err := x.tt.Click("Search"); err != nil {
		x.t.Fatal(err)
	}
}

// outcome is what the gestures so far did, in one line: where they
// navigated, what they played or opened in the browser, what they remembered.
func (x *ixField) outcome() string {
	x.h.Main.Drain()
	var parts []string
	if loc := x.router.Location(); loc != x.start {
		parts = append(parts, "nav "+loc)
	}
	for _, refs := range x.play.played() {
		ids := make([]string, len(refs))
		for i, r := range refs {
			ids[i] = r.ID
		}
		parts = append(parts, "queue "+strings.Join(ids, ","))
	}
	parts = append(parts, x.rec.events()...)
	for _, u := range x.h.Opened() {
		parts = append(parts, "url "+u)
	}
	for _, w := range x.api.writes() {
		parts = append(parts, "wrote "+w)
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, "; ")
}

// settled is the outcome once it has stopped changing (the writes and plays
// run on goroutines): at once when want is what it is, else after a pause.
func (x *ixField) settled(want string) string {
	got := x.outcome()
	for deadline := time.Now().Add(1500 * time.Millisecond); got != want && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
		x.tt.Frame()
		got = x.outcome()
	}
	if want == "nothing" && got == want {
		time.Sleep(60 * time.Millisecond) // what would have run late
		x.tt.Frame()
		got = x.outcome()
	}
	return got
}

// activate moves the list's active row onto the option named name, by the
// keys.
func (x *ixField) activate(name string) {
	x.t.Helper()
	for range 20 {
		if info, ok := x.tt.Access(name); ok && info.Selected {
			return
		}
		x.tt.Key(0, ui.KeyDown)
	}
	x.t.Fatalf("could not make %q the active row; texts %q", name, x.tt.Texts())
}

// gesture is one way to choose a suggestion.
type gesture struct {
	name string
	do   func(x *ixField, row string)
}

var ixGestures = []gesture{
	{"click", func(x *ixField, row string) {
		x.activate(row) // scrolls a row below the fold into view; the click is what chooses
		if err := x.tt.Click(row); err != nil {
			x.t.Fatal(err)
		}
	}},
	{"Enter", func(x *ixField, row string) { x.activate(row); x.tt.Key(0, ui.KeyEnter) }},
	{"Shift+Enter", func(x *ixField, row string) { x.activate(row); x.tt.Key(ui.Shift, ui.KeyEnter) }},
}

// ixHints is Apple's suggestions: two completions then one item of every kind
// a search finds.
func ixHints() services.SearchHints {
	it := ixItems()
	h := services.SearchHints{Terms: []services.SearchTerm{{Term: "blue", Display: "blue"}, {Term: "blue band", Display: "blue band"}}}
	for _, k := range []string{"song", "album", "artist", "playlist", "station", "station-bare", "music-video", "curator", "link"} {
		h.Items = append(h.Items, it[k])
	}
	return h
}

// ixTyped opens the suggestion list for "blu" (or "blue" in the library).
func (x *ixField) ixTyped(show string) {
	x.t.Helper()
	x.focusField()
	x.tt.Type("blu")
	x.tt.Advance(hintDelay + time.Millisecond)
	x.tt.Until("the suggestions", x.tt.Shows(show))
}

// TestInteractionFieldSuggestionRowsPerKind: what choosing each kind of
// suggestion does by click, Enter and Shift+Enter.
func TestInteractionFieldSuggestionRowsPerKind(t *testing.T) {
	cases := []struct {
		row  string
		kind string
		// click, enter, shiftEnter are the outcomes.
		click, enter, shiftEnter string
	}{
		{row: "blue band", kind: "completion",
			click:      "nav /search?q=blue+band; wrote term:catalog:blue band",
			enter:      "nav /search?q=blue+band; wrote term:catalog:blue band",
			shiftEnter: "nav /search?q=blue+band; wrote term:catalog:blue band"},
		{row: "Song Alpha", kind: "song",
			click: "nav /song/s1; wrote item:song:s1",
			enter: "nav /song/s1; wrote item:song:s1",
			// DEPARTS: Shift+Enter plays a song (and does not open it) though the active row's cue is the open chevron and nothing on screen mentions Shift.
			shiftEnter: "queue s1; wrote item:song:s1"},
		{row: "Album Beta", kind: "album",
			click:      "nav /album/a1; wrote item:album:a1",
			enter:      "nav /album/a1; wrote item:album:a1",
			shiftEnter: "nav /album/a1; wrote item:album:a1"},
		{row: "Artist Gamma", kind: "artist",
			click:      "nav /artist/ar1; wrote item:artist:ar1",
			enter:      "nav /artist/ar1; wrote item:artist:ar1",
			shiftEnter: "nav /artist/ar1; wrote item:artist:ar1"},
		{row: "Playlist Delta", kind: "playlist",
			click:      "nav /playlist/pl.1; wrote item:playlist:pl.1",
			enter:      "nav /playlist/pl.1; wrote item:playlist:pl.1",
			shiftEnter: "nav /playlist/pl.1; wrote item:playlist:pl.1"},
		{row: "Station Epsilon", kind: "station with a page",
			click:      "nav /station/ra.1; wrote item:station:ra.1",
			enter:      "nav /station/ra.1; wrote item:station:ra.1",
			shiftEnter: "nav /station/ra.1; wrote item:station:ra.1"},
		{row: "Radio Zeta", kind: "station without a page",
			// DEPARTS: choosing a page-less station plays it, but the row's cue is the open chevron (only a song with no page gets the play glyph).
			click: "item station:ra.2; wrote item:station:ra.2",
			// DEPARTS: same as the click: Enter plays the station under an open chevron.
			enter: "item station:ra.2; wrote item:station:ra.2",
			// DEPARTS: same: Shift+Enter plays the station under an open chevron.
			shiftEnter: "item station:ra.2; wrote item:station:ra.2"},
		{row: "Video Eta", kind: "music video",
			click:      "nav /music-video/mv1; wrote item:music-video:mv1",
			enter:      "nav /music-video/mv1; wrote item:music-video:mv1",
			shiftEnter: "nav /music-video/mv1; wrote item:music-video:mv1"},
		{row: "Curator Theta", kind: "curator",
			click:      "nav /curator/cu1; wrote item:curator:cu1",
			enter:      "nav /curator/cu1; wrote item:curator:cu1",
			shiftEnter: "nav /curator/cu1; wrote item:curator:cu1"},
		{row: "Web Iota", kind: "link",
			// DEPARTS: a web-only link suggestion opens the browser, under the same open chevron as an in-app page.
			click: "url https://example.test/web1; wrote item:link:web1",
			// DEPARTS: as the click.
			enter: "url https://example.test/web1; wrote item:link:web1",
			// DEPARTS: as the click.
			shiftEnter: "url https://example.test/web1; wrote item:link:web1"},
	}
	for _, tc := range cases {
		for i, g := range ixGestures {
			want := []string{tc.click, tc.enter, tc.shiftEnter}[i]
			t.Run(tc.kind+"/"+g.name, func(t *testing.T) {
				x := newIxField(t, "/library/albums", &ixAPI{hints: ixHints()})
				x.ixTyped("Top results")
				g.do(x, tc.row)
				if got := x.settled(want); got != want {
					t.Errorf("%s on the %s suggestion: %s, want %s", g.name, tc.kind, got, want)
				}
			})
		}
	}
}

// TestInteractionFieldSongWithoutAPagePlays: a song suggestion whose page the
// router lacks plays when chosen, and the active row's cue is the play
// triangle then (a library upload has no page at all).
func TestInteractionFieldSongWithoutAPagePlays(t *testing.T) {
	for _, g := range ixGestures {
		t.Run("catalogue song, no song page/"+g.name, func(t *testing.T) {
			x := newIxField(t, "/library/albums", &ixAPI{hints: ixHints()})
			x.h.Env.HasRoute = func(p string) bool { return ixAllRoutes(p) && !strings.HasPrefix(p, "/song/") }
			x.ixTyped("Top results")
			g.do(x, "Song Alpha")
			// A click, Enter and Shift+Enter all play it: the cue (play) says so.
			want := "queue s1; wrote item:song:s1"
			if got := x.settled(want); got != want {
				t.Errorf("%s: %s, want %s", g.name, got, want)
			}
		})
	}
	lib := &ixAPI{lib: services.LibrarySearch{Songs: []services.Song{*ixItems()["song-upload"].Song, *ixItems()["song-libcat"].Song}}}
	for _, g := range ixGestures {
		t.Run("library upload/"+g.name, func(t *testing.T) {
			x := newIxField(t, "/search?scope=library", lib)
			x.ixTyped("In your library")
			g.do(x, kit.UploadedLabel("Upload Kappa")) // an upload's row says so (#398)
			// Played; a library song is never remembered.
			want := "queue u1"
			if got := x.settled(want); got != want {
				t.Errorf("%s: %s, want %s", g.name, got, want)
			}
		})
		t.Run("library song with a catalogue copy/"+g.name, func(t *testing.T) {
			x := newIxField(t, "/search?scope=library", lib)
			x.ixTyped("In your library")
			g.do(x, "Library Lambda")
			// Opens its catalogue copy's page; Shift+Enter plays it.
			want := "nav /song/c.2"
			if g.name == "Shift+Enter" {
				// DEPARTS: Shift+Enter plays the library song (no cue for Shift); Enter and a click open the page.
				want = "queue i.2"
			}
			if got := x.settled(want); got != want {
				t.Errorf("%s: %s, want %s", g.name, got, want)
			}
		})
	}
}

// TestInteractionFieldLibraryScopeKinds: the library scope's suggestions of
// the other kinds open their pages by every gesture.
func TestInteractionFieldLibraryScopeKinds(t *testing.T) {
	lib := &ixAPI{lib: services.LibrarySearch{
		Artists:   []services.Artist{{ID: "l.ar", Source: services.SourceLibrary, Name: "Lib Artist"}},
		Albums:    []services.Album{{ID: "l.al", Source: services.SourceLibrary, Name: "Lib Album", Artist: "Ann"}},
		Playlists: []services.Playlist{{ID: "p.pl", Source: services.SourceLibrary, Name: "Lib List", Curator: "Ann"}},
	}}
	cases := []struct{ row, path string }{
		{"Lib Artist", "/artist/l.ar"}, {"Lib Album", "/album/l.al"}, {"Lib List", "/playlist/p.pl"},
	}
	for _, tc := range cases {
		for _, g := range ixGestures {
			t.Run(tc.row+"/"+g.name, func(t *testing.T) {
				x := newIxField(t, "/search?scope=library", lib)
				x.ixTyped("In your library")
				g.do(x, tc.row)
				want := "nav " + tc.path
				if got := x.settled(want); !strings.HasPrefix(got, want) || strings.Contains(got, "queue") {
					t.Errorf("%s on %s: %s, want to open %s and play nothing", g.name, tc.row, got, tc.path)
				}
			})
		}
	}
}

// ixGlyph crops the right end of the named row, where its trailing glyph is
// drawn, and hashes it: two rows with the same glyph on the same tone hash
// alike.
func (x *ixField) ixGlyph(name string) [20]byte {
	x.t.Helper()
	info, ok := x.tt.Access(name)
	if !ok {
		x.t.Fatalf("no row %q; texts %q", name, x.tt.Texts())
	}
	img := x.tt.Image()
	scale := float32(img.Bounds().Dx()) / 900
	b := info.Bounds
	cy := b.Y + b.H/2
	x0, x1 := int((b.X+b.W-34)*scale), int((b.X+b.W-4)*scale)
	y0, y1 := int((cy-9)*scale), int((cy+9)*scale)
	h := sha1.New()
	for y := y0; y < y1; y++ {
		for px := x0; px < x1; px++ {
			r, g, bl, a := img.At(px, y).RGBA()
			_, _ = fmt.Fprintf(h, "%d,%d,%d,%d;", r, g, bl, a)
		}
	}
	var sum [20]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

// TestInteractionFieldActiveRowCueSaysWhatEnterDoes: the glyph at the end of
// the active row is the chevron (opens) or the play triangle (plays); the
// kinds that play are told apart from the ones that open.
func TestInteractionFieldActiveRowCueSaysWhatEnterDoes(t *testing.T) {
	// The references: the active completion shows the chevron; the active
	// library upload shows the play triangle.
	ref := newIxField(t, "/library/albums", &ixAPI{hints: ixHints()})
	ref.ixTyped("Top results")
	ref.activate("blue band")
	chevron := ref.ixGlyph("blue band")

	lib := newIxField(t, "/search?scope=library", &ixAPI{lib: services.LibrarySearch{Songs: []services.Song{*ixItems()["song-upload"].Song}}})
	lib.ixTyped("In your library")
	lib.activate(kit.UploadedLabel("Upload Kappa"))
	play := lib.ixGlyph(kit.UploadedLabel("Upload Kappa"))
	if chevron == play {
		t.Fatal("the chevron and the play triangle draw alike: the cue cannot be told")
	}

	cases := []struct {
		row  string
		want string // "chevron" or "play"
	}{
		{"Song Alpha", "chevron"},
		{"Album Beta", "chevron"},
		{"Artist Gamma", "chevron"},
		{"Playlist Delta", "chevron"},
		{"Station Epsilon", "chevron"},
		// DEPARTS: Radio Zeta has no page and plays when chosen, yet its active cue is the open chevron.
		{"Radio Zeta", "chevron"},
		{"Video Eta", "chevron"},
		{"Curator Theta", "chevron"},
		// DEPARTS: Web Iota opens the browser when chosen, yet its active cue is the in-app open chevron.
		{"Web Iota", "chevron"},
	}
	for _, tc := range cases {
		t.Run(tc.row, func(t *testing.T) {
			x := newIxField(t, "/library/albums", &ixAPI{hints: ixHints()})
			x.ixTyped("Top results")
			x.activate(tc.row)
			got := "other"
			switch x.ixGlyph(tc.row) {
			case chevron:
				got = "chevron"
			case play:
				got = "play"
			}
			if got != tc.want {
				t.Errorf("%s: the active row's cue is %s, want %s", tc.row, got, tc.want)
			}
		})
	}
	t.Run("a song with no page", func(t *testing.T) {
		x := newIxField(t, "/library/albums", &ixAPI{hints: ixHints()})
		x.h.Env.HasRoute = func(p string) bool { return ixAllRoutes(p) && !strings.HasPrefix(p, "/song/") }
		x.ixTyped("Top results")
		x.activate("Song Alpha")
		if got := x.ixGlyph("Song Alpha"); got != play {
			t.Errorf("a song with no page: the active cue is not the play triangle")
		}
	})
}

// TestInteractionFieldRightClickOnASuggestionDoesNotPick: a right click on a
// suggestion is not a choice.
func TestInteractionFieldRightClickOnASuggestionDoesNotPick(t *testing.T) {
	x := newIxField(t, "/library/albums", &ixAPI{hints: ixHints()})
	x.ixTyped("Top results")
	if err := x.tt.RightClick("Album Beta"); err != nil {
		t.Fatal(err)
	}
	if got := x.settled("nothing"); got != "nothing" {
		t.Errorf("a right click on a suggestion: %s, want nothing", got)
	}
}

// TestInteractionFieldSpaceTypesAndDoesNotChoose: the field handles no Space
// of its own: it is text.
func TestInteractionFieldSpaceTypesAndDoesNotChoose(t *testing.T) {
	x := newIxField(t, "/library/albums", &ixAPI{hints: ixHints()})
	x.ixTyped("Top results")
	x.activate("Album Beta")
	x.tt.Key(0, ui.KeySpace)
	if got := x.settled("nothing"); got != "nothing" {
		t.Errorf("Space on the active row: %s, want nothing", got)
	}
}

// ixRecent is the field's list with nothing typed: the remembered entries.
func ixRecent() []services.RecentSearch {
	return []services.RecentSearch{
		{Key: "t1", Kind: services.RecentTerm, Scope: scopeCatalog, Term: "alpha term"},
		{Key: "i1", Kind: services.ItemAlbum, ItemID: "a1", Source: services.SourceCatalog, Name: "Recent Album", Subtitle: "Ann"},
		{Key: "i2", Kind: services.ItemSong, ItemID: "s1", Source: services.SourceCatalog, Name: "Recent Song", Subtitle: "Ann"},
		{Key: "i3", Kind: services.ItemSong, ItemID: "u1", Source: services.SourceLibrary, Name: "Recent Upload", Subtitle: "Ann"},
		{Key: "i4", Kind: services.ItemStation, ItemID: "ra.2", Source: services.SourceCatalog, Name: "Recent Radio", Subtitle: "Radio Co"},
		{Key: "i5", Kind: services.ItemArtist, ItemID: "ar1", Source: services.SourceCatalog, Name: "Recent Artist"},
	}
}

// TestInteractionFieldRecentRows: the "Recently searched" rows of the empty
// field, by click, Enter and Shift+Enter.
func TestInteractionFieldRecentRows(t *testing.T) {
	cases := []struct {
		row                      string
		click, enter, shiftEnter string
	}{
		{row: "alpha term",
			click:      "nav /search?q=alpha+term; wrote term:catalog:alpha term",
			enter:      "nav /search?q=alpha+term; wrote term:catalog:alpha term",
			shiftEnter: "nav /search?q=alpha+term; wrote term:catalog:alpha term"},
		{row: "Recent Album",
			click:      "nav /album/a1; wrote item:album:a1",
			enter:      "nav /album/a1; wrote item:album:a1",
			shiftEnter: "nav /album/a1; wrote item:album:a1"},
		{row: "Recent Song",
			click: "nav /song/s1; wrote item:song:s1",
			enter: "nav /song/s1; wrote item:song:s1",
			// DEPARTS: Shift+Enter plays a song suggestion but only opens a recent song: the same kind and cue behave differently in the same list.
			shiftEnter: "nav /song/s1; wrote item:song:s1"},
		{row: "Recent Upload",
			// DEPARTS: a recent library song shows the open chevron but choosing it does nothing at all (it has no page and is not played).
			click: "nothing",
			// DEPARTS: as the click: Enter on the active row does nothing.
			enter: "nothing",
			// DEPARTS: as the click: Shift+Enter does nothing.
			shiftEnter: "nothing"},
		{row: "Recent Radio",
			// DEPARTS: a page-less station plays when chosen as a suggestion but, remembered, opens /station/ra.2 (the recent entry keeps no station, so it assumes a page).
			click: "nav /station/ra.2; wrote item:station:ra.2",
			enter: "nav /station/ra.2; wrote item:station:ra.2",
			// as the click
			shiftEnter: "nav /station/ra.2; wrote item:station:ra.2"},
		{row: "Recent Artist",
			click:      "nav /artist/ar1; wrote item:artist:ar1",
			enter:      "nav /artist/ar1; wrote item:artist:ar1",
			shiftEnter: "nav /artist/ar1; wrote item:artist:ar1"},
	}
	for _, tc := range cases {
		for i, g := range ixGestures {
			want := []string{tc.click, tc.enter, tc.shiftEnter}[i]
			t.Run(tc.row+"/"+g.name, func(t *testing.T) {
				x := newIxField(t, "/library/albums", &ixAPI{recent: ixRecent()})
				x.focusField()
				x.tt.Until("the recent entries", x.tt.Shows("Recently searched", tc.row))
				g.do(x, tc.row)
				if got := x.settled(want); got != want {
					t.Errorf("%s on %q: %s, want %s", g.name, tc.row, got, want)
				}
			})
		}
	}
}

// TestInteractionFieldRecentCue: a recent library song's active cue is the
// chevron though it opens nothing.
func TestInteractionFieldRecentCue(t *testing.T) {
	ref := newIxField(t, "/library/albums", &ixAPI{recent: ixRecent()})
	ref.focusField()
	ref.tt.Until("the recent entries", ref.tt.Shows("Recently searched", "Recent Album"))
	ref.activate("Recent Album")
	chevron := ref.ixGlyph("Recent Album")
	ref.activate("Recent Upload")
	if got := ref.ixGlyph("Recent Upload"); got != chevron {
		// DEPARTS: (see TestInteractionFieldRecentRows) the cue would be play or nothing if it were honest; today it is the same chevron as an album.
		t.Errorf("the recent upload's cue differs from an album's chevron")
	}
}

// ---------------------------------------------------------------------
// The results page.
// ---------------------------------------------------------------------

// ixPage is the results page over the synthetic service.
type ixPage struct {
	t     *testing.T
	r     *pageRig
	api   *ixAPI
	rec   *ixRec
	start string
	// the counts of what had happened when the last gesture began
	nq, ne, nu, nw int
}

func newIxPage(t *testing.T, start string, api *ixAPI, routes func(string) bool) *ixPage {
	t.Helper()
	if routes == nil {
		routes = ixAllRoutes
	}
	x := &ixPage{t: t, api: api, rec: &ixRec{}, start: start}
	x.r = newPageRig(t, start, rigOpts{routes: routes, signedIn: true, wrap: func(searchAPI) searchAPI { return api }})
	ixRegisterPlayers(x.r.h.Env, x.rec)
	return x
}

// show waits for texts.
func (x *ixPage) show(texts ...string) {
	x.t.Helper()
	x.r.tt.Until("the page", x.r.tt.Shows(texts...))
}

// mark starts counting from now: what a gesture did is what comes after.
func (x *ixPage) mark() {
	x.r.h.Main.Drain()
	x.r.play.mu.Lock()
	x.nq = len(x.r.play.refs)
	x.r.play.mu.Unlock()
	x.ne = len(x.rec.events())
	x.nu = len(x.r.h.Opened())
	x.api.mu.Lock()
	x.nw = len(x.api.written)
	x.api.mu.Unlock()
}

// back returns to the page after a gesture left it, and marks.
func (x *ixPage) back() {
	x.t.Helper()
	if x.r.loc() != x.start {
		x.r.router.Replace(x.start)
		x.r.tt.Frame()
	}
	x.mark()
}

// outcome is what happened since the mark, in one line.
func (x *ixPage) outcome() string {
	x.r.h.Main.Drain()
	var parts []string
	if loc := x.r.loc(); loc != x.start {
		parts = append(parts, "nav "+loc)
	}
	x.r.play.mu.Lock()
	for i := x.nq; i < len(x.r.play.refs); i++ {
		ids := make([]string, len(x.r.play.refs[i]))
		for j, ref := range x.r.play.refs[i] {
			ids[j] = ref.ID
		}
		parts = append(parts, fmt.Sprintf("queue %s@%d", strings.Join(ids, ","), x.r.play.start[i]))
	}
	x.r.play.mu.Unlock()
	parts = append(parts, x.rec.events()[x.ne:]...)
	for _, u := range x.r.h.Opened()[x.nu:] {
		parts = append(parts, "url "+u)
	}
	x.api.mu.Lock()
	w := slices.Clone(x.api.written[x.nw:])
	x.api.mu.Unlock()
	sort.Strings(w)
	for _, s := range w {
		parts = append(parts, "wrote "+s)
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, "; ")
}

// settled is the outcome once it has stopped changing.
func (x *ixPage) settled(want string) string {
	got := x.outcome()
	for deadline := time.Now().Add(1500 * time.Millisecond); got != want && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
		x.r.tt.Frame()
		got = x.outcome()
	}
	if want == "nothing" && got == want {
		time.Sleep(60 * time.Millisecond)
		x.r.tt.Frame()
		got = x.outcome()
	}
	return got
}

func (x *ixPage) check(what, want string) {
	x.t.Helper()
	if got := x.settled(want); got != want {
		x.t.Errorf("%s: %s, want %s", what, got, want)
	}
}

// focus Tabs to the control named text (the focused control itself: Tester's
// Focused also holds for a page whose scroll area has the focus).
func (x *ixPage) focus(text string) {
	x.t.Helper()
	for range 150 {
		for _, a := range x.r.tt.AccessAll() {
			if a.Focused && strings.Contains(a.Name, text) {
				return
			}
		}
		x.r.tt.Key(0, ui.KeyTab)
	}
	x.t.Fatalf("Tab never reached %q; texts %q", text, x.r.tt.Texts())
}

// role is what the control that opens the item named name is: "link",
// "button", or "none" where there is no control (the row is plain text).
func (x *ixPage) role(name string) string {
	x.t.Helper()
	for _, a := range x.r.tt.AccessAll() {
		if !strings.Contains(a.Name, name) || strings.HasPrefix(a.Name, "Play ") {
			continue
		}
		switch a.Role {
		case ui.RoleLink:
			return "link"
		case ui.RoleButton:
			return "button"
		}
	}
	return "none"
}

// roleExact is role for a control named exactly label (a tile's opener).
func (x *ixPage) roleExact(label string) string {
	x.t.Helper()
	for _, a := range x.r.tt.AccessAll() {
		if a.Name != label {
			continue
		}
		switch a.Role {
		case ui.RoleLink:
			return "link"
		case ui.RoleButton:
			return "button"
		}
	}
	return "none"
}

// hasControl reports whether a control is named label.
func (x *ixPage) hasControl(label string) bool {
	_, ok := x.r.tt.Access(label)
	return ok
}

func ixCatalog(secs ...services.SearchSection) services.SearchResults {
	return services.SearchResults{Term: "blue", Sections: secs}
}

func ixTop(items ...services.Item) services.SearchSection {
	return services.SearchSection{Group: "top", Items: items}
}

// ixFiller is the best match when the item under test is a row beside it.
func ixFiller() services.Item { return ixArtist("fill", "Filler Artist") }

// ixKinds are the kinds under test; routes nil: every page is there.
var ixKinds = []struct {
	name   string
	key    string
	routes func(string) bool
}{
	{name: "song", key: "song"},
	{name: "song with no song page", key: "song", routes: func(p string) bool { return ixAllRoutes(p) && !strings.HasPrefix(p, "/song/") }},
	{name: "album", key: "album"},
	{name: "artist", key: "artist"},
	{name: "playlist", key: "playlist"},
	{name: "station with a page", key: "station"},
	{name: "station without a page", key: "station-bare"},
	{name: "music video", key: "music-video"},
	{name: "curator", key: "curator"},
	{name: "link", key: "link"},
}

// ixCell is the expected outcomes of the gestures on one kind.
type ixCell struct {
	role                            string
	click, enter, shiftEnter, space string
	play                            string // the Play control's outcome; "none": there is none
}

const ixT = "term:catalog:blue"

// ixOpen is an outcome that opens path and remembers the term and the item.
func ixOpen(path, item string) string {
	return "nav " + path + "; wrote item:" + item + "; wrote " + ixT
}

// ixPlay is an outcome that plays through ev and remembers the term and the item.
func ixPlay(ev, item string) string { return ev + "; wrote item:" + item + "; wrote " + ixT }

// ixGestureRun runs one gesture on a fresh page.
func ixGestureRun(t *testing.T, mk func() *ixPage, ready string, what string, do func(x *ixPage), want string) {
	t.Helper()
	x := mk()
	x.show(ready)
	x.mark()
	do(x)
	x.check(what, want)
}

// ixRunCell runs every gesture of c on the control named name, in a page mk builds.
func ixRunCell(t *testing.T, mk func() *ixPage, name string, c ixCell) {
	t.Helper()
	x := mk()
	x.show(name)
	if got := x.role(name); got != c.role {
		t.Errorf("the open control of %s is a %s, want %s", name, got, c.role)
	}
	run := func(what string, do func(x *ixPage), want string) { ixGestureRun(t, mk, name, what, do, want) }
	run("click", func(x *ixPage) { x.r.clickText(name) }, c.click)
	if c.role != "none" { // nothing to focus: the row is plain text
		run("Enter", func(x *ixPage) { x.focus(name); x.r.tt.Key(0, ui.KeyEnter) }, c.enter)
		run("Shift+Enter", func(x *ixPage) { x.focus(name); x.r.tt.Key(ui.Shift, ui.KeyEnter) }, c.shiftEnter)
		run("Space", func(x *ixPage) { x.focus(name); x.r.tt.Key(0, ui.KeySpace) }, c.space)
	}
	run("Play control", func(x *ixPage) {
		if x.hasControl("Play " + name) {
			x.r.click("Play " + name)
		}
	}, c.play)
}

// TestInteractionResultsBestMatchPerKind: the best match panel of each kind
// (the first top result), by click, Enter, Shift+Enter, Space and its Play
// disc.
func TestInteractionResultsBestMatchPerKind(t *testing.T) {
	for _, k := range ixKinds {
		item := ixItems()[k.key]
		t.Run(k.name, func(t *testing.T) {
			ixRunCell(t, func() *ixPage {
				return newIxPage(t, "/search?q=blue", &ixAPI{cat: ixCatalog(ixTop(item))}, k.routes)
			}, item.Name, ixBestWant[k.name])
		})
	}
}

// ixBestWant pins the best match panel: a link (or, where it plays as a whole,
// a button) that opens the page, and a Play disc beside it where the kind plays.
var ixBestWant = map[string]ixCell{
	"song": {role: "link",
		click: ixOpen("/song/s1", "song:s1"), enter: ixOpen("/song/s1", "song:s1"),
		shiftEnter: "nothing", space: "nothing", // a link answers Enter only
		play: ixPlay("queue s1@0", "song:s1")},
	"song with no song page": {role: "button",
		click: ixPlay("queue s1@0", "song:s1"), enter: ixPlay("queue s1@0", "song:s1"),
		shiftEnter: "nothing", space: ixPlay("queue s1@0", "song:s1"),
		play: "nothing"}, // no control of its own: the disc is drawn into the panel, which is the control
	"album": {role: "link",
		click: ixOpen("/album/a1", "album:a1"), enter: ixOpen("/album/a1", "album:a1"), shiftEnter: "nothing", space: "nothing",
		play: ixPlay("item album:a1", "album:a1")},
	"artist": {role: "link",
		click: ixOpen("/artist/ar1", "artist:ar1"), enter: ixOpen("/artist/ar1", "artist:ar1"), shiftEnter: "nothing", space: "nothing",
		// DEPARTS: an artist has no Play disc though an album, a playlist, a station and a video do (no artist player is registered).
		play: "nothing"},
	"playlist": {role: "link",
		click: ixOpen("/playlist/pl.1", "playlist:pl.1"), enter: ixOpen("/playlist/pl.1", "playlist:pl.1"), shiftEnter: "nothing", space: "nothing",
		play: ixPlay("item playlist:pl.1", "playlist:pl.1")},
	"station with a page": {role: "link",
		click: ixOpen("/station/ra.1", "station:ra.1"), enter: ixOpen("/station/ra.1", "station:ra.1"), shiftEnter: "nothing", space: "nothing",
		play: ixPlay("item station:ra.1", "station:ra.1")},
	"station without a page": {role: "button",
		click: ixPlay("item station:ra.2", "station:ra.2"), enter: ixPlay("item station:ra.2", "station:ra.2"),
		shiftEnter: "nothing", space: ixPlay("item station:ra.2", "station:ra.2"),
		play: "nothing"}, // the disc is drawn into the panel, which plays as a whole
	"music video": {role: "link",
		click: ixOpen("/music-video/mv1", "music-video:mv1"), enter: ixOpen("/music-video/mv1", "music-video:mv1"), shiftEnter: "nothing", space: "nothing",
		play: ixPlay("video mv1", "music-video:mv1")},
	"curator": {role: "link",
		click: ixOpen("/curator/cu1", "curator:cu1"), enter: ixOpen("/curator/cu1", "curator:cu1"), shiftEnter: "nothing", space: "nothing",
		// DEPARTS: a curator has no Play disc (no curator player exists), like an artist.
		play: "nothing"},
	// DEPARTS: a web-only link as the best match is not interactive at all: a click does nothing, though the same item opens the browser from the field's suggestions and from a shelf tile.
	"link": {role: "none", click: "nothing", play: "nothing"},
}

// TestInteractionResultsCompactRowsPerKind: the compact rows beside the best
// match, by the same gestures and their Play button.
func TestInteractionResultsCompactRowsPerKind(t *testing.T) {
	for _, k := range ixKinds {
		item := ixItems()[k.key]
		t.Run(k.name, func(t *testing.T) {
			ixRunCell(t, func() *ixPage {
				return newIxPage(t, "/search?q=blue", &ixAPI{cat: ixCatalog(ixTop(ixFiller(), item))}, k.routes)
			}, item.Name, ixRowWant[k.name])
		})
	}
}

// ixRowWant pins the compact rows: the same open control, and a Play button
// for a song only.
var ixRowWant = map[string]ixCell{
	"song": {role: "link",
		click: ixOpen("/song/s1", "song:s1"), enter: ixOpen("/song/s1", "song:s1"), shiftEnter: "nothing", space: "nothing",
		play: ixPlay("queue s1@0", "song:s1")},
	"song with no song page": {role: "button",
		click: ixPlay("queue s1@0", "song:s1"), enter: ixPlay("queue s1@0", "song:s1"),
		shiftEnter: "nothing", space: ixPlay("queue s1@0", "song:s1"),
		play: "nothing"},
	"album": {role: "link",
		click: ixOpen("/album/a1", "album:a1"), enter: ixOpen("/album/a1", "album:a1"), shiftEnter: "nothing", space: "nothing",
		// DEPARTS: the album's best-match panel and its shelf tile have a Play control; its compact row has none.
		play: "nothing"},
	"artist": {role: "link",
		click: ixOpen("/artist/ar1", "artist:ar1"), enter: ixOpen("/artist/ar1", "artist:ar1"), shiftEnter: "nothing", space: "nothing",
		play: "nothing"},
	"playlist": {role: "link",
		click: ixOpen("/playlist/pl.1", "playlist:pl.1"), enter: ixOpen("/playlist/pl.1", "playlist:pl.1"), shiftEnter: "nothing", space: "nothing",
		// DEPARTS: as the album: the playlist's best-match panel and shelf tile have Play, its row has none.
		play: "nothing"},
	"station with a page": {role: "link",
		click: ixOpen("/station/ra.1", "station:ra.1"), enter: ixOpen("/station/ra.1", "station:ra.1"), shiftEnter: "nothing", space: "nothing",
		// DEPARTS: as the album: the station's best-match panel and shelf tile have Play, its row has none.
		play: "nothing"},
	"station without a page": {role: "button",
		click: ixPlay("item station:ra.2", "station:ra.2"), enter: ixPlay("item station:ra.2", "station:ra.2"),
		shiftEnter: "nothing", space: ixPlay("item station:ra.2", "station:ra.2"),
		play: "nothing"},
	"music video": {role: "link",
		click: ixOpen("/music-video/mv1", "music-video:mv1"), enter: ixOpen("/music-video/mv1", "music-video:mv1"), shiftEnter: "nothing", space: "nothing",
		// DEPARTS: as the album: the video's best-match panel and shelf tile have Play, its row has none.
		play: "nothing"},
	"curator": {role: "link",
		click: ixOpen("/curator/cu1", "curator:cu1"), enter: ixOpen("/curator/cu1", "curator:cu1"), shiftEnter: "nothing", space: "nothing",
		play: "nothing"},
	// DEPARTS: a web-only link row is not interactive: a click does nothing (the field and a tile open the browser).
	"link": {role: "none", click: "nothing", play: "nothing"},
}

// ixShelfCatalog is one result group per kind, one item each (a station with a
// page, a station without one), as the shelves of covers.
func ixShelfCatalog() services.SearchResults {
	it := ixItems()
	one := func(group, key string) services.SearchSection {
		return services.SearchSection{Group: group, Items: []services.Item{it[key]}}
	}
	return ixCatalog(
		one("artist", "artist"), one("album", "album"), one("playlist", "playlist"),
		one("station", "station"), one("radio_show", "station-bare"), one("music_video", "music-video"),
		one("curator", "curator"), one("web", "link"),
	)
}

// ixURL is an outcome that opens the browser on a link item's page and remembers.
func ixURL(item string) string {
	return "url https://example.test/" + item[strings.Index(item, ":")+1:] + "; wrote item:" + item + "; wrote " + ixT
}

// ixTileWant pins the shared cover tiles (shelves and "See all" grids): the
// tile is a link (a button where it plays when opened) and its Play control
// sits beside it. The tile's Play control remembers nothing.
var ixTileWant = map[string]ixCell{
	"artist": {role: "link",
		click: ixOpen("/artist/ar1", "artist:ar1"), enter: ixOpen("/artist/ar1", "artist:ar1"), shiftEnter: "nothing", space: "nothing",
		// DEPARTS: an artist tile has no Play control though album, playlist, station and video tiles do (no artist player is registered).
		play: "nothing"},
	"album": {role: "link",
		click: ixOpen("/album/a1", "album:a1"), enter: ixOpen("/album/a1", "album:a1"), shiftEnter: "nothing", space: "nothing",
		// DEPARTS: the tile's Play plays without remembering the term or the album, the best match's disc remembers both.
		play: "item album:a1"},
	"playlist": {role: "link",
		click: ixOpen("/playlist/pl.1", "playlist:pl.1"), enter: ixOpen("/playlist/pl.1", "playlist:pl.1"), shiftEnter: "nothing", space: "nothing",
		play: "item playlist:pl.1"},
	"station": {role: "link",
		click: ixOpen("/station/ra.1", "station:ra.1"), enter: ixOpen("/station/ra.1", "station:ra.1"), shiftEnter: "nothing", space: "nothing",
		play: "item station:ra.1"},
	"station-bare": {role: "button",
		click: ixPlay("item station:ra.2", "station:ra.2"), enter: ixPlay("item station:ra.2", "station:ra.2"),
		shiftEnter: "nothing", space: ixPlay("item station:ra.2", "station:ra.2"),
		play: "nothing"}, // the play mark is drawn into the tile, which plays as a whole
	"music-video": {role: "link",
		click: ixOpen("/music-video/mv1", "music-video:mv1"), enter: ixOpen("/music-video/mv1", "music-video:mv1"), shiftEnter: "nothing", space: "nothing",
		play: "video mv1"},
	"curator": {role: "link",
		click: ixOpen("/curator/cu1", "curator:cu1"), enter: ixOpen("/curator/cu1", "curator:cu1"), shiftEnter: "nothing", space: "nothing",
		// DEPARTS: a curator tile has no Play control (no curator player exists), like an artist's.
		play: "nothing"},
	"link": {role: "link",
		click: ixURL("link:web1"), enter: ixURL("link:web1"), shiftEnter: "nothing", space: "nothing",
		play: "nothing"},
}

// ixTileRun runs the gestures on the tile of each kind in a page that shows
// them, going back to the page between gestures.
func ixTileRun(t *testing.T, x *ixPage) {
	t.Helper()
	for _, k := range []string{"artist", "album", "playlist", "station", "station-bare", "music-video", "curator", "link"} {
		item := ixItems()[k]
		c := ixTileWant[k]
		label := item.Name
		if k == "station-bare" {
			label += " Play" // a tile that plays when opened says so in its name
		}
		t.Run(k, func(t *testing.T) {
			x.t = t
			x.show(item.Name)
			if got := x.roleExact(label); got != c.role {
				t.Errorf("the %s tile is a %s, want %s", k, got, c.role)
			}
			do := func(what string, f func(), want string) {
				x.back()
				x.show(item.Name)
				x.mark()
				f()
				x.check(what, want)
			}
			do("click", func() {
				if err := x.r.tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}, c.click)
			do("Enter", func() { x.focus(item.Name); x.r.tt.Key(0, ui.KeyEnter) }, c.enter)
			do("Shift+Enter", func() { x.focus(item.Name); x.r.tt.Key(ui.Shift, ui.KeyEnter) }, c.shiftEnter)
			do("Space", func() { x.focus(item.Name); x.r.tt.Key(0, ui.KeySpace) }, c.space)
			do("Play control", func() {
				if x.hasControl("Play " + item.Name) {
					x.r.click("Play " + item.Name)
				}
			}, c.play)
			x.back()
		})
	}
}

// TestInteractionResultsShelfTilesPerKind: the shelves of covers of the other
// groups (the shared tiles), by the same gestures and their Play control.
func TestInteractionResultsShelfTilesPerKind(t *testing.T) {
	x := newIxPage(t, "/search?q=blue", &ixAPI{cat: ixShelfCatalog()}, nil)
	x.r.tt.SetSize(1132, 2600) // every shelf in view
	ixTileRun(t, x)
}

// ixSongs are catalogue songs of distinct artists and albums.
func ixSongs() []services.Item {
	var out []services.Item
	for _, n := range []string{"One", "Two", "Three"} {
		id := "t" + strings.ToLower(n)
		s := &services.Song{ID: id, Source: services.SourceCatalog, Name: "Song " + n, Artist: "Artist " + n,
			ArtistID: "ar" + n, Album: "Album " + n, AlbumID: "al" + n, Playable: true, DurationMs: 180000}
		out = append(out, services.Item{Kind: services.ItemSong, ID: id, Name: s.Name, Subtitle: s.Artist, Song: s})
	}
	return out
}

// ixRowPoint is a place on the track row labelled row that no link covers:
// its title cell (the row's own rectangle is what Find gives for its label).
func (x *ixPage) ixRowPoint(row string) (float32, float32) {
	x.t.Helper()
	rect, ok := x.r.tt.Find(row)
	if !ok {
		x.t.Fatalf("%q is not shown; texts %q", row, x.r.tt.Texts())
	}
	return rect.X + 110, rect.Y + rect.H/2
}

// ixDouble double-clicks the row's title cell.
func (x *ixPage) ixDouble(row string) {
	x.t.Helper()
	px, py := x.ixRowPoint(row)
	x.r.tt.ClickAt(px, py)
	x.r.tt.ClickAt(px, py)
}

// ixClickRow clicks the row's title cell.
func (x *ixPage) ixClickRow(row string) {
	x.t.Helper()
	px, py := x.ixRowPoint(row)
	x.r.tt.ClickAt(px, py)
}

// ixRowGestures runs the gestures on a row of a track list in a page the
// caller built, leaving the page between them; wants are keyed by gesture,
// and artist and album are the texts of the row's cells ("": skipped).
func ixRowGestures(t *testing.T, x *ixPage, ready, row, artist, album string, wants map[string]string) {
	t.Helper()
	x.show(ready, row)
	run := func(what string, f func()) {
		x.back()
		x.show(row)
		x.mark()
		f()
		x.check(what, wants[what])
	}
	run("click", func() { x.ixClickRow(row) })
	run("double-click", func() { x.ixDouble(row) })
	run("click then Enter", func() { x.ixClickRow(row); x.r.tt.Frame(); x.r.tt.Key(0, ui.KeyEnter) })
	run("click then Shift+Enter", func() { x.ixClickRow(row); x.r.tt.Frame(); x.r.tt.Key(ui.Shift, ui.KeyEnter) })
	run("click then Space", func() { x.ixClickRow(row); x.r.tt.Frame(); x.r.tt.Key(0, ui.KeySpace) })
	run("hover Play button", func() {
		px, py := x.ixRowPoint(row)
		x.r.tt.Move(px, py)
		x.r.tt.Frame()
		x.r.click("Play " + row)
	})
	if artist != "" {
		run("artist cell", func() { x.r.clickText(artist) })
	}
	if album != "" {
		run("album cell", func() { x.r.clickText(album) })
	}
	x.back()
}

// ixSongRowWant pins a catalogue track list: a click only selects, a
// double-click, Enter and the hover Play button play the list from the row
// (and remember the term and the song), the artist and album cells open
// their pages.
var ixSongRowWant = map[string]string{
	"click":                  "nothing",
	"double-click":           ixPlayList,
	"click then Enter":       ixPlayList,
	"click then Shift+Enter": "nothing",
	"click then Space":       "nothing",
	"hover Play button":      ixPlayList,
	"artist cell":            "nav /artist/arTwo",
	"album cell":             "nav /album/alTwo",
}

const ixPlayList = "queue tone,ttwo,tthree@1; wrote item:song:ttwo; wrote term:catalog:blue"

// TestInteractionResultsSongsTrackList: the Songs track list of the catalogue
// results.
func TestInteractionResultsSongsTrackList(t *testing.T) {
	x := newIxPage(t, "/search?q=blue", &ixAPI{cat: ixCatalog(services.SearchSection{Group: "song", Items: ixSongs()})}, nil)
	ixRowGestures(t, x, "Songs", "Song Two", "Artist Two", "Album Two", ixSongRowWant)
}

// TestInteractionResultsSeeAllSongs: the "See all" page of the songs plays and
// opens as the shelf's list does.
func TestInteractionResultsSeeAllSongs(t *testing.T) {
	x := newIxPage(t, "/search/song?q=blue", &ixAPI{cat: ixCatalog(services.SearchSection{Group: "song", Items: ixSongs()})}, nil)
	ixRowGestures(t, x, "Songs", "Song Two", "Artist Two", "Album Two", ixSongRowWant)
}

// TestInteractionResultsSeeAllItems: the "See all" page of a group of mixed
// items is a grid of the same tiles as the shelves.
func TestInteractionResultsSeeAllItems(t *testing.T) {
	it := ixItems()
	var items []services.Item
	for _, k := range []string{"artist", "album", "playlist", "station", "station-bare", "music-video", "curator", "link"} {
		items = append(items, it[k])
	}
	x := newIxPage(t, "/search/mixed?q=blue", &ixAPI{cat: ixCatalog(services.SearchSection{Group: "mixed", Items: items})}, nil)
	x.r.tt.SetSize(1132, 2600) // the whole grid in view
	ixTileRun(t, x)
}

// ixLibrary is a library search that finds one of each kind and two songs.
func ixLibrary() services.LibrarySearch {
	return services.LibrarySearch{
		Songs: []services.Song{
			{ID: "i.1", Source: services.SourceLibrary, CatalogID: "c.1", Name: "Lib Song One", Artist: "Lib Singer", ArtistID: "l.ar", Album: "Lib Record", AlbumID: "l.al", Playable: true},
			{ID: "i.2", Source: services.SourceLibrary, Name: "Lib Upload Two", Artist: "Lib Singer", ArtistID: "l.ar", Album: "Lib Record", AlbumID: "l.al", Playable: true},
		},
		Artists:   []services.Artist{{ID: "l.ar", Source: services.SourceLibrary, Name: "Lib Artist"}},
		Albums:    []services.Album{{ID: "l.al", Source: services.SourceLibrary, Name: "Lib Album", Artist: "Ann"}},
		Playlists: []services.Playlist{{ID: "p.pl", Source: services.SourceLibrary, Name: "Lib List", Curator: "Ann"}},
	}
}

const ixLibPlay = "queue i.1,i.2@1; wrote term:library:blue"

// ixLibRowWant pins the library's track list: as the catalogue's, but a
// library song is never remembered, only the term.
var ixLibRowWant = map[string]string{
	"click":                  "nothing",
	"double-click":           ixLibPlay,
	"click then Enter":       ixLibPlay,
	"click then Shift+Enter": "nothing",
	"click then Space":       "nothing",
	"hover Play button":      ixLibPlay,
	"artist cell":            "nav /artist/l.ar",
	"album cell":             "nav /album/l.al",
}

// TestInteractionResultsLibrarySongs: the library scope's song list.
func TestInteractionResultsLibrarySongs(t *testing.T) {
	x := newIxPage(t, "/search?q=blue&scope=library", &ixAPI{lib: ixLibrary()}, nil)
	ixRowGestures(t, x, "Songs", kit.UploadedLabel("Lib Upload Two"), "Lib Singer", "Lib Record", ixLibRowWant)
}

// TestInteractionResultsLibraryTiles: the library scope's artist, album and
// playlist shelves are the shared tiles, with the library's term and source.
func TestInteractionResultsLibraryTiles(t *testing.T) {
	x := newIxPage(t, "/search?q=blue&scope=library", &ixAPI{lib: ixLibrary()}, nil)
	x.show("Lib Album")
	const term = "wrote term:library:blue"
	cases := []struct{ name, path, item, play string }{
		{"Lib Artist", "/artist/l.ar", "artist:l.ar", "nothing"}, // DEPARTS: no Play on the artist tile, as in the catalogue.
		{"Lib Album", "/album/l.al", "album:l.al", "item album:l.al"},
		{"Lib List", "/playlist/p.pl", "playlist:p.pl", "item playlist:p.pl"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			x.t = t
			open := "nav " + tc.path + "; wrote item:" + tc.item + "; " + term
			do := func(what string, f func(), want string) {
				x.back()
				x.show(tc.name)
				x.mark()
				f()
				x.check(what, want)
			}
			do("click", func() { x.r.click(tc.name) }, open)
			do("Enter", func() { x.focus(tc.name); x.r.tt.Key(0, ui.KeyEnter) }, open)
			do("Play control", func() {
				if x.hasControl("Play " + tc.name) {
					x.r.click("Play " + tc.name)
				}
			}, tc.play)
			x.back()
		})
	}
}

// ixRecentAll are remembered entries: terms in both scopes, items of every
// kind, a library song (no page) and a library album.
func ixRecentAll() []services.RecentSearch {
	return []services.RecentSearch{
		{Key: "t1", Kind: services.RecentTerm, Scope: scopeCatalog, Term: "alpha term"},
		{Key: "t2", Kind: services.RecentTerm, Scope: scopeLibrary, Term: "library term"},
		{Key: "i1", Kind: services.ItemAlbum, ItemID: "a1", Source: services.SourceCatalog, Name: "Recent Album", Subtitle: "Ann"},
		{Key: "i2", Kind: services.ItemSong, ItemID: "s1", Source: services.SourceCatalog, Name: "Recent Song", Subtitle: "Ann"},
		{Key: "i3", Kind: services.ItemSong, ItemID: "u1", Source: services.SourceLibrary, Name: "Recent Upload", Subtitle: "Ann"},
		{Key: "i4", Kind: services.ItemStation, ItemID: "ra.2", Source: services.SourceCatalog, Name: "Recent Radio", Subtitle: "Radio Co"},
		{Key: "i5", Kind: services.ItemArtist, ItemID: "ar1", Source: services.SourceCatalog, Name: "Recent Artist"},
		{Key: "i6", Kind: services.ItemAlbum, ItemID: "l.al", Source: services.SourceLibrary, Name: "Recent Lib Album", Subtitle: "Ann"},
	}
}

// TestInteractionResultsRecentShelf: the "Recently searched" shelf of the
// empty search page: which entries show, and what a click, Enter, Shift+Enter,
// Space and the X do to each.
func TestInteractionResultsRecentShelf(t *testing.T) {
	x := newIxPage(t, "/search", &ixAPI{recent: ixRecentAll()}, nil)
	x.show("Recently searched", "Recent Album")
	// The catalogue scope lists its own terms and the catalogue's items; the
	// library's terms and items, and every library song, are left out.
	for _, hidden := range []string{"Recent Upload", "library term", "Recent Lib Album"} {
		if x.r.tt.HasText(hidden) {
			t.Errorf("the catalogue scope's shelf shows %q", hidden)
		}
	}
	for _, shown := range []string{"alpha term", "Recent Album", "Recent Song", "Recent Radio", "Recent Artist"} {
		if !x.r.tt.HasText(shown) {
			t.Errorf("the catalogue scope's shelf lacks %q", shown)
		}
	}
	wants := map[string]string{
		"alpha term/click":   "nav /search?q=alpha+term; wrote term:catalog:alpha term",
		"alpha term/X":       "wrote forget:t1",
		"Recent Album/click": "nav /album/a1; wrote item:album:a1",
		"Recent Album/X":     "wrote forget:i1",
		"Recent Song/click":  "nav /song/s1; wrote item:song:s1",
		"Recent Song/X":      "wrote forget:i2",
		// DEPARTS: a page-less station plays when chosen from the field's suggestions, but remembered it opens /station/ra.2 (the entry keeps no station, so it assumes a page).
		"Recent Radio/click":  "nav /station/ra.2; wrote item:station:ra.2",
		"Recent Radio/X":      "wrote forget:i4",
		"Recent Artist/click": "nav /artist/ar1; wrote item:artist:ar1",
		"Recent Artist/X":     "wrote forget:i5",
	}
	for _, name := range []string{"alpha term", "Recent Album", "Recent Song", "Recent Radio", "Recent Artist"} {
		t.Run(name, func(t *testing.T) {
			x.t = t
			do := func(what string, f func()) {
				x.back()
				x.show(name)
				x.mark()
				f()
				want := wants[name+"/"+what]
				if what == "Enter" {
					want = wants[name+"/click"]
				}
				if want == "" {
					want = "nothing" // Shift+Enter and Space
				}
				x.check(what, want)
			}
			do("click", func() { x.r.clickText(name) })
			do("Enter", func() { x.focus(name); x.r.tt.Key(0, ui.KeyEnter) })
			do("Shift+Enter", func() { x.focus(name); x.r.tt.Key(ui.Shift, ui.KeyEnter) })
			do("Space", func() { x.focus(name); x.r.tt.Key(0, ui.KeySpace) })
			do("X", func() {
				rect, _ := x.r.tt.Find(name)
				x.r.tt.Move(rect.X+rect.W/2, rect.Y+rect.H/2)
				x.r.tt.Frame()
				x.r.click("Remove " + name + " from recent searches")
			})
			x.back()
		})
	}
	t.Run("Clear", func(t *testing.T) {
		x.t = t
		x.back()
		x.mark()
		x.r.click("Clear")
		x.check("Clear", "wrote clear")
	})
	t.Run("library scope", func(t *testing.T) {
		l := newIxPage(t, "/search?scope=library", &ixAPI{recent: ixRecentAll()}, nil)
		l.show("Recently searched", "library term", "Recent Lib Album")
		for _, name := range []string{"alpha term", "Recent Album", "Recent Upload"} {
			if l.r.tt.HasText(name) {
				t.Errorf("the library scope's shelf shows %q", name)
			}
		}
		l.mark()
		l.r.clickText("Recent Lib Album")
		l.check("library album click", "nav /album/l.al; wrote item:album:l.al")
	})
	t.Run("pages that are not ported", func(t *testing.T) {
		u := newIxPage(t, "/search", &ixAPI{recent: ixRecentAll()}, func(p string) bool { return p == "/search" || strings.HasPrefix(p, "/search?") })
		u.show("Recently searched", "Recent Album")
		// An entry of a page that is not ported is plain text with its X: a click does nothing.
		u.mark()
		u.r.clickText("Recent Album")
		u.check("unported album click", "nothing")
		u.mark()
		u.r.clickText("alpha term")
		u.check("term click", "nav /search?q=alpha+term; wrote term:catalog:alpha term")
		u.back()
		u.mark()
		u.r.click("Remove Recent Album from recent searches")
		u.check("unported album X", "wrote forget:i1")
	})
}
