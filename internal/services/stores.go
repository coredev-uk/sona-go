package services

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/library"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
)

// historyRetention bounds the local play history.
const historyRetention = 365 * 24 * time.Hour

// AccountStore is auth.AccountStore over the accounts table. The
// storefront's display name is not stored; it returns with the next
// validation.
type AccountStore struct{ DB *db.DB }

var _ auth.AccountStore = AccountStore{}

func (s AccountStore) Load(ctx context.Context) (auth.Account, error) {
	a, err := s.DB.RQ.GetAccount(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.Account{}, auth.ErrNoAccount
	}
	if err != nil {
		return auth.Account{}, err
	}
	out := auth.Account{StorefrontID: a.Storefront, Language: a.Language, Restricted: a.Restricted != 0}
	if a.MutObtainedAt.Valid {
		out.MUTObtainedAt = db.Time(a.MutObtainedAt.Int64)
	}
	if a.MutLastValidatedAt.Valid {
		out.MUTLastValidatedAt = db.Time(a.MutLastValidatedAt.Int64)
	}
	return out, nil
}

func (s AccountStore) Save(ctx context.Context, a auth.Account) error {
	return s.DB.Q.UpsertAccount(ctx, dbq.UpsertAccountParams{
		Storefront:         a.StorefrontID,
		Language:           a.Language,
		Restricted:         db.Bool(a.Restricted),
		MutObtainedAt:      db.NullMillis(a.MUTObtainedAt),
		MutLastValidatedAt: db.NullMillis(a.MUTLastValidatedAt),
		UpdatedAt:          db.Millis(time.Now()),
	})
}

func (s AccountStore) Delete(ctx context.Context) error {
	_, err := s.DB.W.ExecContext(ctx, `DELETE FROM accounts`)
	return err
}

// History records finished listens into playback_history; pass Record as
// coordinator.Options.OnPlayed. It also follows the listen in progress
// (observe, a coordinator activity observer), which playback_history only
// gets when it ends, so the History page can show it as played here.
type History struct {
	DB     *db.DB
	Logger *slog.Logger
	// Context returns the current queue label (coordinator Snapshot
	// Context); nil records none.
	Context func() string
	// Private reports whether a listen that started at start is private
	// (privatemode.Mode.Since): it is not recorded or shown as in
	// progress. nil means never.
	Private func(start time.Time) bool

	mu      sync.Mutex
	current listen // the latest listen started here
}

// listen is a listen started here.
type listen struct {
	item     coordinator.Item
	at       time.Time // when it began
	recorded bool      // Record stored it
}

// Record implements coordinator.Options.OnPlayed. Live radio and episodes
// ("ra.…") are not songs and are not recorded (Apple keeps the station
// history, RadioService.Recent), nor are private listens.
func (h *History) Record(item coordinator.Item, playedFor time.Duration, _ bool) {
	if isStationID(item.ID) {
		return
	}
	now := time.Now()
	start := now.Add(-playedFor)
	h.mu.Lock()
	// The listen in progress knows when it began (pauses make that earlier
	// than its listening time says), unless it is a new listen of the same
	// song (repeat one) that began as this one ended.
	if c := h.current; c.item.ID == item.ID && !c.at.After(now.Add(-playedFor/2)) {
		start = c.at
	}
	h.mu.Unlock()
	if h.private(start) {
		return
	}
	p := dbq.InsertHistoryParams{
		PlayedAt:        db.Millis(now.Add(-playedFor)),
		ItemID:          item.ID,
		Name:            item.Title,
		ArtistName:      item.Artist,
		AlbumName:       item.Album,
		ArtworkTemplate: item.Artwork,
		DurationMs:      item.Duration.Milliseconds(),
		PlayedMs:        playedFor.Milliseconds(),
	}
	if strings.HasPrefix(item.ID, "i.") {
		p.LibraryID = item.ID
	} else {
		p.CatalogID = item.ID
		if t, err := h.DB.RQ.GetTrackByCatalogID(context.Background(), db.NullString(item.ID)); err == nil && t.LibraryID.Valid {
			p.LibraryID = t.LibraryID.String
		}
	}
	if h.Context != nil {
		p.Source = h.Context()
	}
	if _, err := h.DB.Q.InsertHistory(context.Background(), p); err != nil {
		h.Logger.Warn("history: record play", "id", item.ID, "err", err)
		return
	}
	h.mu.Lock()
	// This is the listen in progress, unless a new listen of the same song
	// (repeat one) began as this one ended.
	if c := &h.current; c.item.ID == item.ID && !c.at.After(now.Add(-playedFor/2)) {
		c.recorded = true
	}
	h.mu.Unlock()
}

// observe notes each listen that starts here; a resume continues its
// listen.
func (h *History) observe(a coordinator.Activity) {
	if a.Kind != coordinator.ActivityStart || isStationID(a.Item.ID) {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if a.Position > 0 && a.Item.ID == h.current.item.ID && !h.current.recorded {
		return
	}
	h.current = listen{item: a.Item, at: a.At.Add(-a.Position)}
}

// started returns the latest listen started here while playback_history
// does not hold it yet (see library.MergeHistory).
func (h *History) started(ctx context.Context) []library.Play {
	h.mu.Lock()
	c := h.current
	h.mu.Unlock()
	if c.item.ID == "" || c.recorded || h.private(c.at) {
		return nil
	}
	p := library.Play{
		PlayedAt: c.at, Name: c.item.Title, Artist: c.item.Artist, Album: c.item.Album,
		ArtworkTemplate: c.item.Artwork, Duration: c.item.Duration,
	}
	// Apple's list names the song by its catalog or library ID.
	if strings.HasPrefix(c.item.ID, "i.") {
		p.LibraryID = c.item.ID
		if t, err := h.DB.RQ.GetTrackByLibraryID(ctx, db.NullString(c.item.ID)); err == nil {
			p.CatalogID = t.CatalogID.String
		}
	} else {
		p.CatalogID = c.item.ID
		if t, err := h.DB.RQ.GetTrackByCatalogID(ctx, db.NullString(c.item.ID)); err == nil {
			p.LibraryID = t.LibraryID.String
		}
	}
	return []library.Play{p}
}

func (h *History) private(start time.Time) bool {
	return h.Private != nil && h.Private(start)
}

// trim deletes history older than historyRetention.
func (h *History) trim(ctx context.Context) {
	n, err := h.DB.Q.TrimHistory(ctx, db.Millis(time.Now().Add(-historyRetention)))
	if err != nil {
		h.Logger.Warn("history: trim", "err", err)
	} else if n > 0 {
		h.Logger.Info("history: trimmed", "rows", n)
	}
}
