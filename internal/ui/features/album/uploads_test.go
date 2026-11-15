package album

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/features/artist"
	"github.com/coredev-uk/sona-go/internal/ui/features/library"
	"github.com/coredev-uk/sona-go/internal/ui/features/radio"
	songpage "github.com/coredev-uk/sona-go/internal/ui/features/song"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// libraryTrack is a catalogue song of the library: a library ID, with its
// catalogue copy's.
func libraryTrack(n int, name string) services.Song {
	return track(n, name, func(s *services.Song) {
		s.ID, s.LibraryID, s.Source, s.AlbumID = fmt.Sprintf("i.%d", n), fmt.Sprintf("i.%d", n), services.SourceLibrary, "l.7"
	})
}

// uploadTrack is a song the user uploaded: in the library, with no catalogue
// copy, no disc and no track number (its file has none).
func uploadTrack(id, name string) services.Song {
	return services.Song{
		ID: id, LibraryID: id, Source: services.SourceLibrary, Name: name, Artist: "Mia Vale", Album: "Salt Flats",
		AlbumID: "l.7", DurationMs: 180000, Playable: true,
	}
}

// addMixedAlbum puts library album l.7 in place: Apple's five songs, and two
// uploads Apple files under it, listed first as an order by disc puts a song
// with disc 0. One upload shares a name with a catalogue song.
func (r *rig) addMixedAlbum() {
	r.addCatalog(saltFlats())
	tracks := []services.Song{uploadTrack("i.up1", "Low Tide"), uploadTrack("i.up2", "Demo")}
	for n, name := range []string{"Salt Flats", "Low Tide", "Glass Houses", "Evening Bus", "Paper Moon"} {
		tracks = append(tracks, libraryTrack(n+1, name))
	}
	r.lib.albums["l.7"] = services.AlbumDetail{
		Album:  services.Album{ID: "l.7", Source: services.SourceLibrary, CatalogID: "100", Name: "Salt Flats", Artist: "Mia Vale", ArtistID: "r.1", ReleaseDate: "2019-03-08"},
		Tracks: tracks, DurationMs: 5*200000 + 2*180000, InLibrary: true,
	}
	r.states.inLib["l.7"] = true
}

// rowLabels are the labels of the album's song rows, in order.
func (r *rig) rowLabels() []string {
	r.t.Helper()
	var out []string
	for _, n := range access.Read(r.t, r.tt.Tester).Roles(ui.RoleRow) {
		if !strings.HasPrefix(n.Label, "# ") { // the table's header row
			out = append(out, n.Label)
		}
	}
	return out
}

func count(texts []string, want string) int {
	n := 0
	for _, t := range texts {
		if t == want {
			n++
		}
	}
	return n
}

func TestDiscsOfSkipsSongsWithNoDisc(t *testing.T) {
	none := func(d int64) services.Song { return song(func(s *services.Song) { s.DiscNumber, s.DurationMs = 0, d }) }
	onDisc := func(n int, d int64) services.Song {
		return song(func(s *services.Song) { s.DiscNumber, s.DurationMs = n, d })
	}

	// Every song without a disc: one disc, no index.
	if got := discsOf([]services.Song{none(1), none(2)}); got != nil {
		t.Errorf("songs with no disc are discs: %+v", got)
	}
	// An upload among the songs of one disc doesn't make a second.
	if got := discsOf([]services.Song{none(1), onDisc(1, 2), onDisc(1, 3)}); got != nil {
		t.Errorf("an upload beside one disc made an index: %+v", got)
	}
	// On two discs, an upload at the end is on neither, and no disc is "Disc 0".
	list := []services.Song{onDisc(1, 2), onDisc(1, 3), onDisc(2, 4), none(100)}
	want := []disc{{1, 0, 2, 5}, {2, 2, 1, 4}}
	if got := discsOf(list); !slices.Equal(got, want) {
		t.Errorf("discsOf = %+v, want %+v", got, want)
	}
	// Placed before the discs (Apple's order), it still opens none, and the
	// first disc starts after it.
	list = []services.Song{none(100), onDisc(1, 2), onDisc(2, 4)}
	want = []disc{{1, 1, 1, 2}, {2, 2, 1, 4}}
	if got := discsOf(list); !slices.Equal(got, want) {
		t.Errorf("discsOf = %+v, want %+v", got, want)
	}
}

func TestSongsWithNoDiscComeAfterTheLastDisc(t *testing.T) {
	named := func(name string, disc int) services.Song {
		return song(func(s *services.Song) { s.Name, s.DiscNumber = name, disc })
	}
	names := func(l []services.Song) string {
		var out []string
		for _, s := range l {
			out = append(out, s.Name)
		}
		return strings.Join(out, ",")
	}
	// An order by disc puts disc 0 first: it moves after the last disc, each
	// group in its own order.
	got := discLessLast([]services.Song{named("u1", 0), named("a", 1), named("b", 1), named("u2", 0), named("c", 2)})
	if names(got) != "a,b,c,u1,u2" {
		t.Errorf("discLessLast = %s", names(got))
	}
	// Left as they were: none without a disc, all without one, already last.
	for _, l := range [][]services.Song{
		{named("a", 1), named("b", 2)},
		{named("u1", 0), named("u2", 0)},
		{named("a", 1), named("u", 0)},
	} {
		if got := discLessLast(l); &got[0] != &l[0] {
			t.Errorf("%s was copied", names(l))
		}
	}
}

