package kit

import (
	"slices"
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// heroRatio is the hero card's shape on music.apple.com: its "flowcaseLockup"
// requests the 4:1 subscriptionHero art cropped to 1060×608 ("sr").
const heroRatio = 1060.0 / 608.0

// textMax is the widest a paragraph gets: 72ch, a "0" of the interface font at
// the 14 DIP body size being 9 DIPs wide (measured with Context.MeasureText).
const textMax = 72 * 9

// EditorialState is the state of an EditorialSections: one for each section,
// by its ID, made when it first shows. Keep one per page.
type EditorialState struct {
	shelves  map[string]*CoverShelfState
	wides    map[string]*WideRowState
	tracks   map[string]*TrackShelfState
	links    map[string]*LinkListState
	stations map[string]*StationListState
}

// stateOf is the state of a section: the one in m by key, made when there is
// none.
func stateOf[S any](m *map[string]*S, key string) *S {
	if *m == nil {
		*m = map[string]*S{}
	}
	s := (*m)[key]
	if s == nil {
		s = new(S)
		(*m)[key] = s
	}
	return s
}

// EditorialOpts are the options of EditorialSections.
type EditorialOpts struct {
	// First leaves out the 40 DIPs above the first section: nothing is above
	// it on the page.
	First bool
	// MoreFor is where a section's "See all" goes, as a route path ("" for
	// none); default RoomLink, the section's room.
	MoreFor func(services.Section) string
}

// EditorialSections renders editorial and recommendation sections by kind
// (v0.2.0's EditorialSections): wide hero cards, feature and cover shelves,
// track blocks, station lists, category bricks, link lists and paragraphs.
// New, Radio, curators, groupings, multirooms and Home share it. Sections are
// stacked 40 DIPs apart.
func EditorialSections(c *ui.Context, st *EditorialState, sections []services.Section, ctx ItemsCtx, o EditorialOpts) {
	moreFor := o.MoreFor
	if moreFor == nil {
		moreFor = RoomLink
	}
	// The menus' popup is drawn once, after the last section.
	popup := ctx.Menus != nil && !ctx.NoPopup
	ctx.NoPopup = ctx.NoPopup || popup
	for i := range sections {
		s := sections[i]
		// Each section is built under a key of its own: sections may share a
		// title (two untitled heroes, "More links").
		ui.Column(c.Key(strconv.Itoa(i) + ":" + s.ID)).Children(func() {
			st.section(c, s, ctx, o.First && i == 0, moreFor(s))
		})
	}
	if popup {
		ctx.Menus.Popup(c)
	}
}

// section builds one section by its kind.
func (st *EditorialState) section(c *ui.Context, s services.Section, ctx ItemsCtx, first bool, more string) {
	switch s.Kind {
	case services.SectionHero:
		WideRow(c, stateOf(&st.wides, s.ID), s.Items, ctx, WideRowOpts{
			Title: s.Title, First: first, Min: 420, Ratio: heroRatio, Crop: true,
		})
	case services.SectionBricks:
		WideRow(c, stateOf(&st.wides, s.ID), s.Items, ctx, WideRowOpts{
			Title: s.Title, First: first, Min: 240, Ratio: 2,
		})
	case services.SectionTracks:
		TrackShelf(c, stateOf(&st.tracks, s.ID), s.Items, ctx, TrackShelfOpts{Title: s.Title, First: first, More: more})
	case services.SectionLinks:
		LinkList(c, stateOf(&st.links, s.ID), s.Items, ctx, LinkListOpts{Title: s.Title, First: first})
	case services.SectionStations:
		Shelf(c, ShelfOpts{Title: s.Title, First: first, More: moreFunc(&ctx, more)}, func() {
			var stations []services.Station
			for j := range s.Items {
				if s.Items[j].Station != nil {
					stations = append(stations, *s.Items[j].Station)
				}
			}
			StationList(c, stateOf(&st.stations, s.ID), stations, ctx, StationListOpts{
				Label: s.Title,
				OnPlay: func(station services.Station) {
					// Playing a station plays the section's item for it.
					at := slices.IndexFunc(s.Items, func(it services.Item) bool {
						return it.Station != nil && it.Station.ID == station.ID
					})
					if at >= 0 {
						ctx.Players.PlayItem(s.Items[at])
					}
				},
			})
		})
	case services.SectionText:
		tk := theme.Current()
		body := tk.Type["ui"]
		p := ui.Text(c, s.Text).FontSize(body.Size).FixedLineHeight(body.LineHeight).
			TextColor(theme.C("ink-muted")).MaxWidth(textMax)
		if !first {
			p.Margin(40, 0, 0, 0)
		}
	case services.SectionPlaylists:
		ItemShelf(c, stateOf(&st.shelves, s.ID), i18n.T("Playlists"), s.Items, ctx, ItemShelfOpts{First: first, More: more})
	case services.SectionFeature:
		ItemShelf(c, stateOf(&st.shelves, s.ID), s.Title, s.Items, ctx, ItemShelfOpts{First: first, More: more, Min: 208})
	default:
		ItemShelf(c, stateOf(&st.shelves, s.ID), s.Title, s.Items, ctx, ItemShelfOpts{First: first, More: more})
	}
}

// moreFunc is a "See all" to a route, nil for none.
func moreFunc(ctx *ItemsCtx, path string) func() {
	if path == "" {
		return nil
	}
	return func() { ctx.nav(path) }
}
