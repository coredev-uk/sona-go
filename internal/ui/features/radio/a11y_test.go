package radio

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// What assistive technology reads of Radio's pages, and the keyboard's way
// through them.

// walk is the labels of the Tab stops of the window, in order; the walk
// fails the test on a trap and on a stop that is not on the tree.
func walk(t *testing.T, rg *rig) []string {
	t.Helper()
	var labels []string
	for _, s := range access.TabWalk(t, rg.tt.Tester, 300) {
		labels = append(labels, s.Node.Label)
	}
	return labels
}

// inOrder fails the test unless want is a subsequence of got.
func inOrder(t *testing.T, got []string, want ...string) {
	t.Helper()
	at := 0
	for _, w := range want {
		i := slices.Index(got[at:], w)
		if i < 0 {
			t.Errorf("%q is not a Tab stop after %q; stops %q", w, got[max(0, at-1)], got)
			return
		}
		at += i + 1
	}
}

// withStationActions gives stations a menu, so that every row and tile builds
// its more button.
func withStationActions(rg *rig) {
	rg.h.Env.Actions.Register(kit.ActionDef{
		ID: "test.station", Target: kit.KindStation, Group: kit.GroupItem,
		Label: func(*kit.ActionContext) string { return "Do something" },
		Run:   func(*kit.ActionContext) {},
	})
}

// radioWithSchedule is the Radio page signed in, with a schedule for the first
// live station, a station of the user's and a broadcaster with no page on the
// board.
func radioWithSchedule(t *testing.T, extra ...func(*rig)) *rig {
	t.Helper()
	t0 := base()
	one := appleMusic1
	kiss := liveStation("ra.kiss", "KISS FM", func(s *services.Station) { s.HasPage = false })
	setup := append([]func(*rig){radioFixture, signedIn, fake(func(f *fakeRadio) {
		f.live = []services.Station{appleMusic1, appleHits, kiss}
		f.recent = []services.Station{trackStation("ra.mine", "My Station")}
		f.details["ra.1"] = services.StationDetail{Station: one, Schedule: []services.StationEvent{
			stationEvent("The Zane Lowe Show", t0, t0.Add(time.Hour)),
			stationEvent("The Matt Wilkinson Show", t0.Add(2*time.Hour), t0.Add(3*time.Hour)),
		}}
		f.details["ra.kiss"] = services.StationDetail{Station: kiss, Schedule: []services.StationEvent{}}
	}), withStationActions}, extra...)
	rg := newRig(t, radioPath, 1132, 1253, setup...)
	rg.tt.Until("the page", rg.tt.Shows("Local Broadcasters", "Latest Episodes", "Apple Music 1", "My Station", "The Zane Lowe Show"))
	return rg
}

