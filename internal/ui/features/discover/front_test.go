package discover

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// bannerOf is the wide art of card n: its own key at the hero shape.
func bannerOf(n int) string {
	return fmt.Sprintf("/art/0123456789abcdef0123456789ab%04d/2400x600.jpg", n)
}

// heroCards are n featured cards, "Card 1"…, of a kind that opens a page and
// has nothing to play.
func heroCards(n int) []services.Item {
	out := make([]services.Item, n)
	for i := range out {
		k := i + 1
		out[i] = services.Item{
			Kind: services.ItemCurator, ID: fmt.Sprint(k), Name: fmt.Sprintf("Card %d", k),
			Wide: services.Art{Medium: bannerOf(k), Large: bannerOf(k)},
		}
	}
	return out
}

// newPageWith is an editorial page of sections fetched just now.
func newPageWith(r *fakeAPI, now time.Time, sections ...services.Section) {
	if r.pages == nil {
		r.pages = map[string]services.EditorialPage{}
	}
	r.pages["new"] = services.EditorialPage{Kind: services.ItemGrouping, Title: "New", Sections: sections, FetchedAt: now.UnixMilli()}
}

func heroSection(cards []services.Item) services.Section {
	return services.Section{ID: "hero", Kind: services.SectionHero, Items: cards}
}

// wideWindow is 1276 DIPs wide, so the content column is 1200: the width of
// the reference page's tests (a 1440 window less the sidebar).
func wideWindow() rigOption { return withSize(1276, 900) }

// newFront is Discover with a front page of n cards, settled on its first
// frames.
func newFront(t *testing.T, n int, opts ...rigOption) *rig {
	t.Helper()
	api := &fakeAPI{}
	r := newRig(t, "/new", append([]rigOption{withAPI(api), wideWindow()}, opts...)...)
	newPageWith(api, r.h.Clock.Now(), heroSection(heroCards(n)))
	r.tt.Until("the front page", r.tt.Shows("Featured"))
	r.tt.Drain()
	r.tt.Drain()
	return r
}

func (r *rig) lead() int { return r.d.front.at }

// tick moves the tester's clock on.
func (r *rig) tick(d time.Duration) {
	r.t.Helper()
	r.advance(d)
}

// away moves the pointer off the hero and the focus off it.
func (r *rig) away() {
	r.t.Helper()
	r.tt.Move(2, 2)
	r.tt.ClickAt(2, 2)
	r.tt.Drain()
}

// leadBox is the box the lead's art is requested at.
func (r *rig) leadBox() [2]int {
	lay := layoutFor(r.d.newPg.pg.Inner(), r.d.newPg.pg.Height)
	return [2]int{int(lay.lead), int(lay.art)}
}

// tileBox is the box a queued card's art is requested at.
func (r *rig) tileBox() [2]int {
	lay := layoutFor(r.d.newPg.pg.Inner(), r.d.newPg.pg.Height)
	return [2]int{int(lay.tile), int(lay.tileArt)}
}

// showInLead presses the "Show in lead" button of the queued card named.
func (r *rig) showInLead(name string) {
	r.t.Helper()
	if err := r.tt.Click("Show in lead: " + name); err != nil {
		r.t.Fatalf("no Show in lead for %q: %v: %q", name, err, r.tt.Texts())
	}
	r.tt.Drain()
}

// tileCentre is the centre of the queued card named: only its tile shows
// the name while another card leads.
func (r *rig) tileCentre(name string) (float32, float32) {
	r.t.Helper()
	tile, ok := r.tt.Find(name)
	if !ok {
		r.t.Fatalf("no queued card %q: %q", name, r.tt.Texts())
	}
	return tile.X + tile.W/2, tile.Y + tile.H/2
}

func (r *rig) heroCentre() (float32, float32) {
	hero, ok := r.tt.Find("Featured")
	if !ok {
		r.t.Fatal("no hero")
	}
	return hero.X + hero.W/4, hero.Y + hero.H/3
}

func TestFrontLeadsWithTheFirstCardAndQueuesTheFirstFour(t *testing.T) {
	src := newCoverSource()
	r := newFrontWith(t, 16, src)
	for _, w := range []string{"Featured", "Card 1", "Card 2", "Card 4", "1 of 16", "Previous in Featured", "Next in Featured"} {
		if !r.tt.HasText(w) {
			t.Errorf("the front page lacks %q: %q", w, r.tt.Texts())
		}
	}
	// The queue is the first four, the lead among them.
	for _, w := range []string{"Card 5", "Card 16"} {
		if r.tt.HasText(w) {
			t.Errorf("the queue shows %q", w)
		}
	}
	// Apple's centre crop of the banner at the box the lead shows it in, and
	// at the box of a tile for a queued card.
	if box, got := r.leadBox(), src.requested(bannerOf(1)); !slices.Contains(got, box) {
		t.Errorf("the lead's art was asked for at %v, want %v", got, box)
	}
	r.tt.Until("a tile's art", func() bool { return slices.Contains(src.requested(bannerOf(3)), r.tileBox()) })
	if r.d.front.at != 0 {
		t.Errorf("the lead is %d, want the first card", r.d.front.at)
	}
}

