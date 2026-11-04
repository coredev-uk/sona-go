package album

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

func (r *rig) wantTexts(texts ...string) {
	r.t.Helper()
	for _, w := range texts {
		if !r.tt.HasText(w) {
			r.t.Errorf("page lacks %q; texts %q", w, r.tt.Texts())
		}
	}
}

func (r *rig) wantNoTexts(texts ...string) {
	r.t.Helper()
	for _, w := range texts {
		if r.tt.HasText(w) {
			r.t.Errorf("page has %q; texts %q", w, r.tt.Texts())
		}
	}
}

// path is the router's place.
func (r *rig) path() string { return r.router.Path() }

func (r *rig) played() played {
	r.t.Helper()
	r.tt.Until("a play", func() bool { _, ok := r.queue.last(); return ok })
	p, _ := r.queue.last()
	return p
}

func TestCatalogueAlbumShowsItsHead(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) {
		p.Album.Explicit = true
		p.Album.Traits = services.Traits{Lossless: true}
	}))
	r.shows("Salt Flats", "Mia Vale's second album.")
	r.wantTexts(
		"Mia Vale",
		"Album · 2019 · 5 songs, 17 min · Indie Pop", // the catalogue line, then the genre
		"Explicit", "Lossless",
		"Play Salt Flats", "Shuffle", "Add", "Favourite Salt Flats", "Suggest less like Salt Flats",
		"Songs on Salt Flats", "Popular",
		// The sleeve's fine print.
		"Released", "March 8, 2019", "Genres", "Indie Pop, Pop", "Label", "Quiet Records", "℗ 2019 Mia Vale",
		// The notes follow the songs.
		"Notes", "A record about driving across the flats at dusk.",
	)
	// Popular marks the one song in popularIds, once.
	n := 0
	for _, w := range r.tt.Texts() {
		if w == "Popular" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d Popular marks, want 1", n)
	}
	if got := r.cat.calls("100"); got != 1 {
		t.Errorf("the catalogue was asked %d times", got)
	}
	if len(r.lib.called) != 0 {
		t.Errorf("a catalogue id asked the library: %q", r.lib.called)
	}
	// The lede says it once: when the notes equal it, the shelf is hidden.
}

func TestNotesEqualToTheLedeAreNotRepeated(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) { p.Notes = p.NotesShort }))
	r.shows("Salt Flats")
	r.wantNoTexts("Notes")
	r2 := newRig(t, "/album/100", 1132, 1000)
	r2.addCatalog(saltFlats(func(p *services.CatalogAlbum) { p.Notes = "" }))
	r2.shows("Salt Flats")
	r2.wantNoTexts("Notes")
}

func TestBylineLinksToTheArtistWhereThePageExists(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.addCatalog(saltFlats())
	r.shows("Salt Flats")
	r.click("Mia Vale")
	if got := r.path(); got != "/artist/a1" {
		t.Fatalf("the byline went to %q", got)
	}

	// With no artist page the name is plain text.
	u := newRig(t, "/album/100", 1132, 1000)
	u.unported("/artist")
	u.addCatalog(saltFlats())
	u.shows("Salt Flats")
	_ = u.tt.Click("Mia Vale")
	u.tt.Drain()
	if got := u.path(); got != "/album/100" {
		t.Errorf("a name with no page navigated to %q", got)
	}
}

