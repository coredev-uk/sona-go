package settings

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// pbCDM is the Widevine service without the network.
type pbCDM struct {
	mu      sync.Mutex
	status  services.CDMStatus
	err     error
	retries int
}

func (f *pbCDM) Status() services.CDMStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status
}

func (f *pbCDM) Retry() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retries++
	return f.err
}

func (f *pbCDM) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.retries
}

// pbCaps is a Linux with every playback capability.
func pbCaps() services.Capabilities {
	return services.Capabilities{OS: "linux", Playback: services.PlaybackCapabilities{
		Available: true, Engine: "cdm", Equalizer: true, SoundCheck: true, Crossfade: true, OutputDevice: true,
		PrivateListening: true,
	}}
}

func pbReadyCDM() services.CDMStatus {
	return services.CDMStatus{State: "ready", Version: "4.10.3112.0"}
}

type pbRig struct {
	*rig
	cdm      *pbCDM
	lyricsQs atomic.Int32
}

// newPBRig opens Settings → Playback with the given capabilities and CDM
// state, a tall window (every row visible) and the toaster.
func newPBRig(t *testing.T, caps services.Capabilities, cdm services.CDMStatus, opts ...uitest.Option) *pbRig {
	t.Helper()
	hn := uitest.New(t, opts...)
	r := &rig{t: t, h: hn, router: ui.NewRouter(settingsPath)}
	hn.Env.Nav = r.router
	hn.Env.Shortcuts = func() []module.BoundShortcut { return nil }
	p := &pbRig{rig: r, cdm: &pbCDM{status: cdm}}
	r.s = newSettings(hn.Env,
		services.Service[services.SystemService](hn.Core), services.Service[services.PlatformService](hn.Core),
		fakeScrobbler{}, services.Service[services.IntegrationsService](hn.Core),
		p.cdm, services.Service[services.UpdatesService](hn.Core))
	r.s.init()
	followShell(hn)
	store.Set(hn.Env.Queries, store.KeyCapabilities, caps)
	store.Set(hn.Env.Queries, store.KeyCDM, cdm)
	*stateOf[func() ([]services.OutputDevice, error)](r.s, "seam:outputs") = func() ([]services.OutputDevice, error) {
		return []services.OutputDevice{
			{Name: "alsa_output.speakers", Description: "Speakers", Default: true},
			{Name: "alsa_output.headset", Description: "Headset"},
		}, nil
	}
	r.tt = hn.Tester(t, func(c *ui.Context) {
		c.DisableAnimations() // a frozen test clock would redraw 20 frames per input
		ui.Column(c).Fill().Children(func() {
			r.router.View(c, func(rt *ui.Route) {
				if rt.Match(settingsPath) {
					r.s.page(c, rt)
				}
			})
		})
		kit.Toaster(c)
		// A song page's lyrics query, to see it refetched.
		store.Use(hn.Env.Queries, store.Key{"catalog", "lyrics", "song"}, func(context.Context) (int, error) {
			return int(p.lyricsQs.Add(1)), nil
		}, store.Options{Stale: store.Forever})
	}, 1132, 700)
	r.router.Replace(settingsPath + "?section=playback")
	return p
}

func (p *pbRig) click(label string) {
	p.t.Helper()
	if err := p.tt.Click(label); err != nil {
		p.t.Fatal(err)
	}
	p.tt.Drain()
}

// focus tabs to the control named label (a title and its control share the
// text, so a click by name would land on the title).
func (p *pbRig) focus(label string) {
	p.t.Helper()
	// Find's landing: one or two frames, not a walk of Tab stops (each a frame).
	for _, e := range settingIndex(nil) {
		if e.Title == label {
			p.s.focusSetting(e.ID)
			p.tt.Drain()
			p.tt.Drain()
			break
		}
	}
	for range 300 {
		if p.tt.Focused(label) {
			return
		}
		p.tt.Key(0, ui.KeyTab)
	}
	p.t.Fatalf("Tab never reached %q; texts %q", label, p.tt.Texts())
}

// toggle flips the switch named label.
func (p *pbRig) toggle(label string) {
	p.t.Helper()
	p.focus(label)
	p.tt.Key(0, ui.KeySpace)
	p.tt.Drain()
}

