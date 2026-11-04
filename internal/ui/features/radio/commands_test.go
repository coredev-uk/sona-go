package radio

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// actionOf is a registered action of the radio module by ID.
func actionOf(t *testing.T, rg *rig, kind kit.ItemKind, id string) *kit.ActionDef {
	t.Helper()
	for _, d := range rg.h.Env.Actions.For(kind) {
		if d.ID == id {
			return d
		}
	}
	t.Fatalf("no action %s for %s", id, kind)
	return nil
}

func TestModuleDeclaresV020sRoutesNavActionsAndPlayer(t *testing.T) {
	rg := newRig(t, radioPath, 800, 600)
	m := New(rg.h.Env)
	if m.ID != "radio" {
		t.Errorf("ID %q", m.ID)
	}
	var patterns []string
	for _, r := range m.Routes {
		patterns = append(patterns, r.Pattern)
	}
	if !slices.Equal(patterns, []string{"/radio", "/station/{id}"}) {
		t.Errorf("routes %q", patterns)
	}
	if len(m.Nav) != 1 || m.Nav[0].Label() != "Radio" || m.Nav[0].Icon != "radio" || m.Nav[0].To != "/radio" ||
		m.Nav[0].Order != 18 || m.Nav[0].Section != 0 {
		t.Errorf("nav %+v", m.Nav)
	}
	var ids []string
	for _, a := range m.Actions {
		ids = append(ids, a.ID)
	}
	want := []string{"radio.play-station", "radio.create-station", "radio.create-station-album", "radio.create-station-artist"}
	if !slices.Equal(ids, want) {
		t.Errorf("action IDs %q, want %q", ids, want)
	}
	if m.Init == nil {
		t.Error("no Init")
	}
}

func TestPlayStationActionPlaysAStationAndIsOffWhenItCannotPlay(t *testing.T) {
	rg := newRig(t, radioPath, 800, 600)
	play := actionOf(t, rg, kit.KindStation, "radio.play-station")
	if play.Group != kit.GroupQueue || play.Order != 10 || play.Label(nil) != "Play" {
		t.Errorf("action %+v", play)
	}
	on := rg.h.Env.Actions.Context([]kit.Item{kit.StationItem(appleMusic1)}, nil, nil)
	if play.Enabled != nil && !play.Enabled(on) {
		t.Error("a playable station's Play is off")
	}
	off := appleMusic1
	off.Playable = false
	if play.Enabled(rg.h.Env.Actions.Context([]kit.Item{kit.StationItem(off)}, nil, nil)) {
		t.Error("an unplayable station's Play is on")
	}
	play.Run(on)
	rg.tt.Until("the play", func() bool { return slices.Equal(rg.api.plays(), []string{"ra.1"}) })
}

func TestCreateStationActionsAreListedOnlyWithACatalogueID(t *testing.T) {
	rg := newRig(t, radioPath, 800, 600)
	ctx := func(i kit.Item) *kit.ActionContext { return rg.h.Env.Actions.Context([]kit.Item{i}, nil, nil) }
	for _, c := range []struct {
		id         string
		kind       kit.ItemKind
		with, none kit.Item
	}{
		{"radio.create-station", kit.KindSong, kit.SongItem(services.Song{CatalogID: "1", Name: "Airbag"}), kit.SongItem(services.Song{Name: "Upload"})},
		{"radio.create-station-album", kit.KindAlbum, kit.AlbumItem(services.Album{CatalogID: "2", Name: "OK Computer"}), kit.AlbumItem(services.Album{Name: "Mine"})},
		{"radio.create-station-artist", kit.KindArtist, kit.ArtistItem(services.Artist{CatalogID: "3", Name: "Radiohead"}), kit.ArtistItem(services.Artist{Name: "Local"})},
	} {
		d := actionOf(t, rg, c.kind, c.id)
		if d.Group != kit.GroupQueue || d.Order != 30 || d.Label(nil) != "Create station" || d.Icon != "radio" {
			t.Errorf("%s: %+v", c.id, d)
		}
		if !d.Available(ctx(c.with)) {
			t.Errorf("%s is not listed for an item with a catalogue ID", c.id)
		}
		if d.Available(ctx(c.none)) {
			t.Errorf("%s is listed for an item with no catalogue ID", c.id)
		}
	}
}

