package kit

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

func trackItems(n int) []services.Item {
	var items []services.Item
	for i := 1; i <= n; i++ {
		items = append(items, songItem(strconv.Itoa(i), "Song "+strconv.Itoa(i), "Artist "+strconv.Itoa(i), false))
	}
	return items
}

// trackApp is a track shelf titled "Top Songs" on a 640 wide page: two columns
// of 280 or more.
func trackApp(t *testing.T, items []services.Item, more string, tweak func(*ItemsCtx)) (*itemsApp, *ui.Tester) {
	t.Helper()
	st := &TrackShelfState{}
	a, tt := newItemsApp(t, 640, 500, nil, func(c *ui.Context, ctx ItemsCtx) {
		TrackShelf(c, st, items, ctx, TrackShelfOpts{Title: "Top Songs", First: true, More: more})
	})
	a.tweak = tweak
	a.load(t, tt, 0)
	return a, tt
}

func TestTrackShelfFillsDownEachColumnAsManyColumnsAsFit(t *testing.T) {
	_, tt := trackApp(t, trackItems(10), "", nil)
	pos := func(n int) ui.Rect {
		r, ok := tt.Find("Song " + strconv.Itoa(n))
		if !ok {
			t.Fatalf("no Song %d: %q", n, tt.Texts())
		}
		return r
	}
	// 4 to a column, two columns: songs 1-4 down the first, 5-8 down the second.
	for n := 1; n < 4; n++ {
		if pos(n).X != pos(n+1).X || pos(n+1).Y-pos(n).Y != trackRowH {
			t.Errorf("Song %d %v and Song %d %v are not stacked %d apart", n, pos(n), n+1, pos(n+1), trackRowH)
		}
	}
	if pos(5).X <= pos(1).X || pos(5).Y != pos(1).Y {
		t.Errorf("Song 5 %v is not at the top of the second column (Song 1 %v)", pos(5), pos(1))
	}
	if tt.HasText("Song 9") {
		t.Errorf("a third column's worth of songs shows: %q", tt.Texts())
	}
	// The list is named by the shelf's title.
	if n := countText(tt, "Top Songs"); n < 2 {
		t.Errorf("the title and the list's name: %d × Top Songs", n)
	}
}

func TestTrackShelfRowsAreFocusableAndEnterPlaysFromTheSongWithTheShelfAsTheQueue(t *testing.T) {
	a, tt := trackApp(t, trackItems(10), "", nil)
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("Song 1") {
		t.Fatalf("Tab did not focus the first row")
	}
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("Song 2") {
		t.Fatalf("the second Tab did not focus the second row (the cover button or the artist link took it)")
	}
	tt.Key(0, ui.KeyEnter)
	// The queue is every song of the shelf, not just those showing.
	if !slices.Equal(a.rec.songs, []string{"Top Songs@Song 2/Song 1..Song 10"}) {
		t.Errorf("played %v", a.rec.songs)
	}
}

func TestTrackShelfDoubleClickAndTheCoversPlayButtonPlayFromThatSong(t *testing.T) {
	a, tt := trackApp(t, trackItems(10), "", nil)
	r, _ := tt.Find("Song 3")
	x, y := r.X+60, r.Y // on the row's text, clear of the cover and the artist
	tt.ClickAt(x, y)
	tt.ClickAt(x, y)
	if !slices.Equal(a.rec.songs, []string{"Top Songs@Song 3/Song 1..Song 10"}) {
		t.Errorf("double click played %v", a.rec.songs)
	}
	if err := tt.Click("Play Song 6"); err != nil {
		t.Fatal(err)
	}
	if len(a.rec.songs) != 2 || a.rec.songs[1] != "Top Songs@Song 6/Song 1..Song 10" {
		t.Errorf("the cover's play button played %v", a.rec.songs)
	}
}

func TestTrackShelfUnplayableAndWithheldSongsAreDimmedAndDoNothing(t *testing.T) {
	items := trackItems(3)
	items[0].Song.Playable = false
	items[1] = songItem("2", "Song 2", "Artist 2", true)
	a, tt := trackApp(t, items, "", func(ctx *ItemsCtx) { ctx.Restricted = true })
	for _, n := range []string{"Play Song 1", "Play Song 2"} {
		if _, ok := tt.Find(n); ok {
			t.Errorf("%q: a dimmed song has a play button", n)
		}
	}
	if _, ok := tt.Find("Play Song 3"); !ok {
		t.Error("a playable song has no play button")
	}
	// Still reachable by keyboard, as the React rows are (tabIndex 0), and
	// Enter does nothing there.
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("Song 1") {
		t.Error("an unplayable row is not focusable")
	}
	tt.Key(0, ui.KeyEnter)
	for _, n := range []string{"Song 1", "Song 2"} {
		r, _ := tt.Find(n)
		tt.ClickAt(r.X+60, r.Y)
		tt.ClickAt(r.X+60, r.Y)
	}
	if len(a.rec.songs) != 0 {
		t.Errorf("a dimmed song played %v", a.rec.songs)
	}
}

