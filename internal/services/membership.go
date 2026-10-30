package services

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/db"
)

// Library item kinds (library_items.kind).
const (
	kindSong     = "song"
	kindAlbum    = "album"
	kindArtist   = "artist"
	kindPlaylist = "playlist"
)

// addKinds maps catalog resource types to library item kinds.
var addKinds = map[string]string{
	applemusic.TypeSongs:     kindSong,
	applemusic.TypeAlbums:    kindAlbum,
	applemusic.TypeArtists:   kindArtist,
	applemusic.TypePlaylists: kindPlaylist,
}

// membershipQueries find the library copy of a catalog item.
var membershipQueries = map[string]string{
	kindSong: `SELECT t.library_id FROM tracks t JOIN library_items li ON li.kind = 'song' AND li.ref = t.id
		WHERE t.catalog_id = ? AND t.library_id IS NOT NULL LIMIT 1`,
	kindAlbum: `SELECT a.library_id FROM albums a JOIN library_items li ON li.kind = 'album' AND li.ref = a.id
		WHERE a.catalog_id = ? AND a.library_id IS NOT NULL LIMIT 1`,
	kindArtist: `SELECT r.library_id FROM artists r JOIN library_items li ON li.kind = 'artist' AND li.ref = r.id
		WHERE r.catalog_id = ? AND r.library_id IS NOT NULL LIMIT 1`,
	kindPlaylist: `SELECT p.library_id FROM playlists p JOIN library_items li ON li.kind = 'playlist' AND li.ref = p.id
		WHERE p.catalog_id = ? AND p.library_id IS NOT NULL LIMIT 1`,
}

// How long library states are reused: Apple's answers for stateTTL, and
// the outcome of the user's own change for stateHold (Apple's catalog view
// lags a removal by tens of seconds).
const (
	stateTTL  = 2 * time.Minute
	stateHold = 5 * time.Minute
)

type stateEntry struct {
	st    applemusic.LibraryState
	until time.Time
}

// membership answers whether catalog items are in the user's library:
// from the synced library rows, and for items added this session from a
// record of successful adds, which bridges the time until a sync stores
// them. It also caches the library states (membership, library ID,
// rating) Apple reported for items the UI shows, and the outcome of the
// user's changes.
type membership struct {
	db  *db.DB
	now func() time.Time

	mu     sync.Mutex
	added  map[string]time.Time // kind + ":" + catalog ID → when the add succeeded
	states map[applemusic.ItemKey]stateEntry
}

func newMembership(d *db.DB) *membership {
	return &membership{db: d, now: time.Now, added: map[string]time.Time{}, states: map[applemusic.ItemKey]stateEntry{}}
}

// lookup returns the library ID of catalog item id of kind and whether it
// is in the library (libraryID is "" for an add not yet synced).
func (m *membership) lookup(ctx context.Context, kind, catalogID string) (libraryID string, ok bool) {
	q, known := membershipQueries[kind]
	if catalogID == "" || !known {
		return "", false
	}
	var lid string
	err := m.db.R.QueryRowContext(ctx, q, catalogID).Scan(&lid)
	if err == nil {
		return lid, true
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", false
	}
	m.mu.Lock()
	_, ok = m.added[kind+":"+catalogID]
	m.mu.Unlock()
	return "", ok
}

// markAdded records catalog items of type typ as added.
func (m *membership) markAdded(typ string, ids []string) {
	kind, ok := addKinds[typ]
	if !ok {
		return
	}
	now := m.now()
	m.mu.Lock()
	for _, id := range ids {
		m.added[kind+":"+id] = now
	}
	m.mu.Unlock()
}

// unmarkAdded forgets recorded adds of catalog items of type typ (they
// were removed again).
func (m *membership) unmarkAdded(typ string, ids []string) {
	kind := addKinds[typ]
	m.mu.Lock()
	for _, id := range ids {
		delete(m.added, kind+":"+id)
	}
	m.mu.Unlock()
}

// synced forgets adds recorded before a successful sync that started at
// start: the library rows answer for them now.
func (m *membership) synced(start time.Time) {
	m.mu.Lock()
	for k, t := range m.added {
		if t.Before(start) {
			delete(m.added, k)
		}
	}
	m.mu.Unlock()
}

// state returns the cached library state of k, if still fresh.
func (m *membership) state(k applemusic.ItemKey) (applemusic.LibraryState, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.states[k]
	if !ok || !m.now().Before(e.until) {
		return applemusic.LibraryState{}, false
	}
	return e.st, true
}

// remember caches Apple's answer for k for stateTTL, unless the user's
// own change to k is still held.
func (m *membership) remember(k applemusic.ItemKey, st applemusic.LibraryState) {
	now := m.now()
	m.mu.Lock()
	if e, ok := m.states[k]; !ok || !now.Before(e.until) {
		m.states[k] = stateEntry{st, now.Add(stateTTL)}
	}
	m.mu.Unlock()
}

// hold caches the outcome of the user's change to k for stateHold, over
// anything Apple reports meanwhile.
func (m *membership) hold(k applemusic.ItemKey, st applemusic.LibraryState) {
	m.mu.Lock()
	m.states[k] = stateEntry{st, m.now().Add(stateHold)}
	m.mu.Unlock()
}

// clear forgets every recorded add and cached state (sign-out).
func (m *membership) clear() {
	m.mu.Lock()
	clear(m.added)
	clear(m.states)
	m.mu.Unlock()
}
