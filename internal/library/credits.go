package library

import (
	"context"
	"fmt"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

// creditsGroup is how many songs one round of syncCredits asks about: four
// requests of applemusic.Client.Songs' 100 IDs, stored together, so an
// interrupted pass keeps what it learnt.
const creditsGroup = 400

// syncCredits stores the credited artists of the library songs that were not
// asked about yet and whose artist string may name several
// (applemusic.MayNameSeveral): Apple's library song names its first artist
// only, its catalogue copy lists them all. One batched catalogue request
// covers 100 songs, never one per song; a song is asked about once (an
// answer of one artist, of none, or of a song Apple does not serve in the
// storefront is kept as asked too), so after the first run only the songs
// added since cost a request. Songs with no catalogue ID (uploads) are never
// asked about. A song whose string names one artist has its credits cleared and
// is marked checked without a request. It reports whether credits changed.
// Without a storefront (Syncer.Storefront) nothing is asked.
func (s *Syncer) syncCredits(ctx context.Context) (changed bool, err error) {
	rows, err := s.db.RQ.ListTracksToCredit(ctx)
	if err != nil {
		return false, fmt.Errorf("library: songs to credit: %w", err)
	}
	var ids []string
	var single []int64            // songs whose artist string names one artist
	asked := map[string][]int64{} // catalogue ID → library songs with it
	for _, r := range rows {
		if !applemusic.MayNameSeveral(r.ArtistName) {
			single = append(single, r.ID)
			continue
		}
		if _, ok := asked[r.CatalogID.String]; !ok {
			ids = append(ids, r.CatalogID.String)
		}
		asked[r.CatalogID.String] = append(asked[r.CatalogID.String], r.ID)
	}
	// A song whose artist string no longer names several (or never did) has
	// no credits: any stored ones are stale. It is marked as checked, so the
	// next sync does not read it again. No request is needed.
	if len(single) > 0 {
		now := db.Millis(s.now())
		err := s.db.Tx(context.WithoutCancel(ctx), func(q *dbq.Queries) error {
			for _, track := range single {
				n, err := q.DeleteTrackArtists(context.WithoutCancel(ctx), track)
				if err != nil {
					return err
				}
				changed = changed || n > 0
				if err := q.MarkTrackCredited(context.WithoutCancel(ctx), dbq.MarkTrackCreditedParams{CreditsChecked: now, ID: track}); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return false, fmt.Errorf("library: clear song credits: %w", err)
		}
	}
	sf := ""
	if s.Storefront != nil {
		sf = s.Storefront()
	}
	if sf == "" {
		return changed, nil
	}
	for start := 0; start < len(ids); start += creditsGroup {
		group := ids[start:min(start+creditsGroup, len(ids))]
		songs, err := s.api.Songs(ctx, sf, group)
		if err != nil {
			return changed, fmt.Errorf("library: song credits: %w", err)
		}
		credits := make(map[string][]applemusic.Artist, len(songs))
		for _, so := range songs {
			credits[so.ID] = so.Relationships.Artists.Data
		}
		now := db.Millis(s.now())
		// What the answer taught is stored even if the sync is cancelled now.
		wctx := context.WithoutCancel(ctx)
		err = s.db.Tx(wctx, func(q *dbq.Queries) error {
			for _, id := range group {
				for _, track := range asked[id] {
					if _, err := q.DeleteTrackArtists(wctx, track); err != nil {
						return err
					}
					for pos, a := range credits[id] {
						if a.ID == "" {
							continue
						}
						if err := q.InsertTrackArtist(wctx, dbq.InsertTrackArtistParams{
							TrackID: track, ArtistCatalogID: a.ID, Name: a.Attributes.Name, Position: int64(pos),
						}); err != nil {
							return err
						}
						changed = true
					}
					if err := q.MarkTrackCredited(wctx, dbq.MarkTrackCreditedParams{CreditsChecked: now, ID: track}); err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			return changed, fmt.Errorf("library: store song credits: %w", err)
		}
	}
	return changed, nil
}
