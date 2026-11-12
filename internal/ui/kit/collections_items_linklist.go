package kit

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// LinkListState is the state of a LinkList: the width its columns follow.
type LinkListState struct{ width itemsWidth }

// LinkListOpts are the options of LinkList.
type LinkListOpts struct {
	// Title heads the list; an untitled one continues the list above it
	// (Apple splits a long list in two, the second untitled), 24 DIPs under
	// it, and is named "More links".
	Title string
	// First leaves out the 40 DIPs above a titled list: the first section
	// of a page.
	First bool
}

// LinkList is links to editorial pages as a wall of buttons in even columns,
// in Apple's order down each column (its genre lists are alphabetical;
// itemsLinkGrid); web-only links say so (v0.2.0's LinkList). Each link is a
// `hover` tile, 44 DIPs at least, the name in ink (two lines before it is cut)
// and an arrow out at the end of the one that leaves Sona. In light a 1 DIP
// `line` border sets the tile off the ground; in dark the fill alone does.
func LinkList(c *ui.Context, st *LinkListState, items []services.Item, ctx ItemsCtx, o LinkListOpts) {
	tk := theme.Current()
	label := o.Title
	if label == "" {
		label = i18n.T("More links")
	}
	sec := ui.Column(c.Key("kit.links." + label)).Role(ui.RoleGroup).Label(label)
	switch {
	case o.Title == "":
		sec.Margin(24, 0, 0, 0)
	case !o.First:
		sec.Margin(40, 0, 0, 0)
	}
	sec.Children(func() {
		if o.Title != "" {
			theme.StyleText(ui.Text(c, o.Title), "heading").
				TextColor(theme.C("ink")).SingleLine().Margin(0, 0, 12, 0).Role(ui.RoleHeading).Level(2)
		}
		grid := ui.Grid(c.Key("grid")).Gap(itemsLinkGap).Role(ui.RoleList).Label(label)
		width := st.width.track(c, grid)
		rows := 1
		if width > 0 {
			var cols int
			cols, rows = itemsLinkGrid(width, len(items))
			grid.Columns(cols)
		}
		grid.Children(func() {
			if width <= 0 {
				ui.Box(c).Height(1) // the first frame measures; the next builds the links
				return
			}
			for i := range items {
				// Filled top to bottom, so Tab follows the eye.
				col, row := i/rows, i%rows
				ui.Row(c.Key(strconv.Itoa(i) + ":" + items[i].Name)).Role(ui.RoleListItem).ColumnStart(col + 1).RowStart(row + 1).Children(func() {
					itemsLinkTile(c, items[i], ctx, tk)
				})
			}
		})
	})
}

// itemsLinkTile is one destination as a button: a link to its Sona page, or a
// button that opens it without one.
func itemsLinkTile(c *ui.Context, it services.Item, ctx ItemsCtx, tk *theme.Tokens) {
	link := ItemLink(it)
	plays := PlaysWhenOpened(it, ctx.Players)
	tile := ui.ButtonBase(c).Grow(1).MinHeight(44).PaddingX(16).PaddingY(8).Gap(12).
		Justify(ui.SpaceBetween).Radius(tk.Radius["md"])
	border := theme.C("line")
	if theme.Dark() {
		border = ui.Transparent
	}
	bg := theme.C("hover")
	if tile.Hovered() {
		bg = theme.C("active")
	}
	tile.Border(1, border).Background(bg)
	theme.Fade(tile, "feedback")
	switch {
	case link != "":
		tile.Role(ui.RoleLink)
	case plays:
		// Named by its text, with the action after it.
		tile.Label(it.Name + " " + i18n.T("Play"))
	}
	body := tk.Type["ui"]
	tile.Children(func() {
		ui.Text(c, it.Name).FontSize(body.Size).FixedLineHeight(body.LineHeight).FontWeight(500).
			TextColor(theme.C("ink")).MaxLines(2).TextAlign(ui.Start).Grow(1).MinWidth(0)
		if link == "" && !plays {
			ui.Row(c.Key("out")).Size(16, 16).Shrink(0).Center().Role(ui.RoleImage).Label(i18n.T("Opens in your browser")).
				TextColor(theme.C("ink-muted")).Children(func() { ui.Icon(c, theme.Icon("arrow-up-right")).FontSize(16) })
		}
	})
	if tile.Clicked() {
		if link != "" {
			ctx.nav(link)
		} else {
			OpenWithoutPage(it, ctx)
		}
	}
}
