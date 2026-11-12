package player

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// playingRow gives el, a surface that shows the playing item (the bar's
// song, Now Playing's cover and title), the item's action menu, the one a
// row of the same song opens: right-click, Shift+F10 and the Menu key open
// it. Draw it with PopupFor, once a frame. Nothing plays, or no action
// registry: nothing.
func (p *player) playingRow(el ui.Element, cur *services.QueueItem, slot **kit.ActionMenus) {
	m := p.menusIn(slot)
	if t, ok := kit.PlayingTarget(cur); ok && m != nil {
		m.Row(el, t)
	}
}

// playingMenu is playingRow and its popup for a surface of its own.
func (p *player) playingMenu(c *ui.Context, el ui.Element, cur *services.QueueItem, slot **kit.ActionMenus) {
	p.playingRow(el, cur, slot)
	(*slot).PopupFor(c, cur != nil)
}

// menusIn is the menus held in slot, made on first use; nil without a
// registry.
func (p *player) menusIn(slot **kit.ActionMenus) *kit.ActionMenus {
	if *slot == nil && p.env.Actions != nil {
		m := kit.NewActionMenus(p.env.Actions, nil)
		// Now Playing covers the page: it gives way to the page a chosen
		// action opens (the bar's menu while it is open as well).
		m.Navigate = func(path string) {
			p.ui.closeOverlay()
			p.env.Actions.Navigate(path)
		}
		*slot = m
	}
	return *slot
}
