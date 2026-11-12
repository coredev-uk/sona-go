package settings

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// cdmState is the Widevine row's own state: the request to put the focus on
// the state output.
type cdmState struct {
	focus bool
}

// The Widevine helpers are v0.2.0's features/cdm (state.ts) with its msgids;
// the shared ones (store.CDMProgress, store.CDMCanRetry, kit.RetryCDMWith,
// i18n.CDMProblem, i18n.FormatBytes…) are used.

// cdmRow is Settings → Playback's Widevine row (CdmRow.tsx): the state in
// a status named by the row's label, a running download's bar, the notes,
// and Try again when a retry can help.
func (s *settings) cdmRow(c *ui.Context) {
	const id = "cdm"
	r := s.cdmStatus()
	title := i18n.T("Widevine")
	desc := i18n.T("Google's Widevine plays Apple Music's protected songs. Sona downloads it from Google and keeps it up to date by itself; an update applies the next time playback starts.")
	known := r.HasData
	status := r.Data
	st := stateOf[cdmState](s, "cdm")

	failed := known && status.State == "failed"
	ready := known && status.State == "ready"
	retry := known && store.CDMCanRetry(&status)
	problem := ""
	if known && status.Code != "" {
		problem = i18n.CDMProblem(status.Code)
	}

	c = c.Key("row:" + id)
	s.seen[id] = true
	ui.Column(c).FillWidth().Children(func() {
		s.line(c)
		in := ui.Row(c.Key("in")).FillWidth().Margin(0, -rowWashInset).Padding(rowPadY, rowWashInset).
			AlignItems(ui.Start).Gap(rowGap).Radius(theme.Current().Radius["md"])
		s.wash(in, id)
		var out ui.Element
		in.Children(func() {
			ui.Column(c.Key("text")).Grow(1).Shrink(1).MinWidth(0).MaxWidth(rowTextWidth).Children(func() {
				theme.Text(c, title, "label").TextColor(theme.C("ink"))
				theme.Text(c, desc, "secondary").TextColor(theme.C("ink-muted")).Margin(2, 0, 0, 0)
				if problem != "" {
					p := problem
					if retry {
						p += " " + i18n.T("Sona also tries again by itself later.")
					}
					theme.Text(c, p, "secondary").TextColor(theme.C("ink")).Margin(8, 0, 0, 0).Role(ui.RoleText).Label(p)
				}
			})
			ui.Column(c.Key("control")).Width(selectWidth).Shrink(0).AlignItems(ui.End).Gap(6).Padding(2, 0, 0, 0).Children(func() {
				o := ui.Row(c.Key("state")).AlignItems(ui.Center).Gap(6).Role(ui.RoleStatus).Label(title).Focusable()
				d := desc
				if problem != "" {
					d += " " + problem
				}
				o.Description(d)
				out = o
				o.Children(func() {
					switch {
					case failed:
						ui.Icon(c, theme.Icon("circle-alert")).FontSize(16).TextColor(theme.C("danger")).Shrink(0)
						theme.Text(c, i18n.CDMFailedState(status.Code), "secondary").TextColor(theme.C("ink")).FontWeight(500)
					case ready:
						ui.Icon(c, theme.Icon("circle-check")).FontSize(16).TextColor(theme.C("positive")).Shrink(0)
						v := i18n.T("Ready")
						if status.Version != "" {
							v = i18n.T("Version {version}", "version", status.Version)
						}
						theme.Text(c, v, "secondary").TextColor(theme.C("ink"))
					case known && status.State == "downloading":
						theme.Text(c, i18n.T("Downloading…"), "secondary").TextColor(theme.C("ink"))
					case known:
						theme.Text(c, i18n.T("Not downloaded yet"), "secondary").TextColor(theme.C("ink-muted"))
					default:
						kit.Bar(c.Key("skeleton"), 128, 12)
					}
				})
				if note := s.cdmNote(status, ready); note != "" {
					theme.Text(c, note, "meta").TextColor(theme.C("ink-faint")).TextAlign(ui.End)
				}
				if known && (status.State == "downloading" || ready && status.Updating) {
					cdmProgressBar(c, status)
				}
				if retry {
					// translators: Reads the sign-in from the system keyring again. Apple's wording: ASE.Web.Music.Error.TryAgain
					b := kit.Button(c.Key("retry"), i18n.T("Try again"), kit.ButtonOpts{
						Variant: kit.Outline, Size: kit.SizeSM, Icon: "rotate-cw",
					}).Margin(4, 0, 0, 0)
					if problem != "" {
						b.Description(problem)
					}
					if b.Clicked() {
						s.retryCDM(st)
					}
				}
			})
			if st.focus {
				st.focus = false
				out.Focus()
			}
		})
		s.land(c, id, in, out)
	})
}

// retryCDM asks Go to download again (kit.RetryCDMWith toasts a failure); the
// focus moves to the state (the button goes away while the download runs).
func (s *settings) retryCDM(st *cdmState) {
	st.focus = true
	kit.RetryCDMWith(s.cdm.Retry, nil)
}

// cdmNote is the line under the state: what happens to an installed copy.
func (s *settings) cdmNote(st services.CDMStatus, ready bool) string {
	switch {
	case ready && st.Updating:
		return i18n.T("Downloading an update…")
	case ready && st.Code != "":
		return i18n.T("Couldn't check for updates")
	case ready && st.CheckedAt > 0:
		return i18n.T("Checked for updates {checked}", "checked", i18n.RelativeTime(stampTime(st.CheckedAt), s.now()))
	}
	return ""
}

// cdmProgressBar is a download's bar: its value is the bar's own, the text
// beside it decoration. Unknown size: an empty bar.
func cdmProgressBar(c *ui.Context, st services.CDMStatus) {
	label := i18n.T("Widevine download")
	if st.Updating {
		label = i18n.T("Widevine update")
	}
	f, ok := store.CDMProgress(st)
	ui.Column(c.Key("progress")).Width(selectWidth).Gap(4).Children(func() {
		if !ok {
			kit.ProgressBar(c, -1, kit.ProgressOpts{A11yLabel: label})
			return
		}
		percent := i18n.FormatPercent(f)
		received, total := i18n.FormatBytes(st.Received), i18n.FormatBytes(st.Total)
		kit.ProgressBar(c, f, kit.ProgressOpts{
			A11yLabel: label,
			ValueText: i18n.T("{percent}, {received} of {total}", "percent", percent, "received", received, "total", total),
		})
		theme.Text(c, i18n.T("{percent} · {received} of {total}", "percent", percent, "received", received, "total", total), "meta").
			TextColor(theme.C("ink-faint")).TextAlign(ui.End).Role(ui.RoleNone)
	})
}
