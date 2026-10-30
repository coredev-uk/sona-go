package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

// Playlist folders and local playlist edits. The playlists domain stores
// the folder tree after the playlist list; the Store*/Drop* methods keep
// SQLite consistent with an edit just made on Apple, without a sync.

// TagFavorited marks the Favourite Songs playlist.
const TagFavorited = "favorited"

// moveGrace is how long a folder move made here outranks the tree Apple
// lists: Apple's folder listings lag a move by an unknown time.
const moveGrace = 10 * time.Minute

// pendingMove is a folder move made on this machine, not yet seen in
// Apple's folder listings.
type pendingMove struct {
	folder string // "" for the top level
	at     time.Time
}

// NoteMove records that playlist id was just moved into folder folderID
// ("" for the top level) on Apple, so a sync within moveGrace keeps it
// there even if Apple's listing still shows the old place.
func (s *Syncer) NoteMove(id, folderID string) {
	s.mu.Lock()
	s.moves[id] = pendingMove{folder: folderID, at: s.now()}
	s.mu.Unlock()
}

// unconfirmedMoves returns the moves still within moveGrace that the
// listed tree (playlist ID → parent) does not show yet, and forgets the
// confirmed and expired ones.
func (s *Syncer) unconfirmedMoves(listed map[string]string) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]string{}
	for id, m := range s.moves {
		parent, seen := listed[id]
		if (seen && parent == m.folder) || s.now().Sub(m.at) > moveGrace {
			delete(s.moves, id)
			continue
		}
		out[id] = m.folder
	}
	return out
}

// syncPlaylistTree stores the folder tree and every playlist's place in it
// as part of playlists run gen; folders the run did not see are removed.
// A playlist moved here recently keeps its new folder until Apple lists it
// there (see NoteMove).
func (s *Syncer) syncPlaylistTree(ctx context.Context, gen int64) error {
	entries, err := s.api.PlaylistTree(ctx)
	if err != nil {
		return fmt.Errorf("playlist folders: %w", err)
	}
	listed := map[string]string{}
	for i, e := range entries {
		if e.ParentID == applemusic.PlaylistsRoot {
			entries[i].ParentID = ""
		}
		if e.Type == applemusic.TypeLibraryPlaylists {
			listed[e.ID] = entries[i].ParentID
		}
	}
	keep := s.unconfirmedMoves(listed)
	return s.db.Tx(ctx, func(q *dbq.Queries) error {
		if err := q.ResetPlaylistPlacement(ctx); err != nil {
			return err
		}
		for _, e := range entries {
			switch e.Type {
			case applemusic.TypeLibraryPlaylistFolders:
				err = q.UpsertPlaylistFolder(ctx, dbq.UpsertPlaylistFolderParams{
					LibraryID: e.ID, ParentID: e.ParentID, Name: e.Name, Position: int64(e.Position),
					DateAdded: parseDate(e.DateAdded), Generation: gen,
				})
			case applemusic.TypeLibraryPlaylists:
				err = q.PlacePlaylist(ctx, dbq.PlacePlaylistParams{
					FolderID: e.ParentID, FolderPosition: int64(e.Position),
					Favorited: db.Bool(slices.Contains(e.Tags, TagFavorited)), CanDelete: db.Bool(e.CanDelete), LibraryID: db.NullString(e.ID),
				})
			}
			if err != nil {
				return fmt.Errorf("playlist tree %s: %w", e.ID, err)
			}
		}
		for id, folder := range keep {
			pos, err := q.NextFolderPosition(ctx, folder)
			if err != nil {
				return err
			}
			if err := q.SetPlaylistFolder(ctx, dbq.SetPlaylistFolderParams{FolderID: folder, FolderPosition: pos, LibraryID: db.NullString(id)}); err != nil {
				return err
			}
		}
		n, err := q.DeleteStalePlaylistFolders(ctx, gen)
		if n > 0 {
			s.log.Info("library sync removed playlist folders", "removed", n)
		}
		return err
	})
}

