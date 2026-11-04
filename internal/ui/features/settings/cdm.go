package settings

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// cdmState is the Widevine row's own state: a Retry in flight and the
// request to put the focus on the state output.
type cdmState struct {
	retrying bool
	focus    bool
}

// The Widevine helpers below are small local copies of v0.2.0's
// features/cdm (state.ts) and shared/lib/format.ts, with its msgids; the
// shared ones (store.CDMProgress, i18n.CDMProblem…) are used where they
// exist.

// cdmProgress is the running download's progress from 0 to 1; false while
// its size is unknown.
func cdmProgress(st services.CDMStatus) (float64, bool) {
	if st.Total <= 0 {
		return 0, false
	}
	return min(max(float64(st.Received)/float64(st.Total), 0), 1), true
}

// canRetryCDM reports whether trying again can help (canRetryCdm).
func canRetryCDM(st services.CDMStatus) bool {
	return st.Code != "" && st.Code != "unsupported_platform" &&
		(st.State == "failed" || st.State == "ready") && !st.Updating
}

// formatBytes is bytes as "12.4 MB" in the shown language's numbers.
func formatBytes(n int64) string {
	if n < 1024 {
		return i18n.FormatNumber(float64(n)) + " B"
	}
	v, i := float64(n)/1024, 0
	units := []string{"KB", "MB", "GB", "TB"}
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if v < 10 {
		return fmt.Sprintf("%s %s", fmtFixed(v, 1), units[i])
	}
	return fmt.Sprintf("%s %s", fmtFixed(v, 0), units[i])
}

func fmtFixed(v float64, digits int) string {
	if digits == 0 {
		return i18n.FormatNumber(float64(int64(v + 0.5)))
	}
	return i18n.FormatNumber(float64(int64(v*10+0.5)) / 10)
}

// relativeTime is a past time as "2 hr. ago" (shared/lib/format.ts).
func relativeTime(stamp int64, now time.Time) string {
	if stamp <= 0 {
		return i18n.T("never")
	}
	sec := int(now.Sub(time.UnixMilli(stamp)).Seconds())
	if sec < 45 {
		return i18n.T("just now")
	}
	m := (sec + 30) / 60
	if m < 60 {
		return i18n.T("{minutes} min. ago", "minutes", m)
	}
	h := (m + 30) / 60
	if h < 24 {
		return i18n.T("{hours} hr. ago", "hours", h)
	}
	return i18n.T("{days, plural, one {# day ago} other {# days ago}}", "days", (h+12)/24)
}

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
	retry := known && canRetryCDM(status)
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
				text(c, title, "label").TextColor(theme.C("ink"))
				text(c, desc, "secondary").TextColor(theme.C("ink-muted")).Margin(2, 0, 0, 0)
				if problem != "" {
					p := problem
					if retry {
						p += " " + i18n.T("Sona also tries again by itself later.")
					}
					text(c, p, "secondary").TextColor(theme.C("ink")).Margin(8, 0, 0, 0).Role(ui.RoleText).Label(p)
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
						text(c, i18n.CDMFailedState(status.Code), "secondary").TextColor(theme.C("ink")).FontWeight(500)
					case ready:
						ui.Icon(c, theme.Icon("circle-check")).FontSize(16).TextColor(theme.C("positive")).Shrink(0)
						v := i18n.T("Ready")
						if status.Version != "" {
							v = i18n.T("Version {version}", "version", status.Version)
						}
						text(c, v, "secondary").TextColor(theme.C("ink"))
					case known && status.State == "downloading":
						text(c, i18n.T("Downloading…"), "secondary").TextColor(theme.C("ink"))
					case known:
						text(c, i18n.T("Not downloaded yet"), "secondary").TextColor(theme.C("ink-muted"))
					default:
						ui.Box(c.Key("skeleton")).Width(128).Height(12).Radius(theme.Current().Radius["sm"]).Background(theme.C("hover"))
					}
				})
				if note := s.cdmNote(status, ready); note != "" {
					text(c, note, "meta").TextColor(theme.C("ink-faint")).TextAlign(ui.End)
				}
				if known && (status.State == "downloading" || ready && status.Updating) {
					cdmProgressBar(c, status)
				}
				if retry {
					label := i18n.T("Try again")
					if st.retrying {
						label = i18n.T("Retrying…")
					}
					b := kit.Button(c.Key("retry"), label, kit.ButtonOpts{
						Variant: kit.Outline, Size: kit.SizeSM, Icon: "rotate-cw", Disabled: st.retrying,
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

// retryCDM asks Go to download again; the focus moves to the state (the
// button goes away while the download runs).
func (s *settings) retryCDM(st *cdmState) {
	st.retrying, st.focus = true, true
	go func() {
		err := s.cdm.Retry()
		s.env.Post(func() {
			st.retrying = false
			if err != nil {
				kit.ToastErrorf(i18n.T("Couldn't start the Widevine download"), i18n.ErrorText(services.CodeOf(err)))
			}
		})
	}()
}

// cdmNote is the line under the state: what happens to an installed copy.
func (s *settings) cdmNote(st services.CDMStatus, ready bool) string {
	switch {
	case ready && st.Updating:
		return i18n.T("Downloading an update…")
	case ready && st.Code != "":
		return i18n.T("Couldn't check for updates")
	case ready && st.CheckedAt > 0:
		return i18n.T("Checked for updates {checked}", "checked", relativeTime(st.CheckedAt, s.now()))
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
	f, ok := cdmProgress(st)
	ui.Column(c.Key("progress")).Width(selectWidth).Gap(4).Children(func() {
		if !ok {
			kit.ProgressBar(c, -1, kit.ProgressOpts{A11yLabel: label})
			return
		}
		percent := i18n.FormatPercent(f)
		received, total := formatBytes(st.Received), formatBytes(st.Total)
		kit.ProgressBar(c, f, kit.ProgressOpts{
			A11yLabel: label,
			ValueText: i18n.T("{percent}, {received} of {total}", "percent", percent, "received", received, "total", total),
		})
		text(c, i18n.T("{percent} · {received} of {total}", "percent", percent, "received", received, "total", total), "meta").
			TextColor(theme.C("ink-faint")).TextAlign(ui.End).Role(ui.RoleNone)
	})
}
