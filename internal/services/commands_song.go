package services

import (
	"context"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/library"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
)

// The playing song's state on the command line: like, dislike and library
// act on it, and the status carries it (Liked, Disliked, InLibrary).

// songStateTimeout bounds the status's lookup of the playing song's state.
const songStateTimeout = 30 * time.Second

// songState is the library state of the coordinator item id ("" when
// nothing plays), as the status shows it.
type songState struct {
	id string
	st ItemState
}

// songChanged reports whether snap plays another song than the status's
// state is for, and one not already being looked up.
func (s *commandServer) songChanged(snap coordinator.Snapshot) bool {
	id := ""
	if snap.Current != nil {
		id = snap.Current.Item.ID
	}
	s.songMu.Lock()
	defer s.songMu.Unlock()
	if id == s.song.id || id == s.songWant {
		return false
	}
	s.songWant = id
	return true
}

// lookUpSongSoon asks for the playing song's state to be looked up again:
// it started, or the user changed a rating or the library.
func (s *commandServer) lookUpSongSoon() {
	select {
	case s.kick <- struct{}{}:
	default: // one is already asked for
	}
}

// lookUpSong reads the playing song's state as the player bar does
// (LibraryService.States: cached, else one Apple lookup, else the synced
// library) and publishes the status when it changed. A failure leaves the
// song's state unknown: false in the status.
func (s *commandServer) lookUpSong() {
	var next songState
	if cur := s.e.Coord.Snapshot().Current; cur != nil {
		next.id = cur.Item.ID
		if ref, ok := s.songRef(cur); ok {
			ctx, cancel := context.WithTimeout(context.Background(), songStateTimeout)
			sts, err := s.lib.States(ctx, []ItemRef{ref})
			cancel()
			if err != nil {
				s.e.Logger.Debug("commands: playing song's state", "id", ref.ID, "err", err)
			} else {
				next.st = sts[0]
			}
		}
	}
	s.songMu.Lock()
	changed := next != s.song
	s.song = next
	s.songMu.Unlock()
	if changed {
		s.publish()
	}
}

// songRef is cur as the library service names a song; false for what has
// no rating or library state of its own (live radio, a broadcaster, a
// station's stream, a music video).
func (s *commandServer) songRef(cur *coordinator.Entry) (ItemRef, bool) {
	qi := s.play.item(cur.UID, cur.Item)
	if qi.Live || qi.Broadcast || qi.Video || applemusic.IsStationID(qi.ID) {
		return ItemRef{}, false
	}
	return ItemRef{Kind: applemusic.TypeSongs, ID: qi.ID, Source: qi.Source, CatalogID: qi.CatalogID, LibraryID: qi.LibraryID}, true
}

// playingSong is the song like, dislike and library act on: the playing
// one, with an account signed in. Ratings and the library are the
// account's: nothing goes to Apple without one, and while Sona is still
// reading the stored token (a slow keyring) it says so.
func (s *commandServer) playingSong() (cli.Track, ItemRef, error) {
	switch s.e.Auth.Status().State {
	case auth.StateSignedIn:
	case auth.StateRestoring, auth.StateValidating:
		return cli.Track{}, ItemRef{}, &cli.Error{Code: cli.CodeAccountLoading}
	default:
		return cli.Track{}, ItemRef{}, &cli.Error{Code: string(CodeNotSignedIn)}
	}
	cur := s.e.Coord.Snapshot().Current
	if cur == nil {
		return cli.Track{}, ItemRef{}, &cli.Error{Code: cli.CodeNothingPlaying}
	}
	ref, ok := s.songRef(cur)
	if !ok {
		return cli.Track{}, ItemRef{}, &cli.Error{Code: string(CodeStation)}
	}
	return track(cur.Item), ref, nil
}

// songAnswer is the answer of like, dislike and library.
func songAnswer(t cli.Track, st ItemState) cli.Like {
	return cli.Like{
		Track: t, Liked: st.Rating == applemusic.RatingFavorite,
		Disliked: st.Rating == applemusic.RatingSuggestLess, InLibrary: st.InLibrary,
	}
}

// changedSong tells the UI of the user's change to an item (EventItemState),
// as the player bar's own buttons do.
func (s *commandServer) changedSong(st ItemState) {
	if s.e.Emit != nil {
		s.e.Emit.Emit(EventItemState, st)
	}
}

