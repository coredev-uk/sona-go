package kit

import (
	"bytes"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// collTrackApp is a page with a track list in its scroll container.
type collTrackApp struct {
	songs   []*services.Song
	st      TrackListState
	pos     ui.ScrollState
	art     *ArtLoader
	opts    TrackListOpts
	played  []int
	sorted  []string
	artists []string
	current *services.QueueItem
	target  func(c *ui.Context)
	header  float32
}

func collSong(i int) *services.Song {
	id := strconv.Itoa(i)
	return &services.Song{
		ID: "s" + id, CatalogID: "c" + id, Name: "Song " + id, Artist: "Artist " + id, Album: "Album " + id,
		ArtistID: "ar" + id, AlbumID: "al" + id, DurationMs: 195000 + int64(i)*1000, Playable: true, TrackNumber: i + 1, DiscNumber: 1,
		Art: services.Art{Small: "/art/k" + id + "/s.jpg"},
	}
}

func newCollTrackApp(t *testing.T, n int, w, h int, setup ...func(*collTrackApp)) (*collTrackApp, *ui.Tester) {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	resetDrag()
	t.Cleanup(resetDrag)
	mt := newMainThread()
	a := &collTrackApp{}
	for i := range n {
		a.songs = append(a.songs, collSong(i))
	}
	a.art = newArtLoader(&fakeArt{}, mt.post, 2)
	t.Cleanup(a.art.Close)
	a.opts = TrackListOpts{
		Label:      "Songs in Test",
		Columns:    TrackColumns{Artwork: true, Artist: true, Album: true},
		OnPlay:     func(i int) { a.played = append(a.played, i) },
		Sort:       &TrackSortState{Key: SortTitle, OnSort: func(k string) { a.sorted = append(a.sorted, k) }},
		OpenArtist: func(id string) { a.artists = append(a.artists, id) },
	}
	for _, f := range setup {
		f(a)
	}
	return a, ui.NewTester(a.view, w, h)
}

func (a *collTrackApp) view(c *ui.Context) {
	theme.Apply(c)
	a.opts.Art = a.art
	a.opts.Current = a.current
	ui.Row(c).Fill().Children(func() {
		sc := ui.Scroll(c).TrackScroll(&a.pos).Grow(1).Basis(0).MinWidth(0).FillHeight().Padding(0, 44, 0, 32)
		sc.Children(func() {
			ui.Column(c).Padding(28, 0, 48, 0).Children(func() {
				ui.Box(c.Key("header")).Height(a.header)
				a.opts.Pane = &Pane{Pos: &a.pos, Bounds: func() ui.Rect { return sc.Bounds() }}
				TrackList(c, &a.st, len(a.songs), func(i int) *services.Song { return a.songs[i] }, a.opts)
			})
		})
		if a.target != nil {
			a.target(c)
		}
	})
}

func TestTrackListShowsHeaderAndRows(t *testing.T) {
	_, tt := newCollTrackApp(t, 5, 1000, 500)
	tt.Frame()
	for _, want := range []string{"Songs in Test", "Title", "Artist", "Album", "Song 0", "Artist 0", "Album 4", "3:15", "3:16"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
}

func collRowPos(t *testing.T, tt *ui.Tester, label string) (x, y float32) {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("no %q in %q", label, tt.Texts())
	}
	return r.X + 100, r.Y + r.H/2
}

func TestTrackListSelectsPlaysAndSorts(t *testing.T) {
	a, tt := newCollTrackApp(t, 20, 1000, 700)
	tt.Frame()
	x, y := collRowPos(t, tt, "Song 2")
	tt.ClickAt(x, y)
	if !a.st.Sel.Selected("s2") || a.st.Sel.Len() != 1 {
		t.Errorf("click selected %v", a.st.Sel.State().Keys())
	}
	x4, y4 := collRowPos(t, tt, "Song 4")
	tt.ClickAtWith(ui.Shift, x4, y4)
	if a.st.Sel.Len() != 3 {
		t.Errorf("shift+click selected %v", a.st.Sel.State().Keys())
	}
	tt.ClickAtWith(ui.Cmd, x, y)
	if a.st.Sel.Selected("s2") || a.st.Sel.Len() != 2 {
		t.Errorf("ctrl+click selected %v", a.st.Sel.State().Keys())
	}
	if len(a.played) != 0 {
		t.Errorf("clicks played %v", a.played)
	}
	// Double-click plays from the row.
	x1, y1 := collRowPos(t, tt, "Song 3")
	tt.ClickAt(x1, y1)
	tt.ClickAt(x1, y1)
	if len(a.played) != 1 || a.played[0] != 3 {
		t.Errorf("double-click played %v", a.played)
	}
	// Enter plays the focused row and nothing else.
	tt.Key(0, ui.KeyEnter)
	if len(a.played) != 2 || a.played[1] != 3 {
		t.Errorf("Enter played %v", a.played)
	}
	ax, _ := collRowPos(t, tt, "Artist 3")
	tt.ClickAt(ax-100+10, 28+16)
	if len(a.sorted) != 1 || a.sorted[0] != SortArtist {
		t.Errorf("sorted %v", a.sorted)
	}
}

func TestTrackListKeysMoveTheFocusAndSelect(t *testing.T) {
	a, tt := newCollTrackApp(t, 40, 1000, 700)
	tt.Frame()
	// The sort buttons come first in the Tab order, then the focused row.
	for range 4 {
		tt.Key(0, ui.KeyTab)
	}
	if !tt.Focused("Song 0") {
		t.Fatalf("Tab did not reach the first row")
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyDown)
	if !tt.Focused("Song 2") {
		t.Errorf("Down Down: focus not on Song 2")
	}
	tt.Key(ui.Shift, ui.KeyDown)
	if got := a.st.Sel.State().Keys(); len(got) != 2 {
		t.Errorf("Shift+Down selected %v", got)
	}
	tt.Key(0, ui.KeyEnd)
	if !tt.Focused("Song 39") {
		t.Errorf("End: focus not on the last row")
	}
	tt.Key(0, ui.KeyHome)
	if !tt.Focused("Song 0") {
		t.Errorf("Home: focus not on the first row")
	}
	tt.Key(0, ui.KeyPageDown)
	// A 700 DIP window: 700/44 - 2 = 13 rows.
	if !tt.Focused("Song 13") {
		t.Errorf("Page Down: focus not 13 rows down")
	}
	tt.Key(ui.Cmd, ui.KeyA)
	if a.st.Sel.Len() != 40 {
		t.Errorf("Ctrl+A selected %d", a.st.Sel.Len())
	}
	tt.Key(0, ui.KeyEscape)
	if a.st.Sel.Len() != 0 {
		t.Errorf("Escape left %d", a.st.Sel.Len())
	}
}

func TestTrackListCurrentSongTakesTheNumberAndDisabledRowsDoNothing(t *testing.T) {
	a, tt := newCollTrackApp(t, 6, 1000, 500)
	a.current = &services.QueueItem{ID: "s1"}
	a.songs[2].Playable = false
	a.songs[3].Explicit = true
	a.opts.Restricted = true
	tt.Frame()
	if slices.Contains(tt.Texts(), "2") {
		t.Errorf("the current song still shows its number: %q", tt.Texts())
	}
	if !slices.Contains(tt.Texts(), "3") {
		t.Errorf("another row lost its number: %q", tt.Texts())
	}
	for _, name := range []string{"Song 2", "Song 3"} {
		x, y := collRowPos(t, tt, name)
		tt.ClickAt(x, y)
		tt.ClickAt(x, y)
		tt.Key(0, ui.KeyEnter)
	}
	if len(a.played) != 0 {
		t.Errorf("an unplayable or withheld song played: %v", a.played)
	}
	x, y := collRowPos(t, tt, "Song 4")
	tt.ClickAt(x, y)
	tt.Key(0, ui.KeyEnter)
	if len(a.played) != 1 || a.played[0] != 4 {
		t.Errorf("Enter on a playable row: %v", a.played)
	}
}

// The playing row's number gives way to the equaliser, which is how the row
// says it is the one playing (or, with playback paused, that it is paused).
func TestTrackListCurrentRowSaysNowPlayingOrPaused(t *testing.T) {
	for _, c := range []struct {
		playing bool
		want    string
		not     string
	}{{true, "Now playing", "Paused"}, {false, "Paused", "Now playing"}} {
		a, tt := newCollTrackApp(t, 4, 1000, 400)
		a.current = &services.QueueItem{ID: "s1"}
		a.opts.Playing = c.playing
		tt.Frame()
		if _, ok := tt.Find(c.want); !ok {
			t.Errorf("playing=%v: the current row lacks %q: %q", c.playing, c.want, tt.Texts())
		}
		if _, ok := tt.Find(c.not); ok {
			t.Errorf("playing=%v: shows %q", c.playing, c.not)
		}
	}
	// No song is current: no stand-in anywhere.
	_, tt := newCollTrackApp(t, 4, 1000, 400)
	tt.Frame()
	for _, l := range []string{"Now playing", "Paused"} {
		if _, ok := tt.Find(l); ok {
			t.Errorf("a list with no current song shows %q", l)
		}
	}
}

// The playing row's glyph animates by painting itself alone: in a long list a
// frame of it builds nothing, lays nothing out and paints nothing else, so
// what a song costs while it plays does not grow with the page.
func TestTrackListPlayingGlyphAnimatesWithoutRebuildingTheList(t *testing.T) {
	a, _ := newCollTrackApp(t, 100, 1000, 700)
	a.current = &services.QueueItem{ID: "s2"}
	a.opts.Playing = true
	builds, whole := 0, 0
	tt := ui.NewTester(func(c *ui.Context) {
		builds++
		a.view(c)
		ui.Box(c.Key("page")).Absolute().Left(0).Top(0).Size(1, 1).Draw(func(*ui.Painter, ui.Rect) { whole++ })
	}, 1000, 700)
	tt.Frame()
	if _, ok := tt.Find("Now playing"); !ok {
		t.Fatalf("the current row has no glyph: %q", tt.Texts())
	}
	builds, whole = 0, 0
	prev, changes := append([]byte(nil), tt.Image().Pix...), 0
	for range 120 { // two seconds at 60 Hz
		tt.Advance(time.Second / 60)
		if cur := tt.Image().Pix; !bytes.Equal(cur, prev) {
			changes++
			prev = append(prev[:0], cur...)
		}
	}
	if builds != 0 || whole != 0 {
		t.Errorf("a playing glyph rebuilt the page %d times and painted it whole %d times in two seconds, want 0 and 0", builds, whole)
	}
	// A frame in which the glyph does not change the picture is a frame it
	// stutters: the bars' tops move by fractions of a pixel, so nearly every
	// frame shows a new picture.
	if changes < 100 {
		t.Errorf("the glyph changed the picture in %d of 120 frames in two seconds, want a smooth movement", changes)
	}
}

func TestTrackListPlayButtonAndLinks(t *testing.T) {
	a, tt := newCollTrackApp(t, 6, 1000, 500)
	tt.Frame()
	x, y := collRowPos(t, tt, "Song 2")
	tt.Move(x, y)
	tt.Frame()
	if err := tt.Click("Play Song 2"); err != nil {
		t.Fatal(err)
	}
	if len(a.played) != 1 || a.played[0] != 2 {
		t.Errorf("the play button played %v", a.played)
	}
	// An artist cell is a link: it opens the artist and does not select the row.
	ax, ay := collPos(t, tt, "Artist 4")
	a.st.Sel.Reset()
	tt.ClickAt(ax, ay)
	if len(a.artists) != 1 || a.artists[0] != "ar4" {
		t.Errorf("artist link opened %v", a.artists)
	}
	if a.st.Sel.Len() != 0 {
		t.Errorf("a link click selected %v", a.st.Sel.State().Keys())
	}
}

// A song with several artists shows Apple's artist string in its artist cell as
// a menu button, not a link: a click opens a menu of the artists, each opens
// its page, and a click with a modifier still selects the row. A song with one
// keeps its plain link.
func TestTrackListSeveralArtistsOpenAChooser(t *testing.T) {
	a, tt := newCollTrackApp(t, 6, 1000, 500, func(a *collTrackApp) {
		two := collSong(2)
		two.Artist, two.ArtistID, two.Artists = "The Kid LAROI & Justin Bieber", "1435848034", twoArtists
		a.songs[2] = two
	})
	tt.Frame()
	tr := access.Read(t, tt)
	btn, ok := tr.Find(ui.RoleMenuButton, "The Kid LAROI & Justin Bieber")
	if !ok || btn.HasPopup != ui.PopupMenu || btn.Has(access.Focusable) {
		t.Fatalf("want an unfocusable menu button with a popup in the artist cell:\n%s", tr.Dump())
	}
	if _, ok := tr.Find(ui.RoleLink, "Artist 4"); !ok {
		t.Errorf("a one-artist song lost its link:\n%s", tr.Dump())
	}
	ax, ay := collPos(t, tt, "The Kid LAROI & Justin Bieber")
	a.st.Sel.Reset()
	tt.ClickAtWith(ui.Cmd, ax, ay)
	if a.st.Sel.Len() != 1 || tt.HasText("Choose an artist") || len(a.artists) != 0 {
		t.Fatalf("a Ctrl+click: selected %v, menu %v, opened %v", a.st.Sel.State().Keys(), tt.HasText("Choose an artist"), a.artists)
	}
	a.st.Sel.Reset()
	tt.ClickAt(ax, ay)
	if a.st.Sel.Len() != 0 || len(a.artists) != 0 {
		t.Fatalf("opening the chooser selected %v or opened %v", a.st.Sel.State().Keys(), a.artists)
	}
	tr = access.Read(t, tt)
	for _, name := range []string{"The Kid LAROI", "Justin Bieber"} {
		if _, ok := tr.Find(ui.RoleMenuItem, name); !ok {
			t.Fatalf("no menu item %q:\n%s", name, tr.Dump())
		}
	}
	if err := tt.Click("Justin Bieber"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(50e6)
	tt.Frame()
	if len(a.artists) != 1 || a.artists[0] != "320569549" {
		t.Errorf("the chooser opened %v, want the second artist", a.artists)
	}
}

// A library song knows only its library artist: its row asks the catalogue
// copy and then offers every artist.
func TestTrackListOffersALibrarySongsCatalogueArtists(t *testing.T) {
	f := &fakeSongArtists{by: map[string][]services.Artist{"c1": twoArtists}}
	artists := newSongArtists(f)
	artists.wait = time.Hour // the lookup is sent when the test says, not after a delay a busy machine can overrun
	_, tt := newCollTrackApp(t, 3, 1000, 400, func(a *collTrackApp) {
		lib := collSong(1)
		lib.Source, lib.ID, lib.LibraryID = services.SourceLibrary, "i.1", "i.1"
		lib.Artist, lib.ArtistID = "The Kid LAROI & Justin Bieber", "r.9"
		a.songs[1] = lib
		a.opts.Artists = artists
	})
	tt.Frame()
	if _, ok := access.Read(t, tt).Find(ui.RoleLink, "The Kid LAROI & Justin Bieber"); !ok {
		t.Fatal("before the lookup the library artist is a link")
	}
	if q := queuedSongArtists(artists); len(q) != 1 || q[0] != "c1" {
		t.Fatalf("the row asked for %v, want the one catalogue song", q)
	}
	artists.flush()
	tt.Frame()
	if _, ok := access.Read(t, tt).Find(ui.RoleMenuButton, "The Kid LAROI & Justin Bieber"); !ok {
		t.Error("after the lookup the artists are a chooser")
	}
	if reqs := f.requests(); len(reqs) != 1 || len(reqs[0]) != 1 || reqs[0][0] != "c1" {
		t.Errorf("lookups %v, want the one catalogue song", reqs)
	}
}

func TestTrackListExtras(t *testing.T) {
	a, tt := newCollTrackApp(t, 6, 1000, 700)
	a.opts.Columns = TrackColumns{RowNumbers: true}
	a.opts.Popular = map[string]bool{"c1": true}
	a.opts.Discs = true
	a.songs[3].DiscNumber = 2
	a.songs[4].DiscNumber = 2
	a.songs[5].DiscNumber = 2
	a.opts.Heading = func(s, prev *services.Song) string {
		if s.Name == "Song 4" {
			return "Work B"
		}
		return ""
	}
	a.opts.AddedBy = func(i int) *services.SocialProfile {
		if i == 1 {
			return &services.SocialProfile{ID: "sp.1", Handle: "kim"}
		}
		return nil
	}
	tt.Frame()
	for _, want := range []string{"Disc 1", "Disc 2", "Work B", "Popular", "@kim", "Added By", "Artist 0"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	// Numbered by row, no artist column: the artist shows under the title.
	if !tt.HasText("Artist 5") {
		t.Errorf("no artist subline")
	}
}

func TestTrackListLoadingRowsAndHighlight(t *testing.T) {
	a, tt := newCollTrackApp(t, 6, 1000, 500)
	a.songs[1] = nil
	a.opts.Highlight = "s3"
	tt.Frame()
	if !tt.HasText("Loading") {
		t.Errorf("no loading row: %q", tt.Texts())
	}
	if !tt.HasText("Song 3") {
		t.Fatalf("no row 3")
	}
	// A row that is not loaded cannot be selected.
	a.st.Sel.Click(1, ClickMods{})
	if a.st.Sel.Len() != 0 {
		t.Errorf("selected a loading row: %v", a.st.Sel.State().Keys())
	}
	// The highlighted song's row is where the list's focus starts.
	if got := a.st.Nav().Focus(); got != 3 {
		t.Errorf("initial focus row %d, want 3", got)
	}
}

func TestTrackListBuildsOnlyTheRowsShowing(t *testing.T) {
	a, tt := newCollTrackApp(t, 2000, 1000, 600)
	var ranges [][2]int
	a.opts.OnRange = func(s, e int) { ranges = append(ranges, [2]int{s, e}) }
	tt.Frame()
	tt.Frame()
	built := func() int {
		n := 0
		for _, s := range tt.Texts() {
			if strings.HasPrefix(s, "Artist ") && s != "Artist" {
				n++
			}
		}
		return n
	}
	// 600 DIPs show ~13 rows; 12 more are built below.
	if got := built(); got < 13 || got > 13+12+2 {
		t.Errorf("built %d of 2000 rows", got)
	}
	if len(ranges) == 0 || ranges[0][0] != 0 {
		t.Fatalf("OnRange %v", ranges)
	}
	before := ranges[len(ranges)-1]
	tt.Scroll(500, 300, 0, 20000)
	tt.Frame()
	after := ranges[len(ranges)-1]
	if after[0] <= before[0] {
		t.Errorf("range did not follow the scroll: %v then %v", before, after)
	}
	if got := built(); got > 13+24+2 {
		t.Errorf("built %d rows after scrolling", got)
	}
}

func TestTrackListDragsTheSelection(t *testing.T) {
	a, tt := newCollTrackApp(t, 10, 1200, 600)
	var dropped []*DragPayload
	a.target = func(c *ui.Context) {
		box := ui.Box(c.Key("drop")).Width(200).FillHeight()
		DropOn(box, "queue", DropTarget{
			Describe: func(p *DragPayload) string { return "Add to queue" },
			Drop:     func(p *DragPayload, at DragPoint) { dropped = append(dropped, p) },
		})
		box.Children(func() { ui.Text(c, "Queue target") })
	}
	tt.Frame()
	for _, n := range []string{"Song 1", "Song 2", "Song 3"} {
		x, y := collRowPos(t, tt, n)
		tt.ClickAtWith(ui.Cmd, x, y)
	}
	x, y := collRowPos(t, tt, "Song 2")
	tx, ty := collPos(t, tt, "Queue target")
	tt.Press(x, y)
	tt.Move(x, y+10)
	tt.Move(x, y+20)
	tt.Move(tx, ty)
	tt.Move(tx+1, ty)
	if !tt.HasText("3 songs") || !tt.HasText("Add to queue") {
		t.Errorf("ghost: %q", tt.Texts())
	}
	tt.Release(tx+1, ty)
	if len(dropped) != 1 || len(dropped[0].Items) != 3 || dropped[0].Label != "3 songs" {
		t.Fatalf("dropped %+v", dropped)
	}
	// The row's own click handling did not run: the selection is intact.
	if a.st.Sel.Len() != 3 {
		t.Errorf("the drag changed the selection: %v", a.st.Sel.State().Keys())
	}
}

func TestTrackRowMenuActsOnTheRowOrTheSelection(t *testing.T) {
	a, tt := newCollTrackApp(t, 6, 1000, 500)
	var ran []string
	reg := NewActionRegistry(ActionDef{
		ID: "test.next", Target: KindSong, Group: GroupQueue, Order: 10,
		Label: func(*ActionContext) string { return "Play next" },
		Run:   func(x *ActionContext) { ran = append(ran, x.Item().Name()) },
	})
	a.opts.Menus = NewActionMenus(reg, &a.st.Sel)
	tt.Frame()
	x, y := collRowPos(t, tt, "Song 2")
	tt.RightClickAt(x, y)
	tt.Frame()
	if !tt.HasText("Play next") {
		t.Fatalf("no menu: %q", tt.Texts())
	}
	if !a.st.Sel.Selected("s2") || a.st.Sel.Len() != 1 {
		t.Errorf("selection %v", a.st.Sel.State().Keys())
	}
	if err := tt.Click("Play next"); err != nil {
		t.Fatal(err)
	}
	if len(ran) != 1 || ran[0] != "Song 2" {
		t.Errorf("ran %v", ran)
	}
}

func TestTrackListScrollsTheHighlightedSongToTheCentre(t *testing.T) {
	a, tt := newCollTrackApp(t, 2000, 1000, 600)
	a.opts.Highlight = "s900"
	tt.Frame()
	tt.Frame()
	tt.Frame()
	if !tt.HasText("Song 900") {
		t.Fatalf("the highlighted song was not built (scroll %v)", a.pos.Y)
	}
	r, _ := tt.Find("Song 900")
	if r.Y < 200 || r.Y > 400 {
		t.Errorf("the highlighted row is at %v in a 600 DIP window, want about the centre", r.Y)
	}
}

func TestTrackListCentresTheHighlightedSongBelowDiscHeadings(t *testing.T) {
	a, tt := newCollTrackApp(t, 60, 1000, 600, func(a *collTrackApp) {
		for i := 30; i < 60; i++ {
			a.songs[i].DiscNumber = 2
		}
		a.opts.Discs = true
		a.opts.Highlight = "s50"
	})
	tt.Frame()
	tt.Frame()
	tt.Frame()
	r, ok := tt.Find("Song 50")
	if !ok {
		t.Fatalf("the highlighted song was not built (scroll %v)", a.pos.Y)
	}
	if mid := r.Y + r.H/2; mid < 270 || mid > 330 {
		t.Errorf("the highlighted row's middle is at %v in a 600 DIP window, want the centre (300): the disc headings were ignored", mid)
	}
}

// A withheld song is disabled to assistive technology and says why as the
// row's description.
func TestTrackListWithheldRowIsDisabledAndDescribedWithTheReason(t *testing.T) {
	a, tt := newCollTrackApp(t, 6, 1000, 500)
	a.songs[3].Explicit = true
	a.opts.Restricted = true
	tt.Frame()
	tr := access.Read(t, tt)
	for _, c := range []struct{ name, want string }{
		{"Song 3", "Explicit content is restricted on this account"},
		{"Song 4", ""},
	} {
		n, ok := tr.Find(ui.RoleRow, c.name)
		if !ok {
			t.Fatalf("no row %q", c.name)
		}
		if n.Description != c.want {
			t.Errorf("%q is described %q, want %q", c.name, n.Description, c.want)
		}
		if n.Has(access.Disabled) != (c.want != "") {
			t.Errorf("%q is disabled %v, want %v", c.name, n.Has(access.Disabled), c.want != "")
		}
	}
}

func TestTrackSortToggleCyclesAscendingDescendingNone(t *testing.T) {
	var s TrackSortState
	steps := []struct {
		click string
		key   string
		desc  bool
	}{
		{SortTitle, SortTitle, false},
		{SortTitle, SortTitle, true},
		{SortTitle, "", false}, // the third click: the list's own order
		{SortTitle, SortTitle, false},
		{SortArtist, SortArtist, false}, // another column starts again, ascending
		{SortArtist, SortArtist, true},
		{SortDuration, SortDuration, false}, // from descending too
		{SortDuration, SortDuration, true},
		{SortDuration, "", false},
	}
	for i, st := range steps {
		s.Toggle(st.click)
		if s.Key != st.key || s.Desc != st.desc {
			t.Fatalf("click %d on %q: key %q desc %v, want %q %v", i+1, st.click, s.Key, s.Desc, st.key, st.desc)
		}
	}
}

func TestTrackListBlankDurationLeavesNoDashForASongWithoutLength(t *testing.T) {
	a, tt := newCollTrackApp(t, 3, 1000, 500)
	a.songs[1].DurationMs = 0
	tt.Frame()
	if !tt.HasText("—") {
		t.Fatalf("a song with no length shows no dash: %q", tt.Texts())
	}
	a.opts.BlankDuration = true
	tt.Frame()
	if tt.HasText("—") {
		t.Errorf("BlankDuration still shows a dash: %q", tt.Texts())
	}
	if !tt.HasText("3:15") {
		t.Errorf("a song with a length lost it: %q", tt.Texts())
	}
}

// rankRow reports whether the numeral shown as text sits on the row of label.
func rankRow(t *testing.T, tt *ui.Tester, text, label string) bool {
	t.Helper()
	n, ok := tt.Find(text)
	if !ok {
		t.Fatalf("no numeral %q in %q", text, tt.Texts())
	}
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("no %q in %q", label, tt.Texts())
	}
	return n.Y+n.H/2 >= r.Y && n.Y+n.H/2 <= r.Y+r.H
}

// A ranked list keeps sorting, multi-select and drag; the numerals travel
// with the songs when the host passes Of (chart positions that are neither
// the row index nor the track number).
func TestTrackListRankKeepsSortSelectAndDrag(t *testing.T) {
	var chart map[string]int
	a, tt := newCollTrackApp(t, 12, 1200, 700, func(a *collTrackApp) {
		a.opts.Rank = &TrackRank{Of: func(i int) int { return chart[a.songs[i].ID] }}
	})
	chart = map[string]int{}
	for i, s := range a.songs {
		chart[s.ID] = 41 + i
	}
	var dropped []*DragPayload
	a.target = func(c *ui.Context) {
		box := ui.Box(c.Key("drop")).Width(200).FillHeight()
		DropOn(box, "queue", DropTarget{
			Describe: func(p *DragPayload) string { return "Add to queue" },
			Drop:     func(p *DragPayload, at DragPoint) { dropped = append(dropped, p) },
		})
		box.Children(func() { ui.Text(c, "Queue target") })
	}
	tt.Frame()
	if !tt.HasText("#") || !rankRow(t, tt, "41", "Song 0") || !rankRow(t, tt, "52", "Song 11") {
		t.Errorf("the chart positions are not on their rows: %q", tt.Texts())
	}
	// Sorting: the header calls back; the host reorders and the ranks stay with the songs.
	ax, _ := collRowPos(t, tt, "Artist 3")
	tt.ClickAt(ax-100+10, 28+16)
	if len(a.sorted) != 1 || a.sorted[0] != SortArtist {
		t.Errorf("sorted %v", a.sorted)
	}
	slices.Reverse(a.songs)
	tt.Frame()
	if !rankRow(t, tt, "41", "Song 0") || !rankRow(t, tt, "52", "Song 11") {
		t.Errorf("the ranks did not follow the songs: %q", tt.Texts())
	}
	if _, y0 := collRowPos(t, tt, "Song 0"); y0 <= func() float32 { _, y := collRowPos(t, tt, "Song 11"); return y }() {
		t.Fatal("the list was not reversed")
	}
	// Shift/Ctrl selection and drag.
	x, y := collRowPos(t, tt, "Song 5")
	tt.ClickAt(x, y)
	x2, y2 := collRowPos(t, tt, "Song 3")
	tt.ClickAtWith(ui.Shift, x2, y2)
	if a.st.Sel.Len() != 3 {
		t.Errorf("shift selected %v", a.st.Sel.State().Keys())
	}
	x1, y1 := collRowPos(t, tt, "Song 8")
	tt.ClickAtWith(ui.Cmd, x1, y1)
	if a.st.Sel.Len() != 4 {
		t.Errorf("ctrl selected %v", a.st.Sel.State().Keys())
	}
	tx, ty := collPos(t, tt, "Queue target")
	tt.Press(x, y)
	tt.Move(x, y+10)
	tt.Move(x, y+20)
	tt.Move(tx, ty)
	tt.Move(tx+1, ty)
	tt.Release(tx+1, ty)
	if len(dropped) != 1 || len(dropped[0].Items) != 4 {
		t.Fatalf("dropped %+v", dropped)
	}
}

// With no Of the numerals are the row positions, so a re-sorted list is
// numbered afresh (whatever the songs' own track numbers).
func TestTrackListRankWithoutOfNumbersTheShownOrder(t *testing.T) {
	a, tt := newCollTrackApp(t, 4, 1000, 400, func(a *collTrackApp) { a.opts.Rank = &TrackRank{} })
	for _, s := range a.songs {
		s.TrackNumber = 9
	}
	tt.Frame()
	if tt.HasText("9") || !rankRow(t, tt, "4", "Song 3") {
		t.Fatalf("the numerals are not the positions: %q", tt.Texts())
	}
	slices.Reverse(a.songs)
	tt.Frame()
	if !rankRow(t, tt, "4", "Song 0") || !rankRow(t, tt, "1", "Song 3") {
		t.Errorf("a re-sorted list kept its old numerals: %q", tt.Texts())
	}
}