func TestTrackShelfTheCurrentSongShowsTheEqualiserInsteadOfThePlayButton(t *testing.T) {
	_, tt := trackApp(t, trackItems(4), "", func(ctx *ItemsCtx) {
		ctx.Current = &services.QueueItem{ID: "2"}
		ctx.Playing = true
	})
	if _, ok := tt.Find("Play Song 2"); ok {
		t.Error("the current song has a play button")
	}
	if _, ok := tt.Find("Play Song 3"); !ok {
		t.Error("another song has no play button")
	}
}

func TestTrackShelfRowShowsTheExplicitMarkTheArtistLinkAndTheSongMenu(t *testing.T) {
	items := trackItems(3)
	items[1] = songItem("2", "Song 2", "Artist 2", true)
	items[2].Song.ArtistID = "" // an artist with no page is plain text
	a, tt := trackApp(t, items, "", nil)
	if _, ok := tt.Find("Explicit"); !ok {
		t.Errorf("no explicit mark: %q", tt.Texts())
	}
	if _, ok := tt.Find("More for Song 1"); !ok {
		t.Error("no song menu")
	}
	r, _ := tt.Find("Artist 1")
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	if !slices.Equal(a.rec.nav, []string{"/artist/ar1"}) || len(a.rec.songs) != 0 {
		t.Errorf("the artist link navigated %v, played %v", a.rec.nav, a.rec.songs)
	}
	r, _ = tt.Find("Artist 3")
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	if len(a.rec.nav) != 1 {
		t.Errorf("a plain artist navigated %v", a.rec.nav)
	}
	// None of them takes the Tab key: it goes from row to row.
	for _, row := range []string{"Song 1", "Song 2", "Song 3"} {
		tt.Key(0, ui.KeyTab)
		if !tt.Focused(row) {
			t.Fatalf("Tab did not reach %q: a control inside a row took a tab stop", row)
		}
	}
}

func TestTrackShelfSeeAllNavigatesToTheSectionsRoom(t *testing.T) {
	a, tt := trackApp(t, trackItems(3), "/room/9", nil)
	if err := tt.Click("See all"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.rec.nav, []string{"/room/9"}) {
		t.Errorf("nav %v", a.rec.nav)
	}
}

func TestTrackShelfWithheldSongExplainsWhyOnHover(t *testing.T) {
	items := trackItems(2)
	items[1] = songItem("2", "Song 2", "Artist 2", true)
	_, tt := trackApp(t, items, "", func(ctx *ItemsCtx) { ctx.Restricted = true })
	const reason = "Explicit content is restricted on this account"
	if tt.HasText(reason) {
		t.Error("the tip shows before the pointer rests on the row")
	}
	r, _ := tt.Find("Song 2")
	tt.Move(r.X+60, r.Y)
	tt.Advance(2 * time.Second)
	if !tt.HasText(reason) {
		t.Errorf("no tooltip %q: %q", reason, tt.Texts())
	}
	r, _ = tt.Find("Song 1")
	tt.Move(r.X+60, r.Y)
	tt.Advance(2 * time.Second)
	if tt.HasText(reason) {
		t.Error("a playable song shows the reason")
	}
}

func TestTrackShelfWithheldSongIsDescribedWithTheReason(t *testing.T) {
	items := trackItems(2)
	items[1] = songItem("2", "Song 2", "Artist 2", true)
	_, tt := trackApp(t, items, "", func(ctx *ItemsCtx) { ctx.Restricted = true })
	tr := access.Read(t, tt)
	for name, want := range map[string]string{"Song 2": "Explicit content is restricted on this account", "Song 1": ""} {
		n, ok := tr.Find(ui.RoleListItem, name)
		if !ok {
			t.Fatalf("no list item %q", name)
		}
		if n.Description != want {
			t.Errorf("%q is described %q, want %q", name, n.Description, want)
		}
	}
}
