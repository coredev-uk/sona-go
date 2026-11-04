package player

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// queueTools is the queue's own controls (QueueTools): Clear, while the user
// has songs queued, and the AutoPlay toggle.
func (p *player) queueTools(c *ui.Context) {
	s := p.env.Play.State()
	parts, _ := p.queueParts(s, p.env.Play.Queue())
	ui.Row(c.Key("queue-tools")).Gap(4).AlignItems(ui.Center).Children(func() {
		if parts.mine > 0 && kit.Button(c.Key("clear"), i18n.T("Clear"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).Clicked() {
			p.clearQueue()
		}
		p.autoPlayToggle(c, s)
	})
}

// clearQueue empties the user's part of the queue.
func (p *player) clearQueue() {
	p.run(func() string { return i18n.T("Couldn't clear the queue") }, func(context.Context) error {
		p.svc.Clear()
		return nil
	})
}

// autoPlayToggle is the AutoPlay button: pressed while it is on (a dot
// under the glyph), dimmed when it doesn't apply to what plays (a live
// station never ends), where its tip says why and a click does nothing.
func (p *player) autoPlayToggle(c *ui.Context, s services.PlaybackState) {
	reason := unavailable(s, ctlAutoPlay)
	on := s.AutoPlay && reason == ""
	tip := reason
	switch {
	case reason != "":
	case s.AutoPlay:
		tip = i18n.T("AutoPlay on: similar music plays when the queue ends")
	default:
		tip = i18n.T("AutoPlay off")
	}
	ui.Box(c.Key("autoplay-box")).Size(32, 32).Shrink(0).Children(func() {
		b := kit.Button(c.Key("autoplay"), "", kit.ButtonOpts{
			Variant: kit.Ghost, Size: kit.SizeSM, Icon: "infinity", Pressed: &on, Quiet: true, Muted: true,
			A11yLabel: i18n.T("AutoPlay"), Tip: tip, Reason: reason,
		})
		if reason != "" {
			b.Opacity(0.4) // v0.2.0's opacity-40
		}
		if on {
			ui.Box(c.Key("dot")).Absolute().Left(14).Top(26).Size(4, 4).Radius(2).Background(theme.C("ink")).IgnoreInput(true)
		}
		if b.Clicked() && reason == "" {
			next := !s.AutoPlay
			p.env.Play.Patch(func(s *services.PlaybackState) { s.AutoPlay = next })
			p.run(func() string { return i18n.T("Couldn't change AutoPlay") }, func(context.Context) error { return p.svc.SetAutoPlay(next) })
		}
	})
}
