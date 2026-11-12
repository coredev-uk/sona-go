package onboarding

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// accountStep is the Apple Music step: the account dialog's own sign-in (the
// native form, then the browser and token fallbacks, with its keyring
// notices; its Sign in sits in the setup's foot as the step's primary
// action), or, once signed in, a line saying so. Nothing about the account
// itself is shown here (AccountStep.tsx).
func (o *onboarding) accountStep(c *ui.Context) {
	state := o.authState()
	if !account.HasAccount(state) {
		o.section().Build(c, true)
		return
	}
	line := i18n.T("Signed in to Apple Music")
	if state == "validating" {
		line = i18n.T("Checking your sign-in…")
	}
	well(c.Key("signed-in")).Row().Gap(12).AlignItems(ui.Start).Children(func() {
		statusIcon(c, "circle-check", "positive").Margin(2, 0, 0, 0)
		ui.Column(c.Key("words")).Children(func() {
			theme.Text(c, line, "ui").FontWeight(500).TextColor(theme.C("ink"))
			theme.Text(c, i18n.T("Your library syncs in the background while you carry on."), "sub").TextColor(theme.C("ink-muted"))
		})
	})
}

// section is the step's sign-in section, made when the step is reached
// (AccountSection mounts with the step, so its form starts afresh each time).
func (o *onboarding) section() *account.Section {
	if o.sec == nil {
		o.sec = account.New(o.deps)
	}
	return o.sec
}
