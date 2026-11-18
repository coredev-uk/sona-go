package inspect

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// guide is the width of one level of indentation, and of the arrow of a
// container: a column the rule of each level runs down.
const guide = 16

// labelWidth is the ledger's label column.
const labelWidth = 200

// Token colours of a JSON value, from the palette that every theme validates
// as text on every background it sits on (theme.Validate's textTokens): the
// key is muted, a string ink, a number green, a boolean red, a null faint.
// Colour is never the only sign: strings are in quotes, a null says "null".
func tokenColour(k jkind, key bool) ui.Color {
	if key {
		return theme.C("ink-muted")
	}
	switch k {
	case jNumber:
		return theme.C("positive")
	case jBool:
		return theme.C("danger")
	case jNull:
		return theme.C("ink-faint")
	}
	return theme.C("ink")
}

// tokenNames are the palette tokens tokenColour reads, for the test that
// they stay among those every theme validates and stay distinct.
var tokenNames = map[string]string{"key": "ink-muted", "string": "ink", "number": "positive", "boolean": "danger", "null": "ink-faint"}

// hot reports whether a row shows its actions: the pointer is on it, or it is
// the one chosen (the keyboard's).
func hot(row ui.Element, chosen bool) bool { return row.Hovered() || chosen }

// actions are what the pointer finds on a row it is over: Copy value, Copy
// path and, for a link, Open. They are for the pointer alone and stay off the
// accessibility tree, where a button inside a row could not be reached: the
// keyboard and assistive technology have the same three in the foot, acting on
// the row chosen, and Enter and Ctrl+Shift+C.
func (in *inspector) actions(c *ui.Context, value, path, link string) {
	ui.Row(c.Key("acts")).Shrink(0).Gap(2).AlignItems(ui.Center).Role(ui.RoleNone).Children(func() {
		btn := func(key, icon, label string) bool {
			b := ui.Box(c.Key(key)).Size(24, 24).Shrink(0).Radius(theme.Current().Radius["md"]).Center().Role(ui.RoleNone).Label(label)
			if b.Hovered() {
				b.Background(theme.C("hover"))
			}
			b.Children(func() { theme.Glyph(c, icon, 12, theme.GlyphOpts{Color: theme.C("ink-muted")}) })
			kit.Tooltip(c, b, label)
			return b.Clicked()
		}
		if link != "" && btn("open", "external-link", i18n.T("Open")) {
			in.openLink(link)
		}
		if btn("cv", "copy", i18n.T("Copy value")) {
			in.copy(value, i18n.T("Copied"))
		}
		if path != "" && btn("cp", "link-2", i18n.T("Copy path")) {
			in.copy(path, i18n.T("Copied path"))
		}
	})
}

// ledger is a label/value table of rows: the label muted in a column of 200
// DIPs, the value in ink and selectable. Up and Down choose a row, Enter
// copies its value and Ctrl+Shift+C its label; the pointer shows the same on
// a row as buttons.
func (in *inspector) ledger(c *ui.Context, st *ledgerState, rows []ledgerRow, label string) {
	if len(rows) == 0 {
		in.empty(c, i18n.T("Nothing to show"), i18n.T("Go has no fields for this item."))
		return
	}
	st.list.Selected = &st.chosen
	list := ui.List(c.Key("ledger"), &st.list, len(rows), func(i int) {
		in.ledgerRow(c, st, rows, i)
	}).Grow(1).FillWidth().Padding(4, 24).Label(label)
	in.keys(list)
}

// keys is Enter and Ctrl+Shift+C on a list or tree: Copy value and Copy path of
// the row chosen.
func (in *inspector) keys(list ui.Element) {
	sel := in.selection()
	if !sel.ok {
		return
	}
	if list.Submitted() {
		in.copy(sel.value, i18n.T("Copied"))
	}
	if list.Shortcut(ui.Ctrl|ui.Shift, ui.KeyC) {
		in.copy(sel.path, i18n.T("Copied path"))
	}
}

// selection is the row chosen in the tab shown: what Copy value and Copy path
// put on the clipboard, and the link Open opens ("" for none).
type selection struct {
	value, path, link string
	ok                bool
}