// choose opens the select named label, moves by keys and chooses with Enter.
func (p *pbRig) choose(label string, keys ...ui.Key) {
	p.t.Helper()
	p.focus(label)
	p.tt.Key(0, ui.KeyEnter)
	for _, k := range keys {
		p.tt.Key(0, k)
	}
	p.tt.Key(0, ui.KeyEnter)
	p.tt.Drain()
}

func (p *pbRig) saved(what string, ok func(config.Settings) bool) {
	p.t.Helper()
	p.tt.Until(what, func() bool { return ok(p.h.Settings.Get()) })
}

func (p *pbRig) shows(texts ...string) {
	p.t.Helper()
	p.tt.Until("texts", p.tt.Shows(texts...))
}

func (p *pbRig) lacks(texts ...string) {
	p.t.Helper()
	for _, s := range texts {
		if p.tt.HasText(s) {
			p.t.Errorf("shows %q: %q", s, p.tt.Texts())
		}
	}
}

func TestPlaybackShowsEveryGroupAndRowAndNoVolume(t *testing.T) {
	p := newPBRig(t, pbCaps(), pbReadyCDM())
	p.shows("Widevine", "Sound", "Audio quality", "Output", "Sound Check", "Equaliser", "Crossfade", "Listening",
		"Private listening", "Replacing your queue", "Lyrics", "Advanced", "Free memory when idle", "Version 4.10.3112.0")
	p.lacks("Volume", "Choose…", "Use automatic", "Playback isn't available on this platform yet")
	for _, id := range []string{
		"cdm", "quality", "output", "sound-check", "equalizer", "crossfade", "private-listening",
		"replace-queue", "lyrics-sources", "lyrics-word-timing", "lyrics-translation", "idle",
	} {
		if !p.s.seen[id] {
			t.Errorf("no row %q", id)
		}
	}
	if p.s.seen["crossfade-seconds"] {
		t.Error("the length shows while crossfade is off")
	}
}

func TestPlaybackWithoutPlaybackKeepsOnlyTheLyrics(t *testing.T) {
	caps := pbCaps()
	caps.Playback = services.PlaybackCapabilities{}
	p := newPBRig(t, caps, services.CDMStatus{})
	p.shows("Playback isn't available on this platform yet", "Browsing Apple Music and your library works as usual.", "Lyrics",
		"Lyrics sources")
	p.lacks("Sound", "Sound Check", "Widevine", "Private listening", "Replacing your queue", "Advanced")
}

func TestPlaybackWhileCapabilitiesLoadShowsNoGatedRow(t *testing.T) {
	r := newPBRig(t, pbCaps(), pbReadyCDM())
	// A fresh page whose capabilities have not arrived.
	store.Set(r.h.Env.Queries, store.KeyCapabilities, services.Capabilities{})
	r.h.Env.Queries.Reset()
	r.tt.Frame()
	r.lacks("Sound Check", "Replacing your queue", "Playback isn't available on this platform yet")
}

func TestPlaybackNamesWhatThePlatformCantBack(t *testing.T) {
	caps := pbCaps()
	caps.OS = "darwin"
	caps.Playback.OutputDevice, caps.Playback.Equalizer, caps.Playback.SoundCheck, caps.Playback.Crossfade = false, false, false, false
	caps.Playback.PrivateListening = false
	p := newPBRig(t, caps, pbReadyCDM())
	p.shows("Replacing your queue", "Not available on macOS yet.",
		"This platform's player reports plays to Apple itself, so Sona can't hold them back.")
	for _, id := range []string{"output", "sound-check", "equalizer", "crossfade", "private-listening"} {
		if !p.s.seen[id] {
			t.Errorf("no row %q", id)
		}
	}
	p.lacks("Open equaliser")
}

func TestPlaybackWithAnotherEngineHidesWidevineAndIdle(t *testing.T) {
	caps := pbCaps()
	caps.Playback.Engine = "native"
	p := newPBRig(t, caps, services.CDMStatus{})
	p.shows("Audio quality")
	p.lacks("Widevine", "Advanced", "Free memory when idle")
}

