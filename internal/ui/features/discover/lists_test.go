package discover

import (
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

func songItem(i int, playable bool) services.Item {
	id := fmt.Sprintf("s%d", i)
	return services.Item{Kind: services.ItemSong, ID: id, Name: fmt.Sprintf("Song %d", i), Song: &services.Song{
		ID: id, Source: services.SourceCatalog, Name: fmt.Sprintf("Song %d", i), Artist: fmt.Sprintf("Artist %d", i), ArtistID: id, Album: "Album", AlbumID: "al1", Playable: playable,
	}}
}

func songItems(from, n int) []services.Item {
	out := make([]services.Item, n)
	for i := range out {
		out[i] = songItem(from+i, true)
	}
	return out
}

func albumItems(from, n int) []services.Item {
	out := make([]services.Item, n)
	for i := range out {
		out[i] = albumItem(fmt.Sprint(from+i), fmt.Sprintf("Album %d", from+i))
	}
	return out
}

// calls draws frames until the fake has been asked n times with prefix, then a
// few more, so a request that should not come has had its chance to.
func (r *rig) calls(api *fakeAPI, prefix string, n int) {
	r.t.Helper()
	r.tt.Until(fmt.Sprintf("%d requests for %s", n, prefix), func() bool { return api.count(prefix) >= n })
	for range 6 {
		r.tt.Drain()
	}
}

func newRoom(t *testing.T, path string, room services.RoomPage, more func(offset int) services.ItemPage, opts ...rigOption) (*rig, *fakeAPI) {
	t.Helper()
	api := &fakeAPI{room: room, roomItems: more}
	return newRig(t, path, append([]rigOption{withAPI(api)}, opts...)...), api
}

var roomSorts = []string{"featured", "alphabet", "releaseDate"}

func TestRoomOfTracksShowsTheTitleSongsPlayAndShuffle(t *testing.T) {
	r, api := newRoom(t, "/room/6828645964", services.RoomPage{Title: "New This Week", Kind: services.SectionTracks, Items: songItems(0, 20), Sorts: roomSorts, Sort: "featured"}, nil)
	r.tt.Until("the room", r.tt.Shows("New This Week", "Song 0", "Song 19", "Play New This Week", "Shuffle", "Order", "Editors' order"))
	if got := api.asked(); len(got) != 1 || got[0] != "room/6828645964?" {
		t.Errorf("asked %v", got)
	}
	if err := r.tt.Click("Play New This Week"); err != nil {
		t.Fatal(err)
	}
	got := r.waitPlays(1)[0]
	if len(got.refs) != 20 || got.start != 0 || got.from != (services.PlayFrom{ID: "New This Week"}) {
		t.Errorf("Play played %d songs from %d as %+v", len(got.refs), got.start, got.from)
	}
	if err := r.tt.Click("Shuffle"); err != nil {
		t.Fatal(err)
	}
	got = r.waitPlays(2)[1]
	if len(got.refs) != 20 || got.from != (services.PlayFrom{ID: "New This Week"}) {
		t.Errorf("Shuffle played %+v", got)
	}
}

func TestRoomOfTracksRowPlaysFromThatSongWithTheRoomAsSource(t *testing.T) {
	r, _ := newRoom(t, "/room/1", services.RoomPage{Title: "Room", Kind: services.SectionTracks, Items: songItems(0, 10)}, nil)
	r.tt.Until("the room", r.tt.Shows("Song 4"))
	// A double-click on the row (away from the links in it) plays from it.
	row, ok := r.tt.Find("Song 4")
	if !ok {
		t.Fatal("no row")
	}
	r.tt.ClickAt(row.X+100, row.Y+row.H/2)
	r.tt.ClickAt(row.X+100, row.Y+row.H/2)
	got := r.waitPlays(1)[0]
	if got.start != 4 || len(got.refs) != 10 || got.from != (services.PlayFrom{ID: "Room"}) {
		t.Errorf("played %+v", got)
	}
}

func TestRoomWithNoPlayableSongDisablesPlay(t *testing.T) {
	items := []services.Item{songItem(0, false), songItem(1, false)}
	r, _ := newRoom(t, "/room/1", services.RoomPage{Title: "Room", Kind: services.SectionTracks, Items: items}, nil)
	r.tt.Until("the room", r.tt.Shows("Play Room"))
	_ = r.tt.Click("Play Room")
	r.tt.Drain()
	if got := r.play.plays(); len(got) != 0 {
		t.Errorf("a disabled Play played %+v", got)
	}
}

func TestRoomLoadsFurtherPagesOfAHundredOnDemandFromTheNextOffset(t *testing.T) {
	more := func(offset int) services.ItemPage {
		switch offset {
		case 100:
			return services.ItemPage{Items: songItems(100, 100), Next: 200}
		case 200:
			return services.ItemPage{Items: songItems(200, 50), Next: 0}
		}
		t.Errorf("a page at offset %d was asked for", offset)
		return services.ItemPage{}
	}
	r, api := newRoom(t, "/room/1", services.RoomPage{Title: "Room", Kind: services.SectionTracks, Items: songItems(0, 100), Next: 100}, more)
	r.tt.Until("the first page", r.tt.Shows("Song 0"))
	r.calls(api, "room-items", 1)
	// Two hundred songs is a list built whole, with a row to come: a long
	// list asks for its next page as the user scrolls near the end.
	if got := api.count("room-items"); got != 1 {
		t.Fatalf("%d further pages asked for before any scrolling, want the one the short list needed", got)
	}
	r.d.room.pg.pos.Y = math.MaxFloat32
	r.calls(api, "room-items", 2)
	want := []string{"room/1?", "room-items/1?@100", "room-items/1?@200"}
	if got := api.asked(); !slices.Equal(got, want) {
		t.Errorf("asked %v, want %v", got, want)
	}
	if n := len(r.d.room.list.songs); n != 250 {
		t.Errorf("%d songs loaded, want 250", n)
	}
}

func TestRoomAsksForOnePageAtATime(t *testing.T) {
	more := func(offset int) services.ItemPage {
		return services.ItemPage{Items: songItems(offset, 100), Next: offset + 100}
	}
	r, api := newRoom(t, "/room/1", services.RoomPage{Title: "Room", Kind: services.SectionTracks, Items: songItems(0, 100), Next: 100}, more)
	api.blockItems = make(chan struct{})
	r.tt.Until("the first page", r.tt.Shows("Song 0"))
	r.tt.Until("a page asked for", func() bool { return api.count("room-items") == 1 })
	for range 20 {
		r.tt.Drain()
	}
	if n := api.count("room-items"); n != 1 {
		t.Errorf("%d pages asked for while the first was loading", n)
	}
	close(api.blockItems)
	r.tt.Until("the page", func() bool { return len(r.d.room.list.songs) >= 200 })
}

func TestRoomOfOtherKindsIsAGridLabelledByTheTitle(t *testing.T) {
	more := func(offset int) services.ItemPage { return services.ItemPage{Items: albumItems(100, 20)} }
	r, api := newRoom(t, "/room/2", services.RoomPage{Title: "Albums room", Kind: services.SectionShelf, Items: albumItems(0, 100), Next: 100}, more, withSize(1132, 900))
	r.tt.Until("the grid", r.tt.Shows("Albums room", "Album 0"))
	r.calls(api, "room-items", 1)
	if r.tt.HasText("Play Albums room") {
		t.Error("a grid room has Play and Shuffle")
	}
	if _, ok := r.tt.Find("Albums room"); !ok {
		t.Error("the grid is not labelled by the title")
	}
	if got := api.count("room-items"); got != 1 {
		t.Errorf("%d further pages asked for, want 1", got)
	}
}

func TestRoomOrderSelectChangesTheSortInTheURLAndAsksForIt(t *testing.T) {
	r, api := newRoom(t, "/room/6828645964", services.RoomPage{Title: "Room", Kind: services.SectionShelf, Items: albumItems(0, 5), Sorts: roomSorts, Sort: "featured"}, nil)
	r.tt.Until("the room", r.tt.Shows("Order", "Editors' order"))
	if err := r.tt.Click("Order"); err != nil {
		t.Fatal(err)
	}
	r.tt.Drain()
	for _, w := range []string{"Editors' order", "A to Z", "Newest first"} {
		if !r.tt.HasText(w) {
			t.Fatalf("the Order choices lack %q: %q", w, r.tt.Texts())
		}
	}
	if err := r.tt.Click("A to Z"); err != nil {
		t.Fatal(err)
	}
	r.tt.Drain()
	if got := r.path(); got != "/room/6828645964?sort=alphabet" {
		t.Errorf("the URL is %q", got)
	}
	r.tt.Until("the room in that order", func() bool { return api.count("room/6828645964?alphabet") == 1 })
}

func TestRoomWithOneSortHasNoOrderControl(t *testing.T) {
	r, _ := newRoom(t, "/room/1", services.RoomPage{Title: "Room", Kind: services.SectionShelf, Items: albumItems(0, 3), Sorts: []string{"featured"}, Sort: "featured"}, nil)
	r.tt.Until("the room", r.tt.Shows("Album 0"))
	if r.tt.HasText("Order") {
		t.Error("a room with one order has an Order control")
	}
}

func TestRoomLoadingAndErrorStates(t *testing.T) {
	api := &fakeAPI{block: make(chan struct{}), room: services.RoomPage{Title: "Room", Kind: services.SectionShelf, Items: albumItems(0, 3)}}
	r := newRig(t, "/room/1", withAPI(api))
	r.tt.Until("the skeleton", r.tt.Shows("Loading"))
	close(api.block)
	r.tt.Until("the room", r.tt.Shows("Album 0"))

	bad := &fakeAPI{err: applemusic.ErrNotFound}
	r2 := newRig(t, "/room/1", withAPI(bad))
	r2.tt.Until("the error", r2.tt.Shows("Couldn't load this list", i18n.ErrorText(services.CodeNotFound), "Try again"))
}

// The charts.

func chartsFixture() services.ChartsPage {
	return services.ChartsPage{
		Genres: []services.Genre{{ID: "", Name: "All music"}, {ID: "14", Name: "Pop"}},
		Charts: []services.Chart{
			{Key: "songs", Title: "Songs", Items: songItems(0, 12), Next: 50},
			{Key: "cityCharts", ChartID: "135", Title: "City Charts", Items: []services.Item{playlistItem("c1", "London")}},
			{Key: "albums", Title: "Albums", Items: albumItems(0, 3), Next: 50},
			{Key: "music-videos", Title: "Videos", Items: []services.Item{{Kind: services.ItemMusicVideo, ID: "v1", Name: "A Video", MusicVideo: &services.MusicVideo{ID: "v1", Name: "A Video"}}}},
		},
		FetchedAt: 0,
	}
}

func newCharts(t *testing.T, path string, page services.ChartsPage, opts ...rigOption) (*rig, *fakeAPI) {
	t.Helper()
	api := &fakeAPI{charts: page}
	return newRig(t, path, append([]rigOption{withAPI(api), withSize(1132, 1253)}, opts...)...), api
}

func TestChartsShowTopSongsRankedAndTheOtherChartsAsShelves(t *testing.T) {
	r, _ := newCharts(t, "/charts", chartsFixture())
	r.tt.Until("the charts", r.tt.Shows("Charts", "Top songs", "Song 9", "Top 25 songs by city", "Top albums", "Top music videos", "Genre", "All genres"))
	if r.tt.HasText("Song 10") {
		t.Error("the overview lists more than the ten top songs")
	}
	if n := countText(r.tt.Session, "See all"); n != 4 {
		t.Errorf("%d See all links, want one for each chart", n)
	}
}

func TestChartsFreshnessLineAndNoGenreSelectForOneGenre(t *testing.T) {
	page := chartsFixture()
	page.Genres = page.Genres[:1]
	page.FetchedAt = uitest.Epoch.Add(-3 * time.Minute).UnixMilli()
	r, _ := newCharts(t, "/charts", page)
	r.tt.Until("the charts", r.tt.Shows("Updated 3 min. ago"))
	if r.tt.HasText("Genre") {
		t.Error("a Genre select with a single genre")
	}
}

func TestChartsGenreSelectChangesTheGenreInTheURL(t *testing.T) {
	r, api := newCharts(t, "/charts", chartsFixture())
	r.tt.Until("the charts", r.tt.Shows("Genre"))
	if err := r.tt.Click("Genre"); err != nil {
		t.Fatal(err)
	}
	r.tt.Drain()
	if err := r.tt.Click("Pop"); err != nil {
		t.Fatal(err)
	}
	r.tt.Drain()
	if got := r.path(); got != "/charts?genre=14" {
		t.Errorf("the URL is %q", got)
	}
	r.tt.Until("the charts of the genre", func() bool { return api.count("charts?14") == 1 })
}

func TestChartsSeeAllOpensTheChartInFull(t *testing.T) {
	page := chartsFixture()
	page.Genre = ""
	page.Charts = page.Charts[1:] // no top songs: the first See all is the city chart's
	r, _ := newCharts(t, "/charts?genre=14", page)
	r.tt.Until("the charts", r.tt.Shows("Top 25 songs by city", "See all"))
	if err := r.tt.Click("See all"); err != nil {
		t.Fatal(err)
	}
	r.tt.Drain()
	if got := r.path(); got != "/charts/cityCharts?chartId=135&genre=14" {
		t.Errorf("See all went to %q", got)
	}

	r2, _ := newCharts(t, "/charts", chartsFixture())
	r2.tt.Until("the charts", r2.tt.Shows("Top songs", "See all"))
	if err := r2.tt.Click("See all"); err != nil {
		t.Fatal(err)
	}
	r2.tt.Drain()
	if got := r2.path(); got != "/charts/songs" {
		t.Errorf("the top songs' See all went to %q", got)
	}
}

func TestChartsTopSongsPlayAndShuffleTheFirstPageWithTheChartTitleAsSource(t *testing.T) {
	r, _ := newCharts(t, "/charts", chartsFixture())
	r.tt.Until("the charts", r.tt.Shows("Play Top songs"))
	if err := r.tt.Click("Play Top songs"); err != nil {
		t.Fatal(err)
	}
	got := r.waitPlays(1)[0]
	if len(got.refs) != 12 || got.start != 0 || got.from != (services.PlayFrom{ID: "Top songs"}) {
		t.Errorf("played %d songs from %d as %+v: the chart's first page is the queue", len(got.refs), got.start, got.from)
	}
	if err := r.tt.Click("Shuffle"); err != nil {
		t.Fatal(err)
	}
	if got := r.waitPlays(2)[1]; len(got.refs) != 12 || got.from.ID != "Top songs" {
		t.Errorf("shuffled %+v", got)
	}
}

func TestChartsTopSongsRowsAreOneTabStopTheArrowsMoveAndEnterPlaysFromTheRow(t *testing.T) {
	r, _ := newCharts(t, "/charts", chartsFixture())
	r.tt.Until("the charts", r.tt.Shows("Top songs", "Song 9"))
	focus := func() string {
		if f := access.Read(t, r.tt.Tester).Focus; f != nil {
			return f.Label
		}
		return ""
	}
	for i := 0; i < 20 && focus() != "Song 0"; i++ {
		r.tt.Key(0, ui.KeyTab)
		r.tt.Drain()
	}
	if focus() != "Song 0" {
		t.Fatalf("Tab did not reach the first row: on %q", focus())
	}
	r.tt.Key(0, ui.KeyDown)
	r.tt.Drain()
	if focus() != "Song 1" {
		t.Fatalf("Down did not move the focus to the second row: on %q", focus())
	}
	r.tt.Key(0, ui.KeyEnter)
	got := r.waitPlays(1)[0]
	if got.start != 1 || len(got.refs) != 12 || got.from != (services.PlayFrom{ID: "Top songs"}) {
		t.Errorf("Enter played %d songs from %d as %+v", len(got.refs), got.start, got.from)
	}
}

func TestChartsTopSongsWithNoPlayableSongCannotPlay(t *testing.T) {
	page := chartsFixture()
	page.Charts[0].Items = []services.Item{songItem(0, false)}
	r, _ := newCharts(t, "/charts", page)
	r.tt.Until("the charts", r.tt.Shows("Play Top songs"))
	_ = r.tt.Click("Play Top songs")
	r.tt.Drain()
	if got := r.play.plays(); len(got) != 0 {
		t.Errorf("played %+v", got)
	}
}

func TestChartsLoadingAndErrorStates(t *testing.T) {
	api := &fakeAPI{block: make(chan struct{}), charts: chartsFixture()}
	r := newRig(t, "/charts", withAPI(api))
	r.tt.Until("the skeleton", r.tt.Shows("Charts", "Loading"))
	if r.tt.HasText("Genre") {
		t.Error("the loading page has a Genre select")
	}
	close(api.block)
	r.tt.Until("the charts", r.tt.Shows("Top songs"))

	bad := &fakeAPI{err: applemusic.ErrNotFound}
	r2 := newRig(t, "/charts", withAPI(bad))
	r2.tt.Until("the error", r2.tt.Shows("Couldn't load the charts", i18n.ErrorText(services.CodeNotFound), "Try again"))
}

func TestChartInFullListsSongsAndLoadsPagesOfFiftyAsItNearsTheEnd(t *testing.T) {
	page := chartsFixture()
	page.Charts[0].Items = songItems(0, 50)
	api := &fakeAPI{charts: page, chartItem: func(key string, offset int) services.ItemPage {
		switch offset {
		case 50:
			return services.ItemPage{Items: songItems(50, 50), Next: 100}
		case 100:
			return services.ItemPage{Items: songItems(100, 20)}
		}
		t.Errorf("chart page at %d", offset)
		return services.ItemPage{}
	}}
	r := newRig(t, "/charts/songs", withAPI(api))
	r.tt.Until("the chart", r.tt.Shows("Top songs", "Song 0", "Play Top songs", "Genre"))
	r.calls(api, "chart-items", 2)
	want := []string{"charts?", "chart-items/songs/?@50", "chart-items/songs/?@100"}
	if got := api.asked(); !slices.Equal(got, want) {
		t.Errorf("asked %v, want %v", got, want)
	}
	if n := len(r.d.chart.list.songs); n != 120 {
		t.Errorf("%d songs, want 120", n)
	}
}

func TestChartInFullOtherChartsAreAGridAndTheGenreNamesTheSubline(t *testing.T) {
	page := chartsFixture()
	api := &fakeAPI{charts: page, chartItem: func(string, int) services.ItemPage { return services.ItemPage{} }}
	r := newRig(t, "/charts/albums?genre=14", withAPI(api), withSize(1132, 900))
	r.tt.Until("the chart", r.tt.Shows("Top albums", "Pop", "Album 0"))
	if r.tt.HasText("Play Top albums") {
		t.Error("a grid chart has Play")
	}
	if got := api.asked(); got[0] != "charts?14" {
		t.Errorf("asked %v", got)
	}
}

func TestChartInFullGenreSelectKeepsTheChartID(t *testing.T) {
	api := &fakeAPI{charts: chartsFixture()}
	r := newRig(t, "/charts/cityCharts?chartId=135", withAPI(api), withSize(1132, 900))
	r.tt.Until("the chart", r.tt.Shows("Top 25 songs by city", "Genre"))
	if err := r.tt.Click("Genre"); err != nil {
		t.Fatal(err)
	}
	r.tt.Drain()
	if err := r.tt.Click("Pop"); err != nil {
		t.Fatal(err)
	}
	r.tt.Drain()
	if got := r.path(); got != "/charts/cityCharts?chartId=135&genre=14" {
		t.Errorf("the URL is %q", got)
	}
}

func TestChartInFullUnknownChartHasAnEmptyStateWithTheWayBack(t *testing.T) {
	api := &fakeAPI{charts: chartsFixture()}
	r := newRig(t, "/charts/nothing", withAPI(api))
	r.tt.Until("the empty state", r.tt.Shows("Charts", "No chart here", "Apple Music has no such chart for this genre.", "All charts"))
	if err := r.tt.Click("All charts"); err != nil {
		t.Fatal(err)
	}
	r.tt.Drain()
	if got := r.path(); got != "/charts" {
		t.Errorf("All charts went to %q", got)
	}
}

func TestChartInFullLoadingAndErrorStates(t *testing.T) {
	api := &fakeAPI{block: make(chan struct{}), charts: chartsFixture(), chartItem: func(string, int) services.ItemPage { return services.ItemPage{} }}
	r := newRig(t, "/charts/albums", withAPI(api))
	r.tt.Until("the skeleton", r.tt.Shows("Loading"))
	close(api.block)
	r.tt.Until("the chart", r.tt.Shows("Top albums"))

	bad := &fakeAPI{err: applemusic.ErrNotFound}
	r2 := newRig(t, "/charts/albums", withAPI(bad))
	r2.tt.Until("the error", r2.tt.Shows("Couldn't load this chart", "Try again"))
}

// A refetch of the same size with other songs makes the list and Play again.
func TestRoomSongsFollowARefetchOfTheSameSize(t *testing.T) {
	r, api := newRoom(t, "/room/1", services.RoomPage{Title: "Room", Kind: services.SectionTracks, Items: songItems(0, 3)}, nil)
	r.tt.Until("the room", r.tt.Shows("Song 0", "Song 2"))
	api.mu.Lock()
	api.room.Items = songItems(10, 3)
	api.mu.Unlock()
	r.env.Queries.Invalidate(keyDiscover)
	r.tt.Until("the new songs", r.tt.Shows("Song 10", "Song 12"))
	if r.tt.HasText("Song 0") {
		t.Error("the list still shows the old songs")
	}
	if err := r.tt.Click("Play Room"); err != nil {
		t.Fatal(err)
	}
	got := r.waitPlays(1)[0]
	if len(got.refs) != 3 || got.refs[0].ID != "s10" {
		t.Errorf("Play queued %+v, want the new songs", got.refs)
	}
}
