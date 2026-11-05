package services

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/library"
)

// PlaylistsService serves playlist pages (catalog and library), edits the
// user's library playlists and folders, and keeps the sidebar pins. Every
// edit goes to Apple first; on success SQLite is updated in place (no
// sync) and "playlists:changed" is emitted.
type PlaylistsService struct {
	e *Env
	m mapper

	mu    sync.Mutex
	locks map[string]*sync.Mutex // per playlist: edits apply one at a time
	me    string                 // the user's profile ID, once asked (Collaboration)
}

// newPlaylistsService also refreshes the pins once the network opens and
// on sign-in.
func newPlaylistsService(e *Env) *PlaylistsService {
	s := &PlaylistsService{e: e, m: e.m, locks: map[string]*sync.Mutex{}}
	e.OnNetwork(func() { go s.refreshPinsQuietly() })
	prev := e.Auth.Status().State
	e.Auth.OnChange(func(st auth.Status) {
		was := prev
		prev = st.State
		if st.State != was {
			s.mu.Lock()
			s.me = ""
			s.mu.Unlock()
		}
		if st.State == auth.StateSignedIn && was != auth.StateSignedIn {
			go e.OnNetwork(func() { go s.refreshPinsQuietly() })
		}
	})
	return s
}

func (s *PlaylistsService) lock(id string) func() {
	s.mu.Lock()
	l, ok := s.locks[id]
	if !ok {
		l = &sync.Mutex{}
		s.locks[id] = l
	}
	s.mu.Unlock()
	l.Lock()
	return l.Unlock
}

func (s *PlaylistsService) changed(ev PlaylistsChanged) {
	if ev.IDs == nil {
		ev.IDs = []string{}
	}
	s.e.Emit.Emit(EventPlaylistsChanged, ev)
}

func isLibraryPlaylist(id string) bool { return strings.HasPrefix(id, "p.") }

func requireLibraryPlaylist(id string) error {
	if !isLibraryPlaylist(id) {
		return fmt.Errorf("%w: %q is not a library playlist", ErrInvalidArgument, id)
	}
	return nil
}

// Playlist returns a playlist page: a library playlist ("p.…") from
// SQLite, or a catalog playlist ("pl.…", "pl.u-…", "pl.pm-…", "pl.rp-…")
// from Apple through the catalog cache, with every track.
func (s *PlaylistsService) Playlist(ctx context.Context, id string) (PlaylistDetail, error) {
	if isLibraryPlaylist(id) {
		return s.libraryDetail(ctx, id)
	}
	cp, err := s.e.Catalog.Playlist(ctx, id)
	if err != nil {
		return PlaylistDetail{}, s.e.Fail(err)
	}
	p := cp.Playlist
	a := p.Attributes
	tracks := s.playlistSongs(p.Relationships.Tracks.Data)
	d := PlaylistDetail{
		Playlist:        s.m.amPlaylist(p),
		Tracks:          tracks,
		DurationMs:      totalMs(tracks),
		Header:          s.m.header(ctx, a.Artwork.Template()),
		Kind:            a.PlaylistType,
		IsChart:         a.IsChart,
		Curator:         curatorDTO(cp.Curator),
		FeaturedArtists: s.m.amArtists(cp.FeaturedArtists),
		MoreByCurator:   s.m.amPlaylists(cp.MoreByCurator),
		URL:             a.URL,
	}
	d.Header.Motion = s.m.motionOf(a.EditorialVideo)
	d.Playlist.TrackCount = len(tracks)
	d.Playlist.Description = plainText(cmp.Or(a.Description.Text(), a.EditorialNotes.Text()))
	if d.Curator.Name == "" {
		d.Curator.Name = a.CuratorName
	}
	d.Playlist.LibraryID, d.InLibrary = s.e.members.lookup(ctx, kindPlaylist, d.Playlist.CatalogID)
	return d, nil
}

var curatorKinds = map[string]string{"apple-curators": "apple-curator", "curators": "curator", "social-profiles": "profile"}

