package album

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
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

// upcomingModel is Salt Flats not out until a day after modelNow, with the
// songs at these positions (1-based track numbers) already out.
func upcomingModel(t *testing.T, out ...int) model {
	t.Helper()
	p := saltFlats(func(p *services.CatalogAlbum) {
		p.Album.Prerelease, p.Album.ReleaseAt = true, modelNow.Add(24*time.Hour).UnixMilli()
		p.Album.ReleaseDate = modelNow.Add(24 * time.Hour).Format("2006-01-02")
		for i := range p.Tracks {
			p.Tracks[i].Playable = slices.Contains(out, i+1)
		}
	})
	return newModel(p.AlbumDetail, &p, true, modelNow)
}

func trackNumbers(songs []services.Song) []int {
	var out []int
	for _, s := range songs {
		out = append(out, s.TrackNumber)
	}
	return out
}

func TestUpcomingAlbumListsTheSongsOutNowFirst(t *testing.T) {
	m := upcomingModel(t, 2, 5)
	if want := []int{2, 5, 1, 3, 4}; !slices.Equal(trackNumbers(m.list), want) {
		t.Errorf("order %v, want %v (each group in the record's order)", trackNumbers(m.list), want)
	}
	if !m.mixed || m.songsOut != 2 {
		t.Errorf("mixed %v, songsOut %d", m.mixed, m.songsOut)
	}
	if len(m.discs) != 0 {
		t.Errorf("two groups have %d discs to index", len(m.discs))
	}
	// A record with every song to come, or every one out, keeps its order.
	for _, out := range [][]int{nil, {1, 2, 3, 4, 5}} {
		m := upcomingModel(t, out...)
		if m.mixed || !slices.Equal(trackNumbers(m.list), []int{1, 2, 3, 4, 5}) {
			t.Errorf("out %v: mixed %v, order %v", out, m.mixed, trackNumbers(m.list))
		}
	}
}

func TestUpcomingAlbumHeadingsNameEachGroupOnce(t *testing.T) {
	heads := func(m model) []string {
		h := m.groupHeading(modelNow)
		var out []string
		for i := range m.list {
			var prev *services.Song
			if i > 0 {
				prev = &m.list[i-1]
			}
			out = append(out, h(&m.list[i], prev))
		}
		return out
	}
	day := kit.ReleaseBadge(upcomingModel(t).album, modelNow)
	if day == "" {
		t.Fatal("no release badge for the fixture")
	}
	if got, want := heads(upcomingModel(t, 2, 5)), []string{"Out now", "", day, "", ""}; !slices.Equal(got, want) {
		t.Errorf("mixed: %q, want %q", got, want)
	}
	// Nothing out: one heading names the day; all out: none is needed.
	if got, want := heads(upcomingModel(t)), []string{day, "", "", "", ""}; !slices.Equal(got, want) {
		t.Errorf("none out: %q, want %q", got, want)
	}
	if got := heads(upcomingModel(t, 1, 2, 3, 4, 5)); !slices.Equal(got, []string{"", "", "", "", ""}) {
		t.Errorf("all out: %q", got)
	}
	// An album that is out has no groups.
	p := saltFlats()
	if m := newModel(p.AlbumDetail, &p, true, modelNow); m.groupHeading(modelNow) != nil {
		t.Error("a released album has group headings")
	}
}

func TestUpcomingRowsLeaveTheDayToTheirHeading(t *testing.T) {
	m := upcomingModel(t, 1)
	sub := m.subline(modelNow, false)
	for i, s := range m.list {
		if got := sub(s); got != "" {
			t.Errorf("row %d says %q: the group heading says when", i, got)
		}
	}
}

func TestJustOutAlbumWaitsForItsSongs(t *testing.T) {
	p := saltFlats(func(p *services.CatalogAlbum) {
		p.Album.Prerelease, p.Album.ReleaseAt = true, modelNow.Add(-time.Minute).UnixMilli()
		for i := range p.Tracks {
			p.Tracks[i].Playable = false
		}
	})
	m := newModel(p.AlbumDetail, &p, true, modelNow)
	if m.upcoming || !m.justOut {
		t.Fatalf("upcoming %v, justOut %v", m.upcoming, m.justOut)
	}
	// Its songs are on their way, not "Unavailable".
	if got := m.subline(modelNow, false)(m.list[0]); got != "" {
		t.Errorf("subline %q", got)
	}
}

// The minute an album comes out its page doesn't jump: Apple's stand-in
// credit stays dropped and the running time stays left out, as before.
func TestJustOutAlbumKeepsTheShapeOfTheUpcomingOne(t *testing.T) {
	build := func(release time.Time) model {
		p := saltFlats(func(p *services.CatalogAlbum) {
			p.Album.Prerelease, p.Album.ReleaseAt = true, release.UnixMilli()
			for i := range p.Tracks {
				p.Tracks[i].Playable, p.Tracks[i].DurationMs, p.Tracks[i].Artist = false, 0, "unknown"
			}
		})
		return newModel(p.AlbumDetail, &p, true, modelNow)
	}
	before, after := build(modelNow.Add(time.Minute)), build(modelNow.Add(-time.Minute))
	if !before.upcoming || !after.justOut {
		t.Fatalf("upcoming %v, justOut %v", before.upcoming, after.justOut)
	}
	if after.list[0].Artist != "" || before.list[0].Artist != "" {
		t.Errorf("stand-in credit kept: %q, %q", before.list[0].Artist, after.list[0].Artist)
	}
	if before.meta != after.meta {
		t.Errorf("the catalogue line jumps: %q, then %q", before.meta, after.meta)
	}
}
