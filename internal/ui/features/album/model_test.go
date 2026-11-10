package album

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
)

var modelNow = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func factsOf(m model) map[string]string {
	out := map[string]string{}
	for _, f := range m.facts {
		out[f.label] = f.value
	}
	return out
}

func TestFinePrintFacts(t *testing.T) {
	p := saltFlats(func(p *services.CatalogAlbum) {
		p.Tracks[0].ReleaseDate = "1998-01-01" // an older recording
		p.Tracks[0].Composer = "Mia Vale & Zed"
	})
	m := newModel(p.AlbumDetail, &p, true, modelNow)
	got := factsOf(m)
	for label, want := range map[string]string{
		"Released": "March 8, 2019", "First released": "1998", "Genres": "Indie Pop, Pop",
		"Written by": "Mia Vale and Zed", "Label": "Quiet Records",
	} {
		if got[label] != want {
			t.Errorf("%s = %q, want %q", label, got[label], want)
		}
	}
	// The ℗ line carries the rights, and names the label when it does: then
	// the Label fact is not repeated.
	p.Copyright = "℗ 2019 Quiet Records"
	m = newModel(p.AlbumDetail, &p, true, modelNow)
	if _, ok := factsOf(m)["Label"]; ok {
		t.Error("the label is repeated beside a copyright line that names it")
	}
	// One genre is singular.
	p.Genres = nil
	m = newModel(p.AlbumDetail, &p, true, modelNow)
	if got := factsOf(m); got["Genre"] != "Indie Pop" {
		t.Errorf("facts %v", got)
	}
}

func TestUpcomingAlbumLeavesReleaseDayToTheCountdown(t *testing.T) {
	p := saltFlats(func(p *services.CatalogAlbum) {
		p.Album.Prerelease, p.Album.ReleaseAt = true, modelNow.Add(time.Hour).UnixMilli()
	})
	m := newModel(p.AlbumDetail, &p, true, modelNow)
	if _, ok := factsOf(m)["Released"]; ok {
		t.Error("Released shown for an album not out")
	}
}

func TestWithCatalogueReleaseTakesAnUpcomingCataloguePage(t *testing.T) {
	release := modelNow.Add(24 * time.Hour)
	upcoming := saltFlats(func(p *services.CatalogAlbum) {
		p.Album.Prerelease, p.Album.ReleaseAt, p.Album.ReleaseDate = true, release.UnixMilli(), "2026-11-05"
	})
	library := func() services.AlbumDetail {
		d := services.AlbumDetail{
			Album:  services.Album{ID: "l.9", Source: services.SourceLibrary, CatalogID: "100", ReleaseDate: ""},
			Tracks: []services.Song{{ID: "i.1", Playable: true, ReleaseDate: "2026-09-17"}, {ID: "i.2", Playable: true}},
		}
		return d
	}

	d := library()
	got := withCatalogueRelease(d, &upcoming, modelNow)
	if !got.Album.Prerelease || got.Album.ReleaseAt != release.UnixMilli() || got.Album.ReleaseDate != "2026-11-05" {
		t.Fatalf("album = %+v, want the catalogue's release state", got.Album)
	}
	if !got.Tracks[0].Playable || got.Tracks[1].Playable {
		t.Errorf("playable = %v, %v: the dated song is out, the undated one is not", got.Tracks[0].Playable, got.Tracks[1].Playable)
	}
	if !d.Tracks[1].Playable || d.Album.Prerelease {
		t.Error("the loaded album was changed in place")
	}

	// The same page once it is out, and no page at all, change nothing.
	if got := withCatalogueRelease(library(), &upcoming, release); got.Album.Prerelease || !got.Tracks[1].Playable {
		t.Errorf("a catalogue album out overrode the library's: %+v", got)
	}
	if got := withCatalogueRelease(library(), nil, modelNow); got.Album.Prerelease || !got.Tracks[1].Playable {
		t.Errorf("no catalogue page overrode the library's: %+v", got)
	}
}

func TestPopularNeverMarksASongThatCannotPlay(t *testing.T) {
	p := saltFlats(func(p *services.CatalogAlbum) {
		p.PopularIDs = []string{"c1", "c2"}
		p.Tracks[1].Playable = false
	})
	m := newModel(p.AlbumDetail, &p, true, modelNow)
	if !m.popular["c1"] || m.popular["c2"] {
		t.Errorf("popular %v", m.popular)
	}
	// A library album with no catalogue page marks none.
	lib := newModel(p.AlbumDetail, nil, false, modelNow)
	if len(lib.popular) != 0 {
		t.Errorf("popular without a catalogue page: %v", lib.popular)
	}
}

func TestPartialLibraryAlbumCountsTheWholeRecord(t *testing.T) {
	page := saltFlats()
	detail := page.AlbumDetail
	detail.Tracks = detail.Tracks[:2]
	detail.DurationMs = 400000
	m := newModel(detail, &page, false, modelNow)
	if m.share == nil || *m.share != (share{2, 5}) {
		t.Fatalf("share %v", m.share)
	}
	if want := "Album · 2019 · 5 songs, 17 min · Indie Pop"; m.meta != want {
		t.Errorf("meta %q, want %q", m.meta, want)
	}
	// The catalogue page itself is never a partial.
	if full := newModel(page.AlbumDetail, &page, true, modelNow); full.share != nil {
		t.Errorf("share on the catalogue page: %v", full.share)
	}
}

