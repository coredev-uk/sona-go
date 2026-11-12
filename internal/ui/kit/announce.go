package kit

import (
	"github.com/egoist/mygo/ui"
)

// AnnounceMounted announces msg in the frame the status e is first built,
// when it comes with news. MyGo reads a status (ui.RoleStatus) when its
// text changes, as a live region is read on the web, and not when it first
// appears: so a status that shows up with its message (a note under a
// filter that finds nothing, a notice that turns up) is announced here,
// and what changes it afterwards is its own to read. Announcing both,
// as a status that changes does when it also calls Context.Announce, says
// everything twice. An empty msg announces nothing.
func AnnounceMounted(c *ui.Context, e ui.Element, msg string) {
	if built := ui.Local(e, "announce-mounted", func() bool { return false }); !*built {
		*built = true
		if msg != "" {
			c.Announce(msg)
		}
	}
}