// An upload among Apple's songs is marked where it sits, and only it: the
// badge reads "Uploaded", and the row's name says "uploaded" after the title.
// No "Disc 0", and the upload follows the songs of the record. A catalogue
// song and an upload of the same name are two rows that tell themselves apart.
func TestUploadAmongCatalogueSongsIsMarked(t *testing.T) {
	r := newRig(t, "/album/l.7", 1132, 1000)
	r.addMixedAlbum()
	r.shows("Paper Moon", "Demo")

	if got := count(r.tt.Texts(), "Uploaded"); got != 2 {
		t.Errorf("%d Uploaded badges, want 2 (one per upload): %q", got, r.tt.Texts())
	}
	r.wantNoTexts("Disc 0", "Disc 1")

	want := []string{"Salt Flats", "Low Tide", "Glass Houses", "Evening Bus", "Paper Moon", "Low Tide, uploaded", "Demo, uploaded"}
	if got := r.rowLabels(); !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}
}

// The header counts the songs that play: Apple's five and the two uploads,
// and their running time.
func TestAlbumHeaderCountsUploads(t *testing.T) {
	r := newRig(t, "/album/l.7", 1132, 1000)
	r.addMixedAlbum()
	r.shows("Paper Moon")
	r.wantTexts("Album · 2019 · 7 songs, 23 min")
}

// An album of uploads only has nothing to tell apart: no badge on any row,
// and its songs are named plainly.
func TestAlbumOfOnlyUploadsHasNoBadges(t *testing.T) {
	r := newRig(t, "/album/l.8", 1132, 1000)
	r.lib.albums["l.8"] = services.AlbumDetail{
		Album: services.Album{ID: "l.8", Source: services.SourceLibrary, Name: "Home Tapes", Artist: "Mia Vale", ArtistID: "r.1"},
		Tracks: []services.Song{
			uploadTrack("i.up1", "Take One"), uploadTrack("i.up2", "Take Two"),
		}, DurationMs: 360000, InLibrary: true,
	}
	r.shows("Take One", "Take Two")
	r.wantNoTexts("Uploaded", "Disc 0")
	if got := r.rowLabels(); !slices.Equal(got, []string{"Take One", "Take Two"}) {
		t.Errorf("rows %q", got)
	}
}

// An upload on a two-disc record sits after the last disc: under no "Disc 0",
// out of the disc index.
func TestUploadOnAMultiDiscAlbumFollowsTheLastDisc(t *testing.T) {
	r := newRig(t, "/album/l.7", 1132, 1000)
	r.addCatalog(saltFlats())
	tracks := []services.Song{uploadTrack("i.up1", "Demo")}
	for n, name := range []string{"Salt Flats", "Low Tide", "Glass Houses", "Evening Bus"} {
		s := libraryTrack(n+1, name)
		s.DiscNumber, s.TrackNumber = n/2+1, n%2+1
		tracks = append(tracks, s)
	}
	r.lib.albums["l.7"] = services.AlbumDetail{
		Album:  services.Album{ID: "l.7", Source: services.SourceLibrary, CatalogID: "100", Name: "Salt Flats", Artist: "Mia Vale", ArtistID: "r.1"},
		Tracks: tracks, InLibrary: true,
	}
	r.shows("Evening Bus", "Demo", "Disc 2")
	r.wantNoTexts("Disc 0")
	if got := count(r.tt.Texts(), "Disc 1"); got != 2 { // the heading and the index
		t.Errorf("Disc 1 shown %d times: %q", got, r.tt.Texts())
	}
	if got := r.rowLabels(); got[len(got)-1] != "Demo, uploaded" {
		t.Errorf("rows %q: the upload is not last", got)
	}
}

// The menu of an upload offers none of what only a catalogue song has: no
// Copy link, no View credits, no Create station. Its catalogue neighbour has
// all three, and Go to album stays for both (an upload has a library album).
func TestUploadMenuHasNoCatalogueOnlyActions(t *testing.T) {
	r := newRig(t, "/album/l.7", 1132, 1000)
	r.addMixedAlbum()
	r.registerRealActions()
	r.shows("Paper Moon", "Demo")

	menu := func(name string) string {
		r.click("More for " + name)
		r.tt.Until("the menu", r.tt.Shows("Go to album"))
		texts := strings.Join(r.tt.Texts(), "|")
		r.tt.Key(0, ui.KeyEscape)
		r.tt.Settle()
		return texts
	}
	up := menu("Demo")
	for _, a := range []string{"Copy link", "View credits", "Create station"} {
		if strings.Contains(up, a) {
			t.Errorf("an upload's menu offers %q: %s", a, up)
		}
	}
	cat := menu("Paper Moon")
	for _, a := range []string{"Copy link", "View credits", "Create station"} {
		if !strings.Contains(cat, a) {
			t.Errorf("a catalogue song's menu lacks %q: %s", a, cat)
		}
	}
}

// registerRealActions adds the item actions the other features register, so
// a menu lists what the app's does.
func (r *rig) registerRealActions() {
	env := r.h.Env
	for _, f := range []module.Factory{library.New, songpage.New, radio.New, artist.New} {
		env.Actions.Register(f(env).Actions...)
	}
}
