package settings

import (
	"strings"
	"time"

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

// stampTime is a Unix-millisecond stamp as a time; 0 (never) is the zero time.
func stampTime(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}