func TestA11yRadioPageHasNoControlInsideAnotherAndItsListsHoldOnlyItems(t *testing.T) {
	rg := radioWithSchedule(t)
	access.NoNestedControls(t, rg.tt.Tester)
	tr := access.Read(t, rg.tt.Tester)

	for label, level := range map[string]int{
		"Radio": 1, "Live now": 2, "Local Broadcasters": 2, "Stations": 2, "Your stations": 2, "Stations by Genre": 2,
		"Shows and episodes": 2, "Latest Episodes": 2,
	} {
		if h, ok := tr.Find(ui.RoleHeading, label); !ok {
			t.Errorf("no heading %q", label)
		} else if h.Level != level {
			t.Errorf("heading %q is level %d, want %d", label, h.Level, level)
		}
	}
	for _, label := range []string{"Live now", "Local Broadcasters", "Stations", "Shows and episodes", "Latest Episodes"} {
		if _, ok := tr.Find(ui.RoleGroup, label); !ok {
			t.Errorf("no group %q", label)
		}
	}
	// The board is a list of items, each with its link, its play button and its text.
	board, ok := tr.Find(ui.RoleList, "Live now")
	if !ok {
		t.Fatal("no list of the live stations")
	}
	var names []string
	for _, row := range tr.Children(board) {
		if row.Role != ui.RoleListItem {
			t.Errorf("the board holds %v, not a list item", row)
			continue
		}
		names = append(names, row.Label)
	}
	if len(names) != 3 {
		t.Errorf("the board has %d rows (%q), want 3", len(names), names)
	}
	// A station with a page: a link and a play button beside it, named for the station.
	link, ok := tr.Find(ui.RoleLink, "Apple Music 1")
	if !ok || !link.Has(access.Focusable) {
		t.Errorf("no focusable link for the station: %v", link)
	}
	play, ok := tr.Find(ui.RoleButton, "Play Apple Music 1")
	if !ok || !play.Has(access.Focusable) {
		t.Errorf("no focusable play button for the station: %v", play)
	}
	// One with no page: a button named by its text then the action, and a play button beside it.
	if b, ok := tr.Find(ui.RoleButton, "KISS FM Play"); !ok || !b.Has(access.Focusable) {
		t.Errorf("no play button named by the station's text: %v", b)
	}
	if b, ok := tr.Find(ui.RoleButton, "Play KISS FM"); !ok || !b.Has(access.Focusable) {
		t.Errorf("no play button for the station with no page: %v", b)
	}
	// The progress of the programme on air: a progress indicator with its range and what is left.
	prog, ok := tr.Find(ui.RoleProgress, "Time through this programme")
	if !ok {
		t.Fatalf("no progress indicator:\n%s", tr.Dump())
	}
	if prog.Max != 100 || prog.Now != 25 || !strings.HasSuffix(prog.Value, "left") {
		t.Errorf("the progress reads %v..%v, now %v, %q", prog.Min, prog.Max, prog.Now, prog.Value)
	}
	// The tiles of the user's stations are a table of rows.
	if _, ok := tr.Find(ui.RoleTable, "Your stations"); !ok {
		t.Error("no table of the user's stations")
	}
}

func TestA11yAProgrammesBoxIsALinkNamedForItsSlotThatIsNoTabStop(t *testing.T) {
	rg := radioWithSchedule(t)
	t0 := base()
	name := "The Zane Lowe Show, " + clockAt(t0.UnixMilli()) + "–" + clockAt(t0.Add(time.Hour).UnixMilli())
	l, ok := access.Read(t, rg.tt.Tester).Find(ui.RoleLink, name)
	if !ok || !l.Has(access.Focusable) {
		t.Fatalf("no link %q", name)
	}
	// The station's own link is the one stop: the boxes are reached by the
	// pointer and by screen readers' link lists.
	if slices.Contains(walk(t, rg), name) {
		t.Errorf("a programme's box is a Tab stop")
	}
}

func TestA11yRadioTabWalkGoesBoardThenBroadcastersThenBandsWithoutATrap(t *testing.T) {
	rg := radioWithSchedule(t)
	stops := walk(t, rg)
	inOrder(t, stops,
		"Apple Music 1", "Play Apple Music 1", "Hits", "Play Hits", "KISS FM Play",
		"My Station", "Play My Station", "Pop", "Episode One")
	if slices.Contains(stops, "More for KISS FM") || slices.Contains(stops, "More for My Station") {
		t.Errorf("a more button is a Tab stop: %q", stops)
	}
}

func TestA11yShiftF10OnAStationWithNoPageOpensItsMenuAndEscapeReturnsTheFocus(t *testing.T) {
	rg := radioWithSchedule(t)
	for range 8 {
		rg.tt.Key(0, ui.KeyTab)
		if rg.tt.Focused("KISS FM Play") {
			break
		}
	}
	if !rg.tt.Focused("KISS FM Play") {
		t.Fatal("Tab did not reach the station's button")
	}
	rg.tt.Key(ui.Shift, ui.KeyF10)
	rg.tt.Drain()
	if !rg.tt.HasText("Do something") {
		t.Fatalf("Shift+F10 opened no menu; texts %q", rg.tt.Texts())
	}
	rg.tt.Key(0, ui.KeyEscape)
	rg.advance(300 * time.Millisecond)
	if rg.tt.HasText("Do something") {
		t.Fatal("Escape did not close the menu")
	}
	if !rg.tt.Focused("KISS FM Play") {
		t.Error("the focus did not return to the station's button")
	}
}

