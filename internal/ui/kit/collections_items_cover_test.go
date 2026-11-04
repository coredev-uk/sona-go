package kit

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// itemsRec records what the shared item components asked the app to do.
type itemsRec struct {
	nav      []string
	urls     []string
	opened   []string
	played   []string
	songs    []string
	videos   []string
	toggles  int
	players  *ItemPlayers
	clock    time.Time
	hasVideo bool
}

// newItemsRec is a context whose players record what they play: every kind
// plays, songs and music videos too, on a platform that plays videos.
func newItemsRec() *itemsRec {
	r := &itemsRec{clock: time.Date(2026, 11, 4, 12, 0, 0, 0, time.Local), hasVideo: true}
	p := &ItemPlayers{
		Capabilities: func() *services.Capabilities {
			return &services.Capabilities{Playback: services.PlaybackCapabilities{Video: r.hasVideo}}
		},
		Now: func() time.Time { return r.clock },
	}
	for _, kind := range []string{services.ItemSong, services.ItemAlbum, services.ItemPlaylist, services.ItemArtist, services.ItemStation, services.ItemCurator} {
		p.Register(kind, func(it services.Item) { r.played = append(r.played, it.Kind+":"+it.ID) })
	}
	p.RegisterSongs(func(songs []services.Song, start int, label string) {
		names := make([]string, len(songs))
		for i, s := range songs {
			names[i] = s.Name
		}
		r.songs = append(r.songs, label+"@"+songs[start].Name+"/"+names[0]+".."+names[len(names)-1])
	})
	p.RegisterVideo(func(v services.MusicVideo, _ *ui.Handle) { r.videos = append(r.videos, v.ID) })
	r.players = p
	return r
}

func (r *itemsRec) ctx() ItemsCtx {
	return ItemsCtx{
		Nav:     func(p string) { r.nav = append(r.nav, p) },
		OpenURL: func(u string) { r.urls = append(r.urls, u) },
		Players: r.players,
		OnOpen:  func(it services.Item) { r.opened = append(r.opened, it.Kind+":"+it.ID) },
		Toggle:  func() { r.toggles++ },
	}
}

func stationItem(hasPage, playable bool) services.Item {
	return services.Item{
		Kind: services.ItemStation, ID: "ra.1", Name: "Local FM",
		URL:     "https://music.apple.com/gb/station/local-fm/ra.1",
		Station: &services.Station{ID: "ra.1", Name: "Local FM", Kind: services.StationBroadcast, HasPage: hasPage, Playable: playable},
	}
}

func TestStationTileWithAPageOpensItAndHasAPlayControl(t *testing.T) {
	r := newItemsRec()
	tile := CoverItemOf(stationItem(true, true), r.ctx(), 0)
	tile.Open()
	if !slices.Equal(r.nav, []string{"/station/ra.1"}) {
		t.Errorf("opened %v", r.nav)
	}
	if tile.Play == nil || tile.Play.Label != "Local FM" {
		t.Fatalf("play %+v", tile.Play)
	}
	if tile.PlaysOnOpen {
		t.Error("a station with a page plays on open")
	}
	tile.Play.Run()
	if !slices.Equal(r.played, []string{"station:ra.1"}) {
		t.Errorf("played %v", r.played)
	}
}

func TestStationTileWithoutAPagePlaysWhenOpened(t *testing.T) {
	r := newItemsRec()
	tile := CoverItemOf(stationItem(false, true), r.ctx(), 0)
	if tile.Play != nil {
		t.Error("a page-less station has a second play control")
	}
	if !tile.PlaysOnOpen {
		t.Error("a page-less station does not play on open")
	}
	tile.Open()
	if len(r.nav) != 0 {
		t.Errorf("navigated to %v", r.nav)
	}
	if !slices.Equal(r.played, []string{"station:ra.1"}) || len(r.urls) != 0 {
		t.Errorf("played %v, opened urls %v", r.played, r.urls)
	}
	// Remembered first: OnOpen runs before the station plays.
	if !slices.Equal(r.opened, []string{"station:ra.1"}) {
		t.Errorf("OnOpen ran for %v", r.opened)
	}
}

