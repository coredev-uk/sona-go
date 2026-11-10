package album

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// inOrder reports whether want is a subsequence of got.
func inOrder(got, want []string) bool {
	i := 0
	for _, g := range got {
		if i < len(want) && g == want[i] {
			i++
		}
	}
	return i == len(want)
}

func stopLabels(stops []access.Stop) []string {
	out := make([]string, len(stops))
	for i, s := range stops {
		out[i] = s.Node.Label
	}
	return out
}

// walk checks what every album page promises assistive technology: no
// control inside a leaf, every Tab stop on the tree and named, the walk not a
// trap, and want among the stops in reading order.
func (r *rig) walk(want ...string) access.Tree {
	r.t.Helper()
	r.tt.Settle()
	tt := r.tt.Tester
	access.NoNestedControls(r.t, tt)
	stops := stopLabels(access.TabWalk(r.t, tt, 120))
	if !inOrder(stops, want) {
		r.t.Errorf("Tab walk %q, want in order %q", stops, want)
	}
	tr := access.Read(r.t, tt)
	for _, n := range tr.Nodes {
		if n.Has(access.Focusable) && n.Label == "" && n.Role != ui.RoleGroup && n.Role != ui.RoleScroll {
			r.t.Errorf("nameless control %v\n%s", n, tr.Dump())
		}
	}
	return tr
}

// The catalogue album: the title is the page's h1, the artist a link, the
// actions follow it in reading order, the songs are a named table whose
// controls (Play, More) are on the tree beside their row, and every shelf is
// a heading over a named table of links.
func TestA11yCatalogueAlbum(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 2400)
	r.h.Env.Actions.Register(kit.ActionDef{
		ID: "test.album-action", Target: kit.KindAlbum, Group: kit.GroupItem, Order: 1,
		Label: func(*kit.ActionContext) string { return "Do a thing" }, Icon: "star",
		Run: func(*kit.ActionContext) {},
	})
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) {
		p.OtherVersions = []services.Album{{ID: "101", Name: "Salt Flats (Deluxe)", Artist: "Mia Vale", ReleaseDate: "2020-01-01"}}
		p.Videos = []services.MusicVideo{{ID: "v1", CatalogID: "v1", Name: "Low Tide (Video)", Artist: "Mia Vale", ReleaseDate: "2019-04-01"}}
		p.MoreToHear = []services.Station{{ID: "ra.1", Name: "Salt Flats, the story", ShowName: "Record Club", Playable: true, HasPage: true}}
		p.MoreByArtist = []services.Album{{ID: "200", Name: "Blue Hour", Artist: "Mia Vale", ReleaseDate: "2021-05-01"}}
		p.AppearsOn = []services.Playlist{{ID: "pl.1", Name: "Road Trip", Curator: "Apple Music Indie"}}
		p.Similar = []services.Album{{ID: "300", Name: "Dust", Artist: "Cy Moor"}}
	}))
	r.shows("Other versions", "Music videos", "More to hear")
	tr := r.walk(
		"Mia Vale", "Play Salt Flats", "Shuffle", "Add", "Favourite Salt Flats", "Suggest less like Salt Flats", "More for Salt Flats",
		"Salt Flats", // the first song's row
		"Salt Flats (Deluxe)", "Low Tide (Video)", "Salt Flats, the story", "Play Salt Flats, the story",
		"See all", "Blue Hour", "Road Trip", "Dust",
	)

	if h, ok := tr.Find(ui.RoleHeading, "Salt Flats"); !ok || h.Level != 1 {
		t.Errorf("the title is not the h1: %+v", h)
	}
	if _, ok := tr.Find(ui.RoleLink, "Mia Vale"); !ok {
		t.Error("the artist is not a link")
	}
	table, ok := tr.Find(ui.RoleTable, "Songs on Salt Flats")
	if !ok {
		t.Fatalf("no named song table:\n%s", tr.Dump())
	}
	// Hover-only controls are on the tree at rest, beside their row, none
	// of them a Tab stop of its own.
	for _, song := range []string{"Low Tide", "Paper Moon"} {
		more, ok := tr.Find(ui.RoleMenuButton, "More for "+song)
		if !ok {
			t.Errorf("no More button for %q", song)
			continue
		}
		if more.Has(access.Focusable) || !tr.Within(more, ui.RoleTable) {
			t.Errorf("More for %q: %+v", song, more)
		}
		if tr.Within(more, ui.RoleButton) || tr.Within(more, ui.RoleLink) {
			t.Errorf("More for %q is inside a control", song)
		}
	}
	if _, ok := tr.Find(ui.RoleButton, "Play Low Tide"); !ok {
		t.Error("no Play button on the song's row at rest")
	}
	if len(tr.Children(table)) < 6 {
		t.Errorf("the table holds %d children, want its header and five songs", len(tr.Children(table)))
	}
	for _, shelf := range []string{"Other versions", "Music videos", "More to hear", "More by Mia Vale", "Featured in playlists", "You might also like"} {
		if h, ok := tr.Find(ui.RoleHeading, shelf); !ok || h.Level != 2 {
			t.Errorf("shelf %q has no h2: %+v", shelf, h)
		}
		if _, ok := tr.Find(ui.RoleTable, shelf); !ok {
			t.Errorf("shelf %q has no named table", shelf)
		}
	}
	// The header menu is a menu button, named for what it opens.
	if _, ok := tr.Find(ui.RoleMenuButton, "More for Salt Flats"); !ok {
		t.Error("the header menu is not a menu button")
	}
}