func curatorDTO(c applemusic.Curator) PlaylistCurator {
	return PlaylistCurator{ID: c.ID, Kind: curatorKinds[c.Type], Name: c.Name, Handle: c.Handle, URL: c.URL}
}

// playlistSongs maps playlist tracks; music videos are listed but not
// playable (the audio pipeline plays songs only).
func (s *PlaylistsService) playlistSongs(in []applemusic.Song) []Song {
	out := s.m.amSongs(in)
	for i, t := range in {
		if t.Type != applemusic.TypeSongs && t.Type != applemusic.TypeLibrarySongs {
			out[i].Playable = false
		}
	}
	return out
}

func (s *PlaylistsService) libraryDetail(ctx context.Context, id string) (PlaylistDetail, error) {
	var (
		p   library.Playlist
		all []library.Song
	)
	for offset := 0; ; {
		pl, songs, err := s.e.Library.Playlist(ctx, id, library.ListOptions{Offset: offset, Limit: 1000})
		if err != nil {
			return PlaylistDetail{}, err
		}
		p = pl
		all = append(all, songs...)
		if len(songs) < 1000 {
			break
		}
		offset += len(songs)
	}
	flags, err := s.e.DB.RQ.PlaylistFlags(ctx, db.NullString(id))
	if err != nil {
		return PlaylistDetail{}, fmt.Errorf("playlist %s: %w", id, err)
	}
	tracks := s.m.libSongs(all)
	for i := range tracks {
		if tracks[i].ID == "" && tracks[i].CatalogID != "" {
			// A catalog song in the playlist (not a library song).
			tracks[i].ID, tracks[i].Source = tracks[i].CatalogID, SourceCatalog
		}
	}
	// A playlist Apple gave no cover has one made from its tracks
	// (library.FillCovers), which also shapes its backdrop and colours.
	tmpl := p.ArtworkTemplate
	d := PlaylistDetail{
		Playlist:        s.m.libPlaylist(p),
		Tracks:          tracks,
		DurationMs:      totalMs(tracks),
		InLibrary:       true,
		Header:          s.m.header(ctx, tmpl),
		Kind:            "library",
		Curator:         PlaylistCurator{Name: p.CuratorName},
		FeaturedArtists: []Artist{},
		MoreByCurator:   []Playlist{},
		CanDelete:       flags.CanDelete != 0,
		IsPublic:        flags.IsPublic != 0,
		Favorited:       flags.Favorited != 0,
		FolderID:        flags.FolderID,
	}
	d.Playlist.TrackCount = len(tracks)
	if p.CatalogID != "" && (d.IsPublic || !p.CanEdit) {
		d.URL = "https://music.apple.com/" + s.e.Storefront() + "/playlist/" + p.CatalogID
	}
	return d, nil
}

// Suggestions returns songs Apple suggests adding to library playlist id.
func (s *PlaylistsService) Suggestions(ctx context.Context, id string) ([]Song, error) {
	if err := requireLibraryPlaylist(id); err != nil {
		return nil, err
	}
	songs, err := s.e.API.SuggestedSongs(ctx, id)
	if err != nil {
		return nil, s.e.Fail(err)
	}
	return s.m.amSongs(songs), nil
}

