package library

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// favouriteSong rates the fixture song j of album i Favourite, as the
// ratings sync mirrors it.
func favouriteSong(r *pageRig, i, j int, at int64) {
	r.t.Helper()
	if _, err := r.h.DB.W.ExecContext(r.t.Context(),
		`INSERT INTO favorites (kind, apple_id, value, updated_at) VALUES ('library-songs', ?, 1, ?)`,
		fixture.SongLibraryID(i, j), at); err != nil {
		r.t.Fatal(err)
	}
}

// newFavouritesRig is the page with a signed-in account (the harness keeps
// no account; a library that is empty because it is signed out is tested
// with newSongsRig).
func newFavouritesRig(t *testing.T, before func(l *library), opts ...uitest.Option) *pageRig {
	t.Helper()
	return newSongsRig(t, favouritePath, func(l *library) {
		fakeAuth(l, songsAccount{"signed_in"})
		if before != nil {
			before(l)
		}
	}, opts...)
}

func TestFavouriteSongsListTheRatedSongsNewestFirst(t *testing.T) {
	r := newFavouritesRig(t, nil)
	r.tt.Until("no favourites", r.tt.Shows("No favourite songs yet"))
	favouriteSong(r, 1, 0, 10) // Apple Orchard song 1
	favouriteSong(r, 4, 1, 20) // Blue Hour song 2
	// A rating changed outside the UI (`sona like`) refreshes the list.
	r.h.Env.Bus.Emit(services.EventItemState, services.ItemState{Kind: "song", ID: "x"})
	r.tt.Until("two favourites", r.tt.Shows("Favourite songs", "2 songs", "Blue Hour song 2", "Apple Orchard song 1"))
	if got, want := songsOrder(r), []string{"Blue Hour song 2", "Apple Orchard song 1"}; !slices.Equal(got, want) {
		t.Errorf("favourites shown %q, want %q", got, want)
	}
	if r.tt.HasText("No favourite songs yet") {
		t.Error("the empty state stays")
	}
}

func TestFavouriteSongsPlayAndShuffle(t *testing.T) {
	r := newFavouritesRig(t, nil)
	r.tt.Until("the page", r.tt.Shows("No favourite songs yet"))
	favouriteSong(r, 1, 0, 10)
	favouriteSong(r, 4, 1, 20)
	r.h.Env.Bus.Emit(services.EventSyncDone, services.SyncDone{OK: true})
	r.tt.Until("two favourites", r.tt.Shows("2 songs", "Apple Orchard song 1"))

	if err := r.tt.Click("Play your favourite songs"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the engine to play", func() bool { return len(songsPlayed(r)) > 0 })
	if got := songsPlayed(r)[0]; got != "10041" { // Blue Hour song 2, rated last
		t.Errorf("Play started %v, want 10041", got)
	}
	pb := services.Service[services.PlaybackService](r.h.Core)
	if q := pb.Queue(); len(q.Items) != 2 || q.Items[1].Name != "Apple Orchard song 1" {
		t.Errorf("queue %+v", q.Items)
	}
	if st := pb.State(); st.Context != "Favourite songs" || st.Shuffle {
		t.Errorf("state context %q shuffle %v", st.Context, st.Shuffle)
	}

	// A double-click plays from that row.
	n := len(songsPlayed(r))
	songsClickRow(r, "Apple Orchard song 1", 0, 2)
	r.tt.Until("the row to play", func() bool { return len(songsPlayed(r)) > n })
	if got := songsPlayed(r); got[len(got)-1] != "10010" {
		t.Errorf("double-click played %v", got)
	}

	if err := r.tt.Click("Shuffle"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("shuffle on", func() bool { return pb.State().Shuffle })
}

func TestFavouriteSongsKeyboardAndSelection(t *testing.T) {
	r := newFavouritesRig(t, nil)
	r.tt.Until("the page", r.tt.Shows("No favourite songs yet"))
	favouriteSong(r, 1, 0, 10)
	favouriteSong(r, 4, 1, 20)
	favouriteSong(r, 6, 0, 30)
	r.h.Env.Bus.Emit(services.EventSyncDone, services.SyncDone{OK: true})
	r.tt.Until("three favourites", r.tt.Shows("3 songs"))
	sel := &r.l.favourite.list.st.Sel
	songsClickRow(r, "Paper Planes song 1", 0, 1)
	if sel.Len() != 1 {
		t.Fatalf("a click selected %d rows", sel.Len())
	}
	songsClickRow(r, "Apple Orchard song 1", ui.Cmd, 1)
	if sel.Len() != 2 {
		t.Errorf("Ctrl+click selected %d rows, want 2", sel.Len())
	}
	r.tt.Key(0, ui.KeyDown)
	r.tt.Key(ui.Shift, ui.KeyEnd)
	r.tt.Key(0, ui.KeyHome)
	if !r.tt.Focused("Paper Planes song 1") {
		t.Errorf("Home: focus is not on the first row")
	}
	r.tt.Key(0, ui.KeyEnd)
	if !r.tt.Focused("Apple Orchard song 1") {
		t.Errorf("End: focus is not on the last row")
	}
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Until("Enter to play", func() bool { return len(songsPlayed(r)) > 0 })
	if got := songsPlayed(r)[0]; got != "10010" {
		t.Errorf("Enter played %v, want 10010", got)
	}
}

func TestFavouriteSongsEmptyStates(t *testing.T) {
	r := newFavouritesRig(t, nil)
	r.tt.Until("the empty text", r.tt.Shows("Favourite songs", "No favourite songs yet",
		"Choose Favourite on any song, here or in Apple Music, and it is kept here and in your library."))
	if r.tt.HasText("0 songs") {
		t.Error("an empty list counts its songs")
	}

	opened := false
	r = newSongsRig(t, favouritePath, func(l *library) { l.openAccount = func() { opened = true } }, uitest.Empty())
	r.tt.Until("the signed-out text", r.tt.Shows("You need to sign in to see your favourite songs",
		"Songs you favourite in Sona or any Apple Music app are collected here."))
	if err := r.tt.Click("Sign in"); err != nil {
		t.Fatal(err)
	}
	if !opened {
		t.Error("Sign in did not open the account dialog")
	}
}

func TestFavouriteSongsErrorStateRetries(t *testing.T) {
	var f *songsFailing
	r := newFavouritesRig(t, func(l *library) {
		f = &songsFailing{libraryAPI: l.lib}
		f.fail.Store(true)
		l.lib = f
	})
	r.tt.Until("the error", r.tt.Shows("Couldn't read your favourite songs from this computer", "Try again"))
	favouriteSong(r, 1, 0, 10)
	f.fail.Store(false)
	if err := r.tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the song", r.tt.Shows("1 song", "Apple Orchard song 1"))
}
