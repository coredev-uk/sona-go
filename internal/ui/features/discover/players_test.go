package discover

import (
	"fmt"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

var sledgehammer = services.MusicVideo{
	ID: "l.abc", Source: services.SourceLibrary, CatalogID: "1442907386", LibraryID: "l.abc",
	Name: "Sledgehammer", Artist: "Peter Gabriel", ReleaseDate: "1986-04-21",
}

var videoCard = services.Item{Kind: services.ItemMusicVideo, ID: "1442907386", Name: "Sledgehammer", Subtitle: "Peter Gabriel", MusicVideo: &sledgehammer}

// capabilities is what the platform says: videos play where video is set.
func capabilities(video bool) func() *services.Capabilities {
	return func() *services.Capabilities {
		return &services.Capabilities{Playback: services.PlaybackCapabilities{Available: true, Video: video}}
	}
}

func playersRig(t *testing.T, opts ...rigOption) *rig {
	t.Helper()
	r := newRig(t, "/new", append([]rigOption{withAPI(&fakeAPI{})}, opts...)...)
	r.env.Players.Capabilities = capabilities(true)
	return r
}

// songs builds n songs, the unplayable ones listed.
func songs(n int, unplayable ...int) []services.Song {
	out := make([]services.Song, n)
	for i := range out {
		out[i] = services.Song{ID: fmt.Sprintf("s%d", i), Source: services.SourceCatalog, Name: fmt.Sprintf("Song %d", i), Playable: true}
	}
	for _, i := range unplayable {
		out[i].Playable = false
	}
	return out
}

func (r *rig) waitPlays(n int) []played {
	r.t.Helper()
	r.tt.Until(fmt.Sprintf("%d plays", n), func() bool { return len(r.play.plays()) >= n })
	return r.play.plays()
}

// players.test.ts: the music video player.
func TestPlayersPlayAMusicVideoCardAloneAsTheQueue(t *testing.T) {
	r := playersRig(t)
	p := r.env.Players
	if !p.CanPlay(videoCard) {
		t.Fatal("a music video card with its video has no play control")
	}
	p.PlayItem(videoCard)
	got := r.waitPlays(1)
	want := played{refs: []services.TrackRef{{ID: "l.abc", Source: services.SourceLibrary, Kind: services.TrackKindVideo}}, from: services.PlayFrom{}}
	if len(got) != 1 || got[0].start != 0 || got[0].from != want.from || len(got[0].refs) != 1 || got[0].refs[0] != want.refs[0] {
		t.Errorf("played %+v, want %+v", got, want)
	}
	r.tt.Until("the video view opens", func() bool { return r.env.Video.IsOpen() })
}

func TestPlayersGiveVideoTilesAPlayControlThatPlaysTheVideoAndQueueActions(t *testing.T) {
	r := playersRig(t)
	ctx := r.env.ItemsCtx(nil)
	tile := kit.CoverItemOf(videoCard, ctx, 0)
	if tile.Play == nil || tile.Play.Label != "Sledgehammer" {
		t.Fatalf("tile play control %+v, want one named by the title", tile.Play)
	}
	tile.Play.Run()
	if got := r.waitPlays(1); got[0].refs[0].ID != "l.abc" || got[0].refs[0].Kind != services.TrackKindVideo {
		t.Errorf("played %+v", got)
	}
	if tile.Item == nil || tile.Item.Kind != kit.KindMusicVideo || tile.Item.MusicVideo() != sledgehammer {
		t.Errorf("tile item %+v, want the music video", tile.Item)
	}
	if it, ok := kit.ActionItemOf(videoCard, r.env.Players); !ok || it.Kind != kit.KindMusicVideo {
		t.Errorf("action item %+v %v", it, ok)
	}
}

func TestPlayersShowNoPlayControlOrActionsWhereVideosDontPlayOrBeforeGoHasAnswered(t *testing.T) {
	r := playersRig(t)
	for name, caps := range map[string]func() *services.Capabilities{
		"windows":       capabilities(false),
		"no answer yet": func() *services.Capabilities { return nil },
	} {
		r.env.Players.Capabilities = caps
		ctx := r.env.ItemsCtx(nil)
		tile := kit.CoverItemOf(videoCard, ctx, 0)
		if r.env.Players.CanPlay(videoCard) || tile.Play != nil || tile.Item != nil {
			t.Errorf("%s: can play %v, play control %v, menu item %v", name, r.env.Players.CanPlay(videoCard), tile.Play, tile.Item)
		}
		if _, ok := kit.ActionItemOf(videoCard, r.env.Players); ok {
			t.Errorf("%s: a video has queue actions", name)
		}
	}
}

func TestPlayersHaveNothingToPlayOnACardWithoutItsVideo(t *testing.T) {
	r := playersRig(t)
	card := videoCard
	card.MusicVideo = nil
	if r.env.Players.CanPlay(card) {
		t.Error("a music video card without its video can play")
	}
}

func TestPlayersPlayASongAloneLabelledByItsName(t *testing.T) {
	r := playersRig(t)
	s := songs(1)[0]
	card := services.Item{Kind: services.ItemSong, ID: s.ID, Name: s.Name, Song: &s}
	if !r.env.Players.CanPlay(card) {
		t.Fatal("a playable song card has no play control")
	}
	r.env.Players.PlayItem(card)
	got := r.waitPlays(1)[0]
	if len(got.refs) != 1 || got.refs[0].ID != "s0" || got.start != 0 || got.from != (services.PlayFrom{ID: "Song 0"}) {
		t.Errorf("played %+v", got)
	}
	unplayable := card
	unplayable.Song = &services.Song{ID: "x"}
	if r.env.Players.CanPlay(unplayable) {
		t.Error("an unplayable song has a play control")
	}
}

func TestPlayersPlayAnAlbumAndAPlaylistFromTheirFirstTrackFromTheCollection(t *testing.T) {
	r := playersRig(t, withTracks(fakeTracks{songs: map[string][]services.Song{
		"album:a1":    songs(3, 0),
		"playlist:p1": songs(2),
	}}))
	album := services.Item{Kind: services.ItemAlbum, ID: "a1", Name: "Album", Album: &services.Album{ID: "a1"}}
	playlist := services.Item{Kind: services.ItemPlaylist, ID: "p1", Name: "Playlist", Playlist: &services.Playlist{ID: "p1"}}
	r.env.Players.PlayItem(album)
	r.env.Players.PlayItem(playlist)
	got := r.waitPlays(2)
	byFrom := map[services.PlayFrom]played{}
	for _, p := range got {
		byFrom[p.from] = p
	}
	// The first song of the album cannot play: the queue starts at its next.
	if a := byFrom[services.PlayFrom{Kind: "album", ID: "a1"}]; len(a.refs) != 2 || a.refs[0].ID != "s1" || a.start != 0 {
		t.Errorf("album played %+v", a)
	}
	if p := byFrom[services.PlayFrom{Kind: "playlist", ID: "p1"}]; len(p.refs) != 2 || p.start != 0 {
		t.Errorf("playlist played %+v", p)
	}
}

func TestPlayersAnUpcomingAlbumHasNoPlayControl(t *testing.T) {
	r := playersRig(t)
	r.d.now = func() time.Time { return time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC) }
	r.env.Players.Now = r.d.now
	soon := services.Item{Kind: services.ItemAlbum, ID: "a2", Name: "Soon", Album: &services.Album{ID: "a2", Prerelease: true, ReleaseAt: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC).UnixMilli()}}
	if r.env.Players.CanPlay(soon) {
		t.Error("an album not out yet has a play control")
	}
}

