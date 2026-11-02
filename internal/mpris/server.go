package mpris

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"reflect"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

// ErrNameTaken is returned by New when another connection owns BusName.
var ErrNameTaken = errors.New("mpris: bus name already owned")

const (
	// callTimeout bounds each Player call made on behalf of a D-Bus client.
	callTimeout = 10 * time.Second
	// seekThreshold is how far Update's position may drift from uninterrupted
	// playback before it counts as a discontinuity and emits Seeked.
	seekThreshold = time.Second

	// The D-Bus errors MPRIS calls fail with. Their names and messages are
	// for the programs calling (media key daemons, playerctl), not shown
	// to the user, so they stay in English rather than in Go's message
	// catalogue (internal/i18n/l10n).
	errUnknownInterface = "org.freedesktop.DBus.Error.UnknownInterface"
	errUnknownProperty  = "org.freedesktop.DBus.Error.UnknownProperty"
	errReadOnly         = "org.freedesktop.DBus.Error.PropertyReadOnly"
	errInvalidArgs      = "org.freedesktop.DBus.Error.InvalidArgs"
	errNotSupported     = "org.freedesktop.DBus.Error.NotSupported"
)

// Server exports the MPRIS interfaces for one Player on one connection.
type Server struct {
	conn   *dbus.Conn
	player Player
	log    *slog.Logger
	ctx    context.Context
	cancel context.CancelFunc

	updateMu sync.Mutex // orders Update's signal emission

	mu     sync.Mutex
	snap   Snapshot
	closed bool

	closeOnce sync.Once
	closeErr  error
}

// New exports the MPRIS objects on conn, seeds state from p.Snapshot() and
// requests BusName (without queueing). It returns ErrNameTaken if another
// connection already owns the name. conn stays owned by the caller: Close
// releases the name and unexports, but does not close the connection.
// A nil log discards output.
func New(conn *dbus.Conn, p Player, log *slog.Logger) (*Server, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		conn:   conn,
		player: p,
		log:    log,
		ctx:    ctx,
		cancel: cancel,
		snap:   normalize(p.Snapshot(), time.Now()),
	}
	if err := s.export(); err != nil {
		s.unexport()
		cancel()
		return nil, fmt.Errorf("mpris: export: %w", err)
	}
	reply, err := conn.RequestName(BusName, dbus.NameFlagDoNotQueue)
	if err != nil {
		s.unexport()
		cancel()
		return nil, fmt.Errorf("mpris: request %s: %w", BusName, err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner && reply != dbus.RequestNameReplyAlreadyOwner {
		s.unexport()
		cancel()
		return nil, ErrNameTaken
	}
	return s, nil
}

func (s *Server) export() error {
	root := map[string]any{
		"Raise": func() *dbus.Error { return s.invoke("Raise", s.player.Raise) },
		"Quit":  func() *dbus.Error { return s.invoke("Quit", s.player.Quit) },
	}
	player := map[string]any{
		"Next":        func() *dbus.Error { return s.control("Next", canNext, s.player.Next) },
		"Previous":    func() *dbus.Error { return s.control("Previous", canPrevious, s.player.Previous) },
		"Pause":       func() *dbus.Error { return s.control("Pause", canPause, s.player.Pause) },
		"PlayPause":   func() *dbus.Error { return s.control("PlayPause", canPlayPause, s.player.PlayPause) },
		"Stop":        func() *dbus.Error { return s.control("Stop", nil, s.player.Stop) },
		"Play":        func() *dbus.Error { return s.control("Play", canPlay, s.player.Play) },
		"Seek":        s.seek,
		"SetPosition": s.setPosition,
		"OpenUri": func(string) *dbus.Error {
			return dbus.NewError(errNotSupported, []any{"Sona does not open URIs over MPRIS"})
		},
	}
	props := map[string]any{
		"Get":    s.getProp,
		"GetAll": s.getAllProps,
		"Set":    s.setProp,
	}
	if err := s.conn.ExportMethodTable(root, ObjectPath, rootIface); err != nil {
		return err
	}
	if err := s.conn.ExportMethodTable(player, ObjectPath, playerIface); err != nil {
		return err
	}
	if err := s.conn.ExportMethodTable(props, ObjectPath, propsIface); err != nil {
		return err
	}
	return s.conn.Export(introspect.Introspectable(introspectXML), ObjectPath, introIface)
}

func (s *Server) unexport() {
	for _, iface := range []string{rootIface, playerIface, propsIface, introIface} {
		_ = s.conn.Export(nil, ObjectPath, iface)
	}
}

// Close releases BusName and unexports the objects. Pending Player calls see
// their context cancelled. Safe to call more than once.
func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		s.cancel()
		if _, err := s.conn.ReleaseName(BusName); err != nil {
			s.closeErr = fmt.Errorf("mpris: release %s: %w", BusName, err)
		}
		s.unexport()
	})
	return s.closeErr
}

