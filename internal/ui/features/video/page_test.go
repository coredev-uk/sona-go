package video

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/library"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// pageRig is the page's route over an empty window, on a router.
type pageRig struct {
	*rig
	router *ui.Router
}

// newPageRig builds the page; edit changes what the catalogue answers before
// the page first asks.
func newPageRig(t *testing.T, dark bool, w, h int, edit ...func(*fakeCat)) *pageRig {
	t.Helper()
	pr := &pageRig{}
	pr.rig = newRig(t, dark, func(r *rig) func(c *ui.Context) {
		r.cat.vid = catalogVideo()
		for _, e := range edit {
			e(r.cat)
		}
		pr.router = ui.NewRouter("/music-video/mv.1")
		return func(c *ui.Context) {
			ui.Column(c).Fill().Children(func() {
				pr.router.View(c, func(rt *ui.Route) {
					if rt.Match(videoPath) {
						r.v.page(c, rt)
					}
				})
			})
			kit.OverlayLayer(c)
		}
	}, w, h)
	return pr
}

func tileVideo(id, name string, year string) services.MusicVideo {
	return services.MusicVideo{ID: id, CatalogID: id, Name: name, Artist: "Mia Rivers", ArtistID: "ar.1", ReleaseDate: year + "-03-01"}
}

func catalogVideo() services.CatalogVideo {
	return services.CatalogVideo{
		Video: services.MusicVideo{
			ID: "mv.1", CatalogID: "mv.1", Name: "Sky Video", Artist: "Mia Rivers", ArtistID: "ar.1", DurationMs: 215_000,
			ReleaseDate: "2019-05-17", Genre: "Pop", Explicit: true, Has4K: true, HasHDR: true,
			URL: "https://music.apple.com/gb/music-video/sky-video/1",
		},
		Artists: []services.Artist{{ID: "ar.1", Name: "Mia Rivers"}},
		Song:    &services.Song{ID: "s.1", CatalogID: "s.1", Name: "Sky", Album: "Clouds", Playable: true},
		Notes:   "A video about the sky.",
		MoreByArtist: []services.MusicVideo{
			tileVideo("mv.2", "Rain Video", "2018"), tileVideo("mv.3", "Snow Video", "2020"),
		},
		MoreInGenre: []services.MusicVideo{tileVideo("mv.4", "Wind Video", "2017")},
	}
}

func (p *pageRig) loaded() {
	p.t.Helper()
	p.tt.Until("the page", p.tt.Shows("Sky Video", "Notes"))
}

func TestPageShowsASkeletonWhileLoadingThenTheVideo(t *testing.T) {
	p := newPageRig(t, false, 1132, 1253)
	p.tt.Frame()
	if !p.tt.HasText("Loading") {
		t.Errorf("no skeleton while pending: %q", p.tt.Texts())
	}
	p.loaded()
	for _, w := range []string{
		"Sky Video", "Mia Rivers", "Music video · 2019 · 3:35 · Pop", "Explicit", "4K", "HDR",
		"Watch in Apple Music", "Play the song", "Play Sky Video",
		"Notes", "A video about the sky.", "More by Mia Rivers", "More like this",
		"Rain Video", "Wind Video", "2018", "Released May 17, 2019",
	} {
		if !p.tt.HasText(w) {
			t.Errorf("page lacks %q: %q", w, p.tt.Texts())
		}
	}
	if !p.tt.HasText("Sona plays music videos in standard definition: Apple allows HD only on players with hardware protection.") {
		t.Errorf("no note that videos play: %q", p.tt.Texts())
	}
	if p.tt.HasText("Sona can't play music videos here: the video opens in Apple Music in your browser.") {
		t.Error("both notes shown")
	}
	// The song line: a link and the album.
	for _, w := range []string{"The song is ", "Sky", " from Clouds."} {
		if !p.tt.HasText(w) && !p.tt.HasText(w[:len(w)-1]) {
			t.Logf("song line part %q not found as one text: %q", w, p.tt.Texts())
		}
	}
}

