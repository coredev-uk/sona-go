package kit

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

func itemT(over func(*services.Item)) services.Item {
	it := services.Item{Kind: services.ItemAlbum, ID: "1", Name: "N"}
	if over != nil {
		over(&it)
	}
	return it
}

func TestItemsLinkGrid(t *testing.T) {
	wide := func(cols int) float32 { return float32(cols*itemsLinkMin + (cols-1)*itemsLinkGap) }
	for _, c := range []struct {
		name       string
		width      float32
		count      int
		cols, rows int
	}{
		// fits as many columns as hold the minimum width, and fills them downward
		{"5 columns, 25 links", wide(5), 25, 5, 5},
		{"2 columns, 21 links", wide(2), 21, 2, 11},
		// never strands a lone button in the last column: Apple's 21 radio
		// genres would be 5+5+5+5+1 and leave Wellbeing alone
		{"21 radio genres", wide(5), 21, 5, 6},
		// 2+1 leaves half a column, which is company enough
		{"3 links, 2 columns", wide(2), 3, 2, 2},
		// drops a column one pixel short of fitting it
		{"one pixel short", wide(3) - 1, 9, 2, 5},
		{"just fits", wide(3), 9, 3, 3},
		// a short list is filled down by the rows it needs, keeping every
		// track: Apple's eight hubs on a row that fits five
		{"8 hubs on 5", wide(5), 8, 5, 2},
		{"3 on 5", wide(5), 3, 5, 1},
		// one column before the row has a width, and for an empty list
		{"no width", 0, 6, 1, 6},
		{"empty", wide(4), 0, 4, 1},
	} {
		cols, rows := itemsLinkGrid(c.width, c.count)
		if cols != c.cols || rows != c.rows {
			t.Errorf("%s: itemsLinkGrid(%v, %d) = %d, %d; want %d, %d", c.name, c.width, c.count, cols, rows, c.cols, c.rows)
		}
	}
}

func TestItemLinkOpensEachKindsPage(t *testing.T) {
	for _, c := range []struct{ kind, id, want string }{
		{services.ItemAlbum, "9", "/album/9"},
		{services.ItemPlaylist, "pl.u-1", "/playlist/pl.u-1"},
		{services.ItemArtist, "5", "/artist/5"},
		{services.ItemSong, "7", "/song/7"},
		{services.ItemStation, "ra.1", "/station/ra.1"},
		{services.ItemMusicVideo, "3", "/music-video/3"},
		{services.ItemCurator, "99", "/curator/99"},
		{services.ItemRoom, "6456176470", "/room/6456176470"},
		{services.ItemMultiroom, "5", "/multiroom/5"},
		{services.ItemGrouping, "10023", "/grouping/10023"},
		{services.ItemProfile, "sp.1", "/profile/sp.1"},
		{services.ItemLink, "", ""},
		{"unknown", "1", ""},
	} {
		if got := ItemLink(itemT(func(i *services.Item) { i.Kind, i.ID = c.kind, c.id })); got != c.want {
			t.Errorf("ItemLink(%s %q) = %q, want %q", c.kind, c.id, got, c.want)
		}
	}
}

func TestItemLinkOpensAStationsPageOnlyWhenItHasOne(t *testing.T) {
	station := func(hasPage bool) services.Item {
		return itemT(func(i *services.Item) {
			i.Kind, i.ID = services.ItemStation, "ra.1"
			i.Station = &services.Station{Kind: services.StationBroadcast, HasPage: hasPage}
		})
	}
	if got := ItemLink(station(true)); got != "/station/ra.1" {
		t.Errorf("station with a page: %q", got)
	}
	// A local broadcaster has no schedule: it plays instead of opening.
	if got := ItemLink(station(false)); got != "" {
		t.Errorf("station without a page: %q", got)
	}
}

func TestItemLinkSendsChartsToTheGenresCharts(t *testing.T) {
	charts := func(id string) services.Item {
		return itemT(func(i *services.Item) { i.Kind, i.ID = services.ItemCharts, id })
	}
	if got := ItemLink(charts("")); got != "/charts" {
		t.Errorf("all charts: %q", got)
	}
	if got := ItemLink(charts("14")); got != "/charts?genre=14" {
		t.Errorf("genre charts: %q", got)
	}
}

