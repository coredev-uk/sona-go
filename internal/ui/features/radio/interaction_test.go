package radio

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
)

// The interaction rule under test: choosing a result opens it; Play plays; a
// modifier never does something a visible cue does not hint at. Each test
// drives one Radio surface and pins what every gesture does today. A line
// marked DEPARTS is where today's behaviour does not follow the rule.

// gesture is what a user does to a station.
type gesture string

const (
	gClick      gesture = "click"
	gDouble     gesture = "double-click"
	gEnter      gesture = "Enter"
	gShiftEnter gesture = "Shift+Enter"
	gSpace      gesture = "Space"
	gPlay       gesture = "Play control"
)

var allGestures = []gesture{gClick, gDouble, gEnter, gShiftEnter, gSpace, gPlay}

// target names what a gesture is aimed at: the text a pointer lands on, the
// name of the element the keyboard reaches, and the name of its play control.
type target struct {
	text, key, play string
}

// did is what happened so far: the page it went to, the stations played and
// how often playback was toggled.
func (rg *rig) did() string {
	var parts []string
	if p := rg.router.Path(); p != radioPath {
		parts = append(parts, "nav "+p)
	}
	for _, id := range rg.api.plays() {
		parts = append(parts, "play "+id)
	}
	if n, _ := rg.pb.counts(); n > 0 {
		parts = append(parts, fmt.Sprintf("toggle x%d", n))
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, "; ")
}

// settle waits for what a gesture asked of Go off the main thread.
func (rg *rig) settle() {
	time.Sleep(80 * time.Millisecond)
	rg.tt.Drain()
}

// focusOn tabs until the focus is on the element named label.
func (rg *rig) focusOn(label string) bool {
	for range 60 {
		rg.tt.Key(0, ui.KeyTab)
		rg.tt.Drain()
		for _, a := range rg.tt.AccessAll() {
			if a.Focused && (a.Name == label || strings.HasPrefix(a.Name, label+" ")) {
				return true
			}
		}
	}
	return false
}

// do performs a gesture on tg and says what it did.
func (rg *rig) do(g gesture, tg target) string {
	rg.t.Helper()
	n, ok := rg.tt.Find(tg.text)
	if !ok {
		rg.t.Fatalf("no %q; texts %q", tg.text, rg.tt.Texts())
	}
	x, y := n.X+n.W/2, n.Y+n.H/2
	switch g {
	case gClick, gDouble:
		rg.tt.ClickAt(x, y)
		rg.tt.Drain()
		if g == gDouble {
			rg.tt.ClickAt(x, y)
			rg.tt.Drain()
		}
	case gEnter, gShiftEnter, gSpace:
		if !rg.focusOn(tg.key) {
			return "not focusable"
		}
		switch g {
		case gEnter:
			rg.tt.Key(0, ui.KeyEnter)
		case gShiftEnter:
			rg.tt.Key(ui.Shift, ui.KeyEnter)
		case gSpace:
			rg.tt.Key(0, ui.KeySpace)
		}
		rg.tt.Drain()
	case gPlay:
		rg.tt.Move(x, y)
		rg.tt.Drain()
		if err := rg.tt.Click(tg.play); err != nil {
			return "no play control"
		}
		rg.tt.Drain()
	}
	rg.settle()
	return rg.did()
}

// cue is the role of the element named label for assistive technology: what a
// link, a button, a group or a row tells a user it will do.
func (rg *rig) cue(label string) string {
	named := func(name string) bool {
		return name == label || name == label+" Play" || name == label+" Pause" || strings.HasPrefix(name, label+", ")
	}
	for _, role := range []struct {
		r    ui.Role
		name string
	}{{ui.RoleLink, "link"}, {ui.RoleButton, "button"}, {ui.RoleGroup, "group"}, {ui.RoleListItem, "listitem"}, {ui.RoleRow, "row"}} {
		for _, a := range rg.tt.AccessAll() {
			if a.Role == role.r && named(a.Name) {
				return role.name
			}
		}
	}
	return "none"
}

// stationPlayer registers what New registers for the shared item players: a
// station anywhere (a tile, a shelf, a list) plays through the page's radio.
func stationPlayer(rg *rig) {
	rg.h.Env.Players.Register(services.ItemStation, func(it services.Item) {
		id := it.ID
		if id == "" && it.Station != nil {
			id = it.Station.ID
		}
		rg.r.playStation(id, nil)
	})
}

// surface is one place Radio shows a station, over a page that holds it.
type surface struct {
	name  string
	build func(t *testing.T, kind string) (*rig, target)
}

// kindStation is the synthetic station of a kind: with or without a page.
func kindStation(kind string) services.Station {
	switch strings.TrimPrefix(kind, "tuned-") {
	case "page":
		return liveStation("ra.p", "Live Page")
	case "nopage":
		return liveStation("ra.n", "Live Plain", func(s *services.Station) { s.HasPage = false })
	}
	return liveStation("ra.u", "Live Off", func(s *services.Station) {
		s.HasPage, s.Playable, s.Unavailable = false, false, services.StationNoWidevine
	})
}

