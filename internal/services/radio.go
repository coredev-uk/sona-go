package services

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
)

// RadioService is the Radio page, station pages, Create Station and
// playing stations (track stations through the coordinator's station
// queue, live stations and episodes as one stream item).
type RadioService struct {
	e *Env
}

func newRadioService(e *Env) *RadioService {
	// Parked station batches come from the user's stations.
	e.OnSignOut("station previews", func(context.Context) error {
		e.previews.clear()
		return nil
	})
	return &RadioService{e: e}
}

func (s *RadioService) fail(err error) error { return s.e.Fail(err) }

// recentLimit is how many recently played stations the Radio page shows.
const recentLimit = 30

// Grouping returns the editorial Radio page (Apple's "radio" grouping):
// live stations with schedules, episodes, shows, genre stations.
func (s *RadioService) Grouping(ctx context.Context) (EditorialPage, error) {
	p, err := s.e.editorialGrouping(ctx, "radio")
	return p, s.fail(err)
}

// Live returns Apple Music's live radio stations.
func (s *RadioService) Live(ctx context.Context) ([]Station, error) {
	st, err := s.e.API.LiveStations(ctx, s.e.Storefront())
	if err != nil {
		return nil, s.fail(err)
	}
	return s.e.m.amStations(st), nil
}

// Recent returns the stations the user played recently (Apple's history,
// shared with the other Apple Music apps), newest first.
func (s *RadioService) Recent(ctx context.Context) ([]Station, error) {
	st, err := s.e.API.RecentStations(ctx, recentLimit)
	if err != nil {
		return nil, s.fail(err)
	}
	return s.e.m.amStations(st), nil
}

// Station returns a station page: a live station carries its schedule; a
// track station what it was made from and the songs it plays first
// (parked for the station feed, so they are the ones that play).
func (s *RadioService) Station(ctx context.Context, id string) (StationDetail, error) {
	if !isStationID(id) {
		return StationDetail{}, fmt.Errorf("%w: station %q", ErrInvalidArgument, id)
	}
	st, err := s.e.API.StationDetail(ctx, s.e.Storefront(), id)
	if err != nil {
		return StationDetail{}, s.fail(err)
	}
	out := StationDetail{
		Station:  s.e.m.amStation(st),
		Schedule: s.schedule(st, s.e.Now()),
		Upcoming: []Song{},
	}
	var wg sync.WaitGroup
	wg.Go(func() { out.Header = s.e.m.header(ctx, st.Attributes.Artwork.Template()) })
	if out.Station.Kind == StationTracks {
		wg.Go(func() { out.Seed = s.seed(ctx, st) })
		// next-tracks needs the user's token.
		if out.Station.Playable && s.e.Auth.UserToken() != "" {
			wg.Go(func() { out.Upcoming = s.upcoming(ctx, st.ID) })
		}
	}
	wg.Wait()
	out.Header.Motion = s.e.m.motionOf(st.Attributes.EditorialVideo)
	return out, nil
}

// upcoming returns the songs track station id plays first, as the station
// feed will queue them (songs without play parameters dropped); none when
// Apple does not answer, which does not fail the page.
func (s *RadioService) upcoming(ctx context.Context, id string) []Song {
	songs, err := s.e.stationPreview(ctx, id, stationBatch)
	if err != nil {
		s.e.Report(err)
		s.e.Logger.Warn("radio: station preview", "station", id, "err", err)
		return []Song{}
	}
	out := make([]Song, 0, len(songs))
	for _, so := range songs {
		if so.Attributes.PlayParams != nil && so.CatalogID() != "" {
			out = append(out, s.e.m.amSong(so))
		}
	}
	return out
}

