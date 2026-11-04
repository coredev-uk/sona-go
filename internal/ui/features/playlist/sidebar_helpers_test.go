package playlist

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// sbEnv is the sidebar over the fakes, in a window beside a draggable "Dragged"
// item that carries payload, with the toaster, the confirm host and the
// dialogs mounted as the shell has them.
type sbEnv struct {
	t      *testing.T
	p      *playlists
	api    *fakeAPI
	play   *fakePlay
	h      *uitest.Harness
	router *ui.Router
	tt     *uitest.Session

	payload *kit.DragPayload
	// atX, atY is the pointer while dragging.
	atX, atY float32
}

// sbSetup mounts the sidebar over tree and pins. Every route is registered
// (the pins' pages), and the item actions are in the registry.
func sbSetup(t *testing.T, tree services.PlaylistTree, pins []services.Pin) *sbEnv {
	t.Helper()
	return sbSetupAPI(t, tree, pins, nil)
}

// sbSetupAPI is sbSetup with wrap putting its own API between the feature
// and the fake before the first frame asks for anything.
func sbSetupAPI(t *testing.T, tree services.PlaylistTree, pins []services.Pin, wrap func(*fakeAPI) playlistsAPI) *sbEnv {
	t.Helper()
	p, api, play, h, router := testPlaylists(t, "/library/albums")
	if wrap != nil {
		p.api = wrap(api)
	}
	api.tree, api.pins = tree, pins
	h.Env.HasRoute = func(string) bool { return true }
	h.Env.Actions = kit.NewActionRegistry(p.actions()...)
	h.Env.Actions.SetPoster(h.Main.Post)
	h.Env.Actions.SetNavigator(router.Push)
	e := &sbEnv{t: t, p: p, api: api, play: play, h: h, router: router}
	t.Cleanup(kit.CancelConfirm)
	e.tt = h.Tester(t, e.view, 520, 900)
	return e
}

func (e *sbEnv) view(c *ui.Context) {
	c.DisableAnimations()
	ui.Row(c).Children(func() {
		ui.Column(c.Key("sidebar")).Width(232).Children(func() { e.p.sidebarSection(c) })
		ui.Column(c.Key("tools")).Width(260).Padding(12).Children(func() {
			kit.DragItem(c, kit.DragOpts{
				Key:     "src",
				Row:     true,
				Payload: func() *kit.DragPayload { return e.payload },
				Outer:   func(w ui.Element) { w.Size(160, 40) },
				Source:  func(s ui.Element) { s.Padding(8) },
			}, func(w, s ui.Element) { ui.Text(c, "Dragged") })
		})
	})
	e.p.dialogHost(c)
	kit.Toaster(c)
	kit.ConfirmHost(c)
}

// sbLibrary is v0.2.0's test tree: a folder Running holding Tempo, two
// personal mixes, one playlist from Apple Music and one of the user's own.
func sbLibrary() services.PlaylistTree {
	other := func(id, name string, position int, catalogID string) services.TreePlaylist {
		tp := treePlaylist(id, name, "", position, false)
		tp.Playlist.CatalogID = catalogID
		return tp
	}
	return services.PlaylistTree{
		Folders: []services.PlaylistFolder{folder("f.run", "Running", "", 0)},
		Playlists: []services.TreePlaylist{
			treePlaylist("p.own", "Road Trip", "", 3, true),
			other("p.mix1", "Morning Mix", 0, "pl.pm-1"),
			other("p.mix2", "Discovery Mix", 1, "pl.pm-2"),
			other("p.ed", "Quiet Hours", 4, "pl.abc"),
			treePlaylist("p.run", "Tempo", "f.run", 0, true),
		},
	}
}

// sbPlaylistPins are playlist pins (named by their IDs) for the given IDs.
func sbPlaylistPins(ids ...string) []services.Pin {
	var out []services.Pin
	for _, id := range ids {
		out = append(out, services.Pin{Kind: "playlist", ID: id, Name: id, Action: "drillIn"})
	}
	return out
}

// shows draws until every text is on screen.
func (e *sbEnv) shows(texts ...string) {
	e.t.Helper()
	e.tt.Until("texts "+strings.Join(texts, ", "), e.tt.Shows(texts...))
}

// settle runs what is posted and draws, so a menu's chosen row has run.
func (e *sbEnv) settle() {
	e.t.Helper()
	e.tt.Advance(100 * time.Millisecond)
	e.tt.Drain()
}

