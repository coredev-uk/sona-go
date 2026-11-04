//go:build !dev

package devstate

import (
	"log/slog"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/devinstance"
)

// Load reads the state the last run left: none in a production build.
func Load() (State, bool) { return State{}, false }

// Restore arms the scroll offsets and flags of st.
func Restore(State) {}

// Save writes the state: route is the router's location.
func Save(string, *Window) {}

// SetLocation tells Scroll how to name the page: the router's location.
func SetLocation(func() string) {}

// Scroll keeps the scroll offset of the page being built across restarts.
// Call it in the page's view, before the container that tracks s.
func Scroll(*ui.Context, *ui.ScrollState) {}

// Flag keeps *p (an open overlay, an aside) across restarts under name.
// Call it when the module initialises.
func Flag(string, *bool) {}

// Live reloads tokens.json and the .po catalogues from the source tree as
// they change: nothing in a production build.
func Live(func(func()), *slog.Logger) {}

// Instance is SONA_DEV_INSTANCE, which names a development run so that two
// worktrees run side by side: always empty in a production build.
func Instance() string { return devinstance.Name() }
