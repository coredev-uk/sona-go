package player

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// queueRig is the queue panel over a fake transport: beside a page with the
// bar's Queue toggle and a song to drag, or, with inView, as Now Playing's
// Up next list.
type queueRig struct {
	p    *player
	tt   *uitest.Session
	fake *fakeTransport
	h    *uitest.Harness
	nav  *navRecorder
}

func newQueueRig(t *testing.T, inView bool) *queueRig {
	t.Helper()
	h := uitest.New(t, uitest.Bare())
	r := &queueRig{fake: newFake(), h: h, nav: &navRecorder{}}
	h.Env.Nav = r.nav
	r.p = newPlayer(h.Env, r.fake, func() bool { return true })
	r.p.now = h.Clock.Now
	r.p.ui.queueOpen = true
	song := services.Song{ID: "i.99", Source: "catalog", Name: "Source song", Artist: "Somebody", Playable: true}
	r.tt = h.Tester(t, func(c *ui.Context) {
		r.p.toasts(c)
		ui.Row(c).Fill().Children(func() {
			ui.Column(c.Key("page")).Grow(1).Basis(0).Padding(16).Gap(12).Children(func() {
				kit.Button(c.Key("toggle"), "", kit.ButtonOpts{Variant: kit.Ghost, Icon: "list-music", A11yLabel: "Toggle queue"}).Bind(&r.p.queue.toggle)
				kit.DragItem(c, kit.DragOpts{
					Key: "ext", Row: true,
					Payload: func() *kit.DragPayload {
						return &kit.DragPayload{Items: []kit.Item{kit.SongItem(song)}, Label: song.Name}
					},
					Outer: func(w ui.Element) { w.Size(160, 40) },
				}, func(w, s ui.Element) { ui.Text(c, "Source song") })
			})
			if inView {
				ui.Column(c.Key("view")).Width(320).FillHeight().Children(func() {
					r.p.queueSummary(c, nil)
					r.p.queueList(c, true)
				})
				return
			}
			r.p.queueAside(c)
		})
	}, 1000, 700)
	return r
}

// seed shows a queue of n songs ("Song 1", "Artist 1"…) with the one at index
// playing; mutate may change the state and the queue before it shows.
func (r *queueRig) seed(n, index int, mutate ...func(*services.PlaybackState, *services.QueueState)) {
	items := make([]services.QueueItem, n)
	for i := range items {
		k := i + 1
		items[i] = services.QueueItem{
			UID: uint64(k), ID: fmt.Sprint("i.", k), Source: "catalog", Name: fmt.Sprint("Song ", k),
			Artist: fmt.Sprint("Artist ", k), DurationMs: 180_000,
		}
	}
	s := store.EmptyState()
	s.State, s.Index = store.StatePlaying, index
	if index >= 0 && index < n {
		s.Current = &items[index]
	}
	s.DurationMs, s.At = 180_000, r.h.Clock.Now().UnixMilli()
	s.Controls = services.PlaybackControls{Previous: true, Next: true, Seek: true, Shuffle: true, Repeat: true, AutoPlay: true, Queue: true}
	s.Repeat = "off"
	q := services.QueueState{Index: index, Items: items}
	for _, m := range mutate {
		m(&s, &q)
	}
	r.h.Env.Play.Seed(s, q)
	r.tt.Frame()
	r.tt.Frame()
}

func (r *queueRig) has(label string) bool {
	_, ok := r.tt.Find(label)
	return ok
}

// at is a point on the row (or element) named label, clear of the buttons
// that show at its right while it is hot.
func (r *queueRig) at(t *testing.T, label string) (x, y float32) {
	t.Helper()
	b, ok := r.tt.Find(label)
	if !ok {
		t.Fatalf("no %q in %q", label, r.tt.Texts())
	}
	return b.X + 60, b.Y + b.H/2
}

// source is a point on the song to drag from a page.
func (r *queueRig) source(t *testing.T) (x, y float32) {
	t.Helper()
	b, ok := r.tt.Find("Source song")
	if !ok {
		t.Fatalf("no source song in %q", r.tt.Texts())
	}
	return b.X + 5, b.Y + b.H/2
}

