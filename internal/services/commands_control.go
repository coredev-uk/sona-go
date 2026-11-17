package services

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
)

// The command line's playback controls beyond the transport keys: seek,
// shuffle, repeat, crossfade, the queue's verbs and `play <link>`. Each does
// what the player bar's or the queue's own control does.

// seek moves playback to a position in the playing song, or by a number of
// seconds from where it is, within the song.
func (s *commandServer) seek(ctx context.Context, arg string) (any, error) {
	secs, relative, err := cli.ParseSeek(arg)
	if err != nil {
		return nil, err
	}
	snap := s.e.Coord.Snapshot()
	if snap.Current == nil {
		return nil, &cli.Error{Code: cli.CodeNothingPlaying}
	}
	pos := time.Duration(secs * float64(time.Second))
	if relative {
		pos += position(snap, time.Now())
	}
	pos = max(pos, 0)
	if d := cmp.Or(snap.Duration, snap.Current.Item.Duration); d > 0 {
		pos = min(pos, d)
	}
	return s.transport(ctx, "seek", func(ctx context.Context) error { return s.e.Coord.Seek(ctx, pos) })
}

// switchVerb reads a switch's verb (on, off, toggle, status; status when
// none) against its current value: the value to set, and whether to set it.
func switchVerb(args []string, cur bool) (on, set bool, err error) {
	verb := "status"
	if len(args) > 0 {
		verb = args[0]
	}
	switch verb {
	case "status":
		return cur, false, nil
	case "on", "off":
		return verb == "on", true, nil
	case "toggle":
		return !cur, true, nil
	}
	return false, false, &cli.Error{Code: cli.CodeUsage}
}

// shuffle shows or switches shuffle, as the player bar's button.
func (s *commandServer) shuffle(args []string) (any, error) {
	on, set, err := switchVerb(args, s.e.Coord.Snapshot().Shuffle)
	if err != nil {
		return nil, err
	}
	if set {
		s.e.Logger.Info("playback: command", "cmd", "shuffle", "on", on, "from", "cli")
		s.play.SetShuffle(on)
	}
	return s.status(s.e.Coord.Snapshot()), nil
}

// nextRepeat is the player bar's repeat cycle: off → all → one → off.
func nextRepeat(mode coordinator.Repeat) coordinator.Repeat {
	switch mode {
	case coordinator.RepeatOff, "":
		return coordinator.RepeatAll
	case coordinator.RepeatAll:
		return coordinator.RepeatOne
	}
	return coordinator.RepeatOff
}

// repeat shows or sets repeat; cycle moves on as the player bar's button.
func (s *commandServer) repeat(args []string) (any, error) {
	verb := "status"
	if len(args) > 0 {
		verb = args[0]
	}
	mode := coordinator.Repeat(verb)
	switch verb {
	case "status":
		return s.status(s.e.Coord.Snapshot()), nil
	case "cycle":
		mode = nextRepeat(s.e.Coord.Snapshot().Repeat)
	}
	s.e.Logger.Info("playback: command", "cmd", "repeat", "mode", mode, "from", "cli")
	if err := s.play.SetRepeat(string(mode)); err != nil {
		return nil, s.fail(err)
	}
	return s.status(s.e.Coord.Snapshot()), nil
}

// crossfade shows or switches crossfade, saved as Settings → Playback's
// switch saves it; its length stays.
func (s *commandServer) crossfade(args []string) (any, error) {
	on, set, err := switchVerb(args, s.crossfadeOn.Load())
	if err != nil {
		return nil, err
	}
	if set {
		if !s.e.Engine.Capabilities().Crossfade {
			return nil, s.fail(unsupported("crossfade"))
		}
		if _, err := s.e.updateSettings(func(st *config.Settings) { st.Crossfade.Enabled = on }); err != nil {
			return nil, s.fail(err)
		}
	}
	return s.status(s.e.Coord.Snapshot()), nil
}

