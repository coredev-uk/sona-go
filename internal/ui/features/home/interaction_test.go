package home

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// The interaction rule under test: choosing a result opens it; Play plays; a
// track list's row plays (double-click or Enter); a modifier never does
// something a visible cue does not hint at. Each test drives one Home surface
// per item kind and pins what every gesture does today. A line marked
// DEPARTS is where today's behaviour does not follow the rule.

// gesture is what a user does to an item.
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

// kindItems are one synthetic item per kind Home meets.
func kindItems() map[string]services.Item {
	art := coverArt(7)
	return map[string]services.Item{
		"song": {
			Kind: services.ItemSong, ID: "s1", Name: "Song One", Subtitle: "Artist S", Art: art,
			Song: &services.Song{ID: "s1", Name: "Song One", Artist: "Artist S", Playable: true},
		},
		"album":    albumItem("al1", "Album One", "Artist A", 1),
		"playlist": playlistItem("pl1", "Playlist One", "Curator P", 2),
		"artist": {
			Kind: services.ItemArtist, ID: "ar1", Name: "Artist One", Art: art,
			Artist: &services.Artist{ID: "ar1", Name: "Artist One"},
		},
		"station":        stationItem("sp1", "Station Page", 3, true),
		"station-nopage": stationItem("sn1", "Station Plain", 4, false),
		"video": {
			Kind: services.ItemMusicVideo, ID: "v1", Name: "Video One", Subtitle: "Artist V", Art: art,
			MusicVideo: &services.MusicVideo{ID: "v1", Name: "Video One", Artist: "Artist V"},
		},
		"curator":  {Kind: services.ItemCurator, ID: "cu1", Name: "Curator One", Art: art},
		"grouping": {Kind: services.ItemGrouping, ID: "g1", Name: "Grouping One", Art: art},
		"room":     {Kind: services.ItemRoom, ID: "ro1", Name: "Room One", Art: art},
		"link":     {Kind: services.ItemLink, Name: "Link One", URL: "https://example.test/interview/1", Art: art},
	}
}

// iaRig is Home with every page served and videos playable.
func iaRig(t *testing.T, page services.HomePage, opts ...rigOpt) *rig {
	t.Helper()
	opts = append([]rigOpt{withHome(page), withRoutes(func(string) bool { return true })}, opts...)
	r := newRig(t, "/", 1132, 1400, opts...)
	r.h.Env.Players.RegisterVideo(func(v services.MusicVideo, _ *ui.Handle) { r.rec.add("video:" + v.ID) })
	r.h.Env.Players.Capabilities = func() *services.Capabilities {
		c := new(services.Capabilities)
		c.Playback.Video = true
		return c
	}
	return r
}

// did is what happened so far: the page it went to, what played, what
// opened in the browser.
func (r *rig) did() string {
	if r.tt.HasText("Link One") {
		time.Sleep(50 * time.Millisecond) // the browser is asked off the main thread
	}
	var parts []string
	if p := r.path(); p != "/" {
		parts = append(parts, "nav "+p)
	}
	for _, p := range r.rec.all() {
		parts = append(parts, "play "+p)
	}
	for _, u := range r.h.Opened() {
		parts = append(parts, "url "+u)
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, "; ")
}

// focusOn tabs until the focus is on the element named label.
func (r *rig) focusOn(label string) bool {
	for range 60 {
		r.tt.Key(0, ui.KeyTab)
		r.tt.Drain()
		for _, a := range r.tt.AccessAll() {
			if a.Focused && (a.Name == label || strings.HasPrefix(a.Name, label+" ")) {
				return true
			}
		}
	}
	return false
}

// do performs a gesture on the element named label and says what it did.
func (r *rig) do(g gesture, label string, track bool) string {
	r.t.Helper()
	n, ok := r.tt.Find(label)
	if !ok {
		r.t.Fatalf("no %q; texts %q", label, r.tt.Texts())
	}
	x, y := n.X+n.W/2, n.Y+n.H/2
	if track {
		x = n.X + 100 // the title, not the artist or album link beside it
	}
	switch g {
	case gClick, gDouble:
		r.tt.ClickAt(x, y)
		r.tt.Drain()
		if g == gDouble {
			// The second click lands where the first left the page.
			r.tt.ClickAt(x, y)
			r.tt.Drain()
		}
	case gEnter, gShiftEnter, gSpace:
		if !r.focusOn(label) {
			return "not focusable"
		}
		switch g {
		case gEnter:
			r.tt.Key(0, ui.KeyEnter)
		case gShiftEnter:
			r.tt.Key(ui.Shift, ui.KeyEnter)
		case gSpace:
			r.tt.Key(0, ui.KeySpace)
		}
		r.tt.Drain()
	case gPlay:
		r.tt.Move(x, y) // a row's play control takes input only while its row is hovered
		r.tt.Drain()
		if err := r.tt.Click("Play " + label); err != nil {
			return "no play control"
		}
		r.tt.Drain()
	}
	return r.did()
}

