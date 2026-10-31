package shell

import (
	"context"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/coredev-uk/sona-go/internal/diag"
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
	// HasItem: there is something to play, pause or skip.
	HasItem bool
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
		NowPlaying: now,
		Playing:    snap.State == playback.StatePlaying || snap.State == playback.StateLoading,
		HasItem:    true,
	}
}

// privateItem is the tray's private listening checkbox: its label says
// where a state the user did not choose comes from, and a forced one
// cannot be unchecked.
func privateItem(st privatemode.State) (label string, enabled bool) {
	switch st.Source {
	case privatemode.SourceDevelopment:
		return "Private listening (development build)", true
	case privatemode.SourceEnvironment:
		return "Private listening (" + privatemode.EnvVar + "=1)", false
	}
	return "Private listening", true
}

// tray is the StatusNotifierItem: transport, private listening, the
// windows and Quit. Its menu follows the coordinator's snapshots and the
// private mode.
type tray struct {
	s       *Shell
	icon    *application.SystemTray
	now     *application.MenuItem
	play    *application.MenuItem
	next    *application.MenuItem
	prev    *application.MenuItem
	private *application.MenuItem // nil without a private mode
	mini    *application.MenuItem
	state   trayState
}

func newTray(s *Shell, icon []byte) *tray {
	t := &tray{s: s}
	menu := application.NewMenu()
	t.now = menu.Add("Not playing").SetEnabled(false)
	menu.AddSeparator()
	t.play = menu.Add("Play").SetEnabled(false).OnClick(func(*application.Context) { t.transport("toggle", s.o.Coord.Toggle) })
	t.next = menu.Add("Next").SetEnabled(false).OnClick(func(*application.Context) { t.transport("next", s.o.Coord.Next) })
	t.prev = menu.Add("Previous").SetEnabled(false).OnClick(func(*application.Context) { t.transport("previous", s.o.Coord.Previous) })
	if m := s.o.Private; m != nil {
		menu.AddSeparator()
		st := m.State()
		label, enabled := privateItem(st)
		t.private = menu.AddCheckbox(label, st.On).SetEnabled(enabled).OnClick(func(*application.Context) { t.togglePrivate() })
		m.OnChange(t.privateChanged)
	}
	menu.AddSeparator()
	menu.Add("Show Sona").OnClick(func(*application.Context) { s.ShowMain() })
	t.mini = menu.Add("Mini player").OnClick(func(*application.Context) { s.ToggleMini() })
	menu.AddSeparator()
	menu.Add("Quit Sona").OnClick(func(*application.Context) { s.Quit() })

	t.icon = s.o.App.SystemTray.New()
	t.icon.SetLabel("Sona")
	t.icon.SetIcon(icon)
	t.icon.SetMenu(menu)
	// A primary click (Activate) brings the window back; the menu is on
	// the secondary click.
	t.icon.OnClick(s.ShowMain)
	return t
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
		t.privateChanged(st)
	})
}

// privateChanged shows st on the private listening item.
func (t *tray) privateChanged(st privatemode.State) {
	label, enabled := privateItem(st)
	t.private.SetLabel(label).SetEnabled(enabled).SetChecked(st.On)
}

// follow applies snapshots to the menu until snaps closes.
func (t *tray) follow(snaps <-chan coordinator.Snapshot) {
	for snap := range snaps {
		t.apply(trayModel(snap))
	}
}

func (t *tray) apply(st trayState) {
	if st == t.state {
		return
	}
	t.state = st
	if st.NowPlaying == "" {
		t.now.SetLabel("Not playing")
	} else {
		t.now.SetLabel(st.NowPlaying)
	}
	if st.Playing {
		t.play.SetLabel("Pause")
	} else {
		t.play.SetLabel("Play")
	}
	t.play.SetEnabled(st.HasItem)
	t.next.SetEnabled(st.HasItem)
	t.prev.SetEnabled(st.HasItem)
}

// miniShown updates the mini player item's label.
func (t *tray) miniShown(shown bool) {
	if shown {
		t.mini.SetLabel("Close mini player")
	} else {
		t.mini.SetLabel("Mini player")
	}
}
