package kit

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
)

func station(id, name, provider string) services.Station {
	return services.Station{
		ID: id, Name: name, Provider: provider, Kind: services.StationBroadcast, Playable: true,
		Art: services.Art{Small: "/art/k" + id + "/96x96.jpg"},
	}
}

// stationApp is a station list on a page 600 wide: two columns.
func stationApp(t *testing.T, stations []services.Station, current string, onAir *services.OnAir) (*itemsApp, *ui.Tester, *[]string) {
	t.Helper()
	var played []string
	st := &StationListState{}
	a, tt := newItemsApp(t, 600, 400, nil, func(c *ui.Context, ctx ItemsCtx) {
		StationList(c, st, stations, ctx, StationListOpts{
			Label:  "Local Broadcasters",
			OnPlay: func(s services.Station) { played = append(played, s.ID) },
		})
	})
	a.tweak = func(ctx *ItemsCtx) {
		ctx.CurrentStation = func(s services.Station) bool { return current != "" && s.ID == current }
		ctx.Playing, ctx.OnAir = true, onAir
	}
	a.load(t, tt, 0)
	return a, tt, &played
}

var (
	capital = station("ra.1345165422", "Capital", "")
	heart   = station("ra.1", "Heart UK", "TuneIn")
)

func TestStationListListsEachStationWithItsProviderAndAPlayControl(t *testing.T) {
	_, tt, _ := stationApp(t, []services.Station{capital, heart}, "", nil)
	if _, ok := tt.Find("Local Broadcasters"); !ok {
		t.Errorf("the list is not named by its label: %q", tt.Texts())
	}
	if !tt.HasText("TuneIn") {
		t.Errorf("no provider line: %q", tt.Texts())
	}
	for _, name := range []string{"Capital Play", "Heart UK Play"} {
		if _, ok := tt.Find(name); !ok {
			t.Errorf("no %q: %q", name, tt.Texts())
		}
	}
	// Two columns of 260 or more: Heart sits to the right of Capital.
	a, _ := tt.Find("Capital Play")
	b, _ := tt.Find("Heart UK Play")
	if b.X <= a.X || b.Y != a.Y {
		t.Errorf("Capital %v, Heart UK %v: not side by side", a, b)
	}
}

