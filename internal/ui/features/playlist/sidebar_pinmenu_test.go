package playlist

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// choices are the "When chosen" radios of the open menu, by label: "on" when
// checked, "off" when disabled (both when checked and disabled).
func (e *sbEnv) choices() map[string]string {
	e.t.Helper()
	out := map[string]string{}
	for _, n := range access.Read(e.t, e.tt.Tester).Roles(ui.RoleMenuItemRadio) {
		var s []string
		if n.Has(access.Checked) {
			s = append(s, "on")
		}
		if n.Has(access.Disabled) {
			s = append(s, "off")
		}
		out[n.Label] = strings.Join(s, "+")
	}
	return out
}

// A pin's menu offers only what the pin does: a playlist, an album and a song
// can Play, Shuffle or Open (the checked one is what a click does); an artist
// can only open, so it has no "When chosen" group at all.
func TestPinMenuOffersOnlyWhatThePinDoes(t *testing.T) { inBubble(t, pinMenuOffersOnlyWhatThePinDoes) }

func pinMenuOffersOnlyWhatThePinDoes(t *testing.T) {
	pins := []services.Pin{
		{Kind: "playlist", ID: "p.a", Name: "PL", Action: "shuffle"},
		{Kind: "album", ID: "l.a", Name: "AL", Action: "play"},
		{Kind: "song", ID: "i.a", Name: "SG", Action: "drillIn", AlbumID: "l.cc"},
		{Kind: "artist", ID: "r.a", Name: "AR", Action: "shuffle"},
	}
	e := sbSetup(t, sbLibrary(), pins)
	e.shows("PL, shuffles", "AL, plays", "SG", "AR")
	for _, c := range []struct {
		row, more string
		want      map[string]string
	}{
		{"PL, shuffles", "More for PL", map[string]string{"Play": "", "Shuffle": "on", "Open": ""}},
		{"AL, plays", "More for AL", map[string]string{"Play": "on", "Shuffle": "", "Open": ""}},
		{"SG", "More for SG", map[string]string{"Play": "", "Shuffle": "", "Open": "on"}},
		{"AR", "More for AR", map[string]string{}},
	} {
		e.more(c.row, c.more)
		got := e.choices()
		if len(got) != len(c.want) {
			t.Errorf("%s: choices %v, want %v", c.row, got, c.want)
		}
		for label, state := range c.want {
			if got[label] != state {
				t.Errorf("%s: %s is %q, want %q (all: %v)", c.row, label, got[label], state, got)
			}
		}
		if c.row == "AR" && e.tt.HasText("When chosen") {
			t.Errorf("an artist pin's menu offers \"When chosen\": %q", e.tt.Texts())
		}
		e.tt.Key(0, ui.KeyEscape)
		e.settle()
	}
}

// A pin set to Open whose page is not ported shows Open checked and off (it
// does nothing, and the menu does not say it plays), and Play and Shuffle
// stay open to change it to.
func TestPinMenuOfAPinWithoutItsPageChecksOpenAndOffersTheRest(t *testing.T) {
	inBubble(t, pinMenuOfAPinWithoutItsPage)
}

func pinMenuOfAPinWithoutItsPage(t *testing.T) {
	e := sbSetup(t, sbLibrary(), []services.Pin{
		{Kind: "album", ID: "l.b", Name: "Beta", Action: "drillIn"},
		{Kind: "song", ID: "i.s", Name: "Lone", Action: "drillIn"},
	})
	e.h.Env.HasRoute = func(path string) bool { return !strings.HasPrefix(path, "/album/") }
	e.shows("Beta", "Lone")
	for _, c := range []struct{ row, more string }{{"Beta", "More for Beta"}, {"Lone", "More for Lone"}} {
		e.more(c.row, c.more)
		want := map[string]string{"Play": "", "Shuffle": "", "Open": "on+off"}
		got := e.choices()
		for label, state := range want {
			if got[label] != state {
				t.Errorf("%s: %s is %q, want %q (all: %v)", c.row, label, got[label], state, got)
			}
		}
		if e.tt.HasText("Go to album") {
			t.Errorf("%s: offers Go to album with no album page: %q", c.row, e.tt.Texts())
		}
		e.tt.Key(0, ui.KeyEscape)
		e.settle()
	}
	// Choosing Play there is the way out.
	e.more("Beta", "More for Beta")
	e.choose("Play")
	e.did("SetPinAction l.b play")
}
