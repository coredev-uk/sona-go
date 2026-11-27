package library

import (
	"context"
	"fmt"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/catalog"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/namefold"
)

// creditsGroup is how many songs or albums one round of creditPass asks
// about: four requests of applemusic.Client.Songs' or Albums' 100 IDs, stored
// together, so an interrupted pass keeps what it learnt.
const creditsGroup = 400

// creditRow is a library song or album the catalogue may be asked about: its
// row, its catalogue ID and Apple's artist string.
type creditRow struct {
	id        int64
	catalogID string
	artist    string
}

// creditKind is what creditPass needs to credit one kind of library item: the
// rows not asked about yet, the catalogue lookup (catalogue ID → credited
// artists, in Apple's order) and the writes of an item's credits.
type creditKind struct {
	name   string // "song" or "album", for messages
	rows   func(context.Context) ([]creditRow, error)
	fetch  func(ctx context.Context, sf string, ids []string) (map[string][]applemusic.Artist, error)
	clear  func(ctx context.Context, q *dbq.Queries, id int64) (int64, error)
	insert func(ctx context.Context, q *dbq.Queries, id int64, a applemusic.Artist, position int) error
	mark   func(ctx context.Context, q *dbq.Queries, id, now int64) error
}

func (s *Syncer) songCredits() creditKind {
	return creditKind{
		name: "song",
		rows: func(ctx context.Context) ([]creditRow, error) {
			rows, err := s.db.RQ.ListTracksToCredit(ctx)
			out := make([]creditRow, len(rows))
			for i, r := range rows {
				out[i] = creditRow{r.ID, r.CatalogID.String, r.ArtistName}
			}
			return out, err
		},
		fetch: func(ctx context.Context, sf string, ids []string) (map[string][]applemusic.Artist, error) {
			songs, err := s.api.Songs(ctx, sf, ids)
			out := make(map[string][]applemusic.Artist, len(songs))
			for _, so := range songs {
				out[so.ID] = so.Relationships.Artists.Data
			}
			return out, err
		},
		clear: func(ctx context.Context, q *dbq.Queries, id int64) (int64, error) {
			return q.DeleteTrackArtists(ctx, id)
		},
		insert: func(ctx context.Context, q *dbq.Queries, id int64, a applemusic.Artist, pos int) error {
			return q.InsertTrackArtist(ctx, dbq.InsertTrackArtistParams{
				TrackID: id, ArtistCatalogID: a.ID, Name: a.Attributes.Name, NameKey: namefold.Fold(a.Attributes.Name), Position: int64(pos),
			})
		},
		mark: func(ctx context.Context, q *dbq.Queries, id, now int64) error {
			return q.MarkTrackCredited(ctx, dbq.MarkTrackCreditedParams{CreditsChecked: now, ID: id})
		},
	}
}

func (s *Syncer) albumCredits() creditKind {
	return creditKind{
		name: "album",
		rows: func(ctx context.Context) ([]creditRow, error) {
			rows, err := s.db.RQ.ListAlbumsToCredit(ctx)
			out := make([]creditRow, len(rows))
			for i, r := range rows {
				out[i] = creditRow{r.ID, r.CatalogID.String, r.ArtistName}
			}
			return out, err
		},
		fetch: func(ctx context.Context, sf string, ids []string) (map[string][]applemusic.Artist, error) {
			albums, err := s.api.Albums(ctx, sf, ids)
			out := make(map[string][]applemusic.Artist, len(albums))
			for _, a := range albums {
				out[a.ID] = a.Relationships.Artists.Data
			}
			return out, err
		},
		clear: func(ctx context.Context, q *dbq.Queries, id int64) (int64, error) {
			return q.DeleteAlbumArtists(ctx, id)
		},
		insert: func(ctx context.Context, q *dbq.Queries, id int64, a applemusic.Artist, pos int) error {
			return q.InsertAlbumArtist(ctx, dbq.InsertAlbumArtistParams{
				AlbumID: id, ArtistCatalogID: a.ID, Name: a.Attributes.Name, NameKey: namefold.Fold(a.Attributes.Name), Position: int64(pos),
			})
		},
		mark: func(ctx context.Context, q *dbq.Queries, id, now int64) error {
			return q.MarkAlbumCredited(ctx, dbq.MarkAlbumCreditedParams{CreditsChecked: now, ID: id})
		},
	}
}