func TestAudioQualityIsSaved(t *testing.T) {
	p := newPBRig(t, pbCaps(), pbReadyCDM())
	p.shows("High · 256 kbps")
	p.choose("Audio quality", ui.KeyDown)
	p.saved("quality", func(s config.Settings) bool { return s.AudioQuality == 64 })
	p.shows("Data saver · 64 kbps")
}

func TestCrossfadeSwitchAndSecondsAreSaved(t *testing.T) {
	p := newPBRig(t, pbCaps(), pbReadyCDM())
	p.shows("Crossfade")
	p.toggle("Crossfade")
	p.saved("crossfade on", func(s config.Settings) bool { return s.Crossfade.Enabled })
	p.shows("Crossfade length", "6 s")
	// Keyboard: End goes to 12 s; the value is saved on the key.
	p.focus("Crossfade length")
	p.tt.Key(0, ui.KeyEnd)
	p.tt.Drain()
	p.saved("crossfade seconds", func(s config.Settings) bool { return s.Crossfade.Seconds == 12 })
	p.shows("12 s")
	p.toggle("Crossfade")
	p.saved("crossfade off", func(s config.Settings) bool { return !s.Crossfade.Enabled })
	p.lacks("Crossfade length")
}

func TestSoundCheckIsSaved(t *testing.T) {
	p := newPBRig(t, pbCaps(), pbReadyCDM())
	p.shows("Sound Check")
	p.toggle("Sound Check")
	p.saved("sound check", func(s config.Settings) bool { return s.Audio.SoundCheck })
}

func TestOutputDeviceListAndChoice(t *testing.T) {
	p := newPBRig(t, pbCaps(), pbReadyCDM())
	p.shows("System default · Speakers")
	p.choose("Output", ui.KeyDown, ui.KeyDown)
	p.saved("output", func(s config.Settings) bool { return s.Audio.OutputDevice == "alsa_output.headset" })
	p.choose("Output", ui.KeyUp, ui.KeyUp)
	p.saved("system default", func(s config.Settings) bool { return s.Audio.OutputDevice == "" })
}

func TestOutputDeviceMissingIsNamedAndSaysSo(t *testing.T) {
	p := newPBRig(t, pbCaps(), pbReadyCDM())
	p.shows("Output")
	if _, err := p.h.Settings.Update(func(s *config.Settings) { s.Audio.OutputDevice = "usb_dock" }); err != nil {
		t.Fatal(err)
	}
	store.Set(p.h.Env.Queries, store.KeySettings, p.h.Settings.Get())
	p.shows("usb_dock · not connected",
		"The output Sona was set to isn't connected; it plays to the system default until it's back.")
}

func TestEqualizerRowIsALinkOnlyWhereItsPageExists(t *testing.T) {
	p := newPBRig(t, pbCaps(), pbReadyCDM())
	p.shows("Equaliser", "Ten bands, presets and a preamp, heard as you move them. Off now.")
	p.lacks("Open equaliser")

	q := newPBRig(t, pbCaps(), pbReadyCDM())
	*stateOf[func(string) bool](q.s, "seam:hasRoute") = func(path string) bool { return path == "/equalizer" }
	q.shows("Open equaliser")
	q.focus("Open equaliser")
	q.tt.Key(0, ui.KeyEnter)
	q.tt.Drain()
	if got := q.router.Location(); got != "/equalizer" {
		t.Errorf("went to %q", got)
	}
}

func TestEqualizerRowSaysOnWhenEnabled(t *testing.T) {
	p := newPBRig(t, pbCaps(), pbReadyCDM())
	p.shows("Equaliser")
	if _, err := p.h.Settings.Update(func(s *config.Settings) { s.Audio.Equalizer.Enabled = true }); err != nil {
		t.Fatal(err)
	}
	store.Set(p.h.Env.Queries, store.KeySettings, p.h.Settings.Get())
	p.shows("Ten bands, presets and a preamp, heard as you move them. On now.")
}