// The header menu closes on Escape and the focus returns to its button.
func TestA11yHeaderMenuEscapeReturnsTheFocus(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.h.Env.Actions.Register(kit.ActionDef{
		ID: "test.album-action", Target: kit.KindAlbum, Group: kit.GroupItem, Order: 1,
		Label: func(*kit.ActionContext) string { return "Do a thing" }, Icon: "star",
		Run: func(*kit.ActionContext) {},
	})
	r.addCatalog(saltFlats())
	r.shows("More for Salt Flats")
	r.click("More for Salt Flats")
	r.tt.Until("the menu", r.tt.Shows("Do a thing"))
	if n, ok := access.Read(t, r.tt.Tester).Find(ui.RoleMenuButton, "More for Salt Flats"); !ok || !n.Has(access.Expanded) {
		t.Errorf("the open menu's button is not expanded: %+v", n)
	}
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Advance(300 * time.Millisecond)
	r.tt.Drain()
	if r.tt.HasText("Do a thing") {
		t.Fatal("Escape left the menu open")
	}
	if !r.tt.Focused("More for Salt Flats") {
		t.Error("the focus did not return to the menu's button")
	}
}

// A multi-disc album's index is a named list of items, each holding the
// button that moves to the disc; the discs' headings are in the table.
func TestA11yDiscIndexIsAListOfItems(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 600)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) {
		p.Tracks = nil
		for i := 1; i <= 40; i++ {
			p.Tracks = append(p.Tracks, track(i, fmt.Sprintf("Song %02d", i), func(s *services.Song) {
				s.DiscNumber, s.TrackNumber = 1+(i-1)/20, 1+(i-1)%20
			}))
		}
		p.DurationMs = 40 * 200000
	}))
	r.shows("Discs")
	tr := r.walk("Mia Vale", "Play Salt Flats", "Disc 1 · 20 songs, 1 hr, 7 min", "Disc 2 · 20 songs, 1 hr, 7 min", "Song 01")
	list, ok := tr.Find(ui.RoleList, "Discs")
	if !ok {
		t.Fatalf("no list named Discs:\n%s", tr.Dump())
	}
	items := tr.Children(list)
	if len(items) != 2 {
		t.Fatalf("the disc list holds %v, want two items", items)
	}
	for i, it := range items {
		kids := tr.Children(it)
		if it.Role != ui.RoleListItem || len(kids) != 1 || kids[0].Role != ui.RoleButton {
			t.Errorf("disc %d: %v holds %v, want a list item with one button", i+1, it, kids)
		}
	}
	if _, ok := tr.Find(ui.RoleHeading, "Disc 1"); !ok {
		t.Error("the disc's heading is not in the list")
	}
}

