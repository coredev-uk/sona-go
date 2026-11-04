package playlist

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// waitDid waits for a call the feature makes off the main thread.
func waitDid(t *testing.T, api *fakeAPI, call string) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if api.did(call) {
			return
		}
	}
	t.Fatalf("timed out waiting for %q; calls %q", call, api.asked())
}

// waitPlayed waits for n play calls.
func waitPlayed(t *testing.T, play *fakePlay, n int) []string {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if got := play.played(); len(got) >= n {
			return got
		}
	}
	t.Fatalf("timed out waiting for %d play calls; got %q", n, play.played())
	return nil
}

// overlayView mounts what commands show: toasts and the confirm question.
func overlayView(t *testing.T, h *uitest.Harness) *uitest.Session {
	t.Helper()
	t.Cleanup(kit.CancelConfirm)
	return h.Tester(t, func(c *ui.Context) {
		c.DisableAnimations()
		ui.Column(c).Children(func() {})
		kit.Toaster(c)
		kit.ConfirmHost(c)
	}, 800, 600)
}

func TestPlaylistItems(t *testing.T) {
	items := []kit.Item{
		kit.SongItem(services.Song{ID: "s.1", Source: services.SourceLibrary}),
		kit.AlbumItem(services.Album{ID: "1440", Source: services.SourceCatalog}),
		kit.PlaylistItem(services.Playlist{ID: "p.1", Source: services.SourceLibrary}),
		kit.ArtistItem(services.Artist{ID: "r.1", Source: services.SourceLibrary}),
	}
	want := []services.PlaylistItem{
		{Kind: "song", ID: "s.1", Source: services.SourceLibrary},
		{Kind: "album", ID: "1440", Source: services.SourceCatalog},
		{Kind: "playlist", ID: "p.1", Source: services.SourceLibrary},
	}
	if got := playlistItems(items); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got := playlistItems(items[3:]); len(got) != 0 {
		t.Errorf("an artist alone: %v", got)
	}
}