func TestPrivateListeningSwitchesTheService(t *testing.T) {
	p := newPBRig(t, pbCaps(), pbReadyCDM())
	svc := services.Service[services.PrivateService](p.h.Core)
	p.shows("Private listening")
	if svc.State().On {
		t.Fatal("private listening starts on")
	}
	p.tt.Until("the state", func() bool {
		_, ok := store.Peek[services.PrivateState](p.h.Env.Queries, store.KeyPrivate)
		return ok
	})
	p.toggle("Private listening")
	p.tt.Until("private on", func() bool { return svc.State().On })
	p.tt.Until("the row follows the mirror", func() bool {
		v, ok := store.Peek[services.PrivateState](p.h.Env.Queries, store.KeyPrivate)
		return ok && v.On
	})
}

func TestPrivateListeningForcedAndDevelopmentDescriptions(t *testing.T) {
	p := newPBRig(t, pbCaps(), pbReadyCDM())
	store.Set(p.h.Env.Queries, store.KeyPrivate, services.PrivateState{On: true, Source: "environment"})
	p.shows("On because Sona was started with SONA_PRIVATE=1. Start Sona without it to turn private listening off.")
	called := false
	*stateOf[func(bool)](p.s, "seam:private") = func(bool) { called = true }
	_ = p.tt.Click("Private listening") // disabled
	p.tt.Drain()
	if called {
		t.Error("the forced switch acted")
	}
	store.Set(p.h.Env.Queries, store.KeyPrivate, services.PrivateState{On: true, Source: "development", Saved: false})
	p.shows("This development build of Sona starts with private listening on. Switching it here lasts until Sona quits; your saved choice (off) is unchanged.")
}

func TestReplaceQueueModeIsSaved(t *testing.T) {
	p := newPBRig(t, pbCaps(), pbReadyCDM())
	p.shows("Replace, offer Undo")
	p.choose("Replacing your queue", ui.KeyDown)
	p.saved("queue", func(s config.Settings) bool { return s.Queue.ReplaceConfirm == config.ReplaceConfirm })
}

func TestIdleMinutesAreSavedAndAnUnlistedValueIsAdded(t *testing.T) {
	p := newPBRig(t, pbCaps(), pbReadyCDM())
	p.tt.SetSize(1132, 1100) // the open list needs room under the last row
	p.shows("After 10 minutes")
	// The save lands on disk before Go announces it; wait for that
	// announcement (handlers run in subscription order, so the feature has
	// taken it in) so it can't arrive after the change below.
	var echoed atomic.Bool
	store.On(p.h.Bus, services.EventSettingsChanged, func(config.Settings) { echoed.Store(true) })
	p.choose("Free memory when idle", ui.KeyDown, ui.KeyDown)
	p.saved("idle", func(s config.Settings) bool { return s.EngineIdleMinutes == 30 })
	p.tt.Until("the save's announcement", echoed.Load)

	if _, err := p.h.Settings.Update(func(s *config.Settings) { s.EngineIdleMinutes = 7 }); err != nil {
		t.Fatal(err)
	}
	// As Go tells the UI of a change it made itself.
	p.h.Bus.Emit(services.EventSettingsChanged, p.h.Settings.Get())
	p.shows("After 7 minutes")
	// Open the list from its trigger, which now reads the unlisted value.
	if err := p.tt.Click("After 7 minutes"); err != nil {
		t.Fatal(err)
	}
	p.shows("After 5 minutes", "After 15 minutes")
}

func providerIDs(s config.Settings) []string {
	var ids []string
	for _, p := range s.Lyrics.Providers {
		ids = append(ids, p.ID)
	}
	return ids
}