// rate answers like (value RatingFavorite) and dislike (RatingSuggestLess):
// it sets the playing song's rating to value (on, the default), takes it
// back (off), switches it (toggle) or reports it (status), through the
// library service the player bar's Favourite and Suggest Less use.
func (s *commandServer) rate(ctx context.Context, args []string, value int) (any, error) {
	verb := "on"
	if len(args) > 0 {
		verb = args[0]
	}
	t, ref, err := s.playingSong()
	if err != nil {
		return nil, err
	}
	var on bool
	switch verb {
	case "on", "off":
		on = verb == "on"
	case "toggle", "status":
		sts, err := s.lib.States(ctx, []ItemRef{ref})
		if err != nil {
			return nil, s.fail(err)
		}
		if verb == "status" {
			return songAnswer(t, sts[0]), nil
		}
		on = sts[0].Rating != value
	default:
		return nil, &cli.Error{Code: cli.CodeUsage}
	}
	set := s.lib.Favorite
	if value == applemusic.RatingSuggestLess {
		set = s.lib.SuggestLess
	}
	st, err := set(ctx, ref, on)
	if err != nil {
		return nil, s.fail(err)
	}
	s.changedSong(st)
	return songAnswer(t, st), nil
}

// library answers `library`: the playing song's place in the library
// (status, the default), added (add), removed (remove) or switched
// (toggle) as the song menu's Add to Library and Delete from Library do;
// or the library's playlists or albums.
func (s *commandServer) library(ctx context.Context, args []string) (any, error) {
	verb := "status"
	if len(args) > 0 {
		verb = args[0]
	}
	if verb == "playlists" || verb == "albums" {
		return s.libraryItems(ctx, verb)
	}
	t, ref, err := s.playingSong()
	if err != nil {
		return nil, err
	}
	sts, err := s.lib.States(ctx, []ItemRef{ref})
	if err != nil {
		return nil, s.fail(err)
	}
	st := sts[0]
	add := !st.InLibrary
	switch verb {
	case "status":
		return songAnswer(t, st), nil
	case "add":
		if st.InLibrary {
			return songAnswer(t, st), nil
		}
	case "remove":
		if !st.InLibrary {
			return songAnswer(t, st), nil
		}
		add = false
	case "toggle":
	default:
		return nil, &cli.Error{Code: cli.CodeUsage}
	}
	if add {
		cat, _ := refIDs(ref)
		if cat == "" {
			return nil, s.fail(applemusic.ErrNotFound)
		}
		if err := s.lib.Add(ctx, applemusic.TypeSongs, []string{cat}); err != nil {
			return nil, s.fail(err)
		}
		if sts, err = s.lib.States(ctx, []ItemRef{ref}); err != nil {
			return nil, s.fail(err)
		}
		st = sts[0]
	} else if st, err = s.lib.Remove(ctx, ref); err != nil {
		return nil, s.fail(err)
	}
	s.changedSong(st)
	return songAnswer(t, st), nil
}

// libraryItems lists the library's playlists or albums (verb), most
// recently added first, with the links `play` takes, from the synced
// library.
func (s *commandServer) libraryItems(ctx context.Context, verb string) (any, error) {
	out := cli.LibraryItems{Items: []cli.LibraryItem{}}
	o := library.ListOptions{Sort: library.SortDateAdded, Desc: true, Limit: cli.LibraryItemsMax}
	if verb == "playlists" {
		pls, err := s.e.Library.Playlists(ctx, o)
		if err != nil {
			return nil, s.fail(err)
		}
		c, err := s.e.Library.Counts(ctx)
		if err != nil {
			return nil, s.fail(err)
		}
		out.Kind, out.Total = "playlist", c.Playlists
		for _, p := range pls {
			out.Items = append(out.Items, cli.LibraryItem{
				ID: p.LibraryID, Name: p.Name, Artwork: publicArtURL(p.ArtworkTemplate, commandArtSize),
				URL: "https://music.apple.com/library/playlist/" + p.LibraryID,
			})
		}
		return out, nil
	}
	albums, err := s.e.Library.Albums(ctx, o)
	if err != nil {
		return nil, s.fail(err)
	}
	if out.Total, err = s.e.Library.AlbumCount(ctx, ""); err != nil {
		return nil, s.fail(err)
	}
	out.Kind = "album"
	for _, a := range albums {
		out.Items = append(out.Items, cli.LibraryItem{
			ID: a.LibraryID, Name: a.Name, Subtitle: a.ArtistName, Artwork: publicArtURL(a.ArtworkTemplate, commandArtSize),
			URL: "https://music.apple.com/library/albums/" + a.LibraryID,
		})
	}
	return out, nil
}
