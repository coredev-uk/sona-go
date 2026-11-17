package services

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/privatemode"
)

// playBoth queues the two library songs and plays the first.
func (ce *commandEnv) playBoth(t *testing.T) {
	t.Helper()
	refs := []TrackRef{{ID: "i.100", Source: SourceLibrary}, {ID: "i.101", Source: SourceLibrary}}
	if err := ce.play.PlayItems(t.Context(), refs, 0, PlayFrom{}); err != nil {
		t.Fatal(err)
	}
}

// request runs a full request (with its flags).
func (ce *commandEnv) request(t *testing.T, req cli.Request) (any, error) {
	t.Helper()
	return ce.srv.Call(t.Context(), req)
}

// waitStatus waits until the published status satisfies ok.
func (ce *commandEnv) waitStatus(t *testing.T, what string, ok func(cli.Status) bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		if st := ce.status(t, "status"); ok(st) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("status never %s: %+v", what, ce.status(t, "status"))
		}
	}
}

// sent is what reached Apple but reads.
func (ce *commandEnv) sent() []string {
	return slices.DeleteFunc(ce.apple.take(), func(r string) bool { return strings.HasPrefix(r, "GET") })
}

func TestCommandsSeekShuffleRepeatCrossfade(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{})
	if _, err := ce.call(t, "seek", "10"); code(err) != cli.CodeNothingPlaying {
		t.Fatalf("seek with nothing playing: %v", err)
	}
	ce.playBoth(t)
	// Absolute, by an amount, and kept within the song (60 s here).
	for _, tc := range []struct {
		arg  string
		want float64
	}{{"30", 30}, {"+10.5", 40.5}, {"-100", 0}, {"9999", 60}} {
		if st := ce.status(t, "seek", tc.arg); math.Abs(st.Position-tc.want) > 0.5 {
			t.Errorf("seek %s → %.1f s, want %.1f", tc.arg, st.Position, tc.want)
		}
	}

	for _, tc := range []struct {
		arg  string
		want bool
	}{{"status", false}, {"on", true}, {"status", true}, {"toggle", false}} {
		if st := ce.status(t, "shuffle", tc.arg); st.Shuffle != tc.want {
			t.Errorf("shuffle %s → %v", tc.arg, st.Shuffle)
		}
	}

	// cycle moves on as the player bar's button: off, all, one, off.
	for _, tc := range []struct{ arg, want string }{
		{"status", "off"}, {"cycle", "all"}, {"cycle", "one"}, {"cycle", "off"}, {"one", "one"}, {"off", "off"},
	} {
		if st := ce.status(t, "repeat", tc.arg); st.Repeat != tc.want {
			t.Errorf("repeat %s → %q, want %q", tc.arg, st.Repeat, tc.want)
		}
	}

	// Crossfade is the saved setting, announced to the UI like the
	// settings page's switch; its length stays.
	seconds := ce.settings.Get().Crossfade.Seconds
	if st := ce.status(t, "crossfade", "on"); !st.Crossfade || !ce.settings.Get().Crossfade.Enabled || ce.settings.Get().Crossfade.Seconds != seconds {
		t.Errorf("crossfade on → %v, saved %+v", st.Crossfade, ce.settings.Get().Crossfade)
	}
	if st := ce.status(t, "status"); !st.Crossfade {
		t.Error("the status lost crossfade")
	}
	ce.rec.mu.Lock()
	announced := slices.Contains(ce.rec.events, EventSettingsChanged)
	ce.rec.mu.Unlock()
	if !announced {
		t.Error("the UI never heard of the setting")
	}
	if st := ce.status(t, "crossfade", "toggle"); st.Crossfade || ce.settings.Get().Crossfade.Enabled {
		t.Errorf("crossfade toggle → %v", st.Crossfade)
	}
}

