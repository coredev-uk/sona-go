package miniplayer

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// The interaction-consistency review (#311) pins what the mini player does
// today for each gesture and each kind. The rule being reviewed: choosing a
// result opens it; Play plays; a track list's row plays (double-click or
// Enter); a modifier never does something a visible cue does not hint at. Where
// today's behaviour departs from it, the assertion carries a "DEPARTS:"
// comment, to be found (and flipped) when the behaviour is fixed.

// iaGest is what each gesture on one control did, as iaFx words it.
type iaGest struct {
	click, dbl, enter, shiftEnter, space, right string
}

// iaWant fails for each gesture whose effect is not the one pinned.
func iaWant(t *testing.T, what string, got, want iaGest) {
	t.Helper()
	for _, c := range []struct{ gesture, got, want string }{
		{"click", got.click, want.click},
		{"double-click", got.dbl, want.dbl},
		{"Enter", got.enter, want.enter},
		{"Shift+Enter", got.shiftEnter, want.shiftEnter},
		{"Space", got.space, want.space},
		{"right-click", got.right, want.right},
	} {
		if c.got != c.want {
			t.Errorf("%s, %s: %s, want %s", what, c.gesture, c.got, c.want)
		}
	}
}

// iaMark is the label of an action every kind of item lists, so that a menu
// that opened can be told from none without asserting what a menu holds.
const iaMark = "Menu mark"

// iaMini is the mini player with the effects of a gesture worded: what reached
// the playback service, which window call was made, whether a menu opened.
type iaMini struct {
	t *testing.T
	r *rig
	// calls and desk are how many there were before the gesture.
	calls, desk int
}

func iaMiniRig(t *testing.T, mutate ...func(*services.PlaybackState)) *iaMini {
	t.Helper()
	r := newRig(t)
	for _, k := range []kit.ItemKind{kit.KindSong, kit.KindStation, kit.KindMusicVideo} {
		r.h.Env.Actions.Register(kit.ActionDef{
			ID: "test.ia." + string(k), Target: k, Group: kit.GroupItem, Order: 1,
			Label: func(*kit.ActionContext) string { return iaMark },
			Run:   func(*kit.ActionContext) {},
		})
	}
	r.show(30_000, mutate...)
	return &iaMini{t: t, r: r}
}

// reset closes what a gesture opened and forgets the calls so far.
func (m *iaMini) reset() {
	m.r.tt.Key(0, ui.KeyEscape)
	m.r.tt.Frame()
	m.calls, m.desk = len(m.r.svc.Snapshot()), len(m.r.desk.snapshot())
}

func (m *iaMini) fx() string {
	for range 8 {
		m.r.tt.Drain()
		time.Sleep(4 * time.Millisecond)
	}
	var out []string
	out = append(out, m.r.svc.Snapshot()[m.calls:]...)
	out = append(out, m.r.desk.snapshot()[m.desk:]...)
	if m.r.tt.HasText(iaMark) {
		out = append(out, "menu")
	}
	if len(out) == 0 {
		return "nothing"
	}
	return strings.Join(out, "; ")
}

func (m *iaMini) tab(label string) {
	m.t.Helper()
	for range 40 {
		if tr := access.Read(m.t, m.r.tt.Tester); tr.Focus != nil && tr.Focus.Label == label {
			return
		}
		m.r.tt.Key(0, ui.KeyTab)
		m.r.tt.Frame()
	}
	m.t.Fatalf("Tab never reached %q", label)
}

// gestures does each gesture at (x, y) from a clean start and says what it did.
// A control that is a Tab stop (label) also gets the keys.
func (m *iaMini) gestures(x, y float32, label string) iaGest {
	m.t.Helper()
	g := iaGest{enter: "n/a", shiftEnter: "n/a", space: "n/a"}
	m.reset()
	m.r.tt.ClickAt(x, y)
	g.click = m.fx()
	m.reset()
	m.r.tt.ClickAt(x, y)
	m.r.tt.ClickAt(x, y)
	g.dbl = m.fx()
	if label != "" {
		for _, k := range []struct {
			mods ui.Modifiers
			key  ui.Key
			into *string
		}{{0, ui.KeyEnter, &g.enter}, {ui.Shift, ui.KeyEnter, &g.shiftEnter}, {0, ui.KeySpace, &g.space}} {
			m.reset()
			m.tab(label)
			m.r.tt.Key(k.mods, k.key)
			*k.into = m.fx()
		}
	}
	m.reset()
	m.r.tt.RightClickAt(x, y)
	g.right = m.fx()
	m.reset()
	return g
}

// at is the middle of the element called label.
func (m *iaMini) at(label string) (float32, float32) {
	m.t.Helper()
	r, ok := m.r.tt.Find(label)
	if !ok {
		m.t.Fatalf("%q is not on screen: %q", label, m.r.tt.Texts())
	}
	return r.X + r.W/2, r.Y + r.H/2
}

