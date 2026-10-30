package mpris

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/coredev-uk/sona-go/internal/app/dbustest"
)

type call struct {
	name string
	args []any
}

// fakePlayer records every call it receives.
type fakePlayer struct {
	calls chan call
	snap  Snapshot
}

func newFake(snap Snapshot) *fakePlayer {
	return &fakePlayer{calls: make(chan call, 16), snap: snap}
}

func (f *fakePlayer) rec(name string, args ...any) error {
	f.calls <- call{name, args}
	return nil
}

func (f *fakePlayer) PlayPause(context.Context) error { return f.rec("PlayPause") }
func (f *fakePlayer) Play(context.Context) error      { return f.rec("Play") }
func (f *fakePlayer) Pause(context.Context) error     { return f.rec("Pause") }
func (f *fakePlayer) Stop(context.Context) error      { return f.rec("Stop") }
func (f *fakePlayer) Next(context.Context) error      { return f.rec("Next") }
func (f *fakePlayer) Previous(context.Context) error  { return errors.New("no previous") }
func (f *fakePlayer) Seek(_ context.Context, d time.Duration) error {
	return f.rec("Seek", d)
}
func (f *fakePlayer) SetPosition(_ context.Context, id string, d time.Duration) error {
	return f.rec("SetPosition", id, d)
}
func (f *fakePlayer) SetVolume(_ context.Context, v float64) error { return f.rec("SetVolume", v) }
func (f *fakePlayer) SetLoopStatus(_ context.Context, l LoopStatus) error {
	return f.rec("SetLoopStatus", l)
}
func (f *fakePlayer) SetShuffle(_ context.Context, on bool) error { return f.rec("SetShuffle", on) }
func (f *fakePlayer) Raise(context.Context) error                 { return f.rec("Raise") }
func (f *fakePlayer) Quit(context.Context) error                  { return f.rec("Quit") }
func (f *fakePlayer) Snapshot() Snapshot                          { return f.snap }

func (f *fakePlayer) next(t *testing.T) call {
	t.Helper()
	select {
	case c := <-f.calls:
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("no call reached the player")
		return call{}
	}
}

func (f *fakePlayer) none(t *testing.T) {
	t.Helper()
	select {
	case c := <-f.calls:
		t.Fatalf("unexpected call %s%v", c.name, c.args)
	default:
	}
}

var song = &Track{
	ID:          "1440833098",
	Title:       "Come Together",
	Artist:      "The Beatles",
	Album:       "Abbey Road",
	AlbumArtist: "The Beatles",
	Length:      259 * time.Second,
	ArtURL:      "file:///home/u/.cache/sona/art/abc.jpg",
	URL:         "https://music.apple.com/us/album/abbey-road/1441164426?i=1440833098",
}

func playing() Snapshot {
	return Snapshot{
		Status: StatusPlaying, Loop: LoopPlaylist, Volume: 0.5,
		Position: 10 * time.Second, UpdatedAt: time.Now(), Track: song,
		CanGoNext: true, CanGoPrevious: true, CanPlay: true, CanPause: true, CanSeek: true, CanControl: true,
	}
}

type harness struct {
	srv    *Server
	fake   *fakePlayer
	client *dbus.Conn
	obj    dbus.BusObject
}

