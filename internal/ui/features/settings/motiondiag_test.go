package settings

import (
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/services"
)

var mains = services.PowerState{Followed: true, Source: "mains"}

// playing is a report of motion playing with both codecs.
func playingReport() services.MotionReport {
	return services.MotionReport{Allowed: true, Setting: true, H264: "probably", HEVC: "probably", Visible: true}
}

func idleStatus() *services.MotionStatus {
	return &services.MotionStatus{Power: mains, CacheFiles: 2, CacheBytes: 2048}
}

var diagNow = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func rowOf(t *testing.T, g diagGroup, label string) diagRow {
	t.Helper()
	for _, r := range g.rows {
		if r.label == label {
			return r
		}
	}
	t.Fatalf("no row %q in %+v", label, g.rows)
	return diagRow{}
}

func TestMotionVerdictNamesTheFirstInputThatStopsMotion(t *testing.T) {
	g := motionGroup(playingReport(), idleStatus(), diagNow)
	if r := rowOf(t, g, "Motion artwork"); r.status != statusOK || r.value != "Plays where Apple has it" {
		t.Errorf("playing: %+v", r)
	}
	off := playingReport()
	off.Allowed, off.Setting = false, false
	if v := rowOf(t, motionGroup(off, idleStatus(), diagNow), "Motion artwork").value; !strings.Contains(v, "Settings") {
		t.Errorf("setting off: %q", v)
	}
	// Order: setting, reduce effects, system reduced motion, battery, decoder.
	off = playingReport()
	off.Allowed, off.ReduceEffects, off.ReducedMotion, off.OnBattery = false, true, true, true
	if v := rowOf(t, motionGroup(off, idleStatus(), diagNow), "Motion artwork").value; v != "Off while Reduce effects is on" {
		t.Errorf("reduce effects first: %q", v)
	}
	off.ReduceEffects = false
	if v := rowOf(t, motionGroup(off, idleStatus(), diagNow), "Motion artwork").value; !strings.Contains(v, "reduce motion") {
		t.Errorf("reduced motion next: %q", v)
	}
	off.ReducedMotion = false
	if v := rowOf(t, motionGroup(off, idleStatus(), diagNow), "Motion artwork").value; !strings.Contains(v, "battery") {
		t.Errorf("battery next: %q", v)
	}
	off.OnBattery = false
	if v := rowOf(t, motionGroup(off, idleStatus(), diagNow), "Motion artwork").value; !strings.Contains(v, "decode") {
		t.Errorf("decoder last: %q", v)
	}
}

func TestMissingH264IsAProblemWithThePackagesToInstall(t *testing.T) {
	none := services.MotionReport{Setting: true, Visible: true}
	g := motionGroup(none, idleStatus(), diagNow)
	if r := rowOf(t, g, "H.264 decoding"); r.status != statusError || !strings.Contains(r.detail, "gst-libav") {
		t.Errorf("h264: %+v", r)
	}
	if r := rowOf(t, g, "Motion artwork"); r.status != statusError {
		t.Errorf("verdict: %+v", r)
	}
	hevc := services.MotionReport{Setting: true, HEVC: "probably", Visible: true}
	if r := rowOf(t, motionGroup(hevc, idleStatus(), diagNow), "H.264 decoding"); r.status != statusWarning || !strings.Contains(r.detail, "HEVC is used instead") {
		t.Errorf("hevc only: %+v", r)
	}
}

func TestPowerRowExplainsUPowerAndIgnoredPeripherals(t *testing.T) {
	st := idleStatus()
	st.Power = services.PowerState{Followed: true, Source: "mains", UPower: true, Ignored: 2}
	r := rowOf(t, motionGroup(playingReport(), st, diagNow), "Power")
	if r.status != statusOK || !strings.Contains(r.detail, "2 peripheral batteries") ||
		!strings.Contains(r.detail, "UPower itself reports this computer on battery") {
		t.Errorf("power: %+v", r)
	}
	st.Power = services.PowerState{Unreadable: true}
	r = rowOf(t, motionGroup(playingReport(), st, diagNow), "Power")
	if r.status != statusWarning || r.detail != "UPower didn't answer. Sona's log says why." {
		t.Errorf("unreadable: %+v", r)
	}
	st.Power = services.PowerState{}
	if r = rowOf(t, motionGroup(playingReport(), st, diagNow), "Power"); r.status != statusNone || !strings.Contains(r.value, "Not followed") {
		t.Errorf("not followed: %+v", r)
	}
	for src, want := range map[string]string{"battery": "On battery: motion artwork pauses", "ups": "On UPS battery: motion artwork pauses"} {
		st.Power = services.PowerState{Followed: true, Source: src, OnBattery: true, UPower: true}
		if r = rowOf(t, motionGroup(playingReport(), st, diagNow), "Power"); r.value != want || r.status != statusWarning {
			t.Errorf("%s: %+v", src, r)
		}
	}
	if r = rowOf(t, motionGroup(playingReport(), nil, diagNow), "Power"); r.value != "Checking…" {
		t.Errorf("before the status: %+v", r)
	}
}

