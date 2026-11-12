package equalizer

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	eqp "github.com/coredev-uk/sona-go/internal/playback/equalizer"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// fake is the backend of a test: the bands of Go's equaliser, settings
// held in memory, and a record of what the page asked.
type fake struct {
	mu        sync.Mutex
	info      services.EqualizerInfo
	settings  config.Settings
	failInfo  error
	infoCalls int
	failGet   error
	failSave  error
	saved     []config.Settings
	previews  []config.Equalizer
}

func newFake() *fake {
	f := &fake{settings: config.Settings{Audio: config.Audio{Equalizer: config.Equalizer{Preset: "flat", Custom: make([]float64, eqp.Bands)}}}}
	f.info = services.EqualizerInfo{
		Frequencies: eqp.Frequencies[:], MinGain: eqp.MinGain, MaxGain: eqp.MaxGain,
		MinPreamp: eqp.MinPreamp, MaxPreamp: eqp.MaxPreamp,
	}
	for _, p := range eqp.Presets {
		f.info.Presets = append(f.info.Presets, services.EqualizerPreset{ID: p.ID, Name: p.Name, Gains: slices.Clone(p.Gains[:])})
	}
	return f
}

func (f *fake) Info(context.Context) (services.EqualizerInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.infoCalls++
	return f.info, f.failInfo
}

func (f *fake) Settings(context.Context) (config.Settings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.settings, f.failGet
}

func (f *fake) Preview(_ context.Context, eq config.Equalizer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.previews = append(f.previews, eq)
	return errors.New("the host is busy") // ignored by the page
}

func (f *fake) Save(_ context.Context, s config.Settings) (config.Settings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failSave != nil {
		return config.Settings{}, f.failSave
	}
	f.saved = append(f.saved, s)
	f.settings = s
	return s, nil
}

func (f *fake) set(fn func(*fake)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fake) lastSaved() (config.Equalizer, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.saved) == 0 {
		return config.Equalizer{}, false
	}
	return f.saved[len(f.saved)-1].Audio.Equalizer, true
}

func (f *fake) lastPreview() (config.Equalizer, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.previews) == 0 {
		return config.Equalizer{}, false
	}
	return f.previews[len(f.previews)-1], true
}

// rig is the page over a fake backend in a tester.
type rig struct {
	t      *testing.T
	fake   *fake
	router *ui.Router
	e      *equalizer
	tt     *uitest.Session
}

// newRig builds the page over a fake that setup (nil for none) has changed:
// the first frame already asks it.
func newRig(t *testing.T, supported bool, setup func(*fake), opts ...uitest.Option) *rig {
	t.Helper()
	return newRigAt(t, 900, supported, setup, opts...)
}

// newRigAt is newRig in a window width DIPs wide.
func newRigAt(t *testing.T, width int, supported bool, setup func(*fake), opts ...uitest.Option) *rig {
	t.Helper()
	h := uitest.New(t, append([]uitest.Option{uitest.Bare()}, opts...)...)
	r := &rig{t: t, fake: newFake(), router: ui.NewRouter("/equalizer")}
	if setup != nil {
		setup(r.fake)
	}
	h.Env.Nav = r.router
	r.e = newEqualizer(h.Env, r.fake, func() bool { return supported })
	r.tt = h.Tester(t, func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			r.router.View(c, func(rt *ui.Route) {
				if rt.Match("/equalizer") {
					r.e.page(c, rt)
				}
			})
		})
		kit.OverlayLayer(c) // the toasts
	}, width, 760)
	return r
}

// focus tabs until the element labelled name has the focus.
func (r *rig) focus(name string) {
	r.t.Helper()
	for range 40 {
		if r.tt.Focused(name) {
			return
		}
		r.tt.Key(0, ui.KeyTab)
		r.tt.Drain()
	}
	r.t.Fatalf("the focus never reached %q", name)
}

func (r *rig) savedEq(what string) config.Equalizer {
	r.t.Helper()
	var eq config.Equalizer
	r.tt.Until(what, func() bool {
		var ok bool
		eq, ok = r.fake.lastSaved()
		return ok
	})
	return eq
}

