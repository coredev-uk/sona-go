package services

import (
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/privatemode"
)

// PrivateService is private listening (internal/privatemode) for the
// frontend: the state that applies now and where it comes from, and the
// switch the Settings row, the player bar and the command palette share.
// The tray and `sona private` act on the same Mode, so every change reaches
// the frontend as "private:changed".
type PrivateService struct {
	e *Env
}

func newPrivateService(e *Env) *PrivateService {
	s := &PrivateService{e: e}
	e.Private.OnChange(func(privatemode.State) {
		if e.Emit != nil {
			e.Emit.Emit(EventPrivateChanged, s.State())
		}
	})
	e.Settings.OnChange(func(prev, next config.Settings) {
		if prev.Private == next.Private {
			return
		}
		e.Private.SavedChanged(next.Private.Enabled)
		// The tray and `sona private` save the setting without
		// UpdateSettings: keep the frontend's copy current, so its next
		// save does not write the old value back.
		if e.Emit != nil {
			e.Emit.Emit(EventSettingsChanged, next)
		}
	})
	return s
}

// State returns private listening as it applies now.
func (s *PrivateService) State() PrivateState {
	st := s.e.Private.State()
	return PrivateState{On: st.On, Source: string(st.Source), Saved: s.e.Private.Saved()}
}

// Set turns private listening on or off: the saved setting, or in a
// development build this session only. Turning it off while SONA_PRIVATE=1
// forces it on fails with code "private_forced".
func (s *PrivateService) Set(on bool) (PrivateState, error) {
	_, err := s.e.Private.Set(on)
	return s.State(), err
}
