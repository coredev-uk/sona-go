package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// listenBrainzTokenPage is where a ListenBrainz user token is copied from.
const listenBrainzTokenPage = "https://listenbrainz.org/settings/"

// ListenBrainzTokenHint is "Copy it from your ListenBrainz settings ↗. Sona
// checks it …" under the token field: the translation marks the link as
// <0>text<1/></0>, and it opens listenBrainzTokenPage through openURL
// (Env.OpenURL).
func ListenBrainzTokenHint(c *ui.Context, openURL func(string) error) {
	msg := i18n.T("Copy it from <0>your ListenBrainz settings<1/></0>. Sona checks it with ListenBrainz and keeps it in your keyring.")
	c = c.Key("hint")
	before, link, after, ok := i18n.SplitLink(msg)
	if !ok {
		theme.Text(c, msg, "meta").TextColor(theme.C("ink-muted"))
		return
	}
	ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Children(func() {
		theme.Text(c.Key("a"), before, "meta").TextColor(theme.C("ink-muted"))
		if Button(c.Key("link"), link, ButtonOpts{Variant: Link, Size: SizeXS, Icon: "external-link"}).Clicked() {
			_ = openURL(listenBrainzTokenPage)
		}
		theme.Text(c.Key("b"), after, "meta").TextColor(theme.C("ink-muted"))
	})
}