// storePlaylistTracks replaces the stored tracks of playlist row plID
// (library ID libID) with tracks, in order, and records lastModified as
// the state they reflect.
func storePlaylistTracks(ctx context.Context, q *dbq.Queries, plID int64, libID, lastModified string, tracks []applemusic.Song, now int64) ([]int64, error) {
	if err := q.DeletePlaylistTracks(ctx, plID); err != nil {
		return nil, err
	}
	ids := make([]int64, len(tracks))
	for i, t := range tracks {
		var id int64
		var err error
		if t.Type == applemusic.TypeSongs {
			id, err = q.UpsertCatalogTrack(ctx, catalogTrackParams(t, now))
		} else {
			id, err = q.UpsertLibraryTrack(ctx, libraryTrackParams(t, now))
		}
		if err != nil {
			return nil, fmt.Errorf("playlist %s track %s: %w", libID, t.ID, err)
		}
		if err := q.InsertPlaylistTrack(ctx, dbq.InsertPlaylistTrackParams{PlaylistID: plID, Position: int64(i), TrackID: id}); err != nil {
			return nil, err
		}
		ids[i] = id
	}
	return ids, q.SetPlaylistTracksSynced(ctx, dbq.SetPlaylistTracksSyncedParams{TracksModified: lastModified, TrackCount: int64(len(tracks)), ID: plID})
}

// domainGeneration is the generation an item stored outside a sync should
// carry so the domain's current (or next completed) run keeps it.
func domainGeneration(ctx context.Context, q *dbq.Queries, d Domain) (int64, error) {
	st, err := q.GetSyncState(ctx, string(d))
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return st.Generation, err
}

// StorePlaylist stores library playlist p (just created or edited on
// Apple) as a library item, placed at the end of folder folderID ("" for
// the root) when it is new, and its tracks when tracks is non-nil. Library
// songs among the tracks that the library does not list yet (catalog songs
// Apple added to the library with the playlist edit) become library songs.
func (l *Library) StorePlaylist(ctx context.Context, p applemusic.Playlist, folderID string, tracks []applemusic.Song) error {
	a := p.Attributes
	now := db.Millis(time.Now())
	return l.db.Tx(ctx, func(q *dbq.Queries) error {
		_, existed := q.GetPlaylistByLibraryID(ctx, db.NullString(p.ID))
		id, err := q.UpsertLibraryPlaylist(ctx, dbq.UpsertLibraryPlaylistParams{
			LibraryID: db.NullString(p.ID), CatalogID: db.NullString(p.CatalogID()),
			Name: a.Name, Description: a.Description.Text(), CuratorName: a.CuratorName,
			ArtworkTemplate: a.Artwork.Template(), CanEdit: db.Bool(a.CanEdit), IsPublic: db.Bool(a.IsPublic),
			LastModified: a.LastModifiedDate, UpdatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("library: store playlist %s: %w", p.ID, err)
		}
		if err := q.SetPlaylistCanDelete(ctx, dbq.SetPlaylistCanDeleteParams{CanDelete: db.Bool(a.CanDelete), LibraryID: db.NullString(p.ID)}); err != nil {
			return err
		}
		if errors.Is(existed, sql.ErrNoRows) {
			pos, err := q.NextFolderPosition(ctx, folderID)
			if err != nil {
				return err
			}
			if err := q.SetPlaylistFolder(ctx, dbq.SetPlaylistFolderParams{FolderID: folderID, FolderPosition: pos, LibraryID: db.NullString(p.ID)}); err != nil {
				return err
			}
		}
		gen, err := domainGeneration(ctx, q, DomainPlaylists)
		if err != nil {
			return err
		}
		if err := q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: "playlist", Ref: id, DateAdded: parseDate(a.DateAdded), Generation: gen}); err != nil {
			return err
		}
		if tracks == nil {
			return nil
		}
		return l.storeTracks(ctx, q, id, p.ID, a.LastModifiedDate, tracks, now)
	})
}

// StorePlaylistTracks replaces the stored tracks of library playlist id
// with tracks as Apple now holds them (after an add, reorder or removal);
// lastModified is the playlist's new modification stamp ("" keeps the
// tracks marked for refresh by the next sync).
func (l *Library) StorePlaylistTracks(ctx context.Context, id, lastModified string, tracks []applemusic.Song) error {
	now := db.Millis(time.Now())
	return l.db.Tx(ctx, func(q *dbq.Queries) error {
		row, err := q.GetPlaylistByLibraryID(ctx, db.NullString(id))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if lastModified != "" {
			if err := q.UpdatePlaylistDetails(ctx, dbq.UpdatePlaylistDetailsParams{
				Name: row.Name, Description: row.Description, IsPublic: row.IsPublic,
				LastModified: lastModified, UpdatedAt: now, LibraryID: row.LibraryID,
			}); err != nil {
				return err
			}
		}
		return l.storeTracks(ctx, q, row.ID, id, lastModified, tracks, now)
	})
}

