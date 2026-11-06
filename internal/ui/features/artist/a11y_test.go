package artist

import (
	"errors"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
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

// walk checks what every artist page promises assistive technology: no
// control inside a leaf, every Tab stop on the tree and named, the walk not a
// trap, and want among the stops in reading order. It returns the stops.
func (r *rig) walk(want ...string) ([]string, access.Tree) {
	r.t.Helper()
	r.tt.Settle()
	tt := r.tt.Tester
	access.NoNestedControls(r.t, tt)
	stops := stopLabels(access.TabWalk(r.t, tt, 150))
	if !inOrder(stops, want) {
		r.t.Errorf("Tab walk %q, want in order %q", stops, want)
	}
	tr := access.Read(r.t, tt)
	for _, n := range tr.Nodes {
		if n.Has(access.Focusable) && n.Label == "" && n.Role != ui.RoleGroup && n.Role != ui.RoleScroll {
			r.t.Errorf("nameless control %v\n%s", n, tr.Dump())
		}
	}
	return stops, tr
}

// The catalogue artist: the name is the h1 and the bands h2s, the controls
// follow in reading order, the bands are not Tab stops of their own, and the
// release lists are tables of rows holding the release's link and its More
// button side by side.
func TestA11yCatalogueArtist(t *testing.T) {
	r := catalogRig(t, nil)
	r.shows("Discography", "Related")
	stops, tr := r.walk(
		"Read more", "Play Radiohead's top songs", "Shuffle", "Favourite Radiohead", "Suggest less like Radiohead", "More for Radiohead",
		"Hail to the Thief (Live Recordings 2003-2009)",
		"See all", "Song 1",
		"OK Computer", "Kid A",
		"In Rainbows", "See all", "Daydreaming", "Com Lag: 2+2=5", "KID A MNESIA",
		"Creep (Very 2021 Rmx)", "Radiohead Essentials", "Thom Yorke",
	)
	for _, band := range []string{"Discography", "Related", "About Radiohead"} {
		if slices.Contains(stops, band) {
			t.Errorf("the %q band is a Tab stop before anything asked for it", band)
		}
	}
	if h, ok := tr.Find(ui.RoleHeading, "Radiohead"); !ok || h.Level != 1 {
		t.Errorf("the name is not the h1: %+v", h)
	}
	for _, h2 := range []string{"Top songs", "Essential albums by Radiohead", "Discography", "Related", "Albums by Radiohead", "About Radiohead"} {
		if h, ok := tr.Find(ui.RoleHeading, h2); !ok || h.Level != 2 {
			t.Errorf("no h2 %q: %+v", h2, h)
		}
	}
	// The latest release is a link named by its title, its lines described.
	slip, ok := tr.Find(ui.RoleLink, "Hail to the Thief (Live Recordings 2003-2009)")
	if !ok || slip.Description != "Latest release · May 3, 2016 · Album" {
		t.Errorf("latest release link %+v", slip)
	}
	// A release is a row holding one cell: the link (the opener) and, beside
	// it and not inside, the More menu button, on the tree at rest.
	table, ok := tr.Find(ui.RoleTable, "Essential albums by Radiohead")
	if !ok {
		t.Fatalf("no release table:\n%s", tr.Dump())
	}
	rows := tr.Children(table)
	if len(rows) != 2 {
		t.Fatalf("the essential albums are %v, want two rows", rows)
	}
	for _, row := range rows {
		if row.Role != ui.RoleRow {
			t.Errorf("%v is not a row", row)
			continue
		}
		cells := tr.Children(row)
		if len(cells) != 1 || cells[0].Role != ui.RoleCell {
			t.Errorf("%v holds %v, want one cell", row, cells)
			continue
		}
		var roles []ui.Role
		for _, n := range tr.Children(cells[0]) {
			roles = append(roles, n.Role)
		}
		if !slices.Equal(roles, []ui.Role{ui.RoleLink, ui.RoleMenuButton}) {
			t.Errorf("%v holds %v, want the link then the More button", cells[0], roles)
		}
	}
	if more, ok := tr.Find(ui.RoleMenuButton, "More for Kid A"); !ok || more.Has(access.Focusable) {
		t.Errorf("More for Kid A: %+v (on the tree, not a Tab stop)", more)
	}
}

// A selected release is checked on its cell, and Cmd-click keeps the link a
// link.
func TestA11yReleaseSelectionIsOnTheTree(t *testing.T) {
	r := catalogRig(t, nil)
	r.shows("Essential albums by Radiohead", "Kid A")
	r.tt.Drain()
	r.clickWith(ui.Cmd, "Kid A")
	tr := access.Read(t, r.tt.Tester)
	table, _ := tr.Find(ui.RoleTable, "Essential albums by Radiohead")
	var checked []string
	for _, row := range tr.Children(table) {
		for _, cell := range tr.Children(row) {
			if cell.Has(access.Checked) {
				for _, n := range tr.Children(cell) {
					if n.Role == ui.RoleLink {
						checked = append(checked, n.Label)
					}
				}
			}
		}
	}
	if !slices.Equal(checked, []string{"Kid A"}) {
		t.Errorf("checked releases %q, want Kid A\n%s", checked, tr.Dump())
	}
}

// Read more moves the focus to the About band, which Tab never stops at.
func TestA11yReadMoreFocusesAbout(t *testing.T) {
	r := newRig(t, "/artist/657515", 1132, 600, func(r *rig) { r.cat.page = radiohead() })
	r.shows("Read more")
	r.tt.Drain()
	if err := r.tt.Click("Read more"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the About band", func() bool { return r.tt.Focused("About Radiohead") })
	tr := access.Read(t, r.tt.Tester)
	if tr.Focus == nil || tr.Focus.Label != "About Radiohead" || tr.Focus.Role != ui.RoleGroup {
		t.Fatalf("focus %+v, want the About group", tr.Focus)
	}
	access.NoNestedControls(t, r.tt.Tester)
	for _, l := range access.TabWalk(t, r.tt.Tester, 150) {
		if l.Node.Label == "About Radiohead" {
			t.Errorf("Tab stops at the About band it jumped to")
		}
	}
}

// The artist's concerts section, built by its own feature, sits between the
// lead and the Discography in the reading and the Tab order.
func TestA11yConcertsShelfSitsAfterTheLead(t *testing.T) {
	r := newRig(t, "/artist/657515", 1132, 1500, func(r *rig) {
		r.cat.page = radiohead()
		r.h.Env.ArtistConcerts = func(c *ui.Context, id string, first bool) {
			ui.Column(c.Key("concerts")).Role(ui.RoleGroup).Label("Upcoming concerts").Children(func() {
				ui.Text(c, "Upcoming concerts").Role(ui.RoleHeading).Level(2)
				kit.Button(c.Key("tickets"), "See tickets", kit.ButtonOpts{})
			})
		}
	})
	r.shows("Discography", "Upcoming concerts")
	_, tr := r.walk("Kid A", "See tickets", "In Rainbows")
	if h, ok := tr.Find(ui.RoleHeading, "Upcoming concerts"); !ok || h.Level != 2 {
		t.Errorf("concerts heading %+v", h)
	}
}

// A library artist: the library's shelves come first, then the catalogue's.
func TestA11yLibraryArtist(t *testing.T) {
	r := libraryRig(t, true)
	r.shows("Albums in your library", "Songs in your library")
	_, tr := r.walk("Read more", "Play Mia", "Own Album", "Second Album", "Short One", "Mine 1", "See all", "Song 1")
	for _, name := range []string{"Albums in your library", "Singles and EPs in your library", "Songs in your library"} {
		if h, ok := tr.Find(ui.RoleHeading, name); !ok || h.Level != 2 {
			t.Errorf("no h2 %q", name)
		}
	}
	if _, ok := tr.Find(ui.RoleTable, "Songs by Mia in your library"); !ok {
		t.Errorf("the library songs are not a named table:\n%s", tr.Dump())
	}
}

// The see-all pages: an h1, a link back to the artist, and the list (rows of
// songs, or a grid of tiles) as one named table whose controls are reachable.
func TestA11yViewPages(t *testing.T) {
	t.Run("top songs", func(t *testing.T) {
		r := viewRig(t, "top-songs", func(r *rig) { r.cat.views["top-songs"] = songPages(30) })
		r.shows("Top songs by Radiohead", "Song 1")
		_, tr := r.walk("Radiohead", "Song 1")
		if h, ok := tr.Find(ui.RoleHeading, "Top songs by Radiohead"); !ok || h.Level != 1 {
			t.Errorf("no h1: %+v", h)
		}
		if _, ok := tr.Find(ui.RoleLink, "Radiohead"); !ok {
			t.Error("the subline does not link back to the artist")
		}
		if _, ok := tr.Find(ui.RoleButton, "Play Song 2"); !ok {
			t.Error("no Play button on a row at rest")
		}
	})
	t.Run("albums", func(t *testing.T) {
		r := viewRig(t, "full-albums", func(r *rig) { r.cat.views["full-albums"] = albumPages("full-albums", 30) })
		r.tt.Drain()
		_, tr := r.walk("Radiohead", "Record 0", "Record 1", "Record 9")
		if h, ok := tr.Find(ui.RoleHeading, "Albums by Radiohead"); !ok || h.Level != 1 {
			t.Errorf("no h1: %+v", h)
		}
		if more, ok := tr.Find(ui.RoleMenuButton, "More for Record 0"); !ok || more.Has(access.Focusable) {
			t.Errorf("More for Record 0: %+v", more)
		}
	})
}

// Loading is a status named Loading with nothing to focus; not found and a failure are statuses with
// their way out.
func TestA11yLoadingAndErrorStates(t *testing.T) {
	t.Run("loading", func(t *testing.T) {
		r := newRig(t, "/artist/657515", 1132, 900, func(r *rig) {
			r.cat.page = radiohead()
			r.cat.gate = make(chan struct{})
		})
		r.tt.Drain()
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
		r := newRig(t, "/artist/1", 1132, 900, func(r *rig) { r.cat.fail(applemusic.ErrNotFound) })
		r.shows("This artist isn't available", "Go to Artists")
		_, tr := r.walk("Go to Artists")
		if _, ok := tr.Find(ui.RoleHeading, "This artist isn't available"); !ok {
			t.Errorf("no heading:\n%s", tr.Dump())
		}
	})
	t.Run("failure", func(t *testing.T) {
		r := newRig(t, "/artist/657515", 1132, 900, func(r *rig) {
			r.cat.page = radiohead()
			r.cat.fail(errors.New("boom"))
		})
		r.shows("Couldn't load this artist", "Try again")
		_, tr := r.walk("Try again")
		if _, ok := tr.Find(ui.RoleStatus, "Couldn't load this artist"); !ok {
			t.Errorf("the failure is not a status:\n%s", tr.Dump())
		}
	})
}
