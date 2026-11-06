package nowplaying

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/mpris"
)

type fakeBackend struct {
	mu    sync.Mutex
	calls []string
	infos []Info
	fn    func(Command, time.Duration)
	off   bool
}

func (f *fakeBackend) rec(s string) { f.mu.Lock(); f.calls = append(f.calls, s); f.mu.Unlock() }
func (f *fakeBackend) Set(i Info) {
	f.rec("set")
	f.mu.Lock()
	f.infos = append(f.infos, i)
	f.mu.Unlock()
}

func (f *fakeBackend) SetPlayback(s State, p time.Duration) {
	f.rec("playback")
	f.mu.Lock()
	f.infos = append(f.infos, Info{State: s, Position: p})
	f.mu.Unlock()
}
func (f *fakeBackend) Clear() { f.rec("clear") }
func (f *fakeBackend) OnCommand(fn func(Command, time.Duration)) func() {
	f.fn = fn
	return func() { f.off = true }
}

func (f *fakeBackend) got() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

type fakePlayer struct {
	mpris.Player
	calls chan string
}

func (p *fakePlayer) rec(s string) error             { p.calls <- s; return nil }
func (p *fakePlayer) Play(context.Context) error     { return p.rec("play") }
func (p *fakePlayer) Pause(context.Context) error    { return p.rec("pause") }
func (p *fakePlayer) Stop(context.Context) error     { return p.rec("stop") }
func (p *fakePlayer) Next(context.Context) error     { return p.rec("next") }
func (p *fakePlayer) Previous(context.Context) error { return p.rec("previous") }
func (p *fakePlayer) SetPosition(_ context.Context, id string, pos time.Duration) error {
	return p.rec("seek " + id + " " + pos.String())
}

func snap(status mpris.PlaybackStatus, pos time.Duration) mpris.Snapshot {
	return mpris.Snapshot{
		Status: status, Position: pos, UpdatedAt: time.Now(), CanControl: true,
		CanPlay: true, CanPause: true, CanGoNext: true, CanSeek: true,
		Track: &mpris.Track{ID: "i.1", Title: "T", Artist: "A", Album: "B", Length: 3 * time.Minute},
	}
}

func newAdapter(t *testing.T) (*Adapter, *fakeBackend, *fakePlayer) {
	t.Helper()
	fb, fp := &fakeBackend{}, &fakePlayer{calls: make(chan string, 8)}
	a := New(fb, fp, slog.New(slog.DiscardHandler))
	t.Cleanup(func() { _ = a.Close() })
	return a, fb, fp
}

func TestUpdateMapsAndDiffs(t *testing.T) {
	dir := t.TempDir()
	art := filepath.Join(dir, "a.jpg")
	if err := os.WriteFile(art, []byte("img"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, fb, _ := newAdapter(t)

	snap := func(st mpris.PlaybackStatus, p time.Duration) mpris.Snapshot {
		s := snap(st, p)
		s.Track.ArtURL = "file://" + art
		return s
	}
	s := snap(mpris.StatusPlaying, 5*time.Second)
	a.Update(s)
	in := fb.infos[0]
	if in.Title != "T" || in.Artist != "A" || in.Album != "B" || in.Duration != 3*time.Minute || in.State != Playing || string(in.Artwork) != "img" {
		t.Fatalf("set = %+v", in)
	}
	if want := Play | Pause | Stop | Next | Seek; in.Commands != want {
		t.Fatalf("commands = %b, want %b", in.Commands, want)
	}

	// A tick with the playhead where it should be does nothing.
	a.Update(s)
	// Paused: only the playback.
	a.Update(snap(mpris.StatusPaused, 6*time.Second))
	// A seek.
	a.Update(snap(mpris.StatusPaused, 90*time.Second))
	// Same again.
	a.Update(snap(mpris.StatusPaused, 90*time.Second))
	// A new track.
	n := snap(mpris.StatusPlaying, 0)
	n.Track.Title = "U"
	a.Update(n)
	// Nothing loaded.
	a.Update(mpris.Snapshot{})
	a.Update(mpris.Snapshot{})

	want := []string{"set", "playback", "playback", "set", "clear"}
	if got := fb.got(); len(got) != len(want) {
		t.Fatalf("calls = %v, want %v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("calls = %v, want %v", got, want)
			}
		}
	}
	if fb.infos[1].State != Paused || fb.infos[2].Position != 90*time.Second {
		t.Fatalf("playback = %+v %+v", fb.infos[1], fb.infos[2])
	}
}

func TestCommandsReachPlayerOffMainThread(t *testing.T) {
	a, fb, fp := newAdapter(t)
	a.Update(snap(mpris.StatusPlaying, 0))
	for _, c := range []struct {
		cmd  Command
		want string
	}{
		{Play, "play"},
		{Pause, "pause"},
		{Stop, "stop"},
		{Next, "next"},
		{Previous, "previous"},
		{Seek, "seek i.1 42s"},
	} {
		fb.fn(c.cmd, 42*time.Second)
		select {
		case got := <-fp.calls:
			if got != c.want {
				t.Fatalf("command %d -> %q, want %q", c.cmd, got, c.want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("command %d never reached the player", c.cmd)
		}
	}
}

func TestClose(t *testing.T) {
	a, fb, _ := newAdapter(t)
	a.Update(snap(mpris.StatusPlaying, 0))
	_ = a.Close()
	_ = a.Close()
	if !fb.off || fb.got()[len(fb.got())-1] != "clear" {
		t.Fatalf("off=%v calls=%v", fb.off, fb.got())
	}
}
