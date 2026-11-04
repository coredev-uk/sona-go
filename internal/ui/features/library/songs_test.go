package library

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// songsRecorder passes through to the real LibraryService and notes the
// songs queries it answered.
type songsRecorder struct {
	libraryAPI
	mu   sync.Mutex
	asks []string
}

func (r *songsRecorder) Songs(ctx context.Context, sort string, desc bool, off, lim int) (services.SongPage, error) {
	r.mu.Lock()
	r.asks = append(r.asks, fmt.Sprintf("%s desc=%v offset=%d", sort, desc, off))
	r.mu.Unlock()
	return r.libraryAPI.Songs(ctx, sort, desc, off, lim)
}

func (r *songsRecorder) asked() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.asks)
}

// songsFailing fails the songs reads while fail is set.
type songsFailing struct {
	libraryAPI
	fail atomic.Bool
}

var errSongsRead = errors.New("read failed")

func (f *songsFailing) Songs(ctx context.Context, sort string, desc bool, off, lim int) (services.SongPage, error) {
	if f.fail.Load() {
		return services.SongPage{}, errSongsRead
	}
	return f.libraryAPI.Songs(ctx, sort, desc, off, lim)
}

func (f *songsFailing) FavoriteSongs(ctx context.Context, off, lim int) (services.SongPage, error) {
	if f.fail.Load() {
		return services.SongPage{}, errSongsRead
	}
	return f.libraryAPI.FavoriteSongs(ctx, off, lim)
}

// songsSynth serves a library of total synthetic songs, to page through.
type songsSynth struct {
	libraryAPI
	total atomic.Int64
	mu    sync.Mutex
	asks  []int
}

func (s *songsSynth) Songs(_ context.Context, _ string, _ bool, off, lim int) (services.SongPage, error) {
	s.mu.Lock()
	s.asks = append(s.asks, off)
	s.mu.Unlock()
	total := int(s.total.Load())
	var items []services.Song
	for i := off; i < min(off+lim, total); i++ {
		items = append(items, services.Song{ID: fmt.Sprintf("syn%03d", i), Name: fmt.Sprintf("Synth %03d", i), Artist: "Synth", Playable: true})
	}
	return services.SongPage{Items: items, Total: total}, nil
}

func (s *songsSynth) offsets() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.asks)
}

// songsAccount is an authAPI with a fixed answer.
type songsAccount struct{ state string }

func (a songsAccount) Status() services.AuthStatus { return services.AuthStatus{State: a.state} }

// songsOrder is the names of the songs shown, in the order shown.
func songsOrder(r *pageRig) []string {
	var got []string
	for _, w := range r.tt.Texts() {
		if strings.Contains(w, " song ") && !strings.HasPrefix(w, "Play ") && !strings.HasPrefix(w, "More for ") && (len(got) == 0 || got[len(got)-1] != w) {
			got = append(got, w)
		}
	}
	return got
}

// titleOrder is the seven albums' songs by title.
var titleOrder = []string{
	"Apple Orchard song 1", "Apple Orchard song 2", "Blue Hour song 1", "Blue Hour song 2",
	"Lone Wolf song 1", "Lone Wolf song 2", "Mango Season song 1", "Mango Season song 2",
	"Paper Planes song 1", "Paper Planes song 2", "Single Malt song 1", "Single Malt song 2",
	"Zebra Crossing song 1", "Zebra Crossing song 2",
}

func newSongsRig(t *testing.T, start string, before func(l *library), opts ...uitest.Option) *pageRig {
	t.Helper()
	if len(opts) == 0 {
		opts = []uitest.Option{uitest.Albums(sevenAlbums...)}
	}
	return newPageRig(t, start, 1132, 1000, before, opts...)
}

// songsPlayed is the catalogue IDs the engine was asked to play.
func songsPlayed(r *pageRig) []string { return r.h.Engine.Played() }