// cue is the role of the element named label for assistive technology: what a
// link, a button, a group or a row tells a user it will do.
func (r *rig) cue(label string) string {
	named := func(name string) bool {
		return name == label || name == label+" Play" || name == label+" Pause" || name == label+" Opens in your browser" || strings.HasPrefix(name, label+", ")
	}
	for _, role := range []struct {
		r    ui.Role
		name string
	}{{ui.RoleLink, "link"}, {ui.RoleButton, "button"}, {ui.RoleGroup, "group"}, {ui.RoleListItem, "listitem"}, {ui.RoleRow, "row"}} {
		for _, a := range r.tt.AccessAll() {
			if a.Role == role.r && named(a.Name) {
				return role.name
			}
		}
	}
	return "none"
}

// surface is one place Home shows items, over a page that holds the item.
type surface struct {
	name string
	// track: the item is a row of a track list, which a click on its middle
	// would not reach the title of.
	track bool
	// label names the element to drive when it is not the item's name.
	label string
	build func(t *testing.T, it services.Item) *rig
}

// iaPage is a page of one section of kind sk holding it and a filler.
func iaPage(sk string, it services.Item, filler services.Item, source string) services.HomePage {
	s := section("s", sk, "Shelf", it, filler)
	s.Source = source
	return services.HomePage{Sections: []services.Section{s}}
}

var fillerAlb = albumItem("fa", "Filler Album", "Filler", 9)

func surfaces() []surface {
	return []surface{
		{"shelf", false, "", func(t *testing.T, it services.Item) *rig {
			return iaRig(t, iaPage(services.SectionShelf, it, fillerAlb, ""))
		}},
		{"strip", false, "", func(t *testing.T, it services.Item) *rig {
			return iaRig(t, iaPage(services.SectionShelf, it, fillerAlb, recentlyPlayed))
		}},
		{"stationRows", false, "", func(t *testing.T, it services.Item) *rig {
			return iaRig(t, iaPage(services.SectionShelf, it, stationItem("fs", "Filler Station", 8, true), ""))
		}},
		{"leadPick", false, "", func(t *testing.T, it services.Item) *rig {
			return iaRig(t, iaPage(services.SectionFeature, it, fillerAlb, ""))
		}},
		{"leadOrder", false, "", func(t *testing.T, it services.Item) *rig {
			s := section("s", services.SectionFeature, "Top", albumItem("lead", "Lead Album", "Lead", 5), it, fillerAlb)
			return iaRig(t, services.HomePage{Sections: []services.Section{s}})
		}},
		{"hero", false, "", func(t *testing.T, it services.Item) *rig {
			return iaRig(t, services.HomePage{Sections: []services.Section{section("h", services.SectionHero, "", it)}})
		}},
		{"heroMany", false, "", func(t *testing.T, it services.Item) *rig {
			return iaRig(t, iaPage(services.SectionHero, it, fillerAlb, ""))
		}},
		{"tracks", true, "", func(t *testing.T, it services.Item) *rig {
			two := it
			two.ID, two.Name, two.Song = "s2", "Song Two", &services.Song{ID: "s2", Name: "Song Two", Playable: true}
			return iaRig(t, iaPage(services.SectionTracks, it, two, ""))
		}},
		{"links", false, "", func(t *testing.T, it services.Item) *rig {
			return iaRig(t, iaPage(services.SectionLinks, it, fillerAlb, ""))
		}},
		{"bricks", false, "", func(t *testing.T, it services.Item) *rig {
			return iaRig(t, iaPage(services.SectionBricks, it, fillerAlb, ""))
		}},
		{"stationList", false, "", func(t *testing.T, it services.Item) *rig {
			return iaRig(t, iaPage(services.SectionStations, it, stationItem("fs", "Filler Station", 8, true), ""))
		}},
		{"recentSongs", true, "", func(t *testing.T, it services.Item) *rig {
			return iaRig(t, services.HomePage{}, withLib(func(l *fakeLib) {
				l.recent = []services.Song{*it.Song, {ID: "s2", Name: "Song Two", Playable: true}}
			}))
		}},
		{"recentlyAdded", false, "", func(t *testing.T, it services.Item) *rig {
			return iaRig(t, services.HomePage{}, withLib(func(l *fakeLib) {
				l.albums = []services.Album{*it.Album, {ID: "al2", Name: "Album Two"}}
			}))
		}},
		{"seeAll", false, "See all", func(t *testing.T, it services.Item) *rig {
			sec := section("s", services.SectionShelf, "Shelf", it, fillerAlb)
			sec.SeeAll = true
			return iaRig(t, services.HomePage{Sections: []services.Section{sec}})
		}},
		{"recentPlaylists", false, "", func(t *testing.T, it services.Item) *rig {
			return iaRig(t, services.HomePage{}, withLib(func(l *fakeLib) {
				l.playlists = []services.Playlist{*it.Playlist, {ID: "pl2", Name: "Playlist Two"}}
			}))
		}},
	}
}

