package playlist

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// rigLabel names what the picker adds, as an item's description does.
const rigLabel = "“Hundred”"

// dialogsRig is the dialog host and the overlays (toasts, confirm) over the
// fakes, with one button that opens the add picker, as an item action does.
type dialogsRig struct {
	p      *playlists
	api    *fakeAPI
	router *ui.Router
	tt     *uitest.Session
	// opened counts the opener's clicks.
	opened int
}

func (r *dialogsRig) view(c *ui.Context) {
	c.DisableAnimations()
	ui.Column(c).Padding(20).AlignItems(ui.Start).Children(func() {
		if kit.Button(c.Key("opener"), "Open picker", kit.ButtonOpts{}).Clicked() {
			r.opened++
			r.p.addToPlaylist(rigItems(), rigLabel)
		}
	})
	r.p.dialogHost(c)
	kit.OverlayLayer(c)
}

// rigItems is what the picker adds: one library song.
func rigItems() []services.PlaylistItem {
	return []services.PlaylistItem{{Kind: "song", ID: "s1", Source: services.SourceLibrary}}
}

// newDialogsRig builds the rig in a light window.
func newDialogsRig(t *testing.T) *dialogsRig {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	t.Cleanup(kit.CancelConfirm)
	p, api, _, h, router := testPlaylists(t, "/library/albums")
	r := &dialogsRig{p: p, api: api, router: router}
	r.tt = h.Tester(t, r.view, 900, 700)
	return r
}

// show opens dialog s and lets it come in.
func (r *dialogsRig) show(s dialogState) {
	r.p.dialogs.show(s)
	r.tt.Frame()
	r.tt.Advance(300 * time.Millisecond)
}

// closed reports whether no dialog is open.
func (r *dialogsRig) closed() bool { return !r.p.dialogs.open }

// shows waits for every text.
func (r *dialogsRig) shows(texts ...string) {
	r.tt.T.Helper()
	r.tt.Until("texts "+strings.Join(texts, ", "), r.tt.Shows(texts...))
}

// did waits for the fake to have been asked call.
func (r *dialogsRig) did(call string) {
	r.tt.T.Helper()
	r.tt.Until("call "+call, func() bool { return r.api.did(call) })
}

// asked reports whether any call starts with prefix.
func (r *dialogsRig) asked(prefix string) bool {
	for _, c := range r.api.asked() {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

// count is how many times the fake was asked exactly call.
func (r *dialogsRig) count(call string) int {
	n := 0
	for _, c := range r.api.asked() {
		if c == call {
			n++
		}
	}
	return n
}

// typed types s into the focused field and draws the frames it asks for.
func (r *dialogsRig) typed(s string) {
	r.tt.Type(s)
	r.tt.Frame()
}

// replaced selects what the focused field holds and types s over it.
func (r *dialogsRig) replaced(s string) {
	r.tt.Key(ui.Cmd, ui.KeyA)
	r.typed(s)
}

// seedTree is a tree of editable playlists: two at the top level, one in
// the folder Mixes (which holds Old), and one Apple's, which no picker lists.
func (r *dialogsRig) seedTree() {
	r.api.tree = services.PlaylistTree{
		Folders: []services.PlaylistFolder{folder("pf.1", "Mixes", "", 0), folder("pf.2", "Old", "pf.1", 0)},
		Playlists: []services.TreePlaylist{
			treePlaylist("p.a", "Road Trip", "", 0, true),
			treePlaylist("p.d", "Roadie", "", 1, true),
			treePlaylist("p.b", "Gym", "pf.1", 0, true),
			treePlaylist("p.c", "Apple Curated", "", 2, false),
		},
	}
	r.api.recents = []string{"p.b", "p.c"}
}

// seedNoFolders is a tree with one playlist and no folder.
func (r *dialogsRig) seedNoFolders() {
	r.api.tree = services.PlaylistTree{Playlists: []services.TreePlaylist{treePlaylist("p.a", "Road Trip", "", 0, true)}}
}

// index is where text s first shows in the frame, -1 when it does not.
func (r *dialogsRig) index(s string) int {
	for i, t := range r.tt.Texts() {
		if t == s {
			return i
		}
	}
	return -1
}

// holdTree makes the tree read wait until the returned release runs (also at
// the end of the test): the picker shows its loading state meanwhile.
func (r *dialogsRig) holdTree(t *testing.T) (release func()) {
	t.Helper()
	gate := make(chan struct{})
	release = sync.OnceFunc(func() { close(gate) })
	t.Cleanup(release)
	r.p.api = heldTree{r.api, gate}
	return release
}

// heldTree is the fake with Tree waiting for gate.
type heldTree struct {
	*fakeAPI
	gate chan struct{}
}

func (h heldTree) Tree(ctx context.Context) (services.PlaylistTree, error) {
	<-h.gate
	return h.fakeAPI.Tree(ctx)
}

// holdCreate makes Create wait until the returned release runs (also at the
// end of the test): the form stays busy meanwhile.
func (r *dialogsRig) holdCreate(t *testing.T) (release func()) {
	t.Helper()
	gate := make(chan struct{})
	release = sync.OnceFunc(func() { close(gate) })
	t.Cleanup(release)
	r.p.api = heldCreate{r.api, gate}
	return release
}

// heldCreate is the fake with Create waiting for gate.
type heldCreate struct {
	*fakeAPI
	gate chan struct{}
}

func (h heldCreate) Create(ctx context.Context, in services.PlaylistInput, items []services.PlaylistItem, folderID string) (services.Playlist, error) {
	<-h.gate
	return h.fakeAPI.Create(ctx, in, items, folderID)
}
