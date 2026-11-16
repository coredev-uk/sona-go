package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

// MusicVideo is a library music video as the sync stored it.
type MusicVideo struct {
	LibraryID       string `json:"libraryId"`
	CatalogID       string `json:"catalogId"` // "" when Apple gave none
	Name            string `json:"name"`
	ArtistName      string `json:"artistName"`
	AlbumName       string `json:"albumName"`
	DurationMS      int64  `json:"durationMs"`
	ReleaseDate     string `json:"releaseDate"`
	Genre           string `json:"genre"`
	Explicit        bool   `json:"explicit"`
	Has4K           bool   `json:"has4K"`
	HasHDR          bool   `json:"hasHDR"`
	URL             string `json:"url"`
	ArtworkTemplate string `json:"artworkTemplate"` // raw {w}x{h} URL of Apple's 16:9 frame
	ArtworkWidth    int    `json:"artworkWidth"`
	ArtworkHeight   int    `json:"artworkHeight"`
}

// MusicVideosSynced reports whether the music videos domain has completed a
// sync: before that the stored list is not the library's, and an empty one
// does not mean the library has none.
func (l *Library) MusicVideosSynced(ctx context.Context) (bool, error) {
	st, err := l.db.RQ.GetSyncState(ctx, string(DomainMusicVideos))
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("library: music videos state: %w", err)
	}
	return st.LastSuccess.Valid, nil
}

// MusicVideos lists the stored music videos in Apple's order, and how many
// there are. Only o's Offset and Limit apply.
func (l *Library) MusicVideos(ctx context.Context, o ListOptions) ([]MusicVideo, int, error) {
	limit, offset := o.page()
	rows, err := l.db.RQ.ListLibraryMusicVideos(ctx, dbq.ListLibraryMusicVideosParams{Limit: int64(limit), Offset: int64(offset)})
	if err != nil {
		return nil, 0, fmt.Errorf("library: music videos: %w", err)
	}
	total, err := l.db.RQ.CountLibraryMusicVideos(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("library: music videos: %w", err)
	}
	out := make([]MusicVideo, len(rows))
	for i, r := range rows {
		out[i] = MusicVideo{
			LibraryID: r.LibraryID, CatalogID: r.CatalogID, Name: r.Name, ArtistName: r.ArtistName, AlbumName: r.AlbumName,
			DurationMS: r.DurationMs, ReleaseDate: r.ReleaseDate, Genre: r.Genre,
			Explicit: r.Explicit != 0, Has4K: r.Has4k != 0, HasHDR: r.HasHdr != 0, URL: r.Url,
			ArtworkTemplate: r.ArtworkTemplate, ArtworkWidth: int(r.ArtworkWidth), ArtworkHeight: int(r.ArtworkHeight),
		}
	}
	return out, int(total), nil
}
