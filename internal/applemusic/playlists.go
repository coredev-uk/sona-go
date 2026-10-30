package applemusic

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/sync/errgroup"
)

// Playlist, playlist folder and pin endpoints of the web player's API
// (amp-api.music.apple.com). Every library call needs the Music User Token.

// Resource types and IDs specific to playlists.
const (
	TypeLibraryPlaylistFolders = "library-playlist-folders"
	// PlaylistsRoot is the folder holding the top level of the user's
	// playlist tree.
	PlaylistsRoot = "p.playlistsroot"
)

// Pin actions: what activating a sidebar pin does.
const (
	PinPlay    = "play"
	PinShuffle = "shuffle"
	PinDrillIn = "drillIn"
)

// catalogTrackPage is the page size of a catalog playlist's tracks.
const catalogTrackPage = 300

// ResourceRef names a resource in request bodies: {"id","type"}.
type ResourceRef struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// Deleted reports whether p is the tombstone Apple keeps listing after a
// library playlist is deleted: no name and not deletable.
func (p Playlist) Deleted() bool {
	return p.Attributes.Name == "" && !p.Attributes.CanDelete
}

// Curator is the owner of a catalog playlist: an apple-curators or curators
// resource, or the owner's social-profiles resource for user playlists.
type Curator struct {
	ID     string
	Type   string
	Name   string
	Handle string // social profiles
	URL    string
}

// CatalogPlaylist is a catalog playlist page: the playlist with every track,
// its curator, featured artists and more playlists by the same curator.
type CatalogPlaylist struct {
	Playlist        Playlist
	Curator         Curator
	FeaturedArtists []Artist
	MoreByCurator   []Playlist
}

type curatorAttributes struct {
	Name   string `json:"name"`
	Handle string `json:"handle"`
	URL    string `json:"url"`
}

// catalogPlaylistBody is the playlist page response.
type catalogPlaylistBody struct {
	Data []struct {
		Resource[PlaylistAttributes]
		Relationships struct {
			Tracks  Relationship[Song]                        `json:"tracks"`
			Curator Relationship[Resource[curatorAttributes]] `json:"curator"`
		} `json:"relationships"`
		Views struct {
			FeaturedArtists View[Artist]   `json:"featured-artists"`
			MoreByCurator   View[Playlist] `json:"more-by-curator"`
		} `json:"views"`
	} `json:"data"`
}

// playlistSongFields are the song attributes a playlist page needs.
var playlistSongFields = []string{
	"name", "artistName", "albumName", "artwork", "playParams", "contentRating", "durationInMillis",
	"url", "hasLyrics", "releaseDate", "genreNames", "discNumber", "trackNumber", "composerName", "isrc",
}

func playlistTrackQuery() url.Values {
	return url.Values{
		"include[songs]":        {"artists"},
		"include[music-videos]": {"artists"},
		"relate[songs]":         {"albums"},
		"fields[songs]":         {strings.Join(playlistSongFields, ",")},
		"fields[artists]":       {"name,url,artwork"}, // artwork: the featured-artists view
		"omit[resource]":        {"autos"},
	}
}