// outcomes is what each gesture does to one item, and what its cue is.
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

// homeWant is what each gesture does to an item, by surface and kind, as the code behaves today.
var homeWant = map[string]map[string]outcomes{
	"seeAll": {
		"album": {
			click:      "nav /for-you/s",
			double:     "nav /for-you/s",
			enter:      "nav /for-you/s",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "no play control",
			cue:        "link",
		},
	},
	"shelf": {
		"song": {
			click:      "nav /song/s1",
			double:     "nav /song/s1",
			enter:      "nav /song/s1",
			shiftEnter: "nothing",
			// DEPARTS: Space does nothing here (a role=link takes Enter only) but plays the same item in the strip, the station rows and the lead's order, whose hint reads "Enter opens, Space plays" (rows.go:395 against kit/collections_cover.go:327, which handles only the click)
			space: "nothing",
			play:  "play song:s1",
			cue:   "link",
		},
		"album": {
			click:      "nav /album/al1",
			double:     "nav /album/al1",
			enter:      "nav /album/al1",
			shiftEnter: "nothing",
			// DEPARTS: Space does nothing here (a role=link takes Enter only) but plays the same item in the strip, the station rows and the lead's order, whose hint reads "Enter opens, Space plays" (rows.go:395 against kit/collections_cover.go:327, which handles only the click)
			space: "nothing",
			play:  "play album:al1",
			cue:   "link",
		},
		"playlist": {
			click:      "nav /playlist/pl1",
			double:     "nav /playlist/pl1",
			enter:      "nav /playlist/pl1",
			shiftEnter: "nothing",
			// DEPARTS: Space does nothing here (a role=link takes Enter only) but plays the same item in the strip, the station rows and the lead's order, whose hint reads "Enter opens, Space plays" (rows.go:395 against kit/collections_cover.go:327, which handles only the click)
			space: "nothing",
			play:  "play playlist:pl1",
			cue:   "link",
		},
		"artist": {
			click:      "nav /artist/ar1",
			double:     "nav /artist/ar1",
			enter:      "nav /artist/ar1",
			shiftEnter: "nothing",
			space:      "nothing",
			// Artists and curators open only: no player is registered for them, so no tile, card or row draws a Play control (design.md, "Artists and curators open only").
			play: "no play control",
			cue:  "link",
		},
		"station": {
			click:      "nav /station/sp1",
			double:     "nav /station/sp1",
			enter:      "nav /station/sp1",
			shiftEnter: "nothing",
			// DEPARTS: Space does nothing here (a role=link takes Enter only) but plays the same item in the strip, the station rows and the lead's order, whose hint reads "Enter opens, Space plays" (rows.go:395 against kit/collections_cover.go:327, which handles only the click)
			space: "nothing",
			play:  "play station:sp1",
			cue:   "link",
		},
		"station-nopage": {
			click:      "play station:sn1",
			double:     "play station:sn1",
			enter:      "play station:sn1",
			shiftEnter: "nothing",
			space:      "play station:sn1",
			play:       "no play control",
			cue:        "button",
		},
		"video": {
			click:      "nav /music-video/v1",
			double:     "nav /music-video/v1",
			enter:      "nav /music-video/v1",
			shiftEnter: "nothing",
			// DEPARTS: Space does nothing here (a role=link takes Enter only) but plays the same item in the strip, the station rows and the lead's order, whose hint reads "Enter opens, Space plays" (rows.go:395 against kit/collections_cover.go:327, which handles only the click)
			space: "nothing",
			play:  "play video:v1",
			cue:   "link",
		},
		"curator": {
			click:      "nav /curator/cu1",
			double:     "nav /curator/cu1",
			enter:      "nav /curator/cu1",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "no play control",
			cue:        "link",
		},
		"grouping": {
			click:      "nav /grouping/g1",
			double:     "nav /grouping/g1",
			enter:      "nav /grouping/g1",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "no play control",
			cue:        "link",
		},
		"room": {
			click:      "nav /room/ro1",
			double:     "nav /room/ro1",
			enter:      "nav /room/ro1",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "no play control",
			cue:        "link",
		},
		"link": {
			click:      "url https://example.test/interview/1",
			double:     "url https://example.test/interview/1",
			enter:      "url https://example.test/interview/1",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "no play control",
			cue:        "link",
		},
	},
	"strip": {
		"song": {
			click:      "nav /song/s1",
			double:     "nav /song/s1",
			enter:      "nav /song/s1",
			shiftEnter: "nothing",
			space:      "play song:s1",
			play:       "play song:s1",
			cue:        "link",
		},
		"album": {
			click:      "nav /album/al1",
			double:     "nav /album/al1",
			enter:      "nav /album/al1",
			shiftEnter: "nothing",
			space:      "play album:al1",
			play:       "play album:al1",
			cue:        "link",
		},
		"artist": {
			click:      "nav /artist/ar1",
			double:     "nav /artist/ar1",
			enter:      "nav /artist/ar1",
			shiftEnter: "nothing",
			space:      "nothing",
			// Artists and curators open only: no player is registered for them, so no tile, card or row draws a Play control (design.md, "Artists and curators open only").
			play: "no play control",
			cue:  "link",
		},
		"station-nopage": {
			click:      "play station:sn1",
			double:     "play station:sn1",
			enter:      "play station:sn1",
			shiftEnter: "nothing",
			space:      "play station:sn1",
			play:       "no play control",
			cue:        "button",
		},
		"link": {
			click:      "url https://example.test/interview/1",
			double:     "url https://example.test/interview/1",
			enter:      "url https://example.test/interview/1",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "no play control",
			cue:        "link",
		},
	},
	"stationRows": {
		"station": {
			click:      "nav /station/sp1",
			double:     "nav /station/sp1",
			enter:      "nav /station/sp1",
			shiftEnter: "nothing",
			space:      "play station:sp1",
			play:       "play station:sp1",
			cue:        "link",
		},
		"station-nopage": {
			click:      "play station:sn1",
			double:     "play station:sn1",
			enter:      "play station:sn1",
			shiftEnter: "nothing",
			space:      "play station:sn1",
			play:       "no play control",
			cue:        "button",
		},
	},
	"leadPick": {
		"album": {
			click:      "nav /album/al1",
			double:     "nav /album/al1",
			enter:      "nav /album/al1",
			shiftEnter: "nothing",
			// DEPARTS: Space does nothing here (a role=link takes Enter only) but plays the same item in the strip, the station rows and the lead's order, whose hint reads "Enter opens, Space plays" (rows.go:395 against kit/collections_cover.go:327, which handles only the click)
			space: "nothing",
			play:  "play album:al1",
			cue:   "link",
		},
		"artist": {
			click:      "nav /artist/ar1",
			double:     "nav /artist/ar1",
			enter:      "nav /artist/ar1",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "no play control", // the lead pick draws no Play control for a kind that cannot play, as the hero card and the tiles
			cue:        "link",
		},
		"station-nopage": {
			click:      "play station:sn1",
			double:     "play station:sn1",
			enter:      "play station:sn1",
			shiftEnter: "nothing",
			space:      "play station:sn1",
			play:       "no play control",
			cue:        "button",
		},
		"curator": {
			click:      "nav /curator/cu1",
			double:     "nav /curator/cu1",
			enter:      "nav /curator/cu1",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "no play control", // the lead pick draws no Play control for a kind that cannot play, as the hero card and the tiles
			cue:        "link",
		},
	},
	"leadOrder": {
		"album": {
			click:      "nav /album/al1",
			double:     "nav /album/al1",
			enter:      "nav /album/al1",
			shiftEnter: "nothing",
			space:      "play album:al1",
			play:       "play album:al1",
			cue:        "link",
		},
		"artist": {
			click:      "nav /artist/ar1",
			double:     "nav /artist/ar1",
			enter:      "nav /artist/ar1",
			shiftEnter: "nothing",
			space:      "nothing",
			// Artists and curators open only: no player is registered for them, so no tile, card or row draws a Play control (design.md, "Artists and curators open only").
			play: "no play control",
			cue:  "link",
		},
		"station-nopage": {
			click:      "play station:sn1",
			double:     "play station:sn1",
			enter:      "play station:sn1",
			shiftEnter: "nothing",
			space:      "play station:sn1",
			play:       "no play control",
			cue:        "button",
		},
	},
	"hero": {
		"station-nopage": {
			click:      "play station:sn1",
			double:     "play station:sn1",
			enter:      "play station:sn1",
			shiftEnter: "nothing",
			space:      "play station:sn1",
			play:       "no play control",
			cue:        "button",
		},
		"album": {
			click:      "nav /album/al1",
			double:     "nav /album/al1",
			enter:      "nav /album/al1",
			shiftEnter: "nothing",
			// DEPARTS: Space does nothing here (a role=link takes Enter only) but plays the same item in the strip, the station rows and the lead's order, whose hint reads "Enter opens, Space plays" (rows.go:395 against kit/collections_cover.go:327, which handles only the click)
			space: "nothing",
			play:  "play album:al1",
			cue:   "link",
		},
		"curator": {
			click:      "nav /curator/cu1",
			double:     "nav /curator/cu1",
			enter:      "nav /curator/cu1",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "no play control",
			cue:        "link",
		},
	},
	"heroMany": {
		"album": {
			click:      "nav /album/al1",
			double:     "nav /album/al1",
			enter:      "nav /album/al1",
			shiftEnter: "nothing",
			// DEPARTS: Space does nothing here (a role=link takes Enter only) but plays the same item in the strip, the station rows and the lead's order, whose hint reads "Enter opens, Space plays" (rows.go:395 against kit/collections_cover.go:327, which handles only the click)
			space: "nothing",
			play:  "play album:al1",
			cue:   "link",
		},
		"station-nopage": {
			click:      "play station:sn1",
			double:     "play station:sn1",
			enter:      "play station:sn1",
			shiftEnter: "nothing",
			space:      "play station:sn1",
			play:       "no play control",
			cue:        "button",
		},
	},
	"tracks": {
		"song": {
			click:      "nothing",
			double:     "play songs:Shelf@0/2",
			enter:      "play songs:Shelf@0/2",
			shiftEnter: "nothing",
			// DEPARTS: Space does nothing on a focused track row, though Space plays on Home's item rows
			space: "nothing",
			play:  "play songs:Shelf@0/2",
			cue:   "listitem",
		},
	},
	"links": {
		"album": {
			click:      "nav /album/al1",
			double:     "nav /album/al1",
			enter:      "nav /album/al1",
			shiftEnter: "nothing",
			// DEPARTS: Space does nothing here (a role=link takes Enter only) but plays the same item in the strip, the station rows and the lead's order, whose hint reads "Enter opens, Space plays" (rows.go:395 against kit/collections_cover.go:327, which handles only the click)
			space: "nothing",
			play:  "no play control",
			cue:   "link",
		},
		"station-nopage": {
			click:      "play station:sn1",
			double:     "play station:sn1",
			enter:      "play station:sn1",
			shiftEnter: "nothing",
			space:      "play station:sn1",
			play:       "no play control",
			cue:        "button",
		},
		"link": {
			click:      "url https://example.test/interview/1",
			double:     "url https://example.test/interview/1",
			enter:      "url https://example.test/interview/1",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "no play control",
			cue:        "link",
		},
	},
	"bricks": {
		"room": {
			click:      "nav /room/ro1",
			double:     "nav /room/ro1",
			enter:      "nav /room/ro1",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "no play control",
			cue:        "link",
		},
	},
	"stationList": {
		"station": {
			click:      "nav /station/sp1",
			double:     "nav /station/sp1",
			enter:      "nav /station/sp1",
			shiftEnter: "nothing",
			// DEPARTS: Space does nothing here (a role=link takes Enter only) but plays the same item in the strip, the station rows and the lead's order, whose hint reads "Enter opens, Space plays" (rows.go:395 against kit/collections_cover.go:327, which handles only the click)
			space: "nothing",
			play:  "play station:sp1",
			cue:   "link",
		},
		"station-nopage": {
			click:      "play station:sn1",
			double:     "play station:sn1",
			enter:      "play station:sn1",
			shiftEnter: "nothing",
			space:      "play station:sn1",
			play:       "no play control",
			cue:        "button",
		},
	},
	"recentSongs": {
		"song": {
			click:      "nothing",
			double:     "play songs:recently-played@0/2",
			enter:      "play songs:recently-played@0/2",
			shiftEnter: "nothing",
			// DEPARTS: Space does nothing on a focused track row, though Space plays on Home's item rows
			space: "nothing",
			play:  "play songs:recently-played@0/2",
			cue:   "row",
		},
	},
	"recentlyAdded": {
		"album": {
			click:      "nav /album/al1",
			double:     "nav /album/al1",
			enter:      "nav /album/al1",
			shiftEnter: "nothing",
			space:      "nothing",
			// DEPARTS: no Play control on this local shelf's tile although the same album or playlist has one on Apple's shelves (local.go CoverItem has no Item or Play)
			play: "no play control",
			cue:  "link",
		},
	},
	"recentPlaylists": {
		"playlist": {
			click:      "nav /playlist/pl1",
			double:     "nav /playlist/pl1",
			enter:      "nav /playlist/pl1",
			shiftEnter: "nothing",
			space:      "nothing",
			// DEPARTS: no Play control on this local shelf's tile although the same album or playlist has one on Apple's shelves (local.go CoverItem has no Item or Play)
			play: "no play control",
			cue:  "link",
		},
	},
}

