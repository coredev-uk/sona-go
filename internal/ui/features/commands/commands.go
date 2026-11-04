// Package commands is Ctrl+K and "?": the command palette over every
// registered shortcut (and, as you type, the local library and a search of
// Apple Music), and the sheet listing every shortcut by group with the keys
// it answers to now. Both are built from Env.Shortcuts, so a rebinding or a
// new module's shortcut shows up at once; only one of them is open at a
// time.
package commands

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
)

// groups are the shortcut groups in the order the palette and the help list
// them (v0.2.0's shortcutGroups).
var groups = []module.ShortcutGroup{"playback", "view", "pages", "navigation", "general"}

// groupTitle is a group's heading.
func groupTitle(g module.ShortcutGroup) string {
	switch g {
	case "playback":
		return i18n.T("Playback")
	case "view":
		return i18n.T("View")
	case "pages":
		return i18n.T("Pages")
	case "navigation":
		return i18n.T("Navigation")
	case "general":
		return i18n.T("General")
	}
	return string(g)
}

// library is what the palette asks of the search service
// (*services.SearchService).
type library interface {
	Library(ctx context.Context, term string, perKind int) (services.LibrarySearch, error)
}

// player is what picking a song asks of the playback service
// (*services.PlaybackService).
type player interface {
	PlayItems(ctx context.Context, refs []services.TrackRef, start int, from services.PlayFrom) error
}

// commands is the feature's state.
type commands struct {
	env  *module.Env
	lib  library // nil without services
	play player  // nil without services

	pal  kit.Palette
	help helpSheet

	// The library lookup follows the typing after a pause: typed is the
	// trimmed query last seen and since when, term the one searched.
	typed   string
	typedAt time.Time
	term    string
	// shown keeps the last answer while the next one loads.
	shown []kit.PaletteEntry
}

// New is the commands module.
func New(env *module.Env) module.Module {
	m := newCommands(env)
	if env.Core != nil {
		m.lib = services.Service[services.SearchService](env.Core)
		m.play = services.Service[services.PlaybackService](env.Core)
	}
	return module.Module{
		ID: "commands",
		Shortcuts: []module.Shortcut{
			{
				ID:    "commands.palette",
				Label: func() string { return i18n.T("Open the command palette") },
				Group: "general", Keys: []string{"Ctrl+K"}, Scope: "global", Icon: "command", NoPalette: true,
				Run: m.togglePalette,
			},
			{
				ID:       "commands.help",
				Label:    func() string { return i18n.T("Keyboard shortcuts") },
				Group:    "general",
				Keys:     []string{"?"},
				Keywords: func() string { return i18n.T("help keys bindings") },
				Icon:     "keyboard",
				Run:      m.toggleHelp,
			},
		},
		Slots: module.Slots{Layer: m.layer},
	}
}

func newCommands(env *module.Env) *commands { return &commands{env: env} }

// togglePalette is the palette's own chord: it opens the palette, and closes
// it again.
func (m *commands) togglePalette() {
	if m.pal.IsOpen() {
		m.pal.Close()
		return
	}
	m.help.open = false
	m.typed, m.term, m.shown = "", "", nil // nothing of the last search carries over
	m.pal.Open()
}

// toggleHelp is the help's own chord.
func (m *commands) toggleHelp() {
	if m.help.open {
		m.help.open = false
		return
	}
	m.pal.Close()
	m.help.open = true
}

// layer is the Layer slot: both surfaces, built every frame.
func (m *commands) layer(c *ui.Context) {
	if m.pal.IsOpen() {
		m.follow(c)
	}
	m.pal.Show(c, m.sections(c))
	m.help.show(c, m.env)
}
