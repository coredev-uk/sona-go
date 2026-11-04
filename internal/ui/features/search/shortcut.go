package search

import (
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/module"
)

// focusShortcut is search.focus: "/" or Ctrl+F focuses the top bar's search
// field from anywhere and selects its text (v0.2.0's index.ts and field.ts).
func (s *search) focusShortcut() module.Shortcut {
	return module.Shortcut{
		ID:       "search.focus",
		Label:    func() string { return i18n.T("Focus the search field") },
		Group:    "navigation",
		Keys:     []string{"/", "Ctrl+F"},
		Scope:    "global",
		Keywords: func() string { return i18n.T("find focus") },
		Icon:     "text-cursor-input",
		Run:      s.field.focusAndSelect,
	}
}

// init is the module's Init. v0.2.0's search module subscribed to no event
// (its queries have no invalidation plan: the recent list refreshes after
// each write), so there is nothing to start.
func (s *search) init() {}