func TestA11yTunedStationHasNoPlayButtonAndTheButtonNamesPause(t *testing.T) {
	kiss := liveStation("ra.kiss", "KISS FM", func(s *services.Station) { s.HasPage = false })
	rg := boardRig(t, []services.Station{appleMusic1, kiss}, map[string]services.StationDetail{"ra.kiss": {Station: kiss, Schedule: []services.StationEvent{}}})
	rg.playing("ra.kiss", "playing")
	rg.tt.Until("the rows", rg.tt.Shows("Apple Music 1", "KISS FM", "Now playing"))
	tr := access.Read(t, rg.tt.Tester)
	if _, ok := tr.Find(ui.RoleButton, "KISS FM Pause"); !ok {
		t.Errorf("the tuned station's button does not name Pause:\n%s", tr.Dump())
	}
	if _, ok := tr.Find(ui.RoleButton, "KISS FM Play"); ok {
		t.Error("the tuned station still says Play")
	}
	access.NoNestedControls(t, rg.tt.Tester)
}

func TestA11yAStationThatCannotPlayIsADisabledButtonOrALinkWithItsReasonRead(t *testing.T) {
	off := liveStation("ra.4", "Off", func(s *services.Station) { s.Playable, s.Unavailable = false, services.StationNoWidevine })
	offNoPage := liveStation("ra.5", "Plain", func(s *services.Station) { s.Playable, s.HasPage = false, false })
	rg := boardRig(t, []services.Station{off, offNoPage}, nil)
	rg.tt.Until("the rows", rg.tt.Shows("Off", "Plain"))
	tr := access.Read(t, rg.tt.Tester)
	if l, ok := tr.Find(ui.RoleLink, "Off"); !ok || l.Has(access.Disabled) {
		t.Errorf("a station with a page is a link that opens it even if it cannot play: %v", l)
	}
	if b, ok := tr.Find(ui.RoleButton, "Plain Play"); !ok || !b.Has(access.Disabled) {
		t.Errorf("a station with no page that cannot play is not a disabled button: %v\n%s", b, tr.Dump())
	}
	// The reason is on the tree as text, in the row.
	for _, reason := range []string{"This stream isn't offered with Widevine, the only protection Sona can play.", "Sona can't play this station"} {
		found := false
		for _, n := range tr.Roles(ui.RoleText) {
			found = found || n.Label == reason
		}
		if !found {
			t.Errorf("the reason %q is not on the tree", reason)
		}
	}
	inOrder(t, walk(t, rg), "Off")
}

func TestA11yRadioLoadingFramesAreStatusesAndFailuresAreAnnouncedOnce(t *testing.T) {
	hold := make(chan struct{})
	rg := newRig(t, radioPath, 1132, 1253, radioFixture, fake(func(f *fakeRadio) { f.hold = hold }))
	rg.tt.Frame()
	tr := access.Read(t, rg.tt.Tester)
	if n := len(tr.FindAll(ui.RoleStatus, "Loading")); n != 2 {
		t.Errorf("%d statuses labelled Loading, want one for the board and one for the bands:\n%s", n, tr.Dump())
	}
	close(hold)
	rg.tt.Until("the data", rg.tt.Shows("Apple Music 1 tagline", "Latest Episodes"))

	bad := newRig(t, radioPath, 1132, 1253, fake(func(f *fakeRadio) { f.liveErr, f.groupingErr = errors.New("offline"), errors.New("offline") }))
	bad.tt.Until("the block", bad.tt.Shows("Couldn't load Apple Music's radio", "Try again"))
	if _, ok := access.Read(t, bad.tt.Tester).Find(ui.RoleAlert, "Couldn't load Apple Music's radio"); !ok {
		t.Error("the failure is not an alert")
	}
	if said := bad.tt.Announcements(); slices.ContainsFunc(said, func(s string) bool { return strings.HasPrefix(s, "Couldn't load Apple Music's radio") }) {
		t.Errorf("the failure was announced as well as being an alert: %q", said)
	}
	for range 3 {
		bad.tt.Drain()
	}
	if again := bad.tt.Announcements(); len(again) != 0 {
		t.Errorf("the failure was announced again: %q", again)
	}
	inOrder(t, walk(t, bad), "Try again")
}