func TestLyricsSourcesOrderSwitchesAndRefetch(t *testing.T) {
	p := newPBRig(t, pbCaps(), pbReadyCDM())
	p.shows("Apple Music", "Musixmatch", "LRCLIB", "NetEase Cloud Music")
	p.tt.Until("the lyrics query", func() bool { return p.lyricsQs.Load() >= 1 })
	before := p.lyricsQs.Load()

	p.focus("Move LRCLIB up")
	p.tt.Key(0, ui.KeyEnter)
	p.tt.Drain()
	p.saved("order", func(s config.Settings) bool {
		return slices.Equal(providerIDs(s), []string{"apple", "lrclib", "musixmatch", "netease"})
	})
	p.tt.Until("lyrics refetched", func() bool { return p.lyricsQs.Load() > before })
	// Focus stays on the button that moved the row.
	p.tt.Until("focus", func() bool { return p.tt.Focused("Move LRCLIB up") })

	// Moving up again reaches the top: focus goes to the sibling button.
	p.tt.Key(0, ui.KeyEnter)
	p.tt.Drain()
	p.saved("top", func(s config.Settings) bool { return providerIDs(s)[0] == "lrclib" })
	p.tt.Until("sibling focus", func() bool { return p.tt.Focused("Move LRCLIB down") })

	p.toggle("NetEase Cloud Music")
	p.saved("netease", func(s config.Settings) bool {
		for _, q := range s.Lyrics.Providers {
			if q.ID == "netease" {
				return q.Enabled
			}
		}
		return false
	})
}

func TestLyricsWordTimingAndTranslationAreSaved(t *testing.T) {
	p := newPBRig(t, pbCaps(), pbReadyCDM())
	p.shows("Prefer word-by-word lyrics", "Your Apple Music language")
	p.tt.Until("the lyrics query", func() bool { return p.lyricsQs.Load() >= 1 })
	before := p.lyricsQs.Load()
	p.toggle("Prefer word-by-word lyrics")
	p.saved("word timing", func(s config.Settings) bool { return !s.Lyrics.PreferWordTiming })
	idx := slices.IndexFunc(translationItems(""), func(it kit.SelectItem) bool { return it.Value == "es" })
	downs := make([]ui.Key, idx)
	for i := range downs {
		downs[i] = ui.KeyDown
	}
	p.choose("Lyrics translation", downs...)
	p.saved("translation", func(s config.Settings) bool { return s.Lyrics.TranslationLanguage == "es" })
	p.tt.Until("lyrics refetched", func() bool { return p.lyricsQs.Load() > before })
}

func TestTranslationLanguagesAreTwentyThreeSortedAndKeepASavedOne(t *testing.T) {
	items := translationItems("")
	if len(items) != 24 || items[0].Value != accountLanguage {
		t.Fatalf("%d items, first %q", len(items), items[0].Value)
	}
	names := make([]string, 0, 23)
	for _, it := range items[1:] {
		names = append(names, it.Label)
	}
	if !slices.IsSorted(names) {
		t.Errorf("not sorted: %q", names)
	}
	if got := translationItems("fi"); len(got) != 25 {
		t.Errorf("a saved tag outside the list is not appended: %d", len(got))
	}
}

func TestMoveProviderSwapsWithTheNeighbourAndStopsAtTheEnds(t *testing.T) {
	list := []config.LyricsProvider{{ID: "a", Enabled: true}, {ID: "b"}}
	ids := func(l []config.LyricsProvider) string { return l[0].ID + l[1].ID }
	if got := ids(moveProvider(list, 1, -1)); got != "ba" {
		t.Errorf("up: %s", got)
	}
	if got := ids(moveProvider(list, 0, -1)); got != "ab" {
		t.Errorf("top: %s", got)
	}
	if got := ids(moveProvider(list, 1, 1)); got != "ab" {
		t.Errorf("bottom: %s", got)
	}
}

