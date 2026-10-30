package shell

import "github.com/coredev-uk/sona-go/internal/config"

// observeWindow folds one observation of the main window into the
// remembered state. The size is only taken while the window is neither
// maximised nor fullscreen, so un-maximising after the next start returns
// to it; fullscreen is transient and changes nothing.
func observeWindow(prev config.Window, width, height int, maximised, fullscreen bool) config.Window {
	if fullscreen {
		return prev
	}
	next := prev
	next.Maximised = maximised
	if !maximised && width > 0 && height > 0 {
		next.Width, next.Height = width, height
	}
	return next
}
