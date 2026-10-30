package library

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

func appleSong(id, name string) applemusic.Song {
	return applemusic.Song{Resource: applemusic.Resource[applemusic.SongAttributes]{
		ID: id, Type: applemusic.TypeSongs,
		Attributes: applemusic.SongAttributes{Name: name, PlayParams: &applemusic.PlayParams{ID: id, Kind: "song"}},
	}}
}

// render writes days as "day: entry entry | day: …" with local plays by
// name and Apple-only songs as "~name".
func render(days []HistoryDay) string {
	var b strings.Builder
	for i, d := range days {
		if i > 0 {
			b.WriteString(" | ")
		}
		if d.Day.IsZero() {
			b.WriteString("recent:")
		} else {
			b.WriteString(d.Day.Format("01-02") + ":")
		}
		for _, e := range d.Entries {
			if e.Play != nil {
				b.WriteString(" " + e.Play.Name)
			} else {
				b.WriteString(" ~" + e.Apple.Attributes.Name)
			}
		}
	}
	return b.String()
}

func TestMergeHistory(t *testing.T) {
	loc := time.FixedZone("test", 2*3600)
	at := func(day, hour int) time.Time { return time.Date(2026, 10, day, hour, 0, 0, 0, loc) }
	plays := []Play{
		{Name: "A", CatalogID: "1", PlayedAt: at(30, 9)},
		{Name: "B", CatalogID: "2", LibraryID: "i.2", PlayedAt: at(30, 1)},
		{Name: "A", CatalogID: "1", PlayedAt: at(29, 23)}, // repeat: both plays kept
		{Name: "C", CatalogID: "3", PlayedAt: at(28, 12)},
	}
	last := map[string]time.Time{"1": at(30, 9), "2": at(30, 1), "i.2": at(30, 1), "3": at(28, 12), "old": at(1, 12)}
	apple := []applemusic.Song{
		appleSong("10", "Phone 1"), // newer than anything played here
		appleSong("1", "A"),        // played here: deduplicated
		appleSong("11", "Phone 2"),
		{Resource: applemusic.Resource[applemusic.SongAttributes]{ // B by its library ID
			ID: "i.2", Type: applemusic.TypeLibrarySongs,
			Attributes: applemusic.SongAttributes{Name: "B", PlayParams: &applemusic.PlayParams{ID: "i.2", CatalogID: "2", IsLibrary: true}},
		}},
		appleSong("3", "C"),
		appleSong("12", "Phone 3"),
		appleSong("old", "Old"), // last played here before since: the merge stops
		appleSong("13", "Older phone"),
	}
	got := render(MergeHistory(plays, nil, last, apple, at(20, 0), loc))
	want := "recent: ~Phone 1 | 10-30: A ~Phone 2 B | 10-29: A | 10-28: C ~Phone 3"
	if got != want {
		t.Fatalf("merged\n got %s\nwant %s", got, want)
	}

	// Without Apple's list (offline, signed out) the local plays still group.
	if got := render(MergeHistory(plays, nil, last, nil, at(20, 0), loc)); got != "10-30: A B | 10-29: A | 10-28: C" {
		t.Fatalf("local only: %s", got)
	}
	// Nothing played here: Apple's list is one undated group.
	if got := render(MergeHistory(nil, nil, nil, apple[:3], at(20, 0), loc)); got != "recent: ~Phone 1 ~A ~Phone 2" {
		t.Fatalf("apple only: %s", got)
	}
}

// The song playing here is in Apple's list before playback_history records
// it (when it ends): it is this computer's play, not another device's.
func TestMergeHistoryStartedListen(t *testing.T) {
	loc := time.FixedZone("test", 2*3600)
	at := func(day, hour int) time.Time { return time.Date(2026, 10, day, hour, 0, 0, 0, loc) }
	plays := []Play{{Name: "A", CatalogID: "1", PlayedAt: at(30, 9)}}
	last := map[string]time.Time{"1": at(30, 9)}
	now := Play{Name: "Now", CatalogID: "5", LibraryID: "i.5", PlayedAt: at(30, 10)}
	apple := []applemusic.Song{appleSong("5", "Now"), appleSong("10", "Phone"), appleSong("1", "A")}
	merge := func(started []Play, apple []applemusic.Song) string {
		return render(MergeHistory(plays, started, last, apple, at(20, 0), loc))
	}
	if got := merge(nil, apple); got != "recent: ~Now ~Phone | 10-30: A" {
		t.Fatalf("without the started listen: %s", got)
	}
	for _, tc := range []struct {
		name    string
		started []Play
		apple   []applemusic.Song
		want    string
	}{
		{"playing", []Play{now}, apple, "10-30: Now ~Phone A"},
		{"by library ID", []Play{{Name: "Now", LibraryID: "i.5", PlayedAt: at(30, 10)}}, []applemusic.Song{{Resource: applemusic.Resource[applemusic.SongAttributes]{
			ID: "i.5", Type: applemusic.TypeLibrarySongs, Attributes: applemusic.SongAttributes{Name: "Now"},
		}}, appleSong("1", "A")}, "10-30: Now A"},
		{"not reported yet", []Play{now}, apple[1:], "recent: ~Phone | 10-30: A"},
		// Apple's A is the replay started at 10, not the play recorded at 9.
		{"replay", []Play{{Name: "A", CatalogID: "1", PlayedAt: at(30, 10)}}, []applemusic.Song{appleSong("1", "A"), appleSong("10", "Phone")}, "10-30: A ~Phone A"},
		// Recorded meanwhile: the started listen is that play, kept once.
		{"recorded", []Play{{Name: "A", CatalogID: "1", PlayedAt: at(30, 9)}}, []applemusic.Song{appleSong("1", "A"), appleSong("10", "Phone")}, "10-30: A ~Phone"},
	} {
		if got := merge(tc.started, tc.apple); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.want)
		}
	}
	if got := render(MergeHistory(nil, []Play{now}, nil, apple[:2], at(20, 0), loc)); got != "10-30: Now ~Phone" {
		t.Errorf("nothing recorded here yet: %s", got)
	}
}

func TestHistoryResolvesLibraryPlays(t *testing.T) {
	e := newEnv(t)
	seed(e.fake, 10)
	if err := e.run(t, DomainSongs); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Now()
	for i, p := range []dbq.InsertHistoryParams{
		{PlayedAt: db.Millis(now.Add(-time.Hour)), ItemID: "i.3", LibraryID: "i.3", Name: "Song 3"},
		{PlayedAt: db.Millis(now.Add(-2 * time.Hour)), ItemID: "77", CatalogID: "77", Name: "Catalog"},
		{PlayedAt: db.Millis(now.Add(-40 * 24 * time.Hour)), ItemID: "78", CatalogID: "78", Name: "Long ago"},
	} {
		if _, err := e.db.Q.InsertHistory(ctx, p); err != nil {
			t.Fatal(i, err)
		}
	}
	plays, last, more, err := e.lib.History(ctx, now.Add(-30*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(plays) != 2 || !more {
		t.Fatalf("plays = %+v, more %v", plays, more)
	}
	if plays[0].LibraryID != "i.3" || plays[0].CatalogID != "c3" || plays[1].CatalogID != "77" {
		t.Fatalf("plays = %+v", plays)
	}
	if _, ok := last["78"]; !ok || !last["i.3"].Equal(plays[0].PlayedAt) {
		t.Fatalf("last = %v", last)
	}
}
