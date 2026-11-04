package kit

import "github.com/egoist/mygo/ui"

// ScrollArea is a vertical scroll container for a panel of the interface: it
// scrolls with the wheel, the touchpad, its scroll bar (the theme's thin
// overlay thumb in --scrollbar, shown while the pointer is over it) and the
// keyboard (arrows, Page Up/Down, Space, Home, End), and takes the keyboard
// focus, ringed while it has it, so a region that scrolls is reachable
// without a pointer. Give it a size, or Grow it within its parent.
// (shared/ui/scroll-area.tsx)
func ScrollArea(c *ui.Context) ui.Element {
	return ui.Scroll(c).Focusable()
}