func (in *inspector) selection() selection {
	d := in.data
	if d == nil || in.err != nil || in.loading {
		return selection{}
	}
	if t := in.treeShown(); t != nil {
		n := t.chosenNode()
		if n == nil {
			return selection{}
		}
		sel := selection{value: n.copyValue(), path: n.path, ok: true}
		if n.kind == jString && isWebLink(n.text) {
			sel.link = n.text
		}
		return sel
	}
	var rows []ledgerRow
	var st *ledgerState
	switch {
	case in.tab == tabOverview && in.view == 0:
		rows, st = d.overview, &in.lists[0]
	case in.tab == tabLibrary:
		rows, st = d.library, &in.lists[1]
	default:
		return selection{}
	}
	if st.chosen < 0 || st.chosen >= len(rows) {
		return selection{}
	}
	r := rows[st.chosen]
	return selection{value: r.copy, path: r.label, link: r.url, ok: true}
}

// treeShown is the tree of the tab shown, nil for a ledger.
func (in *inspector) treeShown() *jtree {
	d := in.data
	switch {
	case d == nil:
		return nil
	case in.tab == tabApple:
		return d.apple
	case in.tab == tabAddon:
		return d.addon
	case in.tab == tabOverview && in.view == 1:
		return d.mapped
	}
	return nil
}

func (in *inspector) ledgerRow(c *ui.Context, st *ledgerState, rows []ledgerRow, i int) {
	r := rows[i]
	k := c.Key("lr" + strconv.Itoa(i))
	gap := float32(0)
	if i > 0 && rows[i-1].group != r.group {
		gap = 12
	}
	row := ui.Row(k).FillWidth().Gap(24).Padding(7+gap, 0, 7, 0).AlignItems(ui.Start).Radius(theme.Current().Radius["md"])
	row.Children(func() {
		theme.Text(k.Key("l"), r.label, "secondary").TextColor(theme.C("ink-muted")).Width(labelWidth).Shrink(0).Unselectable()
		ui.Row(k.Key("v")).Grow(1).Shrink(1).MinWidth(0).Gap(8).AlignItems(ui.Start).Children(func() {
			in.ledgerValue(k, r)
		})
		if hot(row, st.chosen == i) && (r.copy != "" || r.url != "") {
			in.actions(k, r.copy, "", r.url)
		}
	})
}

// ledgerValue is the value of a row: a swatch and its hex, a link, tabular
// figures, a null in faint ink, a muted note after it.
func (in *inspector) ledgerValue(c *ui.Context, r ledgerRow) {
	if r.kind == rowColour {
		ui.Box(c.Key("sw")).Size(14, 14).Shrink(0).Margin(2, 0, 0, 0).Radius(3).Background(r.colour).Border(1, theme.C("edge")).Role(ui.RoleImage).Label(r.value)
	}
	switch r.kind {
	case rowLink, rowArt:
		l := ui.Link(c.Key("link"), r.value, "").FontSize(13).FixedLineHeight(18).TextColor(theme.C("ink")).Underline().Shrink(1).MinWidth(0)
		if l.Clicked() {
			in.openLink(r.url)
		}
	default:
		t := theme.Text(c.Key("t"), r.value, "secondary").TextColor(ledgerColour(r.kind)).Selectable().Shrink(1).MinWidth(0)
		if r.kind == rowNumber {
			t.FontFeatures("tnum")
		}
	}
	if r.note != "" {
		theme.Text(c.Key("n"), r.note, "secondary").TextColor(theme.C("ink-muted")).FontFeatures("tnum").Shrink(1).MinWidth(0).Unselectable()
	}
}

// ledgerColour is the colour of a ledger value: ink, a null faint. The ledger
// is for reading, so numbers and booleans keep the ink; the JSON views colour
// them.
func ledgerColour(k rowKind) ui.Color {
	if k == rowNull {
		return tokenColour(jNull, false)
	}
	return tokenColour(jString, false)
}

// empty is a tab with nothing in it: what is missing, and why.
func (in *inspector) empty(c *ui.Context, title, why string) {
	ui.Column(c.Key("empty")).FillWidth().Grow(1).Center().Gap(4).Padding(24).Children(func() {
		theme.Text(c.Key("t"), title, "heading").TextColor(theme.C("ink"))
		theme.Text(c.Key("d"), why, "secondary").TextColor(theme.C("ink-muted"))
	})
}