func TestInteractionHomeGestures(t *testing.T) {
	items := kindItems()
	for _, s := range surfaces() {
		for _, kind := range slices.Sorted(maps.Keys(homeWant[s.name])) {
			want := homeWant[s.name][kind]
			it := items[kind]
			label := it.Name
			if s.label != "" {
				label = s.label
			}
			t.Run(s.name+"/"+kind, func(t *testing.T) {
				for _, g := range allGestures {
					r := s.build(t, it)
					r.until("the item", label)
					if g == gClick {
						if got := r.cue(label); got != want.cue {
							t.Errorf("%s of a %s: its cue is %q, want %q", s.name, kind, got, want.cue)
						}
					}
					if got := r.do(g, label, s.track); got != want.of(g) {
						t.Errorf("%s of a %s on %s: did %q, want %q", g, kind, s.name, got, want.of(g))
					}
				}
			})
		}
	}
}

// accessNamed is whether an element for assistive technology is called name.
func (r *rig) accessNamed(name string) bool {
	for _, a := range r.tt.AccessAll() {
		if a.Name == name {
			return true
		}
	}
	return false
}

// A station with no page is one control on every surface that shows it: named
// by its text and "Play", and "Pause" once the station plays, then "Play"
// again when it is paused.
func TestPagelessStationControlReadsPauseWhilePlaying(t *testing.T) {
	it := kindItems()["station-nopage"]
	for _, s := range surfaces() {
		if _, ok := homeWant[s.name]["station-nopage"]; !ok {
			continue
		}
		t.Run(s.name, func(t *testing.T) {
			r := s.build(t, it)
			label := it.Name
			r.until("the item", label)
			if !r.accessNamed(label+" Play") && s.name != "stationList" && s.name != "stationRows" {
				t.Fatalf("not named %q before it plays; texts %q", label+" Play", r.tt.Texts())
			}
			r.h.Play(0, func(st *services.PlaybackState) { st.Current = &services.QueueItem{ID: it.Station.ID, Name: it.Name} })
			r.tt.Drain()
			if !r.accessNamed(label + " Pause") {
				t.Errorf("not named %q while it plays; texts %q", label+" Pause", r.tt.Texts())
			}
			if r.accessNamed(label + " Play") {
				t.Errorf("still named %q while it plays", label+" Play")
			}
			r.h.Play(0, func(st *services.PlaybackState) {
				st.Current, st.State = &services.QueueItem{ID: it.Station.ID, Name: it.Name}, store.StatePaused
			})
			r.tt.Drain()
			if !r.accessNamed(label + " Play") {
				t.Errorf("not named %q while it is paused; texts %q", label+" Play", r.tt.Texts())
			}
		})
	}
}
