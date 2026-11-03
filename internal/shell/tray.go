package shell

import (
	"context"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/coredev-uk/sona-go/internal/diag"
	"github.com/coredev-uk/sona-go/internal/i18n/l10n"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
	"github.com/coredev-uk/sona-go/internal/privatemode"
)

// transportTimeout bounds a tray transport command.
const transportTimeout = 30 * time.Second

// trayState is what the tray menu shows of playback.
type trayState struct {
	// NowPlaying is "Title – Artist", or "" with nothing selected.
	NowPlaying string
	// Playing: the play item reads Pause.
	Playing bool
	// HasItem: there is something to play or pause.
	HasItem bool
	// CanNext and CanPrevious: skipping applies (not to a live station).
	CanNext, CanPrevious bool
}

func trayModel(snap coordinator.Snapshot) trayState {
	cur := snap.Current
	if cur == nil {
		return trayState{}
	}
	now := cur.Item.Title
	if cur.Item.Artist != "" {
		now += " – " + cur.Item.Artist
	}
	return trayState{
		NowPlaying:  now,
		Playing:     snap.State == playback.StatePlaying || snap.State == playback.StateLoading,
		HasItem:     true,
		CanNext:     snap.Controls.Next,
		CanPrevious: snap.Controls.Previous,
	}
}

// The tray menu's text, in the user's language (Options.Language).
const (
	msgNotPlaying l10n.Message = "Not playing"
	msgPlay       l10n.Message = "Play"
	msgPause      l10n.Message = "Pause"
	msgNext       l10n.Message = "Next"     // plays the next song
	msgPrevious   l10n.Message = "Previous" // plays the previous song
	msgPrivate    l10n.Message = "Private listening"
	msgPrivateDev l10n.Message = "Private listening (development build)"
	msgPrivateEnv l10n.Message = "Private listening (%s)" // the environment variable that forces it on, as NAME=1
	msgShowSona   l10n.Message = "Show Sona"
	msgMiniPlayer l10n.Message = "Mini player"
	msgCloseMini  l10n.Message = "Close mini player"
	msgQuitSona   l10n.Message = "Quit Sona"
)

// privateItem is the tray's private listening checkbox: its label says
// where a state the user did not choose comes from, and a forced one
// cannot be unchecked.
func privateItem(p *l10n.Printer, st privatemode.State) (label string, enabled bool) {
	switch st.Source {
	case privatemode.SourceDevelopment:
		return p.T(msgPrivateDev), true
	case privatemode.SourceEnvironment:
		return p.Sprintf(msgPrivateEnv, privatemode.EnvVar+"=1"), false
	}
	return p.T(msgPrivate), true
}

// tray is the StatusNotifierItem: transport, private listening, the
// windows and Quit. Its menu follows the coordinator's snapshots, the
// private mode, the mini player and the language.
type tray struct {
	s       *Shell
	icon    *application.SystemTray
	now     *application.MenuItem
	play    *application.MenuItem
	next    *application.MenuItem
	prev    *application.MenuItem
	private *application.MenuItem // nil without a private mode
	show    *application.MenuItem
	mini    *application.MenuItem
	quit    *application.MenuItem

	// mu guards what the menu shows; render writes it under renderMu.
	// render never runs on the main thread, and mu is never held while
	// writing: on Linux each menu write waits for the main thread.
	mu         sync.Mutex
	state      trayState
	miniOpen   bool
	privateNow privatemode.State

	renderMu sync.Mutex
	written  trayView
	rendered bool
}

// trayView is the menu's text and states as written.
type trayView struct {
	now, play, next, prev, private, show, mini, quit string
	hasItem, canNext, canPrevious                    bool
	privateEnabled, privateOn                        bool
}

func newTray(s *Shell, icon []byte) *tray {
	t := &tray{s: s}
	menu := application.NewMenu()
	t.now = menu.Add("").SetEnabled(false)
	menu.AddSeparator()
	t.play = menu.Add("").SetEnabled(false).OnClick(func(*application.Context) { t.transport("toggle", s.o.Coord.Toggle) })
	t.next = menu.Add("").SetEnabled(false).OnClick(func(*application.Context) { t.transport("next", s.o.Coord.Next) })
	t.prev = menu.Add("").SetEnabled(false).OnClick(func(*application.Context) { t.transport("previous", s.o.Coord.Previous) })
	if m := s.o.Private; m != nil {
		menu.AddSeparator()
		t.privateNow = m.State()
		t.private = menu.AddCheckbox("", t.privateNow.On).OnClick(func(*application.Context) { t.togglePrivate() })
		m.OnChange(t.privateChanged)
	}
	menu.AddSeparator()
	t.show = menu.Add("").OnClick(func(*application.Context) { s.ShowMain() })
	t.mini = menu.Add("").OnClick(func(*application.Context) { s.ToggleMini() })
	menu.AddSeparator()
	t.quit = menu.Add("").OnClick(func(*application.Context) { s.Quit() })
	t.render()

	t.icon = s.o.App.SystemTray.New()
	t.icon.SetLabel("Sona")
	t.icon.SetIcon(icon)
	t.icon.SetMenu(menu)
	// A primary click (Activate) brings the window back; the menu is on
	// the secondary click.
	t.icon.OnClick(s.ShowMain)
	return t
}