func TestItemLinkOpensConcertsInTheAppAndWebOnlyItemsNowhere(t *testing.T) {
	concerts := itemT(func(i *services.Item) {
		i.Kind, i.ID, i.URL = services.ItemConcerts, "", "https://music.apple.com/gb/concerts"
	})
	if got := ItemLink(concerts); got != "/concerts" {
		t.Errorf("concerts: %q", got)
	}
	web := itemT(func(i *services.Item) { i.Kind, i.URL = services.ItemLink, "https://music.apple.com/gb/post/123" })
	if got := ItemLink(web); got != "" {
		t.Errorf("web-only item: %q", got)
	}
}

func TestItemPresentation(t *testing.T) {
	if got := ItemSubtitle(itemT(func(i *services.Item) { i.Caption, i.Subtitle = "New release", "Radiohead" })); got != "New release · Radiohead" {
		t.Errorf("reason and artist: %q", got)
	}
	if got := ItemSubtitle(itemT(func(i *services.Item) { i.Subtitle = "Radiohead" })); got != "Radiohead" {
		t.Errorf("artist alone: %q", got)
	}
	if got := ItemSubtitle(itemT(func(i *services.Item) { i.Caption = "Made for you" })); got != "Made for you" {
		t.Errorf("reason alone: %q", got)
	}
	if got := ItemSubtitle(itemT(nil)); got != "" {
		t.Errorf("neither: %q", got)
	}
	for kind, round := range map[string]bool{
		services.ItemArtist: true, services.ItemProfile: true, services.ItemPlaylist: false,
		services.ItemAlbum: false, services.ItemMusicVideo: false, services.ItemStation: false,
	} {
		if got := ItemRound(itemT(func(i *services.Item) { i.Kind = kind })); got != round {
			t.Errorf("ItemRound(%s) = %v", kind, got)
		}
	}
	s := services.Section{ID: "s", Kind: services.SectionShelf, Title: "T"}
	if got := RoomLink(s); got != "" {
		t.Errorf("section without a room: %q", got)
	}
	s.Room = "77"
	if got := RoomLink(s); got != "/room/77" {
		t.Errorf("section with a room: %q", got)
	}
}

func TestItemVideoAndRowAspect(t *testing.T) {
	video := itemT(func(i *services.Item) { i.Kind = services.ItemMusicVideo })
	post := itemT(func(i *services.Item) {
		i.Kind, i.URL, i.Art.Medium = services.ItemLink, "https://music.apple.com/gb/post/1", "/art/k1/400x225.jpg"
	})
	movie := itemT(func(i *services.Item) {
		i.Kind, i.URL, i.Art.Medium = services.ItemLink, "https://music.apple.com/gb/music-movie/1", "/art/k1/400x225.jpg"
	})
	noArt := itemT(func(i *services.Item) { i.Kind, i.URL = services.ItemLink, "https://music.apple.com/gb/post/1" })
	page := itemT(func(i *services.Item) {
		i.Kind, i.URL, i.Art.Medium = services.ItemLink, "https://example.com/x", "/art/k1/x.jpg"
	})
	for name, c := range map[string]struct {
		it   services.Item
		want bool
	}{"music video": {video, true}, "post": {post, true}, "music movie": {movie, true}, "post without art": {noArt, false},
		"other link": {page, false}, "album": {itemT(nil), false}} {
		if got := ItemVideo(c.it); got != c.want {
			t.Errorf("ItemVideo(%s) = %v", name, got)
		}
	}
	for name, c := range map[string]struct {
		items []services.Item
		want  ItemAspect
	}{
		"empty":         {nil, AspectSquare},
		"all videos":    {[]services.Item{video, post}, AspectVideo},
		"one square":    {[]services.Item{video, itemT(nil)}, AspectSquare},
		"only a square": {[]services.Item{itemT(nil)}, AspectSquare},
	} {
		if got := RowAspect(c.items); got != c.want {
			t.Errorf("RowAspect(%s) = %v", name, got)
		}
	}
}

