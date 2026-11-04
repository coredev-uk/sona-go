package onboarding

import (
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// settingsNow is the cached settings without starting a fetch (a click's
// handler, an event); false until some view has read them.
func (o *onboarding) settingsNow() (config.Settings, bool) {
	return store.Peek[config.Settings](o.env.Queries, store.KeySettings)
}

// save applies change to a copy of the settings and saves it (kit.UpdateSettings,
// v0.2.0's useUpdateSettings): every control of the setup saves itself with
// it, at once, and a failure puts the old value back with a toast.
func (o *onboarding) save(change func(*config.Settings)) {
	kit.UpdateSettings(o.env.Queries, o.env.Core, change)
}

// complete marks the setup finished (config.Onboarding.Completed) through
// the settings service, on the latest settings Go sent, so the main window no
// longer opens on it. Already marked, nothing is saved (complete.ts'
// useCompleteSetup). Settings.Get cannot fail here, so the "Couldn't save
// the setting" toast of a failed read has nothing to say; a failed save
// toasts through kit.UpdateSettings and the next call tries again.
func (o *onboarding) complete() {
	s, ok := o.settingsNow()
	if !ok {
		s = services.Service[services.SystemService](o.env.Core).Settings()
	}
	if s.Onboarding.Completed {
		return
	}
	o.save(func(s *config.Settings) { s.Onboarding.Completed = true })
}

// leave closes the setup, marked finished, onto the page behind it (complete.ts'
// useLeaveSetup). The layer's overlay gives the focus back to what had it
// when it opened.
func (o *onboarding) leave() {
	o.complete()
	o.env.Surfaces.CloseSetup()
}
