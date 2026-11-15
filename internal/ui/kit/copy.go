package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// CopyOpts are the words of a CopyButton.
type CopyOpts struct {
	// Label names the button for assistive technology and is its tooltip
	// ("Copy ISRC").
	Label string
	// Done is the toast that says it is on the clipboard ("ISRC copied").
	Done string
}

// CopyButton is a small ghost icon button that puts value on the system
// clipboard (Env.Copy) and says so in a toast, or says the clipboard
// wouldn't take it. It serves what selecting text cannot: an identifier for
// someone at the keyboard, one Tab stop away from its value. It reports
// nothing: the click is handled here.
func CopyButton(c *ui.Context, copy func(string) bool, value string, o CopyOpts) ui.Element {
	b := IconButton(c, "copy", o.Label, ButtonOpts{Variant: Ghost, Size: SizeXS, Muted: true})
	if b.Clicked() {
		if copy != nil && copy(value) {
			ShowToast(ToastSpec{Title: o.Done})
		} else {
			ToastErrorf(i18n.ErrorText(services.CodeClipboard), "")
		}
	}
	return b
}
