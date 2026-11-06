package kit

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

func TestButtonTipShowsItsKeysAsKeycaps(t *testing.T) {
	tt := controlsTester(t, 300, 120, func(c *ui.Context) {
		Button(c.Key("play"), "", ButtonOpts{Icon: "play", Variant: Ghost, A11yLabel: "Play", Keys: []string{"Ctrl+ArrowRight", "N"}})
		Button(c.Key("bare"), "", ButtonOpts{Icon: "pause", Variant: Ghost, A11yLabel: "Pause"})
	})
	r := controlsFind(t, tt, "Play")
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Advance(time.Second)
	// Only the first chord is drawn, one cap per key.
	for _, w := range []string{"Play", "Ctrl", "→"} {
		if !tt.HasText(w) {
			t.Errorf("the tip lacks %q: %q", w, tt.Texts())
		}
	}
	if tt.HasText("N") {
		t.Errorf("the second chord is drawn: %q", tt.Texts())
	}
	// Assistive technology reads the chord as the button's description.
	tr := access.Read(t, tt)
	if n, ok := tr.Find(ui.RoleButton, "Play"); !ok || n.Description != "Ctrl+Right Arrow" {
		t.Errorf("description %q (found %v)", n.Description, ok)
	}
	if n, ok := tr.Find(ui.RoleButton, "Pause"); !ok || n.Description != "" {
		t.Errorf("a button with no keys has the description %q", n.Description)
	}
}

func TestTooltipKeysWithNoChordsIsAPlainTooltip(t *testing.T) {
	tt := controlsTester(t, 300, 120, func(c *ui.Context) {
		TooltipKeys(c, ui.Box(c).Size(30, 30).Focusable().Label("Thing"), "Does a thing", nil)
	})
	r := controlsFind(t, tt, "Thing")
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Advance(time.Second)
	if !tt.HasText("Does a thing") {
		t.Errorf("no tip: %q", tt.Texts())
	}
}

func TestOnAirProgrammeFindsTheSlotAtNow(t *testing.T) {
	sched := []services.StationEvent{{ID: "a", StartMs: 100, EndMs: 200}, {ID: "b", StartMs: 200, EndMs: 300}}
	for _, c := range []struct {
		now  int64
		want string
	}{{100, "a"}, {199, "a"}, {200, "b"}, {300, ""}, {99, ""}} {
		got, ok := OnAirProgramme(sched, c.now)
		if ok != (c.want != "") || got.ID != c.want {
			t.Errorf("at %d: %q %v, want %q", c.now, got.ID, ok, c.want)
		}
	}
	if _, ok := OnAirProgramme(nil, 1); ok {
		t.Error("no schedule has nothing on air")
	}
}

func TestStationKeyIsTheRadioPagesKey(t *testing.T) {
	if got := StationKey("ra.1"); len(got) != 3 || got[0] != "radio" || got[1] != "station" || got[2] != "ra.1" {
		t.Errorf("key %v", got)
	}
}

func TestTooltipShowsAfterTheThemesDelay(t *testing.T) {
	tt := controlsTester(t, 300, 120, func(c *ui.Context) {
		Tooltip(c, ui.Box(c).Size(30, 30).Focusable().Label("Thing"), "Does a thing")
	})
	r := controlsFind(t, tt, "Thing")
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Advance(350 * time.Millisecond)
	if tt.HasText("Does a thing") {
		t.Errorf("the tip shows at 350 ms: %q", tt.Texts())
	}
	tt.Advance(50 * time.Millisecond)
	if !tt.HasText("Does a thing") {
		t.Errorf("no tip at 400 ms: %q", tt.Texts())
	}
}