func TestLastDownloadWordsGosFailure(t *testing.T) {
	st := idleStatus()
	st.Last = services.MotionFetch{At: diagNow.Add(-time.Minute).UnixMilli(), Error: services.CodeNoRendition}
	r := rowOf(t, motionGroup(playingReport(), st, diagNow), "Last download")
	if r.status != statusWarning || r.detail != "Apple doesn't offer this video in a format Sona can play." {
		t.Errorf("failed: %+v", r)
	}
	st.Last = services.MotionFetch{At: diagNow.Add(-2 * time.Hour).UnixMilli(), Rendition: "avc1 486×486"}
	if r = rowOf(t, motionGroup(playingReport(), st, diagNow), "Last download"); r.status != statusOK || !strings.HasPrefix(r.value, "avc1 486×486 · ") {
		t.Errorf("ok: %+v", r)
	}
	st.Last = services.MotionFetch{}
	if r = rowOf(t, motionGroup(playingReport(), st, diagNow), "Last download"); r.value != "None since Sona started" {
		t.Errorf("none: %+v", r)
	}
	if r = rowOf(t, motionGroup(playingReport(), nil, diagNow), "Last download"); r.value != "Checking…" {
		t.Errorf("checking: %+v", r)
	}
	if got := rowOf(t, motionGroup(playingReport(), st, diagNow), "Motion cache").value; got != "2 videos · 2 kB" {
		t.Errorf("cache: %q", got)
	}
	for _, g := range []diagGroup{motionGroup(playingReport(), nil, diagNow)} {
		for _, row := range g.rows {
			if row.label == "Motion cache" {
				t.Error("the cache row shows before the status")
			}
		}
	}
}

func TestLastVideoFromThePolicysOutcome(t *testing.T) {
	cases := map[string]diagRow{
		"":                           {value: "None since Sona started"},
		"playing":                    {value: "Played · 5 min. ago", status: statusOK},
		"media-error: no frames":     {value: "Sona couldn't play it · 5 min. ago", status: statusWarning, detail: "no frames"},
		"download-failed: not found": {value: "Couldn't download · 5 min. ago", status: statusWarning, detail: "not found"},
	}
	for in, want := range cases {
		got := lastVideo(in, diagNow.Add(-5*time.Minute).UnixMilli(), diagNow)
		if got.value != want.value || got.status != want.status || got.detail != want.detail {
			t.Errorf("%q: %+v", in, got)
		}
	}
}

func TestSystemReducedMotionAndWindowRows(t *testing.T) {
	r := playingReport()
	r.ReducedMotion, r.Visible = true, false
	g := motionGroup(r, idleStatus(), diagNow)
	if x := rowOf(t, g, "System reduced motion"); x.value != "Requested" || !strings.Contains(x.detail, "gtk-enable-animations") {
		t.Errorf("%+v", x)
	}
	if x := rowOf(t, g, "Window"); x.value != "Hidden: videos pause" {
		t.Errorf("%+v", x)
	}
	if x := rowOf(t, g, "Setting"); x.value != "On" {
		t.Errorf("%+v", x)
	}
}

// Ported from Diagnostics.test.tsx: the rows' logic.
func TestSandboxRows(t *testing.T) {
	p := services.DiagPlayback{Sandboxed: true, SandboxKind: "seatbelt"}
	if r := sandboxRow(p); r.value != "Sandboxed by macOS · no network or keychain access" || r.status != statusOK {
		t.Errorf("seatbelt: %+v", r)
	}
	p = services.DiagPlayback{SandboxKind: "none", SandboxGaps: []playback.SandboxGap{playback.SandboxGapNoSeatbelt}}
	r := sandboxRow(p)
	if r.value != "Not sandboxed" || r.status != statusWarning || !strings.Contains(r.detail, "sandbox-exec is missing") ||
		!strings.Contains(r.detail, "the host can reach the network and your files") {
		t.Errorf("none: %+v", r)
	}
	p.CDMBlocked = true
	if r = sandboxRow(p); r.status != statusError {
		t.Errorf("blocked: %+v", r)
	}
	p = services.DiagPlayback{SandboxKind: "linux", SandboxGaps: []playback.SandboxGap{playback.SandboxGapNamespaces}}
	if r = sandboxRow(p); r.value != "Not fully sandboxed" || !strings.Contains(r.detail, "kernel.unprivileged_userns_clone") {
		t.Errorf("namespaces: %+v", r)
	}
	if r = sandboxRow(services.DiagPlayback{}); r.value != "Unknown (the host did not start)" || r.status != statusNone {
		t.Errorf("unknown: %+v", r)
	}
}