// Update publishes snap. It emits one PropertiesChanged on
// org.mpris.MediaPlayer2.Player carrying only the properties whose values
// changed (Position never; CanControl is not signalled, per spec), and emits
// Seeked when the position jumps relative to uninterrupted playback since the
// previous snapshot, or a new track starts away from zero. Safe for
// concurrent use; a no-op after Close.
func (s *Server) Update(snap Snapshot) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	now := time.Now()
	next := normalize(snap, now)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	prev := s.snap
	s.snap = next
	s.mu.Unlock()

	if changed := diffProps(signalledProps(prev), signalledProps(next)); len(changed) > 0 {
		s.emit(propsIface+".PropertiesChanged", playerIface, changed, []string{})
	}
	if seeked(prev, next) {
		s.emit(playerIface+".Seeked", position(next, now))
	}
}

func (s *Server) emit(name string, values ...any) {
	if err := s.conn.Emit(ObjectPath, name, values...); err != nil {
		s.log.Debug("mpris: emit failed", "signal", name, "err", err)
	}
}

func (s *Server) snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snap
}

// invoke runs fn with a bounded context and maps its error onto D-Bus.
func (s *Server) invoke(method string, fn func(context.Context) error) *dbus.Error {
	ctx, cancel := context.WithTimeout(s.ctx, callTimeout)
	defer cancel()
	if err := fn(ctx); err != nil {
		s.log.Warn("mpris: player call failed", "method", method, "err", err)
		return dbus.MakeFailedError(err)
	}
	return nil
}

// control gates a Player method on CanControl and allowed, then invokes it.
// Gated calls are silent no-ops.
func (s *Server) control(method string, allowed func(Snapshot) bool, fn func(context.Context) error) *dbus.Error {
	snap := s.snapshot()
	if !snap.CanControl || (allowed != nil && !allowed(snap)) {
		s.log.Debug("mpris: call ignored", "method", method)
		return nil
	}
	return s.invoke(method, fn)
}

func canNext(s Snapshot) bool     { return s.CanGoNext }
func canPrevious(s Snapshot) bool { return s.CanGoPrevious }
func canPause(s Snapshot) bool    { return s.CanPause }
func canPlay(s Snapshot) bool     { return s.CanPlay }
func canPlayPause(s Snapshot) bool {
	if s.Status == StatusPlaying {
		return s.CanPause
	}
	return s.CanPlay
}
func canSeek(s Snapshot) bool { return s.CanSeek && s.Track != nil }

func (s *Server) seek(offsetUs int64) *dbus.Error {
	return s.control("Seek", canSeek, func(ctx context.Context) error {
		return s.player.Seek(ctx, time.Duration(offsetUs)*time.Microsecond)
	})
}

// setPosition follows the spec: requests for a track that is no longer
// current, negative positions and positions past the end are ignored.
func (s *Server) setPosition(track dbus.ObjectPath, posUs int64) *dbus.Error {
	snap := s.snapshot()
	pos := time.Duration(posUs) * time.Microsecond
	allowed := func(snap Snapshot) bool {
		return canSeek(snap) && TrackPath(snap.Track.ID) == track &&
			pos >= 0 && (snap.Track.Length <= 0 || pos <= snap.Track.Length)
	}
	if !snap.CanControl || !allowed(snap) {
		s.log.Debug("mpris: call ignored", "method", "SetPosition")
		return nil
	}
	id := snap.Track.ID
	return s.invoke("SetPosition", func(ctx context.Context) error {
		return s.player.SetPosition(ctx, id, pos)
	})
}

