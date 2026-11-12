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
			d.masthead(c, i18n.T("Discover"), freshness(q.Data.FetchedAt, q.Data.Stale, d.now()), false, nil)
			if len(q.Data.Sections) == 0 {
				d.emptyPage(c)
				return
			}
			d.issueOf(q.Data.Sections)
			d.issueView(c)
		case q.Err != nil:
			d.masthead(c, i18n.T("Discover"), "", false, nil)
			d.pageError(c, i18n.T("Couldn't load what's new"), q.Err, q.Fetching, d.retry(keyPage(kindNew, "")))
		default:
			d.masthead(c, i18n.T("Discover"), "", true, nil)
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
// (a TabStop(false), so Tab never stops at it). While it has the focus it is
// a group named label (the shelf's title), so that what the focus lands on
// is read; label "" leaves a place that is named already.
func (d *discover) anchorOf(e ui.Element, id, label string) ui.Element {
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
		e.TabStop(false).FocusRing(false)
		if label != "" {
			e.Role(ui.RoleGroup).Label(label)
		}
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
	wide := st.pg.width >= marginAt
	ui.Column(c.Key("bands")).FillWidth().Children(func() {
		for bi, b := range is.bands {
			sec := ui.Column(c.Key("band:" + string(b.id))).FillWidth().Role(ui.RoleGroup).Label(b.title)
			if bi > 0 {
				sec.Margin(56, 0, 0, 0)
			}
			d.anchorOf(sec, b.anchor, "")
			sec.Children(func() {
				ui.Box(c.Key("rule")).FillWidth().Height(1).Background(theme.C("line"))
				shelves := func() {
					for i, s := range b.sections {
						w := ui.Column(c.Key(strconv.Itoa(i) + ":" + s.ID)).FillWidth()
						// An untitled link list continues the one above it.
						if i > 0 && (s.Kind != services.SectionLinks || s.Title != "") {
							w.Margin(40, 0, 0, 0)
						}
						d.anchorOf(w, b.anchor+"-"+strconv.Itoa(i), s.Title)
						w.Children(func() {
							kit.EditorialSections(c, &st.ed, []services.Section{s}, ctx, kit.EditorialOpts{First: true})
						})
					}
				}
				if !wide {
					bandHeading(c, b.title).Margin(24, 0, 20, 0)
					shelves()
					return
				}
				// The heading stands in a margin column beside its shelves; the
				// shelves' first title is set 6 lower, so the two baselines meet.
				ui.Row(c.Key("body")).FillWidth().Margin(24, 0, 0, 0).Gap(marginGap).AlignItems(ui.Start).Children(func() {
					ui.Column(c.Key("margin")).Width(marginWidth).Shrink(0).Children(func() { bandHeading(c, b.title) })
					ui.Column(c.Key("shelves")).Grow(1).Basis(0).MinWidth(0).Margin(6, 0, 0, 0).Children(shelves)
				})
			})
		}
	})
	st.menus.Popup(c)
}

// bandHeading is a band's name, at the page-title size (the rule above it is
// what sets it apart from the page's h1).
func bandHeading(c *ui.Context, title string) ui.Element {
	return theme.Text(c, title, "title").TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(2)
}

// contents is the page's contents: side by side, wrapping when the width runs
// out, a column for each band, its button (with the number of its titled
// shelves) over a button for each of its shelves, each a jump to that place. It
// saves scrolling (and tabbing) through a page of twenty-odd shelves to reach
// one near the end.
func (d *discover) contents(c *ui.Context, bands []issueBand) {
	label := i18n.T("On this page")
	ui.Column(c.Key("contents")).FillWidth().Margin(0, 0, 48, 0).Role(ui.RoleGroup).Label(label).Children(func() {
		theme.Text(c, label, "heading").TextColor(theme.C("ink")).Margin(0, 0, 12, 0).Role(ui.RoleHeading).Level(2)
		ui.Row(c.Key("columns")).FillWidth().Wrap().Gap(24).AlignItems(ui.Start).Children(func() {
			for _, b := range bands {
				var titled []int
				for i, s := range b.sections {
					if s.Title != "" {
						titled = append(titled, i)
					}
				}
				ui.Column(c.Key(b.anchor)).Width(contentsColumn).Shrink(0).Gap(2).Children(func() {
					d.jumpButton(c, b.anchor, b.title, len(titled))
					ui.Column(c.Key("shelves")).Role(ui.RoleList).Label(b.title).Margin(4, 0, 0, 0).Children(func() {
						for _, i := range titled {
							ui.Row(c.Key(i)).MinWidth(0).Role(ui.RoleListItem).Label(b.sections[i].Title).Children(func() {
								d.shelfButton(c, b.anchor+"-"+strconv.Itoa(i), b.sections[i].Title)
							})
						}
					})
				})
			}
		})
	})
}