func TestWidevineStates(t *testing.T) {
	cases := []struct {
		name   string
		status services.CDMStatus
		shows  []string
		lacks  []string
	}{
		{"ready", services.CDMStatus{State: "ready", Version: "4.10", CheckedAt: 1}, []string{"Version 4.10", "Checked for updates"}, []string{"Try again", "Widevine download"}},
		{"downloading", services.CDMStatus{State: "downloading", Received: 42, Total: 100}, []string{"Downloading…", "42%"}, []string{"Try again"}},
		{"downloading unknown size", services.CDMStatus{State: "downloading"}, []string{"Downloading…"}, []string{"Try again"}},
		{"updating", services.CDMStatus{State: "ready", Version: "4.10", Updating: true, Received: 10, Total: 100}, []string{"Downloading an update…"}, []string{"Try again"}},
		{"idle", services.CDMStatus{State: "idle"}, []string{"Not downloaded yet"}, []string{"Try again"}},
		{"failed", services.CDMStatus{State: "failed", Code: "network"}, []string{
			"Couldn't download",
			"Sona couldn't reach Google's download server. Check your internet connection. Sona also tries again by itself later.", "Try again",
		}, nil},
		{"install failed", services.CDMStatus{State: "failed", Code: "load_check_incomplete"}, []string{"Couldn't install", "Try again"}, nil},
		{"update check failed", services.CDMStatus{State: "ready", Version: "4.10", Code: "hash_mismatch"}, []string{"Couldn't check for updates", "Try again"}, nil},
		{
			"unsupported",
			services.CDMStatus{State: "failed", Code: "unsupported_platform"},
			[]string{"Google doesn't make Widevine for this computer, so songs can't play here."},
			[]string{"Try again", "Sona also tries again by itself later."},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newPBRig(t, pbCaps(), tc.status)
			p.shows(tc.shows...)
			p.lacks(tc.lacks...)
		})
	}
}

func TestWidevineProgressBarIsNamed(t *testing.T) {
	p := newPBRig(t, pbCaps(), services.CDMStatus{State: "downloading", Received: 42, Total: 100})
	p.shows("Downloading…")
	if _, ok := p.tt.Find("Widevine download"); !ok {
		t.Error("no bar named Widevine download")
	}
	q := newPBRig(t, pbCaps(), services.CDMStatus{State: "ready", Version: "1", Updating: true, Total: 100, Received: 1})
	q.shows("Downloading an update…")
	if _, ok := q.tt.Find("Widevine update"); !ok {
		t.Error("no bar named Widevine update")
	}
}

func TestWidevineTryAgainRetriesAndMovesFocusToTheState(t *testing.T) {
	p := newPBRig(t, pbCaps(), services.CDMStatus{State: "failed", Code: "network"})
	p.shows("Try again")
	p.click("Try again")
	p.tt.Until("retry", func() bool { return p.cdm.count() == 1 })
	p.tt.Until("focus on the state", func() bool { return p.tt.Focused("Widevine") })
}

func TestWidevineRetryFailureToasts(t *testing.T) {
	p := newPBRig(t, pbCaps(), services.CDMStatus{State: "failed", Code: "network"})
	p.cdm.err = errors.New("boom")
	p.shows("Try again")
	p.click("Try again")
	p.tt.Until("toast", p.tt.Shows("Couldn't start the Widevine download"))
}

func TestWidevineFollowsTheSharedMirror(t *testing.T) {
	p := newPBRig(t, pbCaps(), services.CDMStatus{State: "downloading", Received: 10, Total: 100})
	p.shows("Downloading…")
	store.Set(p.h.Env.Queries, store.KeyCDM, services.CDMStatus{State: "ready", Version: "9.9"})
	p.tt.Until("ready", p.tt.Shows("Version 9.9"))
}

func TestOutputDevicesAreRefetchedWhenTheWindowGetsTheFocusBack(t *testing.T) {
	p := newPBRig(t, pbCaps(), pbReadyCDM())
	var fetches atomic.Int32
	devices := []services.OutputDevice{{Name: "alsa_output.speakers", Description: "Speakers", Default: true}}
	*stateOf[func() ([]services.OutputDevice, error)](p.s, "seam:outputs") = func() ([]services.OutputDevice, error) {
		fetches.Add(1)
		return devices, nil
	}
	p.shows("Output")
	p.tt.Drain()
	p.h.Env.Window.SetFocused(false)
	p.tt.Frame() // built while another app has the focus
	seen := fetches.Load()
	devices = append(devices, services.OutputDevice{Name: "usb_dac", Description: "USB DAC"})
	p.tt.Frame()
	if fetches.Load() != seen {
		t.Fatal("the list was fetched again while the window lacked the focus")
	}
	p.h.Env.Window.SetFocused(true)
	p.tt.Frame()
	p.tt.Until("refetched", func() bool { return fetches.Load() > seen })
	p.tt.Drain()
	p.choose("Output", ui.KeyDown, ui.KeyDown)
	p.saved("the new device", func(s config.Settings) bool { return s.Audio.OutputDevice == "usb_dac" })
}