func TestCommandsPrevForceAndQueueVerbs(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{})
	ce.playBoth(t)
	if st := ce.status(t, "next"); st.Track.ID != "101" {
		t.Fatalf("next → %+v", st.Track)
	}
	// Past its first seconds, prev restarts the song; --force goes back.
	ce.status(t, "seek", "30")
	if st := ce.status(t, "prev"); st.Track.ID != "101" || st.Position > 1 {
		t.Errorf("prev → %s at %.1f s, want 101 restarted", st.Track.ID, st.Position)
	}
	ce.status(t, "seek", "30")
	res, err := ce.request(t, cli.Request{Command: "prev", Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if st := res.(cli.Status); st.Track.ID != "100" {
		t.Errorf("prev --force → %s, want 100", st.Track.ID)
	}

	// jump counts from 1, as `sona queue` prints the places.
	if st := ce.status(t, "queue", "jump", "2"); st.Track.ID != "101" {
		t.Errorf("queue jump 2 → %+v", st.Track)
	}
	if _, err := ce.call(t, "queue", "jump", "3"); code(err) != cli.CodeUsage {
		t.Errorf("a jump past the end: %v", err)
	}
	// clear keeps the playing song, as the queue's Clear.
	ce.status(t, "queue", "jump", "1")
	res, err = ce.call(t, "queue", "clear")
	if err != nil {
		t.Fatal(err)
	}
	if q := res.(cli.Queue); q.Index != 0 || len(q.Tracks) != 1 || q.Tracks[0].ID != "100" {
		t.Errorf("queue clear → %+v", q)
	}
}

// catalogSong serves catalog song 102, which isn't in the library.
func catalogSong(w http.ResponseWriter, _ *http.Request) {
	_, _ = fmt.Fprint(w, `{"data":[{"id":"102","type":"songs","attributes":{"name":"Octopus's Garden","artistName":"The Beatles",
		"albumName":"Abbey Road","durationInMillis":171000,"playParams":{"id":"102","kind":"song"}}}]}`)
}

func TestCommandsDislikeLibraryAndTheSongInTheStatus(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{})
	ce.signIn(t)
	ce.e.Gate.Open()
	if err := ce.play.PlayItems(t.Context(), []TrackRef{{ID: "i.100", Source: SourceLibrary}}, 0, PlayFrom{}); err != nil {
		t.Fatal(err)
	}
	// The status learns the song's state as the player bar does: Apple says
	// song 100 is a favourite in the library.
	ce.waitStatus(t, "liked", func(st cli.Status) bool { return st.Liked && st.InLibrary && !st.Disliked })
	ce.apple.take()

	for _, tc := range []struct {
		cmd, arg               string
		liked, disliked, inLib bool
		sends                  bool
	}{
		{"dislike", "status", true, false, true, false},
		{"dislike", "on", false, true, true, true}, // Suggest Less replaces the favourite
		{"dislike", "toggle", false, false, true, true},
		{"library", "status", false, false, true, false},
		{"library", "add", false, false, true, false}, // already there
		{"library", "remove", false, false, false, true},
	} {
		res, err := ce.call(t, tc.cmd, tc.arg)
		if err != nil {
			t.Fatalf("%s %s: %v", tc.cmd, tc.arg, err)
		}
		l := res.(cli.Like)
		if l.Liked != tc.liked || l.Disliked != tc.disliked || l.InLibrary != tc.inLib || l.Track.ID != "100" {
			t.Errorf("%s %s = %+v", tc.cmd, tc.arg, l)
		}
		if sent := ce.sent(); (len(sent) > 0) != tc.sends {
			t.Errorf("%s %s sent %v", tc.cmd, tc.arg, sent)
		}
	}
	// The status follows the changes made here (and in the UI).
	ce.waitStatus(t, "out of the library", func(st cli.Status) bool { return !st.InLibrary && !st.Liked && !st.Disliked })

	// A catalogue song not in the library is added to it.
	ce.songs = catalogSong
	if err := ce.play.PlayItems(t.Context(), []TrackRef{{ID: "102", Source: SourceCatalog}}, 0, PlayFrom{}); err != nil {
		t.Fatal(err)
	}
	res, err := ce.call(t, "library", "toggle")
	if err != nil {
		t.Fatal(err)
	}
	if l := res.(cli.Like); !l.InLibrary || l.Track.ID != "102" {
		t.Errorf("library toggle of 102 = %+v", l)
	}
	if sent := ce.sent(); len(sent) != 1 || !strings.HasPrefix(sent[0], "POST /v1/me/library") {
		t.Errorf("adding sent %v", sent)
	}
	ce.waitStatus(t, "in the library", func(st cli.Status) bool { return st.Track != nil && st.Track.ID == "102" && st.InLibrary })
}

