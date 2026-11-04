package main

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

func init() {
	register(section{name: "Overlays / Dialog and Confirm", build: overlaysDialogs})
	register(section{name: "Overlays / Menu and Popover", build: overlaysMenus})
	register(section{name: "Overlays / Actions", build: overlaysActions})
	register(section{name: "Overlays / Toasts", build: overlaysToasts})
	register(section{name: "Overlays / Command palette", build: overlaysPalette})
}

var overlaysState = struct {
	dialog, popover bool
	confirmed       string
	menu            kit.Menu
	ctx             kit.Menu
	checked         bool
	radio           int
	palette         kit.Palette
	ran             string
	am              *kit.ActionMenus
	reg             *kit.ActionRegistry
	sel             *gallerySelection
}{}

// gallerySelection treats the first and last rows as a multi-selection, so
// the menu of either shows the bulk actions.
type gallerySelection struct{ rows []kit.Item }

func (s *gallerySelection) GroupWith(key string) ([]kit.Item, []int, bool) {
	if len(s.rows) < 2 || (key != s.rows[0].ID() && key != s.rows[len(s.rows)-1].ID()) {
		return nil, nil, false
	}
	return []kit.Item{s.rows[0], s.rows[len(s.rows)-1]}, []int{0, len(s.rows) - 1}, true
}

func overlaysDialogs(c *ui.Context, _ *gallery) {
	s := &overlaysState
	kit.OverlayLayer(c)
	note(c, "kit.Dialog: scrim black at 50 % (70 % in dark), centred panel, X Close, Escape and scrim click close, focus returns to the opener.")
	panel(c, func() {
		ui.Row(c).Gap(12).Children(func() {
			if kit.Button(c.Key("open-dialog"), "Rename playlist…", kit.ButtonOpts{Variant: kit.Outline}).Clicked() {
				s.dialog = true
			}
			if kit.Button(c.Key("open-confirm"), "Delete from library…", kit.ButtonOpts{Variant: kit.Destructive}).Clicked() {
				kit.Confirm(kit.ConfirmOptions{
					Title: "Delete “OK Computer” from your library?", Body: "It also leaves any playlists it's in.",
					ConfirmLabel: "Delete", Destructive: true,
				}, func(ok bool) {
					s.confirmed = map[bool]string{true: "confirmed", false: "cancelled"}[ok]
				})
			}
			if s.confirmed != "" {
				note(c, s.confirmed)
			}
		})
	})
	kit.Dialog(c, &s.dialog, kit.DialogOpts{Title: "Rename playlist", Description: "Pick a name that says what is in it."}, func() {
		name := "Evening"
		ui.TextInput(c, &name).Label("Name")
		kit.DialogFooter(c, &s.dialog, true, func() {
			if kit.Button(c.Key("save"), "Save", kit.ButtonOpts{Size: kit.SizeSM}).Clicked() {
				s.dialog = false
			}
		})
	})
}

