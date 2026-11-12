package search

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// rowOpts shape one ItemRow.
type rowOpts struct {
	// key is unique within the page.
	key string
	// size is the artwork's edge in DIPs (default 40).
	size float32
	// onOpen runs when the row is opened or played (recent searches).
	onOpen func(services.Item)
	// menus is the row's more-menu; nil: none.
	menus *kit.ActionMenus
	// focus takes the keyboard focus (once) for the field's ArrowDown.
	focus bool
}

// itemRow is v0.2.0's ItemRow: one result of any kind as a list row:
// artwork, name with the explicit mark, "Kind · subtitle". Songs play in
// place (the play glyph shows on hover and focus); every other kind opens
// its page; one that has no page (a web-only item) or whose page is not
// ported is not interactive. The more-menu shows on hover, focus and while
// open, for the kinds with registered actions.
func (s *search) itemRow(c *ui.Context, it services.Item, o rowOpts) {
	size := o.size
	if size == 0 {
		size = 40
	}
	path := itemPath(it)
	var run func() // what opening the row does; nil: not interactive
	role := ui.RoleLink
	switch {
	case path != "" && s.env.HasRoute != nil && s.env.HasRoute(path):
		run = func() { s.env.Nav.Push(path) }
	case path == "" && it.Kind == services.ItemSong && it.Song != nil && it.Song.Playable:
		role = ui.RoleButton
		run = func() { s.playSong(it) }
	case path == "" && it.Kind != services.ItemSong && s.env.Players != nil && kit.PlaysWhenOpened(it, s.env.Players):
		role = ui.RoleButton
		run = func() { s.env.Players.PlayItem(it) }
	}

	tk := theme.Current()
	row := ui.Row(c.Key(o.key)).FillWidth().AlignItems(ui.Center).Radius(tk.Radius["md"]).PaddingEnd(4)
	shown := row.Hovered() || row.FocusWithin()
	if shown {
		row.Background(theme.C("hover"))
	}
	row.Children(func() {
		body := func() {
			kit.Artwork(c, s.env.Art, it.Art.Small, size, kit.ArtOpts{Round: kit.ItemRound(it), Edge: true})
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Margin(0, 0, 0, 12).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
					theme.Text(c, it.Name, "body").TextColor(theme.C("ink")).SingleLine().Shrink(1).MinWidth(0)
					if it.Explicit {
						kit.ExplicitMark(c).Shrink(0)
					}
				})
				if meta := itemMeta(it); meta != "" {
					theme.Text(c, meta, "secondary").TextColor(theme.C("ink-muted")).SingleLine()
				}
			})
		}
		if run == nil {
			ui.Row(c.Key("body")).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Center).Padding(6, 8).Children(body)
		} else {
			main := ui.ButtonBase(c.Key("open")).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Center).Padding(6, 8).
				Radius(tk.Radius["md"]).Role(role)
			if role == ui.RoleButton {
				// Named by its visible text (WCAG 2.5.3), with the action after it.
				main.Label(it.Name + ", " + i18n.T("Play"))
			}
			main.Children(func() {
				body()
				if role == ui.RoleButton {
					glyph := theme.Glyph(c, "play", 14, theme.GlyphOpts{Color: theme.C("ink-muted")}).Shrink(0)
					if !main.Hovered() && !main.FocusVisible() {
						glyph.Opacity(0)
					}
				}
			})
			if o.focus {
				main.Focus()
			}
			if o.menus != nil && main.RightClicked() {
				// The open button takes the press, so the row never sees it.
				if ai, ok := kit.ActionItemOf(it, s.env.Players); ok {
					x, y, _ := main.PointerPosition()
					wx, wy := main.LocalToWindow(x, y)
					o.menus.OpenAt(kit.ActionTarget{Item: ai, Name: it.Name, Key: o.key}, wx, wy, false)
				}
			}
			if main.Clicked() {
				if o.onOpen != nil {
					o.onOpen(it)
				}
				run()
			}
		}
		if o.menus != nil {
			if ai, ok := kit.ActionItemOf(it, s.env.Players); ok {
				t := kit.ActionTarget{Item: ai, Name: it.Name, Key: o.key}
				o.menus.Trigger(c, t, kit.TriggerOpts{Visible: shown})
			}
		}
	})
	if o.menus != nil {
		if ai, ok := kit.ActionItemOf(it, s.env.Players); ok {
			o.menus.Row(row, kit.ActionTarget{Item: ai, Name: it.Name, Key: o.key})
		}
	}
}