// Tree returns the user's playlist folders and library playlists with
// their places in the tree, from SQLite.
func (s *PlaylistsService) Tree(ctx context.Context) (PlaylistTree, error) {
	folders, err := s.e.DB.RQ.ListPlaylistFolders(ctx)
	if err != nil {
		return PlaylistTree{}, err
	}
	rows, err := s.e.DB.RQ.LibraryPlaylistTree(ctx)
	if err != nil {
		return PlaylistTree{}, err
	}
	out := PlaylistTree{Folders: make([]PlaylistFolder, len(folders)), Playlists: make([]TreePlaylist, len(rows))}
	for i, f := range folders {
		out.Folders[i] = PlaylistFolder{ID: f.LibraryID, ParentID: f.ParentID, Name: f.Name, Position: int(f.Position)}
	}
	pls := make([]library.Playlist, len(rows))
	for i, r := range rows {
		pls[i] = library.Playlist{
			LibraryID: r.LibraryID.String, CatalogID: r.CatalogID.String, Name: r.Name, Description: r.Description,
			CuratorName: r.CuratorName, ArtworkTemplate: r.ArtworkTemplate, CanEdit: r.CanEdit != 0,
			TrackCount: int(r.TrackCount), LastModified: r.LastModified,
		}
		if r.DateAdded.Valid {
			pls[i].DateAdded = db.Time(r.DateAdded.Int64)
		}
	}
	// Playlists Apple gave no cover get one made from their tracks.
	if err := s.e.Library.FillCovers(ctx, pls); err != nil {
		return PlaylistTree{}, err
	}
	for i, r := range rows {
		out.Playlists[i] = TreePlaylist{
			Playlist: s.m.libPlaylist(pls[i]), FolderID: r.FolderID, Position: int(r.FolderPosition),
			CanDelete: r.CanDelete != 0, Favorited: r.Favorited != 0,
		}
	}
	return out, nil
}

// RecentTargets returns the library playlists Apple lists as recent "Add
// to playlist" targets, most recent first (library IDs).
func (s *PlaylistsService) RecentTargets(ctx context.Context) ([]string, error) {
	pls, err := s.e.API.RecentPlaylistTargets(ctx)
	if err != nil {
		return nil, s.e.Fail(err)
	}
	out := make([]string, 0, len(pls))
	for _, p := range pls {
		if p.Attributes.CanEdit {
			out = append(out, p.ID)
		}
	}
	return out, nil
}

// expand turns items into the track references Apple takes, and lists
// the catalog songs among them (adding them puts them in the library).
func (s *PlaylistsService) expand(ctx context.Context, items []PlaylistItem) (refs []applemusic.ResourceRef, catalogSongs []string, err error) {
	addSongs := func(songs []Song) {
		for _, so := range songs {
			if !so.Playable {
				continue
			}
			if so.Source == SourceLibrary {
				refs = append(refs, applemusic.ResourceRef{ID: so.ID, Type: applemusic.TypeLibrarySongs})
			} else {
				refs = append(refs, applemusic.ResourceRef{ID: so.ID, Type: applemusic.TypeSongs})
				catalogSongs = append(catalogSongs, so.ID)
			}
		}
	}
	for _, it := range items {
		switch {
		case it.ID == "":
			return nil, nil, fmt.Errorf("%w: empty %s id", ErrInvalidArgument, it.Kind)
		case it.Kind == kindSong && (it.Source == SourceLibrary || it.Source == SourceCatalog):
			addSongs([]Song{{ID: it.ID, Source: it.Source, Playable: true}})
		case it.Kind == kindAlbum && it.Source == SourceLibrary:
			_, songs, err := s.e.Library.Album(ctx, it.ID)
			if err != nil {
				return nil, nil, err
			}
			addSongs(s.m.libSongs(songs))
		case it.Kind == kindAlbum && it.Source == SourceCatalog:
			a, err := applemusic.AMPResource[applemusic.Album](ctx, s.e.API, s.e.Storefront(), applemusic.TypeAlbums, it.ID,
				applemusic.AMPQuery{Include: []string{"tracks"}})
			if err != nil {
				return nil, nil, s.e.Fail(err)
			}
			if err := applemusic.FollowNext(ctx, s.e.API, &a.Relationships.Tracks); err != nil {
				return nil, nil, s.e.Fail(err)
			}
			addSongs(s.playlistSongs(a.Relationships.Tracks.Data))
		case it.Kind == kindPlaylist:
			d, err := s.Playlist(ctx, it.ID)
			if err != nil {
				return nil, nil, err
			}
			addSongs(d.Tracks)
		default:
			return nil, nil, fmt.Errorf("%w: add %s %q from %q", ErrInvalidArgument, it.Kind, it.ID, it.Source)
		}
	}
	return refs, catalogSongs, nil
}

