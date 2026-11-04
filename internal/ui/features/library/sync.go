package library

import (
	"context"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// syncStore is v0.2.0's useSync: library sync progress from sync:progress
// and sync:done, for the sidebar indicator and the empty states. Events
// arrive on the main thread; a SyncStatus answer is applied on it too, and
// dropped when an event arrived since it was asked for.
type syncStore struct {
	mu      sync.Mutex
	running bool
	// current is the domain being synced and how far it is.
	current *services.SyncProgress
	last    *services.SyncDone
	// events counts the sync:progress and sync:done events that arrived.
	events int
}

func (s *syncStore) progress(p services.SyncProgress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running, s.current = true, &p
	s.events++
}

func (s *syncStore) done(d services.SyncDone) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running, s.current, s.last = false, nil, &d
	s.events++
}

// count is how many events have arrived: what a request notes before it
// asks Go for the status.
func (s *syncStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.events
}

// snapshot applies a SyncStatus answer requested when events was at: once
// any event has arrived since, the events carry the newer state and the
// answer is dropped. running true keeps the current domain, false clears it.
func (s *syncStore) snapshot(running bool, at int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.events != at || s.running == running {
		return
	}
	s.running = running
	if !running {
		s.current = nil
	}
}

// state is a copy of what the views read.
func (s *syncStore) state() (running bool, current *services.SyncProgress, last *services.SyncDone, events int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running, s.current, s.last, s.events
}

// syncStatus is the useSyncStatus query: every answer also brings the sync
// store up to date (unless a sync event overtook it), so the indicator
// follows Go at start-up and whenever the status is refetched.
func (l *library) syncStatus() store.Result[services.SyncStatus] {
	return store.Use(l.env.Queries, keySync, func(ctx context.Context) (services.SyncStatus, error) {
		at := l.sync.count()
		st, err := l.lib.SyncStatus(ctx)
		if err == nil {
			l.env.Post(func() { l.sync.snapshot(st.Running, at) })
		}
		return st, err
	})
}

// authStatus is the account query the library's states read.
func (l *library) authStatus() store.Result[services.AuthStatus] {
	return store.UseAuth(l.env.Queries, l.env.Core)
}

// syncNow starts a library sync (the indicator's Sync now).
func (l *library) syncNow() {
	l.run(i18n.T("Couldn't start a library sync"), func(context.Context) error { return l.lib.Sync() }, nil)
}

// syncingText is the line for a domain being synced: one message per
// domain, so the noun is never spliced into the sentence.
func syncingText(domain string) string {
	switch domain {
	case "artists":
		return i18n.T("Syncing artists")
	case "albums":
		return i18n.T("Syncing albums")
	case "songs":
		return i18n.T("Syncing songs")
	case "playlists":
		return i18n.T("Syncing playlists")
	case "playlist-tracks":
		return i18n.T("Syncing playlist songs")
	case "ratings":
		return i18n.T("Syncing loves")
	}
	return i18n.T("Syncing {domain}", "domain", domain)
}

// syncIndicator is the library sync line at the foot of the sidebar: what
// is syncing and how far, or when the library was last synced, with a Sync
// now action. Nothing here moves on its own: the count and the bar move
// only when Go reports some.
func (l *library) syncIndicator(c *ui.Context) {
	auth := l.authStatus()
	status := l.syncStatus()
	if !auth.HasData || !account.HasAccount(auth.Data.State) {
		return
	}
	running, current, last, _ := l.sync.state()
	tk := theme.Current()
	meta := tk.Type["meta"]
	text := func(s string, color ui.Color) ui.Element {
		return ui.Text(c, s).FontSize(meta.Size).FixedLineHeight(meta.LineHeight).TextColor(color).SingleLine()
	}

	failed := last != nil && !last.OK && last.ErrorCode != services.CodeCancelled
	lastSuccess := status.Data.LastSuccess
	if last != nil && last.OK {
		lastSuccess = last.FinishedAt
	}

	ui.Column(c.Key("sync")).Padding(0, 10).Margin(0, 0, 8, 0).Role(ui.RoleStatus).Label(i18n.T("Library sync")).Children(func() {
		if running {
			syncing := i18n.T("Syncing your library")
			count := ""
			pct := 0.0
			if current != nil {
				syncing = syncingText(current.Domain)
				if current.Total > 0 {
					count = i18n.T("{done} of {total}", "done", i18n.FormatNumber(float64(current.Done)), "total", i18n.FormatNumber(float64(current.Total)))
					pct = min(1, float64(current.Done)/float64(current.Total))
				}
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, theme.Icon("refresh-cw")).FontSize(12).Shrink(0).TextColor(theme.C("ink-muted"))
				text(syncing, theme.C("ink-muted")).Shrink(1).MinWidth(0)
			})
			// The count sits beside the bar, so neither it nor the domain is
			// cut off in the sidebar's width. The row keeps its height before
			// the first count arrives.
			ui.Row(c).Height(16).Gap(8).AlignItems(ui.Center).Margin(4, 0, 0, 0).Children(func() {
				kit.ProgressBar(c, pct, kit.ProgressOpts{A11yLabel: i18n.T("Library sync"), ValueText: count, Height: 2}).Grow(1).MinWidth(0)
				if count != "" {
					text(count, theme.C("ink-faint")).Shrink(0)
				}
			})
			return
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(1).MinWidth(0).Children(func() {
				colour := theme.C("ink-faint")
				if failed {
					colour = theme.C("ink")
					ui.Icon(c, theme.Icon("circle-alert")).FontSize(12).Shrink(0).TextColor(theme.C("danger"))
				}
				switch {
				case failed:
					text(i18n.T("Library sync failed"), colour).Shrink(1).MinWidth(0).Tooltip(i18n.ErrorText(last.ErrorCode))
				case lastSuccess > 0:
					text(i18n.T("Synced {synced}", "synced", i18n.RelativeTime(time.UnixMilli(lastSuccess), time.Now())), colour).Shrink(1).MinWidth(0)
				default:
					text(i18n.T("Library not synced yet"), colour).Shrink(1).MinWidth(0)
				}
			})
			label := i18n.T("Sync now")
			if failed {
				label = i18n.T("Try again")
			}
			b := ui.ButtonBase(c.Key("sync-now")).Shrink(0).Radius(tk.Radius["sm"]).Label(label)
			b.Children(func() {
				t := text(label, theme.C("ink-muted"))
				if b.Hovered() {
					t.Underline()
				}
			})
			if b.Clicked() {
				l.syncNow()
			}
		})
	})
}