// did draws until the fake was asked call.
func (e *sbEnv) did(call string) {
	e.t.Helper()
	e.tt.Until("call "+call, func() bool { return e.api.did(call) })
}

// adds are the Add calls the fake saw.
func (e *sbEnv) adds() []string {
	var out []string
	for _, c := range e.api.asked() {
		if strings.HasPrefix(c, "Add ") {
			out = append(out, c)
		}
	}
	return out
}

// quiet draws a few frames, letting anything posted arrive.
func (e *sbEnv) quiet() {
	for range 6 {
		e.tt.Drain()
		time.Sleep(2 * time.Millisecond)
	}
}

// y is where the element showing text s is, from the top.
func (e *sbEnv) y(s string) float32 {
	e.t.Helper()
	r, ok := e.tt.Find(s)
	if !ok {
		e.t.Fatalf("%q is not on screen: %q", s, e.tt.Texts())
	}
	return r.Y
}

// above fails unless a is drawn above b.
func (e *sbEnv) above(a, b string) {
	e.t.Helper()
	if ya, yb := e.y(a), e.y(b); ya >= yb {
		e.t.Errorf("%q (y=%v) is not above %q (y=%v)", a, ya, b, yb)
	}
}

func (e *sbEnv) center(s string) (float32, float32) {
	e.t.Helper()
	r, ok := e.tt.Find(s)
	if !ok {
		e.t.Fatalf("%q is not on screen: %q", s, e.tt.Texts())
	}
	return r.X + r.W/2, r.Y + r.H/2
}

// hover moves the pointer onto the row showing s, so what it reveals shows.
func (e *sbEnv) hover(s string) {
	e.t.Helper()
	x, y := e.center(s)
	e.tt.Move(x, y)
	e.tt.Frame()
}

// more opens the "More for …" menu of the row showing name.
func (e *sbEnv) more(name, label string) {
	e.t.Helper()
	e.hover(name)
	if err := e.tt.Click(label); err != nil {
		e.t.Fatal(err)
	}
}

// choose clicks a menu row and lets it run.
func (e *sbEnv) choose(label string) {
	e.t.Helper()
	if err := e.tt.Click(label); err != nil {
		e.t.Fatal(err)
	}
	e.settle()
}

// grab starts dragging payload from the "Dragged" item and lets the sidebar
// show what a drag offers.
func (e *sbEnv) grab(payload *kit.DragPayload) {
	e.t.Helper()
	e.payload = payload
	// The drag engine is the process's: a test that ends with a drag still
	// going must not leave it to the next.
	e.t.Cleanup(func() {
		e.tt.Release(e.atX, e.atY)
		e.tt.Frame()
		e.tt.Frame()
		if kit.DragActive(nil) {
			e.t.Error("a drag is still going at the end of the test")
		}
	})
	x, y := e.center("Dragged")
	e.tt.Press(x, y)
	e.tt.Move(x+4, y)
	e.tt.Move(x+10, y)
	e.tt.Move(x+14, y)
	if !kit.DragActive(nil) {
		e.t.Fatal("the drag did not start")
	}
	e.atX, e.atY = x+14, y
	e.tt.Frame()
	e.tt.Frame()
}

// over moves the dragged item onto the row showing s.
func (e *sbEnv) over(s string) {
	e.t.Helper()
	x, y := e.center(s)
	e.tt.Move(x, y)
	e.tt.Move(x+1, y+1)
	e.atX, e.atY = x+1, y+1
}

// drop lets go where the pointer is.
func (e *sbEnv) drop() {
	e.t.Helper()
	e.tt.Release(e.atX, e.atY)
	e.tt.Frame()
	e.tt.Frame()
}

// sbSongPayload is a song dragged out of a list.
func sbSongPayload(s services.Song) *kit.DragPayload {
	return &kit.DragPayload{Items: []kit.Item{kit.SongItem(s)}, Label: s.Name}
}

// has reports whether a text is on screen exactly (HasText matches parts:
// "New playlist" is part of "New playlist or folder").
func (e *sbEnv) has(s string) bool { return slices.Contains(e.tt.Texts(), s) }

// closeDialog closes the playlist dialog that is open, with Escape.
func (e *sbEnv) closeDialog() {
	e.t.Helper()
	e.tt.Key(0, ui.KeyEscape)
	e.settle()
	if e.p.dialogs.open {
		e.t.Fatal("Escape did not close the dialog")
	}
}
