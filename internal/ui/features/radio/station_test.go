package radio

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/services"
)

// stationRig is the station page of s, with its detail as given.
func stationRig(t *testing.T, d services.StationDetail, setup ...func(*rig)) *rig {
	t.Helper()
	return newRig(t, "/station/"+d.Station.ID, 1132, 1253, append([]func(*rig){fake(func(f *fakeRadio) {
		f.details[d.Station.ID] = d
	})}, setup...)...)
}

func TestStationPageShowsItsHeader(t *testing.T) {
	s := liveStation("ra.1", "Apple Music 1", func(s *services.Station) { s.Provider, s.Notes = "Apple Music", "The home of new music." })
	rg := stationRig(t, services.StationDetail{Station: s})
	rg.tt.Until("the header", rg.tt.Shows("Apple Music 1", "Live radio · Apple Music", "The home of new music.", "Apple Music 1 tagline"))
	for _, label := range []string{"Play Apple Music 1", "More for Apple Music 1"} {
		if _, ok := rg.tt.Find(label); !ok {
			t.Errorf("no %q: %q", label, rg.tt.Texts())
		}
	}
	// No schedule, no songs: nothing under the header.
	for _, absent := range []string{"Schedule", "Plays first", "On air"} {
		if rg.tt.HasText(absent) {
			t.Errorf("a section %q without data", absent)
		}
	}
}

func TestStationPageLoadsAndFailsInPlace(t *testing.T) {
	s := liveStation("ra.1", "Apple Music 1")
	hold := make(chan struct{})
	rg := newRig(t, "/station/ra.1", 1132, 1253, fake(func(f *fakeRadio) {
		f.hold = hold
		f.details["ra.1"] = services.StationDetail{Station: s}
		f.detailErr = errors.New("offline")
	}))
	rg.tt.Frame()
	if rg.count("Loading") != 1 {
		t.Errorf("no loading frame: %q", rg.tt.Texts())
	}
	close(hold)
	rg.tt.Until("the error", rg.tt.Shows("Couldn't load this station", "Try again"))
	rg.api.set(func(f *fakeRadio) { f.detailErr = nil })
	rg.click("Try again")
	rg.tt.Until("the station", rg.tt.Shows("Live radio"))
	if rg.tt.HasText("Couldn't load this station") {
		t.Error("the error stayed after a retry that worked")
	}
}

func TestStationPageBylineLinksToTheShowOnlyWhereThePageExists(t *testing.T) {
	ep := episodeStation("ra.ep", "Episode One")
	ep.ShowID = "show.1"
	rg := stationRig(t, services.StationDetail{Station: ep})
	rg.tt.Until("the byline", rg.tt.Shows("Zane Lowe"))
	rg.click("Zane Lowe")
	if rg.router.Path() != "/curator/show.1" {
		t.Errorf("the show's link went to %q", rg.router.Path())
	}
	// Without the curator page ported, the name is plain text.
	rg = stationRig(t, services.StationDetail{Station: ep}, func(rg *rig) {
		rg.h.Env.HasRoute = func(p string) bool { return p != "/curator/show.1" }
	})
	rg.tt.Until("the byline", rg.tt.Shows("Zane Lowe"))
	_ = rg.tt.Click("Zane Lowe")
	if rg.router.Path() != "/station/ra.ep" {
		t.Errorf("a link to a page that does not exist went to %q", rg.router.Path())
	}
}

func TestStationPageBylineFallsBackToTheHostThenTheTagline(t *testing.T) {
	host := episodeStation("ra.ep", "Episode One")
	host.ShowName, host.HostName, host.Tagline = "", "Zane", "A tagline"
	rg := stationRig(t, services.StationDetail{Station: host})
	rg.tt.Until("the byline", rg.tt.Shows("Zane"))
	if rg.count("A tagline") != 0 {
		t.Errorf("the tagline shows beside the host: %q", rg.tt.Texts())
	}
	tag := liveStation("ra.2", "Hits")
	rg = stationRig(t, services.StationDetail{Station: tag})
	rg.tt.Until("the byline", rg.tt.Shows("Hits tagline"))
}

func seedDetail(seed services.StationSeed) services.StationDetail {
	return services.StationDetail{Station: trackStation("ra.78", "Radiohead Station"), Seed: &seed}
}

