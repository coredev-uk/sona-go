package artist

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Geometry of the release list (v0.2.0's ReleaseList), in DIPs: a row is the
// cover plus 6 above and below.
const (
	releaseCover = 40
	releaseRowH  = releaseCover + 12
	releasePadX  = 8
	releaseGap   = 12
	// releaseMore is the size of a row's More button.
	releaseMore = 24
	// releaseColGap is the gap between the list's columns.
	releaseColGap = 24
)

// releaseList is the state of one ReleaseList: its selection, keyboard
// navigation and item menu.
type releaseList struct {
	sel   kit.Selection
	nav   kit.GridNav
	menus *kit.ActionMenus
}

// releaseOpts are the options of a release list.
type releaseOpts struct {
	// label names the list for assistive technology.
	label string
	// columns flows the list down the first column, then the next.
	columns int
	// byArtist: the line says whose release it is, then the year, rather
	// than the year and whether it was a single or an EP.
	byArtist bool
	// lines, when set, are the lines under the titles, one per album (the
	// discography's timeline says the kind: its year is the gutter's). The
	// year still rides in a row's description.
	lines []string
}

func albumPath(id string) string { return "/album/" + url.PathEscape(id) }

// releaseList is ReleaseList: releases as a compact list, the label's back
// catalogue: 40 DIP covers, the title, then the year and whether it was a
// single or an EP (or whose it is). Rows select, open their menu on
// right-click or Shift+F10 and drag like cover tiles; Up and Down move
// between them. A nil album is a row still loading. Covers line up with the
// heading above; the hover fill reaches 8 DIPs past them.
func (a *artist) releaseList(c *ui.Context, st *releaseList, albums []*services.Album, o releaseOpts) {
	if st.menus == nil {
		st.menus = kit.NewActionMenus(a.env.Actions, &st.sel)
	}
	o.columns = max(1, o.columns)
	keys := kit.KeysOf(len(albums), func(i int) string {
		if albums[i] == nil {
			return ""
		}
		return albums[i].ID
	})
	st.sel.Hold = func() bool { return st.menus.IsOpen() || kit.ConfirmPending() }
	st.sel.Rows(keys, func(i int) (kit.Item, bool) {
		if albums[i] == nil {
			return kit.Item{}, false
		}
		return kit.AlbumItem(*albums[i]), true
	})
	st.nav.Count, st.nav.Columns = len(albums), 1
	restricted, now := a.restricted(), a.now()
	perColumn := (len(albums) + o.columns - 1) / o.columns

	root := ui.Row(c.Key("releases:"+o.label)).AlignSelf(ui.Stretch).Margin(0, -releasePadX, 0, -releasePadX).
		Gap(releaseColGap).AlignItems(ui.Start).Role(ui.RoleTable).Label(o.label)
	root.Children(func() {
		for col := range o.columns {
			ui.Column(c.Key(col)).Grow(1).Basis(0).MinWidth(0).Children(func() {
				for i := col * perColumn; i < min(len(albums), (col+1)*perColumn); i++ {
					a.releaseRow(c, st, i, albums[i], o, restricted, now)
				}
			})
		}
		st.nav.Tick(c)
		st.sel.OutsidePress(root)
		st.menus.Popup(c)
	})
}

// releaseDescription is what the lines under a release's title say, read as
// its description: the year and kind, and the explicit mark.
func releaseDescription(line string, explicit bool) string {
	if explicit {
		if line == "" {
			return i18n.T("Explicit")
		}
		return line + " · " + i18n.T("Explicit")
	}
	return line
}

// releaseLine is the line under a release's title.
func releaseLine(al services.Album, kind string, byArtist bool, now time.Time) string {
	when := releaseBadge(al, now)
	if when == "" {
		when = i18n.YearOf(al.ReleaseDate)
	}
	var parts []string
	if byArtist {
		parts = []string{al.Artist, when}
	} else {
		k := ""
		switch kind {
		case "single":
			k = i18n.T("Single")
		case "ep":
			k = i18n.T("EP")
		}
		parts = []string{when, k}
	}
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

// releaseRow builds one release at index i of the list.
func (a *artist) releaseRow(c *ui.Context, st *releaseList, i int, al *services.Album, o releaseOpts, restricted bool, now time.Time) {
	tk := theme.Current()
	hover := theme.C("hover")
	if al == nil {
		ui.Row(c.Key("loading-" + strconv.Itoa(i))).Height(releaseRowH).Shrink(0).PaddingX(releasePadX).Gap(releaseGap).
			AlignItems(ui.Center).Role(ui.RoleNone).Children(func() {
			ui.Box(c).Size(releaseCover, releaseCover).Shrink(0).Radius(tk.Radius["sm"]).Background(hover)
			ui.Column(c).Grow(1).Gap(6).Children(func() {
				ui.Box(c).WidthPercent(60).Height(14).Radius(tk.Radius["sm"]).Background(hover)
				ui.Box(c).WidthPercent(25).Height(12).Radius(tk.Radius["sm"]).Background(hover)
			})
		})
		return
	}
	title, kind := releaseTitle(al.Name)
	line := releaseLine(*al, kind, o.byArtist, now)
	spoken := line
	if o.lines != nil {
		line = o.lines[i]
		spoken = strings.Join(slices.DeleteFunc([]string{i18n.YearOf(al.ReleaseDate), line}, func(s string) bool { return s == "" }), " · ")
	}
	withheld := restricted && al.Explicit
	selected := st.sel.Selected(al.ID)
	path := albumPath(al.ID)
	opens := a.has(path)
	target := kit.ActionTarget{Item: kit.AlbumItem(*al), Name: al.Name, Key: al.ID}

	// A release is a row of one cell (v0.2.0's grid): the cell is the opener
	// and carries the selection, the more button beside it.
	var src ui.Element
	ui.Column(c.Key("row:" + al.ID)).Shrink(0).Role(ui.RoleRow).Children(func() {
		_, src = kit.DragItem(c, kit.DragOpts{
			Key:    al.ID,
			Scope:  st,
			Index:  i,
			Art:    a.env.Art,
			Row:    true,
			Button: true,
			Payload: func() *kit.DragPayload {
				if withheld {
					return nil
				}
				items, _, group := st.sel.GroupWith(al.ID)
				label := title
				if group {
					label = kit.DescribeItems(items)
				} else {
					items = []kit.Item{kit.AlbumItem(*al)}
				}
				return &kit.DragPayload{Items: items, Label: label, Art: al.Art.Medium}
			},
			Outer: func(w ui.Element) {
				w.Height(releaseRowH).Shrink(0).Role(ui.RoleCell).Checked(selected)
				if withheld {
					w.Opacity(0.5).Tooltip(i18n.T("Explicit content is restricted on this account"))
				}
				bg := hover.Alpha(0)
				switch {
				case selected:
					bg = theme.C("ink").Alpha(0.12)
				case w.Hovered():
					bg = hover
				}
				radius := tk.Radius["md"]
				w.Draw(func(p *ui.Painter, r ui.Rect) {
					if bg.A > 0 {
						p.Fill(r, bg, radius)
					}
				})
			},
			Source: func(s ui.Element) {
				// The more button is beside the source (After), not in it: its
				// room is the padding at the end.
				s.Padding(0, releasePadX+releaseMore+releaseGap, 0, releasePadX).Gap(releaseGap).Radius(tk.Radius["md"]).Label(title)
				// What the lines under the title say rides in the
				// description: the source is one piece for assistive
				// technology.
				if desc := releaseDescription(spoken, al.Explicit); desc != "" {
					s.Description(desc)
				}
				if opens {
					s.Role(ui.RoleLink)
				} else {
					s.Role(ui.RoleButton)
				}
			},
			After: func(w, s ui.Element) {
				hot := w.Hovered() || w.FocusWithin()
				st.menus.Trigger(c.Key("more"), target, kit.TriggerOpts{Visible: hot}).
					Absolute().Right(releasePadX).Top((releaseRowH - releaseMore) / 2)
			},
		}, func(w, s ui.Element) {
			kit.Artwork(c, a.env.Art, al.Art.Medium, releaseCover, kit.ArtOpts{Edge: true})
			ui.Column(c.Key("text")).Grow(1).MinWidth(0).Justify(ui.Center).Children(func() {
				ui.Row(c).MinWidth(0).Gap(6).AlignItems(ui.Center).Children(func() {
					t := ui.Text(c, title).FontSize(tk.Type["ui"].Size).FixedLineHeight(tk.Type["ui"].LineHeight).FontWeight(500).
						TextColor(theme.C("ink")).SingleLine().Shrink(1).MinWidth(0)
					if w.Hovered() {
						t.Underline()
					}
					if al.Explicit {
						kit.ExplicitMark(c.Key("explicit"))
					}
				})
				if line != "" {
					ui.Text(c, line).FontSize(tk.Type["sub"].Size).FixedLineHeight(tk.Type["sub"].LineHeight).
						TextColor(theme.C("ink-muted")).FontFeatures("tnum").SingleLine()
				}
			})
		})
	})

	st.nav.Claim(src, i)
	if src.Clicked() {
		if mods := kit.ModsOf(src.ClickModifiers()); mods.Range || mods.Toggle {
			st.sel.Click(i, mods)
		} else if opens {
			a.env.Nav.Push(path)
		}
	}
	if !st.nav.Keys(src, i, &st.sel) {
		st.sel.Keys(src, i)
	}
	// The item's menu: right-click, Shift+F10 or the Menu key; an item
	// outside the selection is selected alone first.
	if src.RightClicked() {
		st.sel.MenuOn(i)
		x, y, _ := src.PointerPosition()
		wx, wy := src.LocalToWindow(x, y)
		st.menus.OpenAt(target, wx, wy, false)
	}
	if src.Shortcut(ui.Shift, ui.KeyF10) || src.Shortcut(0, ui.KeyContextMenu) {
		st.sel.MenuOn(i)
		b := src.Bounds()
		st.menus.OpenAt(target, b.X+16, b.Y+b.H, true)
	}
}
