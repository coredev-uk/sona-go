//go:build darwin

package main

import (
	"context"
	"sync"

	"github.com/egoist/mygo"
)

// backendPowerAssertion is IntegrationsStatus.Inhibit while the Mac is held
// awake: a power-management assertion that stops idle system sleep (MyGo's
// Power.KeepAwake: NSProcessInfo's beginActivity, an IOPMAssertion that
// `pmset -g assertions` lists under Sona with the reason). Needs no app bundle.
const backendPowerAssertion = "power-assertion"

// macInhibitor keeps the Mac from sleeping on its own while held. The
// display may still sleep and the screen lock, as on Linux (the inhibitor
// there blocks suspend only). It is a services.SleepInhibitor.
type macInhibitor struct {
	why       func() string
	keepAwake func(reason string) (release func())

	mu      sync.Mutex
	release func() // nil when released
}

// newMacInhibitor is services.Platform.Inhibitor on macOS; why is the reason
// the system lists, read when the assertion is taken.
func newMacInhibitor(why func() string) *macInhibitor {
	return &macInhibitor{why: why, keepAwake: func(reason string) func() {
		return mygo.Power.KeepAwake(reason, false)
	}}
}

// Acquire takes the assertion unless it is held.
func (i *macInhibitor) Acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.release != nil {
		return nil
	}
	i.release = i.keepAwake(i.why())
	return nil
}

// Release drops the assertion if it is held.
func (i *macInhibitor) Release() error {
	i.mu.Lock()
	release := i.release
	i.release = nil
	i.mu.Unlock()
	if release != nil {
		release()
	}
	return nil
}

// Backend is backendPowerAssertion while held, "" when released.
func (i *macInhibitor) Backend() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.release == nil {
		return ""
	}
	return backendPowerAssertion
}

// Close releases the assertion.
func (i *macInhibitor) Close() error { return i.Release() }