func TestLibraryAlbumLoadsTheLibraryAndItsCataloguePage(t *testing.T) {
	r := newRig(t, "/album/l.9", 1132, 1000)
	cat := saltFlats(func(p *services.CatalogAlbum) {
		p.MoreByArtist = []services.Album{{ID: "200", Name: "Blue Hour", Artist: "Mia Vale", ReleaseDate: "2021-05-01"}}
	})
	r.addCatalog(cat)
	libTracks := fiveTracks()
	for i := range libTracks {
		libTracks[i].ID, libTracks[i].Source = fmt.Sprintf("i.%d", i), services.SourceLibrary
	}
	r.lib.albums["l.9"] = services.AlbumDetail{
		Album:  services.Album{ID: "l.9", Source: services.SourceLibrary, CatalogID: "100", Name: "Salt Flats", Artist: "Mia Vale", ArtistID: "r.1", ReleaseDate: "2019-03-08"},
		Tracks: libTracks, DurationMs: 1000000, InLibrary: true,
	}
	r.states.inLib["l.9"] = true
	r.shows("Salt Flats", "Mia Vale's second album.", "More by Mia Vale", "Blue Hour")
	if len(r.lib.called) == 0 || r.lib.called[0] != "l.9" {
		t.Errorf("library asked %q", r.lib.called)
	}
	if r.cat.calls("100") == 0 {
		t.Error("the linked catalogue page was not loaded")
	}
	// Apple's popular marks come from the linked page (by catalogue id).
	r.wantTexts("Popular", "In library", "Notes")
	r.wantNoTexts("Add")
}

func TestLibraryOnlyAlbumHasNoCatalogueExtras(t *testing.T) {
	r := newRig(t, "/album/l.9", 1132, 1000)
	r.lib.albums["l.9"] = services.AlbumDetail{
		Album:  services.Album{ID: "l.9", Source: services.SourceLibrary, Name: "Demos", Artist: "Mia Vale", ArtistID: "r.1", ReleaseDate: "2017"},
		Tracks: []services.Song{track(1, "First"), track(2, "Second")}, DurationMs: 400000,
	}
	r.shows("Demos", "First")
	r.wantNoTexts("Popular", "Notes", "Other versions", "More by Mia Vale", "You might also like")
	// No catalogue page, so the byline links to the library artist.
	r.click("Mia Vale")
	if got := r.path(); got != "/artist/r.1" {
		t.Errorf("byline to %q", got)
	}
}

func TestPartialLibraryAlbumOffersTheCompleteOne(t *testing.T) {
	r := newRig(t, "/album/l.9", 1132, 1000)
	r.addCatalog(saltFlats())
	r.lib.albums["l.9"] = services.AlbumDetail{
		Album:  services.Album{ID: "l.9", Source: services.SourceLibrary, CatalogID: "100", Name: "Salt Flats", Artist: "Mia Vale", ReleaseDate: "2019-03-08"},
		Tracks: fiveTracks()[:2], DurationMs: 400000,
	}
	r.shows("Show complete album")
	// The catalogue line counts the whole record, not what the library holds.
	r.wantTexts("Album · 2019 · 5 songs, 17 min", "2 of 5 songs in your library · ")
	r.click("Show complete album")
	if got := r.path(); got != "/album/100" {
		t.Errorf("went to %q", got)
	}
}

func TestSkeletonWhileLoading(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.cat.gate = make(chan struct{})
	r.addCatalog(saltFlats())
	r.open()
	if _, ok := r.tt.Find("Loading"); !ok {
		t.Fatalf("no loading skeleton; texts %q", r.tt.Texts())
	}
	r.wantNoTexts("Salt Flats")
	close(r.cat.gate)
	r.shows("Salt Flats")
	if _, ok := r.tt.Find("Loading"); ok {
		t.Error("the skeleton stays after the album loaded")
	}
}

func TestNotFoundAlbumSaysSoAndGoesBack(t *testing.T) {
	r := newRig(t, "/album/999", 1132, 1000)
	r.shows("This album isn't available", "Go to Albums")
	r.click("Go to Albums")
	if got := r.path(); got != "/library/albums" {
		t.Errorf("went to %q", got)
	}
}

func TestFailedLoadCanBeRetried(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.addCatalog(saltFlats())
	r.cat.fail(errors.New("boom"))
	r.shows("Couldn't load this album", "Try again")
	r.cat.fail(nil)
	r.click("Try again")
	r.shows("Salt Flats")
	if got := r.cat.calls("100"); got != 2 {
		t.Errorf("asked %d times, want 2", got)
	}
}