func TestStationPageBylineSaysWhatTheStationWasMadeFrom(t *testing.T) {
	for _, c := range []struct {
		name string
		seed services.StationSeed
		want []string
		path string
	}{
		{
			"song",
			services.StationSeed{Kind: services.SeedSong, ID: "11", Name: "Airbag", Subtitle: "Radiohead", ArtistID: "7"},
			[]string{"Music like ", "Airbag", " by ", "Radiohead"},
			"/song/11",
		},
		{
			"album",
			services.StationSeed{Kind: services.SeedAlbum, ID: "12", Name: "OK Computer", Subtitle: "Radiohead", ArtistID: "7"},
			[]string{"Music like ", "OK Computer", " by ", "Radiohead"},
			"/album/12",
		},
		{
			"artist",
			services.StationSeed{Kind: services.SeedArtist, ID: "7", Name: "Radiohead"},
			[]string{"Music like ", "Radiohead"},
			"/artist/7",
		},
		{
			"only an artist",
			services.StationSeed{Kind: services.SeedArtist, ID: "7", Name: "Radiohead", Only: true},
			[]string{"Only songs by ", "Radiohead"},
			"/artist/7",
		},
	} {
		rg := stationRig(t, seedDetail(c.seed))
		rg.tt.Until(c.name, rg.tt.Shows(c.want...))
		// The seed's name leads to its page.
		rg.click(c.seed.Name)
		if rg.router.Path() != c.path {
			t.Errorf("%s: the name went to %q, want %q", c.name, rg.router.Path(), c.path)
		}
	}
	// The artist of a song is a link of its own.
	rg := stationRig(t, seedDetail(services.StationSeed{Kind: services.SeedSong, ID: "11", Name: "Airbag", Subtitle: "Radiohead", ArtistID: "7"}))
	rg.tt.Until("the byline", rg.tt.Shows("Radiohead"))
	rg.click("Radiohead")
	if rg.router.Path() != "/artist/7" {
		t.Errorf("the artist went to %q", rg.router.Path())
	}
}

func TestSeedPartsKeepTheMessagesOrderAndMarkup(t *testing.T) {
	for _, c := range []struct {
		msg  string
		want []part
	}{
		{"<0>Music like </0>\ue000", []part{{text: "Music like ", muted: true}, {mark: "\ue000"}}},
		{"<0>Music like </0>\ue000<1> by </1>\ue001", []part{
			{text: "Music like ", muted: true}, {mark: "\ue000"}, {text: " by ", muted: true}, {mark: "\ue001"},
		}},
		// A language puts the words where it wants them.
		{"\ue001 <0>peut-être</0> \ue000!", []part{{mark: "\ue001"}, {text: " ", muted: false}, {text: "peut-être", muted: true}, {text: " ", muted: false}, {mark: "\ue000"}, {text: "!"}}},
		{"plain", []part{{text: "plain"}}},
	} {
		if got := seedParts(c.msg); !slices.Equal(got, c.want) {
			t.Errorf("seedParts(%q) = %+v, want %+v", c.msg, got, c.want)
		}
	}
}

func TestStationPlayStartsTheStationAndReadsItAgain(t *testing.T) {
	s := trackStation("ra.78", "Radiohead Station")
	rg := stationRig(t, services.StationDetail{Station: s})
	rg.tt.Until("the page", rg.tt.Shows("Radiohead Station"))
	if n := rg.api.count("station ra.78"); n != 1 {
		t.Fatalf("the station was read %d times", n)
	}
	rg.click("Play Radiohead Station")
	rg.tt.Until("the play", func() bool { return slices.Equal(rg.api.plays(), []string{"ra.78"}) })
	// "Plays first" lists the songs after the played batch: the station is
	// read again once it started.
	rg.tt.Until("the station read again", func() bool { return rg.api.count("station ra.78") == 2 })
}

func TestStationPlayThatFailsSaysWhyAndReadsNothingAgain(t *testing.T) {
	s := trackStation("ra.78", "Radiohead Station")
	for _, c := range []struct {
		err  error
		want string
	}{
		{fmt.Errorf("play: %w", applemusic.ErrNotFound), "Apple Music has no station for this."},
		{fmt.Errorf("play: %w", auth.ErrNotSignedIn), "Sign in to Apple Music from the account menu in the top bar."},
		{fmt.Errorf("play: %w", auth.ErrMUTRejected), "Sign in to Apple Music from the account menu in the top bar."},
		{fmt.Errorf("%s: %w", "x", applemusic.ErrNotPlayable), "This song isn't available to stream in your storefront."},
		{errors.New("boom"), "Something went wrong."},
	} {
		rg := stationRig(t, services.StationDetail{Station: s}, fake(func(f *fakeRadio) { f.playErr = c.err }))
		rg.tt.Until("the page", rg.tt.Shows("Radiohead Station"))
		rg.click("Play Radiohead Station")
		rg.toastShown("Couldn't play this station")
		rg.tt.Until("the reason", rg.tt.Shows(c.want))
		if n := rg.api.count("station ra.78"); n != 1 {
			t.Errorf("%v: the station was read again after a failed play (%d)", c.err, n)
		}
	}
}