func TestCreateStationRunsAndSaysWhatPlays(t *testing.T) {
	rg := newRig(t, radioPath, 800, 600, fake(func(f *fakeRadio) { f.created = services.Station{ID: "ra.1", Name: "Airbag Station"} }))
	for _, c := range []struct {
		id    string
		kind  kit.ItemKind
		item  kit.Item
		seed  string
		catID string
	}{
		{"radio.create-station", kit.KindSong, kit.SongItem(services.Song{CatalogID: "1", Name: "Airbag"}), services.SeedSong, "1"},
		{"radio.create-station-album", kit.KindAlbum, kit.AlbumItem(services.Album{CatalogID: "2", Name: "OK Computer"}), services.SeedAlbum, "2"},
		{"radio.create-station-artist", kit.KindArtist, kit.ArtistItem(services.Artist{CatalogID: "3", Name: "Radiohead"}), services.SeedArtist, "3"},
	} {
		d := actionOf(t, rg, c.kind, c.id)
		d.Run(rg.h.Env.Actions.Context([]kit.Item{c.item}, nil, nil))
		rg.tt.Until("the station", func() bool {
			rg.api.mu.Lock()
			defer rg.api.mu.Unlock()
			return slices.Contains(rg.api.creates, [2]string{c.seed, c.catID})
		})
	}
	rg.toastShown("Playing Airbag Station")
}

func TestCreateStationThatFailsNamesWhatItWasMadeFromAndWhy(t *testing.T) {
	for _, c := range []struct {
		err  error
		want string
	}{
		{fmt.Errorf("x: %w", auth.ErrNotSignedIn), "Sign in to Apple Music from the account menu in the top bar."},
		{fmt.Errorf("x: %w", auth.ErrMUTRejected), "Sign in to Apple Music from the account menu in the top bar."},
		{fmt.Errorf("x: %w", applemusic.ErrNotFound), "Apple Music has no station for this."},
		{errors.New("boom"), "Something went wrong."},
	} {
		rg := newRig(t, radioPath, 800, 600, fake(func(f *fakeRadio) { f.createErr = c.err }))
		d := actionOf(t, rg, kit.KindSong, "radio.create-station")
		d.Run(rg.h.Env.Actions.Context([]kit.Item{kit.SongItem(services.Song{CatalogID: "1", Name: "Airbag"})}, nil, nil))
		rg.toastShown("Couldn't start a station from Airbag")
		rg.tt.Until("the reason", rg.tt.Shows(c.want))
	}
}

