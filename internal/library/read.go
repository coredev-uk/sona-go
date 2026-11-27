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
	// now is the clock of the release rule (release.go); nil: time.Now.
	now func() time.Time
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

// AlbumType narrows the album list: all albums, standard albums, or
// singles and EPs (see IsSingleRelease).
type AlbumType string

// Album types. An unknown type lists every album.
const (
	AlbumTypeAll    AlbumType = ""
	AlbumTypeAlbum  AlbumType = "albums"
	AlbumTypeSingle AlbumType = "singles"
)

// where is the SQL WHERE clause that keeps the albums of type t (none for
// all).
func (t AlbumType) where() string {
	switch t {
	case AlbumTypeAlbum:
		return "WHERE a.is_single = 0"
	case AlbumTypeSingle:
		return "WHERE a.is_single = 1"
	}
	return ""
}

// maxPage is the most rows a list call returns.
const maxPage = 1000

// ListOptions pages and orders a list. Limit 0 means 100; it is capped at
// 1000. Type applies to albums only.
type ListOptions struct {
	// Query keeps the songs the search index finds for it (Songs only).
	Query  string    `json:"query"`
	Sort   Sort      `json:"sort"`
	Desc   bool      `json:"desc"`
	Type   AlbumType `json:"type"`
	Offset int       `json:"offset"`
	Limit  int       `json:"limit"`
}

func (o ListOptions) page() (limit, offset int) {
	limit = o.Limit
	if limit <= 0 {
		limit = 100
	}
	return min(limit, maxPage), max(o.Offset, 0)
}