// stationSeed returns the kind (SeedSong, SeedAlbum, SeedArtist) and
// catalog ID of the item a track station was made from, from its kind and
// ID ("ra.<id>", "ra.cp-<songId>" for AutoPlay, "ra.a-<artistId>"); ""
// for stations without one (personal, mood, genre, editorial).
func stationSeed(st applemusic.Station) (kind, id string) {
	id = strings.TrimPrefix(st.ID, "ra.")
	switch st.Attributes.Kind {
	case applemusic.StationKindSongSeeded:
		kind, id = SeedSong, strings.TrimPrefix(id, "cp-")
	case applemusic.StationKindAlbumSeeded:
		kind = SeedAlbum
	case applemusic.StationKindArtistSeeded:
		kind = SeedArtist
	case applemusic.StationKindArtistOnly:
		kind, id = SeedArtist, strings.TrimPrefix(id, "a-")
	default:
		return "", ""
	}
	if id == "" || strings.ContainsFunc(id, func(r rune) bool { return r < '0' || r > '9' }) {
		return "", ""
	}
	return kind, id
}

// seed resolves a track station's seed through the catalog cache (the
// seed's own page reuses it); nil when it has none or Apple did not
// answer.
func (s *RadioService) seed(ctx context.Context, st applemusic.Station) *StationSeed {
	kind, id := stationSeed(st)
	var (
		out *StationSeed
		err error
	)
	switch kind {
	case SeedSong:
		var so applemusic.Song
		if so, err = s.e.Catalog.Song(ctx, id); err == nil {
			d := s.e.m.amSong(so)
			out = &StationSeed{Kind: kind, ID: id, Name: d.Name, Subtitle: d.Artist, ArtistID: d.ArtistID, Art: d.Art}
		}
	case SeedAlbum:
		var a applemusic.Album
		if a, err = s.e.Catalog.Album(ctx, id); err == nil {
			d := s.e.m.amAlbum(a)
			out = &StationSeed{Kind: kind, ID: id, Name: d.Name, Subtitle: d.Artist, ArtistID: d.ArtistID, Art: d.Art}
		}
	case SeedArtist:
		var a applemusic.Artist
		if a, err = s.e.Catalog.Artist(ctx, id); err == nil {
			d := s.e.m.amArtist(a)
			out = &StationSeed{
				Kind: kind, ID: id, Name: d.Name, Art: d.Art,
				Only: st.Attributes.Kind == applemusic.StationKindArtistOnly,
			}
		}
	}
	if err != nil {
		s.e.Report(err)
		s.e.Logger.Warn("radio: station seed", "station", st.ID, "err", err)
	}
	return out
}

// schedule maps a live station's events that have not ended, in start
// order. A programme's art is the centre square of its 4:1 key art, as
// Apple's apps show it in the schedule: the whole banner fitted into the
// square tile would be a strip a quarter of its height.
func (s *RadioService) schedule(st applemusic.Station, now time.Time) []StationEvent {
	out := []StationEvent{}
	for _, ev := range st.Relationships.Events.Data {
		a := ev.Attributes
		e := StationEvent{
			ID: ev.ID, Title: a.Title, Description: a.Description.Text(),
			StartMs: parseMs(a.StartTime), EndMs: parseMs(a.EndTime),
			Art: s.e.m.centreSquare(a.Artwork),
		}
		if e.EndMs > 0 && e.EndMs <= now.UnixMilli() {
			continue
		}
		out = append(out, e)
	}
	slices.SortStableFunc(out, func(a, b StationEvent) int { return int(a.StartMs - b.StartMs) })
	return out
}

// Play plays station id: a track station becomes the queue's station
// (refilled as it plays); a live station or episode plays as one stream
// item.
func (s *RadioService) Play(ctx context.Context, id string) error {
	st, err := s.resolve(ctx, id)
	if err != nil {
		return err
	}
	return s.play(ctx, st)
}

