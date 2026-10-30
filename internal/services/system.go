package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/auth/store"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
	"github.com/coredev-uk/sona-go/internal/playback/widevine"
)

// probeTimeout bounds the CDM host probe behind Diagnostics.
const probeTimeout = 15 * time.Second

// engineHost is implemented by the native engine: the running child.
type engineHost interface {
	Host() (cdmhost.Info, bool)
}

// SystemService is settings, diagnostics and app-level actions.
type SystemService struct {
	o *Env

	paintOnce sync.Once
	mu        sync.Mutex
	painted   time.Time
}

func newSystemService(e *Env) *SystemService { return &SystemService{o: e} }

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

// UpdateSettings saves next and applies it: volume at once, theme and
// reduce-effects through "settings:changed"; the CDM path, idle timeout
// and WebKit toggles on the next start.
func (s *SystemService) UpdateSettings(ctx context.Context, next config.Settings) (config.Settings, error) {
	prev := s.o.Settings.Get()
	saved, err := s.o.Settings.Update(func(st *config.Settings) { *st = next })
	if err != nil {
		return prev, err
	}
	if saved.Volume != prev.Volume {
		if err := s.o.Coord.SetVolume(ctx, saved.Volume); err != nil {
			s.o.Logger.Warn("settings: apply volume", "err", err)
		}
	}
	s.o.Emit.Emit(EventSettingsChanged, saved)
	return saved, nil
}

// ClearCache deletes cached artwork and cached catalog responses. Library
// data and settings are kept.
func (s *SystemService) ClearCache(ctx context.Context) (ClearResult, error) {
	var out ClearResult
	before := s.o.Art.Size()
	errArt := s.o.Art.Clear()
	out.ArtworkBytes = before - s.o.Art.Size()
	res, errDB := s.o.DB.W.ExecContext(ctx, `DELETE FROM catalog_cache`)
	if errDB == nil {
		out.CatalogEntries, _ = res.RowsAffected()
	}
	s.o.Logger.Info("cache cleared", "artwork_bytes", out.ArtworkBytes, "catalog_entries", out.CatalogEntries)
	return out, errors.Join(errArt, errDB)
}

// OpenLogFolder opens the log directory in the file manager.
func (s *SystemService) OpenLogFolder() error {
	return s.o.OpenURL("file://" + s.o.Paths.State)
}

// Diagnostics reports versions, the playback stack, storage and paths. It
// spawns a short-lived CDM host child to check the CDM and GStreamer.
func (s *SystemService) Diagnostics(ctx context.Context) (Diagnostics, error) {
	d := Diagnostics{
		App: DiagApp{
			Version:   s.o.Version,
			GoVersion: runtime.Version(),
			Platform:  runtime.GOOS + "/" + runtime.GOARCH,
			Session:   os.Getenv("XDG_SESSION_TYPE"),
			PID:       os.Getpid(),
		},
		WebKit:            DiagWebKit{Version: s.o.WebKit},
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
		"WEBKIT_DISABLE_DMABUF_RENDERER", "WEBKIT_DISABLE_COMPOSITING_MODE", widevine.EnvCDM,
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
	override := s.o.Settings.Get().CDMPath
	if h, ok := s.o.Engine.(engineHost); ok {
		if info, running := h.Host(); running {
			out.HostRunning, out.HostPID = true, info.PID
		}
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	info, err := cdmhost.Probe(ctx, cdmhost.Options{CDMPath: override, Logger: s.o.Logger})
	out.CDMPath, out.CDMVersion = info.CDMPath, info.CDMVersion
	out.CDMInterface, out.HostInterface = info.CDMInterface, info.HostInterface
	out.GStreamer, out.Decoder, out.Sink = info.GStreamer, info.Decoder, info.Sink
	out.CDMOK = err == nil
	if err != nil {
		out.CDMError = err.Error()
		if out.CDMPath == "" {
			// The child never got far enough to say; report what discovery finds.
			if p, v, ferr := widevine.FindCDM(override); ferr == nil {
				out.CDMPath, out.CDMVersion = p, v
			}
		}
	}
}
