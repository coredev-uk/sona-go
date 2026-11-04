package player

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// queueDropID is the queue's id as a drag origin (DragOrigin.List) and as a
// drop target.
const queueDropID = "queue"

// keyAuth is the account's query: the library module keeps it in step with
// the auth events, and v0.2.0's useWithheld read the same status.
var keyAuth = store.Key{"auth"}

// queueUI is the queue panel's own state (QueuePanel.tsx's hooks): the list's
// scroll, its roving focus, the move waiting to be announced and the row
// menu. The panel beside the page and Now Playing's Up next pane each have a
// list state of their own; only one is on screen at a time, so the rest is
// shared. Main thread only.
type queueUI struct {
	// toggle is the bar's Queue button: the bar binds it (Bind), and Escape
	// in the panel returns the focus to it.
	toggle ui.Handle
	// parts caches the queue's lines (queueParts).
	parts partsCache
	// hadFocus: the panel had the focus within last frame.
	hadFocus bool
	// over says a drag that the panel would take is over it (the aside's
	// own drop target; Now Playing's list is its own).
	over bool
	// lists are the panel beside the page (0) and the one in Now Playing (1).
	lists [2]queueList
	// focus is the row with the list's Tab stop, by its entry's uid.
	focus *focusTarget
	// moved is a move asked of Go, until it shows in the queue.
	moved *pendingMove
	// menu is the row menu, of the entry menuUID.
	menu    kit.Menu
	menuUID uint64
	// auth is the account's status, for explicit songs the account skips.
	auth func() services.AuthStatus
}

// pendingMove is a move asked of Go, until it shows in the queue: it is
// then announced with the song's new place, and a song with the focus is
// brought into view (it may now be far down the list).
type pendingMove struct {
	uid  uint64
	name string
	// from is the queue index the song moved from.
	from int
	// focus: the focus is on the song (or its menu): it stays on the song.
	focus bool
	// until is when a move that never shows is forgotten.
	until time.Time
}

// followAccount tells the queue which accounts restrict explicit content:
// such songs are dimmed, as Go skips them (v0.2.0's useWithheld). The shell
// calls it once, with the AuthService.
func (p *player) followAccount(auth interface{ Status() services.AuthStatus }) {
	p.queue.auth = auth.Status
	if p.env.Bus != nil {
		store.On(p.env.Bus, services.EventAuthChanged, func(s services.AuthStatus) {
			store.Set(p.env.Queries, keyAuth, s)
		})
	}
}

// restricted reports whether the account restricts explicit content.
func (p *player) restricted() bool {
	if p.queue.auth == nil || p.env.Queries == nil {
		return false
	}
	r := store.Use(p.env.Queries, keyAuth, func(context.Context) (services.AuthStatus, error) { return p.queue.auth(), nil })
	return r.Data.Restricted
}

// indexOfUID is where the entry uid is in items, -1 when it is not.
func indexOfUID(items []services.QueueItem, uid uint64) int {
	for i := range items {
		if items[i].UID == uid {
			return i
		}
	}
	return -1
}

// jumpTo plays the queue entry at qi.
func (p *player) jumpTo(qi int) {
	p.run(func() string { return i18n.T("Couldn't play that song") }, func(ctx context.Context) error { return p.svc.Jump(ctx, qi) })
}

// removeAt takes the queue entry at qi out of the queue.
func (p *player) removeAt(qi int) {
	p.run(func() string { return i18n.T("Couldn't remove it") }, func(ctx context.Context) error { return p.svc.Remove(ctx, qi) })
}

// askMove asks Go for a move of the entry it (now at from): the panel
// announces it once it shows, and forgets it if it fails. A failure is a
// "Couldn't move it" toast.
func (p *player) askMove(it services.QueueItem, from int, focus bool, ask func() error) {
	p.queue.moved = &pendingMove{uid: it.UID, name: it.Name, from: from, focus: focus, until: p.now().Add(5 * time.Second)}
	p.run(func() string { return i18n.T("Couldn't move it") }, func(context.Context) error {
		err := ask()
		if err != nil {
			p.env.Post(func() { p.queue.moved = nil })
		}
		return err
	})
}

// settle is what the queue shows of the pending move, once per frame: when
// the song has landed it is announced ("Song: moved to position 2 of 5")
// and a song with the focus is brought back into view.
func (p *player) settle(c *ui.Context, st *queueList, items []services.QueueItem, g *queueGeom) {
	m := p.queue.moved
	if m == nil {
		return
	}
	at := indexOfUID(items, m.uid)
	switch {
	case at >= 0 && at != m.from:
		p.queue.moved = nil
		c.Announce(i18n.T("{movedName}: moved to position {movedPosition} of {movedOf}",
			"movedName", m.name, "movedPosition", at-g.start, "movedOf", g.userEnd-g.start-1))
		if m.focus {
			p.focusRow(st, items, at)
		}
	case p.now().After(m.until):
		p.queue.moved = nil
	}
}