func TestFrontRotatesToTheNextCardEveryIntervalAndWrapsRound(t *testing.T) {
	r := newFront(t, 8)
	r.tick(rotateEvery - time.Millisecond)
	if r.lead() != 0 {
		t.Fatalf("lead %d just before the interval ends", r.lead())
	}
	r.tick(time.Millisecond)
	if r.lead() != 1 {
		t.Fatalf("lead %d after an interval, want 1", r.lead())
	}
	for range 7 {
		r.tick(rotateEvery)
	}
	if r.lead() != 0 {
		t.Errorf("lead %d after a whole round, want it back at the first card", r.lead())
	}
}

// The lead's segment of the progress fills as its interval runs, and is a
// whole muted mark while the rotation holds.
func TestFrontProgressFillsTheLeadsSegmentAsTheIntervalRuns(t *testing.T) {
	r := newFront(t, 4)
	hero, ok := r.tt.Find("Featured")
	if !ok {
		t.Fatal("no hero")
	}
	lay := layoutFor(r.d.newPg.pg.Inner(), r.d.newPg.pg.Height)
	seg := (lay.lead - 3*segmentGap) / 4
	y := int(hero.Y + lay.art + progressGap + progressHeight/2)
	// filled is the share of the lead's segment drawn darker than its track,
	// and dark says whether the darkest of it is near the ink.
	filled := func() (share float64, dark bool) {
		img := r.tt.Image()
		n, lo := 0, 255
		x0, x1 := int(hero.X)+1, int(hero.X+seg)-1
		for x := x0; x < x1; x++ {
			c := img.RGBAAt(x, y)
			if c.R < 160 {
				n++
			}
			lo = min(lo, int(c.R))
		}
		return float64(n) / float64(x1-x0), lo < 80
	}
	r.tick(rotateEvery / 2)
	r.tt.Drain()
	if share, dark := filled(); share < 0.4 || share > 0.6 || !dark {
		t.Errorf("half way through the interval the lead's segment is %.0f%% filled (ink %v), want half in ink", share*100, dark)
	}
	x, yy := r.heroCentre()
	r.tt.Move(x, yy) // the pointer holds the rotation
	r.tt.Drain()
	if share, dark := filled(); share < 0.95 || dark {
		t.Errorf("held, the lead's segment is %.0f%% filled (ink %v), want a whole muted mark", share*100, dark)
	}
}

// The queue stays still while the lead goes through it, its tile marked, and
// moves on to start at the lead only when the lead leaves it.
func TestFrontQueueStaysStillUntilTheLeadLeavesIt(t *testing.T) {
	r := newFront(t, 8)
	shows := func(want, not []string) {
		t.Helper()
		for _, w := range want {
			if !r.tt.HasText(w) {
				t.Errorf("lead %d: lacks %q: %q", r.lead(), w, r.tt.Texts())
			}
		}
		for _, w := range not {
			if r.tt.HasText(w) {
				t.Errorf("lead %d: shows %q", r.lead(), w)
			}
		}
	}
	r.tick(rotateEvery)
	r.tick(rotateEvery)
	r.tt.Drain()
	if r.lead() != 2 || r.d.front.window != 0 {
		t.Fatalf("lead %d, window %d: the queue moved while the lead was in it", r.lead(), r.d.front.window)
	}
	shows([]string{"Card 1", "Card 2", "Card 3", "Card 4"}, []string{"Card 5"})
	r.tick(rotateEvery)
	r.tick(rotateEvery)
	r.tt.Drain()
	if r.lead() != 4 || r.d.front.window != 4 {
		t.Fatalf("lead %d, window %d: the queue did not move on when the lead left it", r.lead(), r.d.front.window)
	}
	shows([]string{"5 of 8", "Card 5", "Card 6", "Card 7", "Card 8"}, []string{"Card 1", "Card 4"})
	// Chosen out of the queue, the lead starts it again.
	r.d.front.choose(2)
	r.tt.Drain()
	if r.d.front.window != 2 {
		t.Errorf("window %d after the lead left it for the third card", r.d.front.window)
	}
	shows([]string{"Card 3", "Card 4", "Card 5", "Card 6"}, []string{"Card 2", "Card 7"})
	// Round the end: the queue wraps to the first cards.
	r.d.front.choose(6)
	r.tt.Drain()
	shows([]string{"Card 7", "Card 8", "Card 1", "Card 2"}, []string{"Card 3", "Card 6"})
}

