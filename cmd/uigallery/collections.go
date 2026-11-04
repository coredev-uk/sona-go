package main

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// collections is the state of the Collections sections.
var collections = struct {
	size     int // index into collectionSizes
	video    bool
	grid     kit.CoverState
	gridPos  ui.ScrollState
	shelf    kit.CoverShelfState
	shelfN   int // index into shelfSizes
	tracks   kit.TrackListState
	trackPos ui.ScrollState
	trackN   int
	cols     int // 0 numbers by track, 1 by row
	artist   bool
	album    bool
	current  int
	sortKey  string
	sortDesc bool
	dropped  string
	dragGrid kit.CoverState
}{shelfN: 3, trackN: 40, artist: true, album: true, current: 2, sortKey: kit.SortTitle}

var (
	collectionSizes = []int{12, 240, 5000}
	shelfSizes      = []int{0, 4, 8, 12, 24}
)

func init() {
	register(section{name: "Collections / CoverGrid", build: collectionsGrid})
	register(section{name: "Collections / CoverShelf", build: collectionsShelf})
	register(section{name: "Collections / TrackList", build: collectionsTracks})
	register(section{name: "Collections / Drag", build: collectionsDrag})
}

// galleryAlbum is tile i: every seventh is explicit, an artist's round
// tile, an unreleased album; the rest play.
func galleryAlbum(i int, g *gallery) *kit.CoverItem {
	id := strconv.Itoa(i)
	it := kit.AlbumItem(services.Album{ID: id, Name: "Album " + id, Artist: "Artist " + id})
	ci := &kit.CoverItem{
		Key: "a" + id, Title: "Album " + id, Subtitle: "Artist " + id, Art: "/art/k" + id + "/x.jpg", Item: &it,
		Open: func() {}, Play: &kit.CoverPlay{Label: "Album " + id, Run: func() {}},
	}
	switch i % 7 {
	case 1:
		ci.Round, ci.Play = true, nil
	case 3:
		ci.Explicit = true
	case 5:
		ci.Play, ci.Badge = nil, "Out 19 Nov"
	}
	return ci
}

func collectionsGrid(c *ui.Context, g *gallery) {
	k := &collections
	note(c, "kit.CoverGrid: tiles at least 160 wide, 24 across and 28 down, exact-size art; Ctrl/Shift+click and Shift+arrows select, arrows move, drag a tile. Above 200 tiles only the rows in view are built.")
	panel(c, func() {
		knob(c, "Tiles", func() { ui.Segmented(c, &k.size, "12", "240", "5000").Label("Tiles") })
		knob(c, "Aspect", func() { ui.Checkbox(c, &k.video, "16:9 covers") })
	})
	n := collectionSizes[k.size]
	sc := ui.Scroll(c).TrackScroll(&k.gridPos).Height(560).FillWidth().Padding(28, 32, 24, 32)
	sc.Children(func() {
		kit.CoverGrid(c, &k.grid, n, func(i int) *kit.CoverItem { return galleryAlbum(i, g) }, kit.CoverGridOpts{
			CoverOpts: kit.CoverOpts{Label: "Albums", Art: g.art, Video: k.video},
			Pane:      &kit.Pane{Pos: &k.gridPos, Bounds: func() ui.Rect { return sc.Bounds() }},
		})
	})
}

func collectionsShelf(c *ui.Context, g *gallery) {
	k := &collections
	note(c, "kit.CoverShelf: one row that fits the width, \"See all\" to the section's page, else \"Show all\" in place; nothing for an empty list.")
	panel(c, func() {
		knob(c, "Items", func() { ui.Segmented(c, &k.shelfN, "0", "4", "8", "12", "24").Label("Items") })
	})
	items := make([]kit.CoverItem, shelfSizes[k.shelfN])
	for i := range items {
		items[i] = *galleryAlbum(i, g)
	}
	ui.Column(c).Padding(24, 32).Children(func() {
		kit.CoverShelf(c, &k.shelf, "Recently Added", items, kit.CoverShelfOpts{
			CoverOpts: kit.CoverOpts{Art: g.art}, First: true,
		})
		kit.CoverShelf(c, &collectionsMore, "Made for you", items[:min(len(items), 8)], kit.CoverShelfOpts{
			CoverOpts: kit.CoverOpts{Art: g.art}, More: func() {},
		})
	})
}

var collectionsMore kit.CoverShelfState

