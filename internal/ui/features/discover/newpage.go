package discover

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// newPage is the state of Discover's page.
type newPage struct {
	pg    pageView
	menus *kit.ActionMenus
	ed    kit.EditorialState

	// What loads: the skeletons' measures.
	frontSkel, shelvesSkel measure

	// The issue is what the data makes of the page: the front page's cards
	// and the shelves in bands. It is made again when the data changes.
	issue      issue
	issueFirst *services.Section
	issueLen   int

	// anchors are the places in the page's content the contents jump to, by
	// anchor, as the last frame laid them out; focus is the anchor that holds
	// the keyboard focus after a jump (focusNow: it takes it in this frame).
	anchors  map[string]float32
	focus    string
	focusNow bool
}

// issue is the page's sections as v0.2.0's Issue sets them: the hero is the
// front page, the rest are in bands.
type issue struct {
	hero  []services.Item
	bands []issueBand
}

type issueBand struct {
	band
	// anchor and title are the band's; each section's anchor is
	// anchor-<index>.
	anchor, title string
}

// makeIssue sets the page: the first hero section is the front page, and
// what it shows isn't repeated in the cover shelves below it (unless that
// would empty the shelf); the other sections go to their bands.
func makeIssue(sections []services.Section) issue {
	var out issue
	hero := -1
	for i := range sections {
		if sections[i].Kind == services.SectionHero {
			hero = i
			break
		}
	}
	featured := map[string]bool{}
	if hero >= 0 {
		out.hero = sections[hero].Items
		for _, it := range out.hero {
			if it.ID != "" {
				featured[it.Kind+":"+it.ID] = true
			}
		}
	}
	var below []services.Section
	for i, s := range sections {
		if i == hero {
			continue
		}
		if s.Kind == services.SectionShelf || s.Kind == services.SectionFeature {
			var items []services.Item
			for _, it := range s.Items {
				if !featured[it.Kind+":"+it.ID] {
					items = append(items, it)
				}
			}
			if len(items) > 0 {
				s.Items = items
			}
		}
		below = append(below, s)
	}
	for _, b := range toBands(below) {
		out.bands = append(out.bands, issueBand{band: b, anchor: "new:" + string(b.id), title: bandTitle(b.id)})
	}
	return out
}

// newPage is Discover: Apple Music's editorial front page, set as Sona's own
// issue: the featured cards as a lead and a list, the contents, then the
// shelves in bands (songs and albums, playlists, radio, videos, browse).
func (d *discover) newPage(c *ui.Context, r *ui.Route) {
	st := &d.newPg
	q := d.editorial(kindNew, "")
	st.pg.body(c, pathNew, func(_ *kit.Pane, _ float32) {
		switch {
		case q.HasData:
			d.pageHeader(c, i18n.T("Discover"), freshness(q.Data.FetchedAt, q.Data.Stale, d.now()), false)
			if len(q.Data.Sections) == 0 {
				d.emptyPage(c)
				return
			}
			d.issueOf(q.Data.Sections)
			d.issueView(c)
		case q.Err != nil:
			d.pageHeader(c, i18n.T("Discover"), "", false)
			d.pageError(c, i18n.T("Couldn't load what's new"), q.Err, q.Fetching, d.retry(keyPage(kindNew, "")))
		default:
			d.pageHeader(c, i18n.T("Discover"), "", true)
			d.newSkeleton(c)
		}
	})
}

// issueOf makes the issue of the sections when they are not the ones it was
// made of.
func (d *discover) issueOf(sections []services.Section) {
	st := &d.newPg
	if len(sections) > 0 && (st.issueFirst != &sections[0] || st.issueLen != len(sections)) {
		st.issue, st.issueFirst, st.issueLen = makeIssue(sections), &sections[0], len(sections)
	}
}

// anchorOf marks e as a place the contents jump to: its place in the page's
// content is kept for the jump, and it takes the keyboard focus after one
// (sections take it programmatically and by a click, but are not a stop of
// Tab).
func (d *discover) anchorOf(e ui.Element, id string) ui.Element {
	st := &d.newPg
	if b := e.Bounds(); b.H > 0 {
		// The last layout's place, whether or not it is on screen (a Draw
		// callback would only be called for what is painted).
		if st.anchors == nil {
			st.anchors = map[string]float32{}
		}
		st.anchors[id] = b.Y - st.pg.top + st.pg.laidY
	}
	if st.focus == id {
		e.Focusable().FocusRing(false)
		switch {
		case st.focusNow:
			e.Focus()
			st.focusNow = false
		case !e.Focused():
			st.focus = ""
		}
	}
	return e
}

// jump scrolls the anchor to the top of the page (the 24 DIPs of room its
// scroll margin leaves) and moves the focus to it.
func (d *discover) jump(id string) {
	st := &d.newPg
	if y, ok := st.anchors[id]; ok {
		st.pg.pos.Y = max(0, y-24)
	}
	st.focus, st.focusNow = id, true
}