func overlaysMenus(c *ui.Context, _ *gallery) {
	s := &overlaysState
	note(c, "kit.Menu: arrows, Home/End and type-ahead move; Right opens a sub-menu; Escape closes and the focus goes back to the trigger.")
	items := []kit.MenuItem{
		{Key: "label", Kind: kit.MenuLabel, Label: "View"},
		{Key: "queue", Kind: kit.MenuCheckbox, Label: "Show queue", Checked: s.checked, Hint: "Ctrl+Q", Run: func() { s.checked = !s.checked }},
		{Key: "sep", Kind: kit.MenuSeparator},
		{Key: "r0", Kind: kit.MenuRadio, Label: "Compact", Checked: s.radio == 0, Run: func() { s.radio = 0 }},
		{Key: "r1", Kind: kit.MenuRadio, Label: "Roomy", Checked: s.radio == 1, Run: func() { s.radio = 1 }},
		{Key: "sub", Kind: kit.MenuSub, Label: "Sort by", Sub: []kit.MenuItem{{Key: "t", Label: "Title"}, {Key: "a", Label: "Artist"}}},
		{Key: "off", Label: "Disabled", Disabled: true},
		{Key: "del", Label: "Delete", Icon: "trash", Destructive: true},
	}
	panel(c, func() {
		ui.Row(c).Gap(12).Children(func() {
			more := kit.Button(c.Key("menu-trigger"), "Menu", kit.ButtonOpts{Variant: kit.Outline, Icon: "ellipsis"})
			if more.Clicked() {
				if s.menu.IsOpen() {
					s.menu.Close()
				} else {
					s.menu.OpenFrom(more.FocusVisible())
				}
			}
			s.menu.Show(c, kit.MenuOpts{ID: "gallery-menu", Label: "Menu", Width: 224}, more, items)

			area := ui.Box(c.Key("ctx")).Size(240, 64).Radius(8).Border(1, theme.C("line")).Center().Focusable().Label("Right-click here")
			if area.RightClicked() {
				x, y, _ := area.PointerPosition()
				wx, wy := area.LocalToWindow(x, y)
				s.ctx.OpenAt(wx, wy, false)
			}
			area.Children(func() { text(c, "Right-click here", "secondary").TextColor(theme.C("ink-muted")) })
			s.ctx.Show(c, kit.MenuOpts{ID: "gallery-ctx", Label: "Context menu", Width: 224}, area, items)

			pop := kit.Button(c.Key("pop"), "Popover", kit.ButtonOpts{Variant: kit.Outline})
			if pop.Clicked() {
				s.popover = !s.popover
			}
			kit.Popover(c, pop, &s.popover, kit.PopoverOpts{}, func() {
				text(c, "Popover", "heading").TextColor(theme.C("ink"))
				note(c, "Opens below its anchor; a press outside or Escape closes it.")
			})
		})
	})
}

func overlaysActions(c *ui.Context, _ *gallery) {
	s := &overlaysState
	if s.reg == nil {
		rating := kit.MenuData{ID: "rating", Load: func(context.Context, []kit.Item) any { return 1 }}
		s.reg = kit.NewActionRegistry(
			kit.ActionDef{
				ID: "player.play-next", Target: kit.KindSong, Group: kit.GroupQueue, Order: 10, Icon: "list-start",
				Label: func(*kit.ActionContext) string { return "Play next" }, Run: func(a *kit.ActionContext) { s.ran = "Play next: " + a.Item().Name() },
			},
			kit.ActionDef{
				ID: "player.add-to-queue", Target: kit.KindSong, Group: kit.GroupQueue, Order: 20, Icon: "list-end",
				Label:          func(*kit.ActionContext) string { return "Play last" },
				DisabledReason: func(*kit.ActionContext) string { return "Not available while listening to a live station" },
				Run:            func(*kit.ActionContext) {},
			},
			kit.ActionDef{
				ID: "library.favorite.song", Target: kit.KindSong, Group: kit.GroupRating, Order: 10, Icon: "star", Data: []kit.MenuData{rating},
				Label: func(a *kit.ActionContext) string {
					if v, _ := kit.DataOf[int](a, rating); v == 1 {
						return "Undo favourite"
					}
					return "Favourite"
				},
				Filled: func(a *kit.ActionContext) bool { v, _ := kit.DataOf[int](a, rating); return v == 1 },
				Run:    func(*kit.ActionContext) {},
			},
		)
		s.reg.SetPoster(func(fn func()) { fn() }) // the gallery has no window to post to: the loads are instant
		s.sel = &gallerySelection{}
		for _, name := range []string{"Airbag", "Paranoid Android", "Karma Police"} {
			s.sel.rows = append(s.sel.rows, kit.SongItem(services.Song{ID: name, Name: name}))
		}
		s.reg.Register(kit.ActionDef{
			ID: "player.play-next.many", Target: kit.KindSelection, Kinds: []kit.ItemKind{kit.KindSong},
			Group: kit.GroupQueue, Order: 10, Icon: "list-start",
			Label: func(*kit.ActionContext) string { return "Play next" },
			Run:   func(a *kit.ActionContext) { s.ran = "Play next: " + kit.DescribeItems(a.Items) },
		})
		s.am = kit.NewActionMenus(s.reg, s.sel)
	}
	note(c, "kit.ActionMenus: the row's More button (hover), right-click and Shift+F10 open the registry's actions for the item, grouped, with menu data loaded when it opens.")
	panel(c, func() {
		for _, name := range []string{"Airbag", "Paranoid Android", "Karma Police"} {
			it := kit.SongItem(services.Song{ID: name, Name: name})
			t := kit.ActionTarget{Item: it, Name: name, Key: name}
			row := ui.Row(c.Key("row:"+name)).Focusable().Height(40).Padding(0, 8).AlignItems(ui.Center).Label(name).Radius(6)
			if row.Hovered() {
				row.Background(theme.C("hover"))
			}
			s.am.Row(row, t)
			row.Children(func() {
				text(c, name, "body").TextColor(theme.C("ink")).Grow(1)
				s.am.Trigger(c, t, kit.TriggerOpts{Visible: row.Hovered() || row.FocusWithin()})
			})
		}
		s.am.Popup(c)
		if s.ran != "" {
			note(c, s.ran)
		}
	})
}

