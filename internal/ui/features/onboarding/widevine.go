package onboarding

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// widevineBlock is Widevine at the head of the Playback step, in one block:
// its state (a status, so a change is announced once), the download's bar
// while it runs, one line on what it means, and Try again after a failure
// Sona can retry. The download is Sona's own, started after the first paint
// (no consent step). Without playback on this platform it says so; where the
// engine needs no CDM it says nothing (WidevineStatus.tsx).
func (o *onboarding) widevineBlock(c *ui.Context) {
	status := o.cdm()
	view := widevineView(o.capsData(), status)
	if view == viewNotNeeded {
		return
	}
	c = c.Key("widevine")

	version := ""
	if status != nil {
		version = status.Version
	}
	var (
		icon, colour string
		spin         bool
		state        string
		detail       = i18n.T("Google's component that plays Apple Music's protected songs. Sona downloads it, checks Google's signature and keeps it up to date.")
	)
	switch view {
	case viewUnavailable:
		icon, colour = "circle-slash", "ink-muted"
		state = i18n.T("Playback isn't available on this platform yet")
		detail = i18n.T("Browsing Apple Music and your library works as usual.")
	case viewUnsupported:
		icon, colour = "circle-slash", "ink-muted"
		state = i18n.T("Playback isn't available on this computer")
		detail = i18n.CDMProblem("unsupported_platform")
	case viewWaiting:
		icon, colour, spin = "loader-circle", "ink-muted", true
		state = i18n.T("Starting Widevine's download…")
	case viewDownloading:
		icon, colour, spin = "loader-circle", "ink-muted", true
		state = i18n.T("Downloading Widevine…")
		detail = i18n.T("Songs play once it's ready. The download carries on if you continue.")
	case viewReady:
		icon, colour = "circle-check", "positive"
		state = i18n.T("Widevine is ready")
		if version != "" {
			state = i18n.T("Widevine {version} is ready", "version", version)
		}
	case viewFailed:
		icon, colour = "circle-alert", "danger"
		if status != nil {
			state = i18n.CDMFailedState(status.Code)
			detail = i18n.CDMProblem(status.Code)
		} else {
			detail = ""
		}
	default: // loading
		detail = ""
	}
	retry := view == viewFailed && store.CDMCanRetry(status)
	if retry && detail != "" {
		detail += " " + i18n.T("Sona also tries again by itself later.")
	}

	well(c).Children(func() {
		ui.Row(c.Key("head")).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
			out := ui.Row(c.Key("state")).Grow(1).Shrink(1).MinWidth(0).Gap(8).AlignItems(ui.Center).
				Role(ui.RoleStatus).TabStop(false).FocusRing(false)
			if state != "" {
				out.Label(state).Description(detail)
			}
			if o.focusState {
				o.focusState = false
				out.Focus()
			}
			out.Children(func() {
				if icon != "" {
					ic := statusIcon(c, icon, colour)
					theme.Spin(ic, spin)
				}
				if state == "" {
					kit.Skeleton(c, 160, 12)
					return
				}
				theme.Text(c, state, "ui").FontWeight(500).TextColor(theme.C("ink")).Shrink(1).MinWidth(0)
			})
			if retry {
				label := i18n.T("Try again")
				if o.retrying {
					label = i18n.T("Retrying…")
				}
				if kit.Button(c.Key("retry"), label, kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Icon: "rotate-cw", Disabled: o.retrying}).Clicked() {
					o.retryCDM()
				}
			}
		})
		if view == viewDownloading && status != nil {
			o.downloadBar(c, *status)
		}
		if detail != "" {
			theme.Text(c.Key("detail"), detail, "sub").TextColor(theme.C("ink-muted")).MaxWidth(480).Margin(4, 0, 0, 24)
		}
	})
}

// downloadBar is the download's bar: the value is the bar's own, the figures
// beside it decoration.
func (o *onboarding) downloadBar(c *ui.Context, s services.CDMStatus) {
	fraction, known := store.CDMProgress(s)
	percent := cdmPercent(s)
	received, total := i18n.FormatBytes(s.Received), i18n.FormatBytes(s.Total)
	value, valueText, figures := -1.0, "", i18n.T("Starting…")
	if known {
		value = fraction
		valueText = i18n.T("{percent}, {received} of {total}", "percent", percent, "received", received, "total", total)
		figures = i18n.T("{percent} · {received} of {total}", "percent", percent, "received", received, "total", total)
	}
	ui.Row(c.Key("bar")).FillWidth().Gap(12).AlignItems(ui.Center).Margin(10, 0, 0, 24).Children(func() {
		kit.ProgressBar(c, value, kit.ProgressOpts{A11yLabel: i18n.T("Widevine download"), ValueText: valueText, Height: 6}).
			Grow(1).Shrink(1).MinWidth(0)
		theme.Text(c, figures, "meta").TextColor(theme.C("ink-faint")).SingleLine().Shrink(0)
	})
}

// retryCDM asks Go to download Widevine (or look for an update) now, through
// the shared kit.RetryCDMWith (which toasts a failure): Try again reads
// "Retrying…" and is disabled while the call runs, and the focus moves to the
// state, so it stays put when the button goes.
func (o *onboarding) retryCDM() {
	if o.retrying {
		return
	}
	o.retrying, o.focusState = true, true
	kit.RetryCDMWith(o.cdmSvc.Retry, func() { o.env.Post(func() { o.retrying = false }) })
}