func TestOpenWithoutPagePlaysAStationAndOpensWebOnlyItemsInTheBrowser(t *testing.T) {
	r := newItemsRec()
	OpenWithoutPage(stationItem(false, true), r.ctx())
	if len(r.played) != 1 {
		t.Fatalf("played %v", r.played)
	}
	web := services.Item{Kind: services.ItemLink, Name: "Concert", URL: "https://example.com/c"}
	OpenWithoutPage(web, r.ctx())
	if !slices.Equal(r.urls, []string{"https://example.com/c"}) || len(r.played) != 1 {
		t.Errorf("urls %v, played %v", r.urls, r.played)
	}
	// A station that cannot play falls back to its URL.
	OpenWithoutPage(stationItem(false, false), r.ctx())
	if len(r.played) != 1 || len(r.urls) != 2 {
		t.Errorf("an unplayable station: played %v, urls %v", r.played, r.urls)
	}
}

func TestTileOnOpenRunsForPagesAndBeforeOpeningPagelessItems(t *testing.T) {
	r := newItemsRec()
	album := services.Item{Kind: services.ItemAlbum, ID: "9", Name: "A", Album: &services.Album{ID: "9"}}
	CoverItemOf(album, r.ctx(), 0).Open()
	web := services.Item{Kind: services.ItemLink, Name: "Concert", URL: "https://example.com/c"}
	CoverItemOf(web, r.ctx(), 1).Open()
	if !slices.Equal(r.opened, []string{"album:9", "link:"}) {
		t.Errorf("OnOpen ran for %v", r.opened)
	}
	if !slices.Equal(r.nav, []string{"/album/9"}) || !slices.Equal(r.urls, []string{"https://example.com/c"}) {
		t.Errorf("nav %v, urls %v", r.nav, r.urls)
	}
	// Without OnOpen or Nav a tile still opens.
	ctx := r.ctx()
	ctx.OnOpen, ctx.Nav = nil, nil
	CoverItemOf(album, ctx, 0).Open()
}