func setup(t *testing.T, snap Snapshot) *harness {
	t.Helper()
	addr := dbustest.Start(t)
	fake := newFake(snap)
	srv, err := New(dbustest.Conn(t, addr), fake, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	client := dbustest.Conn(t, addr)
	return &harness{srv, fake, client, client.Object(BusName, ObjectPath)}
}

func (h *harness) getAll(t *testing.T, iface string) map[string]dbus.Variant {
	t.Helper()
	var props map[string]dbus.Variant
	if err := h.obj.Call(propsIface+".GetAll", 0, iface).Store(&props); err != nil {
		t.Fatalf("GetAll %s: %v", iface, err)
	}
	return props
}

func (h *harness) signals(t *testing.T) <-chan *dbus.Signal {
	t.Helper()
	if err := h.client.AddMatchSignal(dbus.WithMatchObjectPath(ObjectPath)); err != nil {
		t.Fatal(err)
	}
	ch := make(chan *dbus.Signal, 16)
	h.client.Signal(ch)
	return ch
}

func nextSignal(t *testing.T, ch <-chan *dbus.Signal) *dbus.Signal {
	t.Helper()
	for {
		select {
		case s := <-ch:
			if s.Name == "org.freedesktop.DBus.NameAcquired" {
				continue
			}
			return s
		case <-time.After(5 * time.Second):
			t.Fatal("no signal")
			return nil
		}
	}
}

func TestGetAll(t *testing.T) {
	h := setup(t, playing())

	root := h.getAll(t, rootIface)
	for k, want := range map[string]any{
		"Identity": "Sona", "DesktopEntry": "dev.coredev.Sona", "CanQuit": true, "CanRaise": true, "HasTrackList": false,
	} {
		if got := root[k].Value(); got != want {
			t.Errorf("%s = %v, want %v", k, got, want)
		}
	}

	p := h.getAll(t, playerIface)
	for k, want := range map[string]any{
		"PlaybackStatus": "Playing", "LoopStatus": "Playlist", "Rate": 1.0, "Shuffle": false,
		"Volume": 0.5, "CanGoNext": true, "CanSeek": true, "CanControl": true,
	} {
		if got := p[k].Value(); got != want {
			t.Errorf("%s = %v, want %v", k, got, want)
		}
	}
	md := p["Metadata"].Value().(map[string]dbus.Variant)
	wantMD := map[string]any{
		"mpris:trackid":     dbus.ObjectPath("/dev/coredev/sona/track/1440833098"),
		"mpris:length":      int64(259_000_000),
		"xesam:title":       "Come Together",
		"xesam:artist":      []string{"The Beatles"},
		"xesam:album":       "Abbey Road",
		"xesam:albumArtist": []string{"The Beatles"},
		"mpris:artUrl":      song.ArtURL,
		"xesam:url":         song.URL,
	}
	if len(md) != len(wantMD) {
		t.Errorf("metadata keys = %v", md)
	}
	for k, want := range wantMD {
		if got := md[k].Value(); !reflect.DeepEqual(got, want) {
			t.Errorf("metadata %s = %#v, want %#v", k, got, want)
		}
	}

	// Position is extrapolated at query time while playing.
	pos := time.Duration(p["Position"].Value().(int64)) * time.Microsecond
	if pos < 10*time.Second || pos > 12*time.Second {
		t.Errorf("Position = %v, want ~10s", pos)
	}
	time.Sleep(300 * time.Millisecond)
	v, err := h.obj.GetProperty(playerIface + ".Position")
	if err != nil {
		t.Fatal(err)
	}
	if later := time.Duration(v.Value().(int64)) * time.Microsecond; later < pos+250*time.Millisecond {
		t.Errorf("Position did not advance: %v then %v", pos, later)
	}
}

func TestGetErrors(t *testing.T) {
	h := setup(t, Snapshot{})
	var dErr dbus.Error
	err := h.obj.Call(propsIface+".Get", 0, playerIface, "Nope").Err
	if !errors.As(err, &dErr) || dErr.Name != errUnknownProperty {
		t.Errorf("unknown property: %v", err)
	}
	err = h.obj.Call(propsIface+".GetAll", 0, "org.example.Nope").Err
	if !errors.As(err, &dErr) || dErr.Name != errUnknownInterface {
		t.Errorf("unknown interface: %v", err)
	}
	err = h.obj.SetProperty(playerIface+".PlaybackStatus", dbus.MakeVariant("Playing"))
	if !errors.As(err, &dErr) || dErr.Name != errReadOnly {
		t.Errorf("read-only: %v", err)
	}
	md, err := h.obj.GetProperty(playerIface + ".Metadata")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]dbus.Variant{"mpris:trackid": dbus.MakeVariant(NoTrack)}
	if got := md.Value(); !reflect.DeepEqual(got, want) {
		t.Errorf("empty metadata = %v", got)
	}
}

