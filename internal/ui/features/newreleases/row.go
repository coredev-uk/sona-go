package newreleases

import (
	"context"
	"net/url"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Geometry of a row (DIPs): the cover plus 8 above and below, an unread dot
// in a slot of its own before it, and the room at the end for the two
// buttons (more, remove).
const (
	cover   = 48
	rowH    = cover + 16
	rowPadX = 8
	rowGap  = 12
	dotSlot = 12
	dotSize = 8
	btn     = 24
	// dateW is the column the date is right-aligned in.
	dateW = 104
)

// releaseList is the list's selection, keyboard navigation and item menu,
// across its groups.
type releaseList struct {
	sel   kit.Selection
	nav   kit.GridNav
	menus *kit.ActionMenus
}

func albumPath(id string) string { return "/album/" + url.PathEscape(id) }

// releases builds the groups: each a heading (with its count) over a table
// of rows. Rows select, open their menu on right-click or Shift+F10 and drag
// like cover tiles; Up and Down move between them; opening one marks it read.
func (n *newReleases) releases(c *ui.Context, groups []group, now time.Time) {
	st := &n.list
	kit.MenusIn(&st.menus, n.env.Actions, &st.sel)
	var flat []services.NewRelease
	for _, g := range groups {
		flat = append(flat, g.items...)
	}
	keys := kit.KeysOf(len(flat), func(i int) string { return flat[i].Album.CatalogID })
	st.sel.Hold = func() bool { return st.menus.IsOpen() || kit.ConfirmPending() }
	st.sel.Rows(keys, func(i int) (kit.Item, bool) { return kit.AlbumItem(flat[i].Album), true })
	st.nav.Count, st.nav.Columns = len(flat), 1
	restricted := n.restricted()

	root := ui.Column(c.Key("releases")).AlignSelf(ui.Stretch).Margin(0, -rowPadX, 0, -rowPadX).Gap(32).
		Role(ui.RoleGroup).Label(i18n.T("New releases"))
	root.Children(func() {
		i := 0
		for _, g := range groups {
			ui.Column(c.Key("group:" + g.id)).Children(func() {
				ui.Row(c.Key("heading")).Gap(8).AlignItems(ui.Baseline).Padding(0, rowPadX, 8).Children(func() {
					theme.Text(c, g.title, "heading").TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(2)
					theme.Text(c, i18n.FormatNumber(float64(len(g.items))), "secondary").TextColor(theme.C("ink-faint")).FontFeatures("tnum")
				})
				ui.Column(c.Key("rows")).Role(ui.RoleTable).Label(g.title).Children(func() {
					for _, r := range g.items {
						n.row(c, st, i, r, restricted, now)
						i++
					}
				})
			})
		}
		st.nav.Tick(c)
		st.sel.OutsidePress(root)
		st.menus.Popup(c)
	})
}

// restricted is whether the account may not play explicit releases.
func (n *newReleases) restricted() bool {
	r := store.UseAuth(n.env.Queries, n.env.Core)
	return r.HasData && r.Data.Restricted
}

// kindLabel is what a release is, in words.
func kindLabel(kind string) string {
	switch kind {
	case services.ReleaseEP:
		return i18n.T("EP")
	case services.ReleaseSingle:
		return i18n.T("Single")
	}
	return i18n.T("Album")
}

// dateLabel is when the release is, at the row's end: "Out Nov 20" while it
// is still to come, the day once out, the year when that is all Apple says.
func dateLabel(al services.Album, now time.Time) string {
	if b := kit.ReleaseBadge(al, now); b != "" {
		return b
	}
	if al.ReleaseAt == 0 {
		return i18n.YearOf(al.ReleaseDate)
	}
	return i18n.ShortReleaseDate(time.UnixMilli(al.ReleaseAt), now)
}

// row builds the release at index i of the list: the opener (cover, title,
// the artist and kind under it, the date) with the two buttons beside it.
func (n *newReleases) row(c *ui.Context, st *releaseList, i int, r services.NewRelease, restricted bool, now time.Time) {
	tk := theme.Current()
	hover := theme.C("hover")
	al := r.Album
	title, _ := kit.ReleaseTitle(al.Name)
	line := i18n.Meta(al.Artist, kindLabel(r.Kind))
	when := dateLabel(al, now)
	spoken := i18n.Meta(line, when)
	if !r.Read {
		spoken = i18n.Meta(i18n.T("Unread"), spoken)
	}
	withheld := restricted && al.Explicit
	selected := st.sel.Selected(al.CatalogID)
	path := albumPath(al.CatalogID)
	opens := n.env.Routed(path)
	target := kit.ActionTarget{
		Item: kit.AlbumItem(al), Name: al.Name, Key: al.CatalogID,
		Container: &kit.ActionContainer{Kind: containerKind, ID: containerKind, Index: i},
	}

	var src ui.Element
	ui.Column(c.Key("row:" + al.CatalogID)).Shrink(0).Role(ui.RoleRow).Children(func() {
		_, src = kit.DragItem(c, kit.DragOpts{
			Key:    al.CatalogID,
			Scope:  st,
			Index:  i,
			Art:    n.env.Art,
			Row:    true,
			Button: true,
			Payload: func() *kit.DragPayload {
				if withheld {
					return nil
				}
				items, _, group := st.sel.GroupWith(al.CatalogID)
				label := title
				if group {
					label = kit.DescribeItems(items)
				} else {
					items = []kit.Item{kit.AlbumItem(al)}
				}
				return &kit.DragPayload{Items: items, Label: label, Art: al.Art.Medium}
			},
			Outer: func(w ui.Element) {
				w.Height(rowH).Shrink(0).Role(ui.RoleCell).Checked(selected)
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
				w.Draw(func(p *ui.Painter, rc ui.Rect) {
					if bg.A > 0 {
						p.Fill(rc, bg, radius)
					}
				})
			},
			Source: func(s ui.Element) {
				// The buttons are beside the source (After), not in it: their
				// room is the padding at the end.
				s.Padding(0, rowPadX+2*btn+4+rowGap, 0, rowPadX).Gap(rowGap).Radius(tk.Radius["md"]).Label(title)
				s.Description(spoken)
				if opens {
					s.Role(ui.RoleLink)
				} else {
					s.Role(ui.RoleButton)
				}
			},
			After: func(w, s ui.Element) {
				hot := w.Hovered() || w.FocusWithin()
				st.menus.Trigger(c.Key("more"), target, kit.TriggerOpts{Visible: hot}).
					Absolute().Right(rowPadX).Top((rowH - btn) / 2)
				// The remove button shows under the pointer only: for the
				// keyboard it is the menu's entry and the Delete key, so that a
				// row is one Tab stop and not two.
				if w.Hovered() {
					if kit.IconButton(c.Key("remove"), "x", i18n.T("Remove {name}", "name", title),
						kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeXS, Muted: true}).
						Absolute().Right(rowPadX + btn + 4).Top((rowH - btn) / 2).Clicked() {
						id := al.CatalogID
						n.do(func(a api) error { return a.Dismiss(context.Background(), id) })
					}
				}
			},
		}, func(w, s ui.Element) {
			// The unread dot has a slot on every row, so the covers line up.
			ui.Row(c.Key("dot")).Size(dotSlot, dotSlot).Shrink(0).Center().Role(ui.RoleNone).Children(func() {
				if !r.Read {
					ui.Box(c.Key("unread")).Size(dotSize, dotSize).Radius(dotSize / 2).Background(theme.C("ink")).Role(ui.RoleNone)
				}
			})
			kit.Artwork(c, n.env.Art, al.Art.Medium, cover, kit.ArtOpts{Edge: true})
			ui.Column(c.Key("text")).Grow(1).MinWidth(0).Justify(ui.Center).Children(func() {
				ui.Row(c).MinWidth(0).Gap(6).AlignItems(ui.Center).Children(func() {
					// Unread is told by weight as well as by the dot.
					weight := 400
					if !r.Read {
						weight = 600
					}
					t := ui.Text(c, title).FontSize(tk.Type["label"].Size).FixedLineHeight(tk.Type["label"].LineHeight).FontWeight(weight).
						TextColor(theme.C("ink")).SingleLine().Shrink(1).MinWidth(0)
					if w.Hovered() {
						t.Underline()
					}
					if al.Explicit {
						kit.ExplicitMark(c.Key("explicit"))
					}
				})
				ui.Text(c, line).FontSize(tk.Type["sub"].Size).FixedLineHeight(tk.Type["sub"].LineHeight).
					TextColor(theme.C("ink-muted")).SingleLine()
			})
			if when != "" {
				ui.Text(c, when).FontSize(tk.Type["sub"].Size).FixedLineHeight(tk.Type["sub"].LineHeight).
					TextColor(theme.C("ink-muted")).FontFeatures("tnum").SingleLine().Width(dateW).TextAlign(ui.End).Shrink(0)
			}
		})
	})

	st.nav.Claim(src, i)
	if src.Clicked() {
		if mods := kit.ModsOf(src.ClickModifiers()); mods.Range || mods.Toggle {
			st.sel.Click(i, mods)
		} else if opens {
			if !r.Read {
				id := al.CatalogID
				n.do(func(a api) error { return a.MarkRead(context.Background(), id) })
			}
			n.env.Nav.Push(path)
		}
	}
	if src.Focused() && (src.Shortcut(0, ui.KeyDelete) || src.Shortcut(0, ui.KeyBackspace)) {
		id := al.CatalogID
		n.do(func(a api) error { return a.Dismiss(context.Background(), id) })
	}
	if !st.nav.Keys(src, i, &st.sel) {
		st.sel.Keys(src, i)
	}
	// The item's menu: right-click, Shift+F10 or the Menu key; an item
	// outside the selection is selected alone first.
	st.menus.OpenFor(&st.sel, src, src, target, i)
}
