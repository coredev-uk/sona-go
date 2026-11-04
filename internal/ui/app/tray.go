package app

import (
	"context"
	"time"

	"github.com/egoist/mygo"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// trayTimeout bounds a tray transport command.
const trayTimeout = 30 * time.Second

// trayState is what the tray menu shows: what plays, private listening and
// whether the mini player is shown.
type trayState struct {
	play    services.PlaybackState
	private *services.PrivateState // nil without private listening
	mini    bool
	hasMini bool // a module builds a mini player
}

// trayView is the tray menu's text and states, in the language of the UI.
type trayView struct {
	now, play, next, prev, private, show, mini, quit string
	hasItem, canNext, canPrev                        bool
	privateEnabled, privateOn                        bool
}

// view is v0.2.0's tray menu (internal/shell/tray.go): "Title – Artist" or
// Not playing, Play or Pause, Next, Previous, private listening (labelled with
// where a state the user did not choose comes from; a forced one cannot be
// unchecked), Show Sona, Mini player or Close mini player, Quit Sona.
func (s trayState) view() trayView {
	v := trayView{
		now: i18n.T("Not playing"), play: i18n.T("Play"), next: i18n.T("Next"), prev: i18n.T("Previous"),
		show: i18n.T("Show Sona"), mini: i18n.T("Mini Player"), quit: i18n.T("Quit Sona"),
	}
	if cur := s.play.Current; cur != nil {
		v.now = cur.Name
		if cur.Artist != "" {
			v.now += " – " + cur.Artist
		}
		v.hasItem = true
		v.canNext, v.canPrev = s.play.Controls.Next, s.play.Controls.Previous
	}
	if v.hasItem && (s.play.State == "playing" || s.play.State == "loading") {
		v.play = i18n.T("Pause")
	}
	if s.mini {
		v.mini = i18n.T("Close Mini Player")
	}
	if p := s.private; p != nil {
		v.privateOn, v.privateEnabled = p.On, true
		switch p.Source {
		case "development":
			v.private = i18n.T("Private listening (development build)")
		case "environment":
			v.private, v.privateEnabled = i18n.T("Private listening (forced by SONA_PRIVATE=1)"), false
		default:
			v.private = i18n.T("Private listening")
		}
	}
	return v
}

// tray is Sona's icon in the menu bar (macOS) or the notification area
// (libayatana-appindicator3 on Linux): transport, private listening, the
// windows and Quit. A click on the icon opens its menu. It follows the
// playback mirror, private listening, the mini player and the language.
// Main thread only.
type tray struct {
	icon    *mygo.Tray
	menu    *mygo.Menu
	items   map[string]*mygo.MenuItem
	state   trayState
	written trayView
	drawn   bool
}

// newTray puts the icon up, or returns nil (logged) where the desktop has no
// tray: Sona then quits when its window closes (services.Windows.HasTray).
func (a *App) newTray(icon []byte, template bool) *tray {
	t := &tray{items: map[string]*mygo.MenuItem{}}
	item := func(id string, typ mygo.MenuItemType, click func()) *mygo.MenuItem {
		it := &mygo.MenuItem{ID: id, Type: typ, Label: id}
		if click != nil {
			it.Click = func(*mygo.MenuItem, *mygo.Window) { click() }
		}
		t.items[id] = it
		return it
	}
	var private *services.PrivateService
	if a.opts.Core != nil {
		private = services.Service[services.PrivateService](a.opts.Core)
		st := private.State()
		t.state.private = &st
	}
	t.state.hasMini = a.mini != nil
	items := []*mygo.MenuItem{
		item("now", mygo.MenuItemNormal, nil),
		mygo.Separator(),
		item("play", mygo.MenuItemNormal, func() { a.transport("toggle", a.playback.Toggle) }),
		item("next", mygo.MenuItemNormal, func() { a.transport("next", a.playback.Next) }),
		item("prev", mygo.MenuItemNormal, func() { a.transport("previous", a.playback.Prev) }),
	}
	if private != nil {
		items = append(items, mygo.Separator(), item("private", mygo.MenuItemCheckbox, func() { a.togglePrivate(private) }))
	}
	items = append(items, mygo.Separator(), item("show", mygo.MenuItemNormal, a.opts.Windows.ShowMain))
	if t.state.hasMini {
		items = append(items, item("mini", mygo.MenuItemNormal, a.opts.Windows.ToggleMini))
	}
	items = append(items, mygo.Separator(), item("quit", mygo.MenuItemNormal, a.opts.Windows.Quit))
	t.items["now"].Disabled = true
	t.menu = mygo.NewMenu(items)
	if a.playback != nil {
		t.state.play = a.playback.State()
	}
	t.render()
	var err error
	t.icon, err = mygo.NewTray(mygo.TrayOptions{ID: "sona", Icon: icon, IconIsTemplate: template, ToolTip: windowTitle, Menu: t.menu})
	if err != nil {
		a.log.Warn("tray unavailable: closing the window quits", "err", err)
		return nil
	}
	return t
}

// follow keeps the menu on the playback mirror and private listening.
func (t *tray) follow(bus *store.Bus) {
	store.On(bus, services.EventPlaybackState, func(st services.PlaybackState) {
		t.state.play = st
		t.render()
	})
	store.On(bus, services.EventPrivateChanged, func(st services.PrivateState) {
		t.state.private = &st
		t.render()
	})
}

// setMini follows the mini player's shown state.
func (t *tray) setMini(shown bool) {
	t.state.mini = shown
	t.render()
}

// render writes what changed since the last render, in the current language
// (the language's change renders it again).
func (t *tray) render() {
	v, w := t.state.view(), t.written
	all := !t.drawn
	label := func(id, next, prev string) {
		if it := t.items[id]; it != nil && (all || next != prev) {
			it.SetLabel(next)
		}
	}
	enable := func(id string, next, prev bool) {
		if it := t.items[id]; it != nil && (all || next != prev) {
			it.SetEnabled(next)
		}
	}
	label("now", v.now, w.now)
	label("play", v.play, w.play)
	label("next", v.next, w.next)
	label("prev", v.prev, w.prev)
	label("private", v.private, w.private)
	label("show", v.show, w.show)
	label("mini", v.mini, w.mini)
	label("quit", v.quit, w.quit)
	enable("play", v.hasItem, w.hasItem)
	enable("next", v.canNext, w.canNext)
	enable("prev", v.canPrev, w.canPrev)
	enable("private", v.privateEnabled, w.privateEnabled)
	if it := t.items["private"]; it != nil {
		// The menu flipped the check mark itself on a click: write the
		// state every time, so a refused change shows as it is.
		it.SetChecked(v.privateOn)
	}
	t.written, t.drawn = v, true
}

// transport runs a playback command from the tray off the main thread.
func (a *App) transport(name string, cmd func(context.Context) error) {
	if a.playback == nil {
		return
	}
	a.log.Info("playback: command", "cmd", name, "from", "tray")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), trayTimeout)
		defer cancel()
		if err := cmd(ctx); err != nil {
			a.log.Warn("tray: "+name, "err", err)
		}
	}()
}

// togglePrivate flips private listening from the tray off the main thread
// (saving the setting writes a file); "private:changed" shows the outcome,
// and a refused change is written back over the menu's own flip.
func (a *App) togglePrivate(p *services.PrivateService) {
	go func() {
		st, err := p.Set(!p.State().On)
		if err != nil {
			a.log.Warn("tray: private listening", "err", err)
		}
		a.post(func() {
			if a.tray != nil {
				a.tray.state.private = &st
				a.tray.render()
			}
		})
	}()
}