// CatalogPlaylist fetches catalog playlist id (pl.…, pl.u-…, pl.pm-…,
// pl.rp-…) with all of its tracks: the first 300 come with the playlist,
// the rest are fetched in parallel pages of 300.
func (c *Client) CatalogPlaylist(ctx context.Context, sf, id string) (CatalogPlaylist, error) {
	q := AMPQuery{
		Include: []string{"tracks", "curator"},
		Views:   []string{"featured-artists", "more-by-curator"},
		Extend:  []string{"editorialArtwork", "trackCount"},
		Fields: map[string][]string{
			"curators":        {"name", "url"},
			"apple-curators":  {"name", "url"},
			"social-profiles": {"name", "handle", "url"},
		},
		Extra: playlistTrackQuery(),
	}
	q.Extra.Set("limit[tracks]", strconv.Itoa(catalogTrackPage))
	q.Extra.Set("limit[view.featured-artists]", "15")
	q.Extra.Set("limit[view.more-by-curator]", "15")
	body, err := AMP[catalogPlaylistBody](ctx, c, CatalogPath(sf, TypePlaylists, id), q)
	if err != nil {
		return CatalogPlaylist{}, err
	}
	d, err := first(body.Data, "playlist "+id)
	if err != nil {
		return CatalogPlaylist{}, err
	}
	out := CatalogPlaylist{
		Playlist:        Playlist{Resource: d.Resource},
		FeaturedArtists: d.Views.FeaturedArtists.Data,
		MoreByCurator:   d.Views.MoreByCurator.Data,
	}
	out.Playlist.Relationships.Tracks = d.Relationships.Tracks
	if cur := d.Relationships.Curator.Data; len(cur) > 0 {
		a := cur[0].Attributes
		out.Curator = Curator{ID: cur[0].ID, Type: cur[0].Type, Name: a.Name, Handle: a.Handle, URL: a.URL}
	}
	tracks := &out.Playlist.Relationships.Tracks
	rest, err := c.catalogPlaylistTracks(ctx, sf, id, len(tracks.Data), out.Playlist.Attributes.TrackCount, tracks.Next)
	if err != nil {
		return CatalogPlaylist{}, fmt.Errorf("playlist %s tracks: %w", id, err)
	}
	tracks.Data = append(tracks.Data, rest...)
	tracks.Next = ""
	// The view is often empty; the curator's own playlist list stands in.
	if len(out.MoreByCurator) == 0 && (out.Curator.Type == "apple-curators" || out.Curator.Type == "curators") {
		more, err := AMP[Page[Playlist]](ctx, c, CatalogPath(sf, out.Curator.Type, out.Curator.ID, "playlists"), AMPQuery{Limit: 16})
		if err != nil && !errors.Is(err, ErrNotFound) {
			return CatalogPlaylist{}, fmt.Errorf("playlist %s curator playlists: %w", id, err)
		}
		for _, p := range more.Data {
			if p.ID != id && len(out.MoreByCurator) < 15 {
				out.MoreByCurator = append(out.MoreByCurator, p)
			}
		}
	}
	if out.FeaturedArtists == nil {
		out.FeaturedArtists = []Artist{}
	}
	if out.MoreByCurator == nil {
		out.MoreByCurator = []Playlist{}
	}
	return out, nil
}

