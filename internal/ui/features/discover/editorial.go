package discover

import (
	"math"
	"regexp"
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// editorialPage is the state of the curator, multiroom and grouping pages.
type editorialPage struct {
	pg     pageView
	menus  *kit.ActionMenus
	ed     kit.EditorialState
	skel   measure
	banner measure
}

// maxBanner is the tallest a campaign banner gets (max-h-64).
const maxBanner = 256

func (d *discover) curatorPage(c *ui.Context, r *ui.Route)   { d.editorialPage(c, r, kindCurator) }
func (d *discover) multiroomPage(c *ui.Context, r *ui.Route) { d.editorialPage(c, r, kindMultiroom) }
func (d *discover) groupingPage(c *ui.Context, r *ui.Route)  { d.editorialPage(c, r, kindGrouping) }

// editorialPage is a curator (genre, radio show, brand), multiroom (campaign)
// or grouping (hub) page: its header and its sections.
func (d *discover) editorialPage(c *ui.Context, r *ui.Route, kind string) {
	st := &d.edit
	id := r.Param("id")
	q := d.editorial(kind, id)
	key := kind + "/" + id
	if st.pg.key != key && st.pg.key != "" {
		st.ed = kit.EditorialState{} // another page's sections
	}
	st.pg.body(c, key, func(_ *kit.Pane, _ float32) {
		switch {
		case q.HasData:
			p := q.Data
			r.Title(p.Title)
			d.editorialHeader(c, p)
			if len(p.Sections) == 0 {
				d.emptyPage(c)
				return
			}
			ctx := d.env.ItemsCtx(st.menus)
			kit.EditorialSections(c, &st.ed, p.Sections, ctx, kit.EditorialOpts{First: true})
		case q.Err != nil:
			var title string
			switch kind {
			case kindCurator:
				title = i18n.T("Couldn't load this curator")
			case kindMultiroom:
				title = i18n.T("Couldn't load this collection")
			default:
				title = i18n.T("Couldn't load this page")
			}
			d.pageError(c, title, q.Err, q.Fetching, d.retry(keyPage(kind, id)))
		default:
			bar(c, 288, 36).Margin(0, 0, 24, 0)
			d.shelvesSkeleton(c, &st.skel, false)
		}
	})
}

// editorialHeader is the page's header: a curator has a square logo and gets
// the collection header (title, byline, notes, its colour behind); a campaign
// or hub has a banner or nothing: the banner, the title and its notes.
func (d *discover) editorialHeader(c *ui.Context, p services.EditorialPage) {
	st := &d.edit
	meta := freshness(p.FetchedAt, p.Stale, d.now())
	if p.Art.Large != "" {
		o := kit.CollectionHeaderOpts{
			Title: p.Title, Art: p.Art.Large, Loader: d.env.Art, Description: p.Notes,
			Wash: &kit.BackdropOpts{Color: p.Color, Tone: kit.ToneGround},
		}
		if p.Subtitle != "" {
			o.Byline = func() { ui.Text(c, p.Subtitle) }
		}
		if p.Stale {
			o.Meta = meta
		}
		kit.CollectionHeader(c, o)
		return
	}
	if p.Wide.Large != "" {
		d.banner(c, &st.banner, p.Wide.Large, p.Color)
	}
	subline := p.Subtitle
	if p.Stale {
		subline = meta
	}
	kit.PageHeader(c, p.Title, subline, nil)
	if p.Notes != "" {
		body := theme.Current().Type["ui"]
		ui.Text(c, p.Notes).FontSize(body.Size).FixedLineHeight(body.LineHeight).TextColor(theme.C("ink-muted")).
			MaxWidth(72*9).Margin(-12, 0, 32, 0)
	}
}

// banner is a campaign banner at its own shape, at most 256 tall,
// letterboxed on its own colour when that cap bites; never cropped.
func (d *discover) banner(c *ui.Context, m *measure, src, colour string) {
	tk := theme.Current()
	aspect := bannerAspect(src)
	root := ui.Row(c.Key("banner")).FillWidth().Justify(ui.Center).Margin(0, 0, 24, 0).Radius(tk.Radius["sm"]).ClipX().ClipY().Shrink(0).
		Background(theme.C("hover"))
	if col, ok := itemColour(services.Item{Color: colour}); ok {
		root.Background(col)
	}
	width := m.track(c, root)
	root.Children(func() {
		if width <= 0 {
			ui.Box(c).Height(0)
			return
		}
		// Rounded up to 40 DIPs, so resizing the window reuses cached sizes.
		w := min(width, maxBanner*aspect)
		size := float32(math.Ceil(float64(w)/40) * 40)
		if d.env.Art.Failed(src, size, size/aspect) {
			// A banner that will not load leaves nothing: no grey box.
			root.Margin(0).Background(ui.Color{})
			ui.Box(c).Height(0)
			return
		}
		ui.Box(c.Key("art")).Width(w).Shrink(0).Children(func() {
			kit.FluidArtwork(c, d.env.Art, src, size, kit.ArtOpts{Aspect: aspect})
		})
	})
}

// sizePart is the "<w>x<h>.jpg" (or "<n>.jpg", n by n) a Go-served art URL
// ends in: the shape Apple serves the artwork in (lib/art.ts).
var sizePart = regexp.MustCompile(`/(\d+)(?:x(\d+))?\.jpg(?:$|\?)`)

// bannerAspect is the width over the height of the artwork behind a Go art
// path; 1 when it names no size.
func bannerAspect(src string) float32 {
	m := sizePart.FindStringSubmatch(src)
	if m == nil {
		return 1
	}
	w, _ := strconv.Atoi(m[1])
	h := w
	if m[2] != "" {
		h, _ = strconv.Atoi(m[2])
	}
	if w <= 0 || h <= 0 {
		return 1
	}
	return float32(w) / float32(h)
}