// CreateStation starts the station seeded by a catalog song, artist or
// album (seed: SeedSong, SeedArtist, SeedAlbum) and returns it.
func (s *RadioService) CreateStation(ctx context.Context, seed, catalogID string) (Station, error) {
	switch seed {
	case SeedSong, SeedArtist, SeedAlbum:
	default:
		return Station{}, fmt.Errorf("%w: seed %q", ErrInvalidArgument, seed)
	}
	if catalogID == "" || isLibraryID(catalogID) || strings.Contains(catalogID, ".") {
		return Station{}, fmt.Errorf("%w: %s %q has no catalog station", ErrInvalidArgument, seed, catalogID)
	}
	st, err := s.resolve(ctx, "ra."+catalogID)
	if err != nil {
		return Station{}, err
	}
	if err := s.play(ctx, st); err != nil {
		return Station{}, err
	}
	return s.e.m.amStation(st), nil
}

// resolve fetches station id (ids= lookup, as the web's Create Station).
func (s *RadioService) resolve(ctx context.Context, id string) (applemusic.Station, error) {
	if !isStationID(id) {
		return applemusic.Station{}, fmt.Errorf("%w: station %q", ErrInvalidArgument, id)
	}
	sts, err := s.e.API.Stations(ctx, s.e.Storefront(), []string{id})
	if err != nil {
		return applemusic.Station{}, s.fail(err)
	}
	if len(sts) == 0 {
		return applemusic.Station{}, fmt.Errorf("station %s: %w", id, applemusic.ErrNotFound)
	}
	return sts[0], nil
}

func (s *RadioService) play(ctx context.Context, st applemusic.Station) error {
	if ok, why := stationPlayability(st); !ok {
		return fmt.Errorf("%s: %s: %w", st.Attributes.Name, why, applemusic.ErrNotPlayable)
	}
	s.e.Logger.Info("playback: command", "cmd", "play-station", "station", st.ID, "from", "ui")
	a := st.Attributes
	if st.Format() == applemusic.StationFormatTracks {
		return s.fail(s.e.Coord.PlayStation(ctx, coordinator.Station{ID: st.ID, Name: a.Name}))
	}
	item := coordinator.Item{
		ID:       st.ID,
		Title:    a.Name,
		Artist:   stationSubtitle(st),
		Duration: time.Duration(a.DurationInMillis) * time.Millisecond,
		Artwork:  a.Artwork.Template(),
	}
	return s.fail(s.e.Coord.SetQueue(ctx, []coordinator.Item{item}, 0, st.ID))
}

// stationSubtitle is the line under a stream item's name: the show for an
// episode, else the provider.
func stationSubtitle(st applemusic.Station) string {
	if d := st.Relationships.RadioShow.Data; len(d) > 0 && d[0].Attributes.Name != "" {
		return d[0].Attributes.Name
	}
	if st.Attributes.StationProviderName != "" {
		return st.Attributes.StationProviderName
	}
	return "Apple Music Radio"
}

// isStationID reports whether id is a station or radio episode ("ra.…").
func isStationID(id string) bool { return strings.HasPrefix(id, "ra.") }

// stationKind classifies a station for the UI (Station.Kind).
func stationKind(st applemusic.Station) string {
	if st.Format() != applemusic.StationFormatStream {
		return StationTracks
	}
	p := st.Attributes.PlayParams
	switch {
	case p != nil && p.StreamingKind == 2, p != nil && !p.HasDRM:
		return StationBroadcast
	case st.Attributes.IsLive:
		return StationLive
	default:
		return StationEpisode
	}
}

// stationPlayability says whether Sona can play st, and why not. Track
// stations play song by song; Apple's live stations and episodes stream
// through the Widevine engine; third-party broadcasts (TuneIn Shoutcast
// streams outside Apple's encrypted radio) are not supported.
func stationPlayability(st applemusic.Station) (bool, string) {
	switch stationKind(st) {
	case StationBroadcast:
		return false, "This broadcaster streams through TuneIn outside Apple Music, which Sona can't play."
	case StationLive, StationEpisode:
		if d := st.Attributes.SupportedDRMs; len(d) > 0 && !slices.Contains(d, "widevine") {
			return false, "This stream isn't offered with Widevine, the only protection Sona can play."
		}
	}
	return true, ""
}
