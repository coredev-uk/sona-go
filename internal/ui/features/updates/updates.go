// Package updates is the update notice: the quiet line at the foot of the
// sidebar, shown only while a release newer than this build exists, with its
// version. Go checks for it after the first paint (unless turned off in
// Settings → General) and sends "updates:available"; Settings → General
// shows the same status and checks on request (store.UseUpdates). Where Sona
// updates itself (macOS and Windows installs, UpdateStatus.SelfUpdate) the
// notice offers "Install and restart", with the download's progress from
// "updates:install"; elsewhere it links the release page, since updating is
// the platform's: a package manager or an AppImage's own updater.
package updates

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// updates is the feature's state.
type updates struct {
	env *module.Env
	// openRelease opens the release page (UpdatesService.OpenRelease) and
	// install installs the update and restarts (UpdatesService.Install);
	// tests replace them.
	openRelease func() error
	install     func() error
}

// New is the update notice's module.
func New(env *module.Env) module.Module {
	var open, install func() error
	if env.Core != nil {
		svc := services.Service[services.UpdatesService](env.Core)
		open = svc.OpenRelease
		install = func() error { _, err := svc.Install(context.Background()); return err }
	}
	return newUpdates(env, open, install).module()
}

func newUpdates(env *module.Env, open, install func() error) *updates {
	return &updates{env: env, openRelease: open, install: install}
}

func (u *updates) module() module.Module {
	return module.Module{
		ID:    "updates",
		Slots: module.Slots{SidebarFooter: u.notice},
		Init: func() {
			for _, ev := range []string{services.EventUpdateAvailable, services.EventUpdateInstall} {
				store.On(u.env.Bus, ev, func(s services.UpdateStatus) {
					store.Set(u.env.Queries, store.KeyUpdates, s)
				})
			}
		},
	}
}

// notice builds nothing unless a newer release exists that the user hasn't
// just gone back from (UpdateStatus.Notify).
func (u *updates) notice(c *ui.Context) {
	r := store.UseUpdates(u.env.Queries, u.env.Core)
	if !r.HasData || !r.Data.Notify() {
		return
	}
	st := r.Data
	msg := i18n.T("Sona {latest} is available", "latest", st.Latest)
	switch {
	case st.Installed:
		msg = i18n.T("Restarting into Sona {latest}…", "latest", st.Latest)
	case st.Installing:
		msg = i18n.T("Downloading Sona {latest}…", "latest", st.Latest)
	case st.InstallError != "":
		msg = i18n.T("Couldn't install Sona {latest}", "latest", st.Latest)
	}
	meta := theme.Current().Type["meta"]
	col := ui.Column(c.Key("update-notice")).FillWidth().Gap(6).PaddingX(10).Margin(0, 0, 8, 0)
	col.Children(func() {
		ui.Row(c.Key("line")).FillWidth().Gap(8).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).MinWidth(0).Shrink(1).Children(func() {
				ui.Icon(c, theme.Icon("circle-arrow-up")).FontSize(12).TextColor(theme.C("ink")).Shrink(0)
				// The status is the words, not the notice: its button's label
				// is not news. A status is read when its text changes and not
				// when it first appears, so the notice turning up is announced
				// here and what changes it afterwards is its own to read.
				said := ui.Text(c.Key("said"), msg).FontSize(meta.Size).FixedLineHeight(meta.LineHeight).
					TextColor(theme.C("ink")).SingleLine().MinWidth(0).Shrink(1).Role(ui.RoleStatus)
				kit.AnnounceMounted(c, said, msg)
			})
			if st.Installing || st.Installed {
				return
			}
			label := i18n.T("View")
			if st.SelfUpdate {
				label = i18n.T("Install and restart")
			}
			act := ui.ButtonBase(c.Key("update-act")).Radius(theme.Current().Radius["sm"]).Shrink(0).Label(label)
			fg := theme.C("ink-muted")
			if act.Hovered() {
				fg = theme.C("ink")
			}
			act.TextColor(fg).Children(func() {
				t := ui.Text(c, label).FontSize(meta.Size).FixedLineHeight(meta.LineHeight).SingleLine()
				if act.Hovered() {
					t.Underline()
				}
			})
			if act.Clicked() {
				if st.SelfUpdate {
					u.installAndRestart()
				} else {
					u.view()
				}
			}
		})
		if st.Installing {
			value := -1.0 // unknown until the first bytes arrive
			if st.Total > 0 {
				value = float64(st.Downloaded) / float64(st.Total)
			}
			kit.ProgressBar(c.Key("update-progress"), value, kit.ProgressOpts{A11yLabel: i18n.T("Update download")})
		}
	})
}

// installAndRestart installs the update; a failure toasts and the notice
// says so (the status carries it).
func (u *updates) installAndRestart() {
	install := u.install
	if install == nil {
		return
	}
	go func() {
		if err := install(); err != nil {
			kit.ToastErrorf(i18n.T("Couldn't install the update"), i18n.ErrorMessage(err))
		}
	}()
}

// view opens the release page; a failure toasts "Couldn't open the release".
func (u *updates) view() {
	open := u.openRelease
	if open == nil {
		return
	}
	go func() {
		if err := open(); err != nil {
			kit.ToastErrorf(i18n.T("Couldn't open the release"), i18n.ErrorText(services.CodeOf(err)))
		}
	}()
}