// An album not out yet: its release is one group read in order (the time
// left, then the day), the actions follow it, and the songs are in two
// groups named by headings in the table, so no row has to say when.
func TestA11yUnreleasedAlbum(t *testing.T) {
	// A platform with notifications, so the page offers Remind me.
	r := newRig(t, "/album/100", 1132, 1000, uitest.Platform(func(p *services.Platform) {
		p.Notifications = func(context.Context, func(string)) (services.Notifier, error) { return nil, errors.ErrUnsupported }
	}))
	release := r.now.Add(5*time.Hour + 12*time.Minute)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) {
		p.Album.Prerelease, p.Album.ReleaseAt, p.Album.ReleaseDate = true, release.UnixMilli(), release.Format("2006-01-02")
		for i := 2; i < 5; i++ {
			p.Tracks[i].Playable = false
		}
	}))
	r.shows("Out in 5 hours, 12 minutes", "Pre-add")
	tr := r.walk("Mia Vale", "Play the 2 songs out now", "Pre-add", "Remind me", "Favourite Salt Flats")
	if _, ok := tr.Find(ui.RoleButton, "Play the 2 songs out now"); !ok {
		t.Errorf("no play button for the songs that are out:\n%s", tr.Dump())
	}
	if slices.ContainsFunc(tr.Nodes, func(n access.Node) bool { return n.Label == "Shuffle" }) {
		t.Error("an unreleased album offers Shuffle")
	}
	// The release is one group: the time left, then the day, before the
	// actions in reading order.
	group, ok := tr.Find(ui.RoleGroup, "Release")
	if !ok {
		t.Fatalf("no Release group:\n%s", tr.Dump())
	}
	var said []string
	for _, n := range tr.Children(group) {
		said = append(said, n.Label)
	}
	if want := []string{"Out in 5 hours, 12 minutes", fullReleaseDate(release.UnixMilli())}; !slices.Equal(said, want) {
		t.Errorf("the Release group reads %q, want %q", said, want)
	}
	// The songs that are out and the ones to come are headings in the table.
	badge := kit.ReleaseBadge(services.Album{Prerelease: true, ReleaseAt: release.UnixMilli()}, r.now)
	for _, h := range []string{"Out now", badge} {
		if n, ok := tr.Find(ui.RoleHeading, h); !ok || n.Level == 0 {
			t.Errorf("no heading %q in the songs: %+v", h, n)
		}
	}
	// The Remind me button is one control, not two.
	if !kit.RemindAvailable(r.h.Env.Core, services.Album{CatalogID: "100", ReleaseAt: release.UnixMilli()}) {
		t.Fatal("the rig's platform has no notifications: Remind me is not built")
	}
	if n := len(tr.FindAll(ui.RoleToggleButton, "Remind me")) + len(tr.FindAll(ui.RoleButton, "Remind me")); n != 1 {
		t.Errorf("%d Remind me buttons, want one:\n%s", n, tr.Dump())
	}
}

