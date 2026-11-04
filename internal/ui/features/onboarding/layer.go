package onboarding

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The layer's metrics (SetupLayer.tsx's classes): the panel is 40 rem wide,
// its top edge stays put from step to step at the larger of 24 and a tenth of
// the window, a step taller than the window scrolls inside it.
const (
	panelWidth   = 640
	panelMargin  = 24
	headerHeight = 16 + 32 + 8 + stripHeight // padding, the title row, the gap, the strip
	footHeight   = 64
)

// stepText is each step's title and the one line under it.
func stepText(step string, signedIn bool) (title, line string) {
	switch step {
	case store.SetupAccount:
		return stepName(step), i18n.T("Signing in brings your library, playlists and recommendations, and plays whole songs. Sona keeps the sign-in in your keyring and never saves your password.")
	case store.SetupPlayback:
		return stepName(step), i18n.T("What plays your music and how it sounds.")
	case store.SetupConnections:
		return stepName(step), i18n.T("Optional. Keep a record of what you play, or show it on Discord.")
	case store.SetupAppearance:
		return stepName(step), i18n.T("Each choice applies at once, to the app behind.")
	case store.SetupDesktop:
		return stepName(step), i18n.T("How Sona starts, and what it does in the background.")
	case store.SetupDone:
		title = i18n.T("Setup is done")
		if signedIn {
			title = i18n.T("You're all set")
		}
		return title, i18n.T("Settings has all of this, and Settings → General runs the setup again.")
	}
	return i18n.T("Welcome to Sona"), i18n.T("Sona plays Apple Music on this computer. A few short steps sign you in and set how it plays, looks and starts. Each can be skipped and changed later in Settings.")
}

// arrive is a step the layer has not shown yet (an opening, a way on or back,
// a jump from the strip, a sign-in asked for elsewhere): what belongs to
// reaching it. The step's title takes the focus by itself (it is a new
// element), and its body is a new scroll container, so it starts at its top.
func (o *onboarding) arrive(step string) {
	o.shown = step
	o.sec = nil // the sign-in form starts afresh
	o.lb, o.lbForm = false, tokenForm{}
	o.fadeHeld = false
	if step == store.SetupDone {
		o.complete() // saved once: a later run finds the setup already marked
	}
}