func songsClickRow(r *pageRig, name string, mods ui.Modifiers, times int) {
	r.t.Helper()
	rect, ok := r.tt.Find(name)
	if !ok {
		r.t.Fatalf("no %q in %q", name, r.tt.Texts())
	}
	for range times {
		r.tt.ClickAtWith(mods, rect.X+100, rect.Y+rect.H/2)
	}
}

func TestSongsListShowsTheLibrary(t *testing.T) {
	var rec *songsRecorder
	r := newSongsRig(t, songsPath, func(l *library) {
		rec = &songsRecorder{libraryAPI: l.lib}
		l.lib = rec
	})
	r.tt.Until("the songs", r.tt.Shows("Songs", "14 songs", "Zebra Crossing song 2"))
	if got := songsOrder(r); !slices.Equal(got, titleOrder) {
		t.Errorf("songs shown %q, want %q", got, titleOrder)
	}
	for _, w := range []string{"Title", "Artist", "Album", "Sort", "Shuffle"} {
		if !r.tt.HasText(w) {
			t.Errorf("page lacks %q", w)
		}
	}
	if asks := rec.asked(); len(asks) == 0 || asks[0] != "title desc=false offset=0" {
		t.Errorf("first query %q", asks)
	}
}

func TestSongsSortSelectChangesTheURLAndTheQuery(t *testing.T) {
	var rec *songsRecorder
	r := newSongsRig(t, songsPath, func(l *library) {
		rec = &songsRecorder{libraryAPI: l.lib}
		l.lib = rec
	})
	r.tt.Until("the songs", r.tt.Shows("14 songs"))
	if err := r.tt.Click("Sort"); err != nil {
		t.Fatal(err)
	}
	if err := r.tt.Click("Recently added"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Location(); got != songsPath+"?desc=true&sort=dateAdded" {
		t.Fatalf("after choosing Recently added the URL is %q", got)
	}
	r.tt.Until("newest first", func() bool {
		o := songsOrder(r)
		return len(o) == len(titleOrder) && !slices.Equal(o, titleOrder)
	})
	if asks := rec.asked(); !slices.Contains(asks, "dateAdded desc=true offset=0") {
		t.Errorf("service asks %q", asks)
	}
}

func TestSongsHeaderClickTogglesTheSort(t *testing.T) {
	r := newSongsRig(t, songsPath, nil)
	r.tt.Until("the songs", r.tt.Shows("14 songs"))
	// The Album header, where it is before the Sort select shows that word.
	rect, ok := r.tt.Find("Album")
	if !ok {
		t.Fatal("no Album header")
	}
	x, y := rect.X+rect.W/2, rect.Y+rect.H/2
	r.tt.ClickAt(x, y)
	if got := r.router.Location(); got != songsPath+"?desc=false&sort=album" {
		t.Fatalf("Album header: URL %q", got)
	}
	r.tt.Drain()
	r.tt.ClickAt(x, y)
	if got := r.router.Location(); got != songsPath+"?desc=true&sort=album" {
		t.Fatalf("Album header again: URL %q", got)
	}
	r.tt.Until("reversed", func() bool {
		o := songsOrder(r)
		return len(o) == len(titleOrder) && strings.HasPrefix(o[0], "Zebra Crossing")
	})
}

func TestSongsPlayButtonPlaysTheLibraryFromTheTop(t *testing.T) {
	r := newSongsRig(t, songsPath, nil)
	r.tt.Until("the songs", r.tt.Shows("14 songs"))
	if err := r.tt.Click("Play your songs"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the engine to play", func() bool { return len(songsPlayed(r)) > 0 })
	// Apple Orchard is album 1: its first song.
	if got := songsPlayed(r)[0]; got != "10010" {
		t.Errorf("played %v, want Apple Orchard song 1 (10010)", got)
	}
	if st := services.Service[services.PlaybackService](r.h.Core).State(); st.Shuffle {
		t.Error("Play turned shuffle on")
	}
}

func TestSongsShuffleTurnsShuffleOnAndPlays(t *testing.T) {
	r := newSongsRig(t, songsPath, nil)
	r.tt.Until("the songs", r.tt.Shows("14 songs"))
	if err := r.tt.Click("Shuffle"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the engine to play", func() bool { return len(songsPlayed(r)) > 0 })
	if st := services.Service[services.PlaybackService](r.h.Core).State(); !st.Shuffle {
		t.Error("Shuffle left shuffle off")
	}
}

func TestSongsRowDoubleClickAndEnterPlayFromTheRow(t *testing.T) {
	r := newSongsRig(t, songsPath, nil)
	r.tt.Until("the songs", r.tt.Shows("Blue Hour song 1"))
	songsClickRow(r, "Blue Hour song 1", 0, 2)
	r.tt.Until("the engine to play", func() bool { return len(songsPlayed(r)) > 0 })
	// Blue Hour is album 4; it is the third song by title.
	if got := songsPlayed(r)[0]; got != "10040" {
		t.Errorf("double-click played %v, want 10040", got)
	}
	n := len(songsPlayed(r))
	songsClickRow(r, "Lone Wolf song 2", 0, 1)
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Until("a second play", func() bool { return len(songsPlayed(r)) > n })
	if got := songsPlayed(r); got[len(got)-1] != "10021" {
		t.Errorf("Enter played %v, want Lone Wolf song 2 (10021)", got)
	}
}

func TestSongsKeyboardNavigationAndSelection(t *testing.T) {
	r := newSongsRig(t, songsPath, nil)
	r.tt.Until("the songs", r.tt.Shows("Apple Orchard song 1"))
	sel := &r.l.songs.list.st.Sel
	for i := 0; i < 20 && !r.tt.Focused("Apple Orchard song 1"); i++ {
		r.tt.Key(0, ui.KeyTab)
	}
	if !r.tt.Focused("Apple Orchard song 1") {
		t.Fatal("Tab never reached the list")
	}
	r.tt.Key(0, ui.KeyDown)
	r.tt.Key(0, ui.KeyDown)
	if !r.tt.Focused("Blue Hour song 1") {
		t.Errorf("Down Down: focus is not on the third row")
	}
	r.tt.Key(ui.Shift, ui.KeyDown)
	if sel.Len() != 2 {
		t.Errorf("Shift+Down selected %d rows", sel.Len())
	}
	r.tt.Key(0, ui.KeyEnd)
	if !r.tt.Focused("Zebra Crossing song 2") {
		t.Errorf("End: focus is not on the last row")
	}
	r.tt.Key(0, ui.KeyHome)
	if !r.tt.Focused("Apple Orchard song 1") {
		t.Errorf("Home: focus is not on the first row")
	}
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Until("Enter to play", func() bool { return len(songsPlayed(r)) > 0 })
	if got := songsPlayed(r)[0]; got != "10010" {
		t.Errorf("Enter played %v", got)
	}
}

func TestSongsClickCtrlClickAndShiftClickSelect(t *testing.T) {
	r := newSongsRig(t, songsPath, nil)
	r.tt.Until("the songs", r.tt.Shows("Blue Hour song 2"))
	sel := &r.l.songs.list.st.Sel
	songsClickRow(r, "Apple Orchard song 2", 0, 1)
	if sel.Len() != 1 {
		t.Fatalf("a click selected %d rows", sel.Len())
	}
	songsClickRow(r, "Blue Hour song 2", ui.Shift, 1)
	if sel.Len() != 3 {
		t.Errorf("Shift+click selected %d rows, want 3", sel.Len())
	}
	songsClickRow(r, "Apple Orchard song 2", ui.Cmd, 1)
	if sel.Len() != 2 {
		t.Errorf("Ctrl+click left %d rows selected, want 2", sel.Len())
	}
	if got := songsPlayed(r); len(got) != 0 {
		t.Errorf("selecting played %v", got)
	}
}

func TestSongsOpenArtistAndAlbumOnlyWhenThePagesExist(t *testing.T) {
	r := newSongsRig(t, songsPath, nil)
	r.tt.Until("the songs", r.tt.Shows("14 songs"))
	_ = r.tt.Click("Zed")
	if got := r.router.Location(); got != songsPath {
		t.Errorf("an artist without a page navigated to %q", got)
	}
	r2 := newSongsRig(t, songsPath, func(l *library) {
		l.env.HasRoute = func(p string) bool { return strings.HasPrefix(p, "/artist/") || strings.HasPrefix(p, "/album/") }
	})
	r2.tt.Until("the songs", r2.tt.Shows("14 songs"))
	if err := r2.tt.Click("Zed"); err != nil {
		t.Fatal(err)
	}
	if got := r2.router.Location(); !strings.HasPrefix(got, "/artist/") {
		t.Errorf("the artist cell went to %q", got)
	}
}

func TestSongsGateStates(t *testing.T) {
	// No account: the sign-in text, and its action when the shell has a dialog.
	opened := false
	r := newSongsRig(t, songsPath, func(l *library) { l.openAccount = func() { opened = true } }, uitest.Empty())
	r.tt.Until("the signed-out text", r.tt.Shows("Sign in to see your songs", "Songs"))
	if r.tt.HasText("0 songs") {
		t.Error("an empty library counts its songs")
	}
	if err := r.tt.Click("Sign in"); err != nil {
		t.Fatal(err)
	}
	if !opened {
		t.Error("Sign in did not open the account dialog")
	}

	// Signed in, never synced: the first sync is on its way.
	r = newSongsRig(t, songsPath, func(l *library) {
		fakeAuth(l, songsAccount{"signed_in"})
	}, uitest.Empty())
	r.tt.Until("the syncing text", r.tt.Shows("Your library is syncing"))

	// Signed in, synced: an empty library.
	r = newSongsRig(t, songsPath, func(l *library) { fakeAuth(l, songsAccount{"signed_in"}) })
	emptyTheSyncedLibrary(r)
	r.tt.Until("the empty text", r.tt.Shows("No songs in your library yet",
		"Songs you add in Apple Music on any device appear here after the next sync."))
}

func TestSongsErrorStateRetries(t *testing.T) {
	var f *songsFailing
	r := newSongsRig(t, songsPath, func(l *library) {
		f = &songsFailing{libraryAPI: l.lib}
		f.fail.Store(true)
		l.lib = f
	})
	r.tt.Until("the error", r.tt.Shows("Couldn't read your songs from this computer", "Try again"))
	f.fail.Store(false)
	if err := r.tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the songs", r.tt.Shows("14 songs", "Apple Orchard song 1"))
	if r.tt.HasText("Couldn't read your songs from this computer") {
		t.Error("the error stays after a successful retry")
	}
}

func TestSongsLoadMorePagesAsTheListScrolls(t *testing.T) {
	var s *songsSynth
	r := newSongsRig(t, songsPath, func(l *library) {
		s = &songsSynth{libraryAPI: l.lib}
		s.total.Store(450)
		l.lib = s
	})
	r.tt.Until("the first page", r.tt.Shows("450 songs", "Synth 000"))
	// The first page and its neighbour; the third is for later.
	if got := s.offsets(); slices.Contains(got, 400) {
		t.Fatalf("first read asked for %v, want pages 0 and 1 only", got)
	}
	// Rows 200 and on (44 DIP each) are in range: the third page is read.
	for i := 0; i < 60 && !slices.Contains(s.offsets(), 400); i++ {
		r.tt.Scroll(500, 500, 0, 400)
		r.tt.Drain()
	}
	r.tt.Until("the third page", func() bool { return slices.Contains(s.offsets(), 400) })
}

func TestSongsFollowASync(t *testing.T) {
	var s *songsSynth
	r := newSongsRig(t, songsPath, func(l *library) {
		s = &songsSynth{libraryAPI: l.lib}
		s.total.Store(3)
		l.lib = s
	})
	r.tt.Until("three songs", r.tt.Shows("3 songs"))
	s.total.Store(4)
	r.h.Env.Bus.Emit(services.EventSyncProgress, services.SyncProgress{Domain: "songs", Done: 4, Total: 4, Finished: true})
	r.tt.Until("four songs", r.tt.Shows("4 songs"))
}