func TestFrontCrossFadesTheOldLeadOutAndKeepsItInertOnlyUntilTheFadeEnds(t *testing.T) {
	r := newFront(t, 8, withAnimations())
	r.tick(rotateEvery - time.Millisecond)
	r.tick(time.Millisecond)
	if r.lead() != 1 {
		t.Fatalf("after a tick: lead %d", r.lead())
	}
	// Paused, the progress under the lead stands still: only the fade moves.
	r.d.front.paused = true
	r.tt.Drain()
	named := func() int {
		n := 0
		for _, nd := range access.Read(t, r.tt.Tester).Nodes {
			if nd.Label == "Card 1" {
				n++
			}
		}
		return n
	}
	// The card going is MyGo's copy of it: drawn while it fades (the picture
	// changes frame by frame), but not on the accessibility tree.
	start := bytes.Clone(r.tt.Image().Pix)
	onTree := named()
	r.tick(leadFade / 2)
	mid := bytes.Clone(r.tt.Image().Pix)
	if bytes.Equal(start, mid) {
		t.Error("the picture did not change in the first half of the fade")
	}
	r.tick(leadFade)
	r.tt.Drain()
	end := bytes.Clone(r.tt.Image().Pix)
	if bytes.Equal(mid, end) {
		t.Error("the picture did not change in the second half of the fade")
	}
	r.tick(time.Second)
	if later := r.tt.Image().Pix; !bytes.Equal(end, later) {
		t.Error("the picture still changes after the fade ended")
	}
	if after := named(); after != onTree {
		t.Errorf("%d nodes named Card 1 while it fades and %d after: the outgoing lead was on the accessibility tree", onTree, after)
	}
}

func TestFrontShowsAQueuedCardInTheLeadAndGivesItAWholeInterval(t *testing.T) {
	r := newFront(t, 8)
	r.tick(rotateEvery - time.Second)
	r.showInLead("Card 4")
	if r.lead() != 3 {
		t.Fatalf("lead %d after choosing Card 4", r.lead())
	}
	// The pointer and the focus are on the hero: it holds. Away from it, a
	// whole interval counts from there.
	r.away()
	r.tick(rotateEvery - time.Millisecond)
	if r.lead() != 3 {
		t.Fatalf("lead %d before the interval ended", r.lead())
	}
	r.tick(time.Millisecond)
	if r.lead() != 4 {
		t.Errorf("lead %d after the interval, want 4", r.lead())
	}
}

func TestFrontFetchesTheNextCardsPictureWhileTheLeadIsRead(t *testing.T) {
	src := newCoverSource()
	r := newFrontWith(t, 8, src)
	box := r.leadBox()
	if got := src.requested(bannerOf(2)); !slices.Contains(got, box) {
		t.Fatalf("the next card's art was not asked for at the lead's size: %v", got)
	}
	if got := src.requested(bannerOf(3)); slices.Contains(got, box) {
		t.Errorf("the card after next was fetched at the lead's size: %v", got)
	}
	r.tick(rotateEvery)
	r.tt.Drain()
	r.tt.Drain()
	if got := src.requested(bannerOf(3)); !slices.Contains(got, box) {
		t.Errorf("the next lead's art was not fetched when the lead changed: %v", got)
	}
}

func TestFrontHoldsWhileThePointerIsOverTheHeroAndStartsAWholeIntervalAfterItLeaves(t *testing.T) {
	r := newFront(t, 8)
	x, y := r.heroCentre()
	r.tt.Move(x, y)
	r.tt.Drain()
	r.tick(rotateEvery * 3)
	if r.lead() != 0 {
		t.Fatalf("lead %d while the pointer is over the hero", r.lead())
	}
	r.tt.Move(2, 2)
	r.tt.Drain()
	r.tick(rotateEvery - time.Millisecond)
	if r.lead() != 0 {
		t.Fatalf("lead %d before a whole interval after it left", r.lead())
	}
	r.tick(time.Millisecond)
	if r.lead() != 1 {
		t.Errorf("lead %d, want 1", r.lead())
	}
}

