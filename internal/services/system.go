package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth/store"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/playback"
)

// engineDiagnoser is implemented by engines that can check their
// playback stack (the CDM host engine: a probe child loads the CDM a new
// host would load, and GStreamer).
type engineDiagnoser interface {
	Diagnose(ctx context.Context) (playback.Diagnosis, error)
}

// qualitySetter is implemented by the native engine.
type qualitySetter interface {
	SetQuality(applemusic.Quality)
}

// SystemService is settings, diagnostics and app-level actions.
type SystemService struct {
	o *Env

	paintOnce sync.Once
	mu        sync.Mutex
	painted   time.Time
}

func newSystemService(e *Env) *SystemService {
	// The audio quality applies from the next item.
	if q, ok := e.Engine.(qualitySetter); ok {
		e.Settings.OnChange(func(prev, next config.Settings) {
			if prev.AudioQuality != next.AudioQuality {
				q.SetQuality(applemusic.Quality(next.AudioQuality))
			}
		})
	}
	return &SystemService{o: e}
}

// FirstPaint is called by the frontend once the shell has painted. Network
// work (token validation, stale library sync, cache pruning) starts then.
func (s *SystemService) FirstPaint() {
	s.paintOnce.Do(func() {
		now := time.Now()
		s.mu.Lock()
		s.painted = now
		s.mu.Unlock()
		s.o.Logger.Info("startup: first paint", "since_start_ms", now.Sub(s.o.Started).Milliseconds())
		s.o.firstPaint()
	})
}

// Settings returns the current settings.
func (s *SystemService) Settings() config.Settings { return s.o.Settings.Get() }

// UpdateSettings saves next and applies it: theme and reduce-effects
// through "settings:changed", the audio quality from the next item; the
// idle timeout and WebKit toggles on the next start. The volume is the
// player's (the player bar, MPRIS, `sona volume`; PlaybackService saves
// it), so the saved one is kept whatever next carries: a page holding an
// older copy of the settings can't turn the music back up or down.
func (s *SystemService) UpdateSettings(next config.Settings) (config.Settings, error) {
	prev := s.o.Settings.Get()
	saved, err := s.o.Settings.Update(func(st *config.Settings) {
		volume := st.Volume
		*st = next
		st.Volume = volume
	})
	if err != nil {
		return prev, err
	}
	s.o.Emit.Emit(EventSettingsChanged, saved)
	return saved, nil
}

// SetSidebar saves the sidebar's open folders and folded sections, leaving
// every other setting as it is (UpdateSettings replaces them all, so a
// caller's stale copy would undo another control's change).
func (s *SystemService) SetSidebar(sb config.Sidebar) (config.Settings, error) {
	prev := s.o.Settings.Get()
	saved, err := s.o.Settings.Update(func(st *config.Settings) { st.Sidebar = sb })
	if err != nil {
		return prev, err
	}
	s.o.Emit.Emit(EventSettingsChanged, saved)
	return saved, nil
}

// OpenLogFolder opens the log directory in the file manager.
func (s *SystemService) OpenLogFolder() error {
	return s.o.OpenURL("file://" + s.o.Paths.State)
}

// Diagnostics reports versions, the playback stack, storage and paths.
// With the CDM host engine it spawns a short-lived child to check the CDM
// and GStreamer.
func (s *SystemService) Diagnostics(ctx context.Context) (Diagnostics, error) {
	d := Diagnostics{
		App: DiagApp{
			Version:   s.o.Version,
			GoVersion: runtime.Version(),
			Platform:  runtime.GOOS + "/" + runtime.GOARCH,
			Session:   os.Getenv("XDG_SESSION_TYPE"),
			PID:       os.Getpid(),
		},
		ArtworkCacheBytes: s.o.Art.Size(),
		Paths: DiagPaths{
			Config: s.o.Paths.Config, Data: s.o.Paths.Data, Cache: s.o.Paths.Cache, Art: s.o.Paths.Art,
			State: s.o.Paths.State, Settings: s.o.Paths.SettingsFile(), LogFile: s.o.Paths.LogFile(), Queue: s.o.QueueFile,
		},
	}
	s.mu.Lock()
	if !s.painted.IsZero() {
		d.App.StartupMs = s.painted.Sub(s.o.Started).Milliseconds()
	}
	s.mu.Unlock()

	st := s.o.Auth.Status()
	d.Keyring = DiagKeyring{Backend: string(st.Backend), Unencrypted: st.Backend == store.BackendFile}

	for _, name := range []string{
		"WEBKIT_DISABLE_DMABUF_RENDERER", "WEBKIT_DISABLE_COMPOSITING_MODE",
		"GST_PLUGIN_SYSTEM_PATH_1_0", "GST_PLUGIN_PATH_1_0", "WAYLAND_DISPLAY", "GDK_BACKEND",
	} {
		v, ok := os.LookupEnv(name)
		d.Env = append(d.Env, EnvVar{Name: name, Value: v, Set: ok})
	}

	var errs []error
	if err := s.database(ctx, &d.Database); err != nil {
		errs = append(errs, err)
	}
	s.playback(ctx, &d.Playback)
	return d, errors.Join(errs...)
}

func (s *SystemService) database(ctx context.Context, out *DiagDatabase) error {
	out.Path = s.o.Paths.DatabaseFile()
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if fi, err := os.Stat(out.Path + suffix); err == nil {
			out.SizeBytes += fi.Size()
		}
	}
	if err := s.o.DB.R.QueryRowContext(ctx, `SELECT coalesce(max(version_id), 0) FROM goose_db_version WHERE is_applied`).Scan(&out.SchemaVersion); err != nil {
		return fmt.Errorf("schema version: %w", err)
	}
	c, err := s.o.Library.Counts(ctx)
	if err != nil {
		return err
	}
	out.Counts = Counts(c)
	ds, err := s.o.Library.SyncStatus(ctx)
	if err != nil {
		return err
	}
	out.LastSync = syncStatus(ds, false).LastSuccess
	return nil
}

func (s *SystemService) playback(ctx context.Context, out *DiagPlayback) {
	cur := s.o.Settings.Get()
	out.AudioQuality = cur.AudioQuality
	out.Engine = s.o.Engine.Capabilities().Kind
	if out.Engine == playback.KindCDM && s.o.CDM != nil {
		out.CDM = cdmStatus(s.o.CDM.Status())
	}
	d, ok := s.o.Engine.(engineDiagnoser)
	if !ok {
		return
	}
	info, err := d.Diagnose(ctx)
	out.HostRunning, out.HostPID = info.HostRunning, info.HostPID
	out.CDMPath, out.CDMVersion = info.CDMPath, info.CDMVersion
	out.CDMInterface, out.HostInterface = info.CDMInterface, info.HostInterface
	out.GStreamer, out.Decoder, out.Sink = info.GStreamer, info.Decoder, info.Sink
	out.SandboxKind, out.Sandboxed, out.SandboxGaps = info.Sandbox, info.Sandboxed, info.SandboxGaps
	out.CDMOK = err == nil
	out.CDMBlocked = info.Blocked
	out.Realtime, out.RealtimeVia, out.RealtimePriority, out.RealtimeRefusal = info.Realtime, info.RealtimeVia, info.RealtimePriority, info.RealtimeRefusal
	out.CDMError = CodeOf(err)
}
