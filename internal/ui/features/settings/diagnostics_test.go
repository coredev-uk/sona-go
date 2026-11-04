package settings

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// diagSys is SystemService's diagnostics side without the machine.
type diagSys struct {
	mu        sync.Mutex
	d         services.Diagnostics
	err       error
	gate      chan struct{} // Diagnostics waits for it to close when set
	bundle    services.DiagBundle
	bundleErr error
	logErr    error
	runs      int
	bundles   int
	logs      int
}

func (f *diagSys) Diagnostics(context.Context) (services.Diagnostics, error) {
	f.mu.Lock()
	f.runs++
	gate, d, err := f.gate, f.d, f.err
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	return d, err
}

func (f *diagSys) DiagnosticsBundle(context.Context) (services.DiagBundle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bundles++
	return f.bundle, f.bundleErr
}

func (f *diagSys) OpenLogFolder() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs++
	return f.logErr
}

func (f *diagSys) set(fn func(*diagSys)) { f.mu.Lock(); fn(f); f.mu.Unlock() }
func (f *diagSys) counts() (runs, bundles, logs int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.runs, f.bundles, f.logs
}

type diagMotion struct {
	mu       sync.Mutex
	st       services.MotionStatus
	statuses int
	reports  []services.MotionReport
}

func (f *diagMotion) Status() services.MotionStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses++
	return f.st
}

func (f *diagMotion) Report(r services.MotionReport) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reports = append(f.reports, r)
}

type diagInspector struct {
	mu    sync.Mutex
	err   error
	opens int
}

func (f *diagInspector) OpenDeveloperTools() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opens++
	return f.err
}

func (f *diagInspector) count() int { f.mu.Lock(); defer f.mu.Unlock(); return f.opens }

type diagCDM struct {
	mu sync.Mutex
	st services.CDMStatus
}

func (f *diagCDM) Status() services.CDMStatus { f.mu.Lock(); defer f.mu.Unlock(); return f.st }
func (f *diagCDM) Retry() error               { return nil }

// diagRig is Settings → Diagnostics over fakes.
type diagRig struct {
	*rig
	sys *diagSys
	mot *diagMotion
	dev *diagInspector
	cdm *diagCDM
	now time.Time
}

// engineReport is a Linux report: the CDM engine with a working stack.
func engineReport() services.Diagnostics {
	cdm := services.CDMStatus{State: "ready", Version: "4.10.3112.0", Path: "/data/widevine/libwidevinecdm.so"}
	return services.Diagnostics{
		App: services.DiagApp{Version: "1.0.0", GoVersion: "go1.27", Platform: "linux/amd64", PID: 7},
		Playback: services.DiagPlayback{
			Engine: "cdm", CDM: cdm, CDMOK: true, CDMVersion: "4.10.3112.0", GStreamer: "1.24", Decoder: "avdec_aac",
			Sink: "pipewiresink", Sandboxed: true, SandboxKind: "linux", AudioQuality: 256,
		},
		Database: services.DiagDatabase{Path: "/tmp/sona.db", SizeBytes: 2048, SchemaVersion: 4},
		Keyring:  services.DiagKeyring{Backend: "secret-service"},
		Paths:    services.DiagPaths{LogFile: "/state/sona.log", Settings: "/cfg/settings.json", Data: "/data", Cache: "/cache"},
	}
}

func noEngineReport() services.Diagnostics {
	d := engineReport()
	d.Playback = services.DiagPlayback{}
	d.Keyring.Backend = "keychain"
	return d
}