// The mini player is a remote: its cover, title, artist and album line are not
// controls. A click or a double-click does nothing (a press on them drags the
// window), none is a Tab stop, and a right-click on any of them opens the
// playing item's menu, of its kind. There is no link to the song, its album or
// its artist from here: the player bar's title and artist are links, and the
// mini player's are text.
func TestInteractionMiniPlayerSongText(t *testing.T) {
	for _, c := range []struct {
		kind   string
		mutate []func(*services.PlaybackState)
		title  string
		artist string
		other  string // the line under the artist
	}{
		{"a song", nil, "Song Title", "The Artist", "The Album"},
		{"a music video", []func(*services.PlaybackState){func(s *services.PlaybackState) { s.Current.Video = true }}, "Song Title", "The Artist", "Music video"},
		{"a live station", []func(*services.PlaybackState){station}, "KISS", "", "Live radio"},
	} {
		m := iaMiniRig(t, c.mutate...)
		nothing := iaGest{click: "nothing", dbl: "nothing", enter: "n/a", shiftEnter: "n/a", space: "n/a", right: "menu"}
		x, y := m.at(c.title)
		// A remote: the title is text (the player bar's links, and Open Sona, reach the pages).
		iaWant(t, "the title of "+c.kind, m.gestures(x, y, ""), nothing)
		if c.artist != "" {
			x, y = m.at(c.artist)
			iaWant(t, "the artist of "+c.kind, m.gestures(x, y, ""), nothing)
		}
		x, y = m.at(c.other)
		// The line under the artist opens the playing item's menu too, as every line of text does.
		iaWant(t, "the line under the artist of "+c.kind, m.gestures(x, y, ""), nothing)
		// The cover fills the window's leading edge.
		iaWant(t, "the cover of "+c.kind, m.gestures(coverSize/2, 40, ""), nothing)

		for range 20 {
			m.r.tt.Key(0, ui.KeyTab)
			m.r.tt.Frame()
			if tr := access.Read(t, m.r.tt.Tester); tr.Focus != nil {
				switch tr.Focus.Label {
				case c.title, c.artist, c.other:
					t.Errorf("Tab stops at %q in the mini player of %s", tr.Focus.Label, c.kind)
				}
			}
		}
	}
}

// The third line's every state is part of the text block: what needs the user
// or what failed (a message in place of the album) opens the menu on a
// right-click as the title above it does.
func TestInteractionMiniPlayerMenuCoversTheMessageLines(t *testing.T) {
	for _, c := range []struct {
		name   string
		mutate func(*services.PlaybackState)
		line   string
	}{
		{"a failed song", func(s *services.PlaybackState) { s.State, s.Error = store.StateError, "boom" }, "Couldn't play this song. Press play to try again."},
		{"a sign-in needed", func(s *services.PlaybackState) { s.AuthRequired = true }, "Sign in again in Sona to keep playing"},
	} {
		m := iaMiniRig(t, c.mutate)
		x, y := m.at(c.line)
		m.reset()
		m.r.tt.RightClickAt(x, y)
		if got := m.fx(); got != "menu" {
			t.Errorf("right-click on the line of %s: %s, want the playing item's menu", c.name, got)
		}
	}
}

// The window's own buttons are the mini player's only ways out: Open Sona
// brings the main window back and Close closes the mini player. Each is a
// button: a click, Enter and Space press it; Shift+Enter and a right-click do
// nothing; a double-click presses it twice.
func TestInteractionMiniPlayerWindowButtons(t *testing.T) {
	for _, c := range []struct{ label, call string }{
		{"Open Sona", "show main"},
		{"Close mini player", "close mini"},
	} {
		m := iaMiniRig(t)
		x, y := m.at(c.label)
		iaWant(t, c.label, m.gestures(x, y, c.label), iaGest{
			click: c.call, dbl: c.call + "; " + c.call, enter: c.call, shiftEnter: "nothing", space: c.call, right: "nothing",
		})
	}
}

// Play/Pause, Previous and Next are buttons of the transport, as in the player
// bar: a click, Enter and Space press them.
func TestInteractionMiniPlayerTransport(t *testing.T) {
	for _, c := range []struct{ label, call string }{
		{"Pause", "toggle"},
		{"Previous", "prev"},
		{"Next", "next"},
	} {
		m := iaMiniRig(t)
		x, y := m.at(c.label)
		iaWant(t, c.label, m.gestures(x, y, c.label), iaGest{
			click: c.call, dbl: c.call + "; " + c.call, enter: c.call, shiftEnter: "nothing", space: c.call, right: "nothing",
		})
	}
}
