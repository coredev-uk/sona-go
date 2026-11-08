package settings

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// keyPrevious is the version the last update kept (UpdatesService.Previous),
// asked again each time Diagnostics opens.
var keyPrevious = store.Key{"diagnostics", "previous"}

// previousAPI is what the row asks of UpdatesService.
type previousAPI interface {
	Previous() services.PreviousVersion
	RestorePrevious() error
}

// previousState outlives a frame: a restore in flight and why the last one
// failed.
type previousState struct {
	busy    bool
	failure string
}

// previousVersion is Settings → Diagnostics → Updates → Previous version:
// the version the last update replaced, with "Restore…" to go back to it.
// The group isn't there where Sona doesn't update itself (Linux, a
// development build) or while nothing is kept: UpdatesService.Previous says
// so, and the row is not a stub that fails when pressed. A restore under way
// or a failed one keeps the row up.
func (s *settings) previousVersion(c *ui.Context) {
	api := s.diagSeams().previous(s)
	if api == nil {
		return
	}
	q := store.Use(s.env.Queries, keyPrevious, func(context.Context) (services.PreviousVersion, error) {
		return api.Previous(), nil
	})
	st := stateOf[previousState](s, "previous-version")
	if !(q.HasData && q.Data.Available) && !st.busy && st.failure == "" {
		return
	}
	version := q.Data.Version
	desc := i18n.T("The version from your last update is kept.")
	if version != "" {
		desc = i18n.T("{version} is kept from your last update.", "version", version)
	}
	s.group(c, "previous-version", i18n.T("Updates"), "", func() {
		s.row(c, rowOpts{
			ID: "previous-version", Title: i18n.T("Previous version"), Desc: desc,
			Notes: func() {
				if st.failure != "" {
					ui.Column(c.Key("failure")).FillWidth().Margin(8, 0, 0, 0).Children(func() {
						s.alertLine(c, "circle-alert", st.failure)
					})
				}
			},
		}, func() ui.Element {
			label := i18n.T("Restore…")
			if st.busy {
				label = i18n.T("Restoring…")
			}
			b := kit.Button(c.Key("restore"), label, kit.ButtonOpts{
				Variant: kit.Outline, Size: kit.SizeSM, Icon: "rotate-ccw", Spin: st.busy, Disabled: st.busy,
			})
			if b.Clicked() {
				s.askRestore(st, api, version)
			}
			return b
		})
	})
}

// askRestore asks before going back: Sona restarts. The question stays up,
// busy, while the restore runs.
func (s *settings) askRestore(st *previousState, api previousAPI, version string) {
	title := i18n.T("Go back to the previous version?")
	body := i18n.T("Sona restarts on the previous version. Your settings, sign-in and library are kept.")
	if version != "" {
		title = i18n.T("Go back to {version}?", "version", version)
		body = i18n.T("Sona restarts on {version}. Your settings, sign-in and library are kept.", "version", version)
	}
	kit.Confirm(kit.ConfirmOptions{
		Title: title, Body: body, ConfirmLabel: i18n.T("Restore and restart"), BusyLabel: i18n.T("Restoring…"), Hold: true,
	}, func(ok bool) {
		if ok {
			s.restorePrevious(st, api)
		}
	})
}

// restorePrevious puts the previous version back; Sona then restarts, so a
// success leaves the question busy until the window closes. A failure closes
// it and says why in the row.
func (s *settings) restorePrevious(st *previousState, api previousAPI) {
	if st.busy {
		return
	}
	st.busy, st.failure = true, ""
	go func() {
		err := api.RestorePrevious()
		s.env.Post(func() {
			if err == nil {
				return
			}
			st.busy = false
			st.failure = i18n.T("Couldn't restore the previous version. {reason}", "reason", i18n.ErrorMessage(err))
			kit.ConfirmClose()
			s.env.Queries.Invalidate(keyPrevious)
		})
	}()
}
