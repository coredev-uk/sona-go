package onboarding

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Brand names, never translated.
const (
	nameLastFM       = "Last.fm"
	nameListenBrainz = "ListenBrainz"
)

// listenBrainzTokenPage is where a ListenBrainz user token is copied from.
const listenBrainzTokenPage = "https://listenbrainz.org/settings/"

// tokenForm is the ListenBrainz form's state: the text, a check in flight
// and what the last attempt said.
type tokenForm struct {
	token string
	busy  bool
	err   string
}

// errorLine is what went wrong connecting a scrobbler, in a sentence
// (scrobble.ts' errorLine); "" when the user cancelled.
func errorLine(listenBrainz bool, err error) string {
	switch services.CodeOf(err) {
	case services.CodeCancelled:
		return ""
	case services.CodeInvalidCredentials:
		if listenBrainz {
			return i18n.T("ListenBrainz didn't accept that token. Copy it again from your ListenBrainz settings.")
		}
		return i18n.T("Last.fm refused Sona's request. Try connecting again.")
	case services.CodeTimeout:
		return i18n.T("The Last.fm approval expired before it was confirmed. Connect again to get a new one.")
	case services.CodeNetwork:
		name := nameLastFM
		if listenBrainz {
			name = nameListenBrainz
		}
		return i18n.T("Couldn't reach {name}. Check your connection and try again.", "name", name)
	case services.CodeNotConfigured:
		return i18n.T("This build of Sona has no Last.fm API key, so it can't connect to Last.fm.")
	}
	return i18n.ErrorMessage(err)
}

// connectionsStep is Connections, all optional: Last.fm and ListenBrainz
// scrobbling (the same browser approval and token form as Settings →
// Connections) and Discord status. Once connected, a service says so;
// pausing and disconnecting stay in Settings (ConnectionsStep.tsx).
func (o *onboarding) connectionsStep(c *ui.Context) {
	st := o.scrobbleStatus()
	settings := o.settings()
	integ := o.integrationsStatus()
	discordShown := false
	if caps := o.capsData(); caps != nil {
		discordShown = caps.DiscordPresence
	}
	privateOn := false
	if p := store.UsePrivate(o.env.Queries, o.env.Core); p.HasData {
		privateOn = p.Data.On
	}
	s := settings.Data

	lastfm, lb := st.Data.LastFM, st.Data.ListenBrainz
	lastfmDone := st.HasData && lastfm.Connected && !lastfm.NeedsReconnect
	lbDone := st.HasData && lb.Connected && !lb.NeedsReconnect
	lastfmWaiting := lastfm.Connecting || o.lastfmBusy
	// One message for both services: who Sona scrobbles as.
	scrobblingAs := func(name string) string {
		if name != "" {
			return i18n.T("Scrobbling as {name}.", "name", name)
		}
		return i18n.T("Scrobbling.")
	}
	configured := integ.HasData && integ.Data.Discord.Configured

	ui.Column(c.Key("connections")).FillWidth().Gap(12).Children(func() {
		rows := []func(){
			func() {
				line := i18n.T("Adds each song you play to your Last.fm profile.")
				switch {
				case lastfmDone:
					line = scrobblingAs(s.Scrobbling.LastFM.Username)
				case st.HasData && !lastfm.Available:
					line = i18n.T("This build of Sona can't connect to Last.fm.")
				}
				serviceRow(c, "lastfm", nameLastFM, line, func() {
					if lastfmDone {
						connected(c)
						return
					}
					label := i18n.T("Connect")
					if lastfm.NeedsReconnect {
						label = i18n.T("Reconnect")
					}
					b := kit.Button(c.Key("connect"), label, kit.ButtonOpts{
						Variant: kit.Outline, Size: kit.SizeSM, Icon: "external-link",
						Disabled: !st.HasData || !lastfm.Available || lastfmWaiting,
					}).Label(label + " " + nameLastFM)
					if b.Clicked() {
						o.connectLastFM()
					}
				}, func() {
					if lastfmWaiting {
						o.lastFMWaiting(c)
					}
				})
			},
			func() {
				line := i18n.T("Adds each song you play to your public ListenBrainz history.")
				if lbDone {
					line = scrobblingAs(s.Scrobbling.ListenBrainz.Username)
				}
				serviceRow(c, "listenbrainz", nameListenBrainz, line, func() {
					if lbDone {
						connected(c)
						return
					}
					if o.lb {
						return
					}
					label := i18n.T("Connect")
					if lb.NeedsReconnect {
						label = i18n.T("Reconnect")
					}
					b := kit.Button(c.Key("connect"), label, kit.ButtonOpts{
						Variant: kit.Outline, Size: kit.SizeSM, Disabled: !st.HasData,
					}).Label(label + " " + nameListenBrainz)
					if o.focusConnect {
						// The form's buttons went with it: the focus comes back to Connect.
						o.focusConnect = false
						b.Focus()
					}
					if b.Clicked() {
						o.lb = true
					}
				}, func() {
					if o.lb && !lbDone {
						o.listenBrainzForm(c)
					}
				})
			},
		}
		if discordShown {
			rows = append(rows, func() {
				hint := i18n.T("Shows the song you're playing as your Discord activity.")
				if integ.HasData && !configured {
					hint = i18n.T("Not available in this build of Sona.")
				}
				switchRow(c, "discord", i18n.T("Discord status"), hint,
					configured && s.Integrations.DiscordPresence, !settings.HasData || !configured, func(on bool) {
						o.save(func(s *config.Settings) { s.Integrations.DiscordPresence = on })
					}, nil)
			})
		}
		list(c, "connections", rows)
		if privateOn {
			text(c.Key("private"), i18n.T("Nothing is scrobbled or shown on Discord while private listening is on."), "sub").
				TextColor(theme.C("ink"))
		}
	})
}

