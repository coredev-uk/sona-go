package inspect

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// dialogWidth is the panel's widest: wide enough for a ledger's 200 DIP label
// column beside a long URL.
const dialogWidth = 960

// panel is the dialog: a header with the item, tabs, the tab chosen and a foot.
// It is an Operate surface in the house style: the neutral board, one grotesque
// with tabular figures, structure by 1 DIP rules and indentation guides, no
// colour but the tokens (docs/design.md, "Item inspector").
func (in *inspector) panel(c *ui.Context) {
	_, wh := c.Size()
	panel := kit.Dialog(c.Key("inspect"), &in.open, kit.DialogOpts{
		Title:       i18n.T("Inspect {name}", "name", in.item.Name()),
		Description: i18n.T("What Sona mapped for this item, what Apple sent, and what an add-on receives"),
		HideHeader:  true, MaxWidth: dialogWidth, Top: max(wh*0.04, 12), Padding: 0, Gap: 0, Bare: true,
	}, func() {
		in.header(c)
		in.rule(c, "rule-head")
		in.tabBar(c)
		in.rule(c, "rule-tabs")
		in.body(c, bodyHeight(wh))
		in.rule(c, "rule-body")
		in.foot(c)
	})
	if panel.Valid() {
		panel.ObserveInput(in.observe)
	}
}

// bodyHeight is the room the tab's content has: what the window leaves after
// the header, tabs and foot, at least 260 and at most 600.
func bodyHeight(wh float32) float32 { return min(max(wh-300, 260), 600) }

// observe is the dialog's key handling: Ctrl+F asks for the filter.
func (in *inspector) observe(ev ui.InputEvent) {
	if ev.Kind == ui.InputKeyDown && ev.Mods == ui.Ctrl && ev.Key == ui.KeyF {
		in.wantFilter = true
	}
}

func (in *inspector) rule(c *ui.Context, key string) {
	ui.Box(c.Key(key)).Height(1).FillWidth().Shrink(0).Background(theme.C("line"))
}

// header is the item: its artwork at 56 DIPs, its name at the dialog title
// size, its kind and subtitle, the IDs as chips, and the link to Apple Music.
func (in *inspector) header(c *ui.Context) {
	ui.Row(c.Key("head")).FillWidth().Shrink(0).Padding(20, 56, 16, 24).Gap(16).AlignItems(ui.Start).Children(func() {
		kit.Artwork(c.Key("art"), in.env.Art, artSource(in.item), 56, kit.ArtOpts{Edge: true, Round: in.item.Kind == kit.KindArtist})
		ui.Column(c.Key("who")).Grow(1).MinWidth(0).Gap(4).Children(func() {
			theme.Text(c.Key("name"), in.item.Name(), "heading").FontSize(18).FixedLineHeight(24).FontWeight(600).
				TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(2)
			theme.Text(c.Key("meta"), metaLine(in.item), "secondary").TextColor(theme.C("ink-muted"))
			if chips := in.chipsOf(); len(chips) > 0 {
				ui.Row(c.Key("chips")).Wrap().Gap(6).Margin(4, 0, 0, 0).Role(ui.RoleGroup).Label(i18n.T("IDs")).Children(func() {
					for _, ch := range chips {
						in.chip(c, ch)
					}
				})
			}
		})
		if link := in.report.URL; link != "" {
			b := kit.Button(c.Key("apple"), i18n.T("Open in Apple Music"), kit.ButtonOpts{
				Variant: kit.Outline, Size: kit.SizeSM, Icon: "external-link", IconAfter: true,
			})
			if b.Clicked() {
				in.openLink(link)
			}
		}
	})
}

