package services

import (
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdm"
)

// CDMService reports the Widevine CDM the playback engine loads, which
// Sona downloads from Google and keeps up to date by itself (no prompt),
// and retries a failed download. The download, or the update check when
// one is installed, starts once the network opens after the first paint;
// "cdm:status" follows it. Without a CDM engine (no playback on this
// platform) it reports nothing.
type CDMService struct {
	e  *Env
	on bool
}

func newCDMService(e *Env) *CDMService {
	s := &CDMService{e: e, on: e.CDM != nil && e.Engine.Capabilities().Kind == playback.KindCDM}
	if !s.on {
		return s
	}
	e.OnClose(e.CDM.Subscribe(func(st cdm.Status) { e.Emit.Emit(EventCDMStatus, cdmStatus(st)) }))
	e.OnNetwork(func() {
		e.Logger.Info("startup: widevine CDM check starts", "since_start_ms", time.Since(e.Started).Milliseconds())
		e.CDM.Start()
	})
	return s
}

// Status returns the CDM's state.
func (s *CDMService) Status() CDMStatus {
	if !s.on {
		return CDMStatus{}
	}
	return cdmStatus(s.e.CDM.Status())
}

// Retry checks for the CDM again now: after a failed download, or to look
// for an update. With the developer override there is nothing to fetch.
func (s *CDMService) Retry() error {
	if !s.on {
		return unsupported("the Widevine CDM")
	}
	s.e.CDM.Retry()
	return nil
}
