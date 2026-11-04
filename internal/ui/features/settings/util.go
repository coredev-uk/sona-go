package settings

import (
	"strings"

	"github.com/coredev-uk/sona-go/internal/ui/module"
)

func trim(s string) string { return strings.TrimSpace(s) }

// shortcuts is every registered shortcut with its effective keys
// (Env.Shortcuts), none without a shell.
func (s *settings) shortcuts() []module.BoundShortcut {
	if s.env.Shortcuts == nil {
		return nil
	}
	return s.env.Shortcuts()
}
