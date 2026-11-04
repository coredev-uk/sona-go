// Package cdm is the Widevine CDM that Sona downloads from Google and keeps
// up to date by itself: Go's CDMService mirrored from "cdm:status", and the
// player bar's line explaining a song that waits for it (the snapshot's
// NoCDM). Settings → Playback and Diagnostics, the first-run setup and the
// player read the mirror through store.UseCDM and word it with
// store.CDMProgress, store.CDMCanRetry, i18n.CDMProblem, i18n.CDMFailedState
// and i18n.FormatPercent; kit.RetryCDM asks Go to try again.
package cdm

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
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
		line(ctx, i18n.T("Widevine is ready. Press play to start."))
	case r.HasData && st.State == "failed":
		ui.Row(ctx).Gap(6).AlignItems(ui.Center).MinWidth(0).Tooltip(i18n.CDMProblem(st.Code)).Children(func() {
			ui.Icon(ctx, theme.Icon("circle-alert")).FontSize(14).TextColor(theme.C("danger"))
			text := i18n.T("Couldn't get Widevine · Retry in Settings")
			if st.Code == "unsupported_platform" {
				text = i18n.T("Songs can't play on this computer")
			}
			c.link(ctx, text)
		})
	default:
		text := i18n.T("Waiting for Widevine to download")
		if r.HasData && st.State == "downloading" {
			if f, ok := store.CDMProgress(st); ok {
				text = i18n.T("Waiting for Widevine to download · {percent}", "percent", i18n.FormatPercent(f))
			}
		}
		c.link(ctx, text)
	}
}

// link is text that opens Settings → Playback; plain text while that page is
// not ported.
func (c *cdm) link(ctx *ui.Context, text string) {
	if c.env.HasRoute == nil || !c.env.HasRoute("/settings") {
		line(ctx, text)
		return
	}
	t := theme.Current().Type["sub"]
	ui.Link(ctx.Key("cdm-link"), text, settingsPlayback).FontSize(t.Size).FixedLineHeight(t.LineHeight).
		TextColor(theme.C("ink")).Underline().SingleLine().MinWidth(0).Shrink(1)
}

// line is a plain line in the bar's sub style.
func line(ctx *ui.Context, text string) {
	t := theme.Current().Type["sub"]
	ui.Text(ctx, text).FontSize(t.Size).FixedLineHeight(t.LineHeight).TextColor(theme.C("ink")).SingleLine().MinWidth(0).Shrink(1)
}
