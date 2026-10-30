package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

// Library answers library queries from the local database only; its
// mutations (ratings, removal) call Apple and update the database at once.
type Library struct {
	db  *db.DB
	api *applemusic.Client
}

// New returns a Library over d. api is used only by the mutations
// (SetFavorite, SetSuggestLess, Remove); it may be nil for read-only use.
func New(d *db.DB, api *applemusic.Client) *Library {
	return &Library{db: d, api: api}
}

// ErrNotFound is returned for a library ID that is not stored.
var ErrNotFound = errors.New("library: not found")

// Sort selects a list order.
type Sort string

// List orders. Not every list supports every order; unsupported orders
// fall back to SortTitle.
const (
	SortTitle        Sort = "title"        // all lists
	SortArtist       Sort = "artist"       // songs, albums
	SortAlbum        Sort = "album"        // songs
	SortDateAdded    Sort = "dateAdded"    // songs, albums, playlists; Desc = newest first
	SortReleaseDate  Sort = "releaseDate"  // albums
	SortLastModified Sort = "lastModified" // playlists
)

// ListOptions pages and orders a list. Limit 0 means 100; it is capped at
// 1000.
type ListOptions struct {
	Sort   Sort `json:"sort"`
	Desc   bool `json:"desc"`
	Offset int  `json:"offset"`
	Limit  int  `json:"limit"`
}

func (o ListOptions) page() (limit, offset int) {
	limit = o.Limit
	if limit <= 0 {
		limit = 100
	}
	return min(limit, 1000), max(o.Offset, 0)
}

// Song is a library track.
type Song struct {
	LibraryID       string `json:"libraryId"`
	CatalogID       string `json:"catalogId"` // "" for uploads
	Name            string `json:"name"`
	ArtistName      string `json:"artistName"`
	AlbumName       string `json:"albumName"`
	AlbumLibraryID  string `json:"albumLibraryId"`
	ArtistLibraryID string `json:"artistLibraryId"`
	ArtworkTemplate string `json:"artworkTemplate"` // raw {w}x{h} URL
	DurationMS      int64  `json:"durationMs"`
	TrackNumber     int    `json:"trackNumber"`
	DiscNumber      int    `json:"discNumber"`
	ReleaseDate     string `json:"releaseDate"`
	Genre           string `json:"genre"`
	Explicit        bool   `json:"explicit"`
	HasLyrics       bool   `json:"hasLyrics"`
}

// Album is a library album.
type Album struct {
	LibraryID       string    `json:"libraryId"`
	CatalogID       string    `json:"catalogId"`
	Name            string    `json:"name"`
	ArtistName      string    `json:"artistName"`
	ArtistLibraryID string    `json:"artistLibraryId"`
	ArtworkTemplate string    `json:"artworkTemplate"`
	ReleaseDate     string    `json:"releaseDate"`
	Genre           string    `json:"genre"`
	TrackCount      int       `json:"trackCount"`
	Explicit        bool      `json:"explicit"`
	DateAdded       time.Time `json:"dateAdded"`
}

// Artist is a library artist.
type Artist struct {
	LibraryID       string `json:"libraryId"`
	CatalogID       string `json:"catalogId"`
	Name            string `json:"name"`
	ArtworkTemplate string `json:"artworkTemplate"`
}

// Playlist is a library playlist.
type Playlist struct {
	LibraryID       string    `json:"libraryId"`
	CatalogID       string    `json:"catalogId"` // catalog global ID, "" for private playlists
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	CuratorName     string    `json:"curatorName"`
	ArtworkTemplate string    `json:"artworkTemplate"`
	CanEdit         bool      `json:"canEdit"`
	TrackCount      int       `json:"trackCount"`
	LastModified    string    `json:"lastModified"`
	DateAdded       time.Time `json:"dateAdded"`
}

// Counts are the number of items per library list.
type Counts struct {
	Songs     int `json:"songs"`
	Albums    int `json:"albums"`
	Artists   int `json:"artists"`
	Playlists int `json:"playlists"`
}