// What the account has done about an album not out yet is said in the release
// group and by the controls: Pre-added is the library button, Remind me is
// pressed.
func TestA11yUnreleasedAlbumDoneStates(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000, uitest.Platform(func(p *services.Platform) {
		p.Notifications = func(context.Context, func(string)) (services.Notifier, error) { return nil, errors.ErrUnsupported }
	}))
	release := r.now.Add(48 * time.Hour)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) {
		p.Album.Prerelease, p.Album.ReleaseAt, p.Album.ReleaseDate = true, release.UnixMilli(), release.Format("2006-01-02")
		for i := range p.Tracks {
			p.Tracks[i].Playable = false
		}
	}))
	r.states.inLib["100"] = true
	store.Set(r.h.Env.Queries, store.KeyReminders, []services.Reminder{{AlbumID: "100"}})
	r.shows("Out in 2 days")
	r.tt.Until("the done states", r.tt.Shows(fullReleaseDate(release.UnixMilli())+" · Pre-added · Reminder set"))
	tr := access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleButton, "Pre-added. Delete “Salt Flats” from your library"); !ok {
		t.Errorf("the library button doesn't say Pre-added:\n%s", tr.Dump())
	}
	n, ok := tr.Find(ui.RoleToggleButton, "Remind me")
	if !ok || !n.Has(access.Checked) {
		t.Errorf("Remind me is not pressed: %+v\n%s", n, tr.Dump())
	}
	// Nothing is out: no Play, and no heading for songs out now.
	if _, ok := tr.Find(ui.RoleHeading, "Out now"); ok {
		t.Error("a heading for songs out now when there are none")
	}
}

// A library album holding part of the record links to the complete one.
func TestA11yPartialLibraryAlbum(t *testing.T) {
	r := newRig(t, "/album/l.9", 1132, 1000)
	r.addCatalog(saltFlats())
	r.lib.albums["l.9"] = services.AlbumDetail{
		Album:  services.Album{ID: "l.9", Source: services.SourceLibrary, CatalogID: "100", Name: "Salt Flats", Artist: "Mia Vale", ReleaseDate: "2019-03-08"},
		Tracks: fiveTracks()[:2], DurationMs: 400000,
	}
	r.shows("Show complete album")
	tr := r.walk("Mia Vale", "Show complete album", "Play Salt Flats")
	if _, ok := tr.Find(ui.RoleLink, "Show complete album"); !ok {
		t.Errorf("Show complete album is not a link:\n%s", tr.Dump())
	}
}

// The states of the page: loading is a status named Loading with nothing to
// focus, not found and a failure are announced and offer their way out.
func TestA11yLoadingAndErrorStates(t *testing.T) {
	t.Run("loading", func(t *testing.T) {
		r := newRig(t, "/album/100", 1132, 1000)
		r.cat.gate = make(chan struct{})
		r.addCatalog(saltFlats())
		r.open()
		r.tt.Settle()
		tr := access.Read(t, r.tt.Tester)
		if _, ok := tr.Find(ui.RoleStatus, "Loading"); !ok {
			t.Fatalf("no Loading status:\n%s", tr.Dump())
		}
		for _, n := range tr.Nodes {
			if n.Has(access.Focusable) && n.Role != ui.RoleGroup {
				t.Errorf("the skeleton has a stop: %v", n)
			}
		}
		access.NoNestedControls(t, r.tt.Tester)
		close(r.cat.gate)
	})
	t.Run("not found", func(t *testing.T) {
		r := newRig(t, "/album/999", 1132, 1000)
		r.shows("This album isn't available", "Go to Albums")
		tr := r.walk("Go to Albums")
		if h, ok := tr.Find(ui.RoleHeading, "This album isn't available"); !ok || h.Level == 0 {
			t.Errorf("no heading for the missing album: %+v", h)
		}
	})
	t.Run("failure", func(t *testing.T) {
		r := newRig(t, "/album/100", 1132, 1000)
		r.addCatalog(saltFlats())
		r.cat.fail(errors.New("boom"))
		r.shows("Couldn't load this album", "Try again")
		tr := r.walk("Try again")
		if _, ok := tr.Find(ui.RoleAlert, "Couldn't load this album"); !ok {
			t.Errorf("the failure is not an alert:\n%s", tr.Dump())
		}
	})
}
