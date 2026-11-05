package miniplayer

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// playingRow gives el, a part of the window that shows the playing item (the
// cover, the title and artist), the item's action menu, the one the player
// bar and a song row open: right-click, Shift+F10 and the Menu key. Draw it
// with playingPopup. Nothing plays, or no action registry: nothing.
func (m *mini) playingRow(el ui.Element, cur *services.QueueItem) {
	if m.menus == nil && m.env.Actions != nil {
		m.menus = kit.NewActionMenus(m.env.Actions, nil)
	}
	if t, ok := kit.PlayingTarget(cur); ok && m.menus != nil {
		m.menus.Row(el, t)
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