// catalogPlaylistTracks fetches the tracks after the first have of a
// catalog playlist of total tracks (0: unknown, follow next links).
func (c *Client) catalogPlaylistTracks(ctx context.Context, sf, id string, have, total int, next string) ([]Song, error) {
	path := CatalogPath(sf, TypePlaylists, id, "tracks")
	page := func(ctx context.Context, offset int) (Page[Song], error) {
		q := AMPQuery{Limit: catalogTrackPage, Offset: offset, Extra: playlistTrackQuery()}
		p, err := AMP[Page[Song]](ctx, c, path, q)
		if errors.Is(err, ErrNotFound) { // past the end
			return Page[Song]{}, nil
		}
		return p, err
	}
	if total <= have {
		// Unknown total: walk the next links.
		var out []Song
		for off := have; next != ""; {
			p, err := page(ctx, off)
			if err != nil {
				return nil, err
			}
			out = append(out, p.Data...)
			if len(p.Data) == 0 {
				break
			}
			off += len(p.Data)
			next = p.Next
		}
		return out, nil
	}
	offsets := make([]int, 0, (total-have+catalogTrackPage-1)/catalogTrackPage)
	for off := have; off < total; off += catalogTrackPage {
		offsets = append(offsets, off)
	}
	pages := make([][]Song, len(offsets))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(defaultParallel)
	for i, off := range offsets {
		g.Go(func() error {
			p, err := page(gctx, off)
			pages[i] = p.Data
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	var out []Song
	for _, p := range pages {
		out = append(out, p...)
	}
	return out, nil
}

// LibraryPlaylistTree is one entry of the user's playlist tree: a playlist
// or a folder, under ParentID (PlaylistsRoot at the top), at Position among
// its siblings.
type LibraryPlaylistTree struct {
	ID        string
	Type      string // TypeLibraryPlaylists or TypeLibraryPlaylistFolders
	ParentID  string
	Position  int
	Name      string
	Tags      []string
	CanDelete bool
	DateAdded string
}

type treeItemAttributes struct {
	Name      string   `json:"name"`
	Tags      []string `json:"tags"`
	CanDelete bool     `json:"canDelete"`
	DateAdded string   `json:"dateAdded"`
}

// maxFolderDepth bounds the folder walk (Apple's tree is shallow; this
// only guards against a cycle).
const maxFolderDepth = 16

// PlaylistTree walks the user's playlist folders from the root and returns
// every folder and playlist in tree order. Deleted folders and playlists
// (tombstones) are left out.
func (c *Client) PlaylistTree(ctx context.Context) ([]LibraryPlaylistTree, error) {
	var out []LibraryPlaylistTree
	type level struct {
		id    string
		depth int
	}
	queue := []level{{PlaylistsRoot, 0}}
	seen := map[string]bool{PlaylistsRoot: true}
	for len(queue) > 0 {
		f := queue[0]
		queue = queue[1:]
		q := url.Values{"extend[library-playlists]": {"tags"}, "platform": {"web"}}
		pos := 0
		_, err := FetchPages(ctx, c, c.ampBase()+"/v1/me/library/playlist-folders/"+url.PathEscape(f.id)+"/children", q,
			PageOptions{Parallel: 2}, func(_ int, items []Resource[treeItemAttributes]) error {
				for _, it := range items {
					a := it.Attributes
					if a.Name == "" && !a.CanDelete {
						continue // tombstone
					}
					out = append(out, LibraryPlaylistTree{
						ID: it.ID, Type: it.Type, ParentID: f.id, Position: pos,
						Name: a.Name, Tags: a.Tags, CanDelete: a.CanDelete, DateAdded: a.DateAdded,
					})
					pos++
					if it.Type == TypeLibraryPlaylistFolders && !seen[it.ID] && f.depth < maxFolderDepth {
						seen[it.ID] = true
						queue = append(queue, level{it.ID, f.depth + 1})
					}
				}
				return nil
			})
		if err != nil && !errors.Is(err, ErrNotFound) { // an empty folder answers 404
			return nil, fmt.Errorf("playlist folder %s: %w", f.id, err)
		}
	}
	return out, nil
}

// amp sends a library mutation to the web player's API.
func (c *Client) amp(ctx context.Context, method, path string, q url.Values, body, out any) error {
	if q == nil {
		q = url.Values{}
	}
	q.Set("platform", "web")
	return c.Do(ctx, Request{Method: method, URL: c.ampBase() + path, Query: q, Body: body}, out)
}

func playlistPath(id string, parts ...string) string {
	p := LibraryPlaylistsPath + "/" + url.PathEscape(id)
	for _, s := range parts {
		p += "/" + s
	}
	return p
}

// PlaylistAttributesInput is the editable part of a library playlist.
type PlaylistAttributesInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsPublic    bool   `json:"isPublic"`
}

// CreateLibraryPlaylist creates a library playlist with tracks (songs or
// library-songs; catalog songs are also added to the library) and returns
// it.
func (c *Client) CreateLibraryPlaylist(ctx context.Context, attrs PlaylistAttributesInput, tracks []ResourceRef) (Playlist, error) {
	body := map[string]any{"attributes": attrs}
	if len(tracks) > 0 {
		body["relationships"] = map[string]any{"tracks": map[string]any{"data": tracks}}
	}
	var out Page[Playlist]
	if err := c.amp(ctx, http.MethodPost, LibraryPlaylistsPath, nil, body, &out); err != nil {
		return Playlist{}, err
	}
	return first(out.Data, "created playlist")
}

// UpdateLibraryPlaylist changes a library playlist's name, description and
// visibility.
func (c *Client) UpdateLibraryPlaylist(ctx context.Context, id string, attrs PlaylistAttributesInput) error {
	return c.amp(ctx, http.MethodPatch, playlistPath(id), nil, map[string]any{"attributes": attrs}, nil)
}

// DeleteLibraryPlaylist deletes a library playlist. Apple keeps listing a
// tombstone (see Playlist.Deleted).
func (c *Client) DeleteLibraryPlaylist(ctx context.Context, id string) error {
	return c.amp(ctx, http.MethodDelete, playlistPath(id), nil, nil, nil)
}

// AddPlaylistTracks appends tracks to library playlist id. Catalog songs
// become library songs too.
func (c *Client) AddPlaylistTracks(ctx context.Context, id string, tracks []ResourceRef) error {
	if len(tracks) == 0 {
		return nil
	}
	q := url.Values{"representation": {"ids"}}
	return c.amp(ctx, http.MethodPost, playlistPath(id, "tracks"), q, map[string]any{"data": tracks}, nil)
}

// ReplacePlaylistTracks sets the full, ordered track list of library
// playlist id (reorder and remove both go through it).
func (c *Client) ReplacePlaylistTracks(ctx context.Context, id string, tracks []ResourceRef) error {
	if tracks == nil {
		tracks = []ResourceRef{}
	}
	return c.amp(ctx, http.MethodPut, playlistPath(id, "tracks"), nil, map[string]any{"data": tracks}, nil)
}

// LibraryPlaylistEntries returns the ordered track references of library
// playlist id, as Apple holds them now (empty playlists answer 404).
func (c *Client) LibraryPlaylistEntries(ctx context.Context, id string) ([]Song, error) {
	var out []Song
	_, err := FetchPages(ctx, c, c.ampBase()+playlistPath(id, "tracks"), url.Values{"platform": {"web"}}, PageOptions{Parallel: 2},
		func(_ int, items []Song) error {
			out = append(out, items...)
			return nil
		})
	if errors.Is(err, ErrNotFound) {
		return []Song{}, nil
	}
	return out, err
}

// LibraryPlaylist fetches library playlist id's attributes.
func (c *Client) LibraryPlaylist(ctx context.Context, id string) (Playlist, error) {
	var out Page[Playlist]
	q := url.Values{"extend[library-playlists]": {"tags"}, "relate": {"catalog"}}
	if err := c.amp(ctx, http.MethodGet, playlistPath(id), q, nil, &out); err != nil {
		return Playlist{}, err
	}
	return first(out.Data, "library playlist "+id)
}

// MovePlaylistToFolder moves library playlist id into folder folderID
// (PlaylistsRoot for the top level).
func (c *Client) MovePlaylistToFolder(ctx context.Context, id, folderID string) error {
	body := map[string]any{"data": []ResourceRef{{ID: folderID, Type: TypeLibraryPlaylistFolders}}}
	return c.amp(ctx, http.MethodPut, playlistPath(id, "parent"), nil, body, nil)
}

const playlistFoldersPath = "/v1/me/library/playlist-folders"

// PlaylistFolder is a library playlist folder.
type PlaylistFolder = Resource[treeItemAttributes]

// CreatePlaylistFolder creates a folder at the top of the playlist tree.
func (c *Client) CreatePlaylistFolder(ctx context.Context, name string) (PlaylistFolder, error) {
	var out Page[PlaylistFolder]
	if err := c.amp(ctx, http.MethodPost, playlistFoldersPath, nil, map[string]any{"attributes": map[string]string{"name": name}}, &out); err != nil {
		return PlaylistFolder{}, err
	}
	return first(out.Data, "created folder")
}

// RenamePlaylistFolder renames folder id.
func (c *Client) RenamePlaylistFolder(ctx context.Context, id, name string) error {
	return c.amp(ctx, http.MethodPatch, playlistFoldersPath+"/"+url.PathEscape(id), nil, map[string]any{"attributes": map[string]string{"name": name}}, nil)
}

// DeletePlaylistFolder deletes folder id.
func (c *Client) DeletePlaylistFolder(ctx context.Context, id string) error {
	return c.amp(ctx, http.MethodDelete, playlistFoldersPath+"/"+url.PathEscape(id), nil, nil, nil)
}

// PinAttributes are the attributes of a pinned library item (a playlist,
// album, song, artist or music video).
type PinAttributes struct {
	Name       string      `json:"name"`
	ArtistName string      `json:"artistName"`
	AlbumName  string      `json:"albumName"`
	Artwork    *Artwork    `json:"artwork"`
	PlayParams *PlayParams `json:"playParams"`
}

// PinnedItem is one sidebar pin: a library resource with its pin action.
type PinnedItem struct {
	Resource[PinAttributes]
	Relationships struct {
		Albums  Relationship[Album]  `json:"albums"`
		Catalog Relationship[Artist] `json:"catalog"` // library artists: their catalog artist (artwork)
	} `json:"relationships"`
	Meta struct {
		LibraryPin struct {
			Action string `json:"action"`
		} `json:"libraryPin"`
	} `json:"meta"`
}

// MaxPins is how many pins Apple keeps.
const MaxPins = 6

const pinsPath = "/v1/me/library/pins"

// Pins returns the user's pins in order.
func (c *Client) Pins(ctx context.Context) ([]PinnedItem, error) {
	var out Page[PinnedItem]
	q := url.Values{
		"meta":                          {"libraryPin"},
		"limit":                         {strconv.Itoa(MaxPins)},
		"include[library-songs]":        {"albums"},
		"include[library-music-videos]": {"albums"},
		"include[library-artists]":      {"catalog"},
		"fields[artists]":               {"artwork,name"},
	}
	err := c.amp(ctx, http.MethodGet, pinsPath, q, nil, &out)
	if errors.Is(err, ErrNotFound) {
		return []PinnedItem{}, nil
	}
	return out.Data, err
}

// ErrPinsFull is returned by Pin when the user already has MaxPins pins
// (Apple answers 400).
var ErrPinsFull = errors.New("apple music: pins are full")

// Pin pins library item libraryID (its default action is play).
func (c *Client) Pin(ctx context.Context, libraryID string) error {
	err := c.amp(ctx, http.MethodPost, pinsPath+"/"+url.PathEscape(libraryID), nil, nil, nil)
	var se *StatusError
	if errors.As(err, &se) && se.Code == http.StatusBadRequest {
		return fmt.Errorf("%w: %w", ErrPinsFull, err)
	}
	return err
}

// Unpin removes the pin of libraryID.
func (c *Client) Unpin(ctx context.Context, libraryID string) error {
	return c.amp(ctx, http.MethodDelete, pinsPath+"/"+url.PathEscape(libraryID), nil, nil, nil)
}

// MovePin moves pin libraryID after pin afterID ("" moves it first).
func (c *Client) MovePin(ctx context.Context, libraryID, afterID string) error {
	if afterID == "" {
		afterID = "top"
	}
	return c.amp(ctx, http.MethodPost, pinsPath+"/"+url.PathEscape(libraryID), url.Values{"after": {afterID}}, nil, nil)
}

// SetPinAction sets what activating pin libraryID does (PinPlay,
// PinShuffle, PinDrillIn).
func (c *Client) SetPinAction(ctx context.Context, libraryID, action string) error {
	return c.amp(ctx, http.MethodPatch, pinsPath+"/"+url.PathEscape(libraryID), url.Values{"action": {action}}, nil, nil)
}

// SuggestedSongs returns up to 20 catalog songs Apple suggests adding to
// library playlist id.
func (c *Client) SuggestedSongs(ctx context.Context, id string) ([]Song, error) {
	var out struct {
		Results struct {
			Suggested []Song `json:"suggested"`
		} `json:"results"`
	}
	q := url.Values{
		"contexts":       {"playlist-suggested-songs"},
		"types":          {"songs"},
		"limit":          {"20"},
		"include[songs]": {"artists"},
		"relate[songs]":  {"albums"},
	}
	body := map[string]any{"targetContent": ResourceRef{ID: id, Type: TypeLibraryPlaylists}}
	if err := c.amp(ctx, http.MethodPost, "/v1/me/recommendations/suggested", q, body, &out); err != nil {
		return nil, err
	}
	if out.Results.Suggested == nil {
		return []Song{}, nil
	}
	return out.Results.Suggested, nil
}

// RecentPlaylistTargets returns the library playlists Apple suggests as
// "Add to Playlist" targets (recently used, editable).
func (c *Client) RecentPlaylistTargets(ctx context.Context) ([]Playlist, error) {
	var out Page[Playlist]
	if err := c.amp(ctx, http.MethodGet, LibraryPlaylistsPath, url.Values{"filter[featured]": {"suggested"}}, nil, &out); err != nil {
		return nil, err
	}
	keep := out.Data[:0]
	for _, p := range out.Data {
		if p.ID != "" && !p.Deleted() {
			keep = append(keep, p)
		}
	}
	return keep, nil
}