func TestStationListPlaysAStationWithNoPageWhenItIsOpenedAndOffersItsMenu(t *testing.T) {
	a, tt, played := stationApp(t, []services.Station{capital, heart}, "", nil)
	// No link to a page; the name is itself the play button.
	for _, s := range []string{"Capital", "Heart UK"} {
		if r, ok := tt.Find(s); !ok || r.W == 0 {
			t.Errorf("no name %q", s)
		}
	}
	if err := tt.Click("Capital Play"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(*played, []string{capital.ID}) || len(a.rec.nav) != 0 {
		t.Errorf("played %v, navigated %v", *played, a.rec.nav)
	}
	if _, ok := tt.Find("More for Capital"); !ok {
		t.Errorf("no menu: %q", tt.Texts())
	}
}

func TestStationListLinksAStationThatHasAPageAndPlaysItFromItsOwnButton(t *testing.T) {
	apple := capital
	apple.ID, apple.Name, apple.HasPage = "ra.9", "Apple FM", true
	a, tt, played := stationApp(t, []services.Station{apple}, "", nil)
	if err := tt.Click("Apple FM"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.rec.nav, []string{"/station/ra.9"}) {
		t.Errorf("the name opened %v", a.rec.nav)
	}
	if err := tt.Click("Play Apple FM"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(*played, []string{"ra.9"}) {
		t.Errorf("played %v", *played)
	}
	// Focus stays in the row, on its link, as the equaliser replaces the button.
	if !tt.Focused("Apple FM") {
		t.Errorf("focus is not on the station's link")
	}
	if _, ok := tt.Find("More for Apple FM"); ok {
		t.Error("a station with a page has its menu in the row")
	}
}

func TestStationListMarksThePlayingStationPausesItFromItsNameAndShowsWhatIsOnAir(t *testing.T) {
	a, tt, played := stationApp(t, []services.Station{capital, heart}, capital.ID, &services.OnAir{Title: "Sample Song", Artist: "Sample Artist"})
	if _, ok := tt.Find("Capital Play"); ok {
		t.Error("the playing station is still named Play")
	}
	if !tt.HasText("Sample Artist – Sample Song") {
		t.Errorf("no on-air line: %q", tt.Texts())
	}
	// The station that is not playing keeps its provider line.
	if !tt.HasText("TuneIn") {
		t.Errorf("no provider line: %q", tt.Texts())
	}
	if err := tt.Click("Capital Pause"); err != nil {
		t.Fatal(err)
	}
	if a.rec.toggles != 1 || len(*played) != 0 {
		t.Errorf("toggled %d times, played %v", a.rec.toggles, *played)
	}
}

func TestStationListPausedStationIsNamedPlayAndStillToggles(t *testing.T) {
	a, tt, _ := stationApp(t, []services.Station{capital}, capital.ID, nil)
	a.tweak = func(ctx *ItemsCtx) {
		ctx.CurrentStation = func(s services.Station) bool { return s.ID == capital.ID }
		ctx.Playing = false
	}
	tt.Frame()
	if err := tt.Click("Capital Play"); err != nil {
		t.Fatal(err)
	}
	if a.rec.toggles != 1 {
		t.Errorf("toggled %d times", a.rec.toggles)
	}
}

func TestStationListUnplayableStationsControlsAreDisabled(t *testing.T) {
	dead := capital
	dead.ID, dead.Name, dead.Playable, dead.Unavailable = "ra.7", "Locked FM", false, services.StationNoWidevine
	pageDead := dead
	pageDead.ID, pageDead.Name, pageDead.HasPage = "ra.8", "Locked Page", true
	a, tt, played := stationApp(t, []services.Station{dead, pageDead}, "", nil)
	_ = tt.Click("Locked FM Play")
	_ = tt.Click("Play Locked Page")
	if len(*played) != 0 {
		t.Errorf("played %v", *played)
	}
	// The link still opens the station's page.
	if err := tt.Click("Locked Page"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.rec.nav, []string{"/station/ra.8"}) {
		t.Errorf("nav %v", a.rec.nav)
	}
}

func TestStationListUnplayableStationsReasonShowsAsTheTitle(t *testing.T) {
	dead := capital
	dead.ID, dead.Name, dead.Playable, dead.Unavailable = "ra.7", "Locked FM", false, services.StationNoWidevine
	pageDead := dead
	pageDead.ID, pageDead.Name, pageDead.HasPage = "ra.8", "Locked Page", true
	reason := "This stream isn't offered with Widevine, the only protection Sona can play."
	for _, s := range []services.Station{dead, pageDead} {
		_, tt, _ := stationApp(t, []services.Station{s}, "", nil)
		if tt.HasText(reason) {
			t.Errorf("%s: the reason shows before the pointer rests on it", s.Name)
		}
		name, _ := tt.Find(s.Name)
		if s.HasPage {
			// The page's play button carries the reason; the link does not.
			btn, _ := tt.Find("Play " + s.Name)
			tt.Move(btn.X+btn.W/2, btn.Y+btn.H/2)
		} else {
			tt.Move(name.X+name.W/2, name.Y+name.H/2)
		}
		tt.Advance(2 * time.Second)
		if !tt.HasText(reason) {
			t.Errorf("%s: no title %q: %q", s.Name, reason, tt.Texts())
		}
	}
	// A playable station has none.
	_, tt, _ := stationApp(t, []services.Station{capital}, "", nil)
	name, _ := tt.Find("Capital")
	tt.Move(name.X+name.W/2, name.Y+name.H/2)
	tt.Advance(2 * time.Second)
	if tt.HasText(reason) {
		t.Error("a playable station shows a reason")
	}
}