func TestStationPageSaysWhyAStationCannotBePlayed(t *testing.T) {
	off := liveStation("ra.4", "Off", func(s *services.Station) { s.Playable, s.Unavailable = false, services.StationNoWidevine })
	rg := stationRig(t, services.StationDetail{Station: off})
	rg.tt.Until("the note", rg.tt.Shows("Sona can't play this station",
		"This stream isn't offered with Widevine, the only protection Sona can play."))
	// The play control is off.
	_ = rg.tt.Click("Play Off")
	rg.tt.Drain()
	if got := rg.api.plays(); len(got) != 0 {
		t.Errorf("an unplayable station was played: %v", got)
	}
	playable := stationRig(t, services.StationDetail{Station: liveStation("ra.5", "On")})
	playable.tt.Until("the page", playable.tt.Shows("On"))
	if playable.tt.HasText("Sona can't play this station") {
		t.Error("a playable station carries the note")
	}
}

func TestStationPageOfABroadcasterSaysWhatIsOnAirWhileItPlays(t *testing.T) {
	kiss := broadcaster("ra.kiss", "KISS")
	rg := stationRig(t, services.StationDetail{Station: kiss})
	rg.tt.Until("the section", rg.tt.Shows("On air", "Play the station to see what is on air."))
	// Tuned, with nothing named: it says so.
	rg.playState(services.PlaybackState{
		State: "playing", Current: &services.QueueItem{ID: "ra.kiss", Live: true, Broadcast: true},
		Broadcaster: &services.BroadcasterInfo{Genre: "Pop", Location: "London", Description: "Hits all day."},
	})
	rg.tt.Until("tuned", rg.tt.Shows("This station isn't naming what it is playing.", "Pop · London", "Hits all day."))
	if rg.tt.HasText("Play the station to see what is on air.") {
		t.Error("the prompt to play stayed while it plays")
	}
	// What its stream names on air.
	rg.playState(services.PlaybackState{
		State: "playing", Current: &services.QueueItem{ID: "ra.kiss", Live: true, Broadcast: true},
		OnAir: &services.OnAir{Artist: "Dua Lipa", Title: "Houdini"},
	})
	rg.tt.Until("on air", rg.tt.Shows("Dua Lipa – Houdini"))
	if rg.tt.HasText("Pop · London") {
		t.Error("the broadcaster's profile shows without it being sent")
	}
	// Another station playing: back to the prompt, and no profile.
	rg.playState(services.PlaybackState{
		State: "playing", Current: &services.QueueItem{ID: "ra.other", Live: true},
		Broadcaster: &services.BroadcasterInfo{Genre: "Rock"},
	})
	rg.tt.Until("another", rg.tt.Shows("Play the station to see what is on air."))
	if rg.tt.HasText("Rock") {
		t.Error("another station's profile shows")
	}
}

func TestStationPageOnlyBroadcastersHaveAnOnAirSection(t *testing.T) {
	// A broadcaster Sona cannot play has the note, not the section.
	off := broadcaster("ra.kiss", "KISS")
	off.Playable, off.Unavailable = false, ""
	rg := stationRig(t, services.StationDetail{Station: off})
	rg.tt.Until("the note", rg.tt.Shows("Sona can't play this station"))
	if rg.tt.HasText("Play the station to see what is on air.") {
		t.Error("an unplayable broadcaster has an On air section")
	}
}

func TestStationPageListsTheSongsItPlaysFirst(t *testing.T) {
	s := trackStation("ra.78", "Radiohead Station")
	d := services.StationDetail{Station: s, Upcoming: []services.Song{
		{ID: "s1", Name: "Airbag", Artist: "Radiohead", ArtistID: "7", Album: "OK Computer", AlbumID: "al1", DurationMs: 284000},
		{ID: "s2", Name: "Orphan", Artist: "Someone", Album: "Loose", DurationMs: 61000},
	}}
	rg := stationRig(t, d)
	rg.tt.Until("the songs", rg.tt.Shows("Plays first", "Airbag", "Radiohead", "OK Computer", "4:44", "Orphan", "1:01"))
	// The song leads to its album, to the song, and the artist to theirs.
	rg.click("Airbag")
	if got := rg.router.Location(); got != "/album/al1?song=s1" {
		t.Errorf("the song went to %q", got)
	}
	rg.router.Push("/station/ra.78")
	rg.tt.Drain()
	rg.click("Radiohead")
	if rg.router.Path() != "/artist/7" {
		t.Errorf("the artist went to %q", rg.router.Path())
	}
	// A song with no album or artist ID is plain text.
	rg.router.Push("/station/ra.78")
	rg.tt.Drain()
	before := rg.router.Location()
	_ = rg.tt.Click("Orphan")
	_ = rg.tt.Click("Someone")
	if rg.router.Location() != before {
		t.Errorf("plain text navigated to %q", rg.router.Location())
	}
}

