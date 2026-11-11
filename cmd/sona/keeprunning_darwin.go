//go:build darwin

package main

import (
	"sync"

	"github.com/egoist/mygo"
)

// macKeeper keeps Sona out of App Nap while music plays or loads
// (services.RunningKeeper). The audio comes out of the CDM host child, which
// macOS does not nap with the parent; the parent's timers (preload, scrobble,
// the queue) and its media-key handling are what a hidden window's nap would
// throttle. MyGo's Power.KeepRunning stops that without keeping the computer
// awake, so it is independent of the sleep inhibitor (inhibit_darwin.go),
// which is a setting.
type macKeeper struct {
	why  func() string
	take func(reason string) (release func())

	mu      sync.Mutex
	release func() // nil when released
}

// newMacKeeper is services.Platform.KeepRunning on macOS; why is the reason
// the system lists, read when the hold is taken.
func newMacKeeper(why func() string) *macKeeper {
	return &macKeeper{why: why, take: mygo.Power.KeepRunning}
}

// Hold takes the hold unless it is held.
func (k *macKeeper) Hold() {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.release == nil {
		k.release = k.take(k.why())
	}
}

// Release drops the hold if it is held.
func (k *macKeeper) Release() {
	k.mu.Lock()
	release := k.release
	k.release = nil
	k.mu.Unlock()
	if release != nil {
		release()
	}
}

// Close releases the hold.
func (k *macKeeper) Close() { k.Release() }
