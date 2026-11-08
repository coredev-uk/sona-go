package player

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// queueAside is the queue beside the page (QueuePanel): 320 wide on the
// ground with a line at its left, a 48 high header (the heading, the queue's
// tools and Close) over the list. It is the shell's Aside slot and builds
// nothing while the queue is closed or Now Playing covers the page. Escape
// anywhere in it closes it, back on the bar's queue toggle. The whole panel
// is the queue's drop target.
func (p *player) queueAside(c *ui.Context) {
	if !p.ui.queueOpen || p.ui.overlay != overlayNone {
		if p.queue.menu.IsOpen() {
			p.queue.menu.Close()
		}
		p.queue.over = false
		if p.queue.hadFocus && !p.ui.queueOpen {
			// Closed (Close, or the toggle's keys) with the focus inside: it goes
			// back to the bar's Queue button (v0.2.0's useReturnFocus).
			p.queue.toggle.Focus(c)
		}
		p.queue.hadFocus = false
		return
	}
	aside := ui.Row(c.Key("queue-aside")).Width(320).FillHeight().Shrink(0).Background(theme.C("ground")).
		Role(ui.RoleRegion).Label(i18n.T("Queue"))
	// The region is described by the queue's summary, so that entering it
	// reads "Queue, 2 songs after this one" as the group's text did.
	if sum := summaryOf(p.env.Play.State(), p.env.Play.Queue()); sum != "" {
		aside.Description(sum)
	}
	p.queue.over = kit.DropOn(aside, queueDropID, p.queueTarget(c, &p.queue.lists[0]))
	aside.Children(func() {
		ui.Box(c.Key("edge")).Width(1).FillHeight().Shrink(0).Background(theme.C("line"))
		ui.Column(c.Key("panel")).Grow(1).Basis(0).MinWidth(0).FillHeight().Children(func() {
			ui.Row(c.Key("header")).Height(48).FillWidth().Shrink(0).AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(8).Padding(0, 8, 0, 16).Children(func() {
				label(c.Key("title"), i18n.T("Queue"), "heading").TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(2)
				ui.Row(c.Key("actions")).Gap(4).AlignItems(ui.Center).Children(func() {
					p.queueTools(c)
					if kit.IconButton(c.Key("close"), "x", i18n.T("Close queue"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).Clicked() {
						p.ui.queueOpen = false
					}
				})
			})
			p.queueList(c, false)
		})
	})
	p.queue.hadFocus = aside.FocusWithin()
	if aside.Shortcut(0, ui.KeyEscape) {
		p.ui.queueOpen = false
		p.queue.toggle.Focus(c)
	}
}