// namedStation is a station of kind with name and ID, for the tiles and lists.
func namedStation(kind, id, name string, tracks bool) services.Station {
	s := kindStation(kind)
	s.ID, s.Name = id, name
	if tracks {
		s.Kind = services.StationTracks
	}
	return s
}

func surfaces() []surface {
	return []surface{
		{"board", func(t *testing.T, kind string) (*rig, target) {
			s := kindStation(kind)
			rg := newRig(t, radioPath, 1132, 1253, stationPlayer, fake(func(f *fakeRadio) {
				f.live = []services.Station{s}
				f.details[s.ID] = services.StationDetail{Station: s}
			}))
			if strings.HasPrefix(kind, "tuned-") {
				rg.playing(s.ID, "playing")
			}
			rg.tt.Until("the lane", rg.tt.Shows(s.Name))
			key, action := s.Name, "Play"
			if !s.HasPage {
				key = s.Name + " " + action
				if strings.HasPrefix(kind, "tuned-") {
					key = s.Name + " Pause"
				}
			}
			return rg, target{text: s.Name, key: key, play: "Play " + s.Name}
		}},
		{"yours", func(t *testing.T, kind string) (*rig, target) {
			s := namedStation(kind, "ra.y", "Yours "+kind, true)
			rg := newRig(t, radioPath, 1132, 1253, signedIn, stationPlayer, fake(func(f *fakeRadio) {
				f.recent = []services.Station{s}
			}))
			rg.tt.Until("the tile", rg.tt.Shows(s.Name))
			return rg, target{text: s.Name, key: s.Name, play: "Play " + s.Name}
		}},
		{"genreShelf", func(t *testing.T, kind string) (*rig, target) {
			s := namedStation(kind, "ra.g", "Genre "+kind, true)
			rg := newRig(t, radioPath, 1132, 1253, stationPlayer, fake(func(f *fakeRadio) {
				f.grouping = services.EditorialPage{Sections: []services.Section{
					section("Stations by Genre", itemsOf(s, trackStation("ra.x", "Other Genre"))),
				}}
			}))
			rg.tt.Until("the tile", rg.tt.Shows(s.Name))
			return rg, target{text: s.Name, key: s.Name, play: "Play " + s.Name}
		}},
		{"seeAll", func(t *testing.T, kind string) (*rig, target) {
			s := namedStation("page", "ra.b", "Local Station", false)
			rg := newRig(t, radioPath, 1132, 1253, stationPlayer, fake(func(f *fakeRadio) {
				f.grouping = services.EditorialPage{Sections: []services.Section{
					{ID: "local", Kind: services.SectionStations, Title: "Local Broadcasters", Room: "local", Items: itemsOf(s)},
				}}
			}))
			rg.tt.Until("the heading", rg.tt.Shows("Local Broadcasters", "See all"))
			return rg, target{text: "See all", key: "See all", play: "Play See all"}
		}},
		{"broadcasters", func(t *testing.T, kind string) (*rig, target) {
			s := namedStation(kind, "ra.b", "Local "+kind, false)
			s.Kind = services.StationBroadcast
			rg := newRig(t, radioPath, 1132, 1253, stationPlayer, fake(func(f *fakeRadio) {
				f.grouping = services.EditorialPage{Sections: []services.Section{
					section("Local Broadcasters", itemsOf(s), services.SectionStations),
				}}
			}))
			rg.tt.Until("the row", rg.tt.Shows(s.Name))
			key := s.Name
			if !s.HasPage {
				key = s.Name + " Play"
			}
			return rg, target{text: s.Name, key: key, play: "Play " + s.Name}
		}},
	}
}

// outcomes is what each gesture does to one station, and what its cue is.
type outcomes struct {
	click, double, enter, shiftEnter, space, play, cue string
}

func (o outcomes) of(g gesture) string {
	switch g {
	case gClick:
		return o.click
	case gDouble:
		return o.double
	case gEnter:
		return o.enter
	case gShiftEnter:
		return o.shiftEnter
	case gSpace:
		return o.space
	}
	return o.play
}

