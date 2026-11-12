package playlist

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
)

func lenSong(ms int64) services.Song { return services.Song{DurationMs: ms} }

func TestStartOffsetsAreTheRunningTotalBeforeEachSong(t *testing.T) {
	got := startOffsets([]services.Song{lenSong(200_000), lenSong(0), lenSong(65_000), lenSong(-5)})
	want := []time.Duration{0, 200 * time.Second, 200 * time.Second, 265 * time.Second}
	if len(got) != len(want) {
		t.Fatalf("offsets %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("song %d starts at %v, want %v", i, got[i], want[i])
		}
	}
}

func TestStartOffsetsAreNilWithoutLengths(t *testing.T) {
	if got := startOffsets(nil); got != nil {
		t.Errorf("no songs: %v", got)
	}
	if got := startOffsets([]services.Song{lenSong(0), lenSong(0)}); got != nil {
		t.Errorf("songs without lengths: %v", got)
	}
}

func TestStartLabelsAreKeptUntilTheTracksChange(t *testing.T) {
	var pg page
	tracks := []services.Song{lenSong(200_000), lenSong(3_600_000), lenSong(1000)}
	sc := pg.starts(tracks)
	if sc.at[0] != "0:00" || sc.at[1] != "3:20" || sc.at[2] != "1:03:20" {
		t.Errorf("labels %q", sc.at)
	}
	if sc.byRow[&tracks[1]] != "3:20" {
		t.Errorf("by row %v", sc.byRow)
	}
	if again := pg.starts(tracks); &again.at[0] != &sc.at[0] {
		t.Error("the labels were made again for the same tracks")
	}
	moved := []services.Song{tracks[1], tracks[0], tracks[2]}
	if got := pg.starts(moved); got.at[1] != "1:00:00" {
		t.Errorf("after a move %q", got.at)
	}
}
