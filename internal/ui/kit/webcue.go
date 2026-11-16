package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// OpensInBrowser is true for an item that leaves Sona when it is chosen: it
// has no Sona page, does not play when opened (a station with no page) and
// has a web page to open. Every surface that shows such an item draws the
// same cue, WebCue, and names its opener WebCueLabel.
func OpensInBrowser(it services.Item, p *ItemPlayers) bool {
	return ItemLink(it) == "" && it.URL != "" && !PlaysWhenOpened(it, p)
}

// WebCueLabel is the accessible name of the opener of an item that opens in
// the browser: its name, then what choosing it does.
func WebCueLabel(name string) string {
	return name + " " + i18n.T("Opens in your browser")
}

// WebCue is the arrow out at the end of a title row (16 DIPs), named "Opens
// in your browser": the one cue for an item that leaves Sona, on tiles, cards,
// rows and link tiles alike.
func WebCue(c *ui.Context) ui.Element {
	return ui.Row(c.Key("out")).Size(16, 16).Shrink(0).Center().Role(ui.RoleImage).Label(i18n.T("Opens in your browser")).
		TextColor(theme.C("ink-muted")).Children(func() { ui.Icon(c, theme.Icon("arrow-up-right")).FontSize(16) })
}
