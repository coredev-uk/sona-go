package album

import (
	"reflect"
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
)

// song is v0.2.0's test factory: playable, on disc 1.
func song(p func(*services.Song)) services.Song {
	s := services.Song{ID: "s", DiscNumber: 1, Playable: true}
	if p != nil {
		p(&s)
	}
	return s
}

func TestDiscsAreIndexedByWhereTheyStart(t *testing.T) {
	list := []services.Song{
		song(func(s *services.Song) { s.DiscNumber, s.DurationMs = 1, 1000 }),
		song(func(s *services.Song) { s.DiscNumber, s.DurationMs = 1, 2000 }),
		song(func(s *services.Song) { s.DiscNumber, s.DurationMs = 2, 500 }),
	}
	want := []disc{{1, 0, 2, 3000}, {2, 2, 1, 500}}
	if got := discsOf(list); !reflect.DeepEqual(got, want) {
		t.Errorf("discsOf = %+v, want %+v", got, want)
	}
	// One disc needs no index.
	if got := discsOf(list[:2]); got != nil {
		t.Errorf("one disc has an index: %+v", got)
	}
}

func TestCompilationsGetAnArtistColumn(t *testing.T) {
	by := func(name string) services.Song { return song(func(s *services.Song) { s.Artist = name }) }
	own, feat := by("Zara Larsson"), by("Zara Larsson & David Guetta")
	if isCompilation([]services.Song{own, own, own, feat}, "Zara Larsson") {
		t.Error("a featured artist on one song made a compilation")
	}
	if !isCompilation([]services.Song{by("A"), by("B")}, "Various Artists") {
		t.Error("two artists on a Various Artists album is a compilation")
	}
	// A song with no credit yet counts for neither.
	out, none := by("Kings of Leon"), by("")
	if isCompilation([]services.Song{out, out, none, none, none}, "Kings of Leon") {
		t.Error("songs with no credit counted towards a compilation")
	}
	// Exactly half is not more than half.
	if isCompilation([]services.Song{by("A"), by("B")}, "A") {
		t.Error("half the songs elsewhere is not a compilation")
	}
}

func TestSongwritersAreListedOnceMostCreditedFirst(t *testing.T) {
	by := func(c string) services.Song { return song(func(s *services.Song) { s.Composer = c }) }
	list := []services.Song{
		by("Christine McVie"),
		by("Lindsey Buckingham"),
		by("Lindsey Buckingham & Stevie Nicks"),
		by("Christine McVie, John McVie, Lindsey Buckingham, Mick Fleetwood & Stevie Nicks"),
		by(""),
	}
	want := []string{"Lindsey Buckingham", "Christine McVie", "Stevie Nicks", "John McVie", "Mick Fleetwood"}
	if got := writersOf(list); !reflect.DeepEqual(got, want) {
		t.Errorf("writersOf = %q, want %q", got, want)
	}
}

func TestClassicalMovementsGroupUnderTheirWorks(t *testing.T) {
	aria := song(func(s *services.Song) {
		s.ID, s.Name = "a", "Goldberg Variations, BWV 988 (1955 Recording): Aria"
		s.Work, s.Movement = "Goldberg Variations, BWV 988 (1955 Recording)", "Aria"
		s.Composer = "Johann Sebastian Bach"
	})
	v1 := aria
	v1.ID, v1.Name, v1.Movement = "v1", "…: Variation 1", "Variation 1"
	later := aria
	later.ID, later.Work = "l", "Goldberg Variations, BWV 988 (1981 Recording)"
	pop := song(func(s *services.Song) { s.Name = "Dreams" })

	if !hasWorks([]services.Song{pop, aria}) || hasWorks([]services.Song{pop}) {
		t.Error("hasWorks")
	}
	for _, c := range []struct {
		name  string
		song  services.Song
		prev  *services.Song
		count int
		want  string
	}{
		{"first of a work", aria, nil, 1, "Goldberg Variations, BWV 988 (1955 Recording)"},
		{"next movement", v1, &aria, 1, ""},
		{"another recording", later, &v1, 1, "Goldberg Variations, BWV 988 (1981 Recording)"},
		{"several composers", aria, &pop, 2, "Johann Sebastian Bach · Goldberg Variations, BWV 988 (1955 Recording)"},
		{"a new disc starts the work again", func() services.Song { s := v1; s.DiscNumber = 2; return s }(), &aria, 1, "Goldberg Variations, BWV 988 (1955 Recording)"},
		{"outside any work", pop, &aria, 1, ""},
	} {
		if got := workHeading(c.song, c.prev, c.count); got != c.want {
			t.Errorf("%s: heading %q, want %q", c.name, got, c.want)
		}
	}
	if got := movementTitle(v1); got != "Variation 1" {
		t.Errorf("movement title %q", got)
	}
	if got := movementTitle(pop); got != "Dreams" {
		t.Errorf("title of a piece outside a work %q", got)
	}
}