func TestFrontHoldsWhileFocusIsAnywhereInsideTheHeroAndGoesOnWhenItLeaves(t *testing.T) {
	r := newFront(t, 8)
	r.tt.Key(0, ui.KeyTab)
	r.tt.Drain()
	if !r.tt.Focused("Card 1") {
		t.Fatalf("Tab did not reach the lead card")
	}
	r.tick(rotateEvery * 2)
	if r.lead() != 0 {
		t.Fatalf("lead %d while focus is in the hero", r.lead())
	}
	// Moving between controls inside is not leaving.
	r.tt.Key(0, ui.KeyTab)
	r.tt.Drain()
	r.tick(rotateEvery * 2)
	if r.lead() != 0 {
		t.Fatalf("lead %d after focus moved to another control of the hero", r.lead())
	}
	// Out of the hero: the contents' first button.
	for i := 0; i < 40 && r.tt.Focused("Featured") && !r.tt.Focused("Songs and Albums"); i++ {
		r.tt.Key(0, ui.KeyTab)
		r.tt.Drain()
	}
	r.away()
	r.tick(rotateEvery)
	if r.lead() != 1 {
		t.Errorf("lead %d after focus left, want 1", r.lead())
	}
}

// A click focuses the control it lands on and the focus stays there while the
// pointer goes elsewhere; the hold is for the keyboard's focus, or a click on
// the pager or the pause control would stop the rotation for good (the
// pointer left, and nothing else would end the hold).
func TestFrontGoesOnWhenTheFocusIsOnlyWhereAClickLeftIt(t *testing.T) {
	for name, c := range map[string]struct {
		clicks  []string
		focused string // the control's name once the clicks are done
		lead    int    // the lead after the clicks and a whole interval
	}{
		"the pager":                {[]string{"Next in Featured"}, "Next in Featured", 2},
		"the pause control, twice": {[]string{"Pause featured rotation", "Play featured rotation"}, "Pause featured rotation", 1},
	} {
		t.Run(name, func(t *testing.T) {
			r := newFront(t, 8)
			for _, label := range c.clicks {
				if err := r.tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			r.tt.Move(2, 2) // the pointer leaves; the click's focus stays
			r.tt.Drain()
			if !r.tt.Focused(c.focused) {
				t.Fatalf("the click did not leave the focus on %q", c.focused)
			}
			r.tick(rotateEvery)
			if r.lead() != c.lead {
				t.Errorf("lead %d after a whole interval, want %d: a click's focus holds the rotation", r.lead(), c.lead)
			}
		})
	}
}

func TestFrontStopsWhileTheWindowIsHiddenAndGoesOnWhenItIsBack(t *testing.T) {
	r := newFront(t, 8)
	r.tick(rotateEvery - time.Millisecond)
	r.env.Motion.SetVisible(false)
	r.tt.Frame()
	r.tick(rotateEvery * 3)
	if r.lead() != 0 {
		t.Fatalf("lead %d while the window is hidden", r.lead())
	}
	r.env.Motion.SetVisible(true)
	r.tt.Frame()
	r.tick(rotateEvery - time.Millisecond)
	if r.lead() != 0 {
		t.Fatalf("lead %d before a whole interval after the window came back", r.lead())
	}
	r.tick(time.Millisecond)
	if r.lead() != 1 {
		t.Errorf("lead %d after it, want 1", r.lead())
	}
}

func TestFrontStopsWhileTheWindowLacksTheFocusAndGoesOnWhenItIsBack(t *testing.T) {
	r := newFront(t, 8)
	r.tick(rotateEvery - time.Millisecond)
	// Another app (or the mini player) takes the focus; the shell redraws.
	r.env.Window.SetFocused(false)
	r.tt.Frame()
	r.tick(rotateEvery * 3)
	if r.lead() != 0 {
		t.Fatalf("lead %d while the window is unfocused", r.lead())
	}
	r.env.Window.SetFocused(true)
	r.tt.Frame()
	r.tick(rotateEvery - time.Millisecond)
	if r.lead() != 0 {
		t.Fatalf("lead %d before a whole interval after the focus came back", r.lead())
	}
	r.tick(time.Millisecond)
	if r.lead() != 1 {
		t.Errorf("lead %d after it, want 1", r.lead())
	}
}

func TestFrontStopsWhileScrolledOffScreenAndGoesOnWhenBackInView(t *testing.T) {
	api := &fakeAPI{}
	r := newRig(t, "/new", withAPI(api), wideWindow())
	sections := []services.Section{heroSection(heroCards(8))}
	for i := range 8 { // enough shelves below the hero to scroll it away
		sections = append(sections, services.Section{ID: fmt.Sprint("shelf", i), Title: fmt.Sprint("Shelf ", i), Kind: services.SectionShelf, Items: albumItems(i*10, 6)})
	}
	newPageWith(api, r.h.Clock.Now(), sections...)
	r.tt.Until("the front page", r.tt.Shows("Featured"))
	r.tt.Drain()
	r.tt.Drain()

	r.tick(rotateEvery - time.Millisecond)
	// Scroll the page until the hero is out of the window (the pointer stays off it).
	for range 12 {
		r.tt.Scroll(5, 500, 0, 400)
		r.tt.Drain()
	}
	if hero, ok := r.tt.Find("Featured"); ok && hero.Y+hero.H > 0 {
		t.Fatalf("the hero is still on screen after scrolling: %+v", hero)
	}
	r.tick(rotateEvery * 3)
	if r.lead() != 0 {
		t.Fatalf("lead %d while the hero is scrolled off screen", r.lead())
	}
	for range 12 {
		r.tt.Scroll(5, 500, 0, -400)
		r.tt.Drain()
	}
	if _, ok := r.tt.Find("Featured"); !ok {
		t.Fatal("the hero is not back after scrolling up")
	}
	r.tick(rotateEvery - time.Millisecond)
	if r.lead() != 0 {
		t.Fatalf("lead %d before a whole interval after the hero came back", r.lead())
	}
	r.tick(time.Millisecond)
	if r.lead() != 1 {
		t.Errorf("lead %d after it, want 1", r.lead())
	}
}

func TestFrontWaitsAWholeIntervalMoreAtATickThatFindsAMenuOpen(t *testing.T) {
	r := newFront(t, 8)
	r.registerPlaylistAction()
	// A menu opened on the front page itself.
	r.d.front.menus.OpenAt(kit.ActionTarget{Item: kit.PlaylistItem(services.Playlist{ID: "p"}), Name: "Playlist"}, 100, 100, true)
	r.tt.Drain()
	if !r.d.front.menus.IsOpen() {
		t.Fatal("the menu did not open")
	}
	r.tick(rotateEvery)
	if r.lead() != 0 {
		t.Fatalf("lead %d at a tick that found a menu open", r.lead())
	}
	r.tick(rotateEvery)
	if r.lead() != 0 {
		t.Fatalf("lead %d at the next tick, the menu still open", r.lead())
	}
	r.d.front.menus.Close()
	r.tt.Drain()
	r.tick(rotateEvery)
	if r.lead() != 1 {
		t.Errorf("lead %d once the menu closed, want 1", r.lead())
	}
}

func TestFrontWaitsWhileAMenuOfAnotherOwnerIsOpen(t *testing.T) {
	r := newFront(t, 8)
	var sidebar kit.Menu // the shell's, not the page's
	sidebar.OpenAt(100, 100, true)
	t.Cleanup(sidebar.Close)
	r.tick(rotateEvery)
	if r.lead() != 0 {
		t.Fatalf("lead %d at a tick that found another menu open", r.lead())
	}
	r.tick(rotateEvery)
	if r.lead() != 0 {
		t.Fatalf("lead %d at the next tick, the menu still open", r.lead())
	}
	sidebar.Close()
	r.tick(rotateEvery)
	if r.lead() != 1 {
		t.Errorf("lead %d once it closed, want 1", r.lead())
	}
}

func TestFrontDoesNotRunUnderAPageMadeInert(t *testing.T) {
	r := newFront(t, 8)
	uncover := r.env.Motion.Cover() // Now Playing, lyrics, a modal
	r.tick(rotateEvery * 3)
	if r.lead() != 0 {
		t.Fatalf("lead %d under a covered page", r.lead())
	}
	uncover()
	r.tick(rotateEvery)
	if r.lead() != 1 {
		t.Errorf("lead %d once the page is back, want 1", r.lead())
	}
}

func TestFrontPausesAndResumesTheRotationAndNamesItsState(t *testing.T) {
	r := newFront(t, 8)
	if !r.tt.HasText("Pause featured rotation") {
		t.Fatalf("no pause control: %q", r.tt.Texts())
	}
	if err := r.tt.Click("Pause featured rotation"); err != nil {
		t.Fatal(err)
	}
	r.away()
	if !r.tt.HasText("Play featured rotation") || r.tt.HasText("Pause featured rotation") {
		t.Errorf("the control did not turn into Play: %q", r.tt.Texts())
	}
	r.tick(rotateEvery * 3)
	if r.lead() != 0 {
		t.Fatalf("lead %d while paused", r.lead())
	}
	if err := r.tt.Click("Play featured rotation"); err != nil {
		t.Fatal(err)
	}
	r.away()
	if !r.tt.HasText("Pause featured rotation") {
		t.Errorf("the control did not turn back into Pause: %q", r.tt.Texts())
	}
	r.tick(rotateEvery)
	if r.lead() != 1 {
		t.Errorf("lead %d after resuming and an interval, want 1", r.lead())
	}
}

func TestFrontSaysOnlyWhatTheUserDidAndTheRotationsOwnChangesStaySilent(t *testing.T) {
	r := newFront(t, 8)
	r.tt.Announcements() // what the page said while it loaded
	r.tick(rotateEvery * 3)
	if got := r.tt.Announcements(); len(got) != 0 {
		t.Fatalf("the rotation announced %q", got)
	}
	click := func(name string) []string {
		t.Helper()
		if err := r.tt.Click(name); err != nil {
			t.Fatal(err)
		}
		r.tt.Drain()
		return r.tt.Announcements()
	}
	if got := click("Pause featured rotation"); !slices.Equal(got, []string{"Rotation paused"}) {
		t.Errorf("pausing announced %q", got)
	}
	if got := click("Play featured rotation"); !slices.Equal(got, []string{"Rotation playing"}) {
		t.Errorf("resuming announced %q", got)
	}
	if got := click("Next in Featured"); !slices.Equal(got, []string{"5 of 8"}) {
		t.Errorf("the next card announced %q", got)
	}
}

func TestFrontPagerWrapsRoundSoItNeverDisablesTheFocusedButton(t *testing.T) {
	r := newFront(t, 16)
	if err := r.tt.Click("Previous in Featured"); err != nil {
		t.Fatal(err)
	}
	r.tt.Drain()
	if r.lead() != 15 || !r.tt.HasText("16 of 16") {
		t.Fatalf("Previous from the first card did not wrap to the last: lead %d, %q", r.lead(), r.tt.Texts())
	}
	if err := r.tt.Click("Next in Featured"); err != nil {
		t.Fatal(err)
	}
	r.tt.Drain()
	if r.lead() != 0 || !r.tt.HasText("1 of 16") {
		t.Errorf("Next from the last card did not wrap to the first: lead %d, %q", r.lead(), r.tt.Texts())
	}
}

func TestFrontAnOnlyCardHasNoRotationControlAndNeverRotates(t *testing.T) {
	r := newFront(t, 1)
	r.tick(rotateEvery * 2)
	if r.lead() != 0 {
		t.Errorf("lead %d", r.lead())
	}
	featured := 0
	for _, text := range r.tt.Texts() {
		if text == "Featured" {
			featured++
		}
	}
	if r.tt.HasText("rotation") || featured != 1 { // the section's own name; the queue's list would be a second
		t.Errorf("a single card has a list or a rotation control: %q", r.tt.Texts())
	}
	if !r.tt.HasText("Card 1") {
		t.Error("the only card is not shown")
	}
}

func TestFrontReducedMotionKeepsTheLeadStillWithTheQueueToChangeIt(t *testing.T) {
	for name, set := range map[string]func(*testing.T){
		"Reduce effects":               func(t *testing.T) { reduceEffects(t, true) },
		"the setting's reduced motion": func(t *testing.T) { reduceMotion(t, true) },
	} {
		t.Run(name, func(t *testing.T) {
			set(t)
			r := newFront(t, 8)
			r.tick(rotateEvery * 3)
			if r.lead() != 0 {
				t.Fatalf("lead %d under %s", r.lead(), name)
			}
			if r.tt.HasText("rotation") {
				t.Errorf("a rotation control under %s: %q", name, r.tt.Texts())
			}
			r.showInLead("Card 4")
			if r.lead() != 3 {
				t.Errorf("lead %d after showing Card 4: the queue must still change it", r.lead())
			}
			r.away()
			r.tick(rotateEvery * 3)
			if r.lead() != 3 {
				t.Errorf("lead %d, it rotated", r.lead())
			}
		})
	}
}

func TestFrontReducedMotionOfTheDesktopKeepsTheLeadStill(t *testing.T) {
	r := newFront(t, 8)
	r.tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	r.tt.Frame()
	r.tick(rotateEvery * 3)
	if r.lead() != 0 || r.tt.HasText("rotation") {
		t.Errorf("lead %d, texts %q: the desktop's reduced motion is not followed", r.lead(), r.tt.Texts())
	}
}

// #442: a queued card opens its page as the lead does; its "Show in lead"
// button is what brings it into the lead.
func TestFrontTheLeadAndAQueuedCardOpenTheirPagesAndShowInLeadOnlyChoosesIt(t *testing.T) {
	r := newFront(t, 8)
	if err := r.tt.Click("Card 1"); err != nil { // the lead card
		t.Fatal(err)
	}
	r.tt.Drain()
	if got := r.path(); got != "/curator/1" {
		t.Errorf("the lead opened %q, want /curator/1", got)
	}
	r2 := newFront(t, 8)
	if err := r2.tt.Click("Card 3"); err != nil { // its tile
		t.Fatal(err)
	}
	r2.tt.Drain()
	if got := r2.path(); got != "/curator/3" || r2.lead() != 0 {
		t.Errorf("a queued card led to %q with lead %d, want its page", got, r2.lead())
	}
	r3 := newFront(t, 8)
	r3.showInLead("Card 3")
	if got := r3.path(); got != "/new" || r3.lead() != 2 {
		t.Errorf("Show in lead led to %q with lead %d: it must only choose its card", got, r3.lead())
	}
}

func TestFrontACardWhosePageIsNotPortedLeadsNowhere(t *testing.T) {
	r := newFront(t, 8, withoutRoutes("/curator/{id}"))
	if err := r.tt.Click("Card 1"); err != nil {
		t.Fatal(err)
	}
	r.tt.Drain()
	if got := r.path(); got != "/new" {
		t.Errorf("a card with no page navigated to %q", got)
	}
}

// coverSource is an ArtSource that records what is asked for and serves a
// small picture for anything.
type coverSource struct {
	mu   sync.Mutex
	asks map[string][][2]int
	jpeg []byte
}

func newCoverSource() *coverSource {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.Set(x, y, color.RGBA{200, 80, 40, 255})
		}
	}
	var b bytes.Buffer
	_ = jpeg.Encode(&b, img, nil)
	return &coverSource{asks: map[string][][2]int{}, jpeg: b.Bytes()}
}

