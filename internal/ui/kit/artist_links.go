package kit

import (
	"net/url"
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// ArtistLinksOpts are ArtistLinks' options.
type ArtistLinksOpts struct {
	// Open opens an artist's page by the artist's ID. Nil: the artist page
	// is not ported, and the text is not a link.
	Open func(id string)
	// Style sets the text's type (size, line height, weight, one line); it is
	// called on the text element of a link or of the chooser's button.
	Style func(e ui.Element)
	// Hover is the text's colour while the pointer is over it or the chooser
	// is open (zero: it keeps its colour and takes an underline alone).
	Hover ui.Color
	// Static: the text is not a tab stop (a track row, whose list is one stop
	// and opens the same choice from the row's menu).
	Static bool
	// Click sees a click first; true takes it (a track row's Shift or Ctrl
	// click selects the row instead of opening the artist).
	Click func(m ClickMods) bool
	// Wire adds what the surroundings need to the element that takes the
	// pointer, a link or the chooser's button (the player bar's song menu,
	// which the element takes from the song around it).
	Wire func(e ui.Element)
}

// ArtistLinks is the artist of a song, album or video as a way to their page.
// text is what Apple calls the credit ("A & B"), the one thing shown. With one
// artist it is a link to their page. With several (artists: every credited
// artist, in Apple's order) it is a button that opens a small menu of them,
// each opening their own page: never the first one silently, never a
// "Various Artists" page. Without artists, or where the artist page is not
// ported (Open nil), it is plain text. The button is a menu button
// (haspopup) opened by a click, Enter or Space, its menu moved by the
// arrows and closed by Escape, as every menu of the kit.
// colour is the text's (--ink when zero). It returns the element that takes
// the pointer.
func ArtistLinks(c *ui.Context, artists []services.Artist, text string, color ui.Color, o ArtistLinksOpts) ui.Element {
	if color == (ui.Color{}) {
		color = theme.C("ink")
	}
	var open []services.Artist
	if o.Open != nil {
		for _, a := range artists {
			if a.ID != "" {
				open = append(open, a)
			}
		}
	}
	style := func(e ui.Element) {
		e.TextColor(color)
		if o.Style != nil {
			o.Style(e)
		}
	}
	switch len(open) {
	case 0:
		t := ui.Text(c, text)
		style(t)
		return t
	case 1:
		return artistLink(c, open[0].ID, text, style, o)
	}
	return artistChooser(c, open, text, style, o)
}

// underline marks the text the pointer is over, in the hover colour when
// given.
func (o ArtistLinksOpts) underline(t ui.Element, on bool) {
	if !on {
		return
	}
	t.Underline()
	if o.Hover != (ui.Color{}) {
		t.TextColor(o.Hover)
	}
}

// artistLink is one artist's name as a link to their page.
func artistLink(c *ui.Context, id, text string, style func(ui.Element), o ArtistLinksOpts) ui.Element {
	var l ui.Element
	if o.Static {
		l = ui.Text(c.Key("artist"), text).Role(ui.RoleLink).Cursor(ui.CursorPointer)
	} else {
		// Outside a page ui.Link would hand its URL to the browser: it has
		// none, and the click opens the page.
		l = ui.Link(c.Key("artist"), text, "").Cursor(ui.CursorPointer)
	}
	style(l)
	o.underline(l, l.Hovered())
	if l.Clicked() {
		if m := ModsOf(l.ClickModifiers()); o.Click == nil || !o.Click(m) {
			o.Open(id)
		}
	}
	if o.Wire != nil {
		o.Wire(l)
	}
	return l
}

// artistChooser is the credit as a menu button listing every artist.
func artistChooser(c *ui.Context, artists []services.Artist, text string, style func(ui.Element), o ArtistLinksOpts) ui.Element {
	choose := i18n.T("Choose an artist")
	key := "artists"
	for _, a := range artists {
		key += ":" + a.ID
	}
	var btn ui.Element
	if o.Static {
		btn = ui.Row(c.Key(key)).MinWidth(0).Shrink(1).Cursor(ui.CursorPointer).Label(text)
	} else {
		btn = ui.ButtonBase(c.Key(key)).Padding(0).MinWidth(0).Shrink(1).Background(ui.Transparent).
			Radius(theme.Current().Radius["sm"]).Cursor(ui.CursorPointer).Label(text)
	}
	menu := ui.Local(btn, "menu", func() Menu { return Menu{} })
	btn.Role(ui.RoleMenuButton).HasPopup(ui.PopupMenu).Expanded(menu.IsOpen()).Description(choose)
	btn.Children(func() {
		t := ui.Text(c, text)
		style(t)
		o.underline(t, btn.Hovered() || menu.IsOpen())
	})
	if btn.Clicked() {
		switch m := ModsOf(btn.ClickModifiers()); {
		case o.Click != nil && o.Click(m):
		case menu.IsOpen():
			menu.Close()
		default:
			menu.OpenFrom(btn.FocusVisible())
		}
	}
	if o.Wire != nil {
		o.Wire(btn)
	}
	items := make([]MenuItem, len(artists))
	for i, a := range artists {
		items[i] = MenuItem{Key: strconv.Itoa(i) + ":" + a.ID, Label: a.Name, Run: func() { o.Open(a.ID) }}
	}
	menu.Show(c, MenuOpts{ID: url.QueryEscape(key), Label: choose, Start: true}, btn, items)
	return btn
}
