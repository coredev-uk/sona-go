package discover

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// The interaction rule under test: choosing a result opens it; Play plays; a
// track list's row plays (double-click or Enter); a modifier never does
// something a visible cue does not hint at. Each test drives one Discover
// surface per item kind and pins what every gesture does today. A line marked
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

// iaRec notes the items the registered players were asked to play.
type iaRec struct {
	mu    sync.Mutex
	plays []string
}

func (r *iaRec) add(s string) {
	r.mu.Lock()
	r.plays = append(r.plays, s)
	r.mu.Unlock()
}

func (r *iaRec) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.plays...)
}

// iaItems are one synthetic item per kind Discover meets.
func iaItems() map[string]services.Item {
	return map[string]services.Item{
		"song": {
			Kind: services.ItemSong, ID: "s1", Name: "Song One", Subtitle: "Artist S",
			Song: &services.Song{ID: "s1", Name: "Song One", Artist: "Artist S", Playable: true},
		},
		"album": {
			Kind: services.ItemAlbum, ID: "al1", Name: "Album One", Subtitle: "Artist A",
			Album: &services.Album{ID: "al1", Name: "Album One", Artist: "Artist A"},
		},
		"playlist": {
			Kind: services.ItemPlaylist, ID: "pl1", Name: "Playlist One", Subtitle: "Curator P",
			Playlist: &services.Playlist{ID: "pl1", Name: "Playlist One", Curator: "Curator P"},
		},
		"artist": {
			Kind: services.ItemArtist, ID: "ar1", Name: "Artist One",
			Artist: &services.Artist{ID: "ar1", Name: "Artist One"},
		},
		"station": {
			Kind: services.ItemStation, ID: "sp1", Name: "Station Page",
			Station: &services.Station{ID: "sp1", Name: "Station Page", Playable: true, HasPage: true, Kind: services.StationTracks},
		},
		"station-nopage": {
			Kind: services.ItemStation, ID: "sn1", Name: "Station Plain",
			Station: &services.Station{ID: "sn1", Name: "Station Plain", Playable: true, HasPage: false, Kind: services.StationTracks},
		},
		"video": {
			Kind: services.ItemMusicVideo, ID: "v1", Name: "Video One", Subtitle: "Artist V",
			MusicVideo: &services.MusicVideo{ID: "v1", Name: "Video One", Artist: "Artist V"},
		},
		"curator":  {Kind: services.ItemCurator, ID: "cu1", Name: "Curator One"},
		"grouping": {Kind: services.ItemGrouping, ID: "g1", Name: "Grouping One"},
		"room":     {Kind: services.ItemRoom, ID: "ro1", Name: "Room One"},
		"link":     {Kind: services.ItemLink, Name: "Link One", URL: "https://example.test/interview/1"},
	}
}

// iaFiller are cards of a kind that opens a page and has nothing to play.
func iaFiller(n int) []services.Item {
	out := make([]services.Item, n)
	for i := range out {
		out[i] = services.Item{Kind: services.ItemCurator, ID: fmt.Sprint("f", i), Name: fmt.Sprint("Filler ", i)}
	}
	return out
}

// iaPlayers makes every page served, videos playable, and every player note
// what it is asked to play.
func iaPlayers(r *rig) *iaRec {
	rec := &iaRec{}
	r.env.HasRoute = func(string) bool { return true }
	p := r.env.Players
	for _, kind := range []string{services.ItemSong, services.ItemAlbum, services.ItemPlaylist, services.ItemStation} {
		p.Register(kind, func(it services.Item) { rec.add(it.Kind + ":" + it.ID) })
	}
	p.RegisterVideo(func(v services.MusicVideo, _ *ui.Handle) { rec.add("video:" + v.ID) })
	p.Capabilities = capabilities(true)
	return rec
}

// iaCtx is a Discover page and how to tell what a gesture did to it: start is
// where the router began.
type iaCtx struct {
	r     *rig
	rec   *iaRec
	start string
}

