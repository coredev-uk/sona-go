package settings

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// connections is Settings → Connections (Connections.tsx): the scrobblers,
// then Discord's status.
func (s *settings) connections(c *ui.Context) {
	cfg, caps, ok := s.ready()
	if !ok {
		s.loading(c, 3)
		return
	}
	s.stack(c, func() {
		s.scrobbling(c, cfg)
		s.group(c, "discord", i18n.T("Discord"), "", func() { s.discord(c, cfg, caps) })
	})
}

// scrobbling is the Scrobbling group: what counts as a listen, a Last.fm and
// a ListenBrainz row and the notes about the connections.
func (s *settings) scrobbling(c *ui.Context, cfg config.Settings) {
	st := stateOf[scrobbleUI](s, "scrobbling")
	res := s.scrobbleStatus()
	status := res.Data
	privateOn := store.UsePrivate(s.env.Queries, s.env.Core).Data.On
	s.group(c, "scrobbling", i18n.T("Scrobbling"),
		i18n.T("A song counts once you've played half of it or four minutes. Radio and songs of 30 seconds or less aren't sent. Songs you play offline are sent when you're back online."),
		func() {
			if privateOn {
				text(c, i18n.T("Nothing is scrobbled while private listening is on."), "secondary").TextColor(theme.C("ink")).
					MaxWidth(rowTextWidth + 40)
			}
			s.lastFMRow(c, st, cfg.Scrobbling.LastFM, status.LastFM, res.HasData)
			s.listenBrainzRow(c, st, cfg.Scrobbling.ListenBrainz, status.ListenBrainz, res.HasData)
			if res.Err != nil {
				s.alertLine(c.Key("status-error"), "circle-alert",
					i18n.T("Couldn't read the scrobbler connections: {statusError}", "statusError", i18n.ErrorMessage(res.Err)))
			}
			if status.Unencrypted && (status.LastFM.Connected || status.ListenBrainz.Connected) {
				s.unencryptedNote(c)
			}
		})
}

// accountLine is what a connected (or broken) account is doing, in one
// sentence.
func accountLine(p provider, st services.ScrobblerStatus, acct config.ScrobbleAccount) string {
	waiting := ""
	if st.Pending > 0 {
		waiting = " " + i18n.T("{pending, plural, one {# song waiting to send.} other {# songs waiting to send.}}", "pending", st.Pending)
	}
	if st.NeedsReconnect {
		return i18n.T("{name} no longer accepts Sona's sign-in. Reconnect to keep scrobbling.", "name", providerName(p)) + waiting
	}
	user := trim(acct.Username)
	if !acct.Enabled {
		if user != "" {
			return i18n.T("Connected as {user}, paused. Songs you play now aren't sent.", "user", user) + waiting
		}
		return i18n.T("Connected, paused. Songs you play now aren't sent.") + waiting
	}
	if user != "" {
		return i18n.T("Scrobbling as {user}.", "user", user) + waiting
	}
	return i18n.T("Scrobbling.") + waiting
}

// providerRow is one provider: its state on the left; connect (or the pause
// switch and Disconnect) on the right. connect builds the Connect button
// (shown when not connected or needing a reconnect) and returns it.
func (s *settings) providerRow(c *ui.Context, st *scrobbleUI, p provider, acct config.ScrobbleAccount, ss services.ScrobblerStatus,
	desc string, connect func() ui.Element, toggle func(*config.Settings, bool),
) {
	name := providerName(p)
	id := "scrobble-lastfm"
	if p == services.ScrobblerListenBrainz {
		id = "scrobble-listenbrainz"
	}
	if ss.Connected {
		desc = accountLine(p, ss, acct)
	}
	busy := st.removing == p
	s.tell(c, id, desc) // the account line is a polite live region
	s.row(c, rowOpts{ID: id, Title: name, Desc: desc}, func() ui.Element {
		var first ui.Element
		if ss.Connected {
			if ss.NeedsReconnect {
				first = connect()
			} else {
				on := acct.Enabled
				sw := kit.Switch(c.Key("on"), &on, kit.SwitchOpts{A11yLabel: name, Disabled: busy})
				if sw.Changed() {
					s.save(func(x *config.Settings) { toggle(x, on) })
				}
				first = sw
			}
			if kit.Button(c.Key("disconnect"), i18n.T("Disconnect"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Disabled: busy, A11yLabel: i18n.T("Disconnect") + " " + name}).Clicked() {
				s.askDisconnect(st, p, ss.Pending)
			}
		} else {
			first = connect()
		}
		return first
	})
}

