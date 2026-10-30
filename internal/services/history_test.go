package services

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
)

// History follows the listen in progress until Record stores it.
func TestHistoryStartedListen(t *testing.T) {
	d, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	h := &History{DB: d}
	song := coordinator.Item{ID: "5", Title: "Now"}
	start := func(item coordinator.Item, pos time.Duration, at time.Time) {
		h.observe(coordinator.Activity{Kind: coordinator.ActivityStart, Item: item, Position: pos, At: at})
	}
	startedAt := func() time.Time {
		t.Helper()
		ps := h.started(t.Context())
		if len(ps) == 0 {
			return time.Time{}
		}
		if len(ps) != 1 || ps[0].CatalogID != "5" || ps[0].Name != "Now" {
			t.Fatalf("started = %+v", ps)
		}
		return ps[0].PlayedAt
	}

	if !startedAt().IsZero() {
		t.Fatal("a listen before any started")
	}
	began := time.Now().Add(-time.Minute)
	start(song, 0, began)
	// A resume after a pause continues the listen.
	start(song, 40*time.Second, began.Add(50*time.Second))
	if got := startedAt(); !got.Equal(began) {
		t.Fatalf("started at %v, want %v", got, began)
	}
	h.Record(song, 40*time.Second, false)
	if got := startedAt(); !got.IsZero() {
		t.Fatalf("recorded listen still started at %v", got)
	}

	// Repeat one: the replay starts before the previous listen is recorded.
	replay := time.Now()
	start(song, 0, replay)
	h.Record(song, 3*time.Minute, true)
	if got := startedAt(); !got.Equal(replay) {
		t.Fatalf("replay started at %v, want %v", got, replay)
	}

	// Live radio is not a song.
	start(coordinator.Item{ID: "ra.1"}, 0, time.Now())
	if got := startedAt(); !got.Equal(replay) {
		t.Fatalf("after a station: started at %v, want the replay", got)
	}
}