// A signed-out account is told so, and the button opens the account dialog.
func TestSignedOutErrorOffersSignIn(t *testing.T) {
	r := newRig(t, "/album/100", 899, 900)
	r.addCatalog(saltFlats())
	r.cat.fail(auth.ErrNotSignedIn)
	r.shows("Sign in to see this", "Apple Music needs a signed-in account to show this album.")
	if r.h.Env.Surfaces.AccountOpen() {
		t.Fatal("the account dialog is open before anyone asked")
	}
	r.click("Sign in")
	if !r.h.Env.Surfaces.AccountOpen() {
		t.Error("Sign in did not open the account dialog")
	}
}

func TestPlayButtonsPlayTheAlbum(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) {
		p.Tracks[2].Playable = false // not streamable: skipped
	}))
	r.shows("Play Salt Flats")
	r.click("Play Salt Flats")
	got := r.played()
	if want := []string{"t1", "t2", "t4", "t5"}; !slices.Equal(got.ids, want) || got.start != 0 {
		t.Errorf("played %v from %d, want %v from 0", got.ids, got.start, want)
	}
	if got.from != (services.PlayFrom{Kind: "album", ID: "100"}) {
		t.Errorf("played from %+v", got.from)
	}
}

func TestShufflePlaysFromARandomSongWithShuffleOn(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.addCatalog(saltFlats())
	r.shows("Shuffle")
	r.click("Shuffle")
	got := r.played()
	if got.start < 0 || got.start >= 5 || len(got.ids) != 5 {
		t.Errorf("played %v from %d", got.ids, got.start)
	}
	if !r.queue.shuffle {
		t.Error("shuffle was not turned on")
	}
	if !r.h.Env.Play.State().Shuffle {
		t.Error("the mirror does not show shuffle at once")
	}
}

func TestEnterOnARowPlaysFromThatSong(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) { p.Tracks[1].Playable = false }))
	r.shows("Evening Bus")
	r.click("Evening Bus")
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Drain()
	got := r.played()
	// Song 4 is the third playable one.
	if got.start != 2 || !slices.Equal(got.ids, []string{"t1", "t3", "t4", "t5"}) {
		t.Errorf("played %v from %d", got.ids, got.start)
	}
}

func TestNothingPlayableToasts(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) {
		for i := range p.Tracks {
			p.Tracks[i].Playable = false
		}
	}))
	r.shows("Salt Flats")
	// Nothing can be played: the Play disc is disabled and does nothing.
	_ = r.tt.Click("Play Salt Flats")
	r.tt.Drain()
	if _, ok := r.queue.last(); ok {
		t.Error("a disabled Play played")
	}
	// Every row says why it is greyed out.
	r.wantTexts("Unavailable")
}

func TestFailedPlaybackToasts(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.addCatalog(saltFlats())
	r.queue.err = errors.New("engine down")
	r.shows("Play Salt Flats")
	r.click("Play Salt Flats")
	r.tt.Until("the failure", r.tt.Shows("Couldn't start playback"))
}

func TestLibraryAndRatingButtonsAskTheLibrary(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.addCatalog(saltFlats())
	r.shows("Add")
	// Both buttons are disabled until the item's state has reached the page.
	r.enabled("Add")
	r.enabled("Favourite Salt Flats")
	r.click("Add")
	r.tt.Until("the add", func() bool { return len(r.states.addedIDs()) > 0 })
	if got := r.states.addedIDs(); !slices.Equal(got, []string{"100"}) {
		t.Errorf("added %q", got)
	}
	r.tt.Until("the library button to flip", r.tt.Shows("In library"))
	r.enabled("Favourite Salt Flats")
	r.click("Favourite Salt Flats")
	r.tt.Until("the favourite", func() bool { return len(r.states.favoredIDs()) > 0 })
	if got := r.states.favoredIDs(); !slices.Equal(got, []string{"100"}) {
		t.Errorf("favourited %q", got)
	}
	r.tt.Until("the button to flip", r.tt.Shows("Undo favourite for Salt Flats"))
}

