package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/native"
)

// renewals is a slog.Handler that passes records on and notes the time of
// every licence renewal the engine logs.
type renewals struct {
	slog.Handler
	mu   *sync.Mutex
	seen *[]renewal
}

type renewal struct {
	at  time.Time
	msg string
}

func (r renewals) Handle(ctx context.Context, rec slog.Record) error {
	if strings.HasPrefix(rec.Message, "licence renew") {
		r.mu.Lock()
		*r.seen = append(*r.seen, renewal{rec.Time, rec.Message})
		r.mu.Unlock()
	}
	return r.Handler.Handle(ctx, rec)
}

func (r renewals) WithAttrs(as []slog.Attr) slog.Handler {
	return renewals{r.Handler.WithAttrs(as), r.mu, r.seen}
}

func (r renewals) WithGroup(name string) slog.Handler {
	return renewals{r.Handler.WithGroup(name), r.mu, r.seen}
}

// runSoak plays items back to back (looping) for d, to exercise licence
// renewal on long sessions: a live station ("ra.…") plays for the whole
// run, songs play to their end one after another. The engine logs every
// CDM renewal message and its round trip; runSoak reports each playback
// stall (an audible gap) and whether one fell near a renewal.
func runSoak(log *slog.Logger, envFile string, items []string, term string, d time.Duration, cdm string, quality applemusic.Quality) error {
	ctx, cancel := context.WithTimeout(context.Background(), d+5*time.Minute)
	defer cancel()
	var (
		mu   sync.Mutex
		seen []renewal
	)
	log = slog.New(renewals{log.Handler(), &mu, &seen})

	api, sf, err := signIn(ctx, envFile)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		songs, err := api.SearchSongs(ctx, sf.ID, term, 25)
		if err != nil {
			return err
		}
		for _, s := range songs {
			items = append(items, s.ID)
		}
		if len(items) == 0 {
			return fmt.Errorf("no songs for %q", term)
		}
	}
	step("soak", "items", len(items), "duration", d, "quality", quality)

	eng := native.New(native.Options{Client: api, CDMPath: cdm, Logger: log, IdleTimeout: -1, Quality: quality})
	defer func() { _ = eng.Close() }()
	events := eng.Events()

	type stall struct {
		item     string
		at       time.Time
		lasted   time.Duration
		position time.Duration
	}
	var stalls []stall
	var stalled *stall
	start := time.Now()
	end := start.Add(d)
	played := 0
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for i := 0; time.Now().Before(end); i++ {
		id := items[i%len(items)]
		if err := eng.Play(ctx, id, 0); err != nil {
			step("play failed", "item", id, "err", err)
			if ctx.Err() != nil {
				return err
			}
			time.Sleep(5 * time.Second)
			continue
		}
		played++
		audible := time.Now() // Play returns at first audio; earlier events are its start-up
		step("playing", "item", id, "n", played, "elapsed", time.Since(start).Round(time.Second))
	track:
		for {
			select {
			case ev, ok := <-events:
				if !ok {
					return fmt.Errorf("engine closed")
				}
				if ev.Status.ItemID != id || ev.Status.UpdatedAt.Before(audible) {
					continue
				}
				switch s := ev.Status; s.State {
				case playback.StateLoading:
					if stalled == nil {
						stalled = &stall{item: id, at: time.Now(), position: s.Position}
						step("stall", "item", id, "position", s.Position.Round(100*time.Millisecond))
					}
				case playback.StatePlaying:
					if stalled != nil {
						stalled.lasted = time.Since(stalled.at)
						stalls = append(stalls, *stalled)
						step("stall over", "item", id, "lasted", stalled.lasted.Round(time.Millisecond))
						stalled = nil
					}
				case playback.StateEnded:
					break track
				case playback.StateError:
					step("error", "item", id, "err", s.Err)
					break track
				}
			case <-tick.C:
				s := eng.Status()
				pos := s.Position
				if s.State == playback.StatePlaying {
					pos += time.Since(s.UpdatedAt)
				}
				mu.Lock()
				n := len(seen)
				mu.Unlock()
				step("checkpoint", "elapsed", time.Since(start).Round(time.Second), "item", s.ItemID, "state", s.State,
					"position", pos.Round(time.Second), "renewal_logs", n, "stalls", len(stalls))
				if time.Now().After(end) {
					break track
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	_ = eng.Stop(ctx)

	mu.Lock()
	defer mu.Unlock()
	step("soak done", "elapsed", time.Since(start).Round(time.Second), "items_played", played, "renewal_logs", len(seen), "stalls", len(stalls))
	for _, r := range seen {
		near := 0
		for _, s := range stalls {
			if s.at.Sub(r.at).Abs() < 15*time.Second {
				near++
			}
		}
		step("renewal", "at", r.at.Format(time.TimeOnly), "msg", fmt.Sprintf("%q", r.msg), "stalls_within_15s", near)
	}
	for _, s := range stalls {
		step("stall", "item", s.item, "at", s.at.Format(time.TimeOnly), "position", s.position.Round(time.Second), "lasted", s.lasted.Round(time.Millisecond))
	}
	return nil
}
