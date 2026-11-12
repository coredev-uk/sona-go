package services

import (
	"context"
	"fmt"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
)

// Station and AutoPlay entry points of the playback service: the
// coordinator's StationSource (next-tracks batches and the AutoPlay
// continuation) and the AutoPlay setting.

// stationBatch is how many tracks one next-tracks call asks for (the web
// player's size).
const stationBatch = 10

// stationFeed is the coordinator's StationSource. Its songs' album and
// artist IDs go into the service's metadata so queue rows link like any
// other.
type stationFeed struct{ s *PlaybackService }

// NextTracks fetches the next batch of a station; songs without play
// parameters are dropped.
func (f stationFeed) NextTracks(ctx context.Context, stationID string) ([]coordinator.Item, error) {
	songs, err := f.s.o.nextStationTracks(ctx, stationID, stationBatch)
	if err != nil {
		f.s.o.Report(err)
		return nil, err
	}
	items := make([]coordinator.Item, 0, len(songs))
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	for _, so := range songs {
		if so.Attributes.PlayParams == nil || so.CatalogID() == "" {
			continue
		}
		it := songItem(so)
		dto := f.s.m.amSong(so)
		if _, known := f.s.meta[it.ID]; !known {
			f.s.meta[it.ID] = trackMeta{source: SourceCatalog, id: it.ID, catalogID: it.ID, albumID: dto.AlbumID, artistID: dto.ArtistID}
		}
		items = append(items, it)
	}
	return items, nil
}

// Continue creates the AutoPlay station after seeds, each with the album
// or playlist it was queued from as its container.
func (f stationFeed) Continue(ctx context.Context, seeds []coordinator.Item) (coordinator.Station, error) {
	in := make([]applemusic.StationSeed, len(seeds))
	for i, it := range seeds {
		in[i] = applemusic.StationSeed{ID: it.ID, Library: applemusic.IsLibrarySongID(it.ID)}
		if k := it.Container.Kind; k == coordinator.ContainerAlbum || k == coordinator.ContainerPlaylist {
			in[i].Container = it.Container.ID
		}
	}
	st, err := f.s.o.API.ContinuousStation(ctx, in)
	if err != nil {
		f.s.o.Report(err)
		return coordinator.Station{}, fmt.Errorf("autoplay: %w", err)
	}
	return coordinator.Station{ID: st.ID, Name: st.Attributes.Name}, nil
}

// SetAutoPlay turns AutoPlay on or off; it is remembered in the settings.
func (s *PlaybackService) SetAutoPlay(on bool) error {
	if _, err := s.o.Settings.Update(func(st *config.Settings) { st.AutoPlay = on }); err != nil {
		return err
	}
	s.o.Coord.SetAutoPlay(on)
	return nil
}

// queueStation maps the coordinator's station for the UI.
func queueStation(st *coordinator.Station) *QueueStation {
	if st == nil {
		return nil
	}
	return &QueueStation{ID: st.ID, Name: st.Name, AutoPlay: st.AutoPlay}
}