func TestCompilationShowsAnArtistColumn(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) {
		p.Album.Artist = "Various Artists"
		p.Tracks[0].Artist, p.Tracks[1].Artist, p.Tracks[2].Artist = "Ana", "Bo", "Cy"
	}))
	r.shows("Salt Flats", "Ana", "Bo")
	r.wantTexts("Artist")

	// A featured artist on one song is named under its title instead.
	f := newRig(t, "/album/100", 1132, 1000)
	f.addCatalog(saltFlats(func(p *services.CatalogAlbum) { p.Tracks[0].Artist = "Mia Vale & Zed" }))
	f.shows("Salt Flats", "Mia Vale & Zed")
	f.wantNoTexts("Artist")
}

func TestUnreleasedAlbumCountsDownToTheMinute(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	release := r.now.Add(5*time.Hour + 12*time.Minute)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) {
		p.Album.Prerelease, p.Album.ReleaseAt, p.Album.ReleaseDate = true, release.UnixMilli(), release.Format("2006-01-02")
		for i := 2; i < 5; i++ { // three songs are not out yet
			p.Tracks[i].Playable = false
			p.Tracks[i].DurationMs = 0
			p.Tracks[i].Artist = "unknown" // Apple's stand-in credit
		}
	}))
	r.shows("Out in 5 hours, 12 minutes", "Pre-add")
	day := fullReleaseDate(release.UnixMilli())
	r.wantTexts(day+" · 2 of 5 songs out now", "Play the 2 songs out now", "Pre-add")
	r.wantNoTexts("Shuffle", "Released", "unknown")
	// Songs still to come say when.
	r.wantTexts(kit.ReleaseBadge(services.Album{Prerelease: true, ReleaseAt: release.UnixMilli()}, r.now))
	// The running time of unpublished songs is not guessed.
	r.wantTexts("Album · 2026 · 5 songs · Indie Pop")

	// The page re-reads the clock once a minute, never a ticking counter.
	r.now = r.now.Add(30 * time.Second)
	r.tt.Advance(30 * time.Second)
	r.tt.Drain()
	r.wantTexts("Out in 5 hours, 12 minutes")
	r.now = r.now.Add(31 * time.Second)
	r.tt.Advance(31 * time.Second)
	r.tt.Drain()
	r.tt.Until("the next minute", r.tt.Shows("Out in 5 hours, 11 minutes"))

	// At the release the page reloads the album so its songs become playable.
	before := r.cat.calls("100")
	r.now = release.Add(time.Second)
	r.tt.Advance(5*time.Hour + 12*time.Minute)
	r.tt.Until("the reload", func() bool { return r.cat.calls("100") > before })
}

func TestYearOnlyUnreleasedAlbumHasNoCountdown(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) {
		p.Album.Prerelease, p.Album.ReleaseAt, p.Album.ReleaseDate = true, 0, "2027"
		for i := range p.Tracks {
			p.Tracks[i].Playable = false
		}
	}))
	r.shows("Out 2027")
	r.wantNoTexts("Pre-add ", "Play the 0 songs out now")
	for _, w := range r.tt.Texts() {
		if strings.HasPrefix(w, "Out in") {
			t.Errorf("a countdown without a date: %q", w)
		}
	}
	// No song is out: no Play button.
	if _, ok := r.tt.Find("Play Salt Flats"); ok {
		t.Error("Play on an album with no song out")
	}
}

func TestSongSublineSaysWhyARowIsGreyed(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) {
		p.Tracks[0].Playable = false
		p.Tracks[1].Explicit = true
	}))
	r.shows("Unavailable")
	r.wantNoTexts("Explicit content is restricted on this account")
	// The account's policy arrives as the auth module's update of the cache.
	store.Set(r.h.Env.Queries, store.KeyAuth, services.AuthStatus{State: "signed_in", Restricted: true})
	r.tt.Until("the policy", r.tt.Shows("Explicit content is restricted on this account"))
}

