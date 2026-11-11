package player

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// queueRowIn is what one row of the queue shows (QueueRow's props).
type queueRowIn struct {
	item *services.QueueItem
	qi   int
	// start is the current song's queue index, userEnd where the user's
	// part ends and n the queue's length.
	start, userEnd, n int
	// pos and size are the row's place among the rows of its list (the
	// queue, or the station's part under its heading), from 1: the list
	// builds only the rows in view, so the siblings do not say it.
	pos, size        int
	current, playing bool
	// upNext is the song after the current one.
	upNext bool
	// tab is the list's Tab stop (roving focus).
	tab bool
	// dragged: the row is being dragged to a new place.
	dragged bool
	// autoPlay is an AutoPlay song: not what the user chose, so it is set back.
	autoPlay bool
	// withheld: Go skips it, as the account restricts explicit content.
	withheld bool
	// inView: on Now Playing's wash, where hover and focus fills are ink at
	// 6%, not the opaque hover token.
	inView bool
}

// rowLabel is the row's name for assistive technology.
func rowLabel(r *queueRowIn) string {
	it := r.item
	var l string
	switch {
	case it.Video && r.withheld:
		l = i18n.T("{name}, {artist}, music video, skipped: explicit", "name", it.Name, "artist", it.Artist)
	case it.Video:
		l = i18n.T("{name}, {artist}, music video", "name", it.Name, "artist", it.Artist)
	case r.withheld:
		l = i18n.T("{name}, {artist}, skipped: explicit", "name", it.Name, "artist", it.Artist)
	default:
		l = i18n.T("{name}, {artist}", "name", it.Name, "artist", it.Artist)
	}
	if r.upNext {
		return i18n.T("Up next: {label}", "label", l)
	}
	return l
}

// rowHelp is the keyboard help read after the row's name: the rows that keep
// their place (station and AutoPlay songs) leave out the move keys.
func rowHelp(inView, auto bool) string {
	switch {
	case inView && auto:
		return i18n.T("Enter plays the song, Delete removes it, Shift+F10 opens its menu, Escape closes now playing.")
	case inView:
		return i18n.T("Enter plays the song, Delete removes it, Alt+Up and Alt+Down move it, Shift+F10 opens its menu, Escape closes now playing.")
	case auto:
		return i18n.T("Enter plays the song, Delete removes it, Shift+F10 opens its menu, Escape closes the queue.")
	}
	return i18n.T("Enter plays the song, Delete removes it, Alt+Up and Alt+Down move it, Shift+F10 opens its menu, Escape closes the queue.")
}

// rowPayload is what a row drags: the song itself, so a sidebar playlist
// takes it as well as the queue (which only moves its own row).
func rowPayload(it *services.QueueItem, qi int) *kit.DragPayload {
	item := kit.PlayingItem(it)
	return &kit.DragPayload{
		Items:  []kit.Item{item},
		Origin: &kit.DragOrigin{List: queueDropID, Index: qi},
		Label:  it.Name,
		Art:    it.Art.Small,
	}
}