func TestActionItemOfMapsByPayload(t *testing.T) {
	var p ItemPlayers
	for kind, c := range map[string]struct {
		set  func(*services.Item)
		want ItemKind
	}{
		"song":     {func(i *services.Item) { i.Song = &services.Song{ID: "s"} }, KindSong},
		"album":    {func(i *services.Item) { i.Album = &services.Album{ID: "a"} }, KindAlbum},
		"playlist": {func(i *services.Item) { i.Playlist = &services.Playlist{ID: "p"} }, KindPlaylist},
		"station":  {func(i *services.Item) { i.Station = &services.Station{ID: "r"} }, KindStation},
		"artist":   {func(i *services.Item) { i.Artist = &services.Artist{ID: "x"} }, KindArtist},
	} {
		got, ok := ActionItemOf(itemT(c.set), &p)
		if !ok || got.Kind != c.want {
			t.Errorf("%s: ActionItemOf = %v, %v", kind, got, ok)
		}
	}
	if got, ok := ActionItemOf(itemT(func(i *services.Item) { i.Song = &services.Song{ID: "s"} }), nil); !ok || got.Song().ID != "s" {
		t.Errorf("a song carries its DTO: %v", got)
	}
	// Curators, rooms and links have no actions.
	if _, ok := ActionItemOf(itemT(func(i *services.Item) { i.Kind = services.ItemCurator }), &p); ok {
		t.Error("a curator has actions")
	}
	// A music video only where videos play.
	mv := itemT(func(i *services.Item) { i.Kind, i.MusicVideo = services.ItemMusicVideo, &services.MusicVideo{ID: "v"} })
	if _, ok := ActionItemOf(mv, &p); ok {
		t.Error("a music video has actions where videos do not play")
	}
	withVideo := videoPlayers(true)
	if got, ok := ActionItemOf(mv, withVideo); !ok || got.Kind != KindMusicVideo {
		t.Errorf("a music video where videos play: %v, %v", got, ok)
	}
}

// videoPlayers is a registry with a video player whose platform plays video
// or not.
func videoPlayers(video bool) *ItemPlayers {
	p := &ItemPlayers{Capabilities: func() *services.Capabilities {
		return &services.Capabilities{Playback: services.PlaybackCapabilities{Video: video}}
	}}
	p.RegisterVideo(func(services.MusicVideo) {})
	return p
}

func TestItemsArtAspect(t *testing.T) {
	for path, want := range map[string]float32{
		"/art/k/1000x250.jpg":     4,
		"/art/k/1000x250.jpg?x=1": 4,
		"/art/k/400.jpg":          1,
		"/art/k/400x400.jpg":      1,
		"/art/k/nonsense":         1,
		"":                        1,
		"/art/k/0x10.jpg":         1,
		"/art/k/1060x608.jpg":     1060.0 / 608,
	} {
		if got := itemsArtAspect(path); got < want-0.001 || got > want+0.001 {
			t.Errorf("itemsArtAspect(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestStationTexts(t *testing.T) {
	if got := OnAirLine(services.OnAir{Artist: "Sample Artist", Title: "Sample Song"}); got != "Sample Artist – Sample Song" {
		t.Errorf("on air: %q", got)
	}
	if got := OnAirLine(services.OnAir{Title: "Just the title"}); got != "Just the title" {
		t.Errorf("on air without an artist: %q", got)
	}
	if got := StationUnavailableText(services.StationNoWidevine); got != "This stream isn't offered with Widevine, the only protection Sona can play." {
		t.Errorf("no Widevine: %q", got)
	}
	if got := StationUnavailableText(""); got != "" {
		t.Errorf("available: %q", got)
	}
}

func TestReleaseBadge(t *testing.T) {
	i18n.SetLocale("en")
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.Local)
	out := func(at time.Time, pre bool) services.Album {
		return services.Album{Prerelease: pre, ReleaseAt: at.UnixMilli()}
	}
	same := time.Date(2026, 11, 19, 0, 0, 0, 0, time.Local)
	if got := itemsReleaseBadge(out(same, true), now); got != "Out Nov 19" {
		t.Errorf("this year: %q", got)
	}
	if got := itemsReleaseBadge(out(time.Date(2027, 1, 8, 0, 0, 0, 0, time.Local), true), now); got != "Out Jan 8, 2027" {
		t.Errorf("another year: %q", got)
	}
	if got := itemsReleaseBadge(out(same, false), now); got != "" {
		t.Errorf("not prerelease: %q", got)
	}
	if got := itemsReleaseBadge(out(now.Add(-time.Hour), true), now); got != "" {
		t.Errorf("out once the release time has passed: %q", got)
	}
	if got := itemsReleaseBadge(services.Album{Prerelease: true}, now); got != "" {
		t.Errorf("no full date: %q", got)
	}
	if !itemsUpcoming(services.Album{Prerelease: true}, now) {
		t.Error("a prerelease album with no full date is upcoming")
	}
}
