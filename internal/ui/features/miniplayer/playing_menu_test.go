package miniplayer

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// A song and a station each get the menu of their kind from the mini
// player's title and artist, and a choice runs.
func TestMiniPlayerTitleAndArtistOpenTheMenuOfTheirKind(t *testing.T) {
	r := newRig(t)
	var ran []string
	def := func(id string, k kit.ItemKind, label string) kit.ActionDef {
		return kit.ActionDef{
			ID: id, Target: k, Label: func(*kit.ActionContext) string { return label }, Group: kit.GroupItem, Order: 10,
			Run: func(a *kit.ActionContext) { ran = append(ran, id+":"+a.Item().Station().ID+a.Item().Song().ID) },
		}
	}
	r.h.Env.Actions.Register(def("t.song", kit.KindSong, "Song action"), def("t.station", kit.KindStation, "Station action"))

	r.show(30_000)
	for _, from := range []string{"Song Title", "The Artist"} {
		if err := r.tt.RightClick(from); err != nil {
			t.Fatal(err)
		}
		r.tt.Frame()
		r.tt.Want("Song action")
		r.tt.Lacks("Station action")
		r.tt.Key(0, ui.KeyEscape)
		r.tt.Frame()
	}
	if err := r.tt.RightClick("Song Title"); err != nil {
		t.Fatal(err)
	}
	r.tt.Frame()
	r.tt.MustClick("Song action")
	r.tt.Frame()
	r.tt.Frame()
	if len(ran) != 1 || ran[0] != "t.song:i.1" {
		t.Errorf("ran %q", ran)
	}

	r.show(0, func(s *services.PlaybackState) { station(s) })
	if err := r.tt.RightClick("KISS"); err != nil {
		t.Fatal(err)
	}
	r.tt.Frame()
	r.tt.Want("Station action")
	r.tt.Lacks("Song action")
}

// The window is the handle that drags it: a primary press on the title opens
// no menu and is not taken. A choice that needs the main window (a page, a
// dialog) brings it back first; a queue action works in place. Shift+F10
// opens the menu from the keyboard.
func TestMiniPlayerMenuKeyboardAndMainWindow(t *testing.T) {
	r := newRig(t)
	var ran []string
	def := func(id string, g kit.ActionGroup, label string) kit.ActionDef {
		return kit.ActionDef{
			ID: id, Target: kit.KindSong, Label: func(*kit.ActionContext) string { return label }, Group: g, Order: 10,
			Run: func(*kit.ActionContext) { ran = append(ran, id) },
		}
	}
	r.h.Env.Actions.Register(def("t.queue", kit.GroupQueue, "Queue action"), def("t.page", kit.GroupItem, "Page action"))
	r.show(30_000)

	_ = r.tt.Click("Song Title")
	r.tt.Frame()
	r.tt.Lacks("Queue action")

	for range 6 { // a Tab stop inside the window
		if r.tt.Focused("Open Sona") {
			break
		}
		r.tt.Key(0, ui.KeyTab)
		r.tt.Frame()
	}
	r.tt.Key(ui.Shift, ui.KeyF10)
	r.tt.Frame()
	r.tt.Frame()
	r.tt.Want("Queue action", "Page action")

	r.tt.MustClick("Queue action")
	r.tt.Frame()
	r.tt.Frame()
	if got := r.desk.snapshot(); len(got) != 0 {
		t.Errorf("a queue action called the window: %v", got)
	}
	_ = r.tt.RightClick("Song Title")
	r.tt.Frame()
	r.tt.MustClick("Page action")
	r.tt.Frame()
	r.tt.Frame()
	if got := r.desk.wait(t); len(got) != 1 || got[0] != "show main" {
		t.Errorf("window calls %v, want the main window brought back", got)
	}
	if len(ran) != 2 {
		t.Errorf("ran %q", ran)
	}
}