func TestRealtimeRows(t *testing.T) {
	base := services.DiagPlayback{}
	if r := realtimeRow(base); r.value != "Requested when playback starts" {
		t.Errorf("%+v", r)
	}
	base.HostRunning = true
	if r := realtimeRow(base); r.value != "Requested when audio starts" {
		t.Errorf("%+v", r)
	}
	granted := services.DiagPlayback{Realtime: playback.RealtimeGranted, RealtimeVia: "RTKit", RealtimePriority: 20}
	if r := realtimeRow(granted); r.value != "Realtime · priority 20 (RTKit)" || r.status != statusOK {
		t.Errorf("%+v", r)
	}
	raised := services.DiagPlayback{Realtime: playback.RealtimeRaised, RealtimeVia: "portal", RealtimePriority: -15, RealtimeRefusal: playback.RealtimeRefused}
	r := realtimeRow(raised)
	if r.value != "Raised · nice -15 (Realtime portal)" || r.detail != "Realtime priority was refused: Realtime portal refused it." {
		t.Errorf("%+v", r)
	}
	r = realtimeRow(services.DiagPlayback{Realtime: playback.RealtimeUnavailable})
	if r.value != "Normal" || r.status != statusWarning || !strings.Contains(r.detail, "Install and enable RTKit") {
		t.Errorf("%+v", r)
	}
	if r = realtimeRow(services.DiagPlayback{Realtime: playback.RealtimeUnsupported}); r.value != "Not requested on this platform" {
		t.Errorf("%+v", r)
	}
	r = realtimeRow(services.DiagPlayback{Realtime: playback.RealtimeDenied, RealtimeVia: "RTKit", RealtimeRefusal: playback.RealtimeNoPriority})
	if r.status != statusWarning || !strings.HasPrefix(r.detail, "RTKit allows no realtime priority. Audio may skip") {
		t.Errorf("%+v", r)
	}
}

func TestTokenStorageAndCDMRows(t *testing.T) {
	for backend, want := range map[string]string{
		"secret-service": "Secret Service keyring", "keychain": "macOS Keychain", "credential-manager": "Windows Credential Manager", "x": "x", "": "—",
	} {
		if got := keyringLabel(backend); got != want {
			t.Errorf("%q: %q", backend, got)
		}
	}
	d := noEngineReport()
	d.Keyring.Unencrypted = true
	gs := diagnosticsGroups(d, nil, diagNow)
	if r := gs[1].rows[0]; r.value != "File (unencrypted)" || r.status != statusWarning {
		t.Errorf("%+v", r)
	}
	// A failed download, a warning on a failed update check, the idle state.
	failed := services.DiagPlayback{CDM: services.CDMStatus{State: "failed", Code: "signature_invalid"}}
	if r := cdmStateRow(failed, failed.CDM); r.status != statusError || !strings.Contains(r.detail, "isn't signed by Google") || r.value != "Couldn't download" {
		t.Errorf("%+v", r)
	}
	rows := cdmSourceRows(services.CDMStatus{State: "ready", Code: "network"}, diagNow)
	if rows[1].value != "Couldn't check for updates" || rows[1].status != statusWarning {
		t.Errorf("%+v", rows)
	}
	if rows = cdmSourceRows(services.CDMStatus{}, diagNow); rows != nil {
		t.Errorf("empty state has source rows: %+v", rows)
	}
	if r := cdmStateRow(services.DiagPlayback{}, services.CDMStatus{State: "idle"}); r.value != "Not downloaded yet" {
		t.Errorf("%+v", r)
	}
	if r := cdmStateRow(services.DiagPlayback{CDMError: services.CodeNoRendition}, services.CDMStatus{State: "ready"}); r.value != "Doesn't start" || r.status != statusError || r.detail == "" {
		t.Errorf("%+v", r)
	}
	if r := cdmStateRow(services.DiagPlayback{CDMBlocked: true}, services.CDMStatus{}); r.value != "Blocked" || r.status != statusError {
		t.Errorf("%+v", r)
	}
	// Before the probe nothing is checked, which is not a problem.
	unchecked := cdmRows(services.DiagPlayback{Engine: "cdm", CDM: services.CDMStatus{State: "downloading"}}, services.CDMStatus{State: "downloading", Received: 3, Total: 4}, diagNow)
	for _, r := range unchecked {
		if (r.label == "GStreamer" || r.label == "AAC decoder" || r.label == "Audio output") && (r.value != "Checked once Widevine is ready" || r.status != statusNone) {
			t.Errorf("%+v", r)
		}
		if r.label == "Version" && r.value != "—" {
			t.Errorf("%+v", r)
		}
	}
	if p, w := tally(diagnosticsGroups(engineReport(), nil, diagNow)); p != 0 || w != 0 {
		t.Errorf("a healthy report counts %d problems, %d warnings", p, w)
	}
}