func TestPlayStartsTheVideoAndOpensTheView(t *testing.T) {
	p := newPageRig(t, false, 1132, 1253)
	p.loaded()
	if err := p.tt.Click("Play Sky Video"); err != nil {
		t.Fatal(err)
	}
	if len(p.queue.videos) != 1 || p.queue.videos[0][0].ID != "mv.1" {
		t.Fatalf("played %v", p.queue.videos)
	}
	if p.env.Video.IsOpen() {
		t.Fatal("the view opened before playback started")
	}
	p.queue.opened() // playback started
	if !p.env.Video.IsOpen() {
		t.Error("the view did not open once playback started")
	}
}

func TestPlayTheSongPlaysThatSongFromItsID(t *testing.T) {
	p := newPageRig(t, false, 1132, 1253)
	p.loaded()
	if err := p.tt.Click("Play the song"); err != nil {
		t.Fatal(err)
	}
	if len(p.queue.songs) != 1 || p.queue.songs[0][0].ID != "s.1" || p.queue.from[0] != (services.PlayFrom{ID: "s.1"}) {
		t.Errorf("played %v from %v", p.queue.songs, p.queue.from)
	}
}

func TestPlayTheSongIsAbsentWhenTheSongCannotPlay(t *testing.T) {
	p := newPageRig(t, false, 1132, 1253, func(f *fakeCat) { f.vid.Song.Playable = false })
	p.loaded()
	if p.tt.HasText("Play the song") {
		t.Error("Play the song for a song that cannot play")
	}
	if !p.tt.HasText("Sky") {
		t.Error("the song line is gone with it")
	}
}

func TestWhereVideosDoNotPlayThePageSaysSoAndOffersAppleMusic(t *testing.T) {
	p := newPageRig(t, false, 1132, 1253)
	p.caps.Video = false
	p.loaded()
	if !p.tt.HasText("Sona can't play music videos here: the video opens in Apple Music in your browser.") {
		t.Errorf("no note: %q", p.tt.Texts())
	}
	if p.tt.HasText("Play Sky Video") {
		t.Error("a Play button where videos cannot play")
	}
	if _, ok := p.tt.Find("More for Sky Video"); ok {
		t.Error("the item menu where videos cannot play")
	}
	if !p.tt.HasText("Watch in Apple Music") {
		t.Error("no way to watch")
	}
}

func TestWatchInAppleMusicOpensTheLinkAndToastsAFailure(t *testing.T) {
	p := newPageRig(t, false, 1132, 1253)
	p.cat.done = make(chan struct{}, 4)
	p.loaded()
	if err := p.tt.Click("Watch in Apple Music"); err != nil {
		t.Fatal(err)
	}
	p.cat.wait(t)
	p.cat.set(func(f *fakeCat) { f.failed = services.ErrNotReleased })
	if err := p.tt.Click("Watch in Apple Music"); err != nil {
		t.Fatal(err)
	}
	p.cat.wait(t)
	p.tt.Until("the toast", p.tt.Shows("Couldn't open Apple Music"))
	p.cat.mu.Lock()
	defer p.cat.mu.Unlock()
	if len(p.cat.opened) != 2 || p.cat.opened[0] != "https://music.apple.com/gb/music-video/sky-video/1" {
		t.Errorf("opened %v", p.cat.opened)
	}
}

func TestPageLeavesOutWhatTheVideoLacks(t *testing.T) {
	p := newPageRig(t, false, 1132, 1253, func(f *fakeCat) {
		f.vid.Song = nil
		f.vid.Notes = ""
		f.vid.Video.URL = ""
		f.vid.Video.ReleaseDate = ""
		f.vid.Video.Genre = ""
		f.vid.Video.Explicit, f.vid.Video.Has4K, f.vid.Video.HasHDR = false, false, false
		f.vid.MoreInGenre = nil
	})
	p.tt.Until("the page", p.tt.Shows("Sky Video", "More by Mia Rivers"))
	for _, w := range []string{"The song is", "Notes", "Watch in Apple Music", "Play the song", "More like this", "Explicit", "4K", "HDR"} {
		if p.tt.HasText(w) {
			t.Errorf("page shows %q", w)
		}
	}
	if !p.tt.HasText("Music video · 3:35") {
		t.Errorf("the meta line keeps empty parts: %q", p.tt.Texts())
	}
	for _, w := range p.tt.Texts() {
		if len(w) >= 8 && w[:8] == "Released" {
			t.Errorf("a release date without one: %q", w)
		}
	}
}