// focusRow clicks a row and waits for the focus to settle on it.
func (r *queueRig) focusRow(t *testing.T, label string) {
	t.Helper()
	x, y := r.at(t, label)
	r.tt.ClickAt(x, y)
	r.tt.Frame()
	r.tt.Frame()
	if !r.tt.Focused(label) {
		t.Fatalf("%q has no focus after a click", label)
	}
}

// dragTo drags from one point to another, the way the kit's own tests do.
func (r *queueRig) dragTo(fx, fy, tx, ty float32) {
	r.tt.Press(fx, fy)
	r.tt.Move(fx, fy+10)
	r.tt.Move(fx, fy+20)
	r.tt.Move(tx, ty)
	r.tt.Move(tx+1, ty)
	r.tt.Release(tx+1, ty)
}

// autoPlayFrom makes songs from..end the station's: AutoPlay's, under its heading.
func autoPlayFrom(from int) func(*services.PlaybackState, *services.QueueState) {
	return func(s *services.PlaybackState, q *services.QueueState) {
		for i := from; i < len(q.Items); i++ {
			q.Items[i].Auto = true
		}
		s.AutoPlay = true
		q.Station = &services.QueueStation{ID: "st.1", Name: "Chill", AutoPlay: true}
	}
}

func TestAsideBuildsOnlyWhileTheQueueIsOpenAndNothingCoversThePage(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(3, 0)
	for _, w := range []string{"Queue", "Close queue", "AutoPlay"} {
		if !r.has(w) {
			t.Errorf("an open queue lacks %q: %q", w, r.tt.Texts())
		}
	}
	r.p.ui.queueOpen = false
	r.tt.Frame()
	if r.has("Close queue") || r.has("Song 2, Artist 2") {
		t.Errorf("a closed queue builds: %q", r.tt.Texts())
	}
	r.p.ui.queueOpen = true
	r.p.ui.overlay = overlayNowPlaying
	r.tt.Frame()
	if r.has("Close queue") {
		t.Errorf("the queue shows under Now Playing: %q", r.tt.Texts())
	}
	r.p.ui.overlay = overlayNone
	r.tt.Frame()
	if err := r.tt.Click("Close queue"); err != nil {
		t.Fatal(err)
	}
	r.tt.Frame()
	if r.p.ui.queueOpen || r.has("Close queue") {
		t.Error("Close queue left the queue open")
	}
}

func TestRowsListFromTheCurrentSongOnward(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(6, 2)
	for _, l := range []string{"Song 3, Artist 3", "Up next: Song 4, Artist 4", "Song 5, Artist 5", "Song 6, Artist 6"} {
		if !r.has(l) {
			t.Errorf("no row %q in %q", l, r.tt.Texts())
		}
	}
	for _, l := range []string{"Song 1, Artist 1", "Song 2, Artist 2", "Up next: Song 5, Artist 5"} {
		if r.has(l) {
			t.Errorf("row %q shows", l)
		}
	}
	if !r.has("Now playing") {
		t.Errorf("the current song has no equaliser: %q", r.tt.Texts())
	}
	if !r.tt.HasText("3 songs after this one") || !r.tt.HasText("3:00") {
		t.Errorf("summary or duration missing: %q", r.tt.Texts())
	}
}

func TestEmptyQueueSaysHowToStartOne(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(0, -1)
	for _, w := range []string{"The queue is empty", "Play an album, playlist or station, choose Play next or Play last on any song, or drag songs here."} {
		if !r.tt.HasText(w) {
			t.Errorf("empty queue lacks %q: %q", w, r.tt.Texts())
		}
	}
	if r.has("Clear") {
		t.Error("Clear with nothing to clear")
	}
	// A drop on an empty queue appends.
	x, y := r.source(t)
	b, _ := r.tt.Find("The queue is empty")
	r.dragTo(x, y, b.X+20, b.Y+5)
	r.fake.wait(t, 1)
	if got := r.fake.snapshot(); !slices.Equal(got, []string{"append"}) {
		t.Errorf("calls %v", got)
	}
}

