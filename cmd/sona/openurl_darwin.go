//go:build darwin

package main

import (
	"github.com/egoist/mygo"

	"github.com/coredev-uk/sona-go/internal/app"
)

// openURLs hands the links macOS delivers to the running app (an installed
// app's music:// and itms:// handler) to activate, as a launch with the link
// would. Registered before Run, so the launch's own link comes too. Only
// macOS needs it: on Linux and Windows MyGo scans the launch arguments too,
// which Sona's own argv handling and second launches already cover.
func openURLs(activate func(app.Activation, bool)) {
	mygo.App.OnOpenURL(func(u string) { activate(app.Activation{Args: []string{u}}, true) })
}
