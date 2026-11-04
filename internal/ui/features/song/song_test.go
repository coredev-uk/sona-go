package song

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// fakeCatalog answers SongPage from a table and counts the asks.
type fakeCatalog struct {
	mu    sync.Mutex
	pages map[string]services.CatalogSong
	err   error
	asked []string
	gate  chan struct{} // when set, an ask waits for it
}

func (f *fakeCatalog) SongPage(ctx context.Context, id string) (services.CatalogSong, error) {
	f.mu.Lock()
	f.asked = append(f.asked, id)
	gate, err, p, ok := f.gate, f.err, f.pages[id], false
	_, ok = f.pages[id]
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return services.CatalogSong{}, ctx.Err()
		}
	}
	if err != nil {
		return services.CatalogSong{}, err
	}
	if !ok {
		return services.CatalogSong{}, fmt.Errorf("song %s: %w", id, applemusic.ErrNotFound)
	}
	return p, nil
}

func (f *fakeCatalog) set(err error) {
	f.mu.Lock()
	f.err = err
	f.mu.Unlock()
}

func (f *fakeCatalog) asks() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.asked)
}

// fakeLyrics answers Lyrics, failing while err is set.
type fakeLyrics struct {
	mu    sync.Mutex
	l     services.Lyrics
	err   error
	asked int
	gate  chan struct{} // when set, an ask waits for it
}

func (f *fakeLyrics) Lyrics(ctx context.Context, id string) (services.Lyrics, error) {
	f.mu.Lock()
	f.asked++
	gate := f.gate
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return services.Lyrics{}, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return services.Lyrics{}, f.err
	}
	return f.l, nil
}

// hold makes asks wait until the returned func is called.
func (f *fakeLyrics) hold() (release func()) {
	gate := make(chan struct{})
	f.mu.Lock()
	f.gate = gate
	f.mu.Unlock()
	return func() {
		f.mu.Lock()
		f.gate = nil
		f.mu.Unlock()
		close(gate)
	}
}

func (f *fakeLyrics) fail(err error) {
	f.mu.Lock()
	f.err = err
	f.mu.Unlock()
}

func (f *fakeLyrics) asks() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.asked
}

// fakeQueue records PlayItems.
type fakeQueue struct {
	mu    sync.Mutex
	plays []play
}

type play struct {
	refs  []services.TrackRef
	start int
	from  services.PlayFrom
}

func (f *fakeQueue) PlayItems(_ context.Context, refs []services.TrackRef, start int, from services.PlayFrom) error {
	f.mu.Lock()
	f.plays = append(f.plays, play{refs, start, from})
	f.mu.Unlock()
	return nil
}

func (f *fakeQueue) UndoReplace(context.Context) (bool, error) { return false, nil }

func (f *fakeQueue) played() []play {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]play(nil), f.plays...)
}

// fakeLibrary is the LibraryService's item state calls.
type fakeLibrary struct {
	mu    sync.Mutex
	state services.ItemState
	calls []string
	fail  error
}

func (f *fakeLibrary) States(_ context.Context, refs []services.ItemRef) ([]services.ItemState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf("states %d", len(refs)))
	out := make([]services.ItemState, len(refs))
	for i, r := range refs {
		out[i] = f.state
		out[i].Kind, out[i].ID = r.Kind, r.ID
	}
	return out, nil
}

// change applies fn to the stored state and returns the result for r: the
// service's memory of what was changed.
func (f *fakeLibrary) change(r services.ItemRef, call string, fn func(*services.ItemState)) (services.ItemState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	if f.fail != nil {
		return services.ItemState{}, f.fail
	}
	fn(&f.state)
	st := f.state
	st.Kind, st.ID = r.Kind, r.ID
	return st, nil
}

func (f *fakeLibrary) Favorite(_ context.Context, r services.ItemRef, on bool) (services.ItemState, error) {
	return f.change(r, fmt.Sprintf("favorite %s %s %v", r.Kind, r.ID, on), func(s *services.ItemState) {
		s.Rating, s.InLibrary = 0, true
		if on {
			s.Rating = 1
		}
	})
}

func (f *fakeLibrary) SuggestLess(_ context.Context, r services.ItemRef, on bool) (services.ItemState, error) {
	return f.change(r, fmt.Sprintf("suggest-less %s %s %v", r.Kind, r.ID, on), func(s *services.ItemState) {
		s.Rating = 0
		if on {
			s.Rating = -1
		}
	})
}

