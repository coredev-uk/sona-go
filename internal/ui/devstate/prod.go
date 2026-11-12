//go:build !dev

package devstate

import (
	"log/slog"

	"github.com/egoist/mygo/ui"
)

// Load reads the state the last run left: none in a production build.
func Load() (State, bool) { return State{}, false }

// Restore arms the scroll offsets and flags of st.
func Restore(State) {}

// Save writes the state: route is the router's location.
func Save(string, *Window) {}

// SetName gives the program its own state file: nothing in a production
// build.
func SetName(string) {}

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