// chip is an ID as a small button: a click copies the value and says so.
func (in *inspector) chip(c *ui.Context, ch chip) {
	c = c.Key("chip:" + ch.label)
	b := ui.ButtonBase(c).Height(24).PaddingX(8).Gap(6).AlignItems(ui.Center).Radius(theme.Current().Radius["md"]).
		Border(1, theme.C("control-line")).Label(i18n.T("Copy {label} {id}", "label", ch.label, "id", ch.value))
	if b.Hovered() {
		b.Background(theme.C("hover"))
	}
	b.Children(func() {
		theme.Text(c.Key("l"), ch.label, "meta").TextColor(theme.C("ink-muted")).SingleLine().Unselectable()
		theme.Text(c.Key("v"), ch.value, "meta").TextColor(theme.C("ink")).FontFeatures("tnum").SingleLine()
	})
	if b.Clicked() {
		in.copy(ch.value, i18n.T("Copied {id}", "id", ch.value))
	}
}

// tabBar is the tabs, and beside them the choice of ledger or JSON for
// Overview.
func (in *inspector) tabBar(c *ui.Context) {
	if r := in.reasons(); r[in.tab] != "" {
		in.tab = tabOverview
	}
	ui.Row(c.Key("tabbar")).FillWidth().Shrink(0).Height(48).Padding(0, 24).Gap(12).AlignItems(ui.Center).Children(func() {
		kit.Tabs(c.Key("tabs"), &in.tab, tabLabels(), kit.TabsOpts{
			Line: true, A11yLabel: i18n.T("Inspector sections"), Reasons: in.reasons(),
		})
		ui.Box(c.Key("space")).Grow(1)
		if in.tab == tabOverview && in.data != nil && in.data.mapped != nil {
			kit.Tabs(c.Key("view"), &in.view, []string{i18n.T("Ledger"), i18n.T("JSON")}, kit.TabsOpts{A11yLabel: i18n.T("Overview as")})
		}
	})
}

// body is the tab's content in a box of a fixed height, so the dialog does not
// change size as tabs and data come and go.
func (in *inspector) body(c *ui.Context, h float32) {
	label := tabLabels()[in.tab]
	ui.Column(c.Key("body")).FillWidth().Height(h).Shrink(0).Role(ui.RoleTabPanel).Label(label).Children(func() {
		switch {
		case in.err != nil:
			in.failure(c)
		case in.loading || in.data == nil:
			in.skeleton(c)
		default:
			in.tabBody(c)
		}
	})
}

func (in *inspector) tabBody(c *ui.Context) {
	d := in.data
	switch in.tab {
	case tabOverview:
		if in.view == 1 && d.mapped != nil {
			in.treeView(c, d.mapped, "", i18n.T("Mapped item"))
			return
		}
		in.ledger(c, &in.lists[0], d.overview, i18n.T("Mapped item"))
	case tabApple:
		in.treeView(c, d.apple, i18n.T("{source} · {key}", "source", i18n.T("Go's catalogue cache"), "key", in.report.RawKey), i18n.T("Apple's resource"))
	case tabLibrary:
		in.ledger(c, &in.lists[1], d.library, i18n.T("Library row"))
	case tabAddon:
		in.treeView(c, d.addon, i18n.T("Exactly what an add-on is sent for this item. Add-ons never see private listens or previews; this is your own inspection, so it shows the item regardless."), i18n.T("What an add-on gets"))
	}
}

// skeleton is the final layout as flat bars: a ledger's rows.
func (in *inspector) skeleton(c *ui.Context) {
	ui.Column(c.Key("loading")).FillWidth().Grow(1).Padding(8, 24).Role(ui.RoleStatus).Label(i18n.T("Loading")).Children(func() {
		for i := range 9 {
			k := c.Key("sk" + strconv.Itoa(i))
			ui.Row(k).FillWidth().Height(36).Gap(24).AlignItems(ui.Center).Children(func() {
				kit.Bar(k.Key("l"), 120, 12)
				kit.Bar(k.Key("v"), float32(180+(i*53)%260), 12)
			})
		}
	})
}

