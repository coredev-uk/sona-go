package onboarding

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// goToStep moves the open setup to a step.
func (o *onboarding) goToStep(step string) { o.env.Surfaces.SetSetupStep(step) }

// doneStep is the last step: what is still to do (signing in, a Widevine
// download that hasn't finished), each with the way back to its step, or that
// everything is ready. Reaching it marks the setup finished (the layer does,
// once per arrival); Done closes it (DoneStep.tsx).
func (o *onboarding) doneStep(c *ui.Context) {
	signedIn := account.HasAccount(o.authState())
	status := o.cdm()
	view := widevineView(o.capsData(), status)
	widevine := widevineShort(view, status)
	pending := view == viewWaiting || view == viewDownloading
	failed := view == viewFailed
	noPlayback := view == viewUnavailable || view == viewUnsupported

	if !signedIn || pending || failed {
		stepGroup(c, "left", i18n.T("Still to do"), func() {
			var rows []func()
			if !signedIn {
				rows = append(rows, func() {
					o.leftRow(c, "account", i18n.T("Apple Music"), store.SetupAccount, i18n.T("Sign in"),
						i18n.T("Not signed in. Signing in brings your library and plays whole songs."))
				})
			}
			if pending {
				rows = append(rows, func() {
					o.leftRow(c, "widevine", i18n.T("Widevine"), store.SetupPlayback, i18n.T("Go to Playback"),
						i18n.T("{widevine}. Songs play once it's ready; it carries on in the background.", "widevine", widevine))
				})
			}
			if failed {
				rows = append(rows, func() {
					o.leftRow(c, "widevine", i18n.T("Widevine"), store.SetupPlayback, i18n.T("Go to Playback"),
						i18n.T("{widevine}. Sona tries again by itself later.", "widevine", widevine))
				})
			}
			list(c, "left", rows)
		})
		return
	}
	line := i18n.T("Ready to play")
	if noPlayback {
		line = i18n.T("Playback isn't available here yet")
	}
	well(c.Key("ready")).Row().Gap(12).AlignItems(ui.Start).Children(func() {
		statusIcon(c, "circle-check", "positive").Margin(2, 0, 0, 0)
		ui.Column(c.Key("words")).Children(func() {
			theme.Text(c, line, "ui").FontWeight(500).TextColor(theme.C("ink"))
			theme.Text(c, i18n.T("Your library syncs in the background."), "sub").TextColor(theme.C("ink-muted"))
		})
	})
}

// leftRow is something the setup left undone: what, its state, and the way
// back to its step.
func (o *onboarding) leftRow(c *ui.Context, id, title, step, action, what string) {
	ui.Row(c.Key("left:"+id)).FillWidth().Gap(24).Padding(12, 0).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
		ui.Column(c.Key("words")).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			theme.Text(c, title, "ui").FontWeight(500).TextColor(theme.C("ink"))
			theme.Text(c, what, "sub").TextColor(theme.C("ink-muted"))
		})
		if kit.Button(c.Key("go"), action, kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM}).Clicked() {
			o.goToStep(step)
		}
	})
}