func TestIntrospect(t *testing.T) {
	h := setup(t, Snapshot{})
	var xml string
	if err := h.obj.Call(introIface+".Introspect", 0).Store(&xml); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{rootIface, playerIface, propsIface, `name="Seeked"`, `name="SetPosition"`} {
		if !strings.Contains(xml, want) {
			t.Errorf("introspection lacks %s", want)
		}
	}
}

func TestUpdateEmitsOnlyChanges(t *testing.T) {
	snap := playing()
	h := setup(t, snap)
	sigs := h.signals(t)

	snap.Status = StatusPaused
	snap.Position += 500 * time.Millisecond
	snap.UpdatedAt = snap.UpdatedAt.Add(500 * time.Millisecond)
	h.srv.Update(snap)
	s := nextSignal(t, sigs)
	if s.Name != propsIface+".PropertiesChanged" || s.Body[0] != playerIface {
		t.Fatalf("signal = %s %v", s.Name, s.Body)
	}
	changed := s.Body[1].(map[string]dbus.Variant)
	if len(changed) != 1 || changed["PlaybackStatus"].Value() != "Paused" {
		t.Errorf("changed = %v, want only PlaybackStatus", changed)
	}

	// An identical snapshot emits nothing; the next real change comes first.
	h.srv.Update(snap)
	next := snap
	next.Track = &Track{ID: "i.AbC", Title: "Other", Length: time.Minute}
	next.Position = 0
	h.srv.Update(next)
	s = nextSignal(t, sigs)
	changed = s.Body[1].(map[string]dbus.Variant)
	if len(changed) != 1 {
		t.Errorf("changed = %v, want only Metadata", changed)
	}
	md := changed["Metadata"].Value().(map[string]dbus.Variant)
	if got := md["mpris:trackid"].Value(); got != dbus.ObjectPath("/dev/coredev/sona/track/i_2eAbC") {
		t.Errorf("trackid = %v", got)
	}
}

func TestUpdateSeeked(t *testing.T) {
	snap := playing()
	h := setup(t, snap)
	sigs := h.signals(t)

	// Continuous playback: two seconds later, two seconds further. No signal.
	snap.Position += 2 * time.Second
	snap.UpdatedAt = snap.UpdatedAt.Add(2 * time.Second)
	h.srv.Update(snap)

	// Jump to 2:00.
	snap.Position = 2 * time.Minute
	snap.UpdatedAt = snap.UpdatedAt.Add(100 * time.Millisecond)
	h.srv.Update(snap)
	s := nextSignal(t, sigs)
	if s.Name != playerIface+".Seeked" {
		t.Fatalf("signal = %s %v, want Seeked", s.Name, s.Body)
	}
	if got := time.Duration(s.Body[0].(int64)) * time.Microsecond; got < 2*time.Minute || got > 2*time.Minute+2*time.Second {
		t.Errorf("Seeked position = %v", got)
	}
}