func TestSummaryStrings(t *testing.T) {
	run := func(n, index int, mutate ...func(*services.PlaybackState, *services.QueueState)) string {
		s, q := services.PlaybackState{Index: index, Repeat: "off"}, services.QueueState{Index: index}
		for i := range n {
			q.Items = append(q.Items, services.QueueItem{UID: uint64(i + 1), ID: fmt.Sprint("i.", i)})
		}
		for _, m := range mutate {
			m(&s, &q)
		}
		return summaryOf(s, q)
	}
	live := func(s *services.PlaybackState, q *services.QueueState) { q.Items[0].Live = true }
	for name, tc := range map[string]struct {
		got, want string
	}{
		"empty":            {run(0, -1), ""},
		"live radio":       {run(1, 0, live), "Live radio"},
		"one song":         {run(2, 0), "1 song after this one"},
		"songs":            {run(5, 1), "3 songs after this one"},
		"songs and auto":   {run(5, 0, autoPlayFrom(3)), "2 songs after this one, then AutoPlay"},
		"only AutoPlay":    {run(3, 0, autoPlayFrom(1)), "AutoPlay is on"},
		"nothing after":    {run(1, 0), "Nothing after this song"},
		"last of a played": {run(4, 3), "Nothing after this song"},
		"station only": {run(3, 0, func(s *services.PlaybackState, q *services.QueueState) {
			q.Items[1].Auto, q.Items[2].Auto = true, true
			q.Station = &services.QueueStation{ID: "st", Name: "Chill"}
		}), "Nothing queued after this song"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: %q, want %q", name, tc.got, tc.want)
		}
	}
}

func TestStationPartHasItsHeadingAndTheStationLinksWhereTheRouteExists(t *testing.T) {
	r := newQueueRig(t, false)
	r.h.Env.HasRoute = func(path string) bool { return path == "/station/st.1" }
	r.seed(5, 0, func(s *services.PlaybackState, q *services.QueueState) {
		q.Items[3].Auto, q.Items[4].Auto = true, true
		q.Station = &services.QueueStation{ID: "st.1", Name: "Chill"}
	})
	for _, w := range []string{"Then Chill", "The station continues after your songs", "Playing from ", "Chill"} {
		if !r.tt.HasText(w) {
			t.Errorf("lacks %q: %q", w, r.tt.Texts())
		}
	}
	if err := r.tt.Click("Chill"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.nav.pushed, []string{"/station/st.1"}) {
		t.Errorf("the station link pushed %v", r.nav.pushed)
	}

	// Without the page the name is plain text.
	r.h.Env.HasRoute = func(string) bool { return false }
	r.nav.pushed = nil
	r.tt.Frame()
	_ = r.tt.Click("Chill")
	if len(r.nav.pushed) != 0 {
		t.Errorf("a link to a page that does not exist pushed %v", r.nav.pushed)
	}
}

func TestAutoPlayHeadingAndNote(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(5, 0, autoPlayFrom(3))
	for _, w := range []string{"Similar music will keep playing", "2 songs after this one, then AutoPlay"} {
		if !r.tt.HasText(w) {
			t.Errorf("lacks %q: %q", w, r.tt.Texts())
		}
	}
	r2 := newQueueRig(t, false)
	r2.seed(3, 0, func(s *services.PlaybackState, q *services.QueueState) { s.AutoPlay = true })
	if !r2.tt.HasText("Then AutoPlay continues with similar music") {
		t.Errorf("no note while AutoPlay is on its way: %q", r2.tt.Texts())
	}
}

func TestRowsDoubleClickPlayAndKeysAct(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(6, 0)
	x, y := r.at(t, "Up next: Song 2, Artist 2")
	r.tt.ClickAt(x, y)
	r.tt.ClickAt(x, y)
	if got := r.fake.wait(t, 1); got[len(got)-1] != "jump 1" {
		t.Fatalf("a double click called %v", got)
	}

	r = newQueueRig(t, false)
	r.seed(6, 0)
	r.focusRow(t, "Song 3, Artist 3")
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Key(0, ui.KeyDelete)
	r.tt.Key(ui.Alt, ui.KeyDown)
	r.tt.Key(ui.Alt, ui.KeyUp)
	// Each key's call runs on its own goroutine, so they reach the fake in
	// any order.
	want := []string{"jump 2", "move 2 1", "move 2 3", "remove 2"}
	got := slices.Clone(r.fake.wait(t, len(want)))
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("keys called %v, want %v (in any order)", got, want)
	}
}