func newDiagRig(t *testing.T, caps services.Capabilities, d services.Diagnostics, setup func(*diagRig)) *diagRig {
	t.Helper()
	hn := uitest.New(t)
	r := &rig{t: t, h: hn, router: ui.NewRouter(settingsPath)}
	hn.Env.Nav = r.router
	g := &diagRig{
		rig: r, sys: &diagSys{d: d, bundle: services.DiagBundle{Text: "BUNDLE", Path: "/state/sona-diagnostics-1.txt"}},
		mot: &diagMotion{}, dev: &diagInspector{}, cdm: &diagCDM{st: d.Playback.CDM},
		now: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC),
	}
	r.s = newSettings(hn.Env, services.Service[services.SystemService](hn.Core), services.Service[services.PlatformService](hn.Core),
		fakeScrobbler{}, services.Service[services.IntegrationsService](hn.Core), g.cdm, nil)
	r.s.clock = func() time.Time { return g.now }
	r.s.last = secDiagnostics
	sv := stateOf[diagSeamState](r.s, "seam:diagnostics")
	sv.sys, sv.mot, sv.desk = g.sys, g.mot, g.dev
	if setup != nil {
		setup(g)
	}
	r.s.init()
	store.Set(hn.Env.Queries, store.KeyCDM, g.cdm.st) // the cdm module's mirror
	store.Set(hn.Env.Queries, store.KeyCapabilities, caps)
	r.tt = hn.Tester(t, func(c *ui.Context) {
		c.DisableAnimations() // a frozen test clock would redraw 20 frames per input
		ui.Column(c).Fill().Children(func() {
			r.router.View(c, func(rt *ui.Route) {
				if rt.Match(settingsPath) {
					r.s.page(c, rt)
				}
			})
			kit.Toaster(c)
		})
	}, 1132, 700)
	return g
}

func (g *diagRig) click(label string) {
	g.t.Helper()
	if err := g.tt.Click(label); err != nil {
		g.t.Fatal(err)
	}
	g.tt.Drain()
}

func (g *diagRig) native(r services.MotionReport) {
	stateOf[diagSeamState](g.s, "seam:diagnostics").native = func() (services.MotionReport, bool) { return r, true }
}

func TestDiagnosticsShowsLoadingThenTheVerdict(t *testing.T) {
	gate := make(chan struct{})
	g := newDiagRig(t, linuxCaps(), engineReport(), func(g *diagRig) { g.sys.gate = gate })
	g.tt.Until("checking", g.tt.Shows("Checking the playback host, database and keyring…", "Loading"))
	if g.tt.HasText("Everything Sona needs is working") {
		t.Error("the verdict shows while checking")
	}
	close(gate)
	g.tt.Until("verdict", g.tt.Shows("Everything Sona needs is working", "Widevine CDM"))
	if g.tt.HasText("Loading") {
		t.Error("the skeleton stays")
	}
}

func TestDiagnosticsGroupsInOrder(t *testing.T) {
	g := newDiagRig(t, linuxCaps(), engineReport(), nil)
	g.native(services.MotionReport{Allowed: true, Setting: true, H264: "probably", Visible: true})
	g.tt.Until("groups", g.tt.Shows("Developer tools", "Sona", "Library database"))
	texts := g.tt.Texts()
	pos := func(s string) int { return slices.Index(texts, s) }
	order := []string{"Playback", "Motion artwork", "Credentials", "Library database", "Sona", "Developer tools"}
	last := -1
	for _, o := range order {
		p := pos(o)
		if p < 0 || p <= last {
			t.Fatalf("group %q at %d after %d; texts %q", o, p, last, texts)
		}
		last = p
	}
	for _, gone := range []string{"Rendering", "Session", "WebKitGTK", "Web inspector"} {
		if g.tt.HasText(gone) {
			t.Errorf("WebKit-only %q shows", gone)
		}
	}
}

func TestDiagnosticsHasNoMotionGroupWithoutAPolicy(t *testing.T) {
	g := newDiagRig(t, linuxCaps(), engineReport(), nil)
	g.tt.Until("groups", g.tt.Shows("Credentials"))
	if g.tt.HasText("Motion artwork") {
		t.Error("Motion artwork shows without a probe")
	}
}

func TestDiagnosticsWithoutAnEngine(t *testing.T) {
	g := newDiagRig(t, linuxCaps(), noEngineReport(), nil)
	g.tt.Until("rows", g.tt.Shows("Not available on this platform yet", "macOS Keychain", "Everything Sona needs is working"))
	for _, row := range []string{"Widevine CDM", "GStreamer", "AAC decoder", "Host sandbox"} {
		if g.tt.HasText(row) {
			t.Errorf("%q shows without an engine", row)
		}
	}
}