func (s *coverSource) Template(urlPath string) (string, bool) {
	return urlPath, strings.HasPrefix(urlPath, "/art/")
}

func (s *coverSource) Get(_ context.Context, template string, w, h int) ([]byte, error) {
	s.mu.Lock()
	s.asks[template] = append(s.asks[template], [2]int{w, h})
	s.mu.Unlock()
	return s.jpeg, nil
}

func (s *coverSource) Backdrop(context.Context, string) ([]byte, error) { return s.jpeg, nil }

// requested is the sizes the source was asked for src at.
func (s *coverSource) requested(src string) [][2]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.asks[src])
}

// newFrontWith is newFront over an artwork loader fed by src.
func newFrontWith(t *testing.T, n int, src *coverSource) *rig {
	t.Helper()
	api := &fakeAPI{}
	r := newRig(t, "/new", withAPI(api), wideWindow())
	loader := kit.NewArtLoaderFrom(src, r.h.Main.Post)
	t.Cleanup(func() { loader.Close(); loader.Wait() })
	r.env.Art = loader
	r.tt.Art = append(r.tt.Art, loader)
	newPageWith(api, r.h.Clock.Now(), heroSection(heroCards(n)))
	r.tt.Until("the front page", r.tt.Shows("Featured"))
	r.tt.Until("the covers", func() bool { return len(src.requested(bannerOf(1))) > 0 })
	r.tt.Drain()
	r.tt.Drain()
	return r
}

