//go:build darwin || windows

package main

import (
	"context"

	"github.com/egoist/mygo"
)

// backendPowerAssertion is IntegrationsStatus.Inhibit while the computer is
// held awake by MyGo's Power.KeepAwake. Needs no app bundle.
//
//   - macOS: NSProcessInfo's beginActivity, an IOPMAssertion that
//     `pmset -g assertions` lists under Sona with the reason.
//   - Windows: a power request (PowerCreateRequest, PowerSetRequest with
//     SystemRequired) that `powercfg /requests` lists under SYSTEM with the
//     reason.
const backendPowerAssertion = "power-assertion"

// desktopInhibitor keeps the computer from sleeping on its own while held.
// The display may still sleep and the screen lock, as on Linux (the
// inhibitor there blocks suspend only); Full-Screen Lyrics hold the display
// on themselves while they are open (store.Window.KeepScreenOn). It is a
// services.SleepInhibitor.
type desktopInhibitor struct{ holder }

// newDesktopInhibitor is services.Platform.Inhibitor on macOS and Windows;
// why is the reason the system lists, read when the hold is taken.
func newDesktopInhibitor(why func() string) *desktopInhibitor {
	return &desktopInhibitor{holder{why: why, take: func(reason string) func() {
		return mygo.Power.KeepAwake(reason, false)
	}}}
}

// Acquire takes the hold unless it is held. Where the system refuses it
// (MyGo returns no release), nothing is held and Backend says so.
func (i *desktopInhibitor) Acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	i.hold()
	return nil
}

// Release drops the hold if it is held.
func (i *desktopInhibitor) Release() error {
	i.drop()
	return nil
}

// Backend is backendPowerAssertion while held, "" when released.
func (i *desktopInhibitor) Backend() string {
	if !i.held() {
		return ""
	}
	return backendPowerAssertion
}

// Close releases the hold.
func (i *desktopInhibitor) Close() error { return i.Release() }