// queueCommand lists the queue, plays the song at a place in it (jump, as
// the queue's rows do) or clears it (clear, as the queue's Clear: the
// playing song stays).
func (s *commandServer) queueCommand(ctx context.Context, args []string) (any, error) {
	if len(args) == 0 {
		return s.queue(s.e.Coord.Snapshot()), nil
	}
	if args[0] == "clear" {
		s.e.Logger.Info("playback: command", "cmd", "clear", "from", "cli")
		s.play.Clear()
		return s.queue(s.e.Coord.Snapshot()), nil
	}
	i, err := cli.ParseQueuePlace(args[1])
	if err != nil {
		return nil, err
	}
	return s.jump(ctx, i)
}

// jump plays the song at index i of the queue.
func (s *commandServer) jump(ctx context.Context, i int) (any, error) {
	if i >= len(s.e.Coord.Snapshot().Queue) {
		return nil, &cli.Error{Code: cli.CodeUsage, Message: fmt.Sprintf("queue: there is no song at place %d", i+1)}
	}
	return s.transport(ctx, "jump", func(ctx context.Context) error { return s.e.Coord.Jump(ctx, i) })
}

// errNoPlayer is `play <link>`'s failure in a server without the services
// that load collections (a test's).
var errNoPlayer = errors.New("services: no service loads collections")

// playLink plays an Apple Music link in place of the queue, as its page's
// Play (or Shuffle) does: a song alone, an album's or a playlist's songs
// from the first (shuffled: shuffle on, from a random one), a station.
func (s *commandServer) playLink(ctx context.Context, link string, shuffle bool) (any, error) {
	t, err := app.ParseURL(link)
	if err != nil {
		return nil, &cli.Error{Code: cli.CodeBadLink, Message: err.Error()}
	}
	if s.cat == nil || s.pls == nil || s.radio == nil {
		return nil, s.fail(errNoPlayer)
	}
	s.e.Logger.Info("playback: command", "cmd", "play-link", "kind", t.Kind, "from", "cli")
	var (
		songs []Song
		from  PlayFrom
	)
	switch t.Kind {
	case app.KindSong:
		so, err := s.cat.Song(ctx, t.ID)
		if err != nil {
			return nil, s.fail(err)
		}
		songs, from = []Song{so}, PlayFrom{ID: so.Name}
	case app.KindAlbum:
		a, err := s.cat.Album(ctx, t.ID)
		if err != nil {
			return nil, s.fail(err)
		}
		songs, from = a.Tracks, PlayFrom{Kind: ItemAlbum, ID: t.ID}
	case app.KindLibraryAlbum:
		a, err := s.lib.Album(ctx, t.ID)
		if err != nil {
			return nil, s.fail(err)
		}
		songs, from = a.Tracks, PlayFrom{Kind: ItemAlbum, ID: t.ID}
	case app.KindPlaylist, app.KindLibraryPlaylist:
		p, err := s.pls.Playlist(ctx, t.ID)
		if err != nil {
			return nil, s.fail(err)
		}
		songs, from = p.Tracks, PlayFrom{Kind: ItemPlaylist, ID: t.ID}
	case app.KindStation:
		if err := s.radio.Play(ctx, t.ID); err != nil {
			return nil, s.fail(err)
		}
		return s.status(s.e.Coord.Snapshot()), nil
	default:
		return nil, &cli.Error{Code: cli.CodeUsage, Message: fmt.Sprintf("play: Sona can't play a %s link; `sona open` opens it", t.Kind)}
	}
	var refs []TrackRef
	for _, so := range songs {
		if so.Playable {
			refs = append(refs, TrackRef{ID: so.ID, Source: so.Source})
		}
	}
	if len(refs) == 0 {
		return nil, s.fail(applemusic.ErrNotPlayable)
	}
	start := 0
	if shuffle {
		s.play.SetShuffle(true)
		start = rand.IntN(len(refs))
	}
	if err := s.play.PlayItems(ctx, refs, start, from); err != nil {
		return nil, s.fail(err)
	}
	return s.status(s.e.Coord.Snapshot()), nil
}
