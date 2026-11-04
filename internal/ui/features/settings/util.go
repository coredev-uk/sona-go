package settings

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/module"
)

// told is what a live line last said, to announce only what changed.
type told struct {
	seen bool
	msg  string
}

// tell announces msg when it changes, as an aria-live="polite" region does:
// the first sight of a line is not announced (the page already reads it),
// later changes are. An empty msg says nothing.
func (s *settings) tell(c *ui.Context, key, msg string) {
	t := stateOf[told](s, "told:"+key)
	if !t.seen {
		t.seen, t.msg = true, msg
		return
	}
	if msg != t.msg {
		t.msg = msg
		if msg != "" {
			c.Announce(msg)
		}
	}
}

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