func TestPageShowsHeaderPresetsAndFaders(t *testing.T) {
	r := newRig(t, true, nil)
	r.tt.Until("the page", r.tt.Shows("Equaliser", "Off · Sona plays the music as mastered", "Off", "Preamp"))
	for _, w := range []string{
		"Your curve", "Flat", "Bass Boost", "Bass Reducer", "Treble Boost", "Treble Reducer", "Vocal", "Loudness",
		"Acoustic", "Electronic", "Hip-Hop", "Rock", "Classical", "Jazz", "Piano", "Spoken Word",
		"Preset", "Bands", "+12", "0 dB", "−12",
		"29", "59", "119", "237", "474", "947", "1.9k", "3.8k", "7.5k", "15k",
		"29 Hz", "947 Hz", "15k Hz", "Cuts only: the level stays as it is. Preamp lowers it further.",
	} {
		if !r.tt.HasText(w) {
			t.Errorf("page lacks %q; texts %q", w, r.tt.Texts())
		}
	}
	// Your curve comes first, then Go's order.
	order := []string{"Your curve", "Flat", "Bass Boost", "Spoken Word"}
	var got []string
	for _, w := range r.tt.Texts() {
		if slices.Contains(order, w) && !slices.Contains(got, w) {
			got = append(got, w)
		}
	}
	if !slices.Equal(got, order) {
		t.Errorf("presets listed %q, want %q", got, order)
	}
	// The info is asked of Go once, whatever is drawn after.
	for range 5 {
		r.tt.Drain()
	}
	r.fake.mu.Lock()
	defer r.fake.mu.Unlock()
	if r.fake.infoCalls != 1 {
		t.Errorf("the equaliser's info was fetched %d times", r.fake.infoCalls)
	}
}

func TestChoosingAPresetSavesItAndTurnsTheEqualiserOn(t *testing.T) {
	r := newRig(t, true, nil)
	r.tt.Until("the page", r.tt.Shows("Bass Boost"))
	r.tt.MustClick("Bass Boost")
	eq := r.savedEq("the saved preset")
	if !eq.Enabled || eq.Preset != "bass-boost" {
		t.Errorf("saved %+v, want enabled with bass-boost", eq)
	}
	r.tt.Until("the new header", r.tt.Shows("Bass Boost · shapes everything Sona plays", "On",
		"The level drops 6 dB ahead of the bands, so the largest boost cannot clip. Preamp lowers it further."))
	if r.tt.HasText("Cuts only") {
		t.Error("the cuts-only note stays for a boost")
	}
}

func TestChoosingTheChosenPresetTurnsTheEqualiserOn(t *testing.T) {
	r := newRig(t, true, nil)
	r.tt.Until("the page", r.tt.Shows("Flat"))
	r.tt.MustClick("Flat")
	if eq := r.savedEq("the save"); !eq.Enabled || eq.Preset != "flat" {
		t.Errorf("saved %+v, want enabled with flat", eq)
	}
}

func TestTheSwitchSavesEnabled(t *testing.T) {
	r := newRig(t, true, nil)
	r.tt.Until("the page", r.tt.Shows("Off · Sona plays the music as mastered"))
	r.tt.MustClick("Off")
	if eq := r.savedEq("the save"); !eq.Enabled || eq.Preset != "flat" {
		t.Errorf("saved %+v, want enabled with the same preset", eq)
	}
	r.tt.Until("on", r.tt.Shows("Flat · shapes everything Sona plays", "On"))
	r.tt.MustClick("On")
	r.tt.Until("off", r.tt.Shows("Off · Sona plays the music as mastered"))
	if eq, _ := r.fake.lastSaved(); eq.Enabled {
		t.Errorf("saved %+v, want disabled", eq)
	}
}

func TestAFaderPreviewsThroughAudioAndSavesACustomCurve(t *testing.T) {
	r := newRig(t, true, func(f *fake) {
		f.settings.Audio.Equalizer.Preset, f.settings.Audio.Equalizer.Enabled = "bass-boost", true
	})
	r.tt.Until("the page", r.tt.Shows("947 Hz", "Bass Boost · shapes everything Sona plays"))
	r.focus("947 Hz")
	r.tt.Key(0, ui.KeyUp)
	r.tt.Key(0, ui.KeyUp)

	// The curve is heard (and its error ignored) ...
	var heard config.Equalizer
	r.tt.Until("the preview", func() bool {
		var ok bool
		heard, ok = r.fake.lastPreview()
		return ok && heard.Custom[5] == 1
	})
	if heard.Preset != custom || !heard.Enabled {
		t.Errorf("previewed %+v", heard)
	}
	// ... then saved as the user's curve, starting from the preset's gains.
	eq := r.savedEq("the save")
	want := []float64{6, 5, 4, 2.5, 1, 1, 0, 0, 0, 0}
	if eq.Preset != custom || !slices.Equal(eq.Custom, want) {
		t.Errorf("saved %+v, want custom %v", eq, want)
	}
	r.tt.Until("the curve's name", r.tt.Shows("Your curve · shapes everything Sona plays", "+1"))
	for _, p := range eqp.Presets { // the preset itself is untouched
		if p.ID == "bass-boost" && p.Gains[5] != 0 {
			t.Errorf("the preset changed: %v", p.Gains)
		}
	}
}