// did is what happened so far: the page it went to, the card that leads, what
// played, what opened in the browser.
func (x *iaCtx) did() string {
	if x.r.tt.HasText("Link One") || x.r.tt.HasText("Song 0") {
		time.Sleep(50 * time.Millisecond) // the browser and the queue are asked off the main thread
	}
	x.r.tt.Drain()
	var parts []string
	if p := x.r.path(); p != x.start {
		parts = append(parts, "nav "+p)
	}
	if at := x.r.d.front.at; at != 0 {
		parts = append(parts, fmt.Sprint("lead is card ", at))
	}
	for _, p := range x.rec.all() {
		parts = append(parts, "play "+p)
	}
	for _, p := range x.r.play.plays() {
		parts = append(parts, fmt.Sprintf("queue %d songs from %d", len(p.refs), p.start))
	}
	for _, u := range x.r.h.Opened() {
		parts = append(parts, "url "+u)
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, "; ")
}

// focusOn tabs until the focus is on the element named label.
func (x *iaCtx) focusOn(label string) bool {
	for range 60 {
		x.r.tt.Key(0, ui.KeyTab)
		x.r.tt.Drain()
		for _, a := range x.r.tt.AccessAll() {
			if a.Focused && (a.Name == label || strings.HasPrefix(a.Name, label+" ")) {
				return true
			}
		}
	}
	return false
}

// do performs a gesture on the element named label and says what it did.
func (x *iaCtx) do(g gesture, label string, track bool) string {
	x.r.t.Helper()
	tt := x.r.tt
	n, ok := tt.Find(label)
	if !ok {
		x.r.t.Fatalf("no %q; texts %q", label, tt.Texts())
	}
	cx, cy := n.X+n.W/2, n.Y+n.H/2
	if track {
		cx = n.X + 100 // the title, not the artist or album link beside it
	}
	switch g {
	case gClick, gDouble:
		tt.ClickAt(cx, cy)
		tt.Drain()
		if g == gDouble {
			tt.ClickAt(cx, cy)
			tt.Drain()
		}
	case gEnter, gShiftEnter, gSpace:
		if !x.focusOn(label) {
			return "not focusable"
		}
		switch g {
		case gEnter:
			tt.Key(0, ui.KeyEnter)
		case gShiftEnter:
			tt.Key(ui.Shift, ui.KeyEnter)
		case gSpace:
			tt.Key(0, ui.KeySpace)
		}
		tt.Drain()
	case gPlay:
		tt.Move(cx, cy) // a play control shows while its card is hovered
		tt.Drain()
		if err := tt.Click("Play " + label); err != nil {
			return "no play control"
		}
		tt.Drain()
	}
	return x.did()
}

// cue is the role of the element named label for assistive technology: what a
// link, a button, a group or a row tells a user it will do.
func (x *iaCtx) cue(label string) string {
	named := func(name string) bool {
		return name == label || name == label+" Play" || name == label+" Pause" || name == label+" Opens in your browser" || strings.HasPrefix(name, label+", ")
	}
	for _, role := range []struct {
		r    ui.Role
		name string
	}{{ui.RoleLink, "link"}, {ui.RoleButton, "button"}, {ui.RoleGroup, "group"}, {ui.RoleListItem, "listitem"}, {ui.RoleRow, "row"}} {
		for _, a := range x.r.tt.AccessAll() {
			if a.Role == role.r && named(a.Name) {
				return role.name
			}
		}
	}
	return "none"
}

// surface is one place Discover shows items, over a page that holds the item.
type surface struct {
	name string
	// track: the item is a row of a track list, whose middle is a link to its
	// artist or album.
	track bool
	build func(t *testing.T, it services.Item) *iaCtx
}