// queueRow is QueueRow: 52 high, the cover, the title and the artist, then
// the duration (the song's menu and Remove buttons while the pointer or the
// focus is on the row) or, for the song playing, the equaliser. Rows of the
// user's songs drag by their grip.
func (p *player) queueRow(c *ui.Context, st *queueList, r queueRowIn) {
	it, qi := r.item, r.qi
	tk := theme.Current()
	radius := tk.Radius["md"]
	movable := !r.current && !it.Auto
	lbl := rowLabel(&r)
	// bg is the row's fill, chosen once the row knows whether it is hot; the
	// wrapper paints it.
	var bg ui.Color

	_, src := kit.DragItem(c, kit.DragOpts{
		Key: it.UID, Scope: st, Index: qi, Art: p.env.Art, Row: true,
		Payload: func() *kit.DragPayload {
			if !movable {
				return nil
			}
			return rowPayload(it, qi)
		},
		Outer: func(w ui.Element) {
			w.Height(queueRowH).FillWidth().Shrink(0).Radius(radius).Role(ui.RoleNone)
			switch {
			case r.dragged:
				w.Opacity(0.6)
			case r.withheld:
				w.Opacity(0.5)
			}
			if r.withheld {
				w.Tooltip(i18n.T("Explicit content is restricted on this account; it will be skipped"))
			}
			w.Draw(func(pt *ui.Painter, rc ui.Rect) {
				if bg.A > 0 {
					pt.Fill(rc, bg, radius)
				}
			})
		},
		Source: func(s ui.Element) {
			s.PaddingX(8).Gap(12).Radius(radius).Role(ui.RoleListItem).Label(lbl).PositionInSet(r.pos, r.size)
			if !r.current {
				s.Description(rowHelp(r.inView, it.Auto))
			} else {
				// The song playing is the list's current item.
				s.Current(ui.CurrentItem).Background(ui.Transparent)
			}
			if r.tab {
				s.Focusable()
			}
		},
	}, func(w, s ui.Element) {
		hot := w.Hovered() || s.FocusWithin()
		switch {
		case r.dragged:
			bg = theme.C("active")
		case hot && r.inView:
			bg = theme.C("ink").Alpha(0.06)
		case hot:
			bg = theme.C("hover")
		default:
			bg = ui.Transparent
		}
		p.rowContent(c, &r, hot, movable)
	})

	if st.want == qi+1 {
		// Focus scrolls the row into view only when the focus moves to
		// it. A move target is the row that already holds the focus when
		// a keyboard move reorders it, which Focus leaves where it is: so
		// the scroll stays explicit.
		src.Focus().ScrollIntoView()
		st.want = 0
	}
	if src.FocusWithin() {
		if f := p.queue.focus; f == nil || f.uid != it.UID || f.qi != qi {
			p.queue.focus = &focusTarget{uid: it.UID, qi: qi}
		}
		st.focusedNow = it.UID
	}
	if src.Clicked() && !src.Focused() {
		p.focusRow(st, p.env.Play.Queue().Items, qi)
	}
	if src.DoubleClicked() {
		p.jumpTo(qi)
	}
	if !r.current && src.RightClicked() {
		x, y, _ := src.PointerPosition()
		wx, wy := src.LocalToWindow(x, y)
		p.openRowMenu(it.UID, wx, wy, false)
	}
	if src.Focused() {
		p.rowKeys(st, src, &r)
	}
}

// rowContent is the inside of a row.
func (p *player) rowContent(c *ui.Context, r *queueRowIn, hot, movable bool) {
	it := r.item
	if hot && movable && !r.dragged {
		ui.Icon(c.Key("grip"), theme.Icon("grip-vertical")).FontSize(14).Absolute().Left(0).Top((queueRowH - 14) / 2).
			TextColor(theme.C("ink-faint")).IgnoreInput(true).Cursor(ui.CursorGrab)
	}
	art := kit.Artwork(c.Key("art"), p.env.Art, it.Art.Small, 36, kit.ArtOpts{})
	if r.autoPlay && !hot {
		art.Opacity(0.7)
	}
	ink, muted, faint := theme.C("ink"), theme.C("ink-muted"), theme.C("ink-faint")
	nameColor, artistColor := ink, muted
	switch {
	case r.current:
		nameColor = theme.C("play-ink")
	case r.autoPlay:
		nameColor, artistColor = muted, faint
	}
	ui.Column(c.Key("text")).Grow(1).Basis(0).MinWidth(0).Justify(ui.Center).Children(func() {
		ui.Row(c.Key("name")).Gap(6).AlignItems(ui.Center).MinWidth(0).Children(func() {
			if it.Video {
				p.videoName(c, it, nameColor, r.current)
				ui.Icon(c.Key("video"), theme.Icon("clapperboard")).FontSize(14).TextColor(muted).Shrink(0).
					Role(ui.RoleImage).Label(i18n.T("Music video"))
				return
			}
			t := label(c.Key("name"), it.Name, "ui").TextColor(nameColor)
			if r.current {
				t.FontWeight(500)
			}
		})
		label(c.Key("artist"), it.Artist, "sub").TextColor(artistColor)
	})
	if r.current {
		kit.EqualiserBars(c.Key("eq"), r.playing, p.env.Motion).Margin(0, 6, 0, 0)
		return
	}
	if !hot {
		label(c.Key("dur"), i18n.FormatDuration(time.Duration(it.DurationMs)*time.Millisecond), "meta").TextColor(faint).FontFeatures("tnum").Shrink(0)
	}
	// More and Remove show while the row is hot. At rest they are built all
	// the same, over the duration and unseen (the pointer goes through them,
	// Tab skips them), so that a screen reader finds and presses them.
	actions := ui.Row(c.Key("actions")).Gap(12).AlignItems(ui.Center).Shrink(0)
	if !hot {
		actions.Absolute().Right(8).Top((queueRowH - 24) / 2).IgnoreInput(true).Opacity(0)
	}
	menuOpen := p.queue.menu.IsOpen() && p.queue.menuUID == it.UID
	actions.Children(func() {
		if clicked, b := p.rowButton(c, "more", "ellipsis", i18n.T("More for {name}", "name", it.Name), ui.RoleMenuButton, menuOpen); clicked {
			p.openRowMenu(it.UID, b.X, b.Y+b.H, false) // below the button
		}
		if clicked, _ := p.rowButton(c, "remove", "x", i18n.T("Remove {name} from the queue", "name", it.Name), ui.RoleButton, false); clicked {
			p.removeAt(r.qi)
		}
	})
}

