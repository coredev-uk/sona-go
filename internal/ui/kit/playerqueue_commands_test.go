package kit

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

type fakeQueueSvc struct {
	mu    sync.Mutex
	calls []string
	refs  []services.TrackRef
	done  chan struct{}
}

func newFakeQueueSvc() *fakeQueueSvc { return &fakeQueueSvc{done: make(chan struct{}, 16)} }

func (f *fakeQueueSvc) rec(name string, refs []services.TrackRef) {
	f.mu.Lock()
	f.calls = append(f.calls, name)
	f.refs = append(f.refs, refs...)
	f.mu.Unlock()
	f.done <- struct{}{}
}

func (f *fakeQueueSvc) wait(t *testing.T) {
	t.Helper()
	select {
	case <-f.done:
	case <-time.After(5 * time.Second):
		t.Fatal("no service call")
	}
}

func (f *fakeQueueSvc) PlayItems(_ context.Context, refs []services.TrackRef, _ int, _ services.PlayFrom) error {
	f.rec("playitems", refs)
	return nil
}
func (f *fakeQueueSvc) PlayNext(_ context.Context, refs []services.TrackRef) error {
	f.rec("playnext", refs)
	return nil
}
func (f *fakeQueueSvc) Append(_ context.Context, refs []services.TrackRef) error {
	f.rec("append", refs)
	return nil
}
func (f *fakeQueueSvc) Insert(_ context.Context, _ int, refs []services.TrackRef) error {
	f.rec("insert", refs)
	return nil
}
func (f *fakeQueueSvc) UndoReplace(context.Context) (bool, error) {
	f.rec("undo", nil)
	return true, nil
}
func (f *fakeQueueSvc) SetShuffle(bool) { f.rec("shuffle", nil) }

func (f *fakeQueueSvc) names() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// queueToasts takes the toasts queued so far.
func queueToasts() []string {
	toasts.mu.Lock()
	defer toasts.mu.Unlock()
	var out []string
	for _, t := range toasts.queue {
		out = append(out, t.Title)
	}
	toasts.queue = nil
	return out
}

func songs(n int) []services.Song {
	out := make([]services.Song, n)
	for i := range out {
		out[i] = services.Song{ID: string(rune('a' + i)), Source: services.SourceCatalog, Name: "s", Playable: i != 1}
	}
	return out
}

func newCommands(t *testing.T, added int, mode string) (*QueueCommands, *fakeQueueSvc, *store.Playback) {
	t.Helper()
	b := store.NewBus(nil)
	play := store.NewPlayback(b, nil)
	q := services.QueueState{Index: 0}
	for i := range added + 1 {
		q.Items = append(q.Items, services.QueueItem{UID: uint64(i), Added: i > 0})
	}
	play.Seed(store.EmptyState(), q)
	svc := newFakeQueueSvc()
	queueToasts()
	t.Cleanup(func() { pendingConfirm = nil; queueToasts() })
	return NewQueueCommands(QueueDeps{
		Svc: svc, Play: play, Post: func(fn func()) { fn() },
		ReplaceMode: func() string { return mode },
		Tracks: func(context.Context, Item) ([]services.Song, error) {
			return songs(3), nil
		},
	}), svc, play
}

func TestPlayableRefsKeepsPlayableSongsAndMapsTheStart(t *testing.T) {
	refs, index := PlayableRefs(songs(4), 2)
	if len(refs) != 3 || index != 1 || refs[0].ID != "a" || refs[1].ID != "c" {
		t.Errorf("refs %v index %d", refs, index)
	}
}

func TestPlaySongsWithAShortUserQueueJustStarts(t *testing.T) {
	c, svc, _ := newCommands(t, 3, "undo")
	c.PlaySongs(songs(3), 0, services.PlayFrom{Kind: "album", ID: "x"})
	svc.wait(t)
	if got := svc.names(); len(got) != 1 || got[0] != "playitems" {
		t.Errorf("calls %v", got)
	}
	if got := queueToasts(); len(got) != 0 {
		t.Errorf("toasts %v", got)
	}
}