func cleanInput(in PlaylistInput) (applemusic.PlaylistAttributesInput, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return applemusic.PlaylistAttributesInput{}, ErrNameRequired
	}
	return applemusic.PlaylistAttributesInput{Name: name, Description: strings.TrimSpace(in.Description), IsPublic: in.IsPublic}, nil
}

// Create creates a library playlist with the songs of items (may be
// empty) in folder folderID ("" for the top level) and returns it.
// Catalog songs added to it also join the library.
func (s *PlaylistsService) Create(ctx context.Context, in PlaylistInput, items []PlaylistItem, folderID string) (Playlist, error) {
	attrs, err := cleanInput(in)
	if err != nil {
		return Playlist{}, err
	}
	refs, catalogSongs, err := s.expand(ctx, items)
	if err != nil {
		return Playlist{}, err
	}
	p, err := s.e.API.CreateLibraryPlaylist(ctx, attrs, refs)
	if err != nil {
		return Playlist{}, s.e.Fail(err)
	}
	if folderID != "" {
		if err := s.e.API.MovePlaylistToFolder(ctx, p.ID, folderID); err != nil {
			// The playlist exists at the top level; say so rather than fail.
			s.e.Report(err)
			s.e.Logger.Warn("playlists: move new playlist to folder", "folder", folderID, "err", err)
			folderID = ""
		}
	}
	entries := []applemusic.Song{}
	if len(refs) > 0 {
		if entries, err = s.e.API.LibraryPlaylistEntries(ctx, p.ID); err != nil {
			s.e.Report(err)
			s.e.Logger.Warn("playlists: read new playlist tracks", "err", err)
			entries = nil // stored on the next sync
		}
	}
	if err := s.e.Library.StorePlaylist(ctx, p, folderID, entries); err != nil {
		return Playlist{}, err
	}
	if folderID != "" {
		s.noteMove(p.ID, folderID)
	}
	s.e.members.markAdded(applemusic.TypeSongs, catalogSongs)
	s.changed(PlaylistsChanged{IDs: []string{p.ID}, Tree: true})
	out := s.m.amPlaylist(p)
	out.TrackCount = len(entries)
	return out, nil
}

// Update renames library playlist id and sets its description and
// visibility.
func (s *PlaylistsService) Update(ctx context.Context, id string, in PlaylistInput) error {
	if err := requireLibraryPlaylist(id); err != nil {
		return err
	}
	attrs, err := cleanInput(in)
	if err != nil {
		return err
	}
	defer s.lock(id)()
	if err := s.e.API.UpdateLibraryPlaylist(ctx, id, attrs); err != nil {
		return s.e.Fail(err)
	}
	modified := s.e.Now().UTC().Format("2006-01-02T15:04:05Z")
	if p, err := s.e.API.LibraryPlaylist(ctx, id); err == nil {
		modified = p.Attributes.LastModifiedDate
	}
	if err := s.e.Library.UpdatePlaylistDetails(ctx, id, attrs.Name, attrs.Description, attrs.IsPublic, modified); err != nil {
		return err
	}
	s.renameCover(ctx, id, attrs.Name)
	s.changed(PlaylistsChanged{IDs: []string{id}, Tree: true, Pins: true})
	go s.refreshPinsQuietly() // a pinned playlist shows its new name
	return nil
}

// Delete deletes library playlist id from the library.
func (s *PlaylistsService) Delete(ctx context.Context, id string) error {
	if err := requireLibraryPlaylist(id); err != nil {
		return err
	}
	defer s.lock(id)()
	if err := s.e.API.DeleteLibraryPlaylist(ctx, id); err != nil {
		return s.e.Fail(err)
	}
	if err := s.e.Library.DropPlaylist(ctx, id); err != nil {
		return err
	}
	pruneCoverFiles(ctx, s.e) // the playlist's chosen image goes with it
	s.changed(PlaylistsChanged{IDs: []string{id}, Tree: true})
	go s.refreshPinsQuietly() // a pinned playlist's pin goes with it
	return nil
}

