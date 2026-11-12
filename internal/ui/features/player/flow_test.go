package player

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// flow is the bar and the queue panel over the real services: the playback
// coordinator on the fake engine and the seeded library.
type flow struct {
	h     *uitest.Harness
	env   *module.Env
	mod   module.Module
	tt    *uitest.Session
	pb    *services.PlaybackService
	album fixture.Album
}

// newFlow plays the fixture's second album (three songs) from its first.
func newFlow(t *testing.T, opts ...uitest.Option) *flow {
	t.Helper()
	h := uitest.New(t, opts...)
	f := &flow{h: h, env: h.Env, mod: New(h.Env), pb: services.Service[services.PlaybackService](h.Env.Core), album: fixture.Albums()[1]}
	if f.album.SongCount() != 3 {
		t.Fatalf("the fixture's second album has %d songs", f.album.SongCount())
	}
	f.mod.Init()
	f.env.Play.Seed(f.pb.State(), f.pb.Queue())
	f.tt = h.Tester(t, func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			ui.Row(c).Grow(1).Basis(0).MinHeight(0).Children(func() {
				ui.Box(c).Grow(1)
				f.mod.Slots.Aside(c)
			})
			f.mod.Slots.Bottom(c)
		})
	}, 1132, 700)
	refs := make([]services.TrackRef, f.album.SongCount())
	for j := range refs {
		refs[j] = services.TrackRef{ID: fixture.SongLibraryID(1, j), Source: services.SourceLibrary}
	}
	if err := f.pb.PlayItems(context.Background(), refs, 0, services.PlayFrom{Kind: "album", ID: fixture.AlbumLibraryID(1)}); err != nil {
		t.Fatal(err)
	}
	f.tt.Until("the first song playing", f.tt.Shows("Pause", f.album.Song(0)))
	return f
}

func (f *flow) calls() []string { return f.h.Engine.Calls() }

func (f *flow) hasCall(prefix string) bool {
	return slices.ContainsFunc(f.calls(), func(c string) bool { return strings.HasPrefix(c, prefix) })
}

func (f *flow) shortcut(id string) {
	for _, s := range f.mod.Shortcuts {
		if s.ID == id {
			s.Run()
			return
		}
	}
	panic("no shortcut " + id)
}

func TestTheBarPlaysPausesAndSkipsThroughTheCoordinator(t *testing.T) {
	f := newFlow(t)
	f.tt.MustClick("Pause")
	f.tt.Until("the engine paused and the bar offering Play", func() bool { return f.hasCall("pause") && f.tt.HasText("Play") })
	f.tt.MustClick("Play")
	f.tt.Until("the engine resumed", func() bool { return f.hasCall("resume") && f.tt.HasText("Pause") })

	f.tt.MustClick("Next")
	f.tt.Until("the second song in the bar", f.tt.Shows(f.album.Song(1)))
	if played := f.h.Engine.Played(); len(played) < 2 {
		t.Errorf("the engine played %v", played)
	}
	f.tt.MustClick("Previous")
	f.tt.Until("the first song again", f.tt.Shows(f.album.Song(0)))
}

func TestTheBarSeeksSetsShuffleRepeatAndVolume(t *testing.T) {
	f := newFlow(t)
	f.shortcut("player.seek-forward")
	f.tt.Until("a seek reaching the engine", func() bool { return f.hasCall("seek") })

	f.tt.MustClick("Shuffle off")
	f.tt.Until("shuffle on", f.tt.Shows("Shuffle on"))
	f.tt.MustClick("Shuffle on")
	f.tt.Until("shuffle off", f.tt.Shows("Shuffle off"))

	f.tt.MustClick("Repeat off")
	f.tt.Until("repeat all", f.tt.Shows("Repeat all"))
	f.tt.MustClick("Repeat all")
	f.tt.Until("repeat one", f.tt.Shows("Repeat one"))
	f.tt.MustClick("Repeat one")
	f.tt.Until("repeat off", f.tt.Shows("Repeat off"))

	if f.tt.HasText("Mute") {
		f.tt.MustClick("Mute")
		f.tt.Until("the engine muted", func() bool { return f.hasCall("volume 0") })
		f.tt.Until("Unmute", f.tt.Shows("Unmute"))
	}
}

func TestTheBarFollowsTheCoordinatorWhenItIsDrivenElsewhere(t *testing.T) {
	f := newFlow(t)
	if err := f.pb.Pause(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.tt.Until("the bar showing Play", f.tt.Shows("Play"))
	if err := f.pb.Next(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.tt.Until("the next song in the bar", f.tt.Shows(f.album.Song(1)))
}

// queueRow is the label of a row of the queue panel.
func (f *flow) queueRow(j int) string { return f.album.Song(j) + ", " + f.album.Artist }

func TestTheQueuePanelListsEditsAndReorders(t *testing.T) {
	f := newFlow(t)
	f.tt.MustClick("Queue")
	f.tt.Until("the queue's rows", f.tt.Shows("Up next: "+f.queueRow(1), f.queueRow(2), "2 songs after this one"))

	// Drag the first upcoming song below the second.
	from, ok := f.tt.Find("Up next: " + f.queueRow(1))
	if !ok {
		t.Fatal("no Up next row")
	}
	to, _ := f.tt.Find(f.queueRow(2))
	fx, fy := from.X+from.W/2, from.Y+from.H/2
	ty := to.Y + to.H - 8
	f.tt.Press(fx, fy)
	f.tt.Move(fx, fy+10)
	f.tt.Move(fx, fy+20)
	f.tt.Move(fx, ty)
	f.tt.Move(fx+1, ty)
	f.tt.Release(fx+1, ty)
	f.tt.Until("the queue reordered", func() bool {
		q := f.env.Play.Queue()
		return len(q.Items) == 3 && q.Items[1].Name == f.album.Song(2) && q.Items[2].Name == f.album.Song(1)
	})

	// Remove the last one with its button.
	f.tt.Until("the moved row last", f.tt.Shows(f.queueRow(1)))
	f.tt.MustClick("Remove " + f.album.Song(1) + " from the queue")
	f.tt.Until("two songs left", func() bool { return len(f.env.Play.Queue().Items) == 2 })

	// Clear what is queued after the playing song.
	f.tt.MustClick("Clear")
	f.tt.Until("one song left", func() bool { return len(f.env.Play.Queue().Items) == 1 })
}

func TestTheAutoPlayToggleFollowsTheCoordinator(t *testing.T) {
	f := newFlow(t)
	f.tt.MustClick("Queue")
	f.tt.Until("the queue", f.tt.Shows("AutoPlay"))
	if f.env.Play.State().AutoPlay {
		t.Fatal("AutoPlay starts on")
	}
	// Turning it on asks Apple for a station: the fixture network refuses it, and
	// the coordinator reports that rather than crashing.
	f.tt.MustClick("AutoPlay")
	f.tt.Until("AutoPlay on", func() bool { return f.env.Play.State().AutoPlay })
	f.tt.MustClick("AutoPlay")
	f.tt.Until("AutoPlay off", func() bool { return !f.env.Play.State().AutoPlay })
}