// syncCredits stores the credited artists of the library songs, and of the
// library albums, that a sync of their domain just covered (synced). A
// failure of one kind is logged and leaves it for the next sync: its items
// are listed under their library artist meanwhile. It reports whether
// credits changed.
func (s *Syncer) syncCredits(ctx context.Context, synced map[Domain]bool) (changed bool) {
	for _, k := range []struct {
		domain Domain
		kind   creditKind
	}{{DomainSongs, s.songCredits()}, {DomainAlbums, s.albumCredits()}} {
		if !synced[k.domain] || ctx.Err() != nil {
			continue
		}
		credited, err := s.creditPass(ctx, k.kind)
		if err != nil {
			s.log.Warn("library sync: credits", "kind", k.kind.name, "err", err)
		}
		changed = changed || credited
	}
	return changed
}

// creditPass stores the credited artists of the library items of kind k that
// were not asked about yet and whose artist string may name several
// (applemusic.MayNameSeveral): Apple's library song or album names its first
// artist only, its catalogue copy lists them all. One batched catalogue
// request covers 100 items, never one per item; an item is asked about once
// (an answer of one artist, of none (a compilation: Apple credits "Various
// Artists" to no artist), or of an item Apple does not serve in the
// storefront is kept as asked too), so after the first run only the items
// added since cost a request. Items with no catalogue ID (uploads) are never
// asked about. An item whose string names one artist has its credits cleared
// and is marked checked without a request. It reports whether credits
// changed. Without a storefront (Syncer.Storefront) nothing is asked.
func (s *Syncer) creditPass(ctx context.Context, k creditKind) (changed bool, err error) {
	rows, err := k.rows(ctx)
	if err != nil {
		return false, fmt.Errorf("library: %ss to credit: %w", k.name, err)
	}
	var ids []string
	var single []int64            // items whose artist string names one artist
	asked := map[string][]int64{} // catalogue ID → library items with it
	for _, r := range rows {
		if !applemusic.MayNameSeveral(r.artist) {
			single = append(single, r.id)
			continue
		}
		if _, ok := asked[r.catalogID]; !ok {
			ids = append(ids, r.catalogID)
		}
		asked[r.catalogID] = append(asked[r.catalogID], r.id)
	}
	// An item whose artist string no longer names several (or never did) has
	// no credits: any stored ones are stale. It is marked as checked, so the
	// next sync does not read it again. No request is needed.
	if len(single) > 0 {
		now := db.Millis(s.now())
		wctx := context.WithoutCancel(ctx)
		err := s.db.Tx(wctx, func(q *dbq.Queries) error {
			for _, item := range single {
				n, err := k.clear(wctx, q, item)
				if err != nil {
					return err
				}
				changed = changed || n > 0
				if err := k.mark(wctx, q, item, now); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return false, fmt.Errorf("library: clear %s credits: %w", k.name, err)
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
		credits, err := k.fetch(ctx, sf, group)
		if err != nil {
			return changed, fmt.Errorf("library: %s credits: %w", k.name, err)
		}
		now := db.Millis(s.now())
		// What the answer taught is stored even if the sync is cancelled now.
		wctx := context.WithoutCancel(ctx)
		err = s.db.Tx(wctx, func(q *dbq.Queries) error {
			// The answer names each credited artist with their picture (the
			// same include=artists response): kept as a catalogue artist row,
			// the picture of one the library holds only through credits.
			stored := map[string]bool{}
			for _, id := range group {
				for _, a := range credits[id] {
					if a.ID == "" || stored[a.ID] || a.Attributes.Artwork == nil {
						continue
					}
					stored[a.ID] = true
					if _, err := catalog.StoreArtist(wctx, q, a, now); err != nil {
						return err
					}
				}
			}
			for _, id := range group {
				for _, item := range asked[id] {
					if _, err := k.clear(wctx, q, item); err != nil {
						return err
					}
					for pos, a := range credits[id] {
						if a.ID == "" {
							continue
						}
						if err := k.insert(wctx, q, item, a, pos); err != nil {
							return err
						}
						changed = true
					}
					if err := k.mark(wctx, q, item, now); err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			return changed, fmt.Errorf("library: store %s credits: %w", k.name, err)
		}
	}
	return changed, nil
}