// radioWant is what each gesture does to a station, by surface and kind, as the code behaves today.
var radioWant = map[string]map[string]outcomes{
	"seeAll": {
		"page": {
			click:      "nav /room/local",
			double:     "nav /room/local",
			enter:      "nav /room/local",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "no play control",
			cue:        "link",
		},
	},
	"board": {
		"page": {
			click:      "nav /station/ra.p",
			double:     "nav /station/ra.p",
			enter:      "nav /station/ra.p",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "play ra.p",
			cue:        "link",
		},
		"nopage": {
			click: "play ra.n",
			// DEPARTS: the second click of a double-click fires again: the station is started twice
			double:     "play ra.n; play ra.n",
			enter:      "play ra.n",
			shiftEnter: "nothing",
			space:      "play ra.n",
			// DEPARTS: a page-less live station is played by its whole lane and also has a Play button; the tile, shelf and list rows of a page-less station have its name as the one control (onair.go:203)
			play: "play ra.n",
			cue:  "button",
		},
		"unplayable": {
			click:      "nothing",
			double:     "nothing",
			enter:      "not focusable",
			shiftEnter: "not focusable",
			space:      "not focusable",
			play:       "no play control",
			cue:        "button",
		},
		"tuned-page": {
			click:      "nav /station/ra.p",
			double:     "nav /station/ra.p",
			enter:      "nav /station/ra.p",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "no play control",
			cue:        "link",
		},
		"tuned-nopage": {
			click: "toggle x1",
			// DEPARTS: the second click of a double-click toggles playback again: the station pauses and resumes at once
			double:     "toggle x2",
			enter:      "toggle x1",
			shiftEnter: "nothing",
			space:      "toggle x1",
			play:       "no play control",
			cue:        "button",
		},
	},
	"yours": {
		"page": {
			click:      "nav /station/ra.y",
			double:     "nav /station/ra.y",
			enter:      "nav /station/ra.y",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "play ra.y",
			cue:        "link",
		},
		"nopage": {
			click: "play ra.y",
			// DEPARTS: the second click of a double-click fires again: the station is started twice
			double:     "play ra.y; play ra.y",
			enter:      "play ra.y",
			shiftEnter: "nothing",
			space:      "play ra.y",
			play:       "no play control",
			cue:        "button",
		},
	},
	"genreShelf": {
		"page": {
			click:      "nav /station/ra.g",
			double:     "nav /station/ra.g",
			enter:      "nav /station/ra.g",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "play ra.g",
			cue:        "link",
		},
		"nopage": {
			click: "play ra.g",
			// DEPARTS: the second click of a double-click fires again: the station is started twice
			double:     "play ra.g; play ra.g",
			enter:      "play ra.g",
			shiftEnter: "nothing",
			space:      "play ra.g",
			play:       "no play control",
			cue:        "button",
		},
	},
	"broadcasters": {
		"page": {
			click:      "nav /station/ra.b",
			double:     "nav /station/ra.b",
			enter:      "nav /station/ra.b",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "play ra.b",
			cue:        "link",
		},
		"nopage": {
			click: "play ra.b",
			// DEPARTS: the second click of a double-click fires again: the station is started twice
			double:     "play ra.b; play ra.b",
			enter:      "play ra.b",
			shiftEnter: "nothing",
			space:      "play ra.b",
			play:       "no play control",
			cue:        "button",
		},
	},
}

func TestInteractionRadioGestures(t *testing.T) {
	for _, s := range surfaces() {
		for _, kind := range slices.Sorted(maps.Keys(radioWant[s.name])) {
			want := radioWant[s.name][kind]
			t.Run(s.name+"/"+kind, func(t *testing.T) {
				for _, g := range allGestures {
					rg, tg := s.build(t, kind)
					if g == gClick {
						if got := rg.cue(tg.key); got != want.cue {
							t.Errorf("%s of a %s: its cue is %q, want %q", s.name, kind, got, want.cue)
						}
					}
					if got := rg.do(g, tg); got != want.of(g) {
						t.Errorf("%s of a %s on %s: did %q, want %q", g, kind, s.name, got, want.of(g))
					}
				}
			})
		}
	}
}

// A programme's box on the board links to the station's page, as the lane
// does, but is no Tab stop: the keyboard reaches the page through the lane.
func TestInteractionRadioProgrammeBox(t *testing.T) {
	t0 := base()
	one := appleMusic1
	details := map[string]services.StationDetail{"ra.1": {Station: one, Schedule: []services.StationEvent{
		stationEvent("The Zane Lowe Show", t0, t0.Add(time.Hour)),
		stationEvent("The Matt Wilkinson Show", t0.Add(time.Hour), t0.Add(2*time.Hour)),
	}}}
	want := map[gesture]string{
		gClick:      "nav /station/ra.1",
		gDouble:     "nav /station/ra.1",
		gEnter:      "not focusable",
		gShiftEnter: "not focusable",
		gSpace:      "not focusable",
	}
	for _, g := range []gesture{gClick, gDouble, gEnter, gShiftEnter, gSpace} {
		rg := boardRig(t, []services.Station{one}, details)
		rg.tt.Until("the programme", rg.tt.Shows("The Zane Lowe Show"))
		tg := target{text: "The Zane Lowe Show", key: "The Zane Lowe Show"}
		if got := rg.do(g, tg); got != want[g] {
			t.Errorf("%s on a programme box did %q, want %q", g, got, want[g])
		}
	}
}