func overlaysToasts(c *ui.Context, _ *gallery) {
	kit.OverlayLayer(c)
	note(c, "kit.Toaster: bottom right, 88 above the bottom, popover colours; titles and descriptions are announced politely.")
	panel(c, func() {
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if kit.Button(c.Key("t-ok"), "Success", kit.ButtonOpts{Variant: kit.Outline}).Clicked() {
				kit.ToastSuccessf("Added “Airbag” to your library", "")
			}
			if kit.Button(c.Key("t-err"), "Error", kit.ButtonOpts{Variant: kit.Outline}).Clicked() {
				kit.ToastErrorf("Couldn't favourite “Karma Police”", "Check your connection and try again.")
			}
			if kit.Button(c.Key("t-load"), "Loading", kit.ButtonOpts{Variant: kit.Outline}).Clicked() {
				kit.ShowToast(kit.ToastSpec{ID: "sync", Title: "Syncing your library", Kind: kit.ToastLoading, Duration: -1})
			}
			if kit.Button(c.Key("t-unavail"), "Unavailable (one at a time)", kit.ButtonOpts{Variant: kit.Outline}).Clicked() {
				kit.Unavailable("Not available while listening to a live station")
			}
			if kit.Button(c.Key("t-close"), "Close all", kit.ButtonOpts{Variant: kit.Ghost}).Clicked() {
				kit.CloseToast("")
			}
		})
	})
}

func overlaysPalette(c *ui.Context, _ *gallery) {
	s := &overlaysState
	note(c, "kit.Palette: the base of the command palette; the commands feature supplies the sections.")
	panel(c, func() {
		if kit.Button(c.Key("pal"), "Open the palette", kit.ButtonOpts{Variant: kit.Outline}).Clicked() {
			s.palette.Open()
		}
		if s.ran != "" {
			note(c, s.ran)
		}
	})
	s.palette.Show(c, func(q string) []kit.PaletteSection {
		cmds := []kit.PaletteEntry{
			{Key: "queue", Label: "Show or hide the queue", Icon: "list-music", Keys: []string{"Ctrl+Q"}, Run: func() { s.ran = "queue" }},
			{Key: "next", Label: "Next track", Icon: "fast-forward", Keys: []string{"Ctrl+ArrowRight"}, Run: func() { s.ran = "next" }},
			{Key: "albums", Label: "Library · Albums", Icon: "disc-3", Run: func() { s.ran = "albums" }},
		}
		if q == "" {
			return []kit.PaletteSection{{Title: "Playback", Entries: cmds}}
		}
		return []kit.PaletteSection{{Title: "Commands", Entries: kit.RankCommands(q, cmds,
			func(e kit.PaletteEntry) string { return e.Label }, nil)}}
	})
}