func TestMethodsReachPlayer(t *testing.T) {
	h := setup(t, playing())
	ctrl := func(m string, args ...any) {
		t.Helper()
		if err := h.obj.Call(playerIface+"."+m, 0, args...).Err; err != nil {
			t.Fatalf("%s: %v", m, err)
		}
	}

	for _, m := range []string{"PlayPause", "Play", "Pause", "Stop", "Next"} {
		ctrl(m)
		if c := h.fake.next(t); c.name != m {
			t.Errorf("called %s, want %s", c.name, m)
		}
	}

	ctrl("Seek", int64(-5_000_000))
	if c := h.fake.next(t); c.name != "Seek" || c.args[0] != -5*time.Second {
		t.Errorf("Seek → %v", c)
	}

	ctrl("SetPosition", TrackPath(song.ID), int64(30_000_000))
	if c := h.fake.next(t); c.name != "SetPosition" || c.args[0] != song.ID || c.args[1] != 30*time.Second {
		t.Errorf("SetPosition → %v", c)
	}
	// Stale track id and positions past the end are dropped.
	ctrl("SetPosition", TrackPath("999"), int64(30_000_000))
	ctrl("SetPosition", TrackPath(song.ID), int64(time.Hour/time.Microsecond))
	h.fake.none(t)

	if err := h.obj.SetProperty(playerIface+".Volume", dbus.MakeVariant(1.7)); err != nil {
		t.Fatal(err)
	}
	if c := h.fake.next(t); c.name != "SetVolume" || c.args[0] != 1.0 {
		t.Errorf("SetVolume → %v", c)
	}
	if err := h.obj.SetProperty(playerIface+".LoopStatus", dbus.MakeVariant("Track")); err != nil {
		t.Fatal(err)
	}
	if c := h.fake.next(t); c.name != "SetLoopStatus" || c.args[0] != LoopTrack {
		t.Errorf("SetLoopStatus → %v", c)
	}
	if err := h.obj.SetProperty(playerIface+".Shuffle", dbus.MakeVariant(true)); err != nil {
		t.Fatal(err)
	}
	if c := h.fake.next(t); c.name != "SetShuffle" || c.args[0] != true {
		t.Errorf("SetShuffle → %v", c)
	}
	if err := h.obj.SetProperty(playerIface+".LoopStatus", dbus.MakeVariant("Forever")); err == nil {
		t.Error("invalid LoopStatus accepted")
	}

	for _, m := range []string{"Raise", "Quit"} {
		if err := h.obj.Call(rootIface+"."+m, 0).Err; err != nil {
			t.Fatal(err)
		}
		if c := h.fake.next(t); c.name != m {
			t.Errorf("called %s, want %s", c.name, m)
		}
	}

	// Player errors surface as D-Bus errors.
	if err := h.obj.Call(playerIface+".Previous", 0).Err; err == nil || !strings.Contains(err.Error(), "no previous") {
		t.Errorf("Previous error = %v", err)
	}
}

func TestCanGating(t *testing.T) {
	snap := playing()
	snap.CanGoNext = false
	h := setup(t, snap)
	if err := h.obj.Call(playerIface+".Next", 0).Err; err != nil {
		t.Fatal(err)
	}
	h.srv.Update(Snapshot{Track: song, CanPlay: true, CanSeek: true}) // CanControl false
	for _, m := range []string{"Play", "Stop"} {
		if err := h.obj.Call(playerIface+"."+m, 0).Err; err != nil {
			t.Fatal(err)
		}
	}
	if err := h.obj.Call(playerIface+".Seek", 0, int64(1)).Err; err != nil {
		t.Fatal(err)
	}
	// Calls are synchronous: anything that got through is already queued.
	h.fake.none(t)

	p := h.getAll(t, playerIface)
	if p["CanPlay"].Value() != false || p["CanSeek"].Value() != false || p["CanControl"].Value() != false {
		t.Errorf("CanControl=false must clear Can*: %v", p)
	}
}

func TestNameLifecycle(t *testing.T) {
	addr := dbustest.Start(t)
	conn := dbustest.Conn(t, addr)
	srv, err := New(conn, newFake(Snapshot{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(dbustest.Conn(t, addr), newFake(Snapshot{}), nil); !errors.Is(err, ErrNameTaken) {
		t.Errorf("second New: %v, want ErrNameTaken", err)
	}
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	var has bool
	if err := conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, BusName).Store(&has); err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("name still owned after Close")
	}
	srv.Update(playing()) // no-op, must not panic
}

func TestTrackPath(t *testing.T) {
	seen := map[dbus.ObjectPath]string{}
	for _, id := range []string{"1440833098", "i.AbC", "i_2eAbC", "pl.u-123", "a/b", "ü"} {
		p := TrackPath(id)
		if !p.IsValid() {
			t.Errorf("TrackPath(%q) = %q, invalid", id, p)
		}
		if prev, dup := seen[p]; dup {
			t.Errorf("TrackPath(%q) collides with %q", id, prev)
		}
		seen[p] = id
	}
	if TrackPath("") != NoTrack {
		t.Error("empty id must map to NoTrack")
	}
}