// serviceRow is a service: its name and one line on the left, connecting or
// its state on the right; children sit under it.
func serviceRow(c *ui.Context, id, name, line string, action, children func()) {
	ui.Column(c.Key("service:"+id)).FillWidth().Padding(12, 0).Gap(12).Children(func() {
		ui.Row(c.Key("head")).FillWidth().Gap(24).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
			ui.Column(c.Key("words")).Grow(1).Shrink(1).MinWidth(0).Children(func() {
				text(c, name, "ui").FontWeight(500).TextColor(theme.C("ink"))
				text(c, line, "sub").TextColor(theme.C("ink-muted"))
			})
			ui.Row(c.Key("action")).Shrink(0).AlignItems(ui.Center).Children(action)
		})
		children()
	})
}

// connected is the check and "Connected" a connected service shows.
func connected(c *ui.Context) {
	ui.Row(c.Key("connected")).Gap(6).AlignItems(ui.Center).Children(func() {
		statusIcon(c, "circle-check", "positive").FontSize(14)
		text(c, i18n.T("Connected"), "sub").TextColor(theme.C("ink"))
	})
}

// connectLastFM starts Last.fm's desktop authorisation (the browser opens,
// Go waits for the approval) and stores the status it ends with. A failure
// other than a cancel is a toast (useConnectLastFM). lastfmBusy covers the
// call; Go's connecting the wait.
func (o *onboarding) connectLastFM() {
	if o.lastfmBusy {
		return
	}
	o.lastfmBusy = true
	api := o.scrobbler
	go func() {
		next, err := api.ConnectLastFM(context.Background())
		o.env.Post(func() {
			o.lastfmBusy = false
			if err != nil {
				if line := errorLine(false, err); line != "" {
					kit.ToastErrorf(i18n.T("Couldn't connect Last.fm"), line)
				}
				return
			}
			store.Set(o.env.Queries, keyScrobble, next)
		})
	}()
}