// countText is how many texts of the last frame are s.
func countText(tt *uitest.Session, s string) int {
	n := 0
	for _, w := range tt.Texts() {
		if w == s {
			n++
		}
	}
	return n
}

// registerPlaylistAction gives playlists a menu action, so a menu opens on one.
func (r *rig) registerPlaylistAction() {
	r.env.Actions.Register(kit.ActionDef{
		ID: "test.playlist", Target: kit.KindPlaylist, Group: kit.GroupItem,
		Label: func(*kit.ActionContext) string { return "Do something" },
		Run:   func(*kit.ActionContext) {},
	})
}

// A card that plays has a play control named by it, on the lead and on its tile.
func TestFrontCardsThatPlayHaveAPlayControlOnTheLeadAndTheirTile(t *testing.T) {
	api := &fakeAPI{}
	cards := make([]services.Item, 3)
	for i := range cards {
		cards[i] = playlistItem(fmt.Sprint(i+1), fmt.Sprintf("List %d", i+1))
		cards[i].Wide = services.Art{Medium: bannerOf(i + 1)}
	}
	r := newRig(t, "/new", withAPI(api), wideWindow(), withTracks(fakeTracks{songs: map[string][]services.Song{"playlist:1": songs(2), "playlist:2": songs(3)}}))
	newPageWith(api, r.h.Clock.Now(), heroSection(cards))
	r.tt.Until("the front page", r.tt.Shows("Play List 1", "Play List 2"))
	if err := r.tt.Click("Play List 2"); err != nil { // its tile's control (the lead is List 1)
		t.Fatal(err)
	}
	got := r.waitPlays(1)[0]
	if len(got.refs) != 3 || got.from != (services.PlayFrom{Kind: "playlist", ID: "2"}) {
		t.Errorf("played %+v", got)
	}
	if r.lead() != 0 {
		t.Errorf("playing a queued card made it the lead: %d", r.lead())
	}
}

