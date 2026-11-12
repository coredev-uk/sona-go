package onboarding

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// welcomeStep is the first step: what the setup covers, the language (once there is one besides
// English to choose), then the two things whole songs need and where each
// stands (the download is already running), so nothing about playback is a
// surprise later (WelcomeStep.tsx).
func (o *onboarding) welcomeStep(c *ui.Context) {
	status := o.cdm()
	view := widevineView(o.capsData(), status)
	signedIn := account.HasAccount(o.authState())
	ui.Column(c.Key("welcome")).FillWidth().Gap(24).Children(func() {
		text(c, i18n.T("A few short steps sign you in and set how it plays, looks and starts. Each can be skipped and changed later in Settings."), "ui").
			TextColor(theme.C("ink")).MaxWidth(480)
		o.languageRow(c)
		stepGroup(c, "needs", i18n.T("What whole songs need"), func() {
			state := i18n.T("Next step")
			if signedIn {
				state = i18n.T("Signed in")
			}
			rows := []func(){func() {
				need(c, i18n.T("Apple Music"), state, signedIn,
					i18n.T("A subscription, signed in to Sona. Browsing works without one."))
			}}
			if view != viewNotNeeded {
				rows = append(rows, func() {
					need(c, i18n.T("Widevine"), widevineShort(view, status), view == viewReady,
						i18n.T("Google's component that plays protected songs. Sona downloads it by itself."))
				})
			}
			list(c, "needs", rows)
		})
	})
}

// need is one thing whole songs need: its name and what it is, then where it
// stands now, with a check once it is done.
func need(c *ui.Context, name, state string, done bool, what string) {
	ui.Row(c.Key("need:"+name)).FillWidth().Gap(24).Padding(10, 0).AlignItems(ui.Start).Justify(ui.SpaceBetween).Children(func() {
		ui.Column(c.Key("words")).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			text(c, name, "ui").FontWeight(500).TextColor(theme.C("ink"))
			text(c, what, "sub").TextColor(theme.C("ink-muted"))
		})
		if state == "" {
			return
		}
		ui.Row(c.Key("state")).Shrink(0).Gap(6).AlignItems(ui.Center).Margin(1, 0, 0, 0).Children(func() {
			if done {
				statusIcon(c, "circle-check", "positive").FontSize(14)
			}
			text(c, state, "sub").TextColor(theme.C("ink"))
		})
	})
}

// automatic is the Language select's value for Automatic (the setting's "").
const automatic = "automatic"

// languageRow is the language, once there is one besides English to choose
// (as in Settings → General): changing it sets the language.
func (o *onboarding) languageRow(c *ui.Context) {
	s := o.settings()
	current := ""
	if s.HasData {
		current = s.Data.Language
	}
	options, ok := o.languageOptions(current)
	if !ok || !i18n.OffersLanguages(options, current) {
		return
	}
	items := []kit.SelectItem{kit.SelectOption(automatic, i18n.T("Automatic"))}
	for _, opt := range options {
		items = append(items, kit.SelectOption(opt.Tag, opt.Name))
	}
	value := current
	if value == config.LanguageAutomatic {
		value = automatic
	}
	chosen := value
	ui.Row(c.Key("language")).FillWidth().Gap(24).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
		ui.Column(c.Key("words")).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			text(c, i18n.T("Language"), "ui").FontWeight(500).TextColor(theme.C("ink"))
			text(c, i18n.T("Automatic follows your Apple Music account and your system."), "sub").TextColor(theme.C("ink-muted"))
		})
		kit.Select(c.Key("select"), &chosen, items, kit.SelectOpts{A11yLabel: i18n.T("Language"), Disabled: !s.HasData})
	})
	if chosen != value {
		if chosen == automatic {
			chosen = config.LanguageAutomatic
		}
		o.setLanguage(chosen)
	}
}

// setLanguage saves the Language setting; Apple's content follows it too
// (l=), so every query refetches in the new language (useSetLanguage): once
// Go has saved it (settings:changed, followed in Init), not before, or the
// refetches would still read the old one.
func (o *onboarding) setLanguage(language string) {
	if s, ok := o.settingsNow(); ok && s.Language == language {
		return
	}
	o.language, o.languageAsked = language, true
	kit.UpdateSettings(o.env.Queries, o.env.Core, func(s *config.Settings) { s.Language = language })
}