func TestPlayersAFailedAlbumOrPlaylistToastsWhatFailedWithTheReason(t *testing.T) {
	r := playersRig(t, withTracks(fakeTracks{err: applemusic.ErrNotFound}))
	reason := i18n.ErrorText(services.CodeNotFound)
	r.env.Players.PlayItem(services.Item{Kind: services.ItemAlbum, ID: "a1", Name: "Album", Album: &services.Album{ID: "a1"}})
	r.tt.Until("the album's toast", r.tt.Shows("Couldn't load the album", reason))
	r.env.Players.PlayItem(services.Item{Kind: services.ItemPlaylist, ID: "p1", Name: "Playlist", Playlist: &services.Playlist{ID: "p1"}})
	r.tt.Until("the playlist's toast", r.tt.Shows("Couldn't load the playlist", reason))
	if got := r.play.plays(); len(got) != 0 {
		t.Errorf("played %+v after a failed load", got)
	}
}

func TestPlayersPlayATrackShelfAsTheQueueLabelledBySource(t *testing.T) {
	r := playersRig(t)
	r.env.Players.PlaySongList(songs(4, 1), 2, "Best New Songs")
	got := r.waitPlays(1)[0]
	// The queue skips the unplayable song, so song 2 is the second of three.
	if len(got.refs) != 3 || got.start != 1 || got.refs[got.start].ID != "s2" || got.from != (services.PlayFrom{ID: "Best New Songs"}) {
		t.Errorf("played %+v", got)
	}
}

