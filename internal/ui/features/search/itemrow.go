package search

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// rowOpts shape one ItemRow.
type rowOpts struct {
	// key is unique within the page.
	key string
	// size is the artwork's edge in DIPs (default 40).
	size float32
	// onOpen runs when the row is opened or played (recent searches).
	onOpen func(services.Item)
	// menus is the row's more-menu; nil: none.
	menus *kit.ActionMenus
	// focus takes the keyboard focus (once) for the field's ArrowDown.
	focus bool
}

// itemRun is what opening a result does and the role that says so: nil for
// one that is not interactive (a page that is not ported). A web-only item
// opens the system browser.
func (s *search) itemRun(it services.Item) (run func(), role ui.Role) {
	path := itemPath(it)
	role = ui.RoleLink
	switch {
	case path != "" && s.env.Routed(path):
		run = func() { s.env.Nav.Push(path) }
	case it.Kind == services.ItemSong && s.playable(it):
		// A song without a page here (an upload, or no song page): it plays.
		role = ui.RoleButton
		run = func() { s.playSong(it) }
	case path == "" && it.Kind != services.ItemSong && s.env.Players != nil && kit.PlaysWhenOpened(it, s.env.Players):
		role = ui.RoleButton
		run = func() { s.env.Players.PlayItem(it) }
	case s.leavesSona(it):
		ctx := s.env.ItemsCtx(nil)
		run = func() { kit.OpenWithoutPage(it, ctx) }
	}
	return run, role
}

// leavesSona reports whether opening a result opens the system browser: a
// web-only item (an interview, a concert listing, an Apple link) with a URL
// and nothing of Sona's to show it in.
func (s *search) leavesSona(it services.Item) bool {
	return itemPath(it) == "" && it.URL != "" && it.Kind != services.ItemSong && s.env.ItemsCtx(nil).OpenURL != nil &&
		(s.env.Players == nil || !kit.PlaysWhenOpened(it, s.env.Players))
}

// outCue is the arrow out a web-only result ends in, as the link list draws
// it: it says the result opens in the browser.
func outCue(c *ui.Context) {
	ui.Row(c.Key("out")).Size(16, 16).Shrink(0).Center().Role(ui.RoleImage).Label(i18n.T("Opens in your browser")).
		TextColor(theme.C("ink-muted")).Children(func() { ui.Icon(c, theme.Icon("arrow-up-right")).FontSize(16) })
}

// itemRow is v0.2.0's ItemRow: one result of any kind as a list row:
// artwork, name with the explicit mark, "Kind · subtitle". Every kind opens
// its page, a song included, and every playable kind also has a Play button of
// its own (shown on hover and focus); one that has no page and no URL (a library
// upload plays in place; a web-only item opens the browser, under an arrow out)
// or whose page is not ported is not interactive. The more-menu shows on hover, focus and while open, for the
// kinds with registered actions.
func (s *search) itemRow(c *ui.Context, it services.Item, o rowOpts) {
	size := o.size
	if size == 0 {
		size = 40
	}
	run, role := s.itemRun(it)

	tk := theme.Current()
	row := ui.Row(c.Key(o.key)).FillWidth().AlignItems(ui.Center).Radius(tk.Radius["md"]).PaddingEnd(4)
	shown := kit.HoverRow(row)
	var target kit.ActionTarget // the row's menu target; menus nil or no actions: none
	var hasTarget bool
	if o.menus != nil {
		if ai, ok := kit.ActionItemOf(it, s.env.Players); ok {
			target, hasTarget = kit.ActionTarget{Item: ai, Name: it.Name, Key: o.key}, true
		}
	}
	var main ui.Element // the open button, when the row has one: it takes the right-click
	row.Children(func() {
		body := func() {
			kit.Artwork(c, s.env.Art, it.Art.Small, size, kit.ArtOpts{Round: kit.ItemRound(it), Edge: true})
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Margin(0, 0, 0, 12).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
					theme.Text(c, it.Name, "body").TextColor(theme.C("ink")).SingleLine().Shrink(1).MinWidth(0)
					if it.Explicit {
						kit.ExplicitMark(c).Shrink(0)
					}
					if it.IsUpload() {
						kit.UploadedBadge(c.Key("uploaded"))
					}
				})
				if meta := itemMeta(it); meta != "" {
					theme.Text(c, meta, "secondary").TextColor(theme.C("ink-muted")).SingleLine()
				}
			})
		}
		if run == nil {
			ui.Row(c.Key("body")).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Center).Padding(6, 8).Children(body)
		} else {
			main = ui.ButtonBase(c.Key("open")).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Center).Padding(6, 8).
				Radius(tk.Radius["md"]).Role(role)
			if role == ui.RoleButton {
				// Named by its visible text (WCAG 2.5.3), with the action after it.
				main.Label(accessibleName(it) + ", " + i18n.T("Play"))
			}
			main.Children(func() {
				body()
				if s.leavesSona(it) {
					outCue(c)
				}
				if role == ui.RoleButton {
					glyph := kit.PlayGlyph(c, 14, theme.GlyphOpts{Color: theme.C("ink-muted")}).Shrink(0)
					if !main.Hovered() && !main.FocusVisible() {
						glyph.Opacity(0)
					}
				}
			})
			if o.focus {
				main.Focus()
			}
			if main.Clicked() {
				if o.onOpen != nil {
					o.onOpen(it)
				}
				run()
			}
		}
		// A result that opens a page and also plays has a Play control of its
		// own, shown with the row's other actions, as the best match's disc.
		if role == ui.RoleLink && run != nil && s.playable(it) {
			play := kit.Button(c.Key("play"), "", kit.ButtonOpts{
				Variant: kit.Ghost, Size: kit.SizeSM, Icon: "play", Filled: true, Muted: true,
				A11yLabel: i18n.T("Play {label}", "label", it.Name),
			})
			if !shown {
				play.Opacity(0)
			}
			if play.Clicked() {
				if o.onOpen != nil {
					o.onOpen(it)
				}
				s.playResult(it)
			}
		}
		if hasTarget {
			// The open button takes the press, so the row never sees it: it wires
			// the menu as well.
			o.menus.Trigger(c, row, target, kit.TriggerOpts{Visible: shown}, main)
		}
	})
}