// Hit is a local search result. Kind is "song", "album", "artist" or
// "playlist"; LibraryID identifies it within that kind.
type Hit struct {
	Kind            string `json:"kind"`
	LibraryID       string `json:"libraryId"`
	Name            string `json:"name"`
	ArtistName      string `json:"artistName"`
	AlbumName       string `json:"albumName"`
	ArtworkTemplate string `json:"artworkTemplate"`
}

// DomainStatus is the stored sync state of one domain.
type DomainStatus struct {
	Domain      Domain    `json:"domain"`
	InProgress  bool      `json:"inProgress"` // an unfinished run will resume
	Done        int       `json:"done"`       // checkpoint of the unfinished run
	Total       int       `json:"total"`
	LastStarted time.Time `json:"lastStarted"`
	LastSuccess time.Time `json:"lastSuccess"`
	LastError   string    `json:"lastError"`
}

const trackColumns = `t.library_id, t.catalog_id, t.name, t.artist_name, t.album_name,
	t.album_library_id, t.artist_library_id, t.artwork_template, t.duration_ms,
	t.track_number, t.disc_number, t.release_date, t.genre, t.content_rating, t.has_lyrics`

func scanSong(sc interface{ Scan(...any) error }) (Song, error) {
	var s Song
	var lib, cat, alb, art sql.NullString
	var rating string
	var lyrics int64
	err := sc.Scan(&lib, &cat, &s.Name, &s.ArtistName, &s.AlbumName, &alb, &art, &s.ArtworkTemplate,
		&s.DurationMS, &s.TrackNumber, &s.DiscNumber, &s.ReleaseDate, &s.Genre, &rating, &lyrics)
	s.LibraryID, s.CatalogID, s.AlbumLibraryID, s.ArtistLibraryID = lib.String, cat.String, alb.String, art.String
	s.Explicit, s.HasLyrics = rating == "explicit", lyrics != 0
	return s, err
}

func songFromRow(t dbq.Track) Song {
	return Song{
		LibraryID: t.LibraryID.String, CatalogID: t.CatalogID.String, Name: t.Name,
		ArtistName: t.ArtistName, AlbumName: t.AlbumName, AlbumLibraryID: t.AlbumLibraryID.String,
		ArtistLibraryID: t.ArtistLibraryID.String, ArtworkTemplate: t.ArtworkTemplate, DurationMS: t.DurationMs,
		TrackNumber: int(t.TrackNumber), DiscNumber: int(t.DiscNumber), ReleaseDate: t.ReleaseDate,
		Genre: t.Genre, Explicit: t.ContentRating == "explicit", HasLyrics: t.HasLyrics != 0,
	}
}

func songsFromRows(rows []dbq.Track) []Song {
	out := make([]Song, len(rows))
	for i, r := range rows {
		out[i] = songFromRow(r)
	}
	return out
}

func dir(desc bool) string {
	if desc {
		return "DESC"
	}
	return "ASC"
}

