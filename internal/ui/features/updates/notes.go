package updates

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// offered is the update on offer for the notes dialog: a newer release than
// this build, whether or not the notice offers it (Settings → General says
// it is available after the user went back from it, and opens the notes too).
func (u *updates) offered() (services.UpdateStatus, bool) {
	r := store.UseUpdates(u.env.Queries, u.env.Core)
	if !r.HasData || !r.Data.Available {
		return services.UpdateStatus{}, false
	}
	return r.Data, true
}

// dialog is the Layer: the release notes dialog, open while Surfaces says so
// (the row's and Settings' "What's new") and while there is an update to show.
// Where Sona updates itself it ends in "Install and restart", the install the
// row offers (it closes the dialog as the install starts: the row and the bell
// carry its progress), beside "View release"; elsewhere "View release" is its
// one action, which leaves the dialog open behind the browser.
func (u *updates) dialog(c *ui.Context) {
	open := u.env.Surfaces.ReleaseNotesOpen()
	if !open {
		return
	}
	st, ok := u.offered()
	if !ok {
		u.env.Surfaces.CloseReleaseNotes()
		return
	}
	spec := kit.ReleaseNotesSpec{Version: st.Latest, Notes: st.Notes}
	if st.Published > 0 {
		spec.Published = time.UnixMilli(st.Published)
	}
	view := kit.ReleaseNotesAction{Label: i18n.T("View release"), Icon: "external-link", IconAfter: true, Run: u.view}
	switch {
	case st.SelfUpdate && (st.Installing || st.Installed):
		spec.Secondary = view
	case st.SelfUpdate:
		label := i18n.T("Install and restart")
		if st.InstallError != "" {
			label = i18n.T("Try again")
		}
		spec.Secondary = view
		spec.Primary = kit.ReleaseNotesAction{Label: label, Icon: "arrow-down", Run: func() {
			open = false
			u.installAndRestart()
		}}
	default:
		spec.Primary = view
	}
	u.notes.Show(c, &open, spec)
	if !open {
		u.env.Surfaces.CloseReleaseNotes()
	}
}

// openLink opens a link of the notes in the browser (kit.NewMarkdown hands it
// web addresses only); a failure toasts "Couldn't open the link".
func (u *updates) openLink(url string) {
	if u.env.OpenURL == nil {
		return
	}
	if err := u.env.OpenURL(url); err != nil {
		kit.ToastFailure(i18n.T("Couldn't open the link"), err)
	}
}