func TestCoverItemOfBuildsTheTile(t *testing.T) {
	r := newItemsRec()
	it := services.Item{
		Kind: services.ItemAlbum, ID: "9", Name: "OK Computer", Caption: "New release", Subtitle: "Radiohead",
		Art:   services.Art{Medium: "/art/ka/400x400.jpg"},
		Wide:  services.Art{Medium: "/art/kw/1000x250.jpg"},
		Album: &services.Album{ID: "9", Name: "OK Computer", Motion: "/motion/k/1.mp4"},
	}
	tile := CoverItemOf(it, r.ctx(), 3)
	if tile.Key != "3:album:9" || tile.Title != "OK Computer" || tile.Subtitle != "New release · Radiohead" {
		t.Errorf("tile %+v", tile)
	}
	if tile.Art != "/art/ka/400x400.jpg" || tile.Motion != "/motion/k/1.mp4" || tile.Round || tile.Video {
		t.Errorf("art %q motion %q round %v video %v", tile.Art, tile.Motion, tile.Round, tile.Video)
	}
	if tile.Item == nil || tile.Item.Kind != KindAlbum || tile.Item.Album().ID != "9" {
		t.Errorf("target %+v", tile.Item)
	}
	if tile.Play == nil || tile.Badge != "" {
		t.Errorf("play %+v badge %q", tile.Play, tile.Badge)
	}

	// The wide art stands in for a missing cover; no id keys by the URL.
	it.Art, it.ID, it.URL = services.Art{}, "", "https://music.apple.com/gb/x"
	tile = CoverItemOf(it, r.ctx(), 0)
	if tile.Art != "/art/kw/1000x250.jpg" || tile.Key != "0:album:https://music.apple.com/gb/x" {
		t.Errorf("fallback art %q key %q", tile.Art, tile.Key)
	}

	// Artists are round; their motion rides along. Music videos are 16:9.
	artist := services.Item{Kind: services.ItemArtist, ID: "5", Name: "Bjork", Artist: &services.Artist{ID: "5", Motion: "/motion/k/a.mp4"}}
	if tile = CoverItemOf(artist, r.ctx(), 0); !tile.Round || tile.Motion != "/motion/k/a.mp4" {
		t.Errorf("artist tile %+v", tile)
	}
	pl := services.Item{Kind: services.ItemPlaylist, ID: "p", Playlist: &services.Playlist{ID: "p", Motion: "/motion/k/p.mp4"}}
	if tile = CoverItemOf(pl, r.ctx(), 0); tile.Motion != "/motion/k/p.mp4" {
		t.Errorf("playlist motion %q", tile.Motion)
	}
	st := stationItem(true, true)
	st.Station.Motion = "/motion/k/s.mp4"
	if tile = CoverItemOf(st, r.ctx(), 0); tile.Motion != "/motion/k/s.mp4" {
		t.Errorf("station motion %q", tile.Motion)
	}
	mv := services.Item{Kind: services.ItemMusicVideo, ID: "v", Name: "Video", MusicVideo: &services.MusicVideo{ID: "v"}}
	tile = CoverItemOf(mv, r.ctx(), 0)
	if !tile.Video || tile.Play == nil || tile.Item == nil || tile.Item.Kind != KindMusicVideo {
		t.Errorf("music video tile %+v", tile)
	}
	tile.Play.Run()
	if !slices.Equal(r.videos, []string{"v"}) {
		t.Errorf("played videos %v", r.videos)
	}
	r.hasVideo = false
	if tile = CoverItemOf(mv, r.ctx(), 0); tile.Play != nil || tile.Item != nil {
		t.Errorf("a music video where videos do not play: %+v", tile)
	}
	if tile = CoverItemOf(services.Item{Kind: services.ItemCurator, ID: "c", Name: "C"}, r.ctx(), 0); tile.Item != nil {
		t.Errorf("a curator has a menu: %+v", tile.Item)
	}
}

func TestAnAlbumNotOutYetShowsTheReleaseBadgeInsteadOfAPlayControl(t *testing.T) {
	i18n.SetLocale("en")
	r := newItemsRec()
	soon := r.clock.AddDate(0, 0, 15) // 19 Nov
	it := services.Item{
		Kind: services.ItemAlbum, ID: "9", Name: "Soon",
		Album: &services.Album{ID: "9", Prerelease: true, ReleaseAt: soon.UnixMilli()},
	}
	tile := CoverItemOf(it, r.ctx(), 0)
	if tile.Play != nil {
		t.Error("an upcoming album has a play control")
	}
	if tile.Badge != "Out Nov 19" {
		t.Errorf("badge %q", tile.Badge)
	}
	// Once the release time passes it plays and has no badge.
	r.clock = soon.Add(time.Hour)
	tile = CoverItemOf(it, r.ctx(), 0)
	if tile.Play == nil || tile.Badge != "" {
		t.Errorf("a released album: play %+v badge %q", tile.Play, tile.Badge)
	}
}

