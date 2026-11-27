package newreleases

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

func TestGroupsSortReleasesByWhenTheyComeOut(t *testing.T) {
	list := sample()
	// A second release still to come, sooner than the first.
	list.Items = slices.Insert(list.Items, 1, release("306", "Next Week", "Mia Vale", services.ReleaseSingle, "2026-11-20", true, false))
	g := groupsOf(list.Items, now)
	var titles []string
	var firstSoon []string
	for _, x := range g {
		titles = append(titles, x.title)
		if x.id == "soon" {
			for _, r := range x.items {
				firstSoon = append(firstSoon, r.Album.Name)
			}
		}
	}
	if !slices.Equal(titles, []string{"Coming soon", "This week", "Earlier"}) {
		t.Fatalf("groups = %v", titles)
	}
	if !slices.Equal(firstSoon, []string{"Next Week", "Winter Rooms"}) {
		t.Errorf("coming soon = %v, want the soonest first", firstSoon)
	}
	if got := groupsOf(nil, now); len(got) != 0 {
		t.Errorf("groups of nothing = %v", got)
	}
	// A group with nothing in it is left out.
	only := groupsOf(list.Items[:1], now)
	if len(only) != 1 || only[0].id != "soon" {
		t.Errorf("one upcoming release = %+v", only)
	}
}

func TestThePageListsReleasesUnderTheirHeadings(t *testing.T) {
	r := newRig(t, sample(), true, 1132, 900)
	r.until("the list", r.tt.Shows("New from Artists You Love", "Coming soon", "This week", "Earlier",
		"Winter Rooms", "Night Signals", "Paper Boats", "Lantern Season", "Slow Harbour"))
	for _, want := range []string{
		"Hollow Pines · Album", "Mia Vale · EP", "Hollow Pines · Single", // the kind in words, " - EP" cut from the title
		"Out Nov 27", "Nov 14", "From the artists you've favourited · Checked 3 hr. ago",
	} {
		if !r.tt.HasText(want) {
			t.Errorf("missing %q; texts %q", want, r.tt.Texts())
		}
	}
	if slices.Contains(r.tt.Texts(), "Night Signals - EP") {
		t.Errorf("the EP suffix is shown with the kind: %q", r.tt.Texts())
	}
	if !r.tt.HasText("Mark all as read") || !r.tt.HasText("Clear list") {
		t.Error("the header's actions are missing")
	}
}

// The tracker has no sidebar link: its list is For You's shelf and the page
// behind that shelf's "See all", whose route stays so that a notification
// still opens it.
func TestTheModuleHasOneRouteAndNoSidebarLink(t *testing.T) {
	r := newRig(t, sample(), true, 1132, 700)
	m := r.n.module()
	if len(m.Routes) != 1 || m.Routes[0].Pattern != "/new-releases" {
		t.Fatalf("routes = %+v", m.Routes)
	}
	if len(m.Nav) != 0 {
		t.Errorf("nav = %+v, want no sidebar link", m.Nav)
	}
	if got := m.Routes[0].Title(nil); got != "New from Artists You Love" {
		t.Errorf("window title = %q", got)
	}
}

// The shelf is Env.NewReleasesShelf, set by the module's factory.
func TestTheFactoryHandsTheShelfToTheEnv(t *testing.T) {
	r := newRig(t, sample(), true, 800, 400)
	r.h.Env.NewReleasesShelf = nil
	m := New(r.h.Env)
	if m.ID != "newreleases" || r.h.Env.NewReleasesShelf == nil {
		t.Errorf("module %q, shelf set: %t", m.ID, r.h.Env.NewReleasesShelf != nil)
	}
}