func (f *fakeLibrary) Remove(_ context.Context, r services.ItemRef) (services.ItemState, error) {
	return f.change(r, fmt.Sprintf("remove %s %s", r.Kind, r.ID), func(s *services.ItemState) { *s = services.ItemState{} })
}

func (f *fakeLibrary) Add(_ context.Context, typ string, ids []string) error {
	_, err := f.change(services.ItemRef{}, fmt.Sprintf("add %s %v", typ, ids), func(s *services.ItemState) { s.InLibrary = true })
	return err
}

func (f *fakeLibrary) log() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeLibrary) count(prefix string) int {
	n := 0
	for _, c := range f.log() {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func (f *fakeLibrary) has(prefix string) bool {
	for _, c := range f.log() {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

// airbag is the fixture song's page.
func airbag() services.CatalogSong {
	art := func(n string) services.Art {
		return services.Art{Small: "/art/" + n + "/s.jpg", Medium: "/art/" + n + "/m.jpg", Large: "/art/" + n + "/l.jpg"}
	}
	return services.CatalogSong{
		Song: services.Song{
			ID: "1440", Source: services.SourceCatalog, CatalogID: "1440", Name: "Airbag", Artist: "Radiohead",
			Album: "OK Computer", AlbumID: "al1", ArtistID: "a1", DurationMs: 284000, ReleaseDate: "1997-05-21",
			Genre: "Alternative", Explicit: true, Playable: true, Traits: services.Traits{Lossless: true}, Art: art("airbag"),
		},
		Header:  services.Header{Colors: []string{"#336699"}},
		Album:   &services.Album{ID: "al1", Source: services.SourceCatalog, Name: "OK Computer", Artist: "Radiohead"},
		Artists: []services.Artist{{ID: "a1", Name: "Radiohead"}},
		// Apple's excerpt: a verse break in it.
		LyricsExcerpt: "In the next world war\nIn a jackknifed juggernaut\n\nI am born again",
		Composer:      "Thom Yorke & Jonny Greenwood",
		ISRC:          "GBAYE9700001",
		Credits: []services.CreditGroup{
			{Kind: "performer", Title: "PERFORMING ARTISTS", Credits: []services.Credit{
				{Name: "Radiohead", Roles: []string{"Performer", "Vocals"}},
				{Name: "Nigel Godrich", Roles: []string{"Producer"}, Art: art("nigel")},
				{Name: "Third Person", Roles: []string{"Guitar"}},
			}},
			{Kind: "other", Title: "MIXING & MASTERING", Credits: []services.Credit{
				{Name: "Chris Blair", Roles: []string{"Mastering Engineer"}},
			}},
		},
		Videos: []services.MusicVideo{{
			ID: "v1", CatalogID: "v1", Name: "Airbag (Live)", Artist: "Radiohead", ReleaseDate: "2001-06-01", Art: art("video"),
		}},
		MoreByArtist: []services.Album{
			{ID: "al2", Source: services.SourceCatalog, Name: "Kid A", Artist: "Radiohead", ReleaseDate: "2000-10-02", Art: art("kida")},
			{ID: "al3", Source: services.SourceCatalog, Name: "Amnesiac", Artist: "Radiohead", ReleaseDate: "2001-06-05", Art: art("amnesiac")},
		},
	}
}

// bare is airbag without lyrics, credits, videos and albums.
func bare() services.CatalogSong {
	p := airbag()
	p.LyricsExcerpt, p.Credits, p.Videos, p.MoreByArtist = "", nil, nil, nil
	p.Composer, p.ISRC, p.Album = "", "", nil
	p.Song.ReleaseDate = ""
	return p
}

type rig struct {
	t       *testing.T
	h       *uitest.Harness
	tt      *uitest.Session
	router  *ui.Router
	s       *song
	catalog *fakeCatalog
	lyrics  *fakeLyrics
	queue   *fakeQueue
	library *fakeLibrary
	routes  map[string]bool
	rows    *kit.ActionMenus // the menu of a song row on another page
}

const (
	pageSong = "/song/{id}"
)

func newRig(t *testing.T, w, h int, pages ...services.CatalogSong) *rig {
	t.Helper()
	return newRigWith(t, w, h, nil, pages...)
}

// newRigWith is newRig with setup run before the first frame (what a fake
// does from the first ask on).
func newRigWith(t *testing.T, w, h int, setup func(*rig), pages ...services.CatalogSong) *rig {
	t.Helper()
	return newRigOpts(t, w, h, setup, nil, pages...)
}

func newRigOpts(t *testing.T, w, h int, setup func(*rig), opts []uitest.Option, pages ...services.CatalogSong) *rig {
	t.Helper()
	hn := uitest.New(t, append([]uitest.Option{uitest.Empty()}, opts...)...)
	r := &rig{
		t: t, h: hn, router: ui.NewRouter("/song/1440"),
		catalog: &fakeCatalog{pages: map[string]services.CatalogSong{}},
		lyrics: &fakeLyrics{l: services.Lyrics{
			Songwriters: []string{"Thom Yorke", "Jonny Greenwood"},
			Lines: []services.LyricLine{
				{Text: "In the next world war"}, {Text: "In a jackknifed juggernaut"}, {Text: ""},
				{Text: "I am born again"}, {Text: "In a blaze of light"}, {Text: "In an interstellar burst"},
			},
		}},
		queue:   &fakeQueue{},
		library: &fakeLibrary{state: services.ItemState{InLibrary: false}},
		routes: map[string]bool{
			"/album/": true, "/artist/": true, "/music-video/": true, "/library/songs": true,
		},
	}
	for _, p := range pages {
		r.catalog.pages[p.Song.ID] = p
	}
	env := hn.Env
	env.Nav = r.router
	env.HasRoute = func(path string) bool {
		for p := range r.routes {
			if strings.HasPrefix(path, p) && r.routes[p] {
				return true
			}
		}
		return false
	}
	env.Actions = kit.NewActionRegistry(viewCredits)
	env.Actions.SetNavigator(r.router.Push)
	r.rows = kit.NewActionMenus(env.Actions, nil)
	r.s = newSong(env, r.catalog, r.lyrics,
		&queueCommands{svc: r.queue, play: env.Play, post: env.Post},
		kit.NewItemStates(env.Queries, env.Post, r.library))
	kit.SetToastPoster(env.Post)
	if setup != nil {
		setup(r)
	}
	r.tt = hn.Tester(t, func(c *ui.Context) {
		c.DisableAnimations()
		ui.Column(c).Fill().Children(func() {
			r.router.View(c, func(rt *ui.Route) {
				switch {
				case rt.Match(pageSong):
					r.s.songPage(c, rt)
				case rt.Match("/album/{id}"):
					ui.Text(c, "ALBUM PAGE "+rt.Param("id")+" song="+rt.Query("song"))
					// A row of the album's songs, with its menu.
					r.rows.Trigger(c, kit.ActionTarget{Item: kit.SongItem(services.Song{ID: "1441", CatalogID: "1441", Name: "Paranoid Android"}), Name: "Paranoid Android"},
						kit.TriggerOpts{Header: true})
					r.rows.Popup(c)
				case rt.Match("/artist/{id}/view/{view}"):
					ui.Text(c, "ARTIST VIEW "+rt.Param("id")+" "+rt.Param("view"))
				case rt.Match("/artist/{id}"):
					ui.Text(c, "ARTIST PAGE "+rt.Param("id"))
				case rt.Match("/library/songs"):
					ui.Text(c, "SONGS PAGE")
				case rt.Match("/music-video/{id}"):
					ui.Text(c, "VIDEO PAGE "+rt.Param("id"))
				case rt.Match("/song/{id}"):
				}
			})
			kit.ConfirmHost(c)
			kit.Toaster(c)
		})
	}, w, h)
	t.Cleanup(func() { kit.CloseToast(""); kit.CancelConfirm() })
	return r
}

// open waits for the Airbag page.
func (r *rig) open() {
	r.t.Helper()
	r.tt.Until("the song", r.tt.Shows("Airbag", "Radiohead"))
}

func (r *rig) click(label string) {
	r.t.Helper()
	if err := r.tt.Click(label); err != nil {
		r.t.Fatalf("click %q: %v", label, err)
	}
}

func (r *rig) route() string { return r.router.Location() }

func TestSongPageShowsTheHeader(t *testing.T) {
	r := newRig(t, 1132, 1000, airbag())
	r.open()
	for _, want := range []string{
		"Airbag", "Radiohead", "OK Computer", "Song · 1997 · 4:44 · Alternative", "Play Airbag",
		"Lyrics", "Credits", "Released May 21, 1997", "Composed by Thom Yorke & Jonny Greenwood", "ISRC GBAYE9700001",
	} {
		if !r.tt.HasText(want) {
			t.Errorf("%q is not on the page; texts %q", want, r.tt.Texts())
		}
	}
	// Explicit and the audio trait show as badges.
	if !r.tt.HasText("Lossless") {
		t.Errorf("no Lossless badge: %q", r.tt.Texts())
	}
	// The song is asked for by the route's ID.
	if r.catalog.asks() != 1 {
		t.Errorf("asked %d times", r.catalog.asks())
	}
}

func TestArtistAndAlbumLinksNavigate(t *testing.T) {
	r := newRig(t, 1132, 1000, airbag())
	r.open()
	r.click("OK Computer")
	r.tt.Until("the album page", r.tt.Shows("ALBUM PAGE al1 song=1440"))
	r.router.Back()
	r.open()
	r.click("Radiohead")
	r.tt.Until("the artist page", r.tt.Shows("ARTIST PAGE a1"))
}

func TestLinksToUnportedPagesAreText(t *testing.T) {
	r := newRig(t, 1132, 1000, airbag())
	r.routes = map[string]bool{}
	r.open()
	_ = r.tt.Click("OK Computer")
	_ = r.tt.Click("Radiohead")
	r.tt.Drain()
	if got := r.route(); got != "/song/1440" {
		t.Errorf("navigated to %q with no album or artist route", got)
	}
	// And the shelves that only link to those pages are not drawn.
	for _, no := range []string{"Music videos", "More by Radiohead"} {
		if r.tt.HasText(no) {
			t.Errorf("%q shows with no page to open", no)
		}
	}
}

func TestSongWithoutArtistsNamesThePlainArtist(t *testing.T) {
	p := airbag()
	p.Artists, p.Album = nil, nil
	r := newRig(t, 1132, 1000, p)
	r.open()
	if r.tt.HasText("OK Computer") && strings.Contains(strings.Join(r.tt.Texts(), "|"), " · OK Computer") {
		t.Errorf("an album link shows for a song with no album")
	}
}

func TestPlayPlaysTheSongFromItsOwnID(t *testing.T) {
	r := newRig(t, 1132, 1000, airbag())
	r.open()
	r.click("Play Airbag")
	r.tt.Until("the queue to start", func() bool { return len(r.queue.played()) > 0 })
	got := r.queue.played()[0]
	if len(got.refs) != 1 || got.refs[0] != (services.TrackRef{ID: "1440", Source: services.SourceCatalog}) || got.start != 0 {
		t.Errorf("played %+v", got)
	}
	if got.from != (services.PlayFrom{Kind: "", ID: "1440"}) {
		t.Errorf("from %+v, want the song's ID as the label", got.from)
	}
}

func TestPlayIsDisabledForAnUnplayableSong(t *testing.T) {
	p := airbag()
	p.Song.Playable = false
	r := newRig(t, 1132, 1000, p)
	r.open()
	_ = r.tt.Click("Play Airbag")
	r.tt.Drain()
	if n := len(r.queue.played()); n != 0 {
		t.Errorf("an unplayable song played %d times", n)
	}
}

func TestLibraryAndRatingButtons(t *testing.T) {
	r := newRig(t, 1132, 1000, airbag())
	r.open()
	r.tt.Until("the library state", func() bool { return r.library.has("states 1") })
	r.tt.Drain()
	r.click("Add")
	r.tt.Until("the add call", func() bool { return r.library.has("add songs [1440]") })
	// Adding refreshes the state: let that answer land before the next change.
	r.tt.Until("the refreshed state", func() bool { return r.library.count("states") == 2 })
	r.tt.Until("In library", r.tt.Shows("In library"))
	r.click("Favourite Airbag")
	r.tt.Until("the favourite call", func() bool { return r.library.has("favorite songs 1440 true") })
	// The answer replaces the optimistic state: the button now offers Undo.
	r.tt.Until("the undo button", r.tt.Shows("Undo favourite for Airbag"))
	r.click("Suggest less like Airbag")
	r.tt.Until("the suggest-less call", func() bool { return r.library.has("suggest-less songs 1440 true") })
}

func TestInLibraryAsksBeforeRemoving(t *testing.T) {
	r := newRig(t, 1132, 1000, airbag())
	r.library.state = services.ItemState{InLibrary: true, LibraryID: "i.1"}
	r.open()
	r.tt.Until("In library", r.tt.Shows("In library"))
	r.click("In library")
	r.tt.Until("the question", r.tt.Shows("Delete “Airbag” from your library?"))
	if r.library.has("remove") {
		t.Fatal("removed without asking")
	}
	r.click("Delete")
	r.tt.Until("the removal", func() bool { return r.library.has("remove songs 1440") })
	r.tt.Until("the Add button back", r.tt.Shows("Add"))
}

func TestARatingThatFailsIsPutBackAndSaysSo(t *testing.T) {
	r := newRig(t, 1132, 1000, airbag())
	r.library.fail = errors.New("boom")
	r.open()
	r.tt.Until("the library state", func() bool { return r.library.has("states 1") })
	r.tt.Drain()
	r.click("Favourite Airbag")
	r.tt.Until("the failure toast", r.tt.Shows("Couldn't favourite “Airbag”"))
	if !r.tt.HasText("Favourite Airbag") {
		t.Errorf("the failed favourite is still shown")
	}
}

func TestLyricsExcerptThenFullLyrics(t *testing.T) {
	r := newRig(t, 1132, 1000, airbag())
	r.open()
	// Apple's excerpt, with its trailing mark; the rest is not asked for.
	r.tt.Until("the excerpt", r.tt.Shows("In the next world war", "I am born again", "…", "View full lyrics"))
	if r.tt.HasText("In a blaze of light") || r.lyrics.asks() != 0 {
		t.Fatalf("full lyrics before they were asked for (%d asks)", r.lyrics.asks())
	}
	r.click("View full lyrics")
	r.tt.Until("the full lyrics", r.tt.Shows("In a blaze of light", "In an interstellar burst", "Show less", "Written by Thom Yorke, Jonny Greenwood"))
	if r.tt.HasText("…") {
		t.Errorf("the excerpt's mark shows beside the full lyrics")
	}
	r.click("Show less")
	r.tt.Until("the excerpt again", r.tt.Shows("View full lyrics", "…"))
	if r.tt.HasText("In a blaze of light") {
		t.Errorf("full lyrics after Show less")
	}
	// The lyrics are cached under the key now-playing shares.
	if r.lyrics.asks() != 1 {
		t.Errorf("lyrics asked %d times", r.lyrics.asks())
	}
	cached := store.Use(r.h.Env.Queries, store.KeyLyrics("1440"), func(context.Context) (services.Lyrics, error) {
		return services.Lyrics{}, errors.New("not cached")
	})
	if !cached.HasData || len(cached.Data.Lines) != 6 {
		t.Errorf("cache %+v under %v", cached, store.KeyLyrics("1440"))
	}
}

func TestLyricsShowLoadingThenFail(t *testing.T) {
	r := newRig(t, 1132, 1000, airbag())
	r.lyrics.fail(errors.New("down"))
	release := r.lyrics.hold()
	r.open()
	r.click("View full lyrics")
	r.tt.Until("the lyrics to load", r.tt.Shows("Loading the lyrics…"))
	if !r.tt.HasText("In the next world war") || r.tt.HasText("Couldn't load the lyrics") {
		t.Errorf("while pending: %q", r.tt.Texts())
	}
	release()
	r.tt.Until("the failure", r.tt.Shows("Couldn't load the lyrics", "Try again"))
	if r.tt.HasText("Loading the lyrics…") {
		t.Errorf("still loading after the failure")
	}
	if !r.tt.HasText("In the next world war") {
		t.Errorf("the excerpt went away with the failure")
	}
	asks := r.lyrics.asks()
	// No retry on its own while the failure is shown (v0.2.0's retry: false).
	time.Sleep(50 * time.Millisecond)
	r.tt.Drain()
	if r.lyrics.asks() != asks {
		t.Errorf("lyrics asked again by themselves: %d → %d", asks, r.lyrics.asks())
	}
	r.lyrics.fail(nil)
	r.click("Try again")
	r.tt.Until("the lyrics", r.tt.Shows("In a blaze of light"))
	if r.tt.HasText("Couldn't load the lyrics") {
		t.Errorf("the failure stays after the retry worked")
	}
}

func TestOpeningTheLyricsAgainAfterAFailureAsksAgain(t *testing.T) {
	r := newRig(t, 1132, 1000, airbag())
	r.lyrics.fail(errors.New("down"))
	r.open()
	r.click("View full lyrics")
	r.tt.Until("the failure", r.tt.Shows("Couldn't load the lyrics"))
	asks := r.lyrics.asks()
	r.click("Show less")
	r.tt.Until("the excerpt", r.tt.Shows("View full lyrics"))
	// Apple answers this time; the second opening must ask, and show it pending.
	r.lyrics.fail(nil)
	release := r.lyrics.hold()
	r.click("View full lyrics")
	r.tt.Until("the second ask pending", r.tt.Shows("Loading the lyrics…"))
	if r.tt.HasText("Couldn't load the lyrics") {
		t.Errorf("the old failure shows while asking again")
	}
	if r.lyrics.asks() != asks+1 {
		t.Errorf("asked %d times, want %d", r.lyrics.asks(), asks+1)
	}
	release()
	r.tt.Until("the lyrics", r.tt.Shows("In a blaze of light", "Written by Thom Yorke, Jonny Greenwood"))
	if r.tt.HasText("Couldn't load the lyrics") || r.tt.HasText("Loading the lyrics…") {
		t.Errorf("stale state beside the lyrics: %q", r.tt.Texts())
	}
}

func TestLyricsWithNoSongwritersEndWithoutTheLine(t *testing.T) {
	r := newRig(t, 1132, 1000, airbag())
	r.lyrics.l.Songwriters = nil
	r.open()
	r.click("View full lyrics")
	r.tt.Until("the full lyrics", r.tt.Shows("In a blaze of light"))
	for _, w := range r.tt.Texts() {
		if strings.HasPrefix(w, "Written by") {
			t.Errorf("%q", w)
		}
	}
}

func TestCreditsGroupsAndPeople(t *testing.T) {
	r := newRig(t, 1132, 1200, airbag())
	r.open()
	r.tt.Until("the credits", r.tt.Shows(
		"Performers", "Radiohead", "Performer, Vocals", "Nigel Godrich", "Producer",
		"Mixing and mastering", "Chris Blair", "Mastering Engineer"))
	// A person with no picture gets their initials.
	if !r.tt.HasText("NG") && !r.tt.HasText("TP") {
		t.Errorf("no initials for people without a picture: %q", r.tt.Texts())
	}
}

func TestLyricsAndCreditsSitSideBySideFrom1100(t *testing.T) {
	wide := newRig(t, 967, 1200, airbag())
	wide.open()
	wide.tt.Until("both shelves", wide.tt.Shows("Lyrics", "Credits"))
	a, _ := wide.tt.Find("Lyrics")
	b, _ := wide.tt.Find("Credits")
	if a.Y != b.Y || !(a.X < b.X) {
		t.Errorf("at a 1200 window the shelves are %v and %v: want one row", a, b)
	}
	// The window's width (the page column plus the sidebar), not the column's, decides (a CSS viewport query).
	narrow := newRig(t, 767, 1200, airbag())
	narrow.open()
	narrow.tt.Until("both shelves", narrow.tt.Shows("Lyrics", "Credits"))
	a, _ = narrow.tt.Find("Lyrics")
	b, _ = narrow.tt.Find("Credits")
	if !(a.Y < b.Y) || a.X != b.X {
		t.Errorf("at a 1000 window the shelves are %v and %v: want one above the other", a, b)
	}
}

func TestCreditsHaveTwoColumnsFrom720(t *testing.T) {
	wide := newRig(t, 567, 1400, airbag())
	wide.open()
	wide.tt.Until("people", wide.tt.Shows("Radiohead", "Nigel Godrich", "Third Person"))
	a, _ := wide.tt.Find("Performer, Vocals")
	b, _ := wide.tt.Find("Producer")
	if a.Y != b.Y || !(a.X < b.X) {
		t.Errorf("at 800 the first two people are at %v and %v: want one row", a, b)
	}
	narrow := newRig(t, 467, 1600, airbag())
	narrow.open()
	narrow.tt.Until("people", narrow.tt.Shows("Nigel Godrich"))
	a, _ = narrow.tt.Find("Performer, Vocals")
	b, _ = narrow.tt.Find("Producer")
	if !(a.Y < b.Y) {
		t.Errorf("at 700 the first two people are at %v and %v: want a column", a, b)
	}
}

func TestOnlyLyricsOrOnlyCredits(t *testing.T) {
	p := airbag()
	p.Credits = nil
	r := newRig(t, 1200, 1000, p)
	r.open()
	if !r.tt.HasText("Lyrics") || r.tt.HasText("Credits") {
		t.Errorf("only lyrics: %q", r.tt.Texts())
	}
	q := airbag()
	q.LyricsExcerpt = ""
	r2 := newRig(t, 1200, 1000, q)
	r2.open()
	if r2.tt.HasText("Lyrics") || !r2.tt.HasText("Credits") {
		t.Errorf("only credits: %q", r2.tt.Texts())
	}
}

func TestNeitherLyricsNorCreditsSaysSo(t *testing.T) {
	r := newRig(t, 1132, 1000, bare())
	r.open()
	r.tt.Until("the note", r.tt.Shows("Apple Music has no credits or lyrics for this song."))
	// And with no date, composer or ISRC no footer lines.
	for _, w := range r.tt.Texts() {
		if strings.HasPrefix(w, "Released") || strings.HasPrefix(w, "Composed") || strings.HasPrefix(w, "ISRC") {
			t.Errorf("footer line %q with nothing to say", w)
		}
	}
}

func TestShelvesOfVideosAndMoreByTheArtist(t *testing.T) {
	r := newRig(t, 1132, 1600, airbag())
	r.open()
	r.tt.Until("the shelves", r.tt.Shows("Music videos", "Airbag (Live)", "2001", "More by Radiohead", "Kid A", "2000", "Amnesiac", "See all"))
	r.click("Kid A")
	r.tt.Until("the album", r.tt.Shows("ALBUM PAGE al2 song="))
	r.router.Back()
	r.tt.Until("the song", r.tt.Shows("More by Radiohead"))
	r.click("See all")
	r.tt.Until("all albums", r.tt.Shows("ARTIST VIEW a1 full-albums"))
}

func TestMusicVideoTileOpensItsPage(t *testing.T) {
	r := newRig(t, 1132, 1600, airbag())
	r.open()
	r.tt.Until("the video", r.tt.Shows("Airbag (Live)"))
	r.click("Airbag (Live)")
	r.tt.Until("the video page", r.tt.Shows("VIDEO PAGE v1"))
}

func TestMoreByHasNoSeeAllWithoutAnArtistID(t *testing.T) {
	p := airbag()
	p.Song.ArtistID = ""
	r := newRig(t, 1132, 1600, p)
	r.open()
	r.tt.Until("the shelf", r.tt.Shows("More by Radiohead"))
	if r.tt.HasText("See all") {
		t.Errorf("See all without an artist to list")
	}
}

func TestNoShelvesWhenTheSongHasNone(t *testing.T) {
	r := newRig(t, 1132, 1000, bare())
	r.open()
	if r.tt.HasText("Music videos") || r.tt.HasText("More by Radiohead") {
		t.Errorf("empty shelves are drawn: %q", r.tt.Texts())
	}
}

func TestHeaderMenuListsViewCredits(t *testing.T) {
	r := newRig(t, 1132, 1000, airbag())
	r.open()
	r.click("More for Airbag")
	r.tt.Until("the menu", r.tt.Shows("View credits"))
}

// A song row on another page offers View credits, which opens that song's
// page: the route changes from the album to the song.
func TestViewCreditsOnASongRowOpensTheSongPage(t *testing.T) {
	other := airbag()
	other.Song.ID, other.Song.CatalogID, other.Song.Name = "1441", "1441", "Paranoid Android"
	r := newRigWith(t, 1132, 1000, func(r *rig) { r.router.Replace("/album/al1") }, airbag(), other)
	r.tt.Until("the album page", r.tt.Shows("ALBUM PAGE al1 song="))
	if r.route() != "/album/al1" {
		t.Fatalf("starts at %q", r.route())
	}
	r.click("More for Paranoid Android")
	r.tt.Until("the menu", r.tt.Shows("View credits"))
	r.click("View credits")
	r.tt.Until("the song's page", r.tt.Shows("Paranoid Android", "Credits"))
	if r.route() != "/song/1441" {
		t.Errorf("went to %q", r.route())
	}
}

func TestViewCreditsAction(t *testing.T) {
	var went string
	ctx := (&kit.ActionContext{Items: []kit.Item{kit.SongItem(services.Song{ID: "9", CatalogID: "9"})}, Navigate: func(p string) { went = p }})
	if !viewCredits.Available(ctx) {
		t.Error("not available for a catalogue song")
	}
	viewCredits.Run(ctx)
	if went != "/song/9" {
		t.Errorf("went to %q", went)
	}
	lib := &kit.ActionContext{Items: []kit.Item{kit.SongItem(services.Song{ID: "l.9", Source: services.SourceLibrary})}}
	if viewCredits.Available(lib) {
		t.Error("available for an upload with no catalogue ID")
	}
	if viewCredits.Group != kit.GroupItem || viewCredits.Order != 15 || viewCredits.ID != "song.view-credits" || viewCredits.Target != kit.KindSong {
		t.Errorf("def %+v", viewCredits)
	}
}

func TestModuleDeclaration(t *testing.T) {
	m := moduleOf(t)
	if m.ID != "song" || len(m.Routes) != 1 || m.Routes[0].Pattern != "/song/{id}" || len(m.Actions) != 1 || m.Actions[0].ID != "song.view-credits" {
		t.Errorf("module %+v", m)
	}
}

func moduleOf(t *testing.T) module.Module {
	t.Helper()
	h := uitest.New(t)
	return New(h.Env)
}

func TestLoadingShowsTheSkeletonNotAnError(t *testing.T) {
	r := newRigWith(t, 1132, 1000, func(r *rig) { r.catalog.gate = make(chan struct{}) }, airbag())
	r.tt.Drain()
	r.tt.Drain()
	if r.tt.HasText("Airbag") || r.tt.HasText("Try again") || r.tt.HasText("This song isn't available") {
		t.Errorf("not a skeleton while loading: %q", r.tt.Texts())
	}
	close(r.catalog.gate)
	r.open()
}

func TestAnUnknownSongSaysSoWithAWayBack(t *testing.T) {
	r := newRig(t, 1132, 1000)
	r.tt.Until("the message", r.tt.Shows("This song isn't available", "Go to Songs"))
	r.click("Go to Songs")
	r.tt.Until("the songs page", r.tt.Shows("SONGS PAGE"))
}

func TestAnUnknownSongWithoutASongsPageHasNoBackButton(t *testing.T) {
	r := newRig(t, 1132, 1000)
	r.routes = map[string]bool{}
	r.tt.Until("the message", r.tt.Shows("This song isn't available"))
	if r.tt.HasText("Go to Songs") {
		t.Errorf("a back button to a page that is not there")
	}
}

func TestAFailedLoadRetries(t *testing.T) {
	r := newRigWith(t, 1132, 1000, func(r *rig) { r.catalog.set(errors.New("down")) }, airbag())
	r.tt.Until("the error", r.tt.Shows("Couldn't load this song", "Try again"))
	r.catalog.set(nil)
	r.click("Try again")
	r.open()
}

func TestAnotherSongStartsOverCollapsed(t *testing.T) {
	other := airbag()
	other.Song.ID, other.Song.CatalogID, other.Song.Name = "1441", "1441", "Paranoid Android"
	r := newRig(t, 1132, 1000, airbag(), other)
	r.open()
	r.click("View full lyrics")
	r.tt.Until("full", r.tt.Shows("In a blaze of light"))
	r.router.Push("/song/1441")
	r.tt.Until("the other song", r.tt.Shows("Paranoid Android", "View full lyrics"))
}

func TestGoldenSongPage(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []uitest.Option
		w, h int
	}{
		// The shell's page column: the window less sidebar, top bar, player bar.
		{"song-light", nil, 899, 1133},
		{"song-dark", []uitest.Option{uitest.Dark()}, 899, 1133},
		{"song-narrow-light", nil, 707, 480},
	} {
		t.Run(tc.name, func(t *testing.T) {
			i18n.SetRegional("en-GB")
			t.Cleanup(func() { i18n.SetRegional() })
			r := newRigOpts(t, tc.w, tc.h, nil, tc.opts, airbag())
			r.open()
			r.tt.Until("the library state", func() bool { return r.library.has("states 1") })
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), tc.name)
		})
	}
}