func collectionsTracks(c *ui.Context, g *gallery) {
	k := &collections
	note(c, "kit.TrackList: 44 DIP rows, sortable headers, Ctrl/Shift+click and arrows select, double-click or Enter plays, right-click for the row's menu. Above 200 rows only the rows in view are built.")
	panel(c, func() {
		knob(c, "Rows", func() { ui.Segmented(c, &k.size, "12", "240", "5000").Label("Rows") })
		knob(c, "Numbers", func() { ui.Segmented(c, &k.cols, "Album track", "Row").Label("Numbers") })
		knob(c, "Columns", func() {
			ui.Checkbox(c, &k.artist, "Artist")
			ui.Checkbox(c, &k.album, "Album")
		})
	})
	n := collectionSizes[k.size]
	song := func(i int) *services.Song {
		id := strconv.Itoa(i)
		return &services.Song{ID: "s" + id, Name: "Song " + id, Artist: "Artist " + id, Album: "Album " + id, ArtistID: "a", AlbumID: "b",
			DurationMs: 195000 + int64(i)*1000, Playable: i%11 != 4, Explicit: i%5 == 2, TrackNumber: i + 1, DiscNumber: 1,
			Art: services.Art{Small: "/art/k" + id + "/s.jpg"}}
	}
	sc := ui.Scroll(c).TrackScroll(&k.trackPos).Height(520).FillWidth().Padding(12, 32, 24, 32)
	sc.Children(func() {
		kit.TrackList(c, &k.tracks, n, song, kit.TrackListOpts{
			Label:   "Songs",
			Columns: kit.TrackColumns{RowNumbers: k.cols == 1, Artwork: true, Artist: k.artist, Album: k.album},
			Sort: &kit.TrackSortState{Key: k.sortKey, Desc: k.sortDesc, OnSort: func(key string) {
				k.sortDesc = k.sortKey == key && !k.sortDesc
				k.sortKey = key
			}},
			OnPlay:  func(i int) { k.current = i },
			Current: func(s *services.Song) bool { return s.ID == "s"+strconv.Itoa(k.current) },
			Playing: true,
			Art:     g.art,
			Pane:    &kit.Pane{Pos: &k.trackPos, Bounds: func() ui.Rect { return sc.Bounds() }},
		})
	})
}

func collectionsDrag(c *ui.Context, g *gallery) {
	k := &collections
	note(c, "The drag engine: a press that travels 6 DIPs starts a drag; the ghost card says what a drop does; the innermost target that accepts takes it, and a target that declines lets the one around it have it.")
	text(c, "Last drop: "+k.dropped, "secondary").TextColor(theme.C("ink-muted"))
	ui.Row(c).Gap(24).Padding(16, 0).AlignItems(ui.Start).Children(func() {
		ui.Box(c).Width(520).Children(func() {
			kit.CoverGrid(c, &k.dragGrid, 6, func(i int) *kit.CoverItem { return galleryAlbum(i, g) }, kit.CoverGridOpts{
				CoverOpts: kit.CoverOpts{Label: "Albums", Art: g.art, Min: 140},
			})
		})
		outer := ui.Column(c.Key("queue")).Width(260).Height(260).Padding(24).Gap(16).Radius(theme.Current().Radius["lg"]).Background(theme.C("surface"))
		if kit.DropOn(outer, "queue", kit.DropTarget{
			Describe: func(p *kit.DragPayload) string { return "Add to the queue" },
			Drop:     func(p *kit.DragPayload, at kit.DragPoint) { k.dropped = "queue: " + p.Label },
		}) {
			outer.Border(2, theme.C("ink"))
		}
		outer.Children(func() {
			text(c, "Queue", "heading").TextColor(theme.C("ink"))
			road := ui.Column(c.Key("road")).Padding(16).Radius(theme.Current().Radius["md"]).Background(theme.C("raised"))
			if kit.DropOn(road, "road", kit.DropTarget{
				Describe: func(p *kit.DragPayload) string { return "Add to “Road trip”" },
				Drop:     func(p *kit.DragPayload, at kit.DragPoint) { k.dropped = "Road trip: " + p.Label },
			}) {
				road.Border(2, theme.C("amber-ink"))
			}
			road.Children(func() { text(c, "Road trip", "body").TextColor(theme.C("ink")) })
			locked := ui.Column(c.Key("locked")).Padding(16).Radius(theme.Current().Radius["md"]).Background(theme.C("raised"))
			kit.DropOn(locked, "locked", kit.DropTarget{Accepts: func(*kit.DragPayload) bool { return false }})
			locked.Children(func() { text(c, "Declines: the queue takes it", "body").TextColor(theme.C("ink-muted")) })
		})
	})
}