// The shelf is the upcoming releases (soonest first) and the unread ones that
// are out; the releases already read and out stay on the page.
func TestTheShelfShowsTheUpcomingAndUnreadReleasesOnly(t *testing.T) {
	r := newShelfRig(t, sample(), true, 1132, 700)
	r.until("the shelf", r.tt.Shows("New from Artists You Love", "See all", "Winter Rooms", "Night Signals", "Lantern Season"))
	for _, gone := range []string{"Paper Boats", "Slow Harbour", "Coming soon", "This week", "Earlier", "Mark all as read"} {
		if r.tt.HasText(gone) {
			t.Errorf("the shelf shows %q: %q", gone, r.tt.Texts())
		}
	}
	if !r.tt.HasText("Out Nov 27") {
		t.Errorf("the upcoming release's date is missing: %q", r.tt.Texts())
	}
	// Soonest first, then the newest unread.
	texts := r.tt.Texts()
	at := func(s string) int { return slices.Index(texts, s) }
	if at("Winter Rooms") >= at("Night Signals") || at("Night Signals") >= at("Lantern Season") {
		t.Errorf("order %q", texts)
	}
}

// A release that is still to come stays on the shelf when it is read.
func TestAnUpcomingReleaseStaysOnTheShelfWhenRead(t *testing.T) {
	list := sample()
	for i := range list.Items {
		list.Items[i].Read = true
	}
	list.Unread = 0
	r := newShelfRig(t, list, true, 1132, 700)
	r.until("the shelf", r.tt.Shows("Winter Rooms"))
	for _, gone := range []string{"Night Signals", "Lantern Season", "Paper Boats"} {
		if r.tt.HasText(gone) {
			t.Errorf("the shelf shows the read release %q", gone)
		}
	}
}

// The shelf shows while there is something unread or upcoming and goes when
// there is not, and with the tracker off.
func TestTheShelfComesAndGoesWithWhatItIsFor(t *testing.T) {
	const title = "New from Artists You Love"
	r := newShelfRig(t, sample(), true, 1132, 700)
	r.until("the shelf", r.tt.Shows(title))

	// Everything read and out: nothing is left to say.
	list := sample()
	list.Items = list.Items[1:]
	for i := range list.Items {
		list.Items[i].Read = true
	}
	list.Unread = 0
	r.h.Bus.Emit(services.EventArtistReleasesChanged, list)
	r.until("the shelf gone", func() bool { r.tt.Drain(); return !r.tt.HasText(title) })

	// A new release brings it back.
	r.h.Bus.Emit(services.EventArtistReleasesChanged, sample())
	r.until("the shelf back", r.tt.Shows(title, "Winter Rooms"))

	// Turning the tracker off takes it away, with the list kept.
	cfg := r.h.Settings.Get()
	cfg.ArtistTracker.Enabled = false
	store.Set(r.h.Env.Queries, store.KeySettings, cfg)
	r.until("the shelf off", func() bool { r.tt.Drain(); return !r.tt.HasText(title) })

	empty := newShelfRig(t, services.NewReleases{}, true, 800, 400)
	empty.tt.Settle()
	if empty.tt.HasText(title) {
		t.Errorf("the shelf shows for an empty list: %q", empty.tt.Texts())
	}
}

// The shelf shows four rows; the page has them all.
func TestTheShelfShowsAFewReleases(t *testing.T) {
	var list services.NewReleases
	for i := range 6 {
		id := strconv.Itoa(400 + i)
		list.Items = append(list.Items, release(id, "Unread "+id, "Mia Vale", services.ReleaseAlbum, "2026-11-14", false, false))
		list.Unread++
	}
	r := newShelfRig(t, list, true, 1132, 700)
	r.until("the shelf", r.tt.Shows("Unread 400", "Unread 403"))
	if r.tt.HasText("Unread 404") || r.tt.HasText("Unread 405") {
		t.Errorf("the shelf shows more than %d rows: %q", shelfRows, r.tt.Texts())
	}
	r.click("See all")
	r.until("the page", r.tt.Shows("Mark all as read", "Unread 405"))
}

// "See all" opens the whole list, the page of the old sidebar link.
func TestSeeAllOpensThePage(t *testing.T) {
	r := newShelfRig(t, sample(), true, 1132, 700)
	r.until("the shelf", r.tt.Shows("See all"))
	r.click("See all")
	if got := r.router.Path(); got != "/new-releases" {
		t.Fatalf("See all went to %q", got)
	}
	r.until("the page", r.tt.Shows("Coming soon", "This week", "Earlier", "Paper Boats", "Slow Harbour", "Mark all as read"))
}