func TestCommandsLibraryLists(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{})
	ctx := t.Context()
	for _, q := range []string{
		`INSERT INTO playlists (id, library_id, name, artwork_template, updated_at) VALUES (1, 'p.old', 'Older', '', 0), (2, 'p.new', 'Road trip', 'https://is1-ssl.mzstatic.com/image/thumb/x/{w}x{h}{c}.{f}', 0)`,
		`INSERT INTO albums (id, library_id, name, artist_name, updated_at) VALUES (1, 'l.abbey', 'Abbey Road', 'The Beatles', 0)`,
		`INSERT INTO library_items (kind, ref, date_added, generation) VALUES ('playlist', 1, 1, 1), ('playlist', 2, 2, 1), ('album', 1, 1, 1)`,
	} {
		if _, err := ce.e.DB.W.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	res, err := ce.call(t, "library", "playlists")
	if err != nil {
		t.Fatal(err)
	}
	pl := res.(cli.LibraryItems)
	if pl.Kind != "playlist" || pl.Total != 2 || len(pl.Items) != 2 || pl.Items[0].Name != "Road trip" || pl.Items[1].Name != "Older" ||
		pl.Items[0].Artwork != "https://is1-ssl.mzstatic.com/image/thumb/x/600x600bb.jpg" {
		t.Errorf("playlists = %+v", pl)
	}
	res, err = ce.call(t, "library", "albums")
	if err != nil {
		t.Fatal(err)
	}
	al := res.(cli.LibraryItems)
	if al.Kind != "album" || al.Total != 1 || len(al.Items) != 1 || al.Items[0].Subtitle != "The Beatles" {
		t.Errorf("albums = %+v", al)
	}
	// Their links are what `play` and `open` take.
	for _, it := range []struct {
		url  string
		kind app.Kind
		id   string
	}{{pl.Items[0].URL, app.KindLibraryPlaylist, "p.new"}, {al.Items[0].URL, app.KindLibraryAlbum, "l.abbey"}} {
		if tg, err := app.ParseURL(it.url); err != nil || tg.Kind != it.kind || tg.ID != it.id {
			t.Errorf("%s: %+v, %v", it.url, tg, err)
		}
	}
}

func TestCommandsPlayLink(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{})
	ce.songs = catalogSong
	for link, want := range map[string]string{
		"https://example.com/x":                                cli.CodeBadLink,
		"https://music.apple.com/gb/artist/the-beatles/136975": cli.CodeUsage, // opens, doesn't play
	} {
		if _, err := ce.call(t, "play", link); code(err) != want {
			t.Errorf("play %s: %v, want %s", link, err, want)
		}
	}
	res, err := ce.request(t, cli.Request{Command: "play", Args: []string{"https://music.apple.com/gb/song/octopuss-garden/102"}, Shuffle: true})
	if err != nil {
		t.Fatal(err)
	}
	if st := res.(cli.Status); st.Track == nil || st.Track.ID != "102" || !st.Shuffle || st.State != cli.StatePlaying {
		t.Errorf("play a song link = %+v", st)
	}
	// Without a link, play resumes as before.
	ce.status(t, "pause")
	if st := ce.status(t, "play"); st.State != cli.StatePlaying || st.Track.ID != "102" {
		t.Errorf("play → %+v", st)
	}
}

// An add-on's jump counts places in the queue it sees: without the songs
// heard privately.
func TestAddonQueueJumpCountsTheQueueItSees(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{})
	if _, err := ce.e.Private.Set(true); err != nil {
		t.Fatal(err)
	}
	ce.playBoth(t) // 100 is heard privately: hidden
	ce.srv.addon.observe()
	if _, err := ce.e.Private.Set(false); err != nil {
		t.Fatal(err)
	}
	ce.status(t, "next")
	ce.srv.addon.observe()
	if q := ce.srv.addon.queueNow(); len(q.Tracks) != 1 || q.Tracks[0].ID != "101" || q.Index != 0 {
		t.Fatalf("the add-on's queue = %+v", q)
	}
	res, err := ce.srv.addon.Call(context.Background(), cli.Request{Command: "queue", Args: []string{"jump", "1"}})
	if err != nil {
		t.Fatal(err)
	}
	if st := res.(cli.Status); st.Track == nil || st.Track.ID != "101" {
		t.Errorf("add-on jump 1 → %+v, want 101 (its place 1)", st.Track)
	}
	if _, err := ce.srv.addon.Call(context.Background(), cli.Request{Command: "queue", Args: []string{"jump", "2"}}); code(err) != cli.CodeUsage {
		t.Errorf("add-on jump past its queue: %v", err)
	}
}
