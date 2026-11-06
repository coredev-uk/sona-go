package fixture

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"

	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/library"
)

// day is a day in the milliseconds library_items.date_added counts.
const day = 86_400_000

// CDNHost is Apple's artwork CDN host, the one the fixture's covers are on.
const CDNHost = "is1-ssl.mzstatic.com"

// Template is the artwork template of album i's cover: Apple's artwork CDN
// host (the artwork cache takes no other), a path of its own, served by CDN.
func Template(i int) string {
	return fmt.Sprintf("https://"+CDNHost+"/image/thumb/Fixture/cover%d/{w}x{h}bb.png", i)
}

// PlaylistTemplate is the artwork template of playlist n's cover.
func PlaylistTemplate(n int) string {
	return fmt.Sprintf("https://"+CDNHost+"/image/thumb/Fixture/playlist%d/{w}x{h}bb.png", n)
}

// GeneratedTemplate is the template of the cover Apple generates for a
// playlist called name, in its plain gradient design; CDN serves it in each
// of Apple's designs (artwork.Patterns).
func GeneratedTemplate(name string) string {
	return "https://" + CDNHost + "/image/thumb/gen/{w}x{h}AM.PDCXS11.jpg?c1=F2CC9D&c2=FFA940&c3=B36B00&c4=400000&signature=f1&t=" +
		base64.URLEncoding.EncodeToString([]byte(name)) + "&tc=000000&vkey=1"
}

// ReleaseDate is album i's release date: always in the past, so every song
// plays (a song not out yet does not).
func ReleaseDate(i int) string { return fmt.Sprintf("%d-%02d-01", 2008+i%17, 1+i%12) }

// Seed fills a migrated library with albums, their songs, their artists,
// the playlists of Playlists (when playlists is true), the sync state that
// says the library is synced, and the search index. Album i was added i+1
// days after the epoch, so "Recently added" lists the last one first.
func Seed(ctx context.Context, d *db.DB, albums []Album, playlists []Playlist) error {
	const now = 1_700_000_000_000
	err := d.Tx(ctx, func(q *dbq.Queries) error {
		artistIDs := map[string]string{}
		for n, name := range Artists(albums) {
			id := ArtistLibraryID(n)
			artistIDs[name] = id
			ref, err := q.UpsertLibraryArtist(ctx, dbq.UpsertLibraryArtistParams{
				LibraryID: db.NullString(id), CatalogID: db.NullString(fmt.Sprint(500 + n)), Name: name, UpdatedAt: now,
			})
			if err != nil {
				return err
			}
			if err := q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: "artist", Ref: ref, Generation: 1}); err != nil {
				return err
			}
		}
		trackRefs := map[[2]int]int64{}
		rank := int64(0)
		for i, a := range albums {
			rating := ""
			if a.Explicit {
				rating = "explicit"
			}
			ref, err := q.UpsertLibraryAlbum(ctx, dbq.UpsertLibraryAlbumParams{
				LibraryID: db.NullString(AlbumLibraryID(i)), CatalogID: db.NullString(AlbumCatalogID(i)),
				Name: a.Name, ArtistName: a.Artist, ArtistLibraryID: db.NullString(artistIDs[a.Artist]),
				ArtworkTemplate: Template(i), ReleaseDate: ReleaseDate(i), Genre: "Pop",
				TrackCount: int64(a.SongCount()), ContentRating: rating, IsSingle: db.Bool(a.Single), UpdatedAt: now,
			})
			if err != nil {
				return err
			}
			if err := q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{
				Kind: "album", Ref: ref, DateAdded: sql.NullInt64{Int64: int64(i+1) * day, Valid: true}, Generation: 1,
			}); err != nil {
				return err
			}
			for j := range a.SongCount() {
				rank++
				tref, err := q.UpsertLibraryTrack(ctx, dbq.UpsertLibraryTrackParams{
					LibraryID: db.NullString(SongLibraryID(i, j)), CatalogID: db.NullString(SongCatalogID(i, j)),
					Name: a.Song(j), ArtistName: a.Artist, AlbumName: a.Name,
					AlbumLibraryID: db.NullString(AlbumLibraryID(i)), AlbumCatalogID: db.NullString(AlbumCatalogID(i)),
					ArtistLibraryID: db.NullString(artistIDs[a.Artist]),
					ArtworkTemplate: Template(i), DurationMs: 60_000 + int64(j)*37_000, TrackNumber: int64(j + 1), DiscNumber: 1,
					ReleaseDate: ReleaseDate(i), Genre: "Pop", ContentRating: rating, UpdatedAt: now,
				})
				if err != nil {
					return err
				}
				trackRefs[[2]int{i, j}] = tref
				if err := q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{
					Kind: "song", Ref: tref, AddedRank: sql.NullInt64{Int64: rank, Valid: true}, Generation: 1,
				}); err != nil {
					return err
				}
			}
		}
		for n, p := range playlists {
			ref, err := q.UpsertLibraryPlaylist(ctx, dbq.UpsertLibraryPlaylistParams{
				LibraryID: db.NullString(PlaylistLibraryID(n)), Name: p.Name, Description: p.Description,
				ArtworkTemplate: PlaylistTemplate(n), CanEdit: 1, LastModified: "2026-01-01T00:00:00Z", UpdatedAt: now,
			})
			if err != nil {
				return err
			}
			if err := q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{
				Kind: "playlist", Ref: ref, DateAdded: sql.NullInt64{Int64: int64(n+1) * day, Valid: true}, Generation: 1,
			}); err != nil {
				return err
			}
			for pos, s := range p.Songs {
				if err := q.InsertPlaylistTrack(ctx, dbq.InsertPlaylistTrackParams{PlaylistID: ref, Position: int64(pos), TrackID: trackRefs[s]}); err != nil {
					return err
				}
			}
			if err := q.SetPlaylistTracksSynced(ctx, dbq.SetPlaylistTracksSyncedParams{
				TracksModified: "2026-01-01T00:00:00Z", TrackCount: int64(len(p.Songs)), ID: ref,
			}); err != nil {
				return err
			}
		}
		for _, dom := range []library.Domain{
			library.DomainArtists, library.DomainAlbums, library.DomainSongs,
			library.DomainPlaylists, library.DomainPlaylistTracks, library.DomainRatings,
		} {
			if err := q.StartSync(ctx, dbq.StartSyncParams{Domain: string(dom), Generation: 1, LastStarted: sql.NullInt64{Int64: now, Valid: true}}); err != nil {
				return err
			}
			if err := q.FinishSync(ctx, dbq.FinishSyncParams{
				LastSuccess: sql.NullInt64{Int64: now, Valid: true}, LastFull: sql.NullInt64{Int64: now, Valid: true}, Domain: string(dom),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("fixture: seed: %w", err)
	}
	return library.RebuildSearch(ctx, d)
}