func TestA11yStationPageReadsAsAHeadingAPlayButtonAndItsSections(t *testing.T) {
	t0 := base()
	s := liveStation("ra.1", "Apple Music 1")
	rg := stationRig(t, scheduleDetail(s, t0), withStationActions)
	rg.tt.Until("the schedule", rg.tt.Shows("Schedule", "The Zane Lowe Show", "The Matt Wilkinson Show"))
	access.NoNestedControls(t, rg.tt.Tester)
	tr := access.Read(t, rg.tt.Tester)

	if h, ok := tr.Find(ui.RoleHeading, "Apple Music 1"); !ok || h.Level != 1 {
		t.Errorf("no level 1 heading of the station: %v", h)
	}
	if h, ok := tr.Find(ui.RoleHeading, "Schedule"); !ok || h.Level != 2 {
		t.Errorf("no level 2 heading Schedule: %v", h)
	}
	if _, ok := tr.Find(ui.RoleGroup, "Schedule"); !ok {
		t.Error("the schedule is not a group named by its heading")
	}
	list, ok := tr.Find(ui.RoleList, "Schedule")
	if !ok {
		t.Fatal("no list of the programmes")
	}
	rows := tr.Children(list)
	if len(rows) != 2 {
		t.Fatalf("%d programmes, want the two that have not ended", len(rows))
	}
	for _, row := range rows {
		if row.Role != ui.RoleListItem {
			t.Errorf("the schedule holds %v, not a list item", row)
		}
	}
	// The programme on air says so, in its row's name, and carries the progress.
	if !strings.Contains(rows[0].Label, "On air") || !strings.Contains(rows[0].Label, "The Zane Lowe Show") {
		t.Errorf("the first row is named %q", rows[0].Label)
	}
	if strings.Contains(rows[1].Label, "On air") {
		t.Errorf("the second row says it is on air: %q", rows[1].Label)
	}
	if n := len(tr.Roles(ui.RoleProgress)); n != 1 {
		t.Errorf("%d progress indicators, want 1 (on the programme on air)", n)
	}
	for _, label := range []string{"Play Apple Music 1"} {
		if b, ok := tr.Find(ui.RoleButton, label); !ok || !b.Has(access.Focusable) {
			t.Errorf("%q is not a focusable button: %v", label, b)
		}
	}
	if m, ok := tr.Find(ui.RoleMenuButton, "More for Apple Music 1"); !ok || !m.Has(access.Focusable) {
		t.Errorf("the header's more button is not a stop: %v", m)
	}
	inOrder(t, walk(t, rg), "Play Apple Music 1", "More for Apple Music 1")
}