func TestPlayersNothingPlayableToastsAndStartsNothing(t *testing.T) {
	r := playersRig(t)
	r.env.Players.PlaySongList(songs(2, 0, 1), 0, "Best New Songs")
	r.tt.Until("the toast", r.tt.Shows("Nothing here can be played", "None of these songs is available to stream."))
	if got := r.play.plays(); len(got) != 0 {
		t.Errorf("played %+v", got)
	}
}

func TestPlayersAFailedStartToastsTheCommandAndTheReason(t *testing.T) {
	r := playersRig(t)
	r.play.err = auth401()
	r.env.Players.PlaySongList(songs(2), 0, "Charts")
	r.tt.Until("the toast", r.tt.Shows("Couldn't start playback", i18n.ErrorText(services.CodeNotFound)))
}

// auth401 is an error Go words as "not found".
func auth401() error { return fmt.Errorf("start: %w", applemusic.ErrNotFound) }

// replace.ts: a long queue the user built is asked about, or replaced with an
// Undo.
func TestReplacingALongQueueOffersUndoByDefault(t *testing.T) {
	r := playersRig(t)
	seedQueue(r, 5)
	r.env.Players.PlaySongList(songs(2), 0, "Charts")
	r.waitPlays(1)
	r.tt.Until("the undo toast", r.tt.Shows("Queue replaced", "5 songs you added were removed.", "Undo"))
	if err := r.tt.Click("Undo"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the queue brought back", func() bool {
		r.play.mu.Lock()
		defer r.play.mu.Unlock()
		return r.play.undone == 1
	})
}

func TestReplacingALongQueueAsksFirstWhenSettingsSayConfirm(t *testing.T) {
	r := playersRig(t)
	r.replaceMode = config.ReplaceConfirm
	seedQueue(r, 4)
	r.env.Players.PlaySongList(songs(2), 0, "Charts")
	r.tt.Until("the question", r.tt.Shows("Replace your queue?", "The 4 songs you added to the queue will be removed.", "Replace queue"))
	if got := r.play.plays(); len(got) != 0 {
		t.Fatalf("played %+v before the answer", got)
	}
	if err := r.tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	r.tt.Advance(time.Second) // the dialog closes
	r.tt.Drain()
	if got := r.play.plays(); len(got) != 0 {
		t.Errorf("played %+v after Cancel", got)
	}
	seedQueue(r, 4) // the mirror followed the coordinator's snapshots meanwhile
	r.env.Players.PlaySongList(songs(2), 0, "Charts")
	r.tt.Until("the question again", r.tt.Shows("Replace queue"))
	if err := r.tt.Click("Replace queue"); err != nil {
		t.Fatal(err)
	}
	r.waitPlays(1)
}

func TestAShortQueueIsReplacedWithoutAsking(t *testing.T) {
	r := playersRig(t)
	seedQueue(r, 3)
	r.env.Players.PlaySongList(songs(2), 0, "Charts")
	r.waitPlays(1)
	r.tt.Drain()
	if r.tt.HasText("Queue replaced") || kit.ConfirmPending() {
		t.Error("a queue of three songs the user added was asked about")
	}
}

func TestShuffleTurnsShuffleOnAndStartsFromARandomPlayableSong(t *testing.T) {
	r := playersRig(t)
	r.env.Queue.ShuffleSongs(songs(6, 0), services.PlayFrom{ID: "Top songs"})
	// Shuffle shows on at once, while the command is in flight.
	if !r.env.Play.State().Shuffle {
		t.Error("the mirror does not show shuffle on while the command is in flight")
	}
	got := r.waitPlays(1)[0]
	if len(got.refs) != 5 || got.start < 0 || got.start >= 5 || got.from != (services.PlayFrom{ID: "Top songs"}) {
		t.Errorf("played %+v", got)
	}
	r.play.mu.Lock()
	shuffle := r.play.shuffle
	r.play.mu.Unlock()
	if len(shuffle) != 1 || !shuffle[0] {
		t.Errorf("shuffle calls %v, want one turning it on", shuffle)
	}
}

func seedQueue(r *rig, added int) {
	q := services.QueueState{Index: 0, Items: make([]services.QueueItem, 1+added)}
	for i := 1; i <= added; i++ {
		q.Items[i].Added = true
	}
	r.env.Play.Seed(services.PlaybackState{}, q)
}
