package playlist

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// pageStates is the header's LibraryService, over memory.
type pageStates struct {
	mu    sync.Mutex
	in    bool
	calls []string
}

func (f *pageStates) note(call string) {
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()
}

func (f *pageStates) asked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func (f *pageStates) States(_ context.Context, refs []services.ItemRef) ([]services.ItemState, error) {
	out := make([]services.ItemState, len(refs))
	for i, r := range refs {
		out[i] = services.ItemState{Kind: r.Kind, ID: r.ID, InLibrary: f.in || r.Source == services.SourceLibrary}
	}
	return out, nil
}

func (f *pageStates) Favorite(_ context.Context, r services.ItemRef, on bool) (services.ItemState, error) {
	f.note("Favorite " + r.ID)
	st := services.ItemState{Kind: r.Kind, ID: r.ID, InLibrary: true}
	if on {
		st.Rating = 1
	}
	return st, nil
}

func (f *pageStates) SuggestLess(_ context.Context, r services.ItemRef, on bool) (services.ItemState, error) {
	f.note("SuggestLess " + r.ID)
	st := services.ItemState{Kind: r.Kind, ID: r.ID, InLibrary: true}
	if on {
		st.Rating = -1
	}
	return st, nil
}

func (f *pageStates) Remove(_ context.Context, r services.ItemRef) (services.ItemState, error) {
	f.note("Remove " + r.ID)
	return services.ItemState{Kind: r.Kind, ID: r.ID}, nil
}

func (f *pageStates) Add(_ context.Context, typ string, ids []string) error {
	f.note("Add " + typ + " " + strings.Join(ids, ","))
	return nil
}

// pageRig is the playlist page over the fakes, in a router, with the toasts
// and the dialog host the shell would have.
type pageRig struct {
	t      *testing.T
	p      *playlists
	api    *fakeAPI
	play   *fakePlay
	states *pageStates
	router *ui.Router
	tt     *uitest.Session
	// linked is what env.HasRoute answers for every page but this one's.
	linked bool
}

// newPageRig builds the page of playlist id; details are what the service
// holds. Every other page of Sona is linkable until rig.linked says not.
func newPageRig(t *testing.T, id string, details ...services.PlaylistDetail) *pageRig {
	t.Helper()
	return newPageRigWith(t, id, nil, details...)
}

// newPageRigWith is newPageRig with setup run on the fake service before the
// first frame (the page asks for its playlist as it is built).
func newPageRigWith(t *testing.T, id string, setup func(*fakeAPI), details ...services.PlaylistDetail) *pageRig {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	t.Cleanup(kit.CancelConfirm)
	p, api, play, h, router := testPlaylists(t, "/playlist/"+id)
	r := &pageRig{t: t, p: p, api: api, play: play, states: &pageStates{}, router: router, linked: true}
	h.Env.HasRoute = func(string) bool { return r.linked }
	h.Env.Actions = kit.NewActionRegistry(p.actions()...)
	h.Env.ItemStates = kit.NewItemStates(h.Env.Queries, h.Env.Post, r.states)
	h.Env.ItemStates.Follow(h.Bus)
	for _, d := range details {
		api.details[d.Playlist.ID] = d
	}
	if setup != nil {
		setup(api)
	}
	r.tt = h.Tester(t, func(c *ui.Context) {
		c.DisableAnimations()
		ui.Column(c).Fill().Children(func() {
			router.View(c, func(rt *ui.Route) {
				if rt.Match(playlistPath) {
					p.playlistPage(c, rt)
				}
			})
		})
		p.dialogHost(c)
		kit.Toaster(c)
	}, 1132, 1000)
	return r
}

// shows waits for every text.
func (r *pageRig) shows(texts ...string) {
	r.t.Helper()
	r.tt.Until("texts "+strings.Join(texts, ", "), r.tt.Shows(texts...))
}

// did waits for the fake service to have been asked call.
func (r *pageRig) did(call string) {
	r.t.Helper()
	r.tt.Until("call "+call, func() bool { return r.api.did(call) })
}

// order is names in the order the page shows them (a name once per run).
func (r *pageRig) order(names ...string) []string {
	var out []string
	for _, tx := range r.tt.Texts() {
		if slices.Contains(names, tx) && (len(out) == 0 || out[len(out)-1] != tx) {
			out = append(out, tx)
		}
	}
	return out
}

// click clicks the element showing text and lets the frame settle.
func (r *pageRig) click(text string) {
	r.t.Helper()
	if err := r.tt.Click(text); err != nil {
		r.t.Fatalf("click %q: %v", text, err)
	}
	r.tt.Frame()
}

// calls are the service calls that start with prefix.
func (r *pageRig) calls(prefix string) []string {
	var out []string
	for _, c := range r.api.asked() {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

// shuffled reports whether shuffle was switched on.
func (f *fakePlay) shuffled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.shuffle
}

// pgSong is a song with an album.
func pgSong(id, name, artist, album string) services.Song {
	s := song(id, name, artist)
	s.Album = album
	return s
}

// threeSongs are the songs of the tests' playlists: ids a, b and c.
func threeSongs() []services.Song {
	return []services.Song{
		pgSong("a", "Song A", "Artist A", "Album A"),
		pgSong("b", "Song B", "Artist B", "Album B"),
		pgSong("c", "Song C", "Artist C", "Album C"),
	}
}

// libraryDetail is an editable library playlist.
func libraryDetail(id, name string, tracks ...services.Song) services.PlaylistDetail {
	var ms int64
	for _, s := range tracks {
		ms += s.DurationMs
	}
	return services.PlaylistDetail{
		Playlist:   services.Playlist{ID: id, Source: services.SourceLibrary, Name: name, CanEdit: true, TrackCount: len(tracks)},
		Tracks:     tracks,
		DurationMs: ms,
		InLibrary:  true,
		Kind:       "library",
		CanDelete:  true,
	}
}

// catalogDetail is a playlist of Apple's, which nobody can edit.
func catalogDetail(id, name string, tracks ...services.Song) services.PlaylistDetail {
	d := libraryDetail(id, name, tracks...)
	d.Playlist.Source, d.Playlist.CatalogID, d.Playlist.CanEdit = services.SourceCatalog, id, false
	d.Kind, d.InLibrary, d.CanDelete = "editorial", false, false
	return d
}