// Add appends the songs of items to library playlist id and returns the
// updated playlist. Catalog songs also join the library.
func (s *PlaylistsService) Add(ctx context.Context, id string, items []PlaylistItem) (PlaylistDetail, error) {
	if err := requireLibraryPlaylist(id); err != nil {
		return PlaylistDetail{}, err
	}
	refs, catalogSongs, err := s.expand(ctx, items)
	if err != nil {
		return PlaylistDetail{}, err
	}
	if len(refs) == 0 {
		return PlaylistDetail{}, fmt.Errorf("%w: nothing playable to add", ErrInvalidArgument)
	}
	defer s.lock(id)()
	if err := s.e.API.AddPlaylistTracks(ctx, id, refs); err != nil {
		return PlaylistDetail{}, s.e.Fail(err)
	}
	s.e.members.markAdded(applemusic.TypeSongs, catalogSongs)
	entries, err := s.e.API.LibraryPlaylistEntries(ctx, id)
	if err != nil {
		return PlaylistDetail{}, s.e.Fail(err)
	}
	return s.stored(ctx, id, entries)
}

// MoveTrack moves the track at position from (whose ID is trackID, as the
// caller saw it) to position to in library playlist id. It fails with
// conflict when the playlist changed elsewhere since it was read.
func (s *PlaylistsService) MoveTrack(ctx context.Context, id string, from, to int, trackID string) (PlaylistDetail, error) {
	return s.edit(ctx, id, []int{from}, []string{trackID}, func(entries []applemusic.Song) ([]applemusic.Song, error) {
		return moveEntry(entries, from, to)
	})
}

// RemoveTracks removes the tracks at positions (whose IDs are trackIDs, as
// the caller saw them) from library playlist id; other copies of the same
// song stay. It fails with conflict when the playlist changed elsewhere.
func (s *PlaylistsService) RemoveTracks(ctx context.Context, id string, positions []int, trackIDs []string) (PlaylistDetail, error) {
	return s.edit(ctx, id, positions, trackIDs, func(entries []applemusic.Song) ([]applemusic.Song, error) {
		return removeEntries(entries, positions)
	})
}

// edit reads the playlist's tracks from Apple, checks that the positions
// still hold the songs the caller saw, applies change and replaces the
// track list on Apple and in SQLite.
func (s *PlaylistsService) edit(ctx context.Context, id string, positions []int, ids []string, change func([]applemusic.Song) ([]applemusic.Song, error)) (PlaylistDetail, error) {
	if err := requireLibraryPlaylist(id); err != nil {
		return PlaylistDetail{}, err
	}
	defer s.lock(id)()
	entries, err := s.e.API.LibraryPlaylistEntries(ctx, id)
	if err != nil {
		return PlaylistDetail{}, s.e.Fail(err)
	}
	if err := checkEntries(entries, positions, ids); err != nil {
		// Show the playlist as Apple has it now.
		if serr := s.storeTracks(ctx, id, entries); serr != nil {
			s.e.Logger.Warn("playlists: store current tracks", "err", serr)
		}
		return PlaylistDetail{}, err
	}
	next, err := change(entries)
	if err != nil {
		return PlaylistDetail{}, err
	}
	if err := s.e.API.ReplacePlaylistTracks(ctx, id, entryRefs(next)); err != nil {
		return PlaylistDetail{}, s.e.Fail(err)
	}
	return s.stored(ctx, id, next)
}

// stored records entries as library playlist id's tracks, emits the change
// and returns the playlist page.
func (s *PlaylistsService) stored(ctx context.Context, id string, entries []applemusic.Song) (PlaylistDetail, error) {
	if err := s.storeTracks(ctx, id, entries); err != nil {
		return PlaylistDetail{}, err
	}
	s.changed(PlaylistsChanged{IDs: []string{id}})
	return s.libraryDetail(ctx, id)
}