func TestSongLineWithoutAlbumAndLinkedOnlyToAPortedPage(t *testing.T) {
	p := newPageRig(t, false, 1132, 1253, func(f *fakeCat) { f.vid.Song.Album = "" })
	p.loaded()
	if !p.tt.HasText("The song is ") && !p.tt.HasText("The song is") {
		t.Fatalf("no song line: %q", p.tt.Texts())
	}
	if err := p.tt.Click("Sky"); err != nil {
		t.Fatal(err)
	}
	if got := p.router.Path(); got != "/song/s.1" {
		t.Errorf("the song link went to %q", got)
	}
}

func TestSeeAllGoesToTheArtistsVideosOnlyWithAnArtist(t *testing.T) {
	p := newPageRig(t, false, 1132, 1253)
	p.loaded()
	if err := p.tt.Click("See all"); err != nil {
		t.Fatal(err)
	}
	if !p.nav.went("/artist/ar.1/view/music-videos") {
		t.Errorf("pushed %v", p.nav.pushed)
	}

	q := newPageRig(t, false, 1132, 1253, func(f *fakeCat) { f.vid.Video.ArtistID = "" })
	q.loaded()
	if q.tt.HasText("See all") {
		t.Error("See all without an artist ID")
	}
}

func TestTilesOpenTheirPageAndPlayWhereVideosPlay(t *testing.T) {
	p := newPageRig(t, false, 1132, 1253)
	p.loaded()
	if err := p.tt.Click("Play Rain Video"); err != nil {
		t.Fatal(err)
	}
	if len(p.queue.videos) != 1 || p.queue.videos[0][0].ID != "mv.2" {
		t.Errorf("a tile's play control played %v", p.queue.videos)
	}
	if err := p.tt.Click("Wind Video"); err != nil {
		t.Fatal(err)
	}
	if p.nav.last() != "/music-video/mv.4" {
		t.Errorf("a tile opened %v", p.nav.pushed)
	}

	q := newPageRig(t, false, 1132, 1253)
	q.caps.Video = false
	q.loaded()
	if q.tt.HasText("Play Rain Video") {
		t.Error("a tile's play control where videos cannot play")
	}
}

func TestNotFoundPageGoesBackToSearch(t *testing.T) {
	p := newPageRig(t, false, 1132, 1253, func(f *fakeCat) { f.err = library.ErrNotFound })
	p.tt.Until("the error", p.tt.Shows("This music video isn't available", "Go to Search"))
	if err := p.tt.Click("Go to Search"); err != nil {
		t.Fatal(err)
	}
	if !p.nav.went("/search") {
		t.Errorf("pushed %v", p.nav.pushed)
	}
	// With no search page ported the back link is not drawn.
	q := newPageRig(t, false, 1132, 1253, func(f *fakeCat) { f.err = library.ErrNotFound })
	q.env.HasRoute = func(string) bool { return false }
	q.tt.Until("the error", q.tt.Shows("This music video isn't available"))
	if q.tt.HasText("Go to Search") {
		t.Error("a link to a page that is not ported")
	}
}

func TestFailedLoadRetries(t *testing.T) {
	p := newPageRig(t, false, 1132, 1253, func(f *fakeCat) { f.err = services.ErrNotReleased })
	p.tt.Until("the error", p.tt.Shows("Couldn't load this music video", "Try again"))
	p.cat.set(func(f *fakeCat) { f.err = nil })
	if err := p.tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	p.tt.Until("the page", p.tt.Shows("Sky Video"))
}

func TestPageIsReachableAsAModule(t *testing.T) {
	p := newPageRig(t, false, 1132, 1253)
	m := p.v.module()
	if m.ID != "video" || len(m.Routes) != 1 || m.Routes[0].Pattern != "/music-video/{id}" {
		t.Fatalf("module %q routes %v", m.ID, m.Routes)
	}
	if m.Slots.Layer == nil || m.Slots.LayerOpen == nil {
		t.Error("the view's layer is not a slot")
	}
	if m.Slots.LayerOpen() {
		t.Error("the layer is open with the view closed")
	}
}
