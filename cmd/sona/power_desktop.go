//go:build darwin || windows

package main

import (
	"sync"

	"github.com/egoist/mygo"

	"github.com/coredev-uk/sona-go/internal/power"
	"github.com/coredev-uk/sona-go/internal/services"
)

// desktopPowerMonitor follows whether the computer runs on battery: MyGo's
// Power.IsOnBattery (IOKit's power sources on macOS, GetSystemPowerStatus's
// AC line on Windows) read on a timer, and at once when the computer wakes
// or the screen unlocks, which is when an unplugged laptop is noticed after
// a lid was shut. MyGo has no change notification for the power source. It
// is a services.PowerMonitor.
type desktopPowerMonitor struct {
	*power.Poller
	offs []func()
	once sync.Once
}

// openDesktopPower is services.Platform.Power on macOS and Windows. The
// system always answers, so it doesn't fail: a desktop computer reports
// mains power.
func openDesktopPower() (services.PowerMonitor, error) {
	p := power.NewPoller(power.PollOptions{OnBattery: mygo.Power.IsOnBattery})
	return &desktopPowerMonitor{Poller: p, offs: []func(){
		mygo.Power.OnResume(p.Refresh),
		mygo.Power.OnUnlockScreen(p.Refresh),
	}}, nil
}

// Close stops following: Run returns.
func (m *desktopPowerMonitor) Close() error {
	m.once.Do(func() {
		for _, off := range m.offs {
			off()
		}
	})
	return m.Poller.Close()
}
