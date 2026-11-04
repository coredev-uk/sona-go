package library

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// failingAlbums fails the albums list until it is told to work.
type failingAlbums struct {
	libraryAPI
	fail atomic.Bool
	asks atomic.Int32
}

func (f *failingAlbums) Albums(ctx context.Context, sort string, desc bool, typ string, off, limit int) (services.AlbumPage, error) {
	f.asks.Add(1)
	if f.fail.Load() {
		return services.AlbumPage{}, errors.New("x")
	}
	return f.libraryAPI.Albums(ctx, sort, desc, typ, off, limit)
}

func TestTheGateShowsAReadErrorAndRetries(t *testing.T) {
	f := &failingAlbums{}
	f.fail.Store(true)
	r := newPageRig(t, albumsPath, 1132, 800, func(l *library) { f.libraryAPI = l.lib; l.lib = f })
	r.tt.Until("the error", r.tt.Shows("Couldn't read your albums from this computer", "Try again"))
	f.fail.Store(false)
	if err := r.tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the albums", r.tt.Shows("24 albums"))
}

func TestTheGateSaysTheLibraryIsSyncing(t *testing.T) {
	// Signed in, nothing synced yet: "syncing", not "empty".
	r := newPageRig(t, albumsPath, 1132, 800, func(l *library) { l.auth = accountAPI{"signed_in"} }, uitest.Empty())
	r.tt.Until("syncing", r.tt.Shows("Your library is syncing",
		"Your albums appear here as Sona copies them from Apple Music. Large libraries take a minute or two the first time."))
}

func TestTheGateSaysEmptyOnceSynced(t *testing.T) {
	r := newPageRig(t, albumsPath, 1132, 800, func(l *library) { l.auth = accountAPI{"signed_in"} })
	emptyTheSyncedLibrary(r)
	r.tt.Until("empty", r.tt.Shows("No albums in your library yet",
		"Albums you add in Apple Music appear here after the next sync, newest first."))
	// A sync that is running says so, whatever was synced before.
	r.h.Env.Bus.Emit(services.EventSyncProgress, services.SyncProgress{Domain: "albums", Done: 1, Total: 2})
	r.tt.Until("syncing again", r.tt.Shows("Your library is syncing"))
}

func TestTheGateShowsNothingWhileRestoring(t *testing.T) {
	r := newPageRig(t, albumsPath, 1132, 800, func(l *library) { l.auth = accountAPI{"restoring"} }, uitest.Empty())
	for i := 0; i < 20; i++ {
		r.tt.Drain()
	}
	for _, w := range []string{"Sign in to see your albums", "Your library is syncing", "No albums in your library yet"} {
		if r.tt.HasText(w) {
			t.Errorf("shows %q while the account is restoring", w)
		}
	}
}

func TestSignInOpensTheAccountDialog(t *testing.T) {
	opened := 0
	r := newPageRig(t, albumsPath, 1132, 800, func(l *library) { l.openAccount = func() { opened++ } }, uitest.Empty())
	r.tt.Until("sign in", r.tt.Shows("Sign in to see your albums", "Sign in"))
	for i := 0; i < 6; i++ {
		r.tt.Drain() // the empty state's cover slots settle once measured
	}
	if err := r.tt.Click("Sign in"); err != nil {
		t.Fatal(err)
	}
	if opened != 1 {
		t.Errorf("the account dialog opened %d times", opened)
	}
	// Without a dialog to open the action is not offered.
	r2 := newPageRig(t, albumsPath, 1132, 800, nil, uitest.Empty())
	r2.tt.Until("text", r2.tt.Shows("Sign in to see your albums"))
	if _, ok := r2.tt.Find("Sign in"); ok {
		t.Error("a Sign in button that does nothing")
	}
}

func TestAlbumsGridKeyboardAndSelection(t *testing.T) {
	r := newPageRig(t, albumsPath, 1132, 1000, nil, uitest.Albums(sevenAlbums...))
	r.tt.Until("the albums", r.tt.Shows("Paper Planes", "7 albums"))
	// Tab walks the header's controls, then reaches the grid (newest first).
	for i := 0; i < 8 && !r.tt.Focused("Paper Planes"); i++ {
		r.tt.Key(0, ui.KeyTab)
	}
	if !r.tt.Focused("Paper Planes") {
		t.Fatal("Tab did not reach the first tile")
	}
	r.tt.Key(0, ui.KeyRight)
	if !r.tt.Focused("Single Malt") {
		t.Errorf("Right did not move to the second tile")
	}
	r.tt.Key(0, ui.KeyLeft)
	r.tt.Key(ui.Shift, ui.KeyRight)
	if n := r.l.albums.cover.st.Sel.Len(); n != 2 {
		t.Errorf("Shift+Right selects %d tiles, want 2", n)
	}
	r.tt.Key(ui.Cmd, ui.KeyA)
	if n := r.l.albums.cover.st.Sel.Len(); n != 7 {
		t.Errorf("Ctrl+A selects %d tiles, want 7", n)
	}
	r.tt.Key(0, ui.KeyEscape)
	if n := r.l.albums.cover.st.Sel.Len(); n != 0 {
		t.Errorf("Escape leaves %d selected", n)
	}
}
