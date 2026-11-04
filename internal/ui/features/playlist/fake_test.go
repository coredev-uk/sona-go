package playlist

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/library"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// errOffline classifies as services.CodeNetwork.
var errOffline = &net.OpError{Op: "dial", Err: errors.New("offline")}

// fakeAPI is PlaylistsService over memory: the tests set what it holds and
// read what it was asked (calls, in order, as "Method args").
type fakeAPI struct {
	mu          sync.Mutex
	tree        services.PlaylistTree
	pins        []services.Pin
	details     map[string]services.PlaylistDetail
	collab      map[string]services.PlaylistCollaboration
	suggestions map[string][]services.Song
	recents     []string
	pinTracks   map[string][]services.Song
	// err is returned by every call that changes something while set; reads
	// fail too when failReads is set.
	err       error
	failReads bool
	calls     []string
	// gate, when set, blocks Add/MoveTrack/RemoveTracks until it is closed
	// (to keep an edit in flight).
	gate chan struct{}
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{
		details:     map[string]services.PlaylistDetail{},
		collab:      map[string]services.PlaylistCollaboration{},
		suggestions: map[string][]services.Song{},
		pinTracks:   map[string][]services.Song{},
	}
}

func (f *fakeAPI) note(format string, args ...any) {
	f.mu.Lock()
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
	f.mu.Unlock()
}

// asked returns the calls so far.
func (f *fakeAPI) asked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// did reports whether a call with exactly this text was made.
func (f *fakeAPI) did(call string) bool { return slices.Contains(f.asked(), call) }

func (f *fakeAPI) wait() {
	f.mu.Lock()
	g := f.gate
	f.mu.Unlock()
	if g != nil {
		<-g
	}
}

// setGate sets gate while calls may be running.
func (f *fakeAPI) setGate(g chan struct{}) {
	f.mu.Lock()
	f.gate = g
	f.mu.Unlock()
}

func (f *fakeAPI) readErr() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failReads {
		return errOffline
	}
	return nil
}