func (s *PlaylistsService) storeTracks(ctx context.Context, id string, entries []applemusic.Song) error {
	modified := ""
	if p, err := s.e.API.LibraryPlaylist(ctx, id); err == nil {
		modified = p.Attributes.LastModifiedDate
	} else {
		s.e.Report(err)
	}
	return s.e.Library.StorePlaylistTracks(ctx, id, modified, entries)
}

// ErrConflict reports an edit based on a playlist that changed elsewhere
// since it was read.
var ErrConflict = errors.New("the playlist changed since it was loaded")

// checkEntries verifies that each position holds the track with the
// matching ID (a library song ID, or a catalog song ID for catalog
// entries).
func checkEntries(entries []applemusic.Song, positions []int, ids []string) error {
	if len(positions) != len(ids) || len(positions) == 0 {
		return fmt.Errorf("%w: %d positions for %d track IDs", ErrInvalidArgument, len(positions), len(ids))
	}
	for i, p := range positions {
		if p < 0 || p >= len(entries) || (entries[p].ID != ids[i] && entries[p].CatalogID() != ids[i]) {
			return ErrConflict
		}
	}
	return nil
}

// moveEntry returns list with the element at from moved to position to.
func moveEntry[T any](list []T, from, to int) ([]T, error) {
	if from < 0 || from >= len(list) || to < 0 || to >= len(list) {
		return nil, fmt.Errorf("%w: move %d → %d in %d tracks", ErrInvalidArgument, from, to, len(list))
	}
	out := slices.Clone(list)
	v := out[from]
	out = slices.Delete(out, from, from+1)
	return slices.Insert(out, to, v), nil
}

// removeEntries returns list without the elements at positions.
func removeEntries[T any](list []T, positions []int) ([]T, error) {
	drop := make(map[int]bool, len(positions))
	for _, p := range positions {
		if p < 0 || p >= len(list) {
			return nil, fmt.Errorf("%w: remove position %d of %d", ErrInvalidArgument, p, len(list))
		}
		drop[p] = true
	}
	out := make([]T, 0, len(list)-len(drop))
	for i, v := range list {
		if !drop[i] {
			out = append(out, v)
		}
	}
	return out, nil
}

// entryRefs is the PUT body for a track list: each entry by its own ID
// and type (library songs as library-songs, catalog entries as songs).
func entryRefs(entries []applemusic.Song) []applemusic.ResourceRef {
	out := make([]applemusic.ResourceRef, len(entries))
	for i, e := range entries {
		out[i] = applemusic.ResourceRef{ID: e.ID, Type: e.Type}
	}
	return out
}

// MoveToFolder moves library playlist id into folder folderID ("" for the
// top level).
func (s *PlaylistsService) MoveToFolder(ctx context.Context, id, folderID string) error {
	if err := requireLibraryPlaylist(id); err != nil {
		return err
	}
	target := folderID
	if target == "" {
		target = applemusic.PlaylistsRoot
	}
	if err := s.e.API.MovePlaylistToFolder(ctx, id, target); err != nil {
		return s.e.Fail(err)
	}
	if err := s.e.Library.MovePlaylist(ctx, id, folderID); err != nil {
		return err
	}
	s.noteMove(id, folderID)
	s.changed(PlaylistsChanged{IDs: []string{id}, Tree: true})
	return nil
}

// CreateFolder creates a playlist folder at the top level.
func (s *PlaylistsService) CreateFolder(ctx context.Context, name string) (PlaylistFolder, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return PlaylistFolder{}, ErrNameRequired
	}
	f, err := s.e.API.CreatePlaylistFolder(ctx, name)
	if err != nil {
		return PlaylistFolder{}, s.e.Fail(err)
	}
	if err := s.e.Library.StoreFolder(ctx, f.ID, name, f.Attributes.DateAdded); err != nil {
		return PlaylistFolder{}, err
	}
	s.changed(PlaylistsChanged{Tree: true})
	row, err := s.e.DB.RQ.GetPlaylistFolder(ctx, f.ID)
	if err != nil {
		return PlaylistFolder{}, err
	}
	return PlaylistFolder{ID: row.LibraryID, ParentID: row.ParentID, Name: row.Name, Position: int(row.Position)}, nil
}