func TestLibraryAlbumPlaysTheMotionOfItsCataloguePage(t *testing.T) {
	page := saltFlats(func(p *services.CatalogAlbum) { p.Header.Motion = "/motion/1/2.mp4" })
	detail := page.AlbumDetail
	detail.Header.Motion = ""
	if m := newModel(detail, &page, false, modelNow); m.motion != "/motion/1/2.mp4" {
		t.Errorf("motion %q", m.motion)
	}
	detail.Header.Motion = "/motion/own.mp4"
	if m := newModel(detail, &page, false, modelNow); m.motion != "/motion/own.mp4" {
		t.Errorf("motion %q", m.motion)
	}
}

func TestSinglesAreNamedSo(t *testing.T) {
	p := saltFlats(func(p *services.CatalogAlbum) { p.Album.Single = true })
	if m := newModel(p.AlbumDetail, &p, true, modelNow); !strings.HasPrefix(m.meta, "Single · 2019") {
		t.Errorf("meta %q", m.meta)
	}
}

func TestLongListsHaveNoDiscIndex(t *testing.T) {
	var songs []services.Song
	for i := 0; i < 250; i++ {
		songs = append(songs, track(i+1, fmt.Sprint("S", i), func(s *services.Song) { s.DiscNumber = 1 + i/125 }))
	}
	p := saltFlats(func(p *services.CatalogAlbum) { p.Tracks = songs })
	if m := newModel(p.AlbumDetail, &p, true, modelNow); len(m.discs) != 0 {
		t.Errorf("a virtual list has %d discs", len(m.discs))
	}
	p.Tracks = songs[:150]
	if m := newModel(p.AlbumDetail, &p, true, modelNow); len(m.discs) != 2 {
		t.Errorf("a list of 150 has %d discs", len(m.discs))
	}
}

func TestSublineJoinsCreditComposerAndReason(t *testing.T) {
	p := saltFlats(func(p *services.CatalogAlbum) {
		p.Tracks[0].Artist = "Zed"
		p.Tracks[0].Work, p.Tracks[1].Composer = "A Work", "Cy"
		p.Tracks[2].Playable = false
	})
	m := newModel(p.AlbumDetail, &p, true, modelNow)
	m.composers = 2 // a classical album of several composers
	sub := m.subline(modelNow, false)
	if got := sub(m.list[0]); got != "Zed" { // a credit; the piece is in a work
		t.Errorf("credit: %q", got)
	}
	if got := sub(m.list[1]); got != "Cy" { // the composer of a piece outside any work
		t.Errorf("composer: %q", got)
	}
	if got := sub(m.list[2]); got != "Unavailable" {
		t.Errorf("reason: %q", got)
	}
	m.list[1].Artist = "Zed"
	if got := sub(m.list[1]); got != "Zed · Cy" {
		t.Errorf("both: %q", got)
	}
}

func TestReleaseMemoCopiesTheTracksOncePerLoad(t *testing.T) {
	release := modelNow.Add(24 * time.Hour)
	upcoming := saltFlats(func(p *services.CatalogAlbum) {
		p.Album.Prerelease, p.Album.ReleaseAt, p.Album.ReleaseDate = true, release.UnixMilli(), "2026-11-05"
	})
	d := services.AlbumDetail{
		Album:  services.Album{ID: "l.9", Source: services.SourceLibrary},
		Tracks: []services.Song{{ID: "i.1", Playable: true}, {ID: "i.2", Playable: true, ReleaseDate: "2026-09-17"}},
	}
	var m releaseMemo
	first := m.of(d, &upcoming, modelNow)
	again := m.of(d, &upcoming, modelNow.Add(time.Minute))
	if &first.Tracks[0] != &again.Tracks[0] {
		t.Error("the track list was copied again for the same data")
	}
	if first.Tracks[0].Playable || !first.Tracks[1].Playable || !first.Album.Prerelease {
		t.Errorf("memo answer = %+v", first)
	}
	// A reload (a new slice) or another catalogue date rebuilds it.
	reloaded := d
	reloaded.Tracks = append([]services.Song(nil), d.Tracks...)
	if got := m.of(reloaded, &upcoming, modelNow); &got.Tracks[0] == &first.Tracks[0] {
		t.Error("a reload kept the old copy")
	}
	later := upcoming
	later.Album.ReleaseDate, later.Album.ReleaseAt = "2026-11-06", release.Add(24*time.Hour).UnixMilli()
	if got := m.of(reloaded, &later, modelNow); got.Album.ReleaseDate != "2026-11-06" {
		t.Errorf("a new catalogue date was not taken: %+v", got.Album)
	}
	// Once the catalogue album is out it is the library's own again.
	if got := m.of(d, &upcoming, release); got.Album.Prerelease || !got.Tracks[0].Playable {
		t.Errorf("an album out was overridden: %+v", got.Album)
	}
}