func (l *Library) storeTracks(ctx context.Context, q *dbq.Queries, plID int64, libID, lastModified string, tracks []applemusic.Song, now int64) error {
	ids, err := storePlaylistTracks(ctx, q, plID, libID, lastModified, tracks, now)
	if err != nil {
		return err
	}
	gen, err := domainGeneration(ctx, q, DomainSongs)
	if err != nil {
		return err
	}
	for i, t := range tracks {
		if t.Type != applemusic.TypeLibrarySongs {
			continue
		}
		if _, err := q.AddLibrarySongIfMissing(ctx, dbq.AddLibrarySongIfMissingParams{Ref: ids[i], DateAdded: sql.NullInt64{Int64: now, Valid: true}, Generation: gen}); err != nil {
			return err
		}
	}
	return nil
}

// UpdatePlaylistDetails stores a library playlist's edited name,
// description and visibility.
func (l *Library) UpdatePlaylistDetails(ctx context.Context, id, name, description string, public bool, lastModified string) error {
	return l.db.Q.UpdatePlaylistDetails(ctx, dbq.UpdatePlaylistDetailsParams{
		Name: name, Description: description, IsPublic: db.Bool(public),
		LastModified: lastModified, UpdatedAt: db.Millis(time.Now()), LibraryID: db.NullString(id),
	})
}

// DropPlaylist removes deleted library playlist id and its tracks.
func (l *Library) DropPlaylist(ctx context.Context, id string) error {
	return l.db.Tx(ctx, func(q *dbq.Queries) error {
		row, err := q.GetPlaylistByLibraryID(ctx, db.NullString(id))
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := q.DropPlaylistItem(ctx, row.ID); err != nil {
			return err
		}
		if err := q.DeletePlaylistTracks(ctx, row.ID); err != nil {
			return err
		}
		if _, err := q.DeleteOrphanLibraryPlaylists(ctx); err != nil {
			return err
		}
		_, err = q.DeleteOrphanLibraryTracks(ctx)
		return err
	})
}

// MovePlaylist stores library playlist id at the end of folder folderID
// ("" for the root).
func (l *Library) MovePlaylist(ctx context.Context, id, folderID string) error {
	return l.db.Tx(ctx, func(q *dbq.Queries) error {
		pos, err := q.NextFolderPosition(ctx, folderID)
		if err != nil {
			return err
		}
		return q.SetPlaylistFolder(ctx, dbq.SetPlaylistFolderParams{FolderID: folderID, FolderPosition: pos, LibraryID: db.NullString(id)})
	})
}

// StoreFolder stores a folder just created (at the end of the root) or
// renamed on Apple.
func (l *Library) StoreFolder(ctx context.Context, id, name, dateAdded string) error {
	return l.db.Tx(ctx, func(q *dbq.Queries) error {
		_, err := q.GetPlaylistFolder(ctx, id)
		switch {
		case err == nil:
			return q.RenamePlaylistFolder(ctx, dbq.RenamePlaylistFolderParams{Name: name, LibraryID: id})
		case !errors.Is(err, sql.ErrNoRows):
			return err
		}
		pos, err := q.NextFolderPosition(ctx, "")
		if err != nil {
			return err
		}
		gen, err := domainGeneration(ctx, q, DomainPlaylists)
		if err != nil {
			return err
		}
		return q.UpsertPlaylistFolder(ctx, dbq.UpsertPlaylistFolderParams{
			LibraryID: id, Name: name, Position: pos, DateAdded: parseDate(dateAdded), Generation: gen,
		})
	})
}

// FolderChildren counts the playlists and folders directly inside folder
// id.
func (l *Library) FolderChildren(ctx context.Context, id string) (int, error) {
	n, err := l.db.RQ.CountFolderChildren(ctx, id)
	return int(n), err
}

// DropFolder removes deleted folder id.
func (l *Library) DropFolder(ctx context.Context, id string) error {
	return l.db.Q.DeletePlaylistFolderRow(ctx, id)
}