// videoName is a music video's title: a link to its page when that page is
// ported (a pointer shortcut: the row is the list's one Tab stop, and Enter
// plays it), else plain text.
func (p *player) videoName(c *ui.Context, it *services.QueueItem, col ui.Color, current bool) {
	id := it.CatalogID
	if id == "" {
		id = it.ID
	}
	path := "/music-video/" + id
	t := label(c.Key("name"), it.Name, "ui").TextColor(col)
	if current {
		t.FontWeight(500)
	}
	if p.env.HasRoute == nil || !p.env.HasRoute(path) {
		return
	}
	t.Role(ui.RoleLink).Cursor(ui.CursorPointer)
	if t.Hovered() {
		t.Underline()
	}
	if t.Clicked() {
		p.env.Nav.Push(path)
	}
}

// rowArrows are the keys that move the list's focus, named as queue_logic's
// rowForKey knows them.
var rowArrows = [...]struct {
	key  ui.Key
	name string
}{{ui.KeyDown, "ArrowDown"}, {ui.KeyUp, "ArrowUp"}, {ui.KeyHome, "Home"}, {ui.KeyEnd, "End"}}

// rowKeys is the keyboard of the focused row: Enter plays, Delete removes,
// Alt+Up and Alt+Down move the user's song a step, the Menu key and Shift+F10
// open its menu, and the arrows, Home and End move the list's focus.
func (p *player) rowKeys(st *queueList, src ui.Element, r *queueRowIn) {
	it, qi := r.item, r.qi
	switch {
	case src.Shortcut(0, ui.KeyEnter):
		p.jumpTo(qi)
	case src.Shortcut(0, ui.KeyDelete), src.Shortcut(0, ui.KeyBackspace):
		p.removeAt(qi)
	case src.Shortcut(ui.Alt, ui.KeyUp):
		p.keyMove("ArrowUp", it, r)
	case src.Shortcut(ui.Alt, ui.KeyDown):
		p.keyMove("ArrowDown", it, r)
	case !r.current && (src.Shortcut(0, ui.KeyContextMenu) || src.Shortcut(ui.Shift, ui.KeyF10)):
		b := src.Bounds()
		p.openRowMenu(it.UID, b.X+min(48, b.W/2), b.Y+min(b.H, 40), true)
	default:
		for _, a := range rowArrows {
			if !src.Shortcut(0, a.key) {
				continue
			}
			if to, ok := rowForKey(a.name, qi, r.start, r.n); ok {
				p.focusRow(st, p.env.Play.Queue().Items, to)
			}
			return
		}
	}
}

// keyMove moves the user's song one step: station and AutoPlay songs keep
// their place, and the user's stay before them.
func (p *player) keyMove(key string, it *services.QueueItem, r *queueRowIn) {
	if it.Auto {
		return
	}
	to, ok := moveForKey(key, r.qi, r.userEnd)
	if !ok {
		return
	}
	from := r.qi
	p.askMove(*it, from, true, func() error { return p.svc.Move(from, to) })
}

// rowButton is a row's quiet 24 DIP icon button. It is not a tab stop (the
// list is one stop: Enter, Delete and Shift+F10 on the row do the same);
// it reports a click and where the button is. A menu button says whether
// its menu is open (expanded).
func (p *player) rowButton(c *ui.Context, key, icon, name string, role ui.Role, expanded bool) (clicked bool, at ui.Rect) {
	btn := ui.Row(c.Key(key)).Size(24, 24).Shrink(0).Center().Radius(theme.Current().Radius["md"]).Role(role).
		Label(name).Cursor(ui.CursorPointer)
	if role == ui.RoleMenuButton {
		btn.HasPopup(ui.PopupMenu).Expanded(expanded)
	}
	fg := theme.C("ink-muted")
	if btn.Hovered() {
		fg = theme.C("ink")
		btn.Background(theme.C("hover"))
	}
	btn.TextColor(fg).Children(func() { theme.Glyph(c, icon, 16, theme.GlyphOpts{Color: fg, Inherit: true}) })
	return btn.Clicked(), btn.Bounds()
}