func surfaces() []surface {
	return []surface{
		{"lead", false, func(t *testing.T, it services.Item) *iaCtx {
			return frontOf(t, append([]services.Item{it}, iaFiller(3)...))
		}},
		{"queue", false, func(t *testing.T, it services.Item) *iaCtx {
			cards := iaFiller(4)
			cards[1] = it
			return frontOf(t, cards)
		}},
		{"band", false, func(t *testing.T, it services.Item) *iaCtx {
			api := &fakeAPI{}
			r := newRig(t, "/new", withAPI(api), wideWindow())
			rec := iaPlayers(r)
			newPageWith(api, r.h.Clock.Now(), services.Section{ID: "s", Kind: services.SectionShelf, Title: "Shelf", Items: []services.Item{it, iaFiller(1)[0]}})
			r.tt.Until("the shelf", r.tt.Shows(it.Name))
			return &iaCtx{r: r, rec: rec, start: "/new"}
		}},
		{"room", false, func(t *testing.T, it services.Item) *iaCtx {
			api := &fakeAPI{room: services.RoomPage{Title: "Room", Kind: services.SectionShelf, Items: []services.Item{it, iaFiller(1)[0]}}}
			r := newRig(t, "/room/1", withAPI(api), withSize(1132, 1253))
			rec := iaPlayers(r)
			r.tt.Until("the room", r.tt.Shows(it.Name))
			return &iaCtx{r: r, rec: rec, start: "/room/1"}
		}},
		{"topSongs", true, func(t *testing.T, it services.Item) *iaCtx {
			page := services.ChartsPage{Charts: []services.Chart{{Key: services.ChartSongs, Title: "Songs", Items: songItems(0, 3)}}}
			r, _ := newCharts(t, "/charts", page)
			rec := iaPlayers(r)
			r.tt.Until("the chart", r.tt.Shows("Top Songs", "Song 0"))
			return &iaCtx{r: r, rec: rec, start: "/charts"}
		}},
	}
}