// The item's menu opens on a right-click of a queued card, and on Shift+F10
// of the card the keyboard has focused.
func TestFrontCardMenuOpensOnRightClickAndShiftF10(t *testing.T) {
	api := &fakeAPI{}
	cards := make([]services.Item, 3)
	for i := range cards {
		cards[i] = playlistItem(fmt.Sprint(i+1), fmt.Sprintf("List %d", i+1))
	}
	r := newRig(t, "/new", withAPI(api), wideWindow())
	r.registerPlaylistAction()
	newPageWith(api, r.h.Clock.Now(), heroSection(cards))
	r.tt.Until("the front page", r.tt.Shows("Featured"))
	r.d.front.paused = true
	x, y := r.tileCentre("List 2")
	r.tt.RightClickAt(x, y)
	r.tt.Drain()
	if !r.d.front.menus.IsOpen() || !r.tt.HasText("Do something") {
		t.Fatalf("a right-click on a queued card opened no menu: %q", r.tt.Texts())
	}
	r.tt.Key(0, ui.KeyEscape)
	r.advance(300e6)
	r.tt.Drain()
	if r.d.front.menus.IsOpen() {
		t.Fatal("Escape did not close the menu")
	}
	// The tile's own focus, from the keyboard, then the menu key chord.
	for range 12 {
		r.tt.Key(0, ui.KeyTab)
		if r.tt.Focused("List 2") {
			break
		}
	}
	if !r.tt.Focused("List 2") {
		t.Fatal("Tab never reached the queued card")
	}
	r.tt.Key(ui.Shift, ui.KeyF10)
	r.tt.Drain()
	if !r.d.front.menus.IsOpen() {
		t.Errorf("Shift+F10 on the focused card opened no menu: %q", r.tt.Texts())
	}
}