// A row of the shelf is the page's row: opening it marks it read.
func TestOpeningAReleaseOnTheShelfMarksItReadAndOpensTheAlbum(t *testing.T) {
	r := newShelfRig(t, sample(), true, 1132, 700)
	r.until("the shelf", r.tt.Shows("Night Signals"))
	r.click("Night Signals")
	r.until("marked read", func() bool { return slices.Contains(r.api.seen(), "read 303") })
	if got := r.router.Path(); got != "/album/303" {
		t.Errorf("opened %q", got)
	}
}

func TestOpeningAnUnreadReleaseMarksItReadAndOpensTheAlbum(t *testing.T) {
	r := newRig(t, sample(), true, 1132, 900)
	r.until("the list", r.tt.Shows("Night Signals"))
	r.click("Night Signals")
	r.until("marked read", func() bool { return slices.Contains(r.api.seen(), "read 303") })
	if got := r.router.Path(); got != "/album/303" {
		t.Errorf("opened %q", got)
	}
}

func TestOpeningAReadReleaseOnlyOpensIt(t *testing.T) {
	r := newRig(t, sample(), true, 1132, 900)
	r.until("the list", r.tt.Shows("Paper Boats"))
	r.click("Paper Boats")
	if got := r.router.Path(); got != "/album/304" {
		t.Errorf("opened %q", got)
	}
	r.tt.Drain()
	if calls := r.api.seen(); len(calls) != 0 {
		t.Errorf("calls %v for a release already read", calls)
	}
}

func TestMarkAllAsReadAndClearAskGo(t *testing.T) {
	r := newRig(t, sample(), true, 1132, 900)
	r.until("the list", r.tt.Shows("Mark all as read"))
	r.click("Mark all as read")
	r.until("read all", func() bool { return slices.Contains(r.api.seen(), "read-all") })
	r.click("Clear list")
	r.until("clear", func() bool { return slices.Contains(r.api.seen(), "clear") })
}

func TestMarkAllAsReadIsOffWithNothingUnread(t *testing.T) {
	list := sample()
	for i := range list.Items {
		list.Items[i].Read = true
	}
	list.Unread = 0
	r := newRig(t, list, true, 1132, 900)
	r.until("the list", r.tt.Shows("Mark all as read"))
	_ = r.tt.Click("Mark all as read")
	r.tt.Drain()
	if calls := r.api.seen(); len(calls) != 0 {
		t.Errorf("calls %v with nothing unread", calls)
	}
}

func TestEmptyStatesSayWhy(t *testing.T) {
	off := newRig(t, services.NewReleases{}, false, 1132, 700)
	off.until("off", off.tt.Shows("Release tracking is off", "Open Settings"))
	if off.tt.HasText("Mark all as read") {
		t.Error("the actions show with nothing to act on")
	}
	off.click("Open Settings")
	if got := off.router.Location(); got != "/settings?section=general" {
		t.Errorf("Open Settings went to %q", got)
	}

	on := newRig(t, services.NewReleases{CheckedAt: now.Add(-2 * time.Hour).UnixMilli()}, true, 1132, 700)
	on.until("empty", on.tt.Shows("Nothing new yet", "From the artists you've favourited · Checked 2 hr. ago"))
	if on.tt.HasText("Open Settings") {
		t.Error("Open Settings shows with the tracker on")
	}
	unchecked := newRig(t, services.NewReleases{}, true, 1132, 700)
	unchecked.until("unchecked", unchecked.tt.Shows("Nothing new yet", "Not checked yet"))
}

// A release list that is still tracked while the tracker is off (turned off
// after finding some) stays readable.
func TestAListIsShownEvenWithTheTrackerOff(t *testing.T) {
	r := newRig(t, sample(), false, 1132, 900)
	r.until("the list", r.tt.Shows("Winter Rooms", "Release tracking is off"))
}