// view is what the menu shows in p's language.
func (t *tray) view(p *l10n.Printer) trayView {
	t.mu.Lock()
	st, miniOpen, priv := t.state, t.miniOpen, t.privateNow
	t.mu.Unlock()
	v := trayView{
		now: st.NowPlaying, play: p.T(msgPlay), next: p.T(msgNext), prev: p.T(msgPrevious),
		show: p.T(msgShowSona), mini: p.T(msgMiniPlayer), quit: p.T(msgQuitSona),
		hasItem: st.HasItem, canNext: st.CanNext, canPrevious: st.CanPrevious, privateOn: priv.On,
	}
	if v.now == "" {
		v.now = p.T(msgNotPlaying)
	}
	if st.Playing {
		v.play = p.T(msgPause)
	}
	if miniOpen {
		v.mini = p.T(msgCloseMini)
	}
	v.private, v.privateEnabled = privateItem(p, priv)
	return v
}

// render writes what changed in the menu since the last render, in the
// current language. The last render always shows the latest state.
func (t *tray) render() {
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	v, w := t.view(t.s.o.Language.Printer()), t.written
	all := !t.rendered
	label := func(item *application.MenuItem, next, prev string) {
		if all || next != prev {
			item.SetLabel(next)
		}
	}
	label(t.now, v.now, w.now)
	label(t.play, v.play, w.play)
	label(t.next, v.next, w.next)
	label(t.prev, v.prev, w.prev)
	label(t.show, v.show, w.show)
	label(t.mini, v.mini, w.mini)
	label(t.quit, v.quit, w.quit)
	if all || v.hasItem != w.hasItem {
		t.play.SetEnabled(v.hasItem)
	}
	if all || v.canNext != w.canNext {
		t.next.SetEnabled(v.canNext)
	}
	if all || v.canPrevious != w.canPrevious {
		t.prev.SetEnabled(v.canPrevious)
	}
	if t.private != nil {
		label(t.private, v.private, w.private)
		if all || v.privateEnabled != w.privateEnabled {
			t.private.SetEnabled(v.privateEnabled)
		}
		if all || v.privateOn != w.privateOn {
			t.private.SetChecked(v.privateOn)
		}
	}
	t.written, t.rendered = v, true
}

// transport runs a playback command off the GTK thread, which delivered
// the click.
func (t *tray) transport(name string, cmd func(context.Context) error) {
	t.s.o.Logger.Info("playback: command", "cmd", name, "from", "tray")
	diag.Go(t.s.o.Logger, "tray: "+name, func() {
		ctx, cancel := context.WithTimeout(context.Background(), transportTimeout)
		defer cancel()
		if err := cmd(ctx); err != nil {
			t.s.o.Logger.Warn("tray: "+name, "err", err)
		}
	})
}

// togglePrivate flips private listening off the GTK thread (saving the
// setting writes a file), then shows the resulting state: GTK has already
// flipped the check mark, which a failed or refused change must undo.
func (t *tray) togglePrivate() {
	m := t.s.o.Private
	diag.Go(t.s.o.Logger, "tray: private", func() {
		st, err := m.Toggle()
		if err != nil {
			t.s.o.Logger.Warn("tray: private listening", "err", err)
		}
		// GTK's own flip of the check mark is not in written: have render
		// write it again.
		t.renderMu.Lock()
		t.written.privateOn = !st.On
		t.renderMu.Unlock()
		t.privateChanged(st)
	})
}

// privateChanged shows st on the private listening item.
func (t *tray) privateChanged(st privatemode.State) {
	t.mu.Lock()
	t.privateNow = st
	t.mu.Unlock()
	t.render()
}

// follow applies snapshots to the menu until snaps closes.
func (t *tray) follow(snaps <-chan coordinator.Snapshot) {
	for snap := range snaps {
		t.apply(trayModel(snap))
	}
}

func (t *tray) apply(st trayState) {
	t.mu.Lock()
	changed := st != t.state
	t.state = st
	t.mu.Unlock()
	if changed {
		t.render()
	}
}

// miniShown updates the mini player item's label. It is called on the
// main thread, so the menu is written from another goroutine.
func (t *tray) miniShown(shown bool) {
	t.mu.Lock()
	t.miniOpen = shown
	t.mu.Unlock()
	diag.Go(t.s.o.Logger, "tray: mini player label", t.render)
}