// A page built again after it went away starts over: the first card leads and
// the rotation runs.
func TestFrontStartsOverWhenThePageIsBuiltAgain(t *testing.T) {
	r := newFront(t, 8)
	_ = r.tt.Click("Pause featured rotation")
	r.away()
	r.tick(rotateEvery)
	r.showInLead("Card 4")
	if r.lead() != 3 || !r.d.front.paused {
		t.Fatalf("set up: lead %d paused %v", r.lead(), r.d.front.paused)
	}
	r.router.Replace("/charts")
	r.tt.Drain()
	r.router.Replace("/new")
	r.tt.Drain()
	r.tt.Drain()
	if r.lead() != 0 || r.d.front.paused {
		t.Errorf("after coming back: lead %d paused %v, want the first card and a running rotation", r.lead(), r.d.front.paused)
	}
}

// The lead's caption carries what Apple's hero card does: the name, Apple's
// one line about the card (Description) and the byline. The line's row is
// kept for a card without one, so the queue under the lead (in a narrow
// window) does not move as the lead rotates.
func TestFrontLeadShowsApplesLineAndKeepsItsRowForACardWithout(t *testing.T) {
	api := &fakeAPI{}
	r := newRig(t, "/new", withAPI(api), withSize(708, 1253))
	cards := heroCards(5)
	cards[0].Description = "Remi Wolf drowns her sorrows in “Champagne”."
	cards[0].Caption, cards[0].Subtitle = "Updated playlist", "Apple Music Pop"
	newPageWith(api, r.h.Clock.Now(), heroSection(cards))
	r.tt.Until("the front page", r.tt.Shows("Featured", "Remi Wolf drowns her sorrows in “Champagne”.", "Updated playlist · Apple Music Pop"))
	r.d.front.paused = true
	r.tt.Drain()
	lead, ok := access.Read(t, r.tt.Tester).Find(ui.RoleLink, "Card 1")
	if !ok || !strings.HasPrefix(lead.Description, "Remi Wolf drowns her sorrows") {
		t.Errorf("the lead's description %q does not start with Apple's line", lead.Description)
	}
	// Card 3's tile: the queue holds the first four whichever of them leads.
	queue := func() float32 {
		_, y := r.tileCentre("Card 3")
		return y
	}
	before := queue()
	r.d.front.choose(1) // a card with no line
	r.tt.Drain()
	r.tt.Drain()
	if r.tt.HasText("Remi Wolf drowns her sorrows in “Champagne”.") {
		t.Error("the line of the card before stayed")
	}
	if after := queue(); after != before {
		t.Errorf("the queue moved from y=%v to y=%v when the lead had no line", before, after)
	}
}
