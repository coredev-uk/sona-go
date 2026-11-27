// Package updates is a new version in the notification centre: a live row,
// shown only while a release newer than this build exists, with its version,
// and the bell's ink dot while it waits. Go checks for it after the first
// paint (unless turned off in Settings → General) and sends
// "updates:available"; Settings → General shows the same status and checks
// on request (store.UseUpdates). Where Sona updates itself (macOS and Windows
// installs, UpdateStatus.SelfUpdate) the row offers "Install and restart",
// with the download's progress from "updates:install" on the row and the
// bell; elsewhere it links the release page, since updating is the
// platform's: a package manager or an AppImage's own updater. When the
// update has release notes (UpdateStatus.Notes) the row, and Settings →
// General, also offer "What's new", which opens them in a dialog
// (store.Surfaces.OpenReleaseNotes; notes.go) with the install or the link
// to the release page. After an update, the first start also opens the What's
// New dialog with the notes of the version just installed (whatsnew.go).
package updates

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// updates is the feature's state.
type updates struct {
	env *module.Env
	// openRelease opens the release page (UpdatesService.OpenRelease) and
	// install installs the update and restarts (UpdatesService.Install);
	// tests replace them.
	openRelease func() error
	install     func() error
	// notes is the release notes dialog, whose links open through
	// env.OpenURL.
	notes *kit.ReleaseNotes
	// whatsNew is the What's New dialog, and markSeen records, off the main
	// thread, that its version was shown (UpdatesService.MarkWhatsNewSeen;
	// tests replace it).
	whatsNew *kit.ReleaseNotes
	markSeen func(version string)
	// dialogShown says whether another dialog is open (kit.DialogShown when
	// nil; tests replace it).
	dialogShown func() bool
	// said is the row's title last read out ("" while there is no update).
	said string
}

// New is the update notice's module.
func New(env *module.Env) module.Module {
	var open, install func() error
	var markSeen func(string)
	if env.Core != nil {
		svc := services.Service[services.UpdatesService](env.Core)
		open = svc.OpenRelease
		install = func() error { _, err := svc.Install(context.Background()); return err }
		markSeen = func(v string) {
			go func() {
				if err := svc.MarkWhatsNewSeen(v); err != nil {
					env.Log.Warn("updates: recording the What's New dialog as shown", "err", err)
				}
			}()
		}
	}
	u := newUpdates(env, open, install)
	u.markSeen = markSeen
	return u.module()
}

func newUpdates(env *module.Env, open, install func() error) *updates {
	u := &updates{env: env, openRelease: open, install: install}
	u.notes = kit.NewReleaseNotes(u.openLink)
	u.whatsNew = kit.NewReleaseNotes(u.openLink)
	return u
}

func (u *updates) module() module.Module {
	return module.Module{
		ID:    "updates",
		Slots: module.Slots{Activity: &module.Activity{Status: u.status, Build: u.row}, Layer: u.layers}, // dialogs only: no LayerOpen, so the layer takes no input itself
		Init: func() {
			store.Mirror[services.UpdateStatus](u.env.Bus, u.env.Queries, u.env.Post, store.KeyUpdates, nil, services.EventUpdateAvailable, services.EventUpdateInstall)
			store.Mirror[services.WhatsNew](u.env.Bus, u.env.Queries, u.env.Post, store.KeyWhatsNew, nil, services.EventWhatsNew)
		},
	}
}

// current is the update to show, if any: a newer release the user hasn't
// just gone back from (UpdateStatus.Notify).
func (u *updates) current() (services.UpdateStatus, bool) {
	r := store.UseUpdates(u.env.Queries, u.env.Core)
	if !r.HasData || !r.Data.Notify() {
		return services.UpdateStatus{}, false
	}
	return r.Data, true
}

// status is the Activity's Status: news while the update waits, busy while
// it downloads, an alert when installing it failed. The row's title is read
// out as the update turns up and as it changes, once each, whether or not
// the panel is open: the bell is always built.
func (u *updates) status(c *ui.Context) module.ActivityStatus {
	st, ok := u.current()
	if !ok {
		u.said = ""
		return module.ActivityStatus{}
	}
	if h := headline(st); h != u.said {
		u.said = h
		c.Announce(h)
	}
	s := module.ActivityStatus{Shown: true}
	switch {
	case st.Installing:
		s.Busy, s.Progress, s.Label = true, downloaded(st), headline(st)
	case st.Installed:
	case st.InstallError != "":
		s.Alert = true
	default:
		s.News = true
	}
	return s
}

// downloaded is the download's fraction, below 0 until the first bytes.
func downloaded(st services.UpdateStatus) float64 {
	if st.Total > 0 {
		return float64(st.Downloaded) / float64(st.Total)
	}
	return -1
}

// headline is the row's title in each state.
func headline(st services.UpdateStatus) string {
	switch {
	case st.Installed:
		return i18n.T("Restarting into Sona {latest}…", "latest", st.Latest)
	case st.Installing:
		return i18n.T("Downloading Sona {latest}…", "latest", st.Latest)
	case st.InstallError != "":
		return i18n.T("Couldn't install Sona {latest}", "latest", st.Latest)
	}
	return i18n.T("Sona {latest} is available", "latest", st.Latest)
}

// row is the Activity's row: the version, the download's bar and the action.
// After a failed install the same button tries again, and the release page,
// where the installer is, is one click off: a failure that comes from this
// install (a folder it can't write, an update file it can't unpack) comes
// again, and the row must not be a loop of retries.
func (u *updates) row(c *ui.Context) {
	st, ok := u.current()
	if !ok {
		return
	}
	spec := kit.ActivitySpec{Icon: "circle-arrow-up", Title: headline(st)}
	switch {
	case st.Installing:
		p := downloaded(st)
		spec.Progress, spec.ProgressLabel = &p, i18n.T("Update download")
	case st.Installed:
	case st.SelfUpdate && st.InstallError != "":
		spec.Alert = true
		spec.Action, spec.OnAction = i18n.T("Try again"), u.installAndRestart
		spec.Secondary, spec.OnSecondary = i18n.T("View release"), u.view
	case st.SelfUpdate:
		spec.Action, spec.OnAction = i18n.T("Install and restart"), u.installAndRestart
		spec.Secondary, spec.OnSecondary = i18n.T("View release"), u.view
		if st.Notes != "" {
			// The notes' dialog has "View release" too.
			spec.Secondary, spec.OnSecondary = i18n.T("What's new"), u.env.Surfaces.OpenReleaseNotes
		}
	default:
		spec.Detail = i18n.T("Update it the way you installed Sona.")
		spec.Action, spec.OnAction = i18n.T("View"), u.view
		if st.Notes != "" {
			spec.Action, spec.OnAction = i18n.T("What's new"), u.env.Surfaces.OpenReleaseNotes
			spec.Secondary, spec.OnSecondary = i18n.T("View release"), u.view
		}
	}
	kit.ActivityRow(c, "updates:row", spec)
}

// installAndRestart installs the update; a failure toasts and the row says
// so (the status carries it).
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
			kit.ToastFailure(i18n.T("Couldn't open the release"), err)
		}
	}()
}