func TestTheItemMenuHasMarkReadAndRemoveOnlyInTheList(t *testing.T) {
	r := newRig(t, sample(), true, 1132, 900)
	defs := r.n.actions()
	byID := map[string]kit.ActionDef{}
	for _, d := range defs {
		byID[d.ID] = d
	}
	read, dismiss := byID["newreleases.mark-read"], byID["newreleases.dismiss"]
	if read.Target != kit.KindAlbum || dismiss.Target != kit.KindAlbum {
		t.Fatalf("actions = %+v", defs)
	}
	ctx := func(id string, here bool) *kit.ActionContext {
		var a services.Album
		for _, it := range sample().Items {
			if it.Album.CatalogID == id {
				a = it.Album
			}
		}
		ac := &kit.ActionContext{Items: []kit.Item{kit.AlbumItem(a)}}
		if here {
			ac.Container = &kit.ActionContainer{Kind: containerKind, ID: containerKind}
		}
		return ac
	}
	if !read.Available(ctx("303", true)) || !dismiss.Available(ctx("303", true)) {
		t.Error("an unread release's menu lacks Mark as read or Remove")
	}
	if read.Available(ctx("304", true)) {
		t.Error("Mark as read on a release already read")
	}
	if !dismiss.Available(ctx("304", true)) {
		t.Error("a read release can't be removed")
	}
	if read.Available(ctx("303", false)) || dismiss.Available(ctx("303", false)) {
		t.Error("the list's actions show in another page's album menu")
	}
	read.Run(ctx("303", true))
	dismiss.Run(ctx("304", true))
	r.until("both asked", func() bool { return slices.Equal(sorted(r.api.seen()), []string{"dismiss 304", "read 303"}) })
}

func sorted(s []string) []string { slices.Sort(s); return s }

func TestTheListMirrorsGoEvents(t *testing.T) {
	r := newRig(t, services.NewReleases{}, true, 1132, 700)
	r.until("empty", r.tt.Shows("Nothing new yet"))
	r.h.Bus.Emit(services.EventArtistReleasesChanged, sample())
	r.until("the list", r.tt.Shows("Winter Rooms"))
	got, ok := store.Peek[services.NewReleases](r.h.Env.Queries, store.KeyNewReleases)
	if !ok || got.Unread != 3 {
		t.Errorf("mirror = %+v, %t", got, ok)
	}
}

func TestDeleteRemovesTheFocusedRelease(t *testing.T) {
	r := newRig(t, sample(), true, 1132, 900)
	r.until("the list", r.tt.Shows("Winter Rooms"))
	r.tt.SetFocused(true)
	for range 3 { // Mark all as read, Clear list, the first row
		r.tt.Key(0, ui.KeyTab)
	}
	r.tt.Key(0, ui.KeyDelete)
	r.tt.Drain()
	r.until("dismissed", func() bool { return slices.Contains(r.api.seen(), "dismiss 305") })
}

// For You decides on the shelf when it first sets its shelves, so the shelf is
// drawn on the first frame when the list and the setting are known.
func TestTheShelfIsOnTheFirstFrameWhenTheListIsKnown(t *testing.T) {
	r := newShelfRig(t, sample(), true, 1132, 700)
	if !r.tt.HasText("New from Artists You Love") || !r.tt.HasText("Winter Rooms") {
		t.Errorf("the first frame lacks the shelf: %q", r.tt.Texts())
	}
}

// Init reads the list (SQLite) and the setting off the main thread, so that
// both are in the cache by the time For You asks.
func TestInitFillsTheListAndTheSettingsBeforeAPageAsks(t *testing.T) {
	hn := uitest.New(t)
	n := newNewReleases(hn.Env, &fakeAPI{})
	n.module().Init()
	deadline := time.Now().Add(10 * time.Second)
	for {
		hn.Main.Drain()
		_, hasList := store.Peek[services.NewReleases](hn.Env.Queries, store.KeyNewReleases)
		_, hasSettings := store.Peek[config.Settings](hn.Env.Queries, store.KeySettings)
		if hasList && hasSettings {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("after 10 s: list cached %t, settings cached %t", hasList, hasSettings)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