func TestCanPlay(t *testing.T) {
	r := newItemsRec()
	p := r.players
	soon := services.Album{Prerelease: true, ReleaseAt: r.clock.Add(time.Hour).UnixMilli()}
	for name, c := range map[string]struct {
		it   services.Item
		want bool
	}{
		"unregistered kind":    {services.Item{Kind: services.ItemRoom, ID: "1"}, false},
		"song playable":        {services.Item{Kind: services.ItemSong, Song: &services.Song{Playable: true}}, true},
		"song not playable":    {services.Item{Kind: services.ItemSong, Song: &services.Song{}}, false},
		"song without DTO":     {services.Item{Kind: services.ItemSong}, false},
		"album":                {services.Item{Kind: services.ItemAlbum, Album: &services.Album{}}, true},
		"album without DTO":    {services.Item{Kind: services.ItemAlbum}, true},
		"album not out":        {services.Item{Kind: services.ItemAlbum, Album: &soon}, false},
		"station playable":     {stationItem(true, true), true},
		"station unplayable":   {stationItem(true, false), false},
		"playlist":             {services.Item{Kind: services.ItemPlaylist}, true},
		"curator (registered)": {services.Item{Kind: services.ItemCurator}, true},
		"music video":          {services.Item{Kind: services.ItemMusicVideo, MusicVideo: &services.MusicVideo{}}, true},
		"music video no DTO":   {services.Item{Kind: services.ItemMusicVideo}, false},
	} {
		if got := p.CanPlay(c.it); got != c.want {
			t.Errorf("CanPlay(%s) = %v, want %v", name, got, c.want)
		}
	}
	var none *ItemPlayers
	if none.CanPlay(stationItem(true, true)) || none.CanPlayVideo() {
		t.Error("a nil registry plays")
	}
	none.PlayItem(stationItem(true, true))
	none.PlaySongList([]services.Song{{}}, 0, "x")
}

func TestCanPlayVideoNeedsAVideoPlayerAndTheCapability(t *testing.T) {
	var p ItemPlayers
	if p.CanPlayVideo() {
		t.Error("videos play with nothing registered")
	}
	p.RegisterVideo(func(services.MusicVideo, *ui.Handle) {})
	if p.CanPlayVideo() {
		t.Error("videos play before Go has answered")
	}
	var caps *services.Capabilities
	p.Capabilities = func() *services.Capabilities { return caps }
	if p.CanPlayVideo() {
		t.Error("videos play before the capabilities arrive")
	}
	caps = &services.Capabilities{}
	if p.CanPlayVideo() {
		t.Error("videos play where the platform cannot")
	}
	caps.Playback.Video = true
	if !p.CanPlayVideo() {
		t.Error("videos do not play where they can")
	}
	if videoPlayers(true).CanPlayVideo() != true || videoPlayers(false).CanPlayVideo() != false {
		t.Error("videoPlayers")
	}
	// Without a registered video player the capability alone is not enough.
	q := ItemPlayers{Capabilities: p.Capabilities}
	if q.CanPlayVideo() {
		t.Error("videos play with no video player")
	}
}

func TestPlayItemPlaysAMusicVideoAloneAndSongListsFromAnIndex(t *testing.T) {
	r := newItemsRec()
	p := r.players
	p.PlayItem(services.Item{Kind: services.ItemMusicVideo, MusicVideo: &services.MusicVideo{ID: "v1"}})
	p.PlayItem(services.Item{Kind: services.ItemMusicVideo}) // no payload: nothing
	if !slices.Equal(r.videos, []string{"v1"}) {
		t.Errorf("videos %v", r.videos)
	}
	p.PlayItem(services.Item{Kind: services.ItemAlbum, ID: "9"})
	p.PlayItem(services.Item{Kind: services.ItemRoom, ID: "r"}) // no player: nothing
	if !slices.Equal(r.played, []string{"album:9"}) {
		t.Errorf("played %v", r.played)
	}
	p.PlaySongList([]services.Song{{Name: "A"}, {Name: "B"}, {Name: "C"}}, 1, "Top Songs")
	if !slices.Equal(r.songs, []string{"Top Songs@B/A..C"}) {
		t.Errorf("song lists %v", r.songs)
	}
	// A later registration replaces the earlier one.
	var second []string
	p.Register(services.ItemAlbum, func(it services.Item) { second = append(second, it.ID) })
	p.PlayItem(services.Item{Kind: services.ItemAlbum, ID: "10"})
	if !slices.Equal(second, []string{"10"}) || len(r.played) != 1 {
		t.Errorf("replaced player: second %v, first %v", second, r.played)
	}
}
