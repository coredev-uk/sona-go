package library

import (
	"context"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// gatedLists holds back the list a sort not yet read asks for, so that a test
// sees the frames between choosing the sort and its rows arriving.
type gatedLists struct {
	libraryAPI
	once    sync.Once
	release chan struct{}
	asked   chan struct{}
	sort    string
}

func newGatedLists(api libraryAPI, sort string) *gatedLists {
	return &gatedLists{libraryAPI: api, release: make(chan struct{}), asked: make(chan struct{}), sort: sort}
}

// hold blocks the first read of the gated sort until release is closed.
func (g *gatedLists) hold(ctx context.Context, sort string) {
	if sort != g.sort {
		return
	}
	g.once.Do(func() { close(g.asked) })
	select {
	case <-g.release:
	case <-ctx.Done():
	}
}

func (g *gatedLists) Albums(ctx context.Context, sort string, desc bool, typ string, off, lim int) (services.AlbumPage, error) {
	g.hold(ctx, sort)
	return g.libraryAPI.Albums(ctx, sort, desc, typ, off, lim)
}

func (g *gatedLists) Songs(ctx context.Context, sort string, desc bool, off, lim int) (services.SongPage, error) {
	g.hold(ctx, sort)
	return g.libraryAPI.Songs(ctx, sort, desc, off, lim)
}

func (g *gatedLists) Playlists(ctx context.Context, sort string, desc bool, off, lim int) (services.PlaylistPage, error) {
	g.hold(ctx, sort)
	return g.libraryAPI.Playlists(ctx, sort, desc, off, lim)
}

// TestChoosingAFirstSortMovesNoControl: a sort whose list is not in the cache
// yet leaves the page's controls where they were while it loads and when its
// rows arrive (the line under the title and the controls' row keep their
// height).
func TestChoosingAFirstSortMovesNoControl(t *testing.T) {
	for _, c := range []struct {
		name, path, choose, sort string
		controls                 []string
		shown                    string
	}{
		{"albums", albumsPath, "Title", "title", []string{"Album type", "Sort", "All", "Singles & EPs"}, "Apple Orchard"},
		{"songs", songsPath, "Recently added", "dateAdded", []string{"Sort", "Play your songs", "Shuffle"}, "Apple Orchard song 1"},
		{"playlists", playlistsPath, "Recently updated", "lastModified", []string{"Sort"}, "Sunday morning"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var opts []uitest.Option
			if c.name != "playlists" {
				opts = append(opts, uitest.Albums(sevenAlbums...))
			}
			var g *gatedLists
			r := newPageRig(t, c.path, 1132, 800, func(l *library) {
				g = newGatedLists(l.lib, c.sort)
				l.lib = g
				fakeAuth(l, accountAPI{"signed_in"})
			}, opts...)
			r.tt.Until("the page", r.tt.Shows("Sort"))
			r.tt.Until("a settled page", func() bool { return r.tt.HasText("Sort") && !r.tt.HasText("Loading") })
			before := boundsOf(t, r.tt, c.controls)

			if err := r.tt.Click("Sort"); err != nil {
				t.Fatal(err)
			}
			if err := r.tt.Click(c.choose); err != nil {
				t.Fatal(err)
			}
			<-g.asked
			for range 3 {
				r.tt.Drain() // the new sort's list is pending
			}
			assertBounds(t, "while the list loads", before, boundsOf(t, r.tt, c.controls))

			close(g.release)
			r.tt.Until("the rows of the new sort", r.tt.Shows(c.shown))
			assertBounds(t, "once the list loaded", before, boundsOf(t, r.tt, c.controls))
		})
	}
}

func boundsOf(t *testing.T, tt *uitest.Session, names []string) map[string]ui.Rect {
	t.Helper()
	out := map[string]ui.Rect{}
	for _, n := range names {
		b, ok := tt.Find(n)
		if !ok {
			t.Fatalf("no %q in %q", n, tt.Texts())
		}
		out[n] = b
	}
	return out
}

func assertBounds(t *testing.T, when string, want, got map[string]ui.Rect) {
	t.Helper()
	for n, w := range want {
		if g := got[n]; g != w {
			t.Errorf("%s the %q control moved from %v to %v", when, n, w, g)
		}
	}
}
