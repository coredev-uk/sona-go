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
	"github.com/coredev-uk/sona-go/internal/ui/module"
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
	// phase is the phase the sidebar line showed last frame (the view's
	// alone: only the main thread builds it).
	phase     syncPhase
	phaseSeen bool
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
	case "music-videos":
		return i18n.T("Syncing music videos")
	}
	return i18n.T("Syncing {domain}", "domain", domain)
}

// syncPhase is what the sidebar's sync line shows, for the announcements.
type syncPhase int

const (
	syncIdle syncPhase = iota
	syncRunning
	syncFailed
)

// announceSync reads out a change of the sync's phase: it starts, it ends, it
// fails. The count moving inside a domain is not read out, which is why the
// line is a labelled group and not a status: MyGo reads a status whole every
// time its text changes (the count, the domain, "Synced 2 minutes ago").
// The first frame only records the phase.
func (l *library) announceSync(c *ui.Context, running, failed bool, lastSuccess int64) {
	phase := syncIdle
	switch {
	case running:
		phase = syncRunning
	case failed:
		phase = syncFailed
	}
	s := &l.sync
	if s.phaseSeen && s.phase != phase {
		text := ""
		switch {
		case running:
			text = i18n.T("Syncing your library")
		case failed:
			text = i18n.T("Library sync failed")
		case lastSuccess > 0:
			text = i18n.T("Synced {synced}", "synced", i18n.RelativeTime(time.UnixMilli(lastSuccess), time.Now()))
		}
		if text != "" {
			c.Announce(text)
		}
	}
	s.phase, s.phaseSeen = phase, true
}

// syncView is what the library sync is doing now, for the notification
// centre's bell, its row and its foot line.
type syncView struct {
	// account: there is an account to sync; nothing shows without one.
	account     bool
	running     bool
	failed      bool
	current     *services.SyncProgress
	last        *services.SyncDone
	lastSuccess int64
}

func (l *library) syncNowView() syncView {
	auth := l.authStatus()
	status := l.syncStatus()
	if !auth.HasData || !account.HasAccount(auth.Data.State) {
		return syncView{}
	}
	v := syncView{account: true}
	v.running, v.current, v.last, _ = l.sync.state()
	v.failed = v.last != nil && !v.last.OK && v.last.ErrorCode != services.CodeCancelled
	v.lastSuccess = status.Data.LastSuccess
	if v.last != nil && v.last.OK {
		v.lastSuccess = v.last.FinishedAt
	}
	return v
}

// syncLine is a running sync's words: the domain, how far ("812 of 2,040")
// and the fraction done (0 until the first count arrives).
func syncLine(current *services.SyncProgress) (text, count string, done float64) {
	text = i18n.T("Syncing your library")
	if current != nil {
		text = syncingText(current.Domain)
		if current.Total > 0 {
			count = i18n.T("{done} of {total}", "done", i18n.FormatNumber(float64(current.Done)), "total", i18n.FormatNumber(float64(current.Total)))
			done = min(1, float64(current.Done)/float64(current.Total))
		}
	}
	return text, count, done
}

// syncActivity is the library sync's part of the notification centre: a
// running sync is the bell's bar and a row with its domain, count and bar; a
// failed one is the bell's danger dot and a row with Try again; the panel's
// foot says when the library last synced, with Sync now. Nothing here moves
// on its own: the count and the bars move only when Go reports some.
func (l *library) syncActivity() *module.Activity {
	return &module.Activity{Status: l.syncState, Build: l.syncRow, Foot: l.syncFoot}
}

// syncState is the Activity's Status. It also reads out a change of phase,
// since the bell is always built.
func (l *library) syncState(c *ui.Context) module.ActivityStatus {
	v := l.syncNowView()
	if !v.account {
		return module.ActivityStatus{}
	}
	l.announceSync(c, v.running, v.failed, v.lastSuccess)
	st := module.ActivityStatus{Shown: v.running || v.failed, Busy: v.running, Alert: v.failed}
	if v.running {
		text, count, done := syncLine(v.current)
		st.Progress, st.Label = done, text
		if count != "" {
			// translators: The notification bell's name while the library syncs: "Syncing songs, 812 of 2,040".
			st.Label = i18n.T("{what}, {count}", "what", text, "count", count)
		}
	}
	return st
}

// syncRow is the Activity's row: the running sync, or the failed one.
func (l *library) syncRow(c *ui.Context) {
	v := l.syncNowView()
	switch {
	case v.running:
		text, count, done := syncLine(v.current)
		kit.ActivityRow(c, "library:sync", kit.ActivitySpec{
			Icon: "refresh-cw", Title: text, Count: count, Progress: &done, ProgressLabel: i18n.T("Library sync"),
		})
	case v.failed:
		kit.ActivityRow(c, "library:sync", kit.ActivitySpec{
			Icon: "circle-alert", Alert: true, Title: i18n.T("Library sync failed"), Detail: i18n.ErrorText(v.last.ErrorCode),
			Action: i18n.T("Try again"), OnAction: l.syncNow,
		})
	}
}

// syncFoot is the panel's foot line while no sync runs or failed: when the
// library last synced, after a --positive dot, and Sync now.
func (l *library) syncFoot(c *ui.Context) {
	v := l.syncNowView()
	if !v.account || v.running || v.failed {
		return
	}
	meta := theme.Current().Type["meta"]
	theme.Hairline(c).Margin(8, 0, 0, 0)
	ui.Row(c.Key("library:sync-foot")).FillWidth().Padding(8, 4, 0, 8).Gap(8).AlignItems(ui.Center).
		Role(ui.RoleGroup).Label(i18n.T("Library sync")).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			line := i18n.T("Library not synced yet")
			if v.lastSuccess > 0 {
				ui.Box(c.Key("ok")).Size(6, 6).Radius(3).Shrink(0).Background(theme.C("positive"))
				line = i18n.T("Synced {synced}", "synced", i18n.RelativeTime(time.UnixMilli(v.lastSuccess), time.Now()))
			}
			ui.Text(c, line).FontSize(meta.Size).FixedLineHeight(meta.LineHeight).TextColor(theme.C("ink-muted")).
				SingleLine().Shrink(1).MinWidth(0)
		})
		if kit.Button(c.Key("sync-now"), i18n.T("Sync now"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).Clicked() {
			l.syncNow()
		}
	})
}