// askDisconnect disconnects p, after a confirmation when songs are still
// waiting to be sent.
func (s *settings) askDisconnect(st *scrobbleUI, p provider, pending int) {
	if pending <= 0 {
		s.disconnect(&st.removing, p)
		return
	}
	kit.Confirm(kit.ConfirmOptions{
		Title:        i18n.T("Disconnect {confirmName}?", "confirmName", providerName(p)),
		Body:         pendingLine(pending) + " " + i18n.T("Sona discards them and removes its sign-in from your keyring."),
		ConfirmLabel: i18n.T("Disconnect"),
		CancelLabel:  i18n.T("Keep connected"),
		Destructive:  true,
	}, func(ok bool) {
		if ok {
			s.disconnect(&st.removing, p)
		}
	})
}

// lastFMRow is the Last.fm row: Connect, which opens the browser and waits
// (a panel under the row with Cancel) and, once connected, the pause switch.
func (s *settings) lastFMRow(c *ui.Context, st *scrobbleUI, acct config.ScrobbleAccount, ss services.ScrobblerStatus, loaded bool) {
	desc := i18n.T("Adds each song you play to your Last.fm profile.")
	if loaded && !ss.Available {
		desc = i18n.T("This build of Sona has no Last.fm API key, so it can't connect to Last.fm.")
	}
	connecting := ss.Connecting || st.lastfmBusy
	s.providerRow(c, st, services.ScrobblerLastFM, acct, ss, desc, func() ui.Element {
		label := i18n.T("Connect")
		if ss.NeedsReconnect {
			label = i18n.T("Reconnect")
		}
		b := kit.Button(c.Key("connect"), label, kit.ButtonOpts{
			Variant: kit.Outline, Size: kit.SizeSM, Icon: "external-link", Disabled: !ss.Available || connecting,
			A11yLabel: label + " Last.fm",
		})
		if b.Clicked() {
			s.connectLastFM(&st.lastfmBusy)
		}
		return b
	}, func(x *config.Settings, on bool) { x.Scrobbling.LastFM.Enabled = on })
	if connecting {
		ui.Column(c.Key("lastfm-below")).FillWidth().Padding(0, 0, rowPadY, 0).Children(func() {
			s.lastFMWaiting(c, s.cancelLastFM)
		})
	}
}

// listenBrainzRow is the ListenBrainz row: Connect opens the token form
// under the row; closing it puts the focus back on the row's button.
func (s *settings) listenBrainzRow(c *ui.Context, st *scrobbleUI, acct config.ScrobbleAccount, ss services.ScrobblerStatus, loaded bool) {
	closeForm := func() {
		st.lb, st.focusBack = false, true
	}
	s.providerRow(c, st, services.ScrobblerListenBrainz, acct, ss, i18n.T("Adds each song you play to your public ListenBrainz history."), func() ui.Element {
		if st.lb {
			return ui.Element{}
		}
		label := i18n.T("Connect")
		if ss.NeedsReconnect {
			label = i18n.T("Reconnect")
		}
		b := kit.Button(c.Key("connect"), label, kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Disabled: !loaded, A11yLabel: label + " ListenBrainz"})
		if b.Clicked() {
			st.lb, st.lbForm = true, tokenForm{}
		}
		return b
	}, func(x *config.Settings, on bool) { x.Scrobbling.ListenBrainz.Enabled = on })
	if st.focusBack && !st.lb {
		// The form's buttons went with it: the focus goes to the row's first
		// control (Connect if cancelled, the switch once connected).
		st.focusBack = false
		s.focusSetting("scrobble-listenbrainz")
	}
	if st.lb {
		ui.Column(c.Key("lb-below")).FillWidth().Padding(0, 0, rowPadY, 0).Children(func() {
			s.listenBrainzForm(c, &st.lbForm,
				func() {
					s.submitListenBrainz(&st.lbForm, func(next services.ScrobbleStatus) {
						store.Set(s.env.Queries, keyScrobble, next)
						closeForm()
					})
				}, closeForm)
		})
	}
}