func TestClassicalAlbumGroupsMovementsUnderWorks(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) {
		p.Album.Name = "Goldberg"
		for i := range p.Tracks {
			p.Tracks[i].Work = "Goldberg Variations, BWV 988"
			p.Tracks[i].Composer = "Johann Sebastian Bach"
			p.Tracks[i].Movement = fmt.Sprintf("Variation %d", i)
		}
		p.Tracks[4].Work, p.Tracks[4].Composer, p.Tracks[4].Movement = "Partita No. 1", "Clara Schumann", "Prelude"
	}))
	r.shows("Goldberg")
	// Several composers: the work's heading is led by its composer; a
	// movement is the row's title.
	r.wantTexts(
		"Johann Sebastian Bach · Goldberg Variations, BWV 988", "Clara Schumann · Partita No. 1",
		"Variation 0", "Variation 3", "Prelude",
		"Composers", "Johann Sebastian Bach and Clara Schumann",
	)
	r.wantNoTexts("Written by")
}

func TestWritersAreNamedWithTheirCount(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) {
		var names []string
		for i := 0; i < 12; i++ {
			names = append(names, fmt.Sprintf("W%02d", i))
		}
		p.Tracks[0].Composer = strings.Join(names, ", ")
	}))
	r.shows("Written by", "W00, W01, W02, W03, W04, W05, W06, W07, and 4 others")
}

func TestMultiDiscAlbumsIndexTheirDiscs(t *testing.T) {
	long := func(p *services.CatalogAlbum) {
		p.Tracks = nil
		for i := 1; i <= 40; i++ {
			p.Tracks = append(p.Tracks, track(i, fmt.Sprintf("Song %02d", i), func(s *services.Song) {
				s.DiscNumber, s.TrackNumber = 1+(i-1)/20, 1+(i-1)%20
			}))
		}
		p.DurationMs = 40 * 200000
	}
	r := newRig(t, "/album/100", 1132, 600)
	r.addCatalog(saltFlats(long))
	r.shows("Discs", "Disc 2 · 20 songs, 1 hr, 7 min")
	// The list carries its own headings too.
	r.wantTexts("Disc 1")
	r.tt.Settle()
	first, _ := r.tt.Find("Song 01")
	r.click("Disc 2 · 20 songs, 1 hr, 7 min")
	r.tt.Until("the second disc", func() bool { return r.a.tracks.Nav().Focus() == 20 })
	r.tt.Settle()
	if r.a.pos.Y <= 0 {
		t.Fatalf("the page did not scroll to the disc: %v", r.a.pos.Y)
	}
	// The disc's heading sits 16 DIPs from the pane's top: the page has
	// scrolled to its block's top less 16 (the heading block is the disc and
	// its 46-high heading above the row).
	o, count, at := &r.a.list.opts, r.a.list.count, r.a.list.at
	if got, want := r.a.pos.Y+16, r.a.tracks.BlockTop(o, count, at, 20); got < want-1 || got > want+1 {
		t.Errorf("scrolled to %v+16, the disc's block starts at %v", r.a.pos.Y, want)
	}
	// And on screen: row 21 sits under the heading (46 DIPs), which is 16
	// below the top; row 1 was under the first disc's heading (34 DIPs) below
	// the table's header at its content offset.
	second, _ := r.tt.Find("Song 21")
	wantY := first.Y - (r.a.tracks.BlockTop(o, count, at, 0) + 34) + 16 + 46
	if second.Y < wantY-1.5 || second.Y > wantY+1.5 {
		t.Errorf("row 21 at y=%v, want %v (heading 16 from the top)", second.Y, wantY)
	}

	// Below 860 the sleeve heads the page and has no index: the tracks
	// carry the headings.
	n := newRig(t, "/album/100", 700, 600)
	n.addCatalog(saltFlats(long))
	n.shows("Salt Flats")
	n.wantNoTexts("Discs")
	n.wantTexts("Disc 1")
}