func TestDiagnosticsWithTheCDMEngine(t *testing.T) {
	g := newDiagRig(t, linuxCaps(), engineReport(), nil)
	g.tt.Until("rows", g.tt.Shows("Widevine CDM", "Working", "Source", "Downloaded from Google", "Updates", "Not checked yet",
		"Version", "CDM file", "CDM interface", "GStreamer", "AAC decoder", "Audio output", "Playback host", "Host sandbox", "Audio priority", "Stream format",
		"Secret Service keyring", "AAC 256 kbps", "Contents", "Last full sync", "Never", "Artwork cache", "Log file", "/state/sona.log"))
	if g.tt.HasText("Not available on this platform yet") {
		t.Error("the engine's platform says unavailable")
	}
}

func TestDiagnosticsFollowsTheLiveCDMStatus(t *testing.T) {
	d := engineReport()
	d.Playback.CDMOK = false
	d.Playback.CDM = services.CDMStatus{State: "downloading"}
	g := newDiagRig(t, linuxCaps(), d, nil)
	g.tt.Until("downloading", g.tt.Shows("Downloading…", "Checked once Widevine is ready"))
	store.Set(g.h.Env.Queries, store.KeyCDM, services.CDMStatus{State: "downloading", Received: 3, Total: 4})
	g.tt.Until("live percent", g.tt.Shows("Downloading · 75%"))
}

func TestDiagnosticsCountsProblemsAndWarnings(t *testing.T) {
	d := engineReport()
	d.Playback.CDMOK = false
	d.Playback.CDM = services.CDMStatus{State: "failed", Code: "signature_invalid"}
	d.Keyring.Unencrypted = true
	g := newDiagRig(t, linuxCaps(), d, nil)
	g.tt.Until("verdict", g.tt.Shows("1 problem found", "1 warning to look at below", "Couldn't download", "File (unencrypted)"))
}

func TestCheckAgainRunsTheChecksAndIsDisabledWhileRunning(t *testing.T) {
	g := newDiagRig(t, linuxCaps(), engineReport(), nil)
	g.tt.Until("first run", g.tt.Shows("Everything Sona needs is working"))
	gate := make(chan struct{})
	g.sys.set(func(f *diagSys) { f.gate = gate })
	g.click("Check again")
	g.tt.Until("checking", g.tt.Shows("Checking the playback host, database and keyring…"))
	runs, _, _ := g.sys.counts()
	if runs != 2 {
		t.Errorf("runs %d", runs)
	}
	g.click("Check again") // disabled: no third run
	if runs, _, _ = g.sys.counts(); runs != 2 {
		t.Errorf("a click while checking ran again: %d", runs)
	}
	close(gate)
	g.tt.Until("done", g.tt.Shows("Everything Sona needs is working"))
}

func TestDiagnosticsErrorHasRetry(t *testing.T) {
	g := newDiagRig(t, linuxCaps(), engineReport(), func(g *diagRig) { g.sys.err = errors.New("boom") })
	g.tt.Until("error", g.tt.Shows("Couldn't run the checks", "Try again"))
	if g.tt.HasText("Loading") {
		t.Error("loading skeleton beside the error")
	}
	g.sys.set(func(f *diagSys) { f.err = nil })
	g.click("Try again")
	g.tt.Until("recovered", g.tt.Shows("Everything Sona needs is working"))
}

func TestCopyDiagnosticsCopiesTheBundleWithTheMotionReport(t *testing.T) {
	g := newDiagRig(t, linuxCaps(), engineReport(), nil)
	want := services.MotionReport{Allowed: true, Setting: true, H264: "probably", Visible: true, Last: "playing"}
	g.native(want)
	g.tt.Until("page", g.tt.Shows("Copy diagnostics"))
	g.click("Copy diagnostics")
	g.tt.Until("toast", g.tt.Shows("Diagnostics copied",
		"Tokens and your e-mail address are removed. A copy is saved as sona-diagnostics-1.txt in the log folder."))
	if got := g.h.Copied(); len(got) != 1 || got[0] != "BUNDLE" {
		t.Errorf("copied %q", got)
	}
	g.mot.mu.Lock()
	reports := slices.Clone(g.mot.reports)
	g.mot.mu.Unlock()
	if len(reports) != 1 || reports[0] != want {
		t.Errorf("reports %+v", reports)
	}
	// Show opens the log folder.
	g.click("Show")
	g.tt.Until("log folder", func() bool { _, _, logs := g.sys.counts(); return logs == 1 })
}

