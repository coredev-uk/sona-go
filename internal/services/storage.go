package services

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/lyrics"
	"github.com/coredev-uk/sona-go/internal/playback"
)

// Storage upkeep timing. Upkeep starts upkeepFirst after the first paint
// (never during cold start) and checks every upkeepEvery: it prunes the
// catalogue and lyrics caches every pruneEvery and, when the app is idle (no library
// sync, nothing playing or loading), optimises the database (VACUUM when
// enough of it is free pages) every optimizeEvery.
const (
	upkeepFirst   = 2 * time.Minute
	upkeepEvery   = 10 * time.Minute
	pruneEvery    = 6 * time.Hour
	optimizeEvery = 24 * time.Hour
)

// StorageService is Settings → Storage: cache sizes and limits, clearing
// and compacting, and the scheduled upkeep of the caches and the database.
// It also removes the user's artwork on sign-out.
type StorageService struct {
	e *Env

	ctx    context.Context // cancelled by close
	cancel context.CancelFunc
	done   chan struct{} // closed when the upkeep loop ends; nil until it starts

	mu        sync.Mutex // serialises upkeep and Compact
	pruned    time.Time
	optimized time.Time
}

// newStorageService applies the artwork cache limit when it changes (a
// lower one evicts before the settings save returns), starts upkeep after the first paint and registers the sign-out step that
// removes the user's artwork. The limit itself is set by main when it
// builds the cache.
func newStorageService(e *Env) *StorageService {
	ctx, cancel := context.WithCancel(context.Background())
	s := &StorageService{e: e, ctx: ctx, cancel: cancel}
	e.Settings.OnChange(func(prev, next config.Settings) {
		if prev.Storage.ArtCacheMiB != next.Storage.ArtCacheMiB {
			e.Art.SetMaxBytes(mib(next.Storage.ArtCacheMiB))
		}
	})
	e.OnNetwork(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.ctx.Err() == nil {
			s.done = make(chan struct{})
			e.Go("storage upkeep", s.loop)
		}
	})
	e.OnSignOut("user artwork", func(ctx context.Context) error { return wipeUserArtwork(ctx, e.DB, e.Art, e.Logger) })
	e.OnClose(s.close)
	return s
}

func mib(n int) int64 { return int64(n) << 20 }

func (s *StorageService) loop() {
	defer close(s.done)
	t := time.NewTimer(upkeepFirst)
	defer t.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
		}
		s.upkeep()
		t.Reset(upkeepEvery)
	}
}

// upkeep runs whatever upkeep is due (see the timing constants).
func (s *StorageService) upkeep() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.e.Now()
	if now.Sub(s.pruned) >= pruneEvery {
		s.pruned = now
		if n, err := s.prune(s.ctx); err != nil {
			s.e.Logger.Warn("storage: catalogue prune", "err", err)
		} else if n > 0 {
			s.e.Logger.Info("storage: catalogue cache pruned", "entries", n)
		}
	}
	if now.Sub(s.optimized) >= optimizeEvery && s.idle() {
		s.optimized = now
		start := time.Now()
		freed, err := s.e.DB.Maintain(s.ctx, false)
		if err != nil {
			s.e.Logger.Warn("storage: database upkeep", "err", err)
			return
		}
		s.e.Logger.Info("storage: database optimised", "freed_bytes", freed, "took", time.Since(start).Round(time.Millisecond))
	}
}

// idle reports whether database upkeep may hold the writer now: no library
// sync runs and nothing is playing or loading.
func (s *StorageService) idle() bool {
	if s.e.Syncer.Running() {
		return false
	}
	switch s.e.Coord.Snapshot().State {
	case playback.StatePlaying, playback.StateLoading:
		return false
	}
	return true
}

// close stops upkeep and waits for a running pass.
func (s *StorageService) close() {
	s.cancel()
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	if done != nil {
		<-done
	}
}

// Usage reports the sizes of the artwork cache, the catalogue cache (with
// the cached lyrics) and the database.
func (s *StorageService) Usage(ctx context.Context) (StorageUsage, error) {
	out := StorageUsage{ArtworkBytes: s.e.Art.Size(), ArtworkLimitBytes: s.e.Art.MaxBytes()}
	path := s.e.Paths.DatabaseFile()
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if fi, err := os.Stat(path + suffix); err == nil {
			out.DatabaseBytes += fi.Size()
		}
	}
	var errs []error
	_, free, err := s.e.DB.Space(ctx)
	errs = append(errs, err)
	out.DatabaseFreeBytes = free
	out.CatalogEntries, out.CatalogBytes, err = s.e.Catalog.CacheSize(ctx)
	errs = append(errs, err)
	lyricEntries, lyricBytes, err := lyrics.CacheSize(ctx, s.e.DB)
	out.CatalogEntries += lyricEntries
	out.CatalogBytes += lyricBytes
	errs = append(errs, err)
	s.mu.Lock()
	out.LastMaintenance = unixMs(s.optimized)
	s.mu.Unlock()
	return out, errors.Join(errs...)
}

// Clear deletes the cached artwork, the cached catalogue responses and the
// cached lyrics. Library data and settings are kept; artwork refetches as
// it is shown.
func (s *StorageService) Clear(ctx context.Context) (ClearResult, error) {
	var out ClearResult
	before := s.e.Art.Size()
	errArt := s.e.Art.Clear()
	out.ArtworkBytes = before - s.e.Art.Size()
	n, errDB := s.e.Catalog.ClearCache(ctx)
	m, errLyrics := lyrics.ClearCache(ctx, s.e.DB)
	out.CatalogEntries = n + m
	s.e.Logger.Info("storage: cache cleared", "artwork_bytes", out.ArtworkBytes, "catalog_entries", out.CatalogEntries)
	return out, errors.Join(errArt, errDB, errLyrics)
}

// Compact prunes expired catalogue responses and lyrics and rebuilds the
// database file (VACUUM), returning the bytes given back to the disk.
func (s *StorageService) Compact(ctx context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.e.Now()
	if _, err := s.prune(ctx); err != nil {
		return 0, err
	}
	s.pruned = now
	freed, err := s.e.DB.Maintain(ctx, true)
	if err != nil {
		return 0, err
	}
	s.optimized = now
	s.e.Logger.Info("storage: database compacted", "freed_bytes", freed)
	return freed, nil
}

// prune deletes the expired catalogue responses and cached lyrics.
func (s *StorageService) prune(ctx context.Context) (int64, error) {
	n, err := s.e.Catalog.Prune(ctx)
	m, errLyrics := lyrics.Prune(ctx, s.e.DB, s.e.Now())
	return n + m, errors.Join(err, errLyrics)
}

// wipeUserArtwork removes from the artwork cache what only the signed-in
// user's data references (library playlist covers, personal mixes,
// history, …; see db.UserArtwork) and the mosaic covers composed for their
// playlists before sign-out wipes that data. Catalogue artwork stays.
func wipeUserArtwork(ctx context.Context, d *db.DB, art *artwork.Cache, log *slog.Logger) error {
	refs, err := d.UserArtwork(ctx)
	if err != nil {
		return err
	}
	freed, err := art.Remove(refs)
	mosaics, errMosaics := art.RemoveMosaics()
	log.Info("storage: user artwork removed", "items", len(refs), "bytes", freed+mosaics)
	return errors.Join(err, errMosaics)
}