func TestSingleDiscAlbumHasNoIndex(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.addCatalog(saltFlats())
	r.shows("Salt Flats")
	r.wantNoTexts("Discs", "Disc 1")
}

func TestSongParamMarksAndFocusesTheSong(t *testing.T) {
	r := newRig(t, "/album/100?song=c4", 1132, 1000)
	r.addCatalog(saltFlats())
	r.shows("Evening Bus")
	r.tt.Until("the focus", func() bool { return r.a.tracks.Nav().Focus() == 3 })
}

func TestLayoutFollowsTheContentNotTheWindow(t *testing.T) {
	// A wide window beside a long sleeve: the fine print is in the sleeve
	// column, left of the songs.
	wide := newRig(t, "/album/100", 1500, 1000)
	wide.addCatalog(saltFlats())
	wide.shows("Released")
	rel, _ := wide.tt.Find("Released")
	song, _ := wide.tt.Find("Low Tide")
	if rel.X >= song.X {
		t.Errorf("wide: fine print at x=%v, songs at x=%v", rel.X, song.X)
	}
	narrow := newRig(t, "/album/100", 700, 1000)
	narrow.addCatalog(saltFlats())
	narrow.shows("Released")
	rel, _ = narrow.tt.Find("Released")
	song, _ = narrow.tt.Find("Low Tide")
	if rel.Y <= song.Y {
		t.Errorf("narrow: fine print at y=%v, not below the songs at y=%v", rel.Y, song.Y)
	}
}

func TestSleeveStaysBesideTheSongsOnlyWhenItFits(t *testing.T) {
	long := func(p *services.CatalogAlbum) {
		p.Tracks = nil
		for i := 1; i <= 60; i++ {
			p.Tracks = append(p.Tracks, track(i, fmt.Sprintf("Song %02d", i)))
		}
	}
	y := func(h int) (before, after float32) {
		r := newRig(t, "/album/100", 1132, h)
		r.addCatalog(saltFlats(long))
		r.shows("Released")
		r.tt.Settle()
		b, _ := r.tt.Find("Released")
		r.tt.Scroll(600, 300, 0, 400)
		r.tt.Settle()
		a, _ := r.tt.Find("Released")
		return b.Y, a.Y
	}
	// A tall pane shows the whole sleeve: it stays where it was.
	if b, a := y(900); a != b {
		t.Errorf("a sleeve that fits moved from %v to %v", b, a)
	}
	// A short one scrolls it away with the page.
	if b, a := y(400); a >= b {
		t.Errorf("a sleeve taller than the pane stayed: %v then %v", b, a)
	}
}

func TestCatalogueSections(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 2400)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) {
		p.OtherVersions = []services.Album{{ID: "101", Name: "Salt Flats (Deluxe)", Artist: "Mia Vale", ReleaseDate: "2020-01-01"}}
		p.Videos = []services.MusicVideo{{ID: "v1", CatalogID: "v1", Name: "Low Tide (Video)", Artist: "Mia Vale", ReleaseDate: "2019-04-01"}}
		p.MoreToHear = []services.Station{{ID: "ra.1", Name: "Salt Flats, the story", ShowName: "Record Club", Playable: true, HasPage: true}}
		p.MoreByArtist = []services.Album{{ID: "200", Name: "Blue Hour", Artist: "Mia Vale", ReleaseDate: "2021-05-01"}}
		p.AppearsOn = []services.Playlist{{ID: "pl.1", Name: "Road Trip", Curator: "Apple Music Indie"}}
		p.Similar = []services.Album{{ID: "300", Name: "Dust", Artist: "Cy Moor"}}
	}))
	r.shows("Other versions", "Music videos", "More to hear", "More by Mia Vale", "Featured in playlists", "You might also like")
	r.wantTexts("Salt Flats (Deluxe)", "Low Tide (Video)", "Salt Flats, the story", "Record Club", "Blue Hour", "2021", "Road Trip", "Dust", "Cy Moor")
	// See all of the artist's albums.
	r.click("See all")
	if got := r.path(); got != "/artist/a1/view/full-albums" {
		t.Errorf("See all went to %q", got)
	}
}

