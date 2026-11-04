package kit

import (
	"context"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// This file is what the radio pages, the player bar and Now Playing share of
// a live station (v0.2.0's features/radio/on-air.ts): its page's query, and
// the programme on air in its schedule. A feature cannot import another, so
// the one query key and the one lookup live here.

// StationStale is how long a station page's data (and so its schedule) is
// kept before it is read again.
const StationStale = 5 * time.Minute

// StationKey is the store key of a station's page.
func StationKey(id string) store.Key { return store.Key{"radio", "station", id} }

// UseStation is a station page's data, shared by the radio pages and the
// player's live line; fetch reads it (RadioService.Station).
func UseStation(q *store.Client, id string, fetch func(ctx context.Context, id string) (services.StationDetail, error)) store.Result[services.StationDetail] {
	return store.Use(q, StationKey(id), func(ctx context.Context) (services.StationDetail, error) {
		return fetch(ctx, id)
	}, store.Options{Stale: StationStale})
}

// OnAirProgramme is the programme on air at now (Unix ms), if the schedule
// covers it.
func OnAirProgramme(schedule []services.StationEvent, now int64) (services.StationEvent, bool) {
	for _, e := range schedule {
		if e.StartMs <= now && now < e.EndMs {
			return e, true
		}
	}
	return services.StationEvent{}, false
}