// RenameFolder renames playlist folder id.
func (s *PlaylistsService) RenameFolder(ctx context.Context, id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrNameRequired
	}
	if err := s.e.API.RenamePlaylistFolder(ctx, id, name); err != nil {
		return s.e.Fail(err)
	}
	if err := s.e.Library.StoreFolder(ctx, id, name, ""); err != nil {
		return err
	}
	s.changed(PlaylistsChanged{Tree: true})
	return nil
}

// ErrFolderNotEmpty refuses to delete a folder that still holds playlists
// or folders (what Apple does with them is unverified).
var ErrFolderNotEmpty = errors.New("the folder is not empty")

// DeleteFolder deletes empty playlist folder id.
func (s *PlaylistsService) DeleteFolder(ctx context.Context, id string) error {
	n, err := s.e.Library.FolderChildren(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("%w: %w (%d items)", ErrInvalidArgument, ErrFolderNotEmpty, n)
	}
	if err := s.e.API.DeletePlaylistFolder(ctx, id); err != nil {
		return s.e.Fail(err)
	}
	if err := s.e.Library.DropFolder(ctx, id); err != nil {
		return err
	}
	s.changed(PlaylistsChanged{Tree: true})
	return nil
}

// Pin kinds by library resource type.
var pinKinds = map[string]string{
	applemusic.TypeLibrarySongs:     kindSong,
	applemusic.TypeLibraryAlbums:    kindAlbum,
	applemusic.TypeLibraryArtists:   "artist",
	applemusic.TypeLibraryPlaylists: kindPlaylist,
}

// Pins returns the sidebar pins, from SQLite (refreshed from Apple after
// the first paint and after every pin edit).
func (s *PlaylistsService) Pins(ctx context.Context) ([]Pin, error) {
	rows, err := s.e.DB.RQ.OrderedPins(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Pin, len(rows))
	for i, r := range rows {
		p := Pin{Kind: r.Kind, ID: r.AppleID, Name: r.Name, Subtitle: r.Subtitle, Action: r.Action, Art: s.m.artOf(r.ArtworkTemplate), AlbumID: r.SongAlbumID}
		if r.Kind == kindPlaylist {
			if tmpl, ok := s.coverOfPin(ctx, r.AppleID); ok {
				p.Art = s.m.artOf(tmpl)
			}
		}
		if r.Kind == kindSong {
			if so, err := s.e.Library.Song(ctx, r.AppleID); err == nil {
				p.Song = s.m.libSong(so)
			} else {
				p.Song = Song{
					ID: r.AppleID, Source: SourceLibrary, LibraryID: r.AppleID, CatalogID: r.SongCatalogID,
					Name: r.Name, Artist: r.Subtitle, Playable: true, Art: p.Art,
				}
			}
		}
		out[i] = p
	}
	return out, nil
}

