package discover

import (
	"bytes"
	"context"
	"fmt"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
	"image"
	"image/color"
	"image/jpeg"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

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

// rowCentre is the centre of the nth list row (0-based) on the page showing.
// The section and the list are both labelled "Featured" (as v0.2.0's are),
// so the list is found by a row of it: a card other than the lead, whose
// title only a row shows, in the page the lead is on.
func (r *rig) rowCentre(n int) (float32, float32) {
	lay := layoutFor(r.d.front.width.w, 24)
	base := r.lead() / lay.perPage * lay.perPage
	for k := base; k < base+lay.perPage; k++ {
		if k == r.lead() {
			continue
		}
		for _, name := range []string{"Card %d", "List %d"} { // the names these tests give their cards
			if row, ok := r.tt.Find(fmt.Sprintf(name, k+1)); ok {
				return row.X + row.W/2, row.Y + (float32(n-(k-base))+0.5)*lay.rowHeight
			}
		}
	}
	r.t.Fatal("no list of featured cards")
	return 0, 0
}

func (r *rig) heroCentre() (float32, float32) {
	hero, ok := r.tt.Find("Featured")
	if !ok {
		r.t.Fatal("no hero")
	}
	return hero.X + hero.W/4, hero.Y + hero.H/3
}

func TestFrontLeadsWithTheFirstCardAtTheLeadsWidthAndListsThemAPageAtATime(t *testing.T) {
	src := newCoverSource()
	r := newFrontWith(t, 16, src)
	for _, w := range []string{"Featured", "Card 1", "Card 6", "1 of 3", "Previous in Featured", "Next in Featured"} {
		if !r.tt.HasText(w) {
			t.Errorf("the front page lacks %q: %q", w, r.tt.Texts())
		}
	}
	// The sixth card is the last beside the lead.
	for _, w := range []string{"Card 7", "Card 16"} {
		if r.tt.HasText(w) {
			t.Errorf("the first page lists %q", w)
		}
	}
	// Two thirds of 1200 less the gap, at Apple's 1060:608 hero shape.
	if got := src.requested(bannerOf(1)); !slices.Contains(got, [2]int{784, 450}) {
		t.Errorf("the lead's art was asked for at %v, want 784×450", got)
	}
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

func TestFrontBringsTheListToThePageTheLeadIsOn(t *testing.T) {
	r := newFront(t, 8)
	if !r.tt.HasText("1 of 2") {
		t.Fatalf("not on the first page: %q", r.tt.Texts())
	}
	for range 6 {
		r.tick(rotateEvery)
	}
	if r.lead() != 6 {
		t.Fatalf("lead %d, want the seventh card", r.lead())
	}
	r.tt.Drain()
	if !r.tt.HasText("2 of 2") || !r.tt.HasText("Card 8") {
		t.Errorf("the list did not turn to the lead's page: %q", r.tt.Texts())
	}
}

func TestFrontCrossFadesTheOldLeadOutAndKeepsItInertOnlyUntilTheFadeEnds(t *testing.T) {
	r := newFront(t, 8, withAnimations())
	r.tick(rotateEvery - time.Millisecond)
	r.tick(time.Millisecond)
	if r.lead() != 1 {
		t.Fatalf("after a tick: lead %d", r.lead())
	}
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

func TestFrontMakesAChosenRowTheLeadAndGivesItAWholeInterval(t *testing.T) {
	r := newFront(t, 8)
	r.tick(rotateEvery - time.Second)
	x, y := r.rowCentre(4)
	r.tt.ClickAt(x, y)
	r.tt.Drain()
	if r.lead() != 4 {
		t.Fatalf("lead %d after choosing Card 5", r.lead())
	}
	// The pointer and the focus are on the hero: it holds. Away from it, a
	// whole interval counts from there.
	r.away()
	r.tick(rotateEvery - time.Millisecond)
	if r.lead() != 4 {
		t.Fatalf("lead %d before the interval ended", r.lead())
	}
	r.tick(time.Millisecond)
	if r.lead() != 5 {
		t.Errorf("lead %d after the interval, want 5", r.lead())
	}
}

func TestFrontFetchesTheNextCardsPictureWhileTheLeadIsRead(t *testing.T) {
	src := newCoverSource()
	r := newFrontWith(t, 8, src)
	if got := src.requested(bannerOf(2)); !slices.Contains(got, [2]int{784, 450}) {
		t.Fatalf("the next card's art was not asked for at the lead's size: %v", got)
	}
	if got := src.requested(bannerOf(3)); slices.Contains(got, [2]int{784, 450}) {
		t.Errorf("the card after next was fetched at the lead's size: %v", got)
	}
	r.tick(rotateEvery)
	r.tt.Drain()
	r.tt.Drain()
	if got := src.requested(bannerOf(3)); !slices.Contains(got, [2]int{784, 450}) {
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
	for i := 0; i < 40 && r.tt.Focused("Featured") && !r.tt.Focused("Songs and albums"); i++ {
		r.tt.Key(0, ui.KeyTab)
		r.tt.Drain()
	}
	r.away()
	r.tick(rotateEvery)
	if r.lead() != 1 {
		t.Errorf("lead %d after focus left, want 1", r.lead())
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
	if got := click("Next in Featured"); !slices.Equal(got, []string{"2 of 2"}) {
		t.Errorf("turning the page announced %q", got)
	}
}

func TestFrontPagerWrapsRoundSoItNeverDisablesTheFocusedButton(t *testing.T) {
	r := newFront(t, 16)
	if err := r.tt.Click("Previous in Featured"); err != nil {
		t.Fatal(err)
	}
	r.tt.Drain()
	if !r.tt.HasText("3 of 3") || !r.tt.HasText("Card 13") || !r.tt.HasText("Card 16") || r.tt.HasText("Card 12") {
		t.Fatalf("Previous from the first page did not wrap to the last: %q", r.tt.Texts())
	}
	if err := r.tt.Click("Next in Featured"); err != nil {
		t.Fatal(err)
	}
	r.tt.Drain()
	if !r.tt.HasText("1 of 3") {
		t.Errorf("Next from the last page did not wrap to the first: %q", r.tt.Texts())
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
	if r.tt.HasText("rotation") || featured != 1 { // the section's own name; a list would be a second
		t.Errorf("a single card has a list or a rotation control: %q", r.tt.Texts())
	}
	if !r.tt.HasText("Card 1") {
		t.Error("the only card is not shown")
	}
}

func TestFrontReducedMotionKeepsTheLeadStillWithTheListToChangeIt(t *testing.T) {
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
			x, y := r.rowCentre(3)
			r.tt.ClickAt(x, y)
			r.tt.Drain()
			if r.lead() != 3 {
				t.Errorf("lead %d after choosing Card 4: the list must still change it", r.lead())
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

func TestFrontTheLeadOpensItsPageAndARowOnlyChoosesIt(t *testing.T) {
	r := newFront(t, 8)
	if err := r.tt.Click("Card 1"); err != nil { // the lead card
		t.Fatal(err)
	}
	r.tt.Drain()
	if got := r.path(); got != "/curator/1" {
		t.Errorf("the lead opened %q, want /curator/1", got)
	}
	r2 := newFront(t, 8)
	x, y := r2.rowCentre(2)
	r2.tt.ClickAt(x, y)
	r2.tt.Drain()
	if got := r2.path(); got != "/new" || r2.lead() != 2 {
		t.Errorf("a row led to %q with lead %d: it must only choose its card", got, r2.lead())
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

// A card that plays has a play control named by it, on the lead and on its row.
func TestFrontCardsThatPlayHaveAPlayControlOnTheLeadAndTheirRow(t *testing.T) {
	api := &fakeAPI{}
	cards := make([]services.Item, 3)
	for i := range cards {
		cards[i] = playlistItem(fmt.Sprint(i+1), fmt.Sprintf("List %d", i+1))
		cards[i].Wide = services.Art{Medium: bannerOf(i + 1)}
	}
	r := newRig(t, "/new", withAPI(api), wideWindow(), withTracks(fakeTracks{songs: map[string][]services.Song{"playlist:1": songs(2), "playlist:2": songs(3)}}))
	newPageWith(api, r.h.Clock.Now(), heroSection(cards))
	r.tt.Until("the front page", r.tt.Shows("Play List 1", "Play List 2"))
	if err := r.tt.Click("Play List 2"); err != nil { // its row's control (the lead is List 1)
		t.Fatal(err)
	}
	got := r.waitPlays(1)[0]
	if len(got.refs) != 3 || got.from != (services.PlayFrom{Kind: "playlist", ID: "2"}) {
		t.Errorf("played %+v", got)
	}
	if r.lead() != 0 {
		t.Errorf("playing a row's card made it the lead: %d", r.lead())
	}
}

// The item's menu opens on a right-click and on Shift+F10 of the focused card.
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
	x, y := r.rowCentre(1)
	r.tt.RightClickAt(x, y)
	r.tt.Drain()
	if !r.d.front.menus.IsOpen() || !r.tt.HasText("Do something") {
		t.Fatalf("a right-click on a row opened no menu: %q", r.tt.Texts())
	}
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Drain()
	if r.d.front.menus.IsOpen() {
		t.Fatal("Escape did not close the menu")
	}
	// The row's own focus, then the menu key chord.
	r.tt.ClickAt(x, y)
	r.tt.Key(ui.Shift, ui.KeyF10)
	r.tt.Drain()
	if !r.d.front.menus.IsOpen() {
		t.Errorf("Shift+F10 on the focused row opened no menu: %q", r.tt.Texts())
	}
}

// A page built again after it went away starts over: the first card leads and
// the rotation runs.
func TestFrontStartsOverWhenThePageIsBuiltAgain(t *testing.T) {
	r := newFront(t, 8)
	_ = r.tt.Click("Pause featured rotation")
	r.away()
	r.tick(rotateEvery)
	x, y := r.rowCentre(3)
	r.tt.ClickAt(x, y)
	r.tt.Drain()
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