func TestADraggedFaderSavesOnlyOnRelease(t *testing.T) {
	r := newRig(t, true, nil)
	r.tt.Until("the page", r.tt.Shows("947 Hz"))
	box, ok := r.tt.Find("947 Hz")
	if !ok {
		t.Fatal("no fader")
	}
	x, y := box.X+box.W/2, box.Y+box.H/2
	// Press near the top of the track and drag a little: heard, not saved.
	r.tt.Press(x, y-40)
	r.tt.Drain()
	r.tt.Move(x, y-60)
	r.tt.Drain()
	r.tt.Until("a preview", func() bool { _, ok := r.fake.lastPreview(); return ok })
	if _, saved := r.fake.lastSaved(); saved {
		t.Fatal("saved while the pointer is still down")
	}
	if !r.tt.HasText("Your curve") {
		t.Error("the list vanished")
	}
	r.tt.Release(x, y-60)
	eq := r.savedEq("the save on release")
	if eq.Preset != custom || eq.Custom[5] <= 0 {
		t.Errorf("saved %+v, want custom with band 5 raised", eq)
	}
	if pv, _ := r.fake.lastPreview(); pv.Custom[5] != eq.Custom[5] {
		t.Errorf("the last preview %v is not what was saved %v", pv.Custom[5], eq.Custom[5])
	}
}

func TestThePreampFaderCutsOnlyFromTheTop(t *testing.T) {
	r := newRig(t, true, nil)
	r.tt.Until("the page", r.tt.Shows("Preamp"))
	r.focus("Preamp")
	r.tt.Key(0, ui.KeyDown)
	eq := r.savedEq("the save")
	if eq.Preamp != -0.5 || eq.Preset != "flat" {
		t.Errorf("saved %+v, want preamp -0.5 on flat", eq)
	}
	r.tt.Until("the value", r.tt.Shows("−0.5"))
}

func TestPresetsAreOneRadioGroupWithArrowKeys(t *testing.T) {
	r := newRig(t, true, nil)
	r.tt.Until("the page", r.tt.Shows("Flat"))
	// Tab goes to the chosen preset, not to each of them.
	r.focus("Flat")
	if r.tt.Focused("Your curve") || r.tt.Focused("Bass Boost") {
		t.Error("more than the chosen preset has the focus")
	}
	r.tt.Key(0, ui.KeyDown)
	if eq := r.savedEq("the arrow's save"); eq.Preset != "bass-boost" || !eq.Enabled {
		t.Errorf("Down saved %+v, want bass-boost, enabled", eq)
	}
	r.tt.Until("the focus with the choice", func() bool { return r.tt.Focused("Bass Boost") })
	r.tt.Key(0, ui.KeyRight)
	r.tt.Until("Right", func() bool { eq, _ := r.fake.lastSaved(); return eq.Preset == "bass-reducer" })
	r.tt.Key(0, ui.KeyUp)
	r.tt.Until("Up", func() bool { eq, _ := r.fake.lastSaved(); return eq.Preset == "bass-boost" })
	r.tt.Key(0, ui.KeyLeft)
	r.tt.Until("Left", func() bool { eq, _ := r.fake.lastSaved(); return eq.Preset == "flat" })
	// The ends wrap: the first is Your curve, the last Spoken Word.
	r.tt.Key(0, ui.KeyUp)
	r.tt.Until("wrapping up", func() bool { eq, _ := r.fake.lastSaved(); return eq.Preset == custom })
	r.tt.Key(0, ui.KeyUp)
	r.tt.Until("wrapping to the end", func() bool { eq, _ := r.fake.lastSaved(); return eq.Preset == "spoken-word" })
	r.tt.Key(0, ui.KeyDown)
	r.tt.Until("wrapping to the start", func() bool { eq, _ := r.fake.lastSaved(); return eq.Preset == custom })
	r.tt.Until("the focus on it", func() bool { return r.tt.Focused("Your curve") })
}

func TestAPlatformWithoutAnEqualiserSaysSo(t *testing.T) {
	r := newRig(t, false, nil)
	r.tt.Until("the notice", r.tt.Shows("Equaliser", "Not available on this platform yet"))
	for _, w := range []string{"Preset", "Preamp", "Bands", "Flat"} {
		if r.tt.HasText(w) {
			t.Errorf("an unsupported platform shows %q", w)
		}
	}
}

