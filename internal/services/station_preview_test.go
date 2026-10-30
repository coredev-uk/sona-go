package services

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
)

func TestStationSeed(t *testing.T) {
	st := func(id, kind string) applemusic.Station {
		var s applemusic.Station
		s.ID, s.Attributes.Kind = id, kind
		return s
	}
	for _, tc := range []struct {
		st       applemusic.Station
		kind, id string
	}{
		{st("ra.1109715168", applemusic.StationKindSongSeeded), SeedSong, "1109715168"},
		{st("ra.cp-1109715168", applemusic.StationKindSongSeeded), SeedSong, "1109715168"},
		{st("ra.1650841512", applemusic.StationKindAlbumSeeded), SeedAlbum, "1650841512"},
		{st("ra.657515", applemusic.StationKindArtistSeeded), SeedArtist, "657515"},
		{st("ra.a-657515", applemusic.StationKindArtistOnly), SeedArtist, "657515"},
		// No seed: the user's station, moods, genres; or an ID Sona cannot read.
		{st("ra.u-abc", applemusic.StationKindUser), "", ""},
		{st("ra.q-xyz", applemusic.StationKindQuery), "", ""},
		{st("ra.1000000001", applemusic.StationKindEditorial), "", ""},
		{st("ra.x-12", applemusic.StationKindSongSeeded), "", ""},
	} {
		kind, id := stationSeed(tc.st)
		if kind != tc.kind || id != tc.id {
			t.Errorf("stationSeed(%s %s) = %q %q, want %q %q", tc.st.ID, tc.st.Attributes.Kind, kind, id, tc.kind, tc.id)
		}
	}
}

func TestStationPreviews(t *testing.T) {
	var p stationPreviews
	now := time.Unix(1_000_000, 0)
	batch := func(ids ...string) []applemusic.Song {
		out := make([]applemusic.Song, len(ids))
		for i, id := range ids {
			out[i].ID = id
		}
		return out
	}
	first := func(s []applemusic.Song) string {
		if len(s) == 0 {
			return ""
		}
		return s[0].ID
	}

	// The page parks a batch; a racing page load keeps the first one.
	if got := p.park("ra.1", batch("a", "b"), now); first(got) != "a" {
		t.Fatalf("park = %v", got)
	}
	if got := p.park("ra.1", batch("x"), now.Add(time.Second)); first(got) != "a" {
		t.Fatalf("second park replaced the parked batch: %v", got)
	}
	// Reloading the page shows the same batch.
	if got, ok := p.get("ra.1", now.Add(time.Minute)); !ok || first(got) != "a" {
		t.Fatalf("get = %v, %v", got, ok)
	}
	// Playing takes it once; the next batch comes from Apple.
	if got, ok := p.take("ra.1", now.Add(2*time.Minute)); !ok || first(got) != "a" {
		t.Fatalf("take = %v, %v", got, ok)
	}
	if _, ok := p.take("ra.1", now.Add(2*time.Minute)); ok {
		t.Fatal("a batch played twice")
	}

	// A stale batch is neither shown nor played.
	p.park("ra.2", batch("c"), now)
	late := now.Add(previewTTL + time.Second)
	if _, ok := p.get("ra.2", late); ok {
		t.Error("stale batch shown")
	}
	if _, ok := p.take("ra.2", late); ok {
		t.Error("stale batch played")
	}
	if got := p.park("ra.2", batch("d"), late); first(got) != "d" {
		t.Errorf("a fresh batch did not replace a stale one: %v", got)
	}

	// Sign-out drops them all.
	p.clear()
	if _, ok := p.get("ra.2", late); ok {
		t.Error("batch survived clear")
	}
}