func scheduleDetail(s services.Station, t0 time.Time) services.StationDetail {
	return services.StationDetail{Station: s, Schedule: []services.StationEvent{
		stationEvent("Ended", t0.Add(-2*time.Hour), t0, "Already over."),
		stationEvent("The Zane Lowe Show", t0, t0.Add(time.Hour), "New music with Zane."),
		stationEvent("The Matt Wilkinson Show", t0.Add(time.Hour), t0.Add(2*time.Hour), "Matt plays the best of the week."),
	}}
}

func TestStationPageSchedulesTheProgrammesThatHaveNotEnded(t *testing.T) {
	t0 := base()
	s := liveStation("ra.1", "Apple Music 1")
	rg := stationRig(t, scheduleDetail(s, t0))
	rg.tt.Until("the schedule", rg.tt.Shows("Schedule", "The Zane Lowe Show", "The Matt Wilkinson Show"))
	if rg.tt.HasText("Ended") || rg.tt.HasText("Already over.") {
		t.Errorf("a programme that ended shows: %q", rg.tt.Texts())
	}
	// Each programme's start is at its rail; the one on air says how long is left.
	for _, want := range []string{clockAt(t0.UnixMilli()), clockAt(t0.Add(time.Hour).UnixMilli()), "45 min left", "New music with Zane.", "Matt plays the best of the week."} {
		if !rg.tt.HasText(want) {
			t.Errorf("the schedule lacks %q: %q", want, rg.tt.Texts())
		}
	}
	// The one on air says so, and has the one progress bar.
	if n := rg.count("On air"); n != 1 {
		t.Errorf("%d \"On air\" labels, want 1", n)
	}
	if _, ok := rg.tt.Find("Time through this programme"); !ok {
		t.Error("no progress bar on the programme on air")
	}
}

func TestStationScheduleMovesOnEvery30Seconds(t *testing.T) {
	t0 := base()
	s := liveStation("ra.1", "Apple Music 1")
	rg := stationRig(t, scheduleDetail(s, t0), func(rg *rig) { rg.now = t0.Add(time.Hour - 10*time.Second) })
	rg.tt.Until("the schedule", rg.tt.Shows("The Zane Lowe Show"))
	onAirRow := func() float32 {
		r, ok := rg.tt.Find("On air")
		if !ok {
			t.Fatal("nothing is on air")
		}
		return r.Y
	}
	first := onAirRow()
	rg.advance(20 * time.Second)
	if rg.count("The Zane Lowe Show") == 0 || onAirRow() != first {
		t.Errorf("the schedule moved before 30 seconds: %q", rg.tt.Texts())
	}
	rg.advance(10 * time.Second)
	// The first programme has ended: it leaves, the next is on air.
	if rg.count("The Zane Lowe Show") != 0 {
		t.Errorf("a programme that ended stayed: %q", rg.tt.Texts())
	}
	// The mark is beside the programme that is on air now.
	title, _ := rg.tt.Find("The Matt Wilkinson Show")
	if d := onAirRow() - title.Y; d < 0 || d > 60 {
		t.Errorf("the on-air mark is %v DIPs from the programme on air", d)
	}
	if n := rg.count("On air"); n != 1 {
		t.Errorf("%d \"On air\" labels, want 1", n)
	}
}

func TestIdentitySizeIsOneOfTwoClassesByContentWidth(t *testing.T) {
	for _, c := range []struct{ content, want float32 }{
		{-20, 264}, // the first frame, before the width is known
		{800, 264},
		{1132, 264},
		{1499, 264},
		{1500, 320},
		{1688, 320},
		{2400, 320},
	} {
		if got := identitySize(c.content); got != c.want {
			t.Errorf("identitySize(%v) = %v, want %v", c.content, got, c.want)
		}
	}
	// Both classes, and the lists beside them at their widest, fit in the
	// narrowest content that takes the larger class: the two columns stay
	// together instead of the lists being squeezed.
	if identityWide+identityGap+listsMax > identityWideAt {
		t.Errorf("%d+%d+%d does not fit in %d", identityWide, identityGap, listsMax, identityWideAt)
	}
}