// allPages collects the rows fetch lists from o.Offset on, a page at a time.
func allPages[T any](o ListOptions, fetch func(ListOptions) ([]T, error)) ([]T, error) {
	o.Limit = maxPage
	var all []T
	for {
		page, err := fetch(o)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < maxPage {
			return all, nil
		}
		o.Offset += len(page)
	}
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
	// AlbumCatalogID and ArtistCatalogID are the catalogue's: all a catalogue
	// song stored by a playlist has (library.StoreSong), and what a library
	// song with no library album or artist of its own is opened by.
	AlbumCatalogID  string `json:"albumCatalogId"`
	ArtistCatalogID string `json:"artistCatalogId"`
	ArtworkTemplate string `json:"artworkTemplate"` // raw {w}x{h} URL
	DurationMS      int64  `json:"durationMs"`
	TrackNumber     int    `json:"trackNumber"`
	DiscNumber      int    `json:"discNumber"`
	ReleaseDate     string `json:"releaseDate"`
	Genre           string `json:"genre"`
	Explicit        bool   `json:"explicit"`
	HasLyrics       bool   `json:"hasLyrics"`
	// NoPlayParams: Apple gave the song no playParams.
	NoPlayParams bool `json:"noPlayParams"`
	// Unreleased: the song is not out yet (a placeholder of a pre-added
	// album: release.go's rule, as of the library's clock). It is listed
	// only on its album.
	Unreleased bool `json:"unreleased"`
	// NotInLibrary: a track of a playlist (Playlist) that is not one of the
	// library's songs: a catalogue song of an added or collaborative
	// playlist, which Apple keeps in the playlist only. A row of any other
	// listing is in the library: false.
	NotInLibrary bool `json:"notInLibrary"`
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
	Single          bool      `json:"single"` // a single or an EP (IsSingleRelease)
	DateAdded       time.Time `json:"dateAdded"`
	// CatalogReleaseDate and Prerelease are its catalog album's (release.go):
	// a pre-added album has no ReleaseDate of its own.
	CatalogReleaseDate string `json:"catalogReleaseDate"`
	Prerelease         bool   `json:"prerelease"`
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

// trackColumns are the columns of a library song (scanSong) of the track
// `t` joined with withAlbum, whether it is out as of today (release.go).
func trackColumns(today string) string {
	return `t.library_id, t.catalog_id, t.name, t.artist_name, t.album_name,
	t.album_library_id, t.artist_library_id, t.album_catalog_id, t.artist_catalog_id, t.artwork_template, t.duration_ms,
	t.track_number, t.disc_number, t.release_date, t.genre, t.content_rating, t.has_lyrics, t.no_play_params,
	` + unreleased(today)
}

func scanSong(sc scanner) (Song, error) { return scanSongWith(sc) }

// scanSongWith scans the columns of trackColumns, then the columns that
// follow them into extra.
func scanSongWith(sc scanner, extra ...any) (Song, error) {
	var s Song
	var lib, cat, alb, art, albCat, artCat sql.NullString
	var rating string
	var lyrics, noParams, notOut int64
	err := sc.Scan(append([]any{
		&lib, &cat, &s.Name, &s.ArtistName, &s.AlbumName, &alb, &art, &albCat, &artCat, &s.ArtworkTemplate,
		&s.DurationMS, &s.TrackNumber, &s.DiscNumber, &s.ReleaseDate, &s.Genre, &rating, &lyrics, &noParams, &notOut,
	}, extra...)...)
	s.LibraryID, s.CatalogID, s.AlbumLibraryID, s.ArtistLibraryID = lib.String, cat.String, alb.String, art.String
	s.AlbumCatalogID, s.ArtistCatalogID = albCat.String, artCat.String
	s.Explicit, s.HasLyrics, s.NoPlayParams, s.Unreleased = rating == "explicit", lyrics != 0, noParams != 0, notOut != 0
	return s, err
}

func dir(desc bool) string {
	if desc {
		return "DESC"
	}
	return "ASC"
}

// scanner is what a row and the sql.Rows cursor share: scan the columns.
type scanner interface{ Scan(...any) error }

func query[T any](ctx context.Context, l *Library, q string, scan func(scanner) (T, error), args ...any) ([]T, error) {
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

// Songs lists the library songs that are out (not a pre-added album's
// placeholders: release.go). SortDateAdded uses Apple's library order (Desc:
// most recently added first).
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
	today := l.today()
	where, args := songFilter(o.Query)
	return query(ctx, l, `SELECT `+trackColumns(today)+` FROM tracks t
		JOIN library_items li ON li.kind = 'song' AND li.ref = t.id`+withAlbum+`
		WHERE `+out(today)+where+`
		ORDER BY `+order+` LIMIT ? OFFSET ?`, scanSong, append(args, limit, offset)...)
}

// songFilter is the WHERE clause (to follow the out() one) and arguments that
// keep the songs found by term in the search index: the words of term, each
// a prefix of one in the name, artist or album (Search's rule). A term with
// no words keeps every song.
func songFilter(term string) (string, []any) {
	match := matchExpr(term)
	if match == "" {
		return "", nil
	}
	return ` AND t.library_id IN (SELECT ref FROM search_fts WHERE kind = 'song' AND search_fts MATCH ?)`, []any{match}
}

// SongCount is the number of library songs that are out and found by term
// (all of them when term has no words): Songs' total.
func (l *Library) SongCount(ctx context.Context, term string) (int, error) {
	where, args := songFilter(term)
	if where == "" {
		c, err := l.Counts(ctx)
		return c.Songs, err
	}
	today := l.today()
	n, err := query(ctx, l, `SELECT count(*) FROM tracks t
		JOIN library_items li ON li.kind = 'song' AND li.ref = t.id`+withAlbum+`
		WHERE `+out(today)+where, func(sc scanner) (int, error) {
		var n int
		return n, sc.Scan(&n)
	}, args...)
	if err != nil || len(n) == 0 {
		return 0, err
	}
	return n[0], nil
}

// AllSongs is Songs for every row from o.Offset on, collected page by page.
func (l *Library) AllSongs(ctx context.Context, o ListOptions) ([]Song, error) {
	return allPages(o, func(o ListOptions) ([]Song, error) { return l.Songs(ctx, o) })
}

const albumColumns = `a.library_id, a.catalog_id, a.name, a.artist_name, a.artist_library_id,
	a.artwork_template, a.release_date, a.genre, a.track_count, a.content_rating, a.is_single, li.date_added,
	a.is_prerelease, a.catalog_release_date`

func scanAlbum(sc scanner) (Album, error) {
	var a Album
	var lib, cat, art sql.NullString
	var rating string
	var added sql.NullInt64
	var pre int64
	err := sc.Scan(&lib, &cat, &a.Name, &a.ArtistName, &art, &a.ArtworkTemplate, &a.ReleaseDate, &a.Genre,
		&a.TrackCount, &rating, &a.Single, &added, &pre, &a.CatalogReleaseDate)
	a.LibraryID, a.CatalogID, a.ArtistLibraryID = lib.String, cat.String, art.String
	a.Explicit = rating == "explicit"
	a.Prerelease = pre != 0
	if added.Valid {
		a.DateAdded = db.Time(added.Int64)
	}
	return a, err
}

// Albums lists library albums (o.Type narrows them).
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
		`+o.Type.where()+`
		ORDER BY `+order+` LIMIT ? OFFSET ?`, scanAlbum, limit, offset)
}

// AlbumCount returns the number of library albums of type t.
func (l *Library) AlbumCount(ctx context.Context, t AlbumType) (int, error) {
	var n int
	err := l.db.R.QueryRowContext(ctx, `SELECT count(*) FROM albums a
		JOIN library_items li ON li.kind = 'album' AND li.ref = a.id
		`+t.where()).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("library: album count: %w", err)
	}
	return n, nil
}

func scanArtist(sc scanner) (Artist, error) {
	var a Artist
	var lib, cat sql.NullString
	err := sc.Scan(&lib, &cat, &a.Name, &a.ArtworkTemplate)
	a.LibraryID, a.CatalogID = lib.String, cat.String
	return a, err
}

const playlistColumns = `p.library_id, p.catalog_id, p.name, p.description, p.curator_name,
	p.artwork_template, p.can_edit, p.track_count, p.last_modified, li.date_added`

func scanPlaylist(sc scanner) (Playlist, error) {
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

// Playlists lists library playlists, except Apple's Favourite Songs playlist
// (Favorited): Sona shows those songs once, on Library → Favourite songs.
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
	pls, err := query(ctx, l, `SELECT `+playlistColumns+` FROM playlists p
		JOIN library_items li ON li.kind = 'playlist' AND li.ref = p.id
		WHERE p.favorited = 0
		ORDER BY `+order+` LIMIT ? OFFSET ?`, scanPlaylist, limit, offset)
	if err != nil {
		return nil, err
	}
	return pls, l.FillCovers(ctx, pls)
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

// Album returns library album id and all its tracks in disc/track order,
// those not out yet too (Song.Unreleased): the album's page presents them.
func (l *Library) Album(ctx context.Context, id string) (Album, []Song, error) {
	a, err := one(query(ctx, l, `SELECT `+albumColumns+` FROM albums a
		JOIN library_items li ON li.kind = 'album' AND li.ref = a.id
		WHERE a.library_id = ?`, scanAlbum, id))
	if err != nil {
		return Album{}, nil, err
	}
	// Every song of the album, the ones not out yet included (Unreleased).
	songs, err := query(ctx, l, `SELECT `+trackColumns(l.today())+` FROM tracks t`+withAlbum+`
		WHERE t.album_library_id = ?
		ORDER BY t.disc_number = 0, t.disc_number, t.track_number, t.name COLLATE NOCASE`, scanSong, id)
	if err != nil {
		return Album{}, nil, fmt.Errorf("library: album tracks: %w", err)
	}
	return a, songs, nil
}

// artistRow is the artist the Artists list shows under key: a library
// artist's library ID, or the catalogue ID of an artist the library holds
// only through the credits of songs (credit-only: no library ID, the
// catalogue row's artwork when the catalogue cache has one). ErrNotFound for
// neither.
func (l *Library) artistRow(ctx context.Context, key string) (Artist, error) {
	a, err := one(query(ctx, l, `SELECT r.library_id, r.catalog_id, r.name, r.artwork_template FROM artists r
		WHERE r.library_id = ?`, scanArtist, key))
	if !errors.Is(err, ErrNotFound) {
		return a, err
	}
	return one(query(ctx, l, `SELECT NULL, ta.artist_catalog_id, ta.name,
			coalesce((SELECT c.artwork_template FROM artists c WHERE c.catalog_id = ta.artist_catalog_id AND c.library_id IS NULL LIMIT 1), '')
		FROM track_artists ta
		WHERE ta.artist_catalog_id = ? AND EXISTS (SELECT 1 FROM library_items x WHERE x.kind = 'song' AND x.ref = ta.track_id)
		ORDER BY ta.position, ta.track_id LIMIT 1`, scanArtist, key))
}

// ArtistAlbums returns the artist under key (a library artist's library ID, or
// the catalogue ID of a credit-only artist, see artistRow) and its library
// albums (newest release first), without its songs: what the Artists overview
// shows. A credit-only artist has no album: albums are not attributed by
// credit.
func (l *Library) ArtistAlbums(ctx context.Context, key string) (Artist, []Album, error) {
	a, err := l.artistRow(ctx, key)
	if err != nil {
		return Artist{}, nil, err
	}
	if a.LibraryID == "" {
		return a, []Album{}, nil
	}
	albums, err := query(ctx, l, `SELECT `+albumColumns+` FROM albums a
		JOIN library_items li ON li.kind = 'album' AND li.ref = a.id
		WHERE a.artist_library_id = ?
		ORDER BY a.release_date DESC, a.name COLLATE NOCASE`, scanAlbum, key)
	if err != nil {
		return Artist{}, nil, err
	}
	return a, albums, nil
}

// Artist returns the artist under key (see ArtistAlbums), its library albums
// (newest first) and its library songs that are out: the songs credited to
// it (Syncer.syncCredits), and those of its library artist with no credits.
func (l *Library) Artist(ctx context.Context, key string) (Artist, []Album, []Song, error) {
	a, albums, err := l.ArtistAlbums(ctx, key)
	if err != nil {
		return Artist{}, nil, nil, err
	}
	today := l.today()
	songs, err := query(ctx, l, `SELECT `+trackColumns(today)+` FROM tracks t
		JOIN library_items li ON li.kind = 'song' AND li.ref = t.id`+withAlbum+`
		WHERE (t.artist_library_id = ? OR t.id IN (SELECT k.track_id FROM track_credit_keys k WHERE k.artist_key = ?)) AND `+out(today)+`
		ORDER BY t.album_name COLLATE NOCASE, t.disc_number, t.track_number`, scanSong, key, key)
	if err != nil {
		return Artist{}, nil, nil, fmt.Errorf("library: artist tracks: %w", err)
	}
	return a, albums, songs, nil
}

// ArtistSummary is an artist of the Artists list with how much of them the
// library holds: a library artist (LibraryID set), or one the library holds
// only through the credits of songs (LibraryID empty; CatalogID is its key).
// An artist with no artwork of their own takes the cover of their newest
// library album.
type ArtistSummary struct {
	Artist
	Albums int `json:"albums"`
	Songs  int `json:"songs"`
}

// ArtistIndex lists every artist by name with the number of library albums
// and songs (that are out) they have: the Artists overview's list, filtered in
// the UI, so it is one query and never paged.
//
// Songs group by the artists the catalogue credits them to (track_artists,
// through track_credit_keys), each credited artist counting the song; a song
// with no credits counts for its library artist. A credited artist the library
// has no artist row for is listed from its credits. A library artist whose
// songs were all credited elsewhere ("A & B") and that holds no album is not
// listed: albums are still attributed to their first artist alone, so one with
// albums stays.
func (l *Library) ArtistIndex(ctx context.Context) ([]ArtistSummary, error) {
	today := l.today()
	return query(ctx, l, `WITH cr AS (
			SELECT DISTINCT t.artist_library_id AS key FROM track_artists ta
			JOIN tracks t ON t.id = ta.track_id WHERE t.artist_library_id IS NOT NULL),
		so AS (
			SELECT k.artist_key AS key, COUNT(*) AS n FROM track_credit_keys k
			JOIN tracks t ON t.id = k.track_id
			JOIN library_items x ON x.kind = 'song' AND x.ref = t.id`+withAlbum+`
			WHERE `+out(today)+` GROUP BY 1),
		al AS (
			SELECT a.artist_library_id AS key, COUNT(*) AS n FROM albums a
			JOIN library_items x ON x.kind = 'album' AND x.ref = a.id GROUP BY 1)
		SELECT library_id, catalog_id, name, artwork, albums, songs FROM (
			SELECT r.library_id AS library_id, r.catalog_id AS catalog_id, r.name AS name,
				COALESCE(NULLIF(r.artwork_template, ''), (
					SELECT a.artwork_template FROM albums a
					JOIN library_items x ON x.kind = 'album' AND x.ref = a.id
					WHERE a.artist_library_id = r.library_id AND a.artwork_template <> ''
					ORDER BY a.release_date DESC LIMIT 1), '') AS artwork,
				COALESCE(al.n, 0) AS albums, COALESCE(so.n, 0) AS songs, r.library_id AS ord
			FROM artists r
			JOIN library_items li ON li.kind = 'artist' AND li.ref = r.id
			LEFT JOIN al ON al.key = r.library_id
			LEFT JOIN so ON so.key = r.library_id
			WHERE NOT (COALESCE(al.n, 0) = 0 AND COALESCE(so.n, 0) = 0
				AND r.library_id IN (SELECT key FROM cr))
			UNION ALL
			SELECT NULL, so.key,
				(SELECT ta.name FROM track_artists ta WHERE ta.artist_catalog_id = so.key ORDER BY ta.position, ta.track_id LIMIT 1),
				COALESCE((SELECT c.artwork_template FROM artists c WHERE c.catalog_id = so.key AND c.library_id IS NULL LIMIT 1), ''),
				0, so.n, so.key
			FROM so
			WHERE EXISTS (SELECT 1 FROM track_artists ta WHERE ta.artist_catalog_id = so.key)
				AND NOT EXISTS (SELECT 1 FROM artists r WHERE r.library_id = so.key)
		) ORDER BY name COLLATE NOCASE, ord`, func(sc scanner) (ArtistSummary, error) {
		var s ArtistSummary
		var lib, cat sql.NullString
		err := sc.Scan(&lib, &cat, &s.Name, &s.ArtworkTemplate, &s.Albums, &s.Songs)
		s.LibraryID, s.CatalogID = lib.String, cat.String
		return s, err
	})
}

// playlistRow is library playlist id's row, without the cover FillCovers
// gives it. ErrNotFound when the library holds none.
func (l *Library) playlistRow(ctx context.Context, id string) (Playlist, error) {
	return one(query(ctx, l, `SELECT `+playlistColumns+` FROM playlists p
		JOIN library_items li ON li.kind = 'playlist' AND li.ref = p.id
		WHERE p.library_id = ?`, scanPlaylist, id))
}

// Playlist returns library playlist id and a page of its tracks in
// playlist order (o.Sort is ignored). Apple lets no song that is not out be
// added to a playlist, so it lists every track as stored (a one that were
// there would be Unreleased and not playable). A track that is not in the
// library (library_items) is NotInLibrary: an added or collaborative
// playlist holds songs the library does not.
func (l *Library) Playlist(ctx context.Context, id string, o ListOptions) (Playlist, []Song, error) {
	p, err := l.playlistRow(ctx, id)
	if err != nil {
		return Playlist{}, nil, err
	}
	pls := []Playlist{p}
	if err := l.FillCovers(ctx, pls); err != nil {
		return Playlist{}, nil, err
	}
	p = pls[0]
	limit, offset := o.page()
	songs, err := query(ctx, l, `SELECT `+trackColumns(l.today())+`, li.ref IS NULL FROM playlist_tracks pt
		JOIN playlists p ON p.id = pt.playlist_id
		JOIN tracks t ON t.id = pt.track_id`+withAlbum+`
		LEFT JOIN library_items li ON li.kind = 'song' AND li.ref = t.id
		WHERE p.library_id = ?
		ORDER BY pt.position LIMIT ? OFFSET ?`, func(sc scanner) (Song, error) {
		var out int64
		s, err := scanSongWith(sc, &out)
		s.NotInLibrary = out != 0
		return s, err
	}, id, limit, offset)
	return p, songs, err
}

// AllPlaylist is Playlist with every one of the playlist's tracks.
func (l *Library) AllPlaylist(ctx context.Context, id string) (p Playlist, songs []Song, err error) {
	songs, err = allPages(ListOptions{}, func(o ListOptions) ([]Song, error) {
		var page []Song
		p, page, err = l.Playlist(ctx, id, o)
		return page, err
	})
	return p, songs, err
}

// PlacedPlaylist is a library playlist with its place in the folder tree.
type PlacedPlaylist struct {
	Playlist
	FolderID  string // "" for the top level
	Position  int
	CanDelete bool
	Favorited bool
	// Collaborative and Host: Apple's hasCollaboration and
	// isCollaborativeHost, as the last playlist sync saw them.
	Collaborative bool
	Host          bool
}

// PlaylistTree returns the playlist folders and every library playlist
// with its place in them, from SQLite. Covers are filled as FillCovers does.
func (l *Library) PlaylistTree(ctx context.Context) ([]dbq.PlaylistFolder, []PlacedPlaylist, error) {
	folders, err := l.db.RQ.ListPlaylistFolders(ctx)
	if err != nil {
		return nil, nil, err
	}
	rows, err := l.db.RQ.LibraryPlaylistTree(ctx)
	if err != nil {
		return nil, nil, err
	}
	pls := make([]Playlist, len(rows))
	for i, r := range rows {
		pls[i] = Playlist{
			LibraryID: r.LibraryID.String, CatalogID: r.CatalogID.String, Name: r.Name, Description: r.Description,
			CuratorName: r.CuratorName, ArtworkTemplate: r.ArtworkTemplate, CanEdit: r.CanEdit != 0,
			TrackCount: int(r.TrackCount), LastModified: r.LastModified,
		}
		if r.DateAdded.Valid {
			pls[i].DateAdded = db.Time(r.DateAdded.Int64)
		}
	}
	if err := l.FillCovers(ctx, pls); err != nil {
		return nil, nil, err
	}
	out := make([]PlacedPlaylist, len(rows))
	for i, r := range rows {
		out[i] = PlacedPlaylist{
			Playlist: pls[i], FolderID: r.FolderID, Position: int(r.FolderPosition),
			CanDelete: r.CanDelete != 0, Favorited: r.Favorited != 0,
			Collaborative: r.IsCollaborative != 0, Host: r.CollaborativeHost != 0,
		}
	}
	return folders, out, nil
}

// Song returns library song id (Unreleased when it is not out yet).
func (l *Library) Song(ctx context.Context, id string) (Song, error) {
	return one(query(ctx, l, `SELECT `+trackColumns(l.today())+` FROM tracks t`+withAlbum+`
		WHERE t.library_id = ?`, scanSong, id))
}

// Counts returns the number of songs (that are out), albums, artists and
// playlists.
func (l *Library) Counts(ctx context.Context) (Counts, error) {
	// One statement: a row per kind, the songs less those not out yet
	// (release.go; found by index, not by reading every song) and the
	// playlists less the Favourite Songs one (Playlists leaves it out).
	rows, err := query(ctx, l, `SELECT li.kind, count(*) - CASE li.kind WHEN 'song' THEN (
			SELECT count(*) FROM tracks t
			JOIN library_items x ON x.kind = 'song' AND x.ref = t.id`+withAlbum+`
			WHERE `+unreleased(l.today())+`) WHEN 'playlist' THEN (
			SELECT count(*) FROM playlists p
			JOIN library_items x ON x.kind = 'playlist' AND x.ref = p.id WHERE p.favorited <> 0) ELSE 0 END
		FROM library_items li GROUP BY li.kind`, func(sc scanner) (kindCount, error) {
		var k kindCount
		err := sc.Scan(&k.kind, &k.n)
		return k, err
	})
	if err != nil {
		return Counts{}, fmt.Errorf("library: counts: %w", err)
	}
	var c Counts
	for _, r := range rows {
		switch r.kind {
		case "song":
			c.Songs = r.n
		case "album":
			c.Albums = r.n
		case "artist":
			c.Artists = r.n
		case "playlist":
			c.Playlists = r.n
		}
	}
	return c, nil
}

// kindCount is a library kind's number of items.
type kindCount struct {
	kind string
	n    int
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
	// The index holds a pre-added album's placeholder songs: a song is a hit
	// only while it is out (release.go; the index can't, it doesn't change
	// with the clock).
	today := l.today()
	q += ` AND (kind <> 'song' OR EXISTS (SELECT 1 FROM tracks t` + withAlbum + `
		WHERE t.library_id = search_fts.ref AND ` + out(today) + `))`
	// The Favourite Songs playlist is not a hit: Library → Favourite songs
	// is where those songs are.
	q += ` AND (kind <> 'playlist' OR NOT EXISTS (SELECT 1 FROM playlists p
		WHERE p.library_id = search_fts.ref AND p.favorited <> 0))`
	if len(kinds) > 0 {
		q += ` AND kind IN (` + placeholders(len(kinds)) + `)`
		args = append(args, anys(kinds)...)
	}
	// Weights: kind, ref, artwork (unindexed), name, artist, album.
	q += ` ORDER BY bm25(search_fts, 0, 0, 0, 10, 4, 2) LIMIT ?`
	args = append(args, min(limit, 500))
	return query(ctx, l, q, func(sc scanner) (Hit, error) {
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
