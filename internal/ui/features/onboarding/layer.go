package onboarding

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The layer's metrics: the card's top edge stays put from step to step at the
// larger of 24 and a tenth of the window, a step taller than the window
// scrolls inside it. Where the window has the room (a card of at least
// railMinPanel, and the rail's height under the head) the card is two panes,
// the step rail beside the step, up to panelWidth wide; otherwise it is the
// narrow card, narrowWidth wide, with the steps as a strip under its head.
const (
	panelWidth   = 800
	railMinPanel = 720
	narrowWidth  = 640
	panelMargin  = 24
	headerHeight = 16 + 32 + 8 + stripHeight // the narrow card's head: padding, the title row, the gap, the strip
	headHeight   = 56                        // the wide card's head: the title row alone
	footHeight   = 64
)

// cardWidth is the card's width on a window w wide, and whether it holds the
// rail (wideLayout also asks for the height).
func cardWidth(w float32) (width float32, rail bool) {
	room := max(w-2*panelMargin, 0)
	if room >= railMinPanel {
		return min(panelWidth, room), true
	}
	return min(narrowWidth, room), false
}

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
	return i18n.T("Welcome to Sona"), i18n.T("Sona plays Apple Music on this computer.")
}

// arrive is a step the layer has not shown yet (an opening, a way on or back,
// a jump from the strip, a sign-in asked for elsewhere): what belongs to
// reaching it. The step's title takes the focus by itself (it is a new
// element), and its body is a new scroll container, so it starts at its top.
func (o *onboarding) arrive(step string) {
	o.shown = step
	o.titleFocus = 0
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
	width, rail := cardWidth(w)
	// The rail needs its height between the head and the foot; a short window
	// takes the strip instead.
	rail = rail && maxH-headHeight-footHeight >= railHeight()
	head := float32(headerHeight)
	if rail {
		head = headHeight
	}
	scrim := theme.C("scrim")

	ui.Overlay(c, func() {
		back := ui.Column(c.Key("setup")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Background(scrim).
			Padding(top, 0, 0, 0).AlignItems(ui.Center).Modal()
		back.Children(func() {
			sh := tk.Shadow["float"]
			panel := ui.Column(c.Key("panel")).Width(width).MaxHeight(maxH).
				Radius(tk.Radius["lg"]).Background(theme.C("raised")).Clip().
				Shadow(sh.X, sh.Y, sh.Blur, sh.Spread, sh.Color).Role(ui.RoleDialog).Label(title).Description(line)
			if edge := theme.C("dialog-border"); edge.A > 0 {
				panel.Border(1, edge)
			}
			panel.Children(func() {
				o.header(c, step, rail)
				o.body(c, step, title, line, max(maxH-head-footHeight, 0), rail)
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

// header is the card's head: the mark, the setup's name and position and
// "Skip setup" (not on the last step). On the narrow card the strip of steps
// is under it; beside the step rail the head is the one row.
func (o *onboarding) header(c *ui.Context, step string, rail bool) {
	if rail {
		ui.Row(c.Key("header")).FillWidth().Height(headHeight).Shrink(0).Padding(0, 24).Gap(8).AlignItems(ui.Center).Children(func() {
			o.headRow(c, step)
		})
		return
	}
	ui.Column(c.Key("header")).FillWidth().Height(headerHeight).Shrink(0).Padding(16, 24, 0, 24).Gap(8).Children(func() {
		ui.Row(c.Key("title-row")).FillWidth().Height(32).Gap(8).AlignItems(ui.Center).Children(func() {
			o.headRow(c, step)
		})
		o.stepStrip(c, step)
	})
}

// headRow is what the head's row holds, from the mark to Skip setup.
func (o *onboarding) headRow(c *ui.Context, step string) {
	// The Sona mark: a record label seen from above, amber with a spindle hole.
	ui.Row(c.Key("mark")).Size(20, 20).Shrink(0).Radius(10).Background(theme.C("amber")).Center().Children(func() {
		ui.Box(c.Key("hole")).Size(4, 4).Radius(2).Background(theme.C("surface"))
	})
	theme.Text(c, i18n.T("Set up Sona"), "ui").FontWeight(600).TextColor(theme.C("ink"))
	theme.Text(c, i18n.T("{position} of {total}", "position", stepIndex(step)+1, "total", len(setupSteps)), "meta").
		TextColor(theme.C("ink-faint")).FontFeatures("tnum")
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
}

// body is the step, beside the step rail when the card has one: its title (the
// dialog's name, which takes the focus when the step is reached), its line and
// the step itself, scrolling when taller than the window.
func (o *onboarding) body(c *ui.Context, step, title, line string, maxHeight float32, rail bool) {
	if !rail {
		o.stepPane(c, step, title, line, maxHeight, false)
		return
	}
	ui.Row(c.Key("main")).FillWidth().Shrink(1).MinHeight(0).MaxHeight(maxHeight).AlignItems(ui.Stretch).Children(func() {
		o.stepRail(c, step)
		o.stepPane(c, step, title, line, maxHeight, true)
	})
}

// stepPane is the scrolling step; beside the rail it takes the rest of the row.
func (o *onboarding) stepPane(c *ui.Context, step, title, line string, maxHeight float32, rail bool) {
	pane := ui.Scroll(c.Key("body:" + step)).Shrink(1).MinHeight(0).MaxHeight(maxHeight)
	if rail {
		pane.Grow(1).Basis(0).MinWidth(0).Padding(20, 28, 28, 28)
	} else {
		pane.FillWidth().Padding(20, 24, 24, 24)
	}
	pane.Children(func() {
		ui.Column(c.Key("content")).FillWidth().Children(func() {
			o.stepHead(c, step, title, line)
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

// stepHead is the step's title and its line. The welcome sets them beside
// the large mark, at the display size.
func (o *onboarding) stepHead(c *ui.Context, step, title, line string) {
	// The title takes the focus: a group named by it holds the heading,
	// because Orca reads a focused heading's role and level only (it takes a
	// heading's words from the text, which MyGo gives to labels).
	heading := func(style string) {
		focus := ui.Column(c.Key("title-focus")).FillWidth().Role(ui.RoleGroup).Label(title).TabStop(false).FocusRing(false)
		focus.Children(func() {
			theme.Text(c.Key("title"), title, style).TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(2)
		})
		kit.FocusWhenShown(c, focus, &o.titleFocus)
	}
	if step == store.SetupWelcome {
		ui.Row(c.Key("lead")).FillWidth().Gap(16).AlignItems(ui.Center).Children(func() {
			bigMark(c)
			ui.Column(c.Key("words")).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
				heading("display")
				theme.Text(c.Key("line"), line, "ui").TextColor(theme.C("ink-muted"))
			})
		})
		return
	}
	heading("page")
	theme.Text(c.Key("line"), line, "ui").TextColor(theme.C("ink-muted")).MaxWidth(480).Margin(6, 0, 0, 0)
}

// bigMark is the Sona mark at the welcome's size: the label's amber disc,
// two grooves inside its edge and the spindle hole. Decorative.
func bigMark(c *ui.Context) {
	ui.Row(c.Key("big-mark")).Size(56, 56).Shrink(0).Radius(28).Background(theme.C("amber")).Center().Children(func() {
		ui.Row(c.Key("groove-outer")).Size(42, 42).Radius(21).Border(1, theme.C("on-amber").Alpha(0.2)).Center().Children(func() {
			ui.Row(c.Key("groove-inner")).Size(28, 28).Radius(14).Border(1, theme.C("on-amber").Alpha(0.2)).Center().Children(func() {
				ui.Box(c.Key("hole")).Size(8, 8).Radius(4).Background(theme.C("raised"))
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
		theme.Hairline(c.Key("line"))
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