// lastFMWaiting is Last.fm's desktop authorisation under way: a spinner,
// what to do in the browser and Cancel (LastFMWaiting).
func (o *onboarding) lastFMWaiting(c *ui.Context) {
	msg := i18n.T("Allow Sona on the Last.fm page that opened in your browser. Sona continues by itself.")
	ui.Row(c.Key("waiting")).FillWidth().Gap(12).Padding(12, 16).AlignItems(ui.Center).Radius(theme.Current().Radius["md"]).
		Background(theme.C("hover")).Role(ui.RoleStatus).Label(msg).Children(func() {
		ic := statusIcon(c, "loader-circle", "ink-muted")
		if !theme.ReducedMotion() {
			ic.Rotate(ic.Loop("spin", time.Second, ui.Linear) * 360)
		}
		text(c, msg, "sub").TextColor(theme.C("ink")).Grow(1).Shrink(1).MinWidth(0)
		if kit.Button(c.Key("cancel"), i18n.T("Cancel"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).Clicked() {
			api := o.scrobbler
			go api.CancelConnect()
		}
	})
}

// closeListenBrainz closes the token form; the focus goes back to Connect.
func (o *onboarding) closeListenBrainz() {
	o.lb, o.focusConnect = false, true
	o.lbForm = tokenForm{}
}

// submitListenBrainz checks the token with ListenBrainz and stores the new
// status; a failure leaves its sentence in the form.
func (o *onboarding) submitListenBrainz() {
	f := &o.lbForm
	if f.busy {
		return
	}
	f.busy, f.err = true, ""
	token := f.token
	api := o.scrobbler
	go func() {
		next, err := api.ConnectListenBrainz(context.Background(), token)
		o.env.Post(func() {
			f.busy = false
			if err != nil {
				f.err = errorLine(true, err)
				return
			}
			store.Set(o.env.Queries, keyScrobble, next)
			o.lb = false
			o.lbForm = tokenForm{}
		})
	}()
}

// listenBrainzForm is the user token form under the ListenBrainz row: a
// password field, Connect (disabled while the token is blank or being
// checked) and Cancel, Escape cancelling, a hint with a link to the token
// page and an alert when the token was refused (ListenBrainzForm).
func (o *onboarding) listenBrainzForm(c *ui.Context) {
	f := &o.lbForm
	form := ui.Column(c.Key("lb-form")).FillWidth().Gap(8)
	form.Children(func() {
		text(c, i18n.T("User token"), "sub").FontWeight(500).TextColor(theme.C("ink"))
		ui.Row(c.Key("fields")).FillWidth().Gap(8).Children(func() {
			in := kit.Input(c.Key("token"), &f.token, kit.InputOpts{A11yLabel: i18n.T("User token"), Password: true, Invalid: f.err != "", Error: f.err})
			in.Grow(1).Shrink(1).MinWidth(0).Height(32).AutoFocus()
			ready := !f.busy && strings.TrimSpace(f.token) != ""
			label := i18n.T("Connect")
			if f.busy {
				label = i18n.T("Checking…")
			}
			submit := kit.Button(c.Key("submit"), label, kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Disabled: !ready}).Clicked()
			if (in.Submitted() || submit) && ready {
				o.submitListenBrainz()
			}
			if kit.Button(c.Key("cancel"), i18n.T("Cancel"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).Clicked() {
				o.closeListenBrainz()
			}
		})
		o.tokenHint(c)
		if f.err != "" {
			ui.Row(c.Key("err")).FillWidth().Gap(8).AlignItems(ui.Start).Role(ui.RoleAlert).Label(f.err).Children(func() {
				statusIcon(c, "circle-alert", "danger").Margin(2, 0, 0, 0)
				text(c, f.err, "sub").TextColor(theme.C("ink")).Shrink(1).MinWidth(0)
			})
		}
	})
	if form.Shortcut(0, ui.KeyEscape) {
		o.closeListenBrainz()
	}
}

// tokenHint is "Copy it from your ListenBrainz settings ↗. Sona checks it …":
// the translation marks the link as <0>text<1/></0>.
func (o *onboarding) tokenHint(c *ui.Context) {
	msg := i18n.T("Copy it from <0>your ListenBrainz settings<1/></0>. Sona checks it with ListenBrainz and keeps it in your keyring.")
	before, rest, _ := strings.Cut(msg, "<0>")
	link, after, ok := strings.Cut(rest, "<1/></0>")
	if !ok {
		text(c.Key("hint"), msg, "meta").TextColor(theme.C("ink-muted"))
		return
	}
	ui.Row(c.Key("hint")).FillWidth().Wrap().AlignItems(ui.Center).Children(func() {
		text(c.Key("a"), before, "meta").TextColor(theme.C("ink-muted"))
		if kit.Button(c.Key("link"), link, kit.ButtonOpts{Variant: kit.Link, Size: kit.SizeXS, Icon: "external-link"}).Clicked() {
			_ = o.env.OpenURL(listenBrainzTokenPage)
		}
		text(c.Key("b"), after, "meta").TextColor(theme.C("ink-muted"))
	})
}