func TestPlaySongsWithNothingPlayableSaysSo(t *testing.T) {
	c, svc, _ := newCommands(t, 0, "undo")
	c.PlaySongs([]services.Song{{Name: "x"}}, 0, services.PlayFrom{})
	if got := queueToasts(); len(got) != 1 || got[0] != "Nothing here can be played" {
		t.Errorf("toasts %v", got)
	}
	if len(svc.names()) != 0 {
		t.Errorf("calls %v", svc.names())
	}
}

func TestReplacingALongUserQueueOffersUndo(t *testing.T) {
	c, svc, _ := newCommands(t, 4, "undo")
	c.PlaySongs(songs(3), 0, services.PlayFrom{})
	svc.wait(t)
	var spec ToastSpec
	for i := 0; i < 200 && spec.Title == ""; i++ {
		toasts.mu.Lock()
		for _, q := range toasts.queue {
			if q.Title == "Queue replaced" {
				spec = ToastSpec{Title: q.Title, Action: q.Action, OnAction: q.OnAction}
			}
		}
		toasts.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	if spec.Title == "" || spec.Action != "Undo" {
		t.Fatalf("no Undo toast: %+v", spec)
	}
	spec.OnAction()
	svc.wait(t)
	if got := svc.names(); got[len(got)-1] != "undo" {
		t.Errorf("calls %v", got)
	}
}

func TestReplacingALongUserQueueCanAskFirst(t *testing.T) {
	c, svc, _ := newCommands(t, 4, config.ReplaceConfirm)
	var started []bool
	c.Replace("x", func(context.Context) error { svc.rec("start", nil); return nil }, func(ok bool) { started = append(started, ok) })
	for i := 0; i < 100 && pendingConfirm == nil; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if pendingConfirm == nil || pendingConfirm.Title != "Replace your queue?" || pendingConfirm.ConfirmLabel != "Replace queue" {
		t.Fatalf("no question: %+v", pendingConfirm)
	}
	settleConfirm(false)
	if len(started) != 1 || started[0] || len(svc.names()) != 0 {
		t.Errorf("cancel: started %v calls %v", started, svc.names())
	}
	c.Replace("x", func(context.Context) error { svc.rec("start", nil); return nil }, func(ok bool) { started = append(started, ok) })
	for i := 0; i < 100 && pendingConfirm == nil; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	settleConfirm(true)
	svc.wait(t)
	if got := queueToasts(); len(got) != 0 {
		t.Errorf("a confirmed replace offers no Undo: %v", got)
	}
}

func TestQueueItemsNextAndLastExpandCollections(t *testing.T) {
	c, svc, _ := newCommands(t, 0, "undo")
	c.QueueItems([]Item{SongItem(songs(1)[0]), AlbumItem(services.Album{ID: "al"}), MusicVideoItem(services.MusicVideo{ID: "mv"})}, true, false, 0)
	svc.wait(t)
	if got := svc.names(); got[0] != "playnext" {
		t.Fatalf("calls %v", got)
	}
	// the song, two playable songs of the album (songs(3): the second is not), the video
	if len(svc.refs) != 4 || svc.refs[3].Kind != services.TrackKindVideo {
		t.Errorf("refs %+v", svc.refs)
	}
	c.QueueItems([]Item{SongItem(songs(1)[0])}, false, true, 0)
	svc.wait(t)
	if got := svc.names(); got[1] != "append" {
		t.Errorf("calls %v", got)
	}
}

func TestQueueItemsRefusesOnALiveStation(t *testing.T) {
	c, svc, _ := newCommands(t, 0, "undo")
	c.d.Unavailable = func() string { return "Not available while listening to a live station" }
	c.QueueItems([]Item{SongItem(songs(1)[0])}, true, false, 0)
	if len(svc.names()) != 0 {
		t.Errorf("calls %v", svc.names())
	}
	if got := queueToasts(); len(got) != 1 || got[0] != "Not available while listening to a live station" {
		t.Errorf("toasts %v", got)
	}
}