func TestALoadFailureShowsTheErrorAndRetryAsksBoth(t *testing.T) {
	r := newRig(t, true, func(f *fake) { f.failGet = errors.ErrUnsupported })
	r.tt.Until("the error", r.tt.Shows("Equaliser", "Couldn't load the equaliser", "Try again"))
	if r.tt.HasText("Preset") {
		t.Error("the page shows presets over an error")
	}
	// Fixed: Retry refetches the settings (and the info) and the page appears.
	r.fake.set(func(f *fake) { f.failGet, f.failInfo = nil, nil })
	r.tt.MustClick("Try again")
	r.tt.Until("the page", r.tt.Shows("Preset", "Flat", "Preamp"))
	if r.tt.HasText("Couldn't load the equaliser") {
		t.Error("the error stays")
	}
}

func TestAnInfoFailureIsAnErrorToo(t *testing.T) {
	r := newRig(t, true, func(f *fake) { f.failInfo = errors.New("boom") })
	r.tt.Until("the error", r.tt.Shows("Couldn't load the equaliser"))
}

func TestWhileLoadingTheHeaderAndAPlaceholderShow(t *testing.T) {
	r := newRig(t, true, nil)
	// The first frame: nothing has arrived yet.
	r.tt.Frame()
	if !r.tt.HasText("Equaliser") {
		t.Fatalf("no header while loading; texts %q", r.tt.Texts())
	}
	if r.tt.HasText("Preset") {
		t.Error("presets before the data")
	}
	r.tt.Until("the page", r.tt.Shows("Preset"))
}

func TestAFailedSaveToastsAndAsksForTheRealSettings(t *testing.T) {
	r := newRig(t, true, nil)
	r.tt.Until("the page", r.tt.Shows("Bass Boost"))
	r.fake.set(func(f *fake) { f.failSave = errors.ErrUnsupported })
	r.tt.MustClick("Bass Boost")
	r.tt.Until("the page back as Go has it", r.tt.Shows("Off · Sona plays the music as mastered"))
	r.tt.Until("the toast", r.tt.Shows("Couldn't save the setting"))
}

func TestTheShortcutOpensThePageWhereThereIsAnEqualiser(t *testing.T) {
	r := newRig(t, false, nil)
	on := false
	r.e.supported = func() bool { return on }
	sc := r.e.shortcuts()
	if len(sc) != 1 {
		t.Fatalf("%d shortcuts", len(sc))
	}
	s := sc[0]
	if s.ID != "equalizer.open" || s.Group != "pages" || !slices.Equal(s.Keys, []string{"Ctrl+Shift+E"}) || s.Icon != "sliders-vertical" {
		t.Errorf("shortcut %+v", s)
	}
	if s.Label() != "Open the equaliser" || s.Keywords() != "eq bass treble sound" {
		t.Errorf("label %q keywords %q", s.Label(), s.Keywords())
	}
	if s.When() {
		t.Error("the shortcut is available without an equaliser")
	}
	on = true
	if !s.When() {
		t.Fatal("the shortcut is unavailable with an equaliser")
	}
	r.router.Push("/elsewhere")
	s.Run()
	if got := r.router.Location(); got != "/equalizer" {
		t.Errorf("went to %q", got)
	}
}

func TestTheModule(t *testing.T) {
	env := uitest.Env(t, uitest.Empty())
	m := New(env)
	if m.ID != "equalizer" || len(m.Routes) != 1 || m.Routes[0].Pattern != "/equalizer" {
		t.Fatalf("module %+v", m)
	}
	if got := m.Routes[0].Title(nil); got != "Equaliser" {
		t.Errorf("title %q", got)
	}
	// The test engine has no equaliser: the key is hidden.
	if len(m.Shortcuts) != 1 || m.Shortcuts[0].When() {
		t.Errorf("shortcuts %+v", m.Shortcuts)
	}
	if len(m.Nav) != 0 || m.Slots.Bottom != nil {
		t.Error("the module adds more than a route and a shortcut")
	}
}

func TestOverRealServicesSavesIntoGoSettings(t *testing.T) {
	h := uitest.New(t, uitest.Empty())
	router := ui.NewRouter("/equalizer")
	h.Env.Nav = router
	e := newEqualizer(h.Env, host{
		audio:  services.Service[services.AudioService](h.Core),
		system: services.Service[services.SystemService](h.Core),
	}, func() bool { return true })
	tt := h.Tester(t, func(c *ui.Context) {
		router.View(c, func(rt *ui.Route) { e.page(c, rt) })
	}, 900, 760)
	tt.Until("the page", tt.Shows("Rock", "Off · Sona plays the music as mastered"))
	tt.MustClick("Rock")
	tt.Until("the saved setting", func() bool {
		eq := h.Settings.Get().Audio.Equalizer
		return eq.Enabled && eq.Preset == "rock"
	})
	tt.Until("the page follows", tt.Shows("Rock · shapes everything Sona plays"))
}