// treeView is a payload as a tree: a filter field, then the rows. note is the
// line under the field (where the payload came from).
func (in *inspector) treeView(c *ui.Context, t *jtree, note, label string) {
	if t == nil {
		in.empty(c, i18n.T("Nothing to show"), "")
		return
	}
	ui.Row(c.Key("filter-row")).FillWidth().Shrink(0).Padding(12, 24, 8).Gap(16).AlignItems(ui.Center).Children(func() {
		field := kit.SearchField(c.Key("filter"), &t.query, kit.SearchFieldOpts{
			Placeholder: i18n.T("Filter keys and values"), A11yLabel: i18n.T("Filter {what}", "what", label), Width: 320,
		})
		if in.wantFilter {
			field.Focus()
			in.wantFilter = false
		}
		t.setFilter()
		if t.keep != nil && t.matches > 0 {
			theme.Text(c.Key("count"), i18n.T("{count, plural, one {# match} other {# matches}}", "count", t.matches), "meta").
				TextColor(theme.C("ink-muted")).FontFeatures("tnum").Role(ui.RoleStatus)
		}
		if note != "" {
			theme.Text(c.Key("note"), note, "meta").TextColor(theme.C("ink-muted")).Grow(1).MinWidth(0)
		}
	})
	if t.noMatches() {
		in.empty(c, i18n.T("No matches"), i18n.T("No key or value holds “{query}”.", "query", t.query))
		return
	}
	t.state.Indent = guide
	t.state.Prefix = func(r ui.OutlineRow) { in.prefix(c, r) }
	tree := ui.Outline(c.Key("tree"), &t.state, t.roots(), t.children, func(n *jnode) { in.treeRow(c, t, n) }).
		Grow(1).FillWidth().Padding(0, 24, 4).Label(label)
	in.keys(tree)
	if t.hasMore() {
		ui.Row(c.Key("more")).FillWidth().Shrink(0).Padding(8, 24).Gap(12).AlignItems(ui.Center).Children(func() {
			theme.Text(c.Key("t"), i18n.T("Showing the first {shown} of {matches} matches", "shown", t.shown, "matches", t.matches), "meta").
				TextColor(theme.C("ink-muted")).FontFeatures("tnum")
			if kit.Button(c.Key("more-btn"), i18n.T("Show more"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeXS}).Clicked() {
				t.more()
			}
		})
	}
}

// prefix is the start of a row: one column of rule per level of depth, then
// the arrow of a container.
func (in *inspector) prefix(c *ui.Context, r ui.OutlineRow) {
	for d := range r.Depth {
		ui.Box(c.Key("g" + strconv.Itoa(d))).Width(guide).Shrink(0).AlignSelf(ui.Stretch).Role(ui.RoleNone).
			Draw(func(p *ui.Painter, rc ui.Rect) {
				p.Fill(ui.Rect{X: rc.X + guide/2, Y: rc.Y, W: 1, H: rc.H}, theme.C("line"), 0)
			})
	}
	arrow := r.Arrow().Size(guide, guide).Shrink(0).Center()
	if r.Branch {
		arrow.Children(func() {
			name := "chevron-right"
			if r.Open {
				name = "chevron-down"
			}
			theme.Glyph(c.Key("chev"), name, 12, theme.GlyphOpts{Color: theme.C("ink-muted")})
		})
	}
}

// treeRow is the content of one row: the key, then the value or, for a
// container, how much is in it while it is folded.
func (in *inspector) treeRow(c *ui.Context, t *jtree, n *jnode) {
	k := c.Key("tr")
	row := ui.Row(k).Grow(1).MinWidth(0).Gap(8).Padding(3, 8, 3, 4).AlignItems(ui.Start)
	chosen := t.chosenNode() == n
	row.Children(func() {
		if n.key != "" {
			theme.Text(k.Key("k"), n.key, "secondary").TextColor(tokenColour(n.kind, true)).FontWeight(500).Shrink(0).Unselectable()
		}
		in.treeValue(k, t, n)
		ui.Box(k.Key("sp")).Grow(1)
		if hot(row, chosen) {
			link := ""
			if n.kind == jString && isWebLink(n.text) {
				link = n.text
			}
			in.actions(k, n.copyValue(), n.path, link)
		}
	})
}

func (in *inspector) treeValue(c *ui.Context, t *jtree, n *jnode) {
	switch {
	case n.container():
		if len(n.kids) == 0 || !t.state.Open.Has(n) {
			theme.Text(c.Key("s"), n.summary(), "secondary").TextColor(theme.C("ink-faint")).FontFeatures("tnum").Shrink(0).Unselectable()
		}
	case n.kind == jString && isWebLink(n.text):
		l := ui.Link(c.Key("link"), strconv.Quote(n.text), "").FontSize(13).FixedLineHeight(18).TextColor(tokenColour(jString, false)).Underline().Shrink(1).MinWidth(0)
		if l.Clicked() {
			in.openLink(n.text)
		}
	default:
		tx := theme.Text(c.Key("v"), n.value(), "secondary").TextColor(tokenColour(n.kind, false)).Selectable().Shrink(1).MinWidth(0)
		if n.kind == jNumber {
			tx.FontFeatures("tnum")
		}
	}
}