// failure is the error inside the tab it failed in, with Retry. The words are
// the code's, never Go's own text.
func (in *inspector) failure(c *ui.Context) {
	ui.Column(c.Key("failure")).FillWidth().Grow(1).Center().Gap(8).Padding(24).Role(ui.RoleAlert).Children(func() {
		theme.Glyph(c, "circle-alert", 16, theme.GlyphOpts{Color: theme.C("danger")})
		theme.Text(c.Key("t"), i18n.T("Couldn't read this item"), "heading").TextColor(theme.C("ink"))
		theme.Text(c.Key("d"), i18n.ErrorMessage(in.err), "secondary").TextColor(theme.C("ink-muted"))
		if kit.Button(c.Key("retry"), i18n.T("Retry"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Icon: "refresh-cw"}).Clicked() {
			in.load()
		}
	})
}

// foot is the keys of the view and the two copies: the current tab, and
// everything, as JSON.
func (in *inspector) foot(c *ui.Context) {
	ui.Row(c.Key("foot")).FillWidth().Shrink(0).Height(52).Padding(0, 24).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Row(c.Key("hints")).Grow(1).MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
			sel := in.selection()
			// Each is a button for the row chosen, with its key beside it.
			act := func(key, label, chord string, on bool, run func()) {
				k := c.Key("act:" + key)
				ui.Row(k).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
					if kit.Button(k.Key("b"), label, kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeXS, Disabled: !on}).Clicked() {
						run()
					}
					kit.Keycaps(k.Key("k"), chord)
				})
			}
			if sel.link != "" {
				k := c.Key("act:open")
				ui.Row(k).Shrink(0).Children(func() {
					if kit.Button(k.Key("b"), i18n.T("Open"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeXS, Icon: "external-link"}).Clicked() {
						in.openLink(sel.link)
					}
				})
			}
			act("value", i18n.T("Copy value"), "Enter", sel.ok, func() { in.copy(sel.value, i18n.T("Copied")) })
			act("path", i18n.T("Copy path"), "Ctrl+Shift+C", sel.ok, func() { in.copy(sel.path, i18n.T("Copied path")) })
			if in.treeShown() != nil {
				k := c.Key("hint:find")
				ui.Row(k).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
					kit.Keycaps(k.Key("k"), "Ctrl+F")
					theme.Text(k.Key("l"), i18n.T("Filter"), "meta").TextColor(theme.C("ink-muted")).SingleLine()
				})
			}
		})
		ready := in.err == nil && !in.loading && in.data != nil
		text, ok := in.tabText()
		cp := kit.Button(c.Key("copy"), i18n.T("Copy"), kit.ButtonOpts{
			Variant: kit.Outline, Size: kit.SizeSM, Icon: "copy", Disabled: !ready || !ok,
			Tip: i18n.T("Copy this tab as JSON"),
		})
		if cp.Clicked() {
			in.copy(text, i18n.T("Copied"))
		}
		all := kit.Button(c.Key("copy-all"), i18n.T("Copy as JSON"), kit.ButtonOpts{
			Variant: kit.Default, Size: kit.SizeSM, Icon: "copy", Disabled: !ready,
			Tip: i18n.T("Copy everything as JSON"),
		})
		if all.Clicked() {
			in.copy(in.asJSON(), i18n.T("Copied"))
		}
	})
}

// tabText is the current tab as JSON, and whether it has any.
func (in *inspector) tabText() (string, bool) {
	r := in.report
	var s string
	switch in.tab {
	case tabOverview:
		s = r.Mapped
	case tabApple:
		s = r.Raw
	case tabLibrary:
		s = r.Row
	case tabAddon:
		s = in.addonJSON()
	}
	return s, s != ""
}

// copy puts text on the clipboard and says so in a toast, or says the
// clipboard would not take it.
func (in *inspector) copy(text, done string) {
	if in.env.Copy != nil && in.env.Copy(text) {
		kit.ShowToast(kit.ToastSpec{Title: done})
		return
	}
	kit.ToastErrorf(i18n.ErrorText(services.CodeClipboard), "")
}

// openLink opens a link the user clicked, if it is one of the web's.
func (in *inspector) openLink(link string) {
	if !isWebLink(link) || in.env.OpenURL == nil {
		return
	}
	if err := in.env.OpenURL(link); err != nil {
		kit.ToastFailure(i18n.T("Couldn't open the link"), err)
	}
}
