package lyrics

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

// Cache lifetimes: lyrics rarely change; a provider without lyrics may
// get them, so "none" is asked again sooner; a result missing a part (a
// translation that failed to load) is retried within the hour.
const (
	FoundTTL    = 30 * 24 * time.Hour
	NotFoundTTL = 3 * 24 * time.Hour
	PartialTTL  = time.Hour
)

// cacheEntry is one provider's cached answer.
type cacheEntry struct {
	lyrics Lyrics
	found  bool
	fresh  bool // not expired
}

// cached returns the provider's cached answer for the track; ok is false
// when there is none (or it can't be read).
func (r *Resolver) cached(ctx context.Context, p Provider, track, lang string) (cacheEntry, bool) {
	row, err := r.db.RQ.GetLyricsCache(ctx, dbq.GetLyricsCacheParams{Provider: p.ID(), Track: track, Language: lang})
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			r.log.Warn("lyrics cache: read", "provider", p.ID(), "err", err)
		}
		return cacheEntry{}, false
	}
	e := cacheEntry{found: row.Found != 0, fresh: r.now().UnixMilli() < row.ExpiresAt}
	if e.found {
		if err := json.Unmarshal(row.Body, &e.lyrics); err != nil {
			r.log.Warn("lyrics cache: undecodable entry", "provider", p.ID(), "err", err)
			return cacheEntry{}, false
		}
	}
	return e, true
}

// store caches the provider's answer: l, or "none" when l is nil.
func (r *Resolver) store(ctx context.Context, p Provider, track, lang string, l *Lyrics) {
	now := r.now()
	arg := dbq.PutLyricsCacheParams{Provider: p.ID(), Track: track, Language: lang, FetchedAt: db.Millis(now), ExpiresAt: db.Millis(now.Add(NotFoundTTL)), Body: []byte{}}
	if l != nil {
		b, err := json.Marshal(l)
		if err != nil {
			r.log.Warn("lyrics cache: encode", "provider", p.ID(), "err", err)
			return
		}
		ttl := FoundTTL
		if l.partial {
			ttl = PartialTTL
		}
		arg.Found, arg.Body, arg.ExpiresAt = 1, b, db.Millis(now.Add(ttl))
	}
	if err := r.db.Q.PutLyricsCache(ctx, arg); err != nil {
		r.log.Warn("lyrics cache: write", "provider", p.ID(), "err", err)
	}
}

// Prune deletes expired cache entries and returns how many it removed.
func Prune(ctx context.Context, d *db.DB, now time.Time) (int64, error) {
	return d.Q.DeleteExpiredLyricsCache(ctx, db.Millis(now))
}

// CacheSize reports the cached answers and their bytes.
func CacheSize(ctx context.Context, d *db.DB) (entries, bytes int64, err error) {
	row, err := d.RQ.LyricsCacheSize(ctx)
	return row.Entries, row.Bytes, err
}

// ClearCache deletes every cached answer and returns how many it removed.
func ClearCache(ctx context.Context, d *db.DB) (int64, error) {
	return d.Q.ClearLyricsCache(ctx)
}