func query[T any](ctx context.Context, l *Library, q string, scan func(interface{ Scan(...any) error }) (T, error), args ...any) ([]T, error) {
	rows, err := l.db.R.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("library: %w", err)
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("library: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Songs lists library songs. SortDateAdded uses Apple's library order
// (Desc: most recently added first).
func (l *Library) Songs(ctx context.Context, o ListOptions) ([]Song, error) {
	d := dir(o.Desc)
	var order string
	switch o.Sort {
	case SortArtist:
		order = "t.artist_name COLLATE NOCASE " + d + ", t.album_name COLLATE NOCASE, t.disc_number, t.track_number, t.id"
	case SortAlbum:
		order = "t.album_name COLLATE NOCASE " + d + ", t.disc_number, t.track_number, t.id"
	case SortDateAdded:
		// added_rank 0 is the newest.
		order = "li.added_rank " + dir(!o.Desc) + ", t.id"
	default:
		order = "t.name COLLATE NOCASE " + d + ", t.artist_name COLLATE NOCASE, t.id"
	}
	limit, offset := o.page()
	return query(ctx, l, `SELECT `+trackColumns+` FROM tracks t
		JOIN library_items li ON li.kind = 'song' AND li.ref = t.id
		ORDER BY `+order+` LIMIT ? OFFSET ?`, scanSong, limit, offset)
}

const albumColumns = `a.library_id, a.catalog_id, a.name, a.artist_name, a.artist_library_id,
	a.artwork_template, a.release_date, a.genre, a.track_count, a.content_rating, li.date_added`

func scanAlbum(sc interface{ Scan(...any) error }) (Album, error) {
	var a Album
	var lib, cat, art sql.NullString
	var rating string
	var added sql.NullInt64
	err := sc.Scan(&lib, &cat, &a.Name, &a.ArtistName, &art, &a.ArtworkTemplate, &a.ReleaseDate, &a.Genre,
		&a.TrackCount, &rating, &added)
	a.LibraryID, a.CatalogID, a.ArtistLibraryID = lib.String, cat.String, art.String
	a.Explicit = rating == "explicit"
	if added.Valid {
		a.DateAdded = db.Time(added.Int64)
	}
	return a, err
}

// Albums lists library albums.
func (l *Library) Albums(ctx context.Context, o ListOptions) ([]Album, error) {
	d := dir(o.Desc)
	var order string
	switch o.Sort {
	case SortArtist:
		order = "a.artist_name COLLATE NOCASE " + d + ", a.name COLLATE NOCASE, a.id"
	case SortDateAdded:
		order = "li.date_added " + d + ", a.id"
	case SortReleaseDate:
		order = "a.release_date " + d + ", a.name COLLATE NOCASE, a.id"
	default:
		order = "a.name COLLATE NOCASE " + d + ", a.artist_name COLLATE NOCASE, a.id"
	}
	limit, offset := o.page()
	return query(ctx, l, `SELECT `+albumColumns+` FROM albums a
		JOIN library_items li ON li.kind = 'album' AND li.ref = a.id
		ORDER BY `+order+` LIMIT ? OFFSET ?`, scanAlbum, limit, offset)
}

func scanArtist(sc interface{ Scan(...any) error }) (Artist, error) {
	var a Artist
	var lib, cat sql.NullString
	err := sc.Scan(&lib, &cat, &a.Name, &a.ArtworkTemplate)
	a.LibraryID, a.CatalogID = lib.String, cat.String
	return a, err
}

// Artists lists library artists by name.
func (l *Library) Artists(ctx context.Context, o ListOptions) ([]Artist, error) {
	limit, offset := o.page()
	return query(ctx, l, `SELECT r.library_id, r.catalog_id, r.name, r.artwork_template FROM artists r
		JOIN library_items li ON li.kind = 'artist' AND li.ref = r.id
		ORDER BY r.name COLLATE NOCASE `+dir(o.Desc)+`, r.id LIMIT ? OFFSET ?`, scanArtist, limit, offset)
}

const playlistColumns = `p.library_id, p.catalog_id, p.name, p.description, p.curator_name,
	p.artwork_template, p.can_edit, p.track_count, p.last_modified, li.date_added`

func scanPlaylist(sc interface{ Scan(...any) error }) (Playlist, error) {
	var p Playlist
	var lib, cat sql.NullString
	var canEdit int64
	var added sql.NullInt64
	err := sc.Scan(&lib, &cat, &p.Name, &p.Description, &p.CuratorName, &p.ArtworkTemplate, &canEdit,
		&p.TrackCount, &p.LastModified, &added)
	p.LibraryID, p.CatalogID, p.CanEdit = lib.String, cat.String, canEdit != 0
	if added.Valid {
		p.DateAdded = db.Time(added.Int64)
	}
	return p, err
}

// Playlists lists library playlists.
func (l *Library) Playlists(ctx context.Context, o ListOptions) ([]Playlist, error) {
	d := dir(o.Desc)
	var order string
	switch o.Sort {
	case SortDateAdded:
		order = "li.date_added " + d + ", p.id"
	case SortLastModified:
		order = "p.last_modified " + d + ", p.id"
	default:
		order = "p.name COLLATE NOCASE " + d + ", p.id"
	}
	limit, offset := o.page()
	return query(ctx, l, `SELECT `+playlistColumns+` FROM playlists p
		JOIN library_items li ON li.kind = 'playlist' AND li.ref = p.id
		ORDER BY `+order+` LIMIT ? OFFSET ?`, scanPlaylist, limit, offset)
}

func one[T any](items []T, err error) (T, error) {
	var zero T
	if err != nil {
		return zero, err
	}
	if len(items) == 0 {
		return zero, ErrNotFound
	}
	return items[0], nil
}

// Album returns library album id and its tracks in disc/track order.
func (l *Library) Album(ctx context.Context, id string) (Album, []Song, error) {
	a, err := one(query(ctx, l, `SELECT `+albumColumns+` FROM albums a
		JOIN library_items li ON li.kind = 'album' AND li.ref = a.id
		WHERE a.library_id = ?`, scanAlbum, id))
	if err != nil {
		return Album{}, nil, err
	}
	rows, err := l.db.RQ.LibraryAlbumTracks(ctx, db.NullString(id))
	if err != nil {
		return Album{}, nil, fmt.Errorf("library: album tracks: %w", err)
	}
	return a, songsFromRows(rows), nil
}

// Artist returns library artist id, its library albums (newest first) and
// its library songs.
func (l *Library) Artist(ctx context.Context, id string) (Artist, []Album, []Song, error) {
	a, err := one(query(ctx, l, `SELECT r.library_id, r.catalog_id, r.name, r.artwork_template FROM artists r
		WHERE r.library_id = ?`, scanArtist, id))
	if err != nil {
		return Artist{}, nil, nil, err
	}
	albums, err := query(ctx, l, `SELECT `+albumColumns+` FROM albums a
		JOIN library_items li ON li.kind = 'album' AND li.ref = a.id
		WHERE a.artist_library_id = ?
		ORDER BY a.release_date DESC, a.name COLLATE NOCASE`, scanAlbum, id)
	if err != nil {
		return Artist{}, nil, nil, err
	}
	rows, err := l.db.RQ.LibraryArtistTracks(ctx, db.NullString(id))
	if err != nil {
		return Artist{}, nil, nil, fmt.Errorf("library: artist tracks: %w", err)
	}
	return a, albums, songsFromRows(rows), nil
}

// Playlist returns library playlist id and a page of its tracks in
// playlist order (o.Sort is ignored).
func (l *Library) Playlist(ctx context.Context, id string, o ListOptions) (Playlist, []Song, error) {
	p, err := one(query(ctx, l, `SELECT `+playlistColumns+` FROM playlists p
		JOIN library_items li ON li.kind = 'playlist' AND li.ref = p.id
		WHERE p.library_id = ?`, scanPlaylist, id))
	if err != nil {
		return Playlist{}, nil, err
	}
	limit, offset := o.page()
	songs, err := query(ctx, l, `SELECT `+trackColumns+` FROM playlist_tracks pt
		JOIN playlists p ON p.id = pt.playlist_id
		JOIN tracks t ON t.id = pt.track_id
		WHERE p.library_id = ?
		ORDER BY pt.position LIMIT ? OFFSET ?`, scanSong, id, limit, offset)
	return p, songs, err
}

// Song returns library song id.
func (l *Library) Song(ctx context.Context, id string) (Song, error) {
	t, err := l.db.RQ.GetTrackByLibraryID(ctx, db.NullString(id))
	if errors.Is(err, sql.ErrNoRows) {
		return Song{}, ErrNotFound
	}
	if err != nil {
		return Song{}, fmt.Errorf("library: %w", err)
	}
	return songFromRow(t), nil
}

// Counts returns the number of songs, albums, artists and playlists.
func (l *Library) Counts(ctx context.Context) (Counts, error) {
	rows, err := l.db.RQ.CountLibraryItems(ctx)
	if err != nil {
		return Counts{}, fmt.Errorf("library: counts: %w", err)
	}
	var c Counts
	for _, r := range rows {
		switch r.Kind {
		case "song":
			c.Songs = int(r.N)
		case "album":
			c.Albums = int(r.N)
		case "artist":
			c.Artists = int(r.N)
		case "playlist":
			c.Playlists = int(r.N)
		}
	}
	return c, nil
}

// Search finds library items whose name, artist or album contain words
// starting with every word of term (prefix matching, case and diacritics
// insensitive), best matches first. kinds filters by Hit.Kind (none: all).
func (l *Library) Search(ctx context.Context, term string, limit int, kinds ...string) ([]Hit, error) {
	match := matchExpr(term)
	if match == "" {
		return []Hit{}, nil
	}
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT kind, ref, name, artist, album, artwork FROM search_fts WHERE search_fts MATCH ?`
	args := []any{match}
	if len(kinds) > 0 {
		q += ` AND kind IN (?` + strings.Repeat(",?", len(kinds)-1) + `)`
		for _, k := range kinds {
			args = append(args, k)
		}
	}
	// Weights: kind, ref, artwork (unindexed), name, artist, album.
	q += ` ORDER BY bm25(search_fts, 0, 0, 0, 10, 4, 2) LIMIT ?`
	args = append(args, min(limit, 500))
	return query(ctx, l, q, func(sc interface{ Scan(...any) error }) (Hit, error) {
		var h Hit
		err := sc.Scan(&h.Kind, &h.LibraryID, &h.Name, &h.ArtistName, &h.AlbumName, &h.ArtworkTemplate)
		return h, err
	}, args...)
}

// matchExpr turns free text into an FTS5 query: every word must match as a
// prefix. Words are quoted, so FTS5 syntax in term is literal.
func matchExpr(term string) string {
	words := strings.FieldsFunc(term, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	parts := make([]string, 0, len(words))
	for _, w := range words {
		parts = append(parts, `"`+w+`"*`)
	}
	return strings.Join(parts, " ")
}

// SyncStatus returns the stored state of every domain that has synced.
func (l *Library) SyncStatus(ctx context.Context) ([]DomainStatus, error) {
	rows, err := l.db.RQ.ListSyncState(ctx)
	if err != nil {
		return nil, fmt.Errorf("library: sync state: %w", err)
	}
	out := make([]DomainStatus, 0, len(rows))
	for _, r := range rows {
		s := DomainStatus{
			Domain: Domain(r.Domain), InProgress: r.InProgress != 0, Done: int(r.NextOffset),
			Total: int(r.Total), LastError: r.LastError,
		}
		if r.LastStarted.Valid {
			s.LastStarted = db.Time(r.LastStarted.Int64)
		}
		if r.LastSuccess.Valid {
			s.LastSuccess = db.Time(r.LastSuccess.Int64)
		}
		out = append(out, s)
	}
	return out, nil
}

// Rating returns the stored rating (applemusic.RatingFavorite,
// RatingSuggestLess or 0) of resource id of Apple ratings type typ (e.g.
// applemusic.TypeLibrarySongs).
func (l *Library) Rating(ctx context.Context, typ, id string) (int, error) {
	v, err := l.db.RQ.GetFavorite(ctx, dbq.GetFavoriteParams{Kind: typ, AppleID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return int(v), err
}

// Favorites returns the IDs of resources of type typ rated Favourite, most
// recently rated first.
func (l *Library) Favorites(ctx context.Context, typ string) ([]string, error) {
	rows, err := l.db.RQ.ListFavorites(ctx, typ)
	if err != nil {
		return nil, fmt.Errorf("library: favorites: %w", err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.Value == applemusic.RatingFavorite {
			out = append(out, r.AppleID)
		}
	}
	return out, nil
}
