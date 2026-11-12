package artist

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The discography is a career line: the releases the page holds (albums,
// singles and EPs, live albums, compilations) merged and grouped by year,
// newest first. The year is the left gutter, a thin rail links the years, and
// each year's releases are compact rows to its right.

// timelineMax is how many releases the timeline shows; the shelves' views hold
// the rest ("See all").
const timelineMax = 18

// timelineYear is one year of the career line.
type timelineYear struct {
	// label is the year, "" for releases with no date.
	label string
	albums []*services.Album
	// lines are the kind (and the badge of an upcoming one) under each title.
	lines []string
}

// timelineLink is a shelf's page, offered under the timeline when the
// timeline does not show all of it.
type timelineLink struct{ view string }

// releaseKind is the kind a release is listed as: the view it came from, or
// for singles and EPs what its title says.
func releaseKind(view string, al services.Album) string {
	switch view {
	case services.ViewSingles:
		if _, kind := releaseTitle(al.Name); kind == "ep" {
			return i18n.T("EP")
		}
		return i18n.T("Single")
	case services.ViewLiveAlbums:
		return i18n.T("Live")
	case services.ViewCompilations:
		return i18n.T("Compilation")
	}
	switch _, kind := releaseTitle(al.Name); {
	case kind == "ep":
		return i18n.T("EP")
	case kind == "single" || al.Single:
		return i18n.T("Single")
	}
	return i18n.T("Album")
}

// buildTimeline merges the shelves' releases (once each, the first shelf
// that has one naming its kind), orders them newest first, keeps timelineMax
// and groups them by year. more are the shelves the timeline does not show
// whole, in the order given.
func buildTimeline(shelves []services.ArtistShelf, now time.Time) (years []timelineYear, more []timelineLink) {
	type entry struct {
		al   *services.Album
		kind string
		view string
	}
	var all []entry
	seen := map[string]bool{}
	for _, sh := range shelves {
		if !isArtistView(sh.View) {
			continue
		}
		for i := range sh.Albums {
			al := &sh.Albums[i]
			if seen[al.ID] {
				continue
			}
			seen[al.ID] = true
			all = append(all, entry{al, releaseKind(sh.View, *al), sh.View})
		}
	}
	slices.SortStableFunc(all, func(x, y entry) int {
		// ISO dates and bare years order as strings; no date goes last.
		switch dx, dy := x.al.ReleaseDate, y.al.ReleaseDate; {
		case dx == "" && dy != "":
			return 1
		case dy == "" && dx != "":
			return -1
		default:
			return -cmp.Compare(dx, dy)
		}
	})
	if len(all) > timelineMax {
		all = all[:timelineMax]
	}
	shown := map[string]int{}
	for _, e := range all {
		shown[e.view]++
		label := i18n.YearOf(e.al.ReleaseDate)
		if len(years) == 0 || years[len(years)-1].label != label {
			years = append(years, timelineYear{label: label})
		}
		y := &years[len(years)-1]
		y.albums = append(y.albums, e.al)
		line := e.kind
		if badge := releaseBadge(*e.al, now); badge != "" {
			line += " · " + badge
		}
		y.lines = append(y.lines, line)
	}
	for _, sh := range shelves {
		if isArtistView(sh.View) && len(sh.Albums) > 0 && (sh.More || shown[sh.View] < len(sh.Albums)) {
			more = append(more, timelineLink{sh.View})
		}
	}
	return years, more
}

// timeline builds the career line; it builds nothing, and says so, when the
// shelves hold no release.
func (a *artist) timeline(c *ui.Context, pg *pageState, shelves []services.ArtistShelf, art services.Artist, width float32) bool {
	years, more := buildTimeline(shelves, a.now())
	if len(years) == 0 {
		return false
	}
	tk := theme.Current()
	h := tk.Type["heading"]
	narrowGutter := width > 0 && width < narrowUnder
	yearW := float32(64)
	if narrowGutter {
		yearW = 52
	}
	cols := 1
	if width >= timelineTwoFrom {
		cols = 2
	}
	ui.Column(c.Key("timeline")).FillWidth().Children(func() {
		for i, y := range years {
			last := i == len(years)-1 && len(more) == 0
			label := y.label
			if label == "" {
				label = i18n.T("Undated")
			}
			ui.Row(c.Key("year:"+y.label)).FillWidth().AlignItems(ui.Stretch).Role(ui.RoleGroup).Label(label).Children(func() {
				ui.Column(c.Key("gutter")).Width(yearW).Shrink(0).Children(func() {
					ui.Text(c, label).FontSize(h.Size).FixedLineHeight(h.LineHeight).FontWeight(int(h.Weight)).
						FontFeatures("tnum").TextColor(theme.C("ink-faint")).SingleLine().Margin(15, 0, 0, 0)
				})
				ui.Column(c.Key("rail")).Width(9).Shrink(0).AlignItems(ui.Center).Children(func() {
					ui.Box(c.Key("dot")).Size(9, 9).Shrink(0).Radius(tk.Radius["full"]).Background(theme.C("control-line")).Margin(21, 0, 0, 0)
					if !last {
						ui.Box(c.Key("line")).Width(1).Grow(1).Background(theme.C("line"))
					}
				})
				ui.Column(c.Key("releases")).Grow(1).Basis(0).MinWidth(0).Padding(0, 0, 0, 14).Children(func() {
					a.releaseList(c, pg.list("year:"+y.label), y.albums, releaseOpts{
						label: label, columns: yearCols(cols, len(y.albums)), lines: y.lines,
					})
					if !last {
						ui.Box(c.Key("gap")).Height(12)
					}
				})
			})
		}
		if len(more) == 0 {
			return
		}
		ui.Row(c.Key("more")).Wrap().Gap(4).Margin(8, 0, 0, yearW+9+14-10).Children(func() {
			for _, l := range more {
				title, _ := viewTitle(l.view, art.Name)
				path := viewPath(art.ID, l.view)
				if !a.has(path) {
					continue
				}
				b := kit.Button(c.Key("all:"+l.view), strings.TrimSpace(title), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Icon: "chevron-right", IconAfter: true})
				if b.Clicked() {
					a.env.Nav.Push(path)
				}
			}
		})
	})
	return true
}

// yearCols is the columns a year's releases flow into: one, or the page's
// two when the year has enough to fill them.
func yearCols(page, releases int) int {
	if page > 1 && releases >= 4 {
		return page
	}
	return 1
}