// unencryptedNote warns that no keyring holds the scrobbler sign-ins.
func (s *settings) unencryptedNote(c *ui.Context) {
	c = c.Key("unencrypted")
	msg := i18n.T("Your scrobbler sign-ins are stored unencrypted")
	row := ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Start).Padding(12, 16).Radius(theme.Current().Radius["lg"]).
		Background(theme.C("raised")).Border(1, theme.C("control-line")).Role(ui.RoleStatus).Label(msg)
	announceOnce(c, row, msg)
	row.Children(func() {
		ui.Icon(c.Key("i"), theme.Icon("shield-alert")).FontSize(16).TextColor(theme.C("danger")).Margin(2, 0, 0, 0).Shrink(0)
		ui.Column(c.Key("t")).Shrink(1).MinWidth(0).Children(func() {
			text(c, i18n.T("Your scrobbler sign-ins are stored unencrypted"), "label").TextColor(theme.C("ink"))
			text(c, i18n.T("No Secret Service keyring is running, so Sona keeps them in files only your user can read. Start GNOME Keyring or KWallet and Sona moves them into the keyring."), "secondary").
				TextColor(theme.C("ink-muted")).Margin(2, 0, 0, 0)
		})
	})
}

// discord is the Discord status row (IntegrationsConnectionsRows): an opt-in
// switch, off and disabled in a build without a Discord application ID, and
// an unavailable row where Sona can't reach Discord.
func (s *settings) discord(c *ui.Context, cfg config.Settings, caps services.Capabilities) {
	title := i18n.T("Discord status")
	if !caps.DiscordPresence {
		s.unavailableRow(c, "discord-presence", title, "")
		return
	}
	res := s.integrationsStatus()
	status := res.Data
	configured := res.HasData && status.Discord.Configured
	privateOn := store.UsePrivate(s.env.Queries, s.env.Core).Data.On
	on := cfg.Integrations.DiscordPresence
	s.switchRow(c, rowOpts{ID: "discord-presence", Title: title, Desc: discordDescription(res.HasData, status.Discord, on, privateOn)},
		configured && on, !configured, func(v bool) {
			s.save(func(x *config.Settings) { x.Integrations.DiscordPresence = v })
		})
}

// discordDescription says what the switch does and how it is doing.
func discordDescription(loaded bool, d services.DiscordStatus, on, privateOn bool) string {
	if loaded && !d.Configured {
		return i18n.T("Not available in this build: it was made without a Discord application ID.")
	}
	if !on || !loaded {
		return i18n.T("Shows the song you're playing, with its artwork and time, as your Discord activity. Cleared when you pause.")
	}
	if privateOn {
		return i18n.T("Shows the song you're playing, with its artwork and time, as your Discord activity. Nothing is shown while private listening is on.")
	}
	if d.Connected {
		return i18n.T("Shows the song you're playing, with its artwork and time, as your Discord activity. Cleared when you pause. Connected to Discord.")
	}
	return i18n.T("Shows the song you're playing, with its artwork and time, as your Discord activity. Cleared when you pause. Discord isn't reachable yet; Sona keeps trying while music plays.")
}
