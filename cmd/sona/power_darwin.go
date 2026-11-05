//go:build darwin

package main

import (
	"sync"

	"github.com/egoist/mygo"

	"github.com/coredev-uk/sona-go/internal/power"
	"github.com/coredev-uk/sona-go/internal/services"
)

// macPowerMonitor follows whether the Mac runs on battery: MyGo's
// Power.IsOnBattery (IOKit's power sources) read on a timer, and at once when
// the Mac wakes or the screen unlocks, which is when an unplugged laptop is
// noticed after a lid was shut. MyGo has no change notification for the
// power source. It is a services.PowerMonitor.
type macPowerMonitor struct {
	*power.Poller
	offs []func()
	once sync.Once
}

// openMacPower is services.Platform.Power on macOS. IOKit always answers, so
// it doesn't fail: a desktop Mac reports mains power.
func openMacPower() (services.PowerMonitor, error) {
	p := power.NewPoller(power.PollOptions{OnBattery: mygo.Power.IsOnBattery})
	return &macPowerMonitor{Poller: p, offs: []func(){
		mygo.Power.OnResume(p.Refresh),
		mygo.Power.OnUnlockScreen(p.Refresh),
	}}, nil
}

// Close stops following: Run returns.
func (m *macPowerMonitor) Close() error {
	m.once.Do(func() {
		for _, off := range m.offs {
			off()
		}
	})
	return m.Poller.Close()
}