// shelfButton is a shelf of the contents: a button that jumps to the shelf at
// 13 muted, underlined while hovered.
func (d *discover) shelfButton(c *ui.Context, id, title string) ui.Element {
	tk := theme.Current()
	b := ui.ButtonBase(c.Key(id)).Label(title).Radius(tk.Radius["sm"]).AlignSelf(ui.Start).MinWidth(0).PaddingX(12).Height(24).AlignItems(ui.Center)
	hover := b.Hovered()
	b.Children(func() {
		t := theme.Text(c, title, "secondary").TextColor(theme.C("ink-muted")).SingleLine().MinWidth(0)
		if hover {
			t.Underline().TextColor(theme.C("ink"))
		}
	})
	if b.Clicked() {
		d.jump(id)
	}
	return b
}

// jumpButton is a band's button of the contents: its name and its number of
// shelves, jumping to the band's place.
func (d *discover) jumpButton(c *ui.Context, id, title string, shelves int) ui.Element {
	tk := theme.Current()
	b := ui.ButtonBase(c.Key(id)).Label(title).Height(tk.Metric["control"]).PaddingX(12).Gap(8).AlignItems(ui.Center).Radius(tk.Radius["md"]).AlignSelf(ui.Start)
	if shelves > 0 {
		b.Description(i18n.T("{0, plural, one {# shelf} other {# shelves}}", "0", shelves))
	}
	bg := theme.C("hover")
	if b.Hovered() {
		bg = theme.C("active")
	}
	b.Background(bg)
	theme.Fade(b, "press")
	b.Children(func() {
		theme.Text(c, title, "label").TextColor(theme.C("ink")).SingleLine()
		if shelves > 0 {
			theme.Text(c, strconv.Itoa(shelves), "secondary").TextColor(theme.C("ink-muted")).FontFeatures("tnum").SingleLine()
		}
	})
	if b.Clicked() {
		d.jump(id)
	}
	return b
}

// newSkeleton is the page while it loads: the front page, the contents, the
// heading of the first band and the shelves, as flat bars at their real
// sizes.
func (d *discover) newSkeleton(c *ui.Context) {
	st := &d.newPg
	tk := theme.Current()
	d.frontSkeleton(c, &st.frontSkel)
	ui.Column(c.Key("contents-skeleton")).FillWidth().Margin(0, 0, 48, 0).Role(ui.RoleNone).Children(func() {
		ui.Row(c).Height(22).Margin(0, 0, 12, 0).AlignItems(ui.Center).Children(func() { bar(c, 112, 16) })
		ui.Row(c).FillWidth().Gap(24).AlignItems(ui.Start).Children(func() {
			for i := range 3 {
				ui.Column(c.Key(i)).Width(contentsColumn).Shrink(0).Gap(6).Children(func() {
					ui.Box(c).Size(136, tk.Metric["control"]).Radius(tk.Radius["md"]).Background(theme.C("hover"))
					for range 2 {
						ui.Row(c).Height(24).PaddingX(12).AlignItems(ui.Center).Children(func() { bar(c, 112, 12) })
					}
				})
			}
		})
	})
	ui.Column(c.Key("band-skeleton")).FillWidth().Role(ui.RoleNone).Children(func() {
		ui.Box(c).FillWidth().Height(1).Background(theme.C("line"))
		if st.pg.width < marginAt {
			ui.Row(c).Height(30).Margin(24, 0, 20, 0).AlignItems(ui.Center).Children(func() { bar(c, 176, 20) })
			d.shelvesSkeleton(c, &st.shelvesSkel, false)
			return
		}
		ui.Row(c).FillWidth().Margin(24, 0, 0, 0).Gap(marginGap).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Width(marginWidth).Shrink(0).Children(func() {
				ui.Row(c).Height(30).AlignItems(ui.Center).Children(func() { bar(c, 176, 20) })
			})
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Margin(6, 0, 0, 0).Children(func() {
				d.shelvesSkeleton(c, &st.shelvesSkel, false)
			})
		})
	})
}