func TestMoveKeysStayInTheUsersPart(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(5, 0, autoPlayFrom(3)) // user's songs: qi 1, 2; AutoPlay's: 3, 4
	r.focusRow(t, "Song 3, Artist 3")
	r.tt.Key(ui.Alt, ui.KeyDown) // into AutoPlay's part: refused
	r.tt.Frame()
	if got := r.fake.snapshot(); len(got) != 0 {
		t.Errorf("a move past the user's part called %v", got)
	}
	r.focusRow(t, "Song 4, Artist 4")
	r.tt.Key(ui.Alt, ui.KeyUp) // AutoPlay's songs keep their place
	r.tt.Frame()
	if got := r.fake.snapshot(); len(got) != 0 {
		t.Errorf("an AutoPlay song moved: %v", got)
	}
}

func TestArrowsMoveTheListsOneTabStop(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(5, 0)
	r.focusRow(t, "Up next: Song 2, Artist 2")
	r.tt.Key(0, ui.KeyDown)
	r.tt.Frame()
	r.tt.Frame()
	if !r.tt.Focused("Song 3, Artist 3") {
		t.Errorf("Down did not move to the next row")
	}
	r.tt.Key(0, ui.KeyEnd)
	r.tt.Frame()
	r.tt.Frame()
	if !r.tt.Focused("Song 5, Artist 5") {
		t.Errorf("End did not reach the last row")
	}
	r.tt.Key(0, ui.KeyHome)
	r.tt.Frame()
	r.tt.Frame()
	if !r.tt.Focused("Song 1, Artist 1") {
		t.Errorf("Home did not reach the current song")
	}
}

func TestFocusGoesToTheRowThatTakesTheRemovedRowsPlace(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(5, 0)
	r.focusRow(t, "Song 3, Artist 3")
	// Go removes it: the queue now has Song 4 where it was.
	items := slices.Delete(slices.Clone(r.h.Env.Play.Queue().Items), 2, 3)
	s := r.h.Env.Play.State()
	r.h.Env.Play.Seed(s, services.QueueState{Index: 0, Items: items})
	for range 4 {
		r.tt.Frame()
	}
	if !r.tt.Focused("Song 4, Artist 4") {
		t.Errorf("focus was not handed to the row in its place")
	}
}

func TestAMoveIsAnnouncedWhenItLandsAndKeepsFocus(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(5, 0)
	r.focusRow(t, "Song 3, Artist 3")
	r.tt.Key(ui.Alt, ui.KeyDown)
	r.fake.wait(t, 1)
	if len(r.tt.Announcements()) != 0 {
		t.Error("announced before it landed")
	}
	// Go's snapshot with the two songs swapped.
	items := slices.Clone(r.h.Env.Play.Queue().Items)
	items[2], items[3] = items[3], items[2]
	r.h.Env.Play.Seed(r.h.Env.Play.State(), services.QueueState{Index: 0, Items: items})
	var heard []string
	for range 4 {
		r.tt.Frame()
		heard = append(heard, r.tt.Announcements()...)
	}
	if !slices.Contains(heard, "Song 3: moved to position 3 of 4") {
		t.Errorf("announcements %q", heard)
	}
	if !r.tt.Focused("Song 3, Artist 3") {
		t.Error("the moved row lost the focus")
	}
}

func TestClearAndAutoPlayCallTheService(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(4, 0)
	if err := r.tt.Click("Clear"); err != nil {
		t.Fatal(err)
	}
	if err := r.tt.Click("AutoPlay"); err != nil {
		t.Fatal(err)
	}
	if got := r.fake.wait(t, 2); !slices.Equal(got, []string{"clear", "autoplay on"}) {
		t.Fatalf("calls %v", got)
	}
	// The mirror shows it at once.
	if !r.h.Env.Play.State().AutoPlay {
		t.Error("AutoPlay not patched into the mirror")
	}
	r.tt.Frame()
	_ = r.tt.Click("AutoPlay")
	if got := r.fake.wait(t, 1); got[len(got)-1] != "autoplay off" {
		t.Errorf("calls %v", got)
	}
}