func TestFirstReleasedSaysWhenRecordingsAreOlder(t *testing.T) {
	at := func(d string) services.Song { return song(func(s *services.Song) { s.ReleaseDate = d }) }
	list := []services.Song{at("1982-09-02"), at("1959-01-01")}
	for _, c := range []struct {
		list  []services.Song
		album string
		want  string
	}{
		{list, "2013-01-28", "1959"},
		{list, "1959-05-01", ""},
		{[]services.Song{at("")}, "2013", ""},
		{list, "", ""},
	} {
		if got := firstReleased(c.list, c.album); got != c.want {
			t.Errorf("firstReleased(%q) = %q, want %q", c.album, got, c.want)
		}
	}
}

func TestPlaceholderCreditsAreDroppedOnAnUnreleasedAlbum(t *testing.T) {
	placeholder := song(func(s *services.Song) { s.Name, s.Artist, s.Playable, s.DurationMs = "Track 3", "unknown", false, 0 })
	announced := song(func(s *services.Song) { s.Artist, s.Playable, s.DurationMs = "Morgan Wallen", false, 196000 })
	out := song(func(s *services.Song) { s.Artist, s.DurationMs = "Yung Lean", 161000 })
	in := []services.Song{placeholder, announced, out}
	got := withoutPlaceholderCredits(in, true)
	if got[0].Artist != "" || got[1].Artist != "Morgan Wallen" || got[2].Artist != "Yung Lean" {
		t.Errorf("credits %q %q %q", got[0].Artist, got[1].Artist, got[2].Artist)
	}
	if in[0].Artist != "unknown" {
		t.Error("the input was changed")
	}
	// Once out, songs keep whatever Apple credits them with.
	if got := withoutPlaceholderCredits(in, false); got[0].Artist != "unknown" {
		t.Errorf("an album that is out lost a credit: %q", got[0].Artist)
	}
}

func TestLibraryShare(t *testing.T) {
	if got := libraryShare(2, 13); got == nil || *got != (share{2, 13}) {
		t.Errorf("libraryShare(2,13) = %v", got)
	}
	if libraryShare(13, 13) != nil {
		t.Error("a whole album has a share")
	}
	if libraryShare(2, 0) != nil {
		t.Error("an unknown catalogue album has a share")
	}
}

func TestLayoutFollowsTheContentWidth(t *testing.T) {
	for _, c := range []struct {
		width float32
		want  layout
	}{
		{1900, layout{true, 384}},
		{1687, layout{true, 384}}, // a 1920 window beside the sidebar
		{1600, layout{true, 384}},
		{1599, layout{true, 320}},
		{1280, layout{true, 320}},
		{1279, layout{true, 256}},
		{860, layout{true, 256}},
		{859, layout{false, 168}},
		{0, layout{false, 168}},
	} {
		if got := albumLayout(c.width); got != c.want {
			t.Errorf("albumLayout(%v) = %+v, want %+v", c.width, got, c.want)
		}
	}
}

func TestLibraryIDs(t *testing.T) {
	if !isLibraryID("l.fxv3") || isLibraryID("6784018115") || isLibraryID("r.abc") {
		t.Error("isLibraryID")
	}
}
