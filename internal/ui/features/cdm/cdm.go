// Package cdm is the Widevine CDM that Sona downloads from Google and keeps
// up to date by itself: Go's CDMService mirrored from "cdm:status", and the
// player bar's line explaining a song that waits for it (the snapshot's
// NoCDM). Settings → Playback and Diagnostics, the first-run setup and the
// player read the mirror through store.UseCDM and word it with
// store.CDMProgress, store.CDMCanRetry, i18n.CDMProblem, i18n.CDMFailedState
// and i18n.FormatPercent; kit.RetryCDMWith asks Go to try again.
package cdm

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// settingsPlayback is where the download's progress and Retry are.
const settingsPlayback = "/settings?section=playback"

// cdm is the feature's state.
type cdm struct {
	env *module.Env
}

// New is the Widevine CDM module.
func New(env *module.Env) module.Module {
	c := &cdm{env: env}
	env.PlayerNotice.Add(0, c.notice)
	return module.Module{ID: "cdm", Init: c.init}
}

// init mirrors Go's status. Go sends "cdm:status" for every change once the
// handler is bound, so a status event that lands before the snapshot's answer
// is at least as new: the snapshot only fills the mirror while it is still
// empty (a download that fails at once otherwise showed as idle).
func (c *cdm) init() {
	store.On(c.env.Bus, services.EventCDMStatus, c.apply)
	if c.env.Core == nil {
		return
	}
	svc := services.Service[services.CDMService](c.env.Core)
	go func() {
		s := svc.Status()
		c.env.Post(func() { store.SetIfEmpty(c.env.Queries, store.KeyCDM, s) })
	}()
}

// apply replaces the mirror with Go's latest status. A new state, version or
// code changes what the playback probe finds, so the diagnostics are asked
// again; the download's progress alone does not.
func (c *cdm) apply(s services.CDMStatus) {
	prev, had := store.Peek[services.CDMStatus](c.env.Queries, store.KeyCDM)
	store.Set(c.env.Queries, store.KeyCDM, s)
	if !had || prev.State != s.State || prev.Version != s.Version || prev.Code != s.Code {
		c.env.Queries.Invalidate(store.Key{"diagnostics"})
	}
}

// notice is the player bar's line while the song waits for the CDM: still
// downloading, failed, or ready for another press of play. It links to
// Settings → Playback (where the download's progress and Retry are) once that
// page is ported.
func (c *cdm) notice(ctx *ui.Context) {
	r := store.UseCDM(c.env.Queries, c.env.Core)
	st := r.Data
	switch {
	case r.HasData && st.State == "ready":
		ui.Row(ctx).Gap(6).AlignItems(ui.Center).MinWidth(0).Children(func() {
			ui.Icon(ctx, theme.Icon("check")).FontSize(14).TextColor(theme.C("ink")).Shrink(0)
			line(ctx, i18n.T("Widevine is ready. Press play to start."))
		})
	case r.HasData && st.State == "failed":
		ui.Row(ctx).Gap(6).AlignItems(ui.Center).MinWidth(0).Tooltip(i18n.CDMProblem(st.Code)).Children(func() {
			// A danger dot: the colour is not the only carrier, the words say it.
			ui.Box(ctx.Key("cdm-dot")).Size(6, 6).Shrink(0).Radius(3).Background(theme.C("danger"))
			text := i18n.T("Couldn't get Widevine · Retry in Settings")
			if st.Code == "unsupported_platform" {
				text = i18n.T("Songs can't play on this computer")
			}
			c.link(ctx, text, i18n.CDMProblem(st.Code))
		})
	default:
		downloading := r.HasData && st.State == "downloading"
		ui.Row(ctx).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
			c.link(ctx, i18n.T("Waiting for Widevine to download"), "")
			if !downloading {
				return
			}
			// The meter is 48x3 beside the percent: a glance says it is
			// moving. Unknown size: the empty track and no figure.
			f, ok := store.CDMProgress(st)
			value := -1.0
			if ok {
				value = f
			}
			label := i18n.T("Widevine download")
			if st.Updating {
				label = i18n.T("Widevine update")
			}
			kit.ProgressBar(ctx.Key("cdm-meter"), value, kit.ProgressOpts{A11yLabel: label, Width: 48, Height: 3})
			if ok {
				t := theme.Current().Type["sub"]
				ui.Text(ctx, i18n.FormatPercent(f)).FontSize(t.Size).FixedLineHeight(t.LineHeight).
					TextColor(theme.C("ink-muted")).SingleLine().Shrink(0).Role(ui.RoleNone)
			}
		})
	}
}

// link is text that opens Settings → Playback; plain text while that page is
// not ported.
// desc is what the tooltip says, for assistive technology too.
func (c *cdm) link(ctx *ui.Context, text, desc string) {
	if c.env.HasRoute == nil || !c.env.HasRoute("/settings") {
		line(ctx, text)
		return
	}
	t := theme.Current().Type["sub"]
	// The bar is built outside the routed pages, where a ui.Link would hand
	// its URL to the browser: the link has none and the click goes to the router.
	l := ui.Link(ctx.Key("cdm-link"), text, "").FontSize(t.Size).FixedLineHeight(t.LineHeight).
		TextColor(theme.C("ink")).Underline().SingleLine().MinWidth(0).Shrink(1)
	if desc != "" {
		l.Description(desc)
	}
	if l.Clicked() && c.env.Nav != nil {
		c.env.Nav.Push(settingsPlayback)
	}
}

// line is a plain line in the bar's sub style.
func line(ctx *ui.Context, text string) {
	t := theme.Current().Type["sub"]
	ui.Text(ctx, text).FontSize(t.Size).FixedLineHeight(t.LineHeight).TextColor(theme.C("ink")).SingleLine().MinWidth(0).Shrink(1)
}
