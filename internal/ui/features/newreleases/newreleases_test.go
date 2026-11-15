package newreleases

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
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
	r.until("the list", r.tt.Shows("New releases", "Coming soon", "This week", "Earlier",
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

func TestTheSidebarCountIsTheUnreadReleases(t *testing.T) {
	r := newRig(t, sample(), true, 1132, 900)
	nav := r.n.module().Nav
	if len(nav) != 1 || nav[0].To != "/new-releases" || nav[0].Count == nil {
		t.Fatalf("nav = %+v", nav)
	}
	if got := nav[0].Count(); got != 3 {
		t.Errorf("count = %d, want the 3 unread of the sample", got)
	}
	// A change from Go moves it.
	list := sample()
	list.Items[1].Read, list.Unread = true, 2
	r.h.Bus.Emit(services.EventArtistReleasesChanged, list)
	r.tt.Drain()
	if got := nav[0].Count(); got != 2 {
		t.Errorf("count after a change = %d, want 2", got)
	}
	// Before the list is known there is nothing to count.
	e := newRig(t, services.NewReleases{}, true, 800, 400)
	if got := e.n.module().Nav[0].Count(); got != 0 {
		t.Errorf("empty count = %d", got)
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