func TestCopyDiagnosticsFailureToast(t *testing.T) {
	g := newDiagRig(t, linuxCaps(), engineReport(), func(g *diagRig) { g.sys.bundleErr = errors.New("disk full") })
	g.tt.Until("page", g.tt.Shows("Copy diagnostics"))
	g.click("Copy diagnostics")
	g.tt.Until("toast", g.tt.Shows("Couldn't collect diagnostics"))
	if len(g.h.Copied()) != 0 {
		t.Error("a failed bundle was copied")
	}
}

func TestOpenLogFolder(t *testing.T) {
	g := newDiagRig(t, linuxCaps(), engineReport(), nil)
	g.tt.Until("page", g.tt.Shows("Open log folder"))
	g.click("Open log folder")
	g.tt.Until("opened", func() bool { _, _, logs := g.sys.counts(); return logs == 1 })
	g.sys.set(func(f *diagSys) { f.logErr = errors.New("no file manager") })
	g.click("Open log folder")
	g.tt.Until("failure toast", g.tt.Shows("Couldn't open the log folder"))
}

func TestDeveloperToolsOpensTheInspectorWhereItIsOn(t *testing.T) {
	g := newDiagRig(t, linuxCaps(), engineReport(), nil)
	g.tt.Until("row", g.tt.Shows("Developer tools", "Open the inspector"))
	g.reveal("developer-tools")
	g.click("Open")
	g.tt.Until("opened", func() bool { return g.dev.count() == 1 })
	g.dev.mu.Lock()
	g.dev.err = errors.New("gone")
	g.dev.mu.Unlock()
	g.reveal("developer-tools")
	g.click("Open")
	g.tt.Until("failure toast", g.tt.Shows("Couldn't open the inspector"))
	for _, id := range []string{"diagnostics-copy", "diagnostics-logs", "developer-tools"} {
		if !g.s.seen[id] {
			t.Errorf("no row %q", id)
		}
	}
}

func TestDeveloperToolsIsUnavailableOutsideDevelopmentBuilds(t *testing.T) {
	caps := linuxCaps()
	caps.DeveloperTools = false
	g := newDiagRig(t, caps, engineReport(), nil)
	g.tt.Until("row", g.tt.Shows("Open the inspector", "Available in development builds started with SONA_INSPECTOR=1."))
	g.reveal("developer-tools")
	if err := g.tt.Click("Open the inspector"); err == nil && g.dev.count() != 0 {
		t.Error("the unavailable row opened the inspector")
	}
	if !g.s.seen["developer-tools"] {
		t.Error("no developer-tools row")
	}
}

func TestMotionStatusRefetchesEveryFiveSecondsWhileOpen(t *testing.T) {
	g := newDiagRig(t, linuxCaps(), engineReport(), nil)
	g.native(services.MotionReport{Allowed: true, Setting: true, H264: "probably", Visible: true})
	g.mot.mu.Lock()
	g.mot.st = services.MotionStatus{CacheFiles: 2, CacheBytes: 2048, Power: services.PowerState{Followed: true, Source: "ac"}}
	g.mot.mu.Unlock()
	g.tt.Until("motion group", g.tt.Shows("Mains power (AC adapter connected)", "2 videos · 2 KB"))
	count := func() int { g.mot.mu.Lock(); defer g.mot.mu.Unlock(); return g.mot.statuses }
	n := count()
	g.now = g.now.Add(6 * time.Second)
	g.tt.Until("refetched", func() bool { return count() > n })
}

func TestMotionGroupRowsFromTheNativeReport(t *testing.T) {
	g := newDiagRig(t, linuxCaps(), engineReport(), nil)
	g.native(services.MotionReport{Setting: true, ReduceEffects: true, Visible: false, Last: "media-error: decode failed"})
	g.tt.Until("rows", g.tt.Shows("Off while Reduce effects is on", "Hidden: videos pause", "Sona couldn't play it", "decode failed",
		"H.264 decoding", "HEVC decoding", "System reduced motion", "Not requested"))
}
