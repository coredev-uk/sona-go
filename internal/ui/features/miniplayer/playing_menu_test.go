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
		r.want(t, "Song action")
		r.wantNot(t, "Station action")
		r.tt.Key(0, ui.KeyEscape)
		r.tt.Frame()
	}
	if err := r.tt.RightClick("Song Title"); err != nil {
		t.Fatal(err)
	}
	r.tt.Frame()
	if err := r.tt.Click("Song action"); err != nil {
		t.Fatal(err)
	}
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
	r.want(t, "Station action")
	r.wantNot(t, "Song action")
}
