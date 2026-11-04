package store

import (
	"context"
	"errors"

	"github.com/coredev-uk/sona-go/internal/services"
)

// The shared mirrors of what Go reports about the account's side of the
// app: private listening, the Widevine CDM, the update check and the release
// reminders. v0.2.0 kept the first two in zustand stores and the others in
// TanStack queries; here they are all queries under these keys, so a
// feature that shows one (the account menu, Settings, the player bar, an
// album page) reads it with the UseX below and the module that owns it
// (features/private, cdm, updates, reminders) keeps it current from Go's
// events. Every one is never stale: events replace the data.
var (
	KeyPrivate   = Key{"private"}
	KeyCDM       = Key{"cdm"}
	KeyUpdates   = Key{"updates"}
	KeyReminders = Key{"reminders"}
)

var errNoCore = errors.New("ui: no services")

// UsePrivate is private listening as Go applies it (PrivateService.State,
// then "private:changed"): Source is "setting", "development" or
// "environment". No data until Go answers.
func UsePrivate(cl *Client, core *services.Core) Result[services.PrivateState] {
	return Use(cl, KeyPrivate, func(context.Context) (services.PrivateState, error) {
		if core == nil {
			return services.PrivateState{}, errNoCore
		}
		return services.Service[services.PrivateService](core).State(), nil
	}, Options{Stale: Forever})
}

// PrivateForced reports that SONA_PRIVATE=1 keeps private listening on: the
// switch cannot turn it off.
func PrivateForced(s services.PrivateState) bool { return s.Source == "environment" }

// UseCDM is the Widevine CDM as Go reports it (CDMService.Status, then
// "cdm:status"). State "" means the platform's playback needs no CDM. No data
// until Go answers.
func UseCDM(cl *Client, core *services.Core) Result[services.CDMStatus] {
	return Use(cl, KeyCDM, func(context.Context) (services.CDMStatus, error) {
		if core == nil {
			return services.CDMStatus{}, errNoCore
		}
		return services.Service[services.CDMService](core).Status(), nil
	}, Options{Stale: Forever})
}

// CDMProgress is the running download's progress from 0 to 1; false while its
// size is unknown.
func CDMProgress(s services.CDMStatus) (float64, bool) {
	if s.Total <= 0 {
		return 0, false
	}
	return min(max(float64(s.Received)/float64(s.Total), 0), 1), true
}

// CDMCanRetry reports whether trying again can help: a download that
// failed, or whose last update check failed, on a platform Google makes the
// CDM for.
func CDMCanRetry(s *services.CDMStatus) bool {
	return s != nil && s.Code != "" && s.Code != "unsupported_platform" &&
		(s.State == "failed" || s.State == "ready") && !s.Updating
}

// UseUpdates is the update check's latest result (UpdatesService.Status, then
// "updates:available"; Settings' manual check writes its answer with Set).
func UseUpdates(cl *Client, core *services.Core) Result[services.UpdateStatus] {
	return Use(cl, KeyUpdates, func(context.Context) (services.UpdateStatus, error) {
		if core == nil {
			return services.UpdateStatus{}, errNoCore
		}
		return services.Service[services.UpdatesService](core).Status(), nil
	}, Options{Stale: Forever})
}

// UseReminders is every release reminder set (RemindersService.List, then
// "reminders:changed").
func UseReminders(cl *Client, core *services.Core) Result[[]services.Reminder] {
	return Use(cl, KeyReminders, func(ctx context.Context) ([]services.Reminder, error) {
		if core == nil {
			return nil, errNoCore
		}
		return services.Service[services.RemindersService](core).List(ctx)
	}, Options{Stale: Forever})
}

// ReminderFor is the reminder set for a catalogue album, if any.
func ReminderFor(list []services.Reminder, catalogID string) (services.Reminder, bool) {
	for _, r := range list {
		if r.AlbumID == catalogID {
			return r, true
		}
	}
	return services.Reminder{}, false
}

// Peek is the data cached under key, without starting a fetch or marking the
// query in use (a shortcut's When, an event handler).
func Peek[T any](cl *Client, key Key) (T, bool) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if e := cl.entries[key.id()]; e != nil && e.has {
		v, ok := e.data.(T)
		return v, ok
	}
	var zero T
	return zero, false
}

// SetIfEmpty writes data under key only while the query holds none, so a
// snapshot that arrives after an event never replaces the newer data; it
// reports whether it wrote.
func SetIfEmpty[T any](cl *Client, key Key, v T) bool {
	cl.mu.Lock()
	if e := cl.entries[key.id()]; e != nil && e.has {
		if _, ok := e.data.(T); ok {
			cl.mu.Unlock()
			return false
		}
	}
	cl.setLocked(key, v)
	cl.mu.Unlock()
	cl.post(func() {})
	return true
}