func TestA11yStationPageSectionsAreGroupsNamedByTheirHeadings(t *testing.T) {
	// The songs a track station plays first: a list of items, with links to their pages.
	s := trackStation("ra.78", "Radiohead Station")
	d := services.StationDetail{Station: s, Upcoming: []services.Song{
		{ID: "s1", Name: "Airbag", Artist: "Radiohead", ArtistID: "7", Album: "OK Computer", AlbumID: "al1", DurationMs: 284000},
		{ID: "s2", Name: "Orphan", Artist: "Someone", Album: "Loose", DurationMs: 61000},
	}}
	rg := stationRig(t, d)
	rg.tt.Until("the songs", rg.tt.Shows("Plays first", "Airbag"))
	access.NoNestedControls(t, rg.tt.Tester)
	tr := access.Read(t, rg.tt.Tester)
	if _, ok := tr.Find(ui.RoleGroup, "Plays first"); !ok {
		t.Error("no group Plays first")
	}
	list, ok := tr.Find(ui.RoleList, "Plays first")
	if !ok {
		t.Fatal("no list of the songs")
	}
	for _, it := range tr.Children(list) {
		if it.Role != ui.RoleListItem {
			t.Errorf("the songs hold %v, not a list item", it)
		}
	}
	if l, ok := tr.Find(ui.RoleLink, "Airbag"); !ok || !l.Has(access.Focusable) {
		t.Errorf("the song is not a link: %v", l)
	}
	if _, ok := tr.Find(ui.RoleLink, "Orphan"); ok {
		t.Error("a song with no album page is a link")
	}
	inOrder(t, walk(t, rg), "Play Radiohead Station", "Airbag", "Radiohead")

	// A broadcaster: the "On air" section is a group with a heading.
	kiss := stationRig(t, services.StationDetail{Station: broadcaster("ra.kiss", "KISS")})
	kiss.tt.Until("the section", kiss.tt.Shows("On air", "Play the station to see what is on air."))
	ktr := access.Read(t, kiss.tt.Tester)
	if _, ok := ktr.Find(ui.RoleGroup, "On air"); !ok {
		t.Error("no group On air")
	}
	if h, ok := ktr.Find(ui.RoleHeading, "On air"); !ok || h.Level != 2 {
		t.Errorf("no level 2 heading On air: %v", h)
	}

	// A station Sona cannot play: the note is a group named for it.
	off := liveStation("ra.4", "Off", func(s *services.Station) { s.Playable, s.Unavailable = false, services.StationNoWidevine })
	no := stationRig(t, services.StationDetail{Station: off})
	no.tt.Until("the note", no.tt.Shows("Sona can't play this station"))
	ntr := access.Read(t, no.tt.Tester)
	if _, ok := ntr.Find(ui.RoleGroup, "Sona can't play this station"); !ok {
		t.Errorf("the note is not a group:\n%s", ntr.Dump())
	}
	if b, ok := ntr.Find(ui.RoleButton, "Play Off"); !ok || !b.Has(access.Disabled) {
		t.Errorf("the play button of a station that cannot play is not disabled: %v", b)
	}
}

func TestA11yStationLoadingIsAStatusAndFailuresAreSaidWithTheirReason(t *testing.T) {
	hold := make(chan struct{})
	rg := stationRig(t, services.StationDetail{Station: liveStation("ra.1", "Apple Music 1")}, fake(func(f *fakeRadio) { f.hold = hold }))
	rg.tt.Frame()
	if _, ok := access.Read(t, rg.tt.Tester).Find(ui.RoleStatus, "Loading"); !ok {
		t.Errorf("the loading station is not a status:\n%s", access.Read(t, rg.tt.Tester).Dump())
	}
	close(hold)

	bad := stationRig(t, services.StationDetail{Station: liveStation("ra.1", "Apple Music 1")}, fake(func(f *fakeRadio) { f.detailErr = errors.New("offline") }))
	bad.tt.Until("the error", bad.tt.Shows("Couldn't load this station", "Try again"))
	if _, ok := access.Read(t, bad.tt.Tester).Find(ui.RoleAlert, "Couldn't load this station"); !ok {
		t.Error("the failure is not an alert")
	}
	if said := bad.tt.Announcements(); slices.ContainsFunc(said, func(s string) bool { return strings.HasPrefix(s, "Couldn't load this station") }) {
		t.Errorf("the failure was announced as well as being an alert: %q", said)
	}

	// A play that fails is a toast, which says so once.
	failing := stationRig(t, services.StationDetail{Station: trackStation("ra.78", "Radiohead Station")}, fake(func(f *fakeRadio) { f.playErr = errors.New("boom") }))
	failing.tt.Until("the page", failing.tt.Shows("Radiohead Station"))
	failing.tt.Announcements()
	failing.click("Play Radiohead Station")
	failing.toastShown("Couldn't play this station")
	if said := failing.tt.Announcements(); !slices.ContainsFunc(said, func(s string) bool { return strings.Contains(s, "Couldn't play this station") }) {
		t.Errorf("the failed play was announced as %q", said)
	}
}