func TestSectionTilesOpenPagesAndPlay(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1400)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) {
		p.Videos = []services.MusicVideo{{ID: "v1", CatalogID: "v1", Name: "Low Tide (Video)", Artist: "Mia Vale"}}
		p.MoreToHear = []services.Station{{ID: "ra.1", Name: "The story", Playable: true, HasPage: true}}
		p.MoreByArtist = []services.Album{{ID: "200", Name: "Blue Hour", Artist: "Mia Vale"}}
	}))
	r.shows("The story")
	// A radio episode plays from its tile.
	r.click("Play The story")
	r.tt.Until("the station", func() bool { return len(r.radio.got()) > 0 })
	if got := r.radio.got(); got[0] != "ra.1" {
		t.Errorf("played station %q", got)
	}
	// Where videos play, a video's tile has a play control (as discover's
	// cards do), and playing opens the video view.
	r.wantNoTexts("Play Low Tide (Video)")
	var played []string
	r.h.Env.Players.RegisterVideo(func(v services.MusicVideo, _ *ui.Handle) { played = append(played, v.ID) })
	r.h.Env.Players.Capabilities = func() *services.Capabilities {
		return &services.Capabilities{Playback: services.PlaybackCapabilities{Available: true, Video: true}}
	}
	r.tt.Until("the video's play control", r.tt.Shows("Play Low Tide (Video)"))
	r.click("Play Low Tide (Video)")
	if len(played) != 1 || played[0] != "v1" {
		t.Errorf("played videos %v", played)
	}
	// A tile opens its album.
	r.click("Blue Hour")
	if got := r.path(); got != "/album/200" {
		t.Errorf("the tile went to %q", got)
	}
}

func TestSectionsLinkOnlyToPortedPages(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1400)
	r.unported("/artist", "/station", "/playlist", "/music-video")
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) {
		p.MoreByArtist = []services.Album{{ID: "200", Name: "Blue Hour", Artist: "Mia Vale"}}
		p.AppearsOn = []services.Playlist{{ID: "pl.1", Name: "Road Trip"}}
	}))
	r.shows("More by Mia Vale", "Road Trip")
	r.wantNoTexts("See all")
	_ = r.tt.Click("Road Trip")
	r.tt.Drain()
	if got := r.path(); got != "/album/100" {
		t.Errorf("a tile to a page that is not ported went to %q", got)
	}
}

func TestGoToAlbumAction(t *testing.T) {
	a := newAlbum(newRig(t, "/album/100", 800, 600).h.Env)
	defs := a.module().Actions
	if len(defs) != 1 {
		t.Fatalf("%d actions", len(defs))
	}
	def := defs[0]
	if def.ID != "album.go-to" || def.Target != kit.KindSong || def.Group != kit.GroupItem || def.Order != 10 {
		t.Errorf("action %+v", def)
	}
	reg := kit.NewActionRegistry(def)
	var went string
	reg.SetNavigator(func(p string) { went = p })
	with := reg.Context([]kit.Item{kit.SongItem(track(1, "A"))}, nil, nil)
	without := reg.Context([]kit.Item{kit.SongItem(track(1, "A", func(s *services.Song) { s.AlbumID = "" }))}, nil, nil)
	if !def.Available(with) || def.Available(without) {
		t.Error("Go to album is available only for a song with an album")
	}
	def.Run(with)
	if went != "/album/100" {
		t.Errorf("went to %q", went)
	}
}

