package miniplayer

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// menusFor is the playing song's action menu, made on first use; nil without
// an action registry. The window is the handle that drags the frameless
// window, so the menu never takes a primary press (see playingRow). What a
// chosen action does happens in the main window (its router, dialogs and
// toasts), so the window comes back first, except for the queue actions,
// which work in place.
func (m *mini) menusFor() *kit.ActionMenus {
	if m.menus == nil && m.env.Actions != nil {
		m.menus = kit.NewActionMenus(m.env.Actions, nil)
		m.menus.Before = func(d *kit.ActionDef) {
			if d.Group != kit.GroupQueue {
				m.expand()
			}
		}
	}
	return m.menus
}

// playingRow gives el, a part of the window that shows the playing item (the
// cover, the title and artist), the item's action menu on a right-click: the
// one the player bar and a song row open. Only the secondary press is taken
// (Element.RightClicked would make el interactive, and a press on it would no
// longer drag the window). Draw it with playingPopup. Nothing playing, or no
// action registry: nothing.
func (m *mini) playingRow(el ui.Element, cur *services.QueueItem) {
	menus := m.menusFor()
	t, ok := kit.PlayingTarget(cur)
	if menus == nil || !ok {
		return
	}
	b := el.Bounds() // as last laid out: the event comes after the build, when el is gone
	el.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputPointerDown || ev.Button != 1 {
			return false
		}
		menus.OpenAt(t, b.X+ev.X, b.Y+ev.Y, false)
		return true
	})
}

// playingKeys opens the menu from the keyboard (Shift+F10 or the Menu key
// while the focus is in the window), below the cover.
func (m *mini) playingKeys(root ui.Element, cur *services.QueueItem) {
	menus := m.menusFor()
	t, ok := kit.PlayingTarget(cur)
	if menus == nil || !ok {
		return
	}
	if root.Shortcut(ui.Shift, ui.KeyF10) || root.Shortcut(0, ui.KeyContextMenu) {
		b := root.Bounds()
		menus.OpenAt(t, b.X+pad, b.Y+coverSize, true)
	}
}

// playingPopup draws the menu playingRow opened, once a frame.
func (m *mini) playingPopup(c *ui.Context, cur *services.QueueItem) {
	switch {
	case m.menus == nil:
	case cur == nil:
		m.menus.Close()
	default:
		m.menus.Popup(c)
	}
}
