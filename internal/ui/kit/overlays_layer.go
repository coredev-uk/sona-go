package kit

import "github.com/egoist/mygo/ui"

// OverlayLayer is what the shell mounts once for the kit's global overlays:
// the confirm dialog Confirm asks through and the Toaster the toast helpers
// show in. Everything else (menus, popovers, dialogs of a page) is built
// where it is used.
func OverlayLayer(c *ui.Context) {
	ConfirmHost(c)
	Toaster(c)
}