func TestAutoPlayDoesNothingWhereItDoesNotApply(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(3, 0, func(s *services.PlaybackState, q *services.QueueState) {
		q.Items[0].Live = true
		s.Controls = services.PlaybackControls{}
	})
	_ = r.tt.Click("AutoPlay")
	r.tt.Frame()
	if got := r.fake.snapshot(); len(got) != 0 {
		t.Errorf("AutoPlay on a live station called %v", got)
	}
	if r.h.Env.Play.State().AutoPlay {
		t.Error("the mirror turned AutoPlay on")
	}
}

func TestRowMenuOffersWhatChangesTheOrder(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(6, 0)
	x, y := r.at(t, "Song 4, Artist 4")
	r.tt.RightClickAt(x, y)
	r.tt.Frame()
	for _, w := range []string{"Play", "Play next", "Move to end", "Remove"} {
		if !r.tt.HasText(w) {
			t.Fatalf("the menu lacks %q: %q", w, r.tt.Texts())
		}
	}
	if err := r.tt.Click("Move to end"); err != nil {
		t.Fatal(err)
	}
	if got := r.fake.wait(t, 1); got[0] != "movetoend 3" {
		t.Errorf("calls %v", got)
	}

	// The next song needs no Play next; the last, no Move to end.
	x, y = r.at(t, "Up next: Song 2, Artist 2")
	r.tt.RightClickAt(x, y)
	r.tt.Frame()
	if r.tt.HasText("Play next") || !r.tt.HasText("Move to end") {
		t.Errorf("menu of the next song: %q", r.tt.Texts())
	}
	if err := r.tt.Click("Remove"); err != nil {
		t.Fatal(err)
	}
	if got := r.fake.wait(t, 1); got[len(got)-1] != "remove 1" {
		t.Errorf("calls %v", got)
	}
}

func TestRowMenuPlayAndPlayNextAndTheMoreButton(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(6, 0)
	x, y := r.at(t, "Song 4, Artist 4")
	r.tt.Move(x, y)
	r.tt.Frame()
	if !r.has("Remove Song 4 from the queue") {
		t.Fatalf("no row buttons while hot: %q", r.tt.Texts())
	}
	if err := r.tt.Click("More for Song 4"); err != nil {
		t.Fatal(err)
	}
	r.tt.Frame()
	if err := r.tt.Click("Play next"); err != nil {
		t.Fatalf("%v: %q", err, r.tt.Texts())
	}
	if got := r.fake.wait(t, 1); got[0] != "movenext 3" {
		t.Errorf("calls %v", got)
	}
	r.tt.Frame()
	x, y = r.at(t, "Song 5, Artist 5")
	r.tt.RightClickAt(x, y)
	r.tt.Frame()
	if err := r.tt.Click("Play"); err != nil {
		t.Fatal(err)
	}
	if got := r.fake.wait(t, 1); got[len(got)-1] != "jump 4" {
		t.Errorf("calls %v", got)
	}
	// The row's remove button.
	r.tt.Frame()
	x, y = r.at(t, "Song 6, Artist 6")
	r.tt.Move(x, y)
	r.tt.Frame()
	if err := r.tt.Click("Remove Song 6 from the queue"); err != nil {
		t.Fatal(err)
	}
	if got := r.fake.wait(t, 1); got[len(got)-1] != "remove 5" {
		t.Errorf("calls %v", got)
	}
}

func TestThePlayingRowHasNoMenu(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(3, 0)
	x, y := r.at(t, "Song 1, Artist 1")
	r.tt.RightClickAt(x, y)
	r.tt.Frame()
	if r.tt.HasText("Move to end") || r.tt.HasText("Remove") {
		t.Errorf("the playing song has a menu: %q", r.tt.Texts())
	}
}

func TestDraggingARowReordersTheQueue(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(6, 0)
	fx, fy := r.at(t, "Up next: Song 2, Artist 2")
	b, ok := r.tt.Find("Song 4, Artist 4")
	if !ok {
		t.Fatal("no Song 4")
	}
	// The lower half of Song 4: the gap after it, so Song 2 lands at 3.
	r.dragTo(fx, fy, fx, b.Y+b.H-8)
	if got := r.fake.wait(t, 1); got[0] != "move 1 3" {
		t.Errorf("calls %v", got)
	}

	// The gap right under its own row moves nothing.
	r = newQueueRig(t, false)
	r.seed(6, 0)
	fx, fy = r.at(t, "Song 3, Artist 3")
	b, _ = r.tt.Find("Song 3, Artist 3")
	r.dragTo(fx, fy, fx, b.Y+b.H-8)
	r.tt.Frame()
	if got := r.fake.snapshot(); len(got) != 0 {
		t.Errorf("a drop on its own gap called %v", got)
	}
}