func TestHeaderMenuListsTheAlbumsActions(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.h.Env.Actions.Register(kit.ActionDef{
		ID: "test.album-action", Target: kit.KindAlbum, Group: kit.GroupItem, Order: 1,
		Label: func(*kit.ActionContext) string { return "Do a thing" }, Icon: "star",
		Run: func(*kit.ActionContext) {},
	})
	r.addCatalog(saltFlats())
	r.shows("More for Salt Flats")
	r.click("More for Salt Flats")
	r.tt.Until("the menu", r.tt.Shows("Do a thing"))
}

func TestLayoutFollowsWindowResizes(t *testing.T) {
	r := newRig(t, "/album/100", 1500, 1000)
	r.addCatalog(saltFlats())
	r.shows("Released")
	rel, _ := r.tt.Find("Released")
	song, _ := r.tt.Find("Low Tide")
	if rel.X >= song.X {
		t.Fatalf("wide: fine print at x=%v, songs at x=%v", rel.X, song.X)
	}
	// The sidebar or the queue taking room changes the content's width; the
	// layout follows it, not the window.
	r.tt.SetSize(700, 1000)
	r.tt.Until("the narrow layout", func() bool {
		rel, _ := r.tt.Find("Released")
		song, _ := r.tt.Find("Low Tide")
		return rel.Y > song.Y
	})
}

func TestPopularIsNotShownOnAnUnplayableSong(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) { p.Tracks[1].Playable = false }))
	r.shows("Low Tide")
	r.wantNoTexts("Popular")
}

func TestMotionAlbumStillShows(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) { p.Header.Motion = "/motion/100/1.mp4" }))
	// The motion policy is nil here (nothing moves): the cover is the still.
	r.shows("Salt Flats", "Songs on Salt Flats")
}

func TestAnotherAlbumStartsAtItsTop(t *testing.T) {
	long := func(id string) func(p *services.CatalogAlbum) {
		return func(p *services.CatalogAlbum) {
			p.Album.ID, p.Album.CatalogID, p.Album.Name = id, id, "Album "+id
			p.Tracks = nil
			for i := 1; i <= 60; i++ {
				p.Tracks = append(p.Tracks, track(i, fmt.Sprintf("Song %02d", i)))
			}
		}
	}
	r := newRig(t, "/album/100", 1132, 500)
	r.addCatalog(saltFlats(long("100")))
	r.addCatalog(saltFlats(long("101")))
	r.shows("Album 100")
	r.tt.Settle()
	r.tt.Scroll(600, 300, 0, 600)
	r.tt.Settle()
	if r.a.pos.Y <= 0 {
		t.Fatalf("the first album did not scroll: %v", r.a.pos.Y)
	}
	r.router.Push("/album/101")
	r.tt.Until("the other album", r.tt.Shows("Album 101"))
	r.tt.Settle()
	if r.a.pos.Y != 0 {
		t.Errorf("the other album starts at %v, not at its top", r.a.pos.Y)
	}
	if r.a.tracks.Nav().Focus() != 0 {
		t.Errorf("the other album's list kept the focus at %d", r.a.tracks.Nav().Focus())
	}
}

func TestUnreleasedAlbumOffersARemindWhereTheyWork(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	release := r.now.Add(48 * time.Hour)
	p := saltFlats(func(p *services.CatalogAlbum) {
		p.Album.Prerelease, p.Album.ReleaseAt, p.Album.ReleaseDate = true, release.UnixMilli(), release.Format("2006-01-02")
	})
	r.addCatalog(p)
	r.shows("Out in 2 days")
	// kit.RemindButton builds nothing on a platform without notifications.
	if kit.RemindAvailable(r.h.Env.Core, p.Album) {
		r.tt.Until("the reminder button", r.tt.Shows("Remind me"))
	} else {
		r.wantNoTexts("Remind me")
	}
	// An album that is out has none.
	o := newRig(t, "/album/100", 1132, 1000)
	o.addCatalog(saltFlats())
	o.shows("Salt Flats")
	o.wantNoTexts("Remind me")
}