// RefreshPins reloads the pins from Apple.
func (s *PlaylistsService) RefreshPins(ctx context.Context) error {
	items, err := s.e.API.Pins(ctx)
	if err != nil {
		return s.e.Fail(err)
	}
	now := db.Millis(s.e.Now())
	err = s.e.DB.Tx(ctx, func(q *dbq.Queries) error {
		if err := q.ClearPins(ctx); err != nil {
			return err
		}
		pos := 0
		for _, it := range items {
			kind, ok := pinKinds[it.Type]
			if !ok {
				continue // music videos: not shown
			}
			a := it.Attributes
			tmpl := a.Artwork.Template()
			if cat := it.Relationships.Catalog.Data; tmpl == "" && len(cat) > 0 {
				tmpl = cat[0].Attributes.Artwork.Template()
			}
			action := it.Meta.LibraryPin.Action
			if action == "" {
				action = applemusic.PinPlay
			}
			if err := q.InsertPin(ctx, dbq.InsertPinParams{
				Kind: kind, AppleID: it.ID, Name: a.Name, Subtitle: a.ArtistName, ArtworkTemplate: tmpl,
				Position: int64(pos), PinnedAt: now, Action: action,
			}); err != nil {
				return err
			}
			pos++
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.changed(PlaylistsChanged{Pins: true})
	return nil
}

func (s *PlaylistsService) refreshPinsQuietly() {
	if s.e.Auth.Status().State != auth.StateSignedIn {
		return
	}
	if err := s.RefreshPins(context.Background()); err != nil {
		s.e.Logger.Warn("playlists: refresh pins", "err", err)
	}
}

func requirePinnable(id string) error {
	for _, p := range []string{"i.", "l.", "r.", "p."} {
		if strings.HasPrefix(id, p) {
			return nil
		}
	}
	return fmt.Errorf("%w: only library items can be pinned (%q)", ErrInvalidArgument, id)
}

// Pin pins library item id (a song, album, artist or playlist; at most
// six pins). It fails with pins_full when six are pinned.
func (s *PlaylistsService) Pin(ctx context.Context, id string) error {
	if err := requirePinnable(id); err != nil {
		return err
	}
	if err := s.e.API.Pin(ctx, id); err != nil {
		return s.e.Fail(err)
	}
	return s.RefreshPins(ctx)
}

// Unpin removes the pin of library item id.
func (s *PlaylistsService) Unpin(ctx context.Context, id string) error {
	if err := s.e.API.Unpin(ctx, id); err != nil && !errors.Is(err, applemusic.ErrNotFound) {
		return s.e.Fail(err)
	}
	return s.RefreshPins(ctx)
}

// MovePin moves pin id after pin afterID ("" moves it to the top).
func (s *PlaylistsService) MovePin(ctx context.Context, id, afterID string) error {
	if err := s.e.API.MovePin(ctx, id, afterID); err != nil {
		return s.e.Fail(err)
	}
	return s.RefreshPins(ctx)
}

// SetPinAction sets what activating pin id does: "play", "shuffle" or
// "drillIn".
func (s *PlaylistsService) SetPinAction(ctx context.Context, id, action string) error {
	switch action {
	case applemusic.PinPlay, applemusic.PinShuffle, applemusic.PinDrillIn:
	default:
		return fmt.Errorf("%w: pin action %q", ErrInvalidArgument, action)
	}
	if err := s.e.API.SetPinAction(ctx, id, action); err != nil {
		return s.e.Fail(err)
	}
	return s.RefreshPins(ctx)
}

// PinTracks returns the songs a pin plays: the song, or the library
// album's, playlist's or artist's songs, from SQLite.
func (s *PlaylistsService) PinTracks(ctx context.Context, id string) ([]Song, error) {
	switch {
	case strings.HasPrefix(id, "i."):
		so, err := s.e.Library.Song(ctx, id)
		if err != nil {
			return nil, err
		}
		return []Song{s.m.libSong(so)}, nil
	case strings.HasPrefix(id, "l."):
		_, songs, err := s.e.Library.Album(ctx, id)
		return s.m.libSongs(songs), err
	case strings.HasPrefix(id, "r."):
		_, _, songs, err := s.e.Library.Artist(ctx, id)
		return s.m.libSongs(songs), err
	case isLibraryPlaylist(id):
		d, err := s.libraryDetail(ctx, id)
		return d.Tracks, err
	}
	return nil, fmt.Errorf("%w: pin %q", ErrInvalidArgument, id)
}

// noteMove tells the syncer about a folder move made here, so a sync
// during Apple's folder-listing lag does not put the playlist back.
func (s *PlaylistsService) noteMove(id, folderID string) {
	if s.e.Syncer != nil {
		s.e.Syncer.NoteMove(id, folderID)
	}
}