func (s *Server) getProp(iface, name string) (dbus.Variant, *dbus.Error) {
	all, derr := s.getAllProps(iface)
	if derr != nil {
		return dbus.Variant{}, derr
	}
	v, ok := all[name]
	if !ok {
		return dbus.Variant{}, dbus.NewError(errUnknownProperty, []any{fmt.Sprintf("%s has no property %q", iface, name)})
	}
	return v, nil
}

func (s *Server) getAllProps(iface string) (map[string]dbus.Variant, *dbus.Error) {
	switch iface {
	case rootIface:
		return rootProps(), nil
	case playerIface:
		snap := s.snapshot()
		props := signalledProps(snap)
		props["Position"] = dbus.MakeVariant(position(snap, time.Now()))
		props["CanControl"] = dbus.MakeVariant(snap.CanControl)
		return props, nil
	}
	return nil, dbus.NewError(errUnknownInterface, []any{fmt.Sprintf("unknown interface %q", iface)})
}

func (s *Server) setProp(iface, name string, value dbus.Variant) *dbus.Error {
	if _, derr := s.getProp(iface, name); derr != nil {
		return derr
	}
	if iface != playerIface {
		return readOnly(iface, name)
	}
	switch name {
	case "LoopStatus":
		v, ok := value.Value().(string)
		l := LoopStatus(v)
		if !ok || (l != LoopNone && l != LoopTrack && l != LoopPlaylist) {
			return invalidArgs("LoopStatus must be None, Track or Playlist")
		}
		return s.control("Set LoopStatus", nil, func(ctx context.Context) error { return s.player.SetLoopStatus(ctx, l) })
	case "Shuffle":
		v, ok := value.Value().(bool)
		if !ok {
			return invalidArgs("Shuffle must be a boolean")
		}
		return s.control("Set Shuffle", nil, func(ctx context.Context) error { return s.player.SetShuffle(ctx, v) })
	case "Volume":
		v, ok := value.Value().(float64)
		if !ok {
			return invalidArgs("Volume must be a double")
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return invalidArgs("Volume must be a finite number")
		}
		v = clamp01(v)
		return s.control("Set Volume", nil, func(ctx context.Context) error { return s.player.SetVolume(ctx, v) })
	case "Rate":
		// Only normal speed is supported; MinimumRate = MaximumRate = 1.
		if _, ok := value.Value().(float64); !ok {
			return invalidArgs("Rate must be a double")
		}
		return nil
	}
	return readOnly(iface, name)
}

func readOnly(iface, name string) *dbus.Error {
	return dbus.NewError(errReadOnly, []any{fmt.Sprintf("%s.%s is read-only", iface, name)})
}

func invalidArgs(msg string) *dbus.Error {
	return dbus.NewError(errInvalidArgs, []any{msg})
}

func rootProps() map[string]dbus.Variant {
	return map[string]dbus.Variant{
		"CanQuit":             dbus.MakeVariant(true),
		"CanRaise":            dbus.MakeVariant(true),
		"HasTrackList":        dbus.MakeVariant(false),
		"Identity":            dbus.MakeVariant(Identity),
		"DesktopEntry":        dbus.MakeVariant(DesktopEntry),
		"SupportedUriSchemes": dbus.MakeVariant([]string{}),
		"SupportedMimeTypes":  dbus.MakeVariant([]string{}),
	}
}