func TestStationFailureWording(t *testing.T) {
	for _, c := range []struct {
		err  error
		want string
	}{
		{fmt.Errorf("x: %w", auth.ErrNotSignedIn), "Sign in to Apple Music from the account menu in the top bar."},
		{fmt.Errorf("x: %w", auth.ErrMUTRejected), "Sign in to Apple Music from the account menu in the top bar."},
		{fmt.Errorf("x: %w", applemusic.ErrNotFound), "Apple Music has no station for this."},
		// Anything else is worded by its code, as everywhere.
		{fmt.Errorf("x: %w", applemusic.ErrNotPlayable), "This song isn't available to stream in your storefront."},
		{context.DeadlineExceeded, "That took too long and timed out. Try again."},
	} {
		if got := stationFailure(c.err); got != c.want {
			t.Errorf("stationFailure(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}

func TestStationsPlayThroughTheSharedItemPlayers(t *testing.T) {
	rg := newRig(t, radioPath, 800, 600)
	if rg.h.Env.Players.CanPlay(stationItemOf(appleMusic1)) {
		t.Fatal("stations play before the radio module registered its player")
	}
	New(rg.h.Env)
	if !rg.h.Env.Players.CanPlay(stationItemOf(appleMusic1)) {
		t.Error("a playable station has no play control")
	}
	off := appleMusic1
	off.Playable = false
	if rg.h.Env.Players.CanPlay(stationItemOf(off)) {
		t.Error("an unplayable station has a play control")
	}
}

// replaceRig is a queueCommands over the Env's mirror, with a played queue of
// n songs the user added.
func replaceRig(t *testing.T, added int, mode string) (*rig, *queueCommands) {
	t.Helper()
	rg := newRig(t, radioPath, 800, 600)
	items := []services.QueueItem{{ID: "now"}}
	for i := range added {
		items = append(items, services.QueueItem{ID: fmt.Sprint("add", i), Added: true})
	}
	rg.h.Bus.Emit(services.EventPlaybackQueue, services.QueueState{Index: 0, Items: items})
	rg.tt.Drain()
	q := &queueCommands{
		play: rg.h.Env.Play, post: rg.h.Env.Post, svc: rg.pb,
		mode: func() string { return mode }, fail: stationFailed,
	}
	return rg, q
}

func TestReplaceStartsAtOnceOverAShortQueue(t *testing.T) {
	rg, q := replaceRig(t, longUserQueue, config.ReplaceConfirm)
	started := make(chan bool, 1)
	q.Replace("Couldn't play this station", func(context.Context) error { return nil }, func(ok bool) { started <- ok })
	rg.tt.Until("started", func() bool { return len(started) == 1 })
	if !<-started {
		t.Error("not started")
	}
	// Three songs the user added are not a long queue: no question, no Undo.
	if kit.ConfirmPending() || rg.tt.HasText("Queue replaced") {
		t.Errorf("a short queue was asked about: %q", rg.tt.Texts())
	}
}

func TestReplaceOffersUndoOverALongQueue(t *testing.T) {
	rg, q := replaceRig(t, longUserQueue+1, config.ReplaceUndo)
	q.Replace("Couldn't play this station", func(context.Context) error { return nil }, nil)
	rg.toastShown("Queue replaced")
	rg.tt.Until("the description", rg.tt.Shows("4 songs you added were removed.", "Undo"))
	rg.click("Undo")
	rg.tt.Until("the undo", func() bool { _, n := rg.pb.counts(); return n == 1 })
}

func TestReplaceAsksFirstWhenTheSettingSaysConfirm(t *testing.T) {
	rg, q := replaceRig(t, longUserQueue+2, config.ReplaceConfirm)
	started := make(chan bool, 2)
	ran := make(chan struct{}, 2)
	replace := func() {
		q.Replace("Couldn't play this station", func(context.Context) error { ran <- struct{}{}; return nil }, func(ok bool) { started <- ok })
	}
	replace()
	rg.tt.Until("the question", rg.tt.Shows("Replace your queue?", "The 5 songs you added to the queue will be removed."))
	if len(ran) != 0 {
		t.Fatal("the queue was replaced before the answer")
	}
	// No: nothing starts and nothing is offered back.
	rg.click("Cancel")
	if ok := <-started; ok || len(ran) != 0 {
		t.Errorf("a refused question started the station (started %v, ran %d)", ok, len(ran))
	}
	// Yes: it starts, and with the question answered there is no Undo.
	replace()
	rg.tt.Until("the question", rg.tt.Shows("Replace your queue?"))
	rg.click("Replace queue")
	rg.tt.Until("started", func() bool { return len(started) == 1 })
	if ok := <-started; !ok || len(ran) != 1 {
		t.Errorf("a confirmed question did not start the station (started %v, ran %d)", ok, len(ran))
	}
	rg.tt.Drain()
	if rg.tt.HasText("Queue replaced") {
		t.Error("Undo was offered after the user confirmed")
	}
}

func TestReplaceThatFailsSaysWhyAndNotifiesNoUndo(t *testing.T) {
	rg, q := replaceRig(t, longUserQueue+1, config.ReplaceUndo)
	done := make(chan bool, 1)
	q.Replace("Couldn't play this station", func(context.Context) error { return fmt.Errorf("x: %w", applemusic.ErrNotFound) }, func(ok bool) { done <- ok })
	rg.toastShown("Couldn't play this station")
	rg.tt.Until("the reason", rg.tt.Shows("Apple Music has no station for this."))
	if ok := <-done; ok {
		t.Error("a failed start reported success")
	}
	if rg.tt.HasText("Queue replaced") {
		t.Error("Undo was offered for a queue that was not replaced")
	}
}