// issueView is the page's content once it has sections.
func (d *discover) issueView(c *ui.Context) {
	st := &d.newPg
	is := &st.issue
	ctx := d.env.ItemsCtx(st.menus)
	ctx.NoPopup = true // the page draws the menus' popup once, after the shelves
	if len(is.hero) > 0 {
		d.frontPage(c, is.hero)
	}
	if len(is.bands) > 1 {
		d.contents(c, is.bands)
	}
	ui.Column(c.Key("bands")).FillWidth().Children(func() {
		for bi, b := range is.bands {
			sec := ui.Column(c.Key("band:" + string(b.id))).FillWidth().Role(ui.RoleGroup).Label(b.title)
			if bi > 0 {
				sec.Margin(56, 0, 0, 0)
			}
			d.anchorOf(sec, b.anchor)
			sec.Children(func() {
				ui.Box(c.Key("rule")).FillWidth().Height(1).Background(theme.C("line")).Margin(0, 0, 24, 0)
				text(c, b.title, "page").TextColor(theme.C("ink")).Margin(0, 0, 20, 0).Role(ui.RoleHeading).Level(2)
				for i, s := range b.sections {
					w := ui.Column(c.Key(strconv.Itoa(i) + ":" + s.ID)).FillWidth()
					// An untitled link list continues the one above it.
					if i > 0 && (s.Kind != services.SectionLinks || s.Title != "") {
						w.Margin(40, 0, 0, 0)
					}
					d.anchorOf(w, b.anchor+"-"+strconv.Itoa(i))
					w.Children(func() {
						kit.EditorialSections(c, &st.ed, []services.Section{s}, ctx, kit.EditorialOpts{First: true})
					})
				}
			})
		}
	})
	st.menus.Popup(c)
}

// contents is the page's contents: one line per band, its name and then its
// shelves, each a jump to that place. It saves scrolling (and tabbing)
// through a page of twenty-odd shelves to reach one near the end.
func (d *discover) contents(c *ui.Context, bands []issueBand) {
	tk := theme.Current()
	label := i18n.T("On this page")
	ui.Column(c.Key("contents")).FillWidth().Margin(0, 0, 48, 0).Role(ui.RoleGroup).Label(label).Children(func() {
		text(c, label, "heading").TextColor(theme.C("ink")).Margin(0, 0, 8, 0).Role(ui.RoleHeading).Level(2)
		for _, b := range bands {
			ui.Row(c.Key(b.anchor)).FillWidth().Gap(24).AlignItems(ui.Start).Padding(4, 0).Children(func() {
				ui.Row(c.Key("band")).Width(144).Shrink(0).Children(func() {
					d.jumpButton(c, b.anchor, b.title, "label", theme.C("ink"))
				})
				ui.Row(c.Key("shelves")).Grow(1).Basis(0).MinWidth(0).Wrap().Gap(8).Role(ui.RoleList).Label(b.title).Children(func() {
					var titled []int
					for i, s := range b.sections {
						if s.Title != "" {
							titled = append(titled, i)
						}
					}
					for n, i := range titled {
						ui.Row(c.Key(i)).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
							d.jumpButton(c, b.anchor+"-"+strconv.Itoa(i), b.sections[i].Title, "secondary", theme.C("ink-muted"))
							if n < len(titled)-1 {
								text(c, "·", "secondary").TextColor(theme.C("ink-faint")).Role(ui.RoleNone)
							}
						})
					}
				})
			})
		}
	})
	_ = tk
}

// jumpButton is a line of the contents: a button that jumps to an anchor,
// underlined while hovered.
func (d *discover) jumpButton(c *ui.Context, id, title, style string, col ui.Color) ui.Element {
	b := ui.ButtonBase(c.Key(id)).Label(title).Radius(theme.Current().Radius["sm"]).AlignSelf(ui.Start)
	hover := b.Hovered()
	b.Children(func() {
		t := text(c, title, style).TextColor(col).SingleLine()
		if hover {
			t.Underline().TextColor(theme.C("ink"))
		}
	})
	if b.Clicked() {
		d.jump(id)
	}
	return b
}

// pageHeader is the page's title and the line about its data under it. While
// the data loads, holding is a placeholder bar that keeps the line's place,
// so nothing moves when it lands.
func (d *discover) pageHeader(c *ui.Context, title, subline string, holding bool) {
	if !holding {
		kit.PageHeader(c, title, subline, nil)
		return
	}
	tk := theme.Current()
	ui.Row(c.Key("header")).FillWidth().AlignItems(ui.End).Margin(0, 0, 24, 0).Shrink(0).Children(func() {
		ui.Column(c).MinWidth(0).Shrink(1).Children(func() {
			text(c, title, "title").TextColor(theme.C("ink")).SingleLine().Role(ui.RoleHeading).Level(1)
			ui.Row(c).Height(tk.Type["secondary"].LineHeight).Margin(4, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				bar(c, 112, 12)
			})
		})
	})
}

// newSkeleton is the page while it loads: the front page, the contents, the
// heading of the first band and the shelves, as flat bars at their real
// sizes.
func (d *discover) newSkeleton(c *ui.Context) {
	st := &d.newPg
	tk := theme.Current()
	d.frontSkeleton(c, &st.frontSkel)
	ui.Column(c.Key("contents-skeleton")).FillWidth().Margin(0, 0, 48, 0).Role(ui.RoleNone).Children(func() {
		ui.Row(c).Height(22).Margin(0, 0, 8, 0).AlignItems(ui.Center).Children(func() { bar(c, 112, 16) })
		for i, w := range []float32{0.7, 0.45, 0.3, 0.3, 0.25} {
			ui.Row(c.Key(i)).FillWidth().Height(28).Gap(24).AlignItems(ui.Center).Children(func() {
				ui.Row(c).Width(120).Shrink(0).Children(func() { bar(c, 96, 14) })
				ui.Box(c).WidthPercent(w * 100).Height(12).Radius(tk.Radius["sm"]).Background(theme.C("hover"))
			})
		}
	})
	ui.Column(c.Key("band-skeleton")).FillWidth().Margin(0, 0, 20, 0).Role(ui.RoleNone).Children(func() {
		ui.Box(c).FillWidth().Height(1).Background(theme.C("line")).Margin(0, 0, 24, 0)
		ui.Row(c).Height(30).AlignItems(ui.Center).Children(func() { bar(c, 176, 20) })
	})
	d.shelvesSkeleton(c, &st.shelvesSkel, false)
}