// layer is the first-run setup: a modal sheet over the live app (a module
// layer, so it covers the sidebar, top bar and player bar, which stay inert
// behind the scrim), sized to its step. Its frame: where the setup is (a strip
// of the steps), the step's title and line, the step, and its way on along the
// foot. Every step can be skipped ("Not now", Continue, a later step in the
// strip); "Skip setup" ends it at once, and Escape asks first. Opened by Go on
// a first start (/welcome, services.StartRoute), its ?step= links and
// Settings → General's "Set up Sona again" (SetupLayer.tsx).
func (o *onboarding) layer(c *ui.Context) {
	surf := o.env.Surfaces
	if !surf.SetupOpen() {
		if o.shown != "" {
			o.shown, o.sec = "", nil
			o.lb, o.lbForm = false, tokenForm{}
		}
		return
	}
	step := surf.SetupStep()
	if step != o.shown {
		o.arrive(step)
	}
	signedIn := account.HasAccount(o.authState())
	title, line := stepText(step, signedIn)

	tk := theme.Current()
	w, h := c.Size()
	top := max(panelMargin, h/10)
	maxH := max(h-top-panelMargin, 0)
	scrim := ui.RGBA(0, 0, 0, 0.5)
	if theme.Dark() {
		scrim = ui.RGBA(0, 0, 0, 0.7)
	}

	ui.Overlay(c, func() {
		back := ui.Column(c.Key("setup")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Background(scrim).
			Padding(top, 0, 0, 0).AlignItems(ui.Center).Modal()
		back.Children(func() {
			sh := tk.Shadow["float"]
			panel := ui.Column(c.Key("panel")).Width(min(panelWidth, max(w-2*panelMargin, 0))).MaxHeight(maxH).
				Radius(tk.Radius["lg"]).Background(theme.C("raised")).Clip().
				Shadow(sh.X, sh.Y, sh.Blur, sh.Spread, sh.Color).Role(ui.RoleDialog).Label(title).Description(line)
			if theme.Dark() {
				panel.Border(1, theme.C("white").Alpha(0.1))
			}
			panel.Children(func() {
				o.header(c, step)
				o.body(c, step, title, line, max(maxH-headerHeight-footHeight, 0))
				o.foot(c, step, signedIn)
			})
		})
		// A press on the scrim does nothing; Escape on the last step is Done,
		// elsewhere it asks first.
		if back.OverlayShortcut(0, ui.KeyEscape) {
			if step == store.SetupDone {
				o.leave()
			} else {
				o.askToSkip()
			}
		}
	})
}

// askToSkip asks before Escape ends the setup; what was chosen so far is kept.
func (o *onboarding) askToSkip() {
	kit.Confirm(kit.ConfirmOptions{
		Title:        i18n.T("Skip the rest of the setup?"),
		Body:         i18n.T("What you've chosen so far is kept. Settings → General runs the setup again."),
		ConfirmLabel: i18n.T("Skip setup"),
	}, func(skip bool) {
		if skip {
			o.leave()
		}
	})
}

// header is the mark, the setup's name and position, "Skip setup" (not on the
// last step) and the strip of steps.
func (o *onboarding) header(c *ui.Context, step string) {
	ui.Column(c.Key("header")).FillWidth().Height(headerHeight).Shrink(0).Padding(16, 24, 0, 24).Gap(8).Children(func() {
		ui.Row(c.Key("title-row")).FillWidth().Height(32).Gap(8).AlignItems(ui.Center).Children(func() {
			// The Sona mark: a record label seen from above, amber with a spindle hole.
			ui.Row(c.Key("mark")).Size(20, 20).Shrink(0).Radius(10).Background(theme.C("amber")).Center().Children(func() {
				ui.Box(c.Key("hole")).Size(4, 4).Radius(2).Background(theme.C("surface"))
			})
			text(c, i18n.T("Set up Sona"), "ui").FontWeight(600).TextColor(theme.C("ink"))
			text(c, i18n.T("{position} of {total}", "position", stepIndex(step)+1, "total", len(setupSteps)), "meta").
				TextColor(theme.C("ink-faint"))
			ui.Box(c.Key("spacer")).Grow(1)
			if step != store.SetupDone {
				skip := kit.Button(c.Key("skip"), i18n.T("Skip setup"), kit.ButtonOpts{
					Variant: kit.Ghost, Size: kit.SizeSM, Muted: true,
					Tip: i18n.T("Ends the setup; Settings → General opens it again."),
				}).Margin(0, -10, 0, 0)
				if skip.Clicked() {
					o.leave()
				}
			}
		})
		o.stepStrip(c, step)
	})
}

// body is the step: its title (the dialog's name, which takes the focus when
// the step is reached), its line and the step itself, scrolling when taller
// than the window.
func (o *onboarding) body(c *ui.Context, step, title, line string, maxHeight float32) {
	ui.Scroll(c.Key("body:"+step)).FillWidth().Shrink(1).MinHeight(0).MaxHeight(maxHeight).
		Padding(20, 24, 24, 24).Children(func() {
		ui.Column(c.Key("content")).FillWidth().Children(func() {
			text(c.Key("title"), title, "page").TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(2).
				Focusable().FocusRing(false).AutoFocus()
			text(c.Key("line"), line, "ui").TextColor(theme.C("ink-muted")).MaxWidth(480).Margin(6, 0, 0, 0)
			ui.Column(c.Key("step")).FillWidth().Margin(20, 0, 0, 0).Children(func() {
				switch step {
				case store.SetupAccount:
					o.accountStep(c)
				case store.SetupPlayback:
					o.playbackStep(c)
				case store.SetupConnections:
					o.connectionsStep(c)
				case store.SetupAppearance:
					o.appearanceStep(c)
				case store.SetupDesktop:
					o.desktopStep(c)
				case store.SetupDone:
					o.doneStep(c)
				default:
					o.welcomeStep(c)
				}
			})
		})
	})
}

// foot is the way on along the bottom: Back (not on the welcome) and the
// forward button: "Get started", "Continue", "Done" (which leaves), or on the
// Apple Music step while signed out the ghost "Not now" with the sign-in
// form's own Sign in beside it as the primary action.
func (o *onboarding) foot(c *ui.Context, step string, signedIn bool) {
	ui.Column(c.Key("foot")).FillWidth().Height(footHeight).Shrink(0).Children(func() {
		ui.Box(c.Key("line")).Height(1).FillWidth().Background(theme.C("line"))
		ui.Row(c.Key("row")).FillWidth().Grow(1).Padding(0, 24).Gap(12).AlignItems(ui.Center).Children(func() {
			if step != store.SetupWelcome {
				back := kit.Button(c.Key("back"), i18n.T("Back"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Icon: "chevron-left"}).
					Margin(0, 0, 0, -10)
				if back.Clicked() {
					o.goToStep(stepBefore(step))
				}
			}
			ui.Box(c.Key("spacer")).Grow(1)
			next := func(label string, v kit.ButtonVariant) {
				if kit.Button(c.Key("forward"), label, kit.ButtonOpts{Variant: v, Size: kit.SizeSM}).Clicked() {
					o.goToStep(stepAfter(step))
				}
			}
			switch {
			case step == store.SetupWelcome:
				next(i18n.T("Get started"), kit.Default)
			case step == store.SetupAccount && !signedIn:
				// "Not now" is the way past the sign-in; its own Sign in lands beside it.
				next(i18n.T("Not now"), kit.Ghost)
				o.section().FootAction(c)
			case step == store.SetupDone:
				if kit.Button(c.Key("forward"), i18n.T("Done"), kit.ButtonOpts{Size: kit.SizeSM}).Clicked() {
					o.leave()
				}
			default:
				next(i18n.T("Continue"), kit.Default)
			}
		})
	})
}