// signalledProps is every Player property that PropertiesChanged carries:
// all of them except Position and CanControl.
func signalledProps(s Snapshot) map[string]dbus.Variant {
	return map[string]dbus.Variant{
		"PlaybackStatus": dbus.MakeVariant(string(s.Status)),
		"LoopStatus":     dbus.MakeVariant(string(s.Loop)),
		"Rate":           dbus.MakeVariant(1.0),
		"MinimumRate":    dbus.MakeVariant(1.0),
		"MaximumRate":    dbus.MakeVariant(1.0),
		"Shuffle":        dbus.MakeVariant(s.Shuffle),
		"Metadata":       dbus.MakeVariant(metadata(s.Track)),
		"Volume":         dbus.MakeVariant(s.Volume),
		"CanGoNext":      dbus.MakeVariant(s.CanGoNext),
		"CanGoPrevious":  dbus.MakeVariant(s.CanGoPrevious),
		"CanPlay":        dbus.MakeVariant(s.CanPlay),
		"CanPause":       dbus.MakeVariant(s.CanPause),
		"CanSeek":        dbus.MakeVariant(s.CanSeek),
	}
}

func diffProps(prev, next map[string]dbus.Variant) map[string]dbus.Variant {
	changed := maps.Clone(next)
	maps.DeleteFunc(changed, func(k string, v dbus.Variant) bool {
		return reflect.DeepEqual(prev[k].Value(), v.Value())
	})
	return changed
}

// metadata builds the MPRIS Metadata map; empty fields are omitted.
func metadata(t *Track) map[string]dbus.Variant {
	if t == nil {
		return map[string]dbus.Variant{"mpris:trackid": dbus.MakeVariant(NoTrack)}
	}
	m := map[string]dbus.Variant{"mpris:trackid": dbus.MakeVariant(TrackPath(t.ID))}
	if t.Length > 0 {
		m["mpris:length"] = dbus.MakeVariant(t.Length.Microseconds())
	}
	str := func(key, v string) {
		if v != "" {
			m[key] = dbus.MakeVariant(v)
		}
	}
	list := func(key, v string) {
		if v != "" {
			m[key] = dbus.MakeVariant([]string{v})
		}
	}
	str("xesam:title", t.Title)
	list("xesam:artist", t.Artist)
	str("xesam:album", t.Album)
	list("xesam:albumArtist", t.AlbumArtist)
	str("mpris:artUrl", t.ArtURL)
	str("xesam:url", t.URL)
	return m
}

// normalize fills defaults and enforces the CanControl rule.
func normalize(s Snapshot, now time.Time) Snapshot {
	switch s.Status {
	case StatusPlaying, StatusPaused, StatusStopped:
	default:
		s.Status = StatusStopped
	}
	switch s.Loop {
	case LoopNone, LoopTrack, LoopPlaylist:
	default:
		s.Loop = LoopNone
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = now
	}
	s.Volume = clamp01(s.Volume)
	s.Position = max(s.Position, 0)
	if s.Track != nil {
		t := *s.Track
		s.Track = &t
	}
	if !s.CanControl {
		s.CanGoNext, s.CanGoPrevious, s.CanPlay, s.CanPause, s.CanSeek = false, false, false, false, false
	}
	return s
}

// at extrapolates the snapshot position to now while playing, clamped to the
// track length.
func at(s Snapshot, now time.Time) time.Duration {
	pos := s.Position
	if s.Status == StatusPlaying {
		if d := now.Sub(s.UpdatedAt); d > 0 {
			pos += d
		}
	}
	if s.Track != nil && s.Track.Length > 0 {
		pos = min(pos, s.Track.Length)
	}
	return pos
}

// position is the MPRIS Position in microseconds.
func position(s Snapshot, now time.Time) int64 {
	return at(s, now).Microseconds()
}

// seeked reports whether next's position is discontinuous with prev.
func seeked(prev, next Snapshot) bool {
	if next.Track == nil {
		return false
	}
	if prev.Track == nil || prev.Track.ID != next.Track.ID {
		return next.Position > seekThreshold
	}
	drift := next.Position - at(prev, next.UpdatedAt)
	return drift > seekThreshold || drift < -seekThreshold
}

// clamp01 clamps v to [0, 1]; NaN, which min and max pass through, is 0.
func clamp01(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	return min(max(v, 0), 1)
}
