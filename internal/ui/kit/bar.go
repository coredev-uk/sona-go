package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Bar is a flat --hover placeholder bar w by h DIPs: the pieces of a page's
// loading skeleton (v0.2.0's pages draw them still; Skeleton pulses).
func Bar(c *ui.Context, w, h float32) ui.Element {
	return ui.Box(c).Width(w).Height(h).Radius(theme.Current().Radius["sm"]).Background(theme.C("hover")).Shrink(0).Role(ui.RoleNone)
}
