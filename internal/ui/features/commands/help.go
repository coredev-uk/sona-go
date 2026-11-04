package commands

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// helpSheet is the "?" sheet: every shortcut by group with its current keys.
type helpSheet struct {
	open bool
	// frames counts the frames the sheet has been open; the list takes the
	// focus on the second, once the dialog has noted where the focus was.
	frames int
}

// narrow is the window width below which the groups list in one column.
const narrow = 720

// typeText is text in one of the theme's type styles.
func typeText(c *ui.Context, s, style string) ui.Element {
	t := theme.Current().Type[style]
	return ui.Text(c, s).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).LetterSpacing(t.Tracking)
}

// show draws the sheet while it is open: a dialog (760 wide, at most 80 % of
// the window high) with its title, the line about Settings, and the list,
// which takes the focus on opening so the arrows and Page Down scroll it.
func (h *helpSheet) show(c *ui.Context, env *module.Env) {
	if !h.open {
		h.frames = 0
		return
	}
	w, wh := c.Size()
	closeOwn := func() {
		if kit.ChordPressed(c, keysOf(env, "commands.help")...) {
			h.open = false
		}
	}
	kit.Dialog(c.Key("help"), &h.open, kit.DialogOpts{
		Title: i18n.T("Keyboard shortcuts"), Description: plainAbout(), HideHeader: true, MaxWidth: 760, Padding: 0, Gap: 0, Bare: true, Raised: true,
	}, func() {
		closeOwn() // the sheet's own chord closes it again, which only the dialog sees
		ui.Column(c.Key("help:head")).Gap(2).Padding(20, 56, 16, 24).FillWidth().Children(func() {
			typeText(c.Key("title"), i18n.T("Keyboard shortcuts"), "heading").TextColor(theme.C("ink")).Role(ui.RoleHeading)
			h.about(c, env)
		})
		list := ui.Scroll(c.Key("help:list")).Role(ui.RoleGroup).Label(i18n.T("Shortcuts")).
			FillWidth().MaxHeight(min(wh*0.8, 720)-80).Padding(0, 24, 24)
		list.Children(func() { h.groups(c, env, w) })
		if h.frames++; h.frames == 2 {
			list.Focus()
		} else if h.frames < 2 {
			c.AnimationFrame()
		}
	})
}

// keysOf are the keys a shortcut answers to now.
func keysOf(env *module.Env, id string) []string {
	if env.Shortcuts == nil {
		return nil
	}
	for _, s := range env.Shortcuts() {
		if s.ID == id {
			return s.Keys
		}
	}
	return nil
}

// about is the line under the title: "Change any of them in Settings.", the
// word a link to the shortcut settings where that page exists.
func (h *helpSheet) about(c *ui.Context, env *module.Env) {
	msg := i18n.T("Change any of them in <0>Settings</0>.")
	before, rest, _ := strings.Cut(msg, "<0>")
	name, after, _ := strings.Cut(rest, "</0>")
	// Spaces at the seams become the row's gap: the text measures without them.
	gap := float32(0)
	if strings.HasSuffix(before, " ") || strings.HasPrefix(name, " ") {
		gap = 4
	}
	before = strings.TrimSpace(before)
	ui.Row(c.Key("about")).AlignItems(ui.Center).Gap(gap).Children(func() {
		typeText(c.Key("a"), before, "secondary").TextColor(theme.C("ink-muted")).SingleLine().Shrink(0)
		ui.Row(c.Key("link")).AlignItems(ui.Center).Children(func() { h.link(c, env, name, after) })
	})
}

// link is the word that links to Settings (plain text where that page does
// not exist), and what follows it.
func (h *helpSheet) link(c *ui.Context, env *module.Env, name, after string) {
	if env.HasRoute != nil && env.HasRoute("/settings") {
		link := ui.ButtonBase(c.Key("settings")).Role(ui.RoleLink).Label(name).Cursor(ui.CursorPointer)
		link.Children(func() {
			typeText(c, name, "secondary").TextColor(theme.C("ink")).Underline().SingleLine().Shrink(0)
		})
		if link.Clicked() {
			h.open = false
			env.Nav.Push("/settings?section=shortcuts")
		}
	} else {
		typeText(c.Key("s"), name, "secondary").TextColor(theme.C("ink-muted")).SingleLine().Shrink(0)
	}
	typeText(c.Key("b"), after, "secondary").TextColor(theme.C("ink-muted")).SingleLine().Shrink(0)
}

// groups lists every shortcut by group, in two columns (one when the window
// is narrow). Groups with no shortcuts are left out.
func (h *helpSheet) groups(c *ui.Context, env *module.Env, width float32) {
	var all []module.BoundShortcut
	if env.Shortcuts != nil {
		all = env.Shortcuts()
	}
	type section struct {
		group module.ShortcutGroup
		list  []module.BoundShortcut
	}
	var sections []section
	rows := 0
	for _, g := range groups {
		var list []module.BoundShortcut
		for _, s := range all {
			if s.Group == g {
				list = append(list, s)
			}
		}
		if len(list) > 0 {
			sections = append(sections, section{g, list})
			rows += len(list) + 1
		}
	}
	// Columns fill the first before the second, a group never split.
	cols := [][]section{sections}
	if width >= narrow {
		cols = [][]section{nil, nil}
		n := 0
		for _, s := range sections {
			col := 0
			if n >= rows/2 {
				col = 1
			}
			cols[col] = append(cols[col], s)
			n += len(s.list) + 1
		}
	}
	ui.Row(c.Key("help:cols")).Gap(40).AlignItems(ui.Start).FillWidth().Children(func() {
		for i, col := range cols {
			ui.Column(c.Key("col:" + string(rune('0'+i)))).Grow(1).Basis(0).MinWidth(0).Children(func() {
				for _, s := range col {
					h.section(c, s.group, s.list)
				}
			})
		}
	})
}

// section is one group: its title and a definition list of label and keys.
func (h *helpSheet) section(c *ui.Context, g module.ShortcutGroup, list []module.BoundShortcut) {
	ui.Column(c.Key("g:"+string(g))).FillWidth().Padding(0, 0, 24).Role(ui.RoleGroup).Label(groupTitle(g)).Children(func() {
		typeText(c.Key("h"), groupTitle(g), "body").FontWeight(600).TextColor(theme.C("ink")).Role(ui.RoleHeading).Padding(0, 0, 6)
		for _, s := range list {
			ui.Row(c.Key("s:"+s.ID)).MinHeight(32).FillWidth().Padding(2, 0).Gap(16).AlignItems(ui.Center).Role(ui.RoleListItem).Label(s.Label()).Children(func() {
				typeText(c.Key("l"), s.Label(), "body").TextColor(theme.C("ink")).Grow(1).MinWidth(0)
				if len(s.Keys) > 0 {
					kit.Keycaps(c.Key("k"), s.Keys...)
				} else {
					typeText(c.Key("n"), i18n.T("Not set"), "meta").TextColor(theme.C("ink-faint")).Shrink(0)
				}
			})
		}
	})
}

// plainAbout is the line under the title without its link markup, for the
// dialog's description.
func plainAbout() string {
	return strings.NewReplacer("<0>", "", "</0>", "").Replace(i18n.T("Change any of them in <0>Settings</0>."))
}