func TestAutoPlayRowsDoNotDrag(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(5, 0, autoPlayFrom(3))
	b, _ := r.tt.Find("Song 5, Artist 5")
	fx, fy := b.X+60, b.Y+b.H/2
	t2, _ := r.tt.Find("Up next: Song 2, Artist 2")
	r.dragTo(fx, fy, fx, t2.Y+4)
	r.tt.Frame()
	if got := r.fake.snapshot(); len(got) != 0 {
		t.Errorf("an AutoPlay row dragged: %v", got)
	}
}

func TestDroppingASongFromAPageQueuesItInTheGap(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(6, 0)
	sx, sy := r.source(t)
	b, _ := r.tt.Find("Song 3, Artist 3")
	// The upper half of Song 3: before it, slot 2.
	r.dragTo(sx, sy, b.X+60, b.Y+6)
	if got := r.fake.wait(t, 1); got[0] != "insert 2" {
		t.Errorf("calls %v", got)
	}
	if len(r.fake.refs) != 1 || r.fake.refs[0].ID != "i.99" {
		t.Errorf("queued %+v", r.fake.refs)
	}
}

func TestADropOnALiveStationsQueueIsRefused(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(3, 0, func(s *services.PlaybackState, q *services.QueueState) {
		q.Items[0].Live = true
		s.Current.Live = true
		s.Controls = services.PlaybackControls{}
	})
	sx, sy := r.source(t)
	b, _ := r.tt.Find("Song 2, Artist 2")
	r.dragTo(sx, sy, b.X+60, b.Y+6)
	r.tt.Frame()
	if got := r.fake.snapshot(); len(got) != 0 {
		t.Errorf("a live station's queue took a drop: %v", got)
	}
}

func TestEscapeClosesTheQueueBackOnTheToggle(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(4, 0)
	r.focusRow(t, "Song 3, Artist 3")
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Frame()
	r.tt.Frame()
	if r.p.ui.queueOpen {
		t.Fatal("Escape left the queue open")
	}
	if !r.tt.Focused("Toggle queue") {
		t.Error("the focus did not return to the queue toggle")
	}
}

func TestOnlyTheRowsShowingAreBuilt(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(500, 0)
	built := func() (n int) {
		for _, s := range r.tt.Texts() {
			if strings.HasPrefix(s, "Song ") && !strings.Contains(s, ",") {
				n++
			}
		}
		return n
	}
	if n := built(); n < 10 || n > 45 {
		t.Errorf("built %d of 500 rows", n)
	}
	if r.has("Song 300, Artist 300") {
		t.Error("a row far below is built")
	}
	x, y := r.at(t, "Song 1, Artist 1")
	r.tt.Scroll(x, y, 0, 5200)
	r.tt.Frame()
	r.tt.Frame()
	top := int(r.p.queue.lists[0].pos.Y / queueRowH)
	if top < 50 {
		t.Fatalf("the list scrolled to %v", r.p.queue.lists[0].pos.Y)
	}
	if !r.tt.HasText(fmt.Sprint("Song ", top+3)) {
		t.Errorf("the rows at the scroll offset are not built: %q", r.tt.Texts())
	}
	if n := built(); n > 45 {
		t.Errorf("built %d rows after scrolling", n)
	}
}

// fakeAuth is the account.
type fakeAuth struct{ restricted bool }

func (f fakeAuth) Status() services.AuthStatus { return services.AuthStatus{Restricted: f.restricted} }

func TestExplicitSongsAreWithheldOnARestrictedAccount(t *testing.T) {
	r := newQueueRig(t, false)
	r.p.followAccount(fakeAuth{restricted: true})
	r.seed(4, 0, func(s *services.PlaybackState, q *services.QueueState) { q.Items[2].Explicit = true })
	r.tt.Until("the explicit song withheld", func() bool { return r.has("Song 3, Artist 3, skipped: explicit") })

	r = newQueueRig(t, false)
	r.p.followAccount(fakeAuth{restricted: false})
	r.seed(4, 0, func(s *services.PlaybackState, q *services.QueueState) { q.Items[2].Explicit = true })
	r.tt.Frame()
	if r.has("Song 3, Artist 3, skipped: explicit") {
		t.Error("an unrestricted account withholds explicit songs")
	}
}

