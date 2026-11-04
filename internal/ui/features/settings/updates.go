package settings

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// updateState outlives a frame: a manual check in flight and its failure.
type updateState struct {
	checking bool
	failure  string
}

// verdict is what the update check says: a line, a detail under it, and
// whether it is good news (+1), bad (-1) or neither (0).
type verdict struct {
	text, detail string
	ok           int
}

// updateVerdict words an update status (Updates.tsx verdict).
func (s *settings) updateVerdict(st services.UpdateStatus) verdict {
	switch {
	case !st.Checkable:
		if st.Current != "" {
			return verdict{text: i18n.T("Development build ({current}): not checked for updates", "current", st.Current)}
		}
		return verdict{text: i18n.T("Development build (no version): not checked for updates")}
	case st.Available:
		d := i18n.T("Download the new package from the release page, or update it with your package manager.")
		if st.AppImage {
			d = i18n.T("Update this AppImage in place with AppImageUpdate, or download the new one from the release page.")
		}
		return verdict{text: i18n.T("Sona {latest} is available", "latest", st.Latest), detail: d}
	case st.Error != "":
		return verdict{text: i18n.T("Couldn't check GitHub for updates"), detail: i18n.ErrorText(st.Error), ok: -1}
	case st.Checked == 0:
		return verdict{text: i18n.T("Sona {current} · not checked yet", "current", st.Current)}
	}
	checked := i18n.RelativeTime(stampTime(st.Checked), s.now())
	text := i18n.T("Sona {current} is up to date", "current", st.Current)
	if st.Latest == "" {
		text = i18n.T("Sona {current} · no release published yet", "current", st.Current)
	}
	return verdict{text: text, detail: i18n.T("Checked {checked}.", "checked", checked), ok: 1}
}

// updatesGroup is the start-up update check and its latest result.
func (s *settings) updatesGroup(c *ui.Context, cfg config.Settings) {
	s.group(c, "general-updates", i18n.T("Updates"), "", func() {
		s.switchRow(c, rowOpts{
			ID: "updates-check", Title: i18n.T("Check for updates when Sona starts"),
			Desc: i18n.T("Asks GitHub for the newest Sona release once the window has opened. Nothing about you, your account or your library is sent."),
		}, cfg.Updates.Check, false, func(on bool) {
			s.save(func(st *config.Settings) { st.Updates.Check = on })
		})
		if st := s.updateStatus(); st.HasData {
			s.updateStatusRow(c, st.Data)
		}
	})
}

// updateStatusRow is the verdict with View release and Check now.
func (s *settings) updateStatusRow(c *ui.Context, st services.UpdateStatus) {
	us := stateOf[updateState](s, "updates")
	v := s.updateVerdict(st)
	c = c.Key("update-status")
	s.line(c)
	ui.Row(c).FillWidth().Padding(rowPadY, 0).AlignItems(ui.Start).Justify(ui.SpaceBetween).Gap(rowGap).
		Role(ui.RoleStatus).Label(i18n.T("Updates")).Children(func() {
		ui.Column(c.Key("text")).Shrink(1).MinWidth(0).Children(func() {
			ui.Row(c.Key("line")).Gap(8).AlignItems(ui.Start).Children(func() {
				if v.ok > 0 {
					ui.Icon(c, theme.Icon("circle-check")).FontSize(16).TextColor(theme.C("positive")).Margin(1, 0, 0, 0).Shrink(0)
				}
				if v.ok < 0 || us.failure != "" {
					ui.Icon(c, theme.Icon("circle-alert")).FontSize(16).TextColor(theme.C("danger")).Margin(1, 0, 0, 0).Shrink(0)
				}
				line := v.text
				if us.failure != "" {
					line = i18n.T("Couldn't check: {failure}", "failure", us.failure)
				}
				t := text(c, line, "secondary").TextColor(theme.C("ink")).Shrink(1).MinWidth(0)
				if st.Available {
					t.FontWeight(500)
				}
			})
			if v.detail != "" && us.failure == "" {
				text(c, v.detail, "secondary").TextColor(theme.C("ink-muted")).Margin(4, 0, 0, 0).MaxWidth(448)
			}
		})
		ui.Row(c.Key("actions")).Shrink(0).Gap(8).Children(func() {
			if st.Available {
				if kit.Button(c.Key("release"), i18n.T("View release"), kit.ButtonOpts{Size: kit.SizeSM, Icon: "external-link"}).Clicked() {
					s.openRelease()
				}
			}
			if st.Checkable {
				label := i18n.T("Check now")
				if us.checking {
					label = i18n.T("Checking…")
				}
				if kit.Button(c.Key("check"), label, kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Icon: "rotate-cw", Disabled: us.checking}).Clicked() {
					s.checkNow(us)
				}
			}
		})
	})
}

// openRelease opens the release page; failure toasts.
func (s *settings) openRelease() {
	u := s.updates
	go func() {
		if err := u.OpenRelease(); err != nil {
			s.env.Post(func() { kit.ToastErrorf(i18n.T("Couldn't open the release"), i18n.ErrorMessage(err)) })
		}
	}()
}

// checkNow runs the check; its result replaces the cached status.
func (s *settings) checkNow(us *updateState) {
	us.checking, us.failure = true, ""
	u := s.updates
	go func() {
		st, err := u.Check(context.Background())
		s.env.Post(func() {
			us.checking = false
			if err != nil {
				us.failure = i18n.ErrorMessage(err)
				s.env.Queries.Invalidate(store.KeyUpdates)
				return
			}
			store.Set(s.env.Queries, store.KeyUpdates, st)
		})
	}()
}
