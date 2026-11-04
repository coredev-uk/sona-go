//go:build dev

package app

import (
	"github.com/egoist/mygo"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/devstate"
)

// devHook is what a development build does with the main window: restore
// the last run's screen, scroll and window place (cmd/devloop restarts the
// app on every build), save them again as it quits, and reload design
// tokens and catalogues as their files change (devstate).
func (a *App) devHook(win *mygo.Window) {
	devstate.SetLocation(a.router.Location)
	devstate.Live(a.post, a.log)
	if st, ok := devstate.Load(); ok {
		a.devRestore(win, st)
	}
	save := func() { a.devSave(win) }
	win.OnClose(func(*mygo.CloseEvent) { save() })
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { save() })
}

// devRestore puts the window and the router where st says.
func (a *App) devRestore(win *mygo.Window, st devstate.State) {
	if w := st.Window; w != nil && w.Width >= config.MinWindowWidth && w.Height >= config.MinWindowHeight {
		win.SetBounds(mygo.Rectangle{X: w.X, Y: w.Y, Width: w.Width, Height: w.Height})
		if w.Maximised {
			win.Maximize()
		}
	}
	if st.Route != "" {
		a.router.Replace(st.Route)
	}
	devstate.Restore(st)
}

// devSave leaves where the app is for the next run.
func (a *App) devSave(win *mygo.Window) {
	if win.IsDestroyed() {
		return
	}
	b := win.Bounds()
	if b.Width <= 0 || b.Height <= 0 {
		return
	}
	devstate.Save(a.router.Location(), &devstate.Window{X: b.X, Y: b.Y, Width: b.Width, Height: b.Height, Maximised: win.IsMaximized()})
}
