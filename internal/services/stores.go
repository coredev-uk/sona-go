package services

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
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
// coordinator.Options.OnPlayed.
type History struct {
	DB     *db.DB
	Logger *slog.Logger
	// Context returns the current queue label (coordinator Snapshot
	// Context); nil records none.
	Context func() string
}

// Record implements coordinator.Options.OnPlayed. Live radio and episodes
// ("ra.…") are not songs and are not recorded (Apple keeps the station
// history, RadioService.Recent).
func (h *History) Record(item coordinator.Item, playedFor time.Duration, _ bool) {
	if isStationID(item.ID) {
		return
	}
	p := dbq.InsertHistoryParams{
		PlayedAt:        db.Millis(time.Now().Add(-playedFor)),
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
	}
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