func TestLibraryIDOf(t *testing.T) {
	for _, c := range []struct {
		name string
		item kit.Item
		want string
	}{
		{"library song", kit.SongItem(services.Song{ID: "i.1", Source: services.SourceLibrary}), "i.1"},
		{"catalogue song with a library copy", kit.SongItem(services.Song{ID: "99", Source: services.SourceCatalog, LibraryID: "i.7"}), "i.7"},
		{"catalogue song", kit.SongItem(services.Song{ID: "99", Source: services.SourceCatalog}), ""},
		{"library album", kit.AlbumItem(services.Album{ID: "l.1", Source: services.SourceLibrary}), "l.1"},
		{"catalogue album in the library", kit.AlbumItem(services.Album{ID: "55", Source: services.SourceCatalog, LibraryID: "l.2"}), "l.2"},
		{"library artist", kit.ArtistItem(services.Artist{ID: "r.1", Source: services.SourceLibrary}), "r.1"},
		{"catalogue artist", kit.ArtistItem(services.Artist{ID: "77", Source: services.SourceCatalog}), ""},
		{"library playlist", kit.PlaylistItem(services.Playlist{ID: "p.1", Source: services.SourceLibrary}), "p.1"},
		{"catalogue playlist in the library", kit.PlaylistItem(services.Playlist{ID: "pl.x", Source: services.SourceCatalog, LibraryID: "p.9"}), "p.9"},
		{"a station", kit.StationItem(services.Station{ID: "ra.1"}), ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := libraryIDOf(c.item); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func unplayable(id string) services.Song {
	s := song(id, id, "A")
	s.Playable = false
	return s
}

func TestPlaySongsStartIndex(t *testing.T) {
	songs := []services.Song{song("a", "A", "X"), unplayable("b"), song("c", "C", "X"), song("d", "D", "X")}
	from := services.PlayFrom{Kind: "playlist", ID: "p.x"}
	for _, c := range []struct {
		name  string
		start int
		want  string
	}{
		{"first", 0, "[a c d] start=0 from=playlist:p.x"},
		{"after an unplayable one", 2, "[a c d] start=1 from=playlist:p.x"},
		{"last", 3, "[a c d] start=2 from=playlist:p.x"},
		{"the start itself is unplayable: from the top", 1, "[a c d] start=0 from=playlist:p.x"},
		{"out of range: from the top", 9, "[a c d] start=0 from=playlist:p.x"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, _, play, _, _ := testPlaylists(t, "/")
			p.playSongs(t.Context(), songs, c.start, from, false)
			if got := play.played(); !slices.Equal(got, []string{c.want}) {
				t.Errorf("got %q, want %q", got, c.want)
			}
			if play.shuffle {
				t.Error("shuffle turned on")
			}
		})
	}
}

func TestPlaySongsShuffle(t *testing.T) {
	p, _, play, _, _ := testPlaylists(t, "/")
	songs := []services.Song{song("a", "A", "X"), unplayable("b"), song("c", "C", "X")}
	p.playSongs(t.Context(), songs, 0, services.PlayFrom{Kind: "playlist", ID: "p.x"}, true)
	got := play.played()
	if len(got) != 1 {
		t.Fatalf("plays %q", got)
	}
	if !play.shuffle {
		t.Error("shuffle was not turned on")
	}
	// Only the playable songs are queued, from a start inside them.
	if got[0] != "[a c] start=0 from=playlist:p.x" && got[0] != "[a c] start=1 from=playlist:p.x" {
		t.Errorf("got %q", got[0])
	}
}

func TestPlaySongsNothingPlayable(t *testing.T) {
	p, _, play, h, _ := testPlaylists(t, "/")
	tt := overlayView(t, h)
	songs := []services.Song{unplayable("a"), unplayable("b")}
	p.playSongs(t.Context(), songs, 0, services.PlayFrom{}, true)
	tt.Drain()
	if len(play.played()) != 0 || play.shuffle {
		t.Errorf("played %q, shuffle %v", play.played(), play.shuffle)
	}
	if tt.HasText("Nothing here can be played") {
		t.Error("a shuffle of nothing should be silent")
	}
	p.playSongs(t.Context(), songs, 0, services.PlayFrom{}, false)
	tt.Until("the toast", tt.Shows("Nothing here can be played", "None of these songs is available to stream."))
	if len(play.played()) != 0 {
		t.Errorf("played %q", play.played())
	}
}

func TestPlayPin(t *testing.T) {
	for _, c := range []struct {
		name    string
		id      string
		shuffle bool
		want    string
	}{
		{"playlist pin", "p.1", false, "[s1 s2] start=0 from=playlist:p.1"},
		{"album pin", "l.9", false, "[s1 s2] start=0 from=album:l.9"},
		{"song pin has no collection", "i.4", false, "[s1 s2] start=0 from=:i.4"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, api, play, _, _ := testPlaylists(t, "/")
			api.pinTracks[c.id] = []services.Song{song("s1", "One", "A"), song("s2", "Two", "A")}
			p.playPin(c.id, "Mix", c.shuffle)
			got := waitPlayed(t, play, 1)
			if got[0] != c.want {
				t.Errorf("got %q, want %q", got[0], c.want)
			}
		})
	}
	t.Run("shuffle", func(t *testing.T) {
		p, api, play, _, _ := testPlaylists(t, "/")
		api.pinTracks["p.1"] = []services.Song{song("s1", "One", "A"), song("s2", "Two", "A")}
		p.playPin("p.1", "Mix", true)
		waitPlayed(t, play, 1)
		if !play.shuffle {
			t.Error("shuffle was not turned on")
		}
	})
}

func TestPlayPinWithNoSongsSaysSo(t *testing.T) {
	p, _, play, h, _ := testPlaylists(t, "/")
	tt := overlayView(t, h)
	p.playPin("p.empty", "Quiet", false)
	tt.Until("the toast", tt.Shows("“Quiet” has no songs to play"))
	if len(play.played()) != 0 {
		t.Errorf("played %q", play.played())
	}
}

func TestPlayPinFailureIsToasted(t *testing.T) {
	p, api, play, h, _ := testPlaylists(t, "/")
	tt := overlayView(t, h)
	api.err = errOffline
	p.playPin("p.1", "Mix", false)
	tt.Until("the toast", tt.Shows("Couldn't play “Mix”"))
	if len(play.played()) != 0 {
		t.Errorf("played %q", play.played())
	}
}

func TestAddToPlaylistNow(t *testing.T) {
	items := []services.PlaylistItem{{Kind: "song", ID: "s.1", Source: services.SourceLibrary}}
	t.Run("adds and offers to open", func(t *testing.T) {
		p, api, _, h, router := testPlaylists(t, "/")
		api.details["p.1"] = editDetail("a", "b")
		tt := overlayView(t, h)
		p.addToPlaylistNow("p.1", "Mix", items, "“Hundred”")
		waitDid(t, api, "Add p.1 [song:s.1]")
		tt.Until("the toast", tt.Shows("Added “Hundred” to “Mix”", "Open"))
		if err := tt.Click("Open"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
		if got := router.Path(); got != "/playlist/p.1" {
			t.Errorf("went to %q", got)
		}
	})
	t.Run("failure", func(t *testing.T) {
		p, api, _, h, router := testPlaylists(t, "/")
		api.err = errOffline
		tt := overlayView(t, h)
		p.addToPlaylistNow("p.1", "Mix", items, "“Hundred”")
		tt.Until("the toast", tt.Shows("Couldn't add to “Mix”"))
		if tt.HasText("Open") || router.Path() != "/" {
			t.Error("an Open button after a failure")
		}
	})
}

func TestDeletePlaylist(t *testing.T) {
	const ask = "Delete “Mix”?"
	run := func(t *testing.T, answer string, fail bool) (api *fakeAPI, tt *uitest.Session, done *[]bool) {
		t.Helper()
		p, api, _, h, _ := testPlaylists(t, "/")
		api.err = nil
		if fail {
			api.err = errOffline
		}
		tt = overlayView(t, h)
		done = new([]bool)
		p.deletePlaylist("p.1", "Mix", func(ok bool) { *done = append(*done, ok) })
		tt.Until("the question", tt.Shows(ask, "Delete playlist", "Cancel"))
		if err := tt.Click(answer); err != nil {
			t.Fatal(err)
		}
		return api, tt, done
	}
	t.Run("confirmed", func(t *testing.T) {
		api, tt, done := run(t, "Delete playlist", false)
		waitDid(t, api, "Delete p.1")
		tt.Until("the toast", tt.Shows("Deleted “Mix”"))
		if !slices.Equal(*done, []bool{true}) {
			t.Errorf("done %v", *done)
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		api, tt, done := run(t, "Cancel", false)
		tt.Drain()
		if api.did("Delete p.1") {
			t.Error("deleted after Cancel")
		}
		if !slices.Equal(*done, []bool{false}) {
			t.Errorf("done %v", *done)
		}
	})
	t.Run("failure", func(t *testing.T) {
		api, tt, done := run(t, "Delete playlist", true)
		waitDid(t, api, "Delete p.1")
		tt.Until("the toast", tt.Shows("Couldn't delete “Mix”"))
		if !slices.Equal(*done, []bool{false}) {
			t.Errorf("done %v", *done)
		}
		if tt.HasText("Deleted “Mix”") {
			t.Error("success toast after a failure")
		}
	})
}

func TestDeleteActionLeavesTheDeletedPlaylistsPage(t *testing.T) {
	for _, c := range []struct {
		name   string
		start  string
		routes bool // the library's Playlists page is ported
		want   string
	}{
		{"on its page", "/playlist/p.1", true, playlistsPath},
		{"on its page, no Playlists page: back", "/playlist/p.1", false, "/library/albums"},
		{"elsewhere", "/library/albums", true, "/library/albums"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, api, _, h, router := testPlaylists(t, "/library/albums")
			if c.start != "/library/albums" {
				router.Push(c.start)
			}
			h.Env.HasRoute = func(path string) bool { return c.routes && path == playlistsPath }
			tt := overlayView(t, h)
			r := kit.NewActionRegistry(p.actions()...)
			r.SetNavigator(func(path string) { router.Push(path) })
			var del *kit.ActionDef
			for _, d := range p.actions() {
				if d.ID == "playlists.delete" {
					del = &d
				}
			}
			ctx := r.Context([]kit.Item{libPlaylist("p.1", true, "")}, nil, map[string]any{})
			del.Run(ctx)
			tt.Until("the question", tt.Shows("Delete “PL p.1”?"))
			if err := tt.Click("Delete playlist"); err != nil {
				t.Fatal(err)
			}
			waitDid(t, api, "Delete p.1")
			tt.Until("the toast", tt.Shows("Deleted “PL p.1”"))
			if got := router.Path(); got != c.want {
				t.Errorf("path %q, want %q", got, c.want)
			}
		})
	}
}

func TestDeleteFolder(t *testing.T) {
	p, api, _, h, _ := testPlaylists(t, "/")
	tt := overlayView(t, h)
	p.deleteFolder("f.a", "Old")
	tt.Until("the question", tt.Shows("Delete the folder “Old”?", "Delete folder"))
	if err := tt.Click("Delete folder"); err != nil {
		t.Fatal(err)
	}
	waitDid(t, api, "DeleteFolder f.a")
	tt.Until("the toast", tt.Shows("Deleted the folder “Old”"))
}