func TestMusicVideosAreMarkedAndLinkWhereThePageExists(t *testing.T) {
	r := newQueueRig(t, false)
	r.h.Env.HasRoute = func(path string) bool { return path == "/music-video/mv.1" }
	r.seed(3, 0, func(s *services.PlaybackState, q *services.QueueState) {
		q.Items[1].Video, q.Items[1].ID, q.Items[1].CatalogID = true, "mv.1", "mv.1"
	})
	if !r.has("Up next: Song 2, Artist 2, music video") || !r.has("Music video") {
		t.Fatalf("a music video is not marked: %q", r.tt.Texts())
	}
	if err := r.tt.Click("Song 2"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.nav.pushed, []string{"/music-video/mv.1"}) {
		t.Errorf("the title pushed %v", r.nav.pushed)
	}
}

func TestInViewListUsesTheViewsTexts(t *testing.T) {
	r := newQueueRig(t, true)
	r.seed(4, 0)
	for _, l := range []string{"Up next: Song 2, Artist 2", "Song 3, Artist 3"} {
		if !r.has(l) {
			t.Errorf("no row %q: %q", l, r.tt.Texts())
		}
	}
	if !r.tt.HasText("3 songs after this one") {
		t.Errorf("the view builds its own summary line: %q", r.tt.Texts())
	}
	// The rows read the view's keyboard help.
	if rowHelp(true, false) == rowHelp(false, false) || rowHelp(true, true) == rowHelp(false, true) {
		t.Error("the view's help is the queue's")
	}
	if strings.Contains(rowHelp(false, true), "Alt+Up") || strings.Contains(rowHelp(true, true), "Alt+Up") {
		t.Error("an AutoPlay row's help names move keys")
	}
	// A drop on the view's list reorders too.
	fx, fy := r.at(t, "Up next: Song 2, Artist 2")
	b, _ := r.tt.Find("Song 4, Artist 4")
	r.dragTo(fx, fy, fx, b.Y+b.H-8)
	if got := r.fake.wait(t, 1); got[0] != "move 1 3" {
		t.Errorf("calls %v", got)
	}
}

func TestAnAutoPlayRowThatStartsPlayingKeepsTheFocus(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(6, 0, autoPlayFrom(3))
	r.focusRow(t, "Song 4, Artist 4")
	// It plays now: the entry is the current song, in the first list.
	r.seed(6, 3, autoPlayFrom(3))
	for range 3 {
		r.tt.Frame()
	}
	if !r.tt.Focused("Song 4, Artist 4") {
		t.Error("the focus was lost when the AutoPlay row became the current song")
	}
}

func TestTheRowsButtonsAreNotTabStops(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(4, 0)
	r.focusRow(t, "Up next: Song 2, Artist 2")
	b, _ := r.tt.Find("Up next: Song 2, Artist 2")
	r.tt.Move(b.X+60, b.Y+b.H/2)
	r.tt.Frame()
	if !r.has("More for Song 2") || !r.has("Remove Song 2 from the queue") {
		t.Fatalf("a hovered row shows no buttons: %q", r.tt.Texts())
	}
	for range 4 {
		r.tt.Key(0, ui.KeyTab)
		if r.tt.Focused("More for Song 2") || r.tt.Focused("Remove Song 2 from the queue") {
			t.Fatal("a row's button took a Tab stop")
		}
	}
}

func TestCloseQueueReturnsTheFocusToTheToggle(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(4, 0)
	r.focusRow(t, "Up next: Song 2, Artist 2")
	if err := r.tt.Click("Close queue"); err != nil {
		t.Fatal(err)
	}
	r.tt.Frame()
	r.tt.Frame()
	if r.p.ui.queueOpen {
		t.Fatal("Close left the queue open")
	}
	if !r.tt.Focused("Toggle queue") {
		t.Error("the focus did not return to the queue toggle")
	}
}