// frontOf is Discover's front page over these cards, its lead the first.
func frontOf(t *testing.T, cards []services.Item) *iaCtx {
	t.Helper()
	api := &fakeAPI{}
	r := newRig(t, "/new", withAPI(api), wideWindow())
	rec := iaPlayers(r)
	newPageWith(api, r.h.Clock.Now(), heroSection(cards))
	r.tt.Until("the front page", r.tt.Shows("Featured"))
	r.tt.Drain()
	r.tt.Drain()
	return &iaCtx{r: r, rec: rec, start: "/new"}
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

// discoverWant is what each gesture does to an item, by surface and kind, as the code behaves today.
var discoverWant = map[string]map[string]outcomes{
	"lead": {
		"album": {
			click:      "nav /album/al1",
			double:     "nav /album/al1",
			enter:      "nav /album/al1",
			shiftEnter: "nothing",
			space:      "nothing",
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
		"station": {
			click:      "nav /station/sp1",
			double:     "nav /station/sp1",
			enter:      "nav /station/sp1",
			shiftEnter: "nothing",
			space:      "nothing",
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
		"video": {
			click:      "nav /music-video/v1",
			double:     "nav /music-video/v1",
			enter:      "nav /music-video/v1",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "play video:v1",
			cue:        "link",
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
		"link": {
			click:      "url https://example.test/interview/1",
			double:     "url https://example.test/interview/1",
			enter:      "url https://example.test/interview/1",
			shiftEnter: "nothing",
			space:      "url https://example.test/interview/1",
			play:       "no play control",
			cue:        "button",
		},
	},
	"queue": {
		"album": {
			click:      "nav /album/al1",
			double:     "nav /album/al1",
			enter:      "nav /album/al1",
			shiftEnter: "nothing",
			space:      "nothing",
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
	"band": {
		"album": {
			click:      "nav /album/al1",
			double:     "nav /album/al1",
			enter:      "nav /album/al1",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "play album:al1",
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
		"video": {
			click:      "nav /music-video/v1",
			double:     "nav /music-video/v1",
			enter:      "nav /music-video/v1",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "play video:v1",
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
	"room": {
		"album": {
			click:      "nav /album/al1",
			double:     "nav /album/al1",
			enter:      "nav /album/al1",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "play album:al1",
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
		"video": {
			click:      "nav /music-video/v1",
			double:     "nav /music-video/v1",
			enter:      "nav /music-video/v1",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "play video:v1",
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
	"topSongs": {
		"song": {
			click:      "nothing",
			double:     "queue 3 songs from 0",
			enter:      "queue 3 songs from 0",
			shiftEnter: "nothing",
			// DEPARTS: Space does nothing on a focused track row, though Enter and a double-click play it and Space plays on Home's item rows
			space: "nothing",
			play:  "queue 3 songs from 0",
			cue:   "row",
		},
	},
}

func TestInteractionDiscoverGestures(t *testing.T) {
	items := iaItems()
	for _, s := range surfaces() {
		for _, kind := range slices.Sorted(maps.Keys(discoverWant[s.name])) {
			want := discoverWant[s.name][kind]
			it := items[kind]
			label := it.Name
			if s.name == "topSongs" {
				label = "Song 0"
			}
			t.Run(s.name+"/"+kind, func(t *testing.T) {
				for _, g := range allGestures {
					x := s.build(t, it)
					if g == gClick {
						if got := x.cue(label); got != want.cue {
							t.Errorf("%s of a %s: its cue is %q, want %q", s.name, kind, got, want.cue)
						}
					}
					if got := x.do(g, label, s.track); got != want.of(g) {
						t.Errorf("%s of a %s on %s: did %q, want %q", g, kind, s.name, got, want.of(g))
					}
				}
			})
		}
	}
}

// The Top Songs chart's own controls: Play queues the chart from its first
// song, See all opens the whole chart.
func TestInteractionDiscoverTopSongsControls(t *testing.T) {
	for name, want := range map[string]string{
		"Play Top Songs": "queue 3 songs from 0",
		"See all":        "nav /charts/songs",
	} {
		page := services.ChartsPage{Charts: []services.Chart{{Key: services.ChartSongs, Title: "Songs", Items: songItems(0, 3)}}}
		r, _ := newCharts(t, "/charts", page)
		x := &iaCtx{r: r, rec: iaPlayers(r), start: "/charts"}
		r.tt.Until("the chart", r.tt.Shows("Top Songs", "Song 0"))
		if got := x.do(gClick, name, false); got != want {
			t.Errorf("click on %q did %q, want %q", name, got, want)
		}
	}
}

// A station with no page is one control on Discover's lead and its strip
// cards: named by its text and "Play", "Pause" while the station plays, "Play"
// again when it is paused, with no second Play disc; a press on the one the
// queue is on pauses or resumes it and never starts it over.
func TestPagelessStationControlReadsPauseWhilePlaying(t *testing.T) {
	it := iaItems()["station-nopage"]
	for _, s := range surfaces() {
		if s.name != "lead" && s.name != "queue" {
			continue
		}
		t.Run(s.name, func(t *testing.T) {
			x := s.build(t, it)
			named := func(name string) bool {
				for _, a := range x.r.tt.AccessAll() {
					if a.Name == name {
						return true
					}
				}
				return false
			}
			if !named(it.Name+" Play") || named("Play "+it.Name) {
				t.Fatalf("want one control named %q and no Play disc; texts %q", it.Name+" Play", x.r.tt.Texts())
			}
			x.r.h.Play(0, func(st *services.PlaybackState) { st.Current = &services.QueueItem{ID: it.Station.ID, Name: it.Name} })
			x.r.tt.Drain()
			if !named(it.Name+" Pause") || named(it.Name+" Play") {
				t.Errorf("not named %q while it plays; texts %q", it.Name+" Pause", x.r.tt.Texts())
			}
			if got := x.do(gClick, it.Name, false); got != "nothing" {
				t.Errorf("a press on the playing station did %q, want it to pause rather than play again", got)
			}
			x.r.h.Play(0, func(st *services.PlaybackState) {
				st.Current, st.State = &services.QueueItem{ID: it.Station.ID, Name: it.Name}, store.StatePaused
			})
			x.r.tt.Drain()
			if !named(it.Name + " Play") {
				t.Errorf("not named %q while it is paused; texts %q", it.Name+" Play", x.r.tt.Texts())
			}
		})
	}
}