func (f *fakeAPI) Playlist(_ context.Context, id string) (services.PlaylistDetail, error) {
	f.note("Playlist %s", id)
	if err := f.readErr(); err != nil {
		return services.PlaylistDetail{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.details[id]
	if !ok {
		return services.PlaylistDetail{}, library.ErrNotFound
	}
	return d, nil
}

func (f *fakeAPI) Collaboration(_ context.Context, id string) (services.PlaylistCollaboration, error) {
	f.note("Collaboration %s", id)
	if err := f.readErr(); err != nil {
		return services.PlaylistCollaboration{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.collab[id], nil
}

func (f *fakeAPI) Suggestions(_ context.Context, id string) ([]services.Song, error) {
	f.note("Suggestions %s", id)
	if err := f.readErr(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.suggestions[id], nil
}

func (f *fakeAPI) Tree(context.Context) (services.PlaylistTree, error) {
	f.note("Tree")
	if err := f.readErr(); err != nil {
		return services.PlaylistTree{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tree, nil
}

func (f *fakeAPI) RecentTargets(context.Context) ([]string, error) {
	f.note("RecentTargets")
	if err := f.readErr(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.recents, nil
}

func itemsText(items []services.PlaylistItem) string {
	var s []string
	for _, it := range items {
		s = append(s, it.Kind+":"+it.ID)
	}
	return fmt.Sprint(s)
}

func (f *fakeAPI) Create(_ context.Context, in services.PlaylistInput, items []services.PlaylistItem, folderID string) (services.Playlist, error) {
	f.note("Create %q desc=%q public=%v items=%s folder=%q", in.Name, in.Description, in.IsPublic, itemsText(items), folderID)
	if f.err != nil {
		return services.Playlist{}, f.err
	}
	return services.Playlist{ID: "p.created", Source: services.SourceLibrary, Name: in.Name, CanEdit: true}, nil
}

func (f *fakeAPI) Update(_ context.Context, id string, in services.PlaylistInput) error {
	f.note("Update %s %q desc=%q public=%v", id, in.Name, in.Description, in.IsPublic)
	return f.err
}

func (f *fakeAPI) Delete(_ context.Context, id string) error {
	f.note("Delete %s", id)
	return f.err
}

func (f *fakeAPI) Add(_ context.Context, id string, items []services.PlaylistItem) (services.PlaylistDetail, error) {
	f.note("Add %s %s", id, itemsText(items))
	f.wait()
	if f.err != nil {
		return services.PlaylistDetail{}, f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	d := f.details[id]
	if d.Playlist.ID == "" {
		d.Playlist.ID = id
	}
	return d, nil
}

func (f *fakeAPI) MoveTrack(_ context.Context, id string, from, to int, trackID string) (services.PlaylistDetail, error) {
	f.note("MoveTrack %s %d %d %s", id, from, to, trackID)
	f.wait()
	if f.err != nil {
		return services.PlaylistDetail{}, f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	d := withMove(f.details[id], from, to)
	f.details[id] = d
	return d, nil
}

func (f *fakeAPI) RemoveTracks(_ context.Context, id string, positions []int, trackIDs []string) (services.PlaylistDetail, error) {
	f.note("RemoveTracks %s %v %v", id, positions, trackIDs)
	f.wait()
	if f.err != nil {
		return services.PlaylistDetail{}, f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	d := withRemoved(f.details[id], positions)
	f.details[id] = d
	return d, nil
}

func (f *fakeAPI) MoveToFolder(_ context.Context, id, folderID string) error {
	f.note("MoveToFolder %s %q", id, folderID)
	return f.err
}

func (f *fakeAPI) CreateFolder(_ context.Context, name string) (services.PlaylistFolder, error) {
	f.note("CreateFolder %q", name)
	if f.err != nil {
		return services.PlaylistFolder{}, f.err
	}
	return services.PlaylistFolder{ID: "pf.created", Name: name}, nil
}

func (f *fakeAPI) RenameFolder(_ context.Context, id, name string) error {
	f.note("RenameFolder %s %q", id, name)
	return f.err
}

func (f *fakeAPI) DeleteFolder(_ context.Context, id string) error {
	f.note("DeleteFolder %s", id)
	return f.err
}

func (f *fakeAPI) Pins(context.Context) ([]services.Pin, error) {
	f.note("Pins")
	if err := f.readErr(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.pins), nil
}

func (f *fakeAPI) Pin(_ context.Context, id string) error {
	f.note("Pin %s", id)
	return f.err
}

func (f *fakeAPI) Unpin(_ context.Context, id string) error {
	f.note("Unpin %s", id)
	return f.err
}

func (f *fakeAPI) MovePin(_ context.Context, id, afterID string) error {
	f.note("MovePin %s after %q", id, afterID)
	return f.err
}

func (f *fakeAPI) SetPinAction(_ context.Context, id, action string) error {
	f.note("SetPinAction %s %s", id, action)
	return f.err
}

func (f *fakeAPI) PinTracks(_ context.Context, id string) ([]services.Song, error) {
	f.note("PinTracks %s", id)
	if f.err != nil {
		return nil, f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pinTracks[id], nil
}

// fakePlay is PlaybackService's play calls, noted.
type fakePlay struct {
	mu      sync.Mutex
	plays   []string
	shuffle bool
}

func (f *fakePlay) PlayItems(_ context.Context, refs []services.TrackRef, start int, from services.PlayFrom) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := make([]string, len(refs))
	for i, r := range refs {
		ids[i] = r.ID
	}
	f.plays = append(f.plays, fmt.Sprintf("%v start=%d from=%s:%s", ids, start, from.Kind, from.ID))
	return nil
}

func (f *fakePlay) SetShuffle(on bool) {
	f.mu.Lock()
	f.shuffle = on
	f.mu.Unlock()
}

func (f *fakePlay) played() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.plays)
}

// testPlaylists builds the feature over the fakes on a bare uitest Env with
// a router at start and a prefs file of its own. p.init has run.
func testPlaylists(t *testing.T, start string) (*playlists, *fakeAPI, *fakePlay, *uitest.Harness, *ui.Router) {
	t.Helper()
	h := uitest.New(t, uitest.Bare())
	router := ui.NewRouter(start)
	h.Env.Nav = router
	api, play := newFakeAPI(), &fakePlay{}
	p := newPlaylists(h.Env, api, play, newPrefs(config.Sidebar{}, nil))
	p.init()
	return p, api, play, h, router
}

// song, treePlaylist and folder build test data.
func song(id, name, artist string) services.Song {
	return services.Song{ID: id, Source: services.SourceLibrary, Name: name, Artist: artist, DurationMs: 200_000, Playable: true}
}

func treePlaylist(id, name, folder string, position int, canEdit bool) services.TreePlaylist {
	return services.TreePlaylist{
		Playlist: services.Playlist{ID: id, Source: services.SourceLibrary, Name: name, CanEdit: canEdit, TrackCount: 3},
		FolderID: folder, Position: position, CanDelete: canEdit,
	}
}

func folder(id, name, parent string, position int) services.PlaylistFolder {
	return services.PlaylistFolder{ID: id, Name: name, ParentID: parent, Position: position}
}
