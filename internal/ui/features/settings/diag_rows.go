package settings

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// diagStatus is the verdict of a diagnostics row.
type diagStatus uint8

const (
	statusNone diagStatus = iota
	statusOK
	statusWarning
	statusError
)

// diagRow is one label/value row of Diagnostics (Diagnostics.tsx's Row).
type diagRow struct {
	label, value string
	status       diagStatus
	// detail is what to do about a warning or error, or what the value means.
	detail string
}

// diagGroup is a titled group of rows.
type diagGroup struct {
	id, title string
	rows      []diagRow
}

const dash = "—"

// seatbeltOff is the variable that turns the macOS sandbox off
// (cdmhost.EnvSeatbelt).
const seatbeltOff = "SONA_SEATBELT=0"

// sandboxGap says what a missing layer of the host's sandbox means
// (playback.SandboxGap), as a sentence.
func sandboxGap(gap playback.SandboxGap) string {
	switch gap {
	case playback.SandboxGapNamespaces:
		return i18n.T("It has no network or mount namespace of its own, because unprivileged user namespaces are blocked.")
	case playback.SandboxGapFilesystem:
		return i18n.T("It has no private view of the filesystem.")
	case playback.SandboxGapLandlock:
		return i18n.T("Landlock doesn't restrict its files.")
	case playback.SandboxGapSeccomp:
		return i18n.T("Its seccomp filter isn't installed.")
	case playback.SandboxGapNetwork:
		return i18n.T("It can open network connections.")
	case playback.SandboxGapKeychain:
		return i18n.T("It can read your keychain folder.")
	case playback.SandboxGapTurnedOff:
		return i18n.T("The macOS sandbox is turned off ({SEATBELT_OFF}).", "SEATBELT_OFF", seatbeltOff)
	case playback.SandboxGapNoSeatbelt:
		return i18n.T("macOS's sandbox-exec is missing or didn't start.")
	case playback.SandboxGapRefused:
		return i18n.T("macOS's sandbox-exec refused Sona's profile.")
	case playback.SandboxGapNoAppContainer:
		return i18n.T("Windows couldn't set up its AppContainer sandbox for it.")
	case playback.SandboxGapUserFiles:
		return i18n.T("It can list your app data folders.")
	}
	return ""
}

// realtimeRefusal says why realtime priority was refused
// (playback.RealtimeRefusal), as a sentence.
func realtimeRefusal(r playback.RealtimeRefusal, via string) string {
	switch r {
	case playback.RealtimeNoPriority:
		return i18n.T("{via} allows no realtime priority.", "via", via)
	case playback.RealtimeCPULimit:
		return i18n.T("Sona couldn't limit the host's realtime CPU time, which {via} requires.", "via", via)
	case playback.RealtimeRefused:
		return i18n.T("{via} refused it.", "via", via)
	}
	return ""
}

// sandboxRow is the playback host's confinement (cdmhost.Sandbox): namespaces,
// Landlock and seccomp on Linux; macOS's sandbox-exec profile, or none, on
// macOS; an AppContainer in a job object, or none, on Windows.
func sandboxRow(p services.DiagPlayback) diagRow {
	label := i18n.T("Host sandbox")
	if p.Sandboxed {
		switch p.SandboxKind {
		case playback.SandboxSeatbelt:
			return diagRow{label: label, value: i18n.T("Sandboxed by macOS · no network or keychain access"), status: statusOK}
		case playback.SandboxAppContainer:
			return diagRow{label: label, value: i18n.T("Sandboxed by Windows · no network or file access"), status: statusOK}
		}
		return diagRow{label: label, value: i18n.T("Sandboxed · no network or session access"), status: statusOK}
	}
	gaps := p.SandboxGaps
	if p.SandboxKind == "" || len(gaps) == 0 {
		return diagRow{label: label, value: i18n.T("Unknown (the host did not start)")}
	}
	words := make([]string, len(gaps))
	hasNamespaces, noAppContainer := false, false
	for i, g := range gaps {
		words[i] = sandboxGap(g)
		hasNamespaces = hasNamespaces || g == playback.SandboxGapNamespaces
		noAppContainer = noAppContainer || g == playback.SandboxGapNoAppContainer
	}
	note := strings.Join(words, " ")
	if p.CDMBlocked {
		return diagRow{label: label, value: i18n.T("Not sandboxed"), status: statusError, detail: note}
	}
	if p.SandboxKind == playback.SandboxNone {
		if noAppContainer {
			return diagRow{
				label: label, value: i18n.T("Not sandboxed"), status: statusWarning,
				detail: i18n.T("{note} Previews still play, but Sona loads the Widevine CDM only inside its sandbox, so full songs can't.", "note", note),
			}
		}
		return diagRow{
			label: label, value: i18n.T("Not sandboxed"), status: statusWarning,
			detail: i18n.T("{note} Playback still works, but the host can reach the network and your files. macOS's sandbox-exec confines it where it works.", "note", note),
		}
	}
	detail := i18n.T("{note} Playback still works.", "note", note)
	if hasNamespaces {
		detail = i18n.T("{note} Playback still works. To sandbox it fully, allow unprivileged user namespaces (sysctl kernel.unprivileged_userns_clone, or the AppArmor user namespace restriction).", "note", note)
	}
	return diagRow{label: label, value: i18n.T("Not fully sandboxed"), status: statusWarning, detail: detail}
}

// realtimeRow is the scheduling the playing host's audio thread got
// (cdmhost.Realtime). The sandboxed host has no D-Bus, so on Linux Sona asks
// RTKit or the Realtime portal for it on the host's behalf.
func realtimeRow(p services.DiagPlayback) diagRow {
	label := i18n.T("Audio priority")
	priority := strconv.Itoa(p.RealtimePriority)
	via := p.RealtimeVia
	if via == "portal" {
		via = i18n.T("Realtime portal")
	}
	refusal := realtimeRefusal(p.RealtimeRefusal, via)
	skips := i18n.T("Audio may skip while the computer is under heavy load.")
	switch p.Realtime {
	case playback.RealtimeGranted:
		return diagRow{label: label, value: i18n.T("Realtime · priority {priority} ({via})", "priority", priority, "via", via), status: statusOK}
	case playback.RealtimeRaised:
		detail := i18n.T("Realtime priority was refused.")
		if refusal != "" {
			detail = i18n.T("Realtime priority was refused: {refusal}", "refusal", refusal)
		}
		return diagRow{label: label, value: i18n.T("Raised · nice {priority} ({via})", "priority", priority, "via", via), status: statusOK, detail: detail}
	case playback.RealtimeDenied:
		detail := skips
		if refusal != "" {
			detail = refusal + " " + skips
		}
		return diagRow{label: label, value: i18n.T("Normal"), status: statusWarning, detail: detail}
	case playback.RealtimeUnavailable:
		return diagRow{
			label: label, value: i18n.T("Normal"), status: statusWarning,
			detail: i18n.T("Neither RTKit nor the Realtime portal is available. {skips} Install and enable RTKit (rtkit) to fix this.", "skips", skips),
		}
	case playback.RealtimeUnsupported:
		return diagRow{label: label, value: i18n.T("Not requested on this platform")}
	}
	if p.HostRunning {
		return diagRow{label: label, value: i18n.T("Requested when audio starts")}
	}
	return diagRow{label: label, value: i18n.T("Requested when playback starts")}
}

// keyringLabel is the token storage backend Go reports, by name; any other
// keeps Go's name.
func keyringLabel(backend string) string {
	switch backend {
	case "secret-service":
		return i18n.T("Secret Service keyring")
	case "keychain":
		return i18n.T("macOS Keychain")
	case "credential-manager":
		return i18n.T("Windows Credential Manager")
	}
	if backend == "" {
		return dash
	}
	return backend
}

// cdmPercent is a running CDM download's progress as a percentage, "" while
// its size is unknown (features/cdm/state.ts cdmPercent).
func cdmPercent(c services.CDMStatus) string {
	if c.Total <= 0 {
		return ""
	}
	return i18n.FormatPercent(math.Min(1, math.Max(0, float64(c.Received)/float64(c.Total))))
}

// cdmStateRow is the Widevine CDM's verdict: the probe's when it loaded one,
// else the download's state.
func cdmStateRow(p services.DiagPlayback, cdm services.CDMStatus) diagRow {
	label := i18n.T("Widevine CDM")
	if p.CDMOK {
		return diagRow{label: label, value: i18n.T("Working"), status: statusOK}
	}
	if p.CDMBlocked {
		return diagRow{
			label: label, value: i18n.T("Blocked"), status: statusError,
			detail: i18n.T("Sona loads the CDM only inside its sandbox, which keeps it off the network, and the sandbox could not be set up. Only 30-second previews play."),
		}
	}
	switch cdm.State {
	case "idle":
		return diagRow{
			label: label, value: i18n.T("Not downloaded yet"),
			detail: i18n.T("Sona downloads it from Google once it's online. Songs play when it's ready."),
		}
	case "downloading":
		value := i18n.T("Downloading…")
		if pc := cdmPercent(cdm); pc != "" {
			value = i18n.T("Downloading · {percent}", "percent", pc)
		}
		return diagRow{label: label, value: value, detail: i18n.T("Songs play when the download finishes.")}
	case "failed":
		return diagRow{label: label, value: i18n.CDMFailedState(cdm.Code), status: statusError, detail: i18n.CDMProblem(cdm.Code)}
	}
	row := diagRow{label: label, value: i18n.T("Doesn't start"), status: statusError}
	if p.CDMError != "" {
		row.detail = i18n.ErrorText(p.CDMError)
	}
	return row
}

// cdmSourceRows say where the CDM comes from and how its updates are going.
func cdmSourceRows(cdm services.CDMStatus, now time.Time) []diagRow {
	if cdm.State == "" {
		return nil
	}
	var updates diagRow
	switch {
	case cdm.State == "ready" && cdm.Updating:
		value := i18n.T("Downloading an update")
		if pc := cdmPercent(cdm); pc != "" {
			value = i18n.T("Downloading an update · {percent}", "percent", pc)
		}
		updates = diagRow{label: i18n.T("Updates"), value: value, detail: i18n.T("It applies the next time playback starts.")}
	case cdm.State == "ready" && cdm.Code != "":
		updates = diagRow{label: i18n.T("Updates"), value: i18n.T("Couldn't check for updates"), status: statusWarning, detail: i18n.CDMProblem(cdm.Code)}
	case cdm.CheckedAt > 0:
		updates = diagRow{label: i18n.T("Updates"), value: i18n.T("Checked {checked}", "checked", i18n.RelativeTime(stampTime(cdm.CheckedAt), now))}
	default:
		updates = diagRow{label: i18n.T("Updates"), value: i18n.T("Not checked yet")}
	}
	return []diagRow{{label: i18n.T("Source"), value: i18n.T("Downloaded from Google")}, updates}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func orDash(s string) string {
	if s == "" {
		return dash
	}
	return s
}

// cdmRows are the CDM playback engine's checks: the CDM, GStreamer, the host
// and its sandbox. The probe child that checks GStreamer and the sandbox
// needs a CDM to load, so without one (the report's CDM not ready) those rows
// say they weren't checked rather than failing.
func cdmRows(p services.DiagPlayback, cdm services.CDMStatus, now time.Time) []diagRow {
	probed := p.CDM.State == "ready" || p.CDMOK || p.CDMBlocked
	unchecked := i18n.T("Checked once Widevine is ready")
	rows := []diagRow{cdmStateRow(p, cdm)}
	rows = append(rows, cdmSourceRows(cdm, now)...)
	iface := dash
	if p.CDMInterface != 0 {
		iface = i18n.T("{cdmInterface} (host {hostInterface})", "cdmInterface", p.CDMInterface, "hostInterface", p.HostInterface)
	}
	rows = append(rows,
		diagRow{label: i18n.T("Version"), value: orDash(firstNonEmpty(cdm.Version, p.CDMVersion))},
		diagRow{label: i18n.T("CDM file"), value: orDash(firstNonEmpty(p.CDMPath, cdm.Path))},
		diagRow{label: i18n.T("CDM interface"), value: iface},
	)
	check := func(label, found, missing, hint string, notFound diagStatus) diagRow {
		switch {
		case !probed:
			return diagRow{label: label, value: unchecked}
		case found != "":
			return diagRow{label: label, value: found, status: statusOK}
		}
		return diagRow{label: label, value: missing, status: notFound, detail: hint}
	}
	rows = append(rows,
		check(i18n.T("GStreamer"), p.GStreamer, i18n.T("Not found"), i18n.T("Install GStreamer 1.x with the base and good plugins."), statusError),
		check(i18n.T("AAC decoder"), p.Decoder, i18n.T("None"), i18n.T("Install gst-libav or the fdkaac plugin to decode Apple Music's AAC."), statusError),
		check(i18n.T("Audio output"), p.Sink, i18n.T("None"), i18n.T("No PipeWire or PulseAudio sink was found; GStreamer falls back to autoaudiosink."), statusWarning),
	)
	host := i18n.T("Stopped (starts with the next song)")
	if p.HostRunning {
		host = i18n.T("Running · pid {hostPid}", "hostPid", p.HostPID)
	}
	rows = append(rows,
		diagRow{label: i18n.T("Playback host"), value: host},
		sandboxRow(p),
		realtimeRow(p),
		diagRow{
			label:  i18n.T("Stream format"),
			value:  i18n.T("AAC {audioQuality} kbps", "audioQuality", p.AudioQuality),
			detail: i18n.T("Sona plays AAC only. Albums marked Lossless or Dolby Atmos play as AAC: Apple doesn't license lossless to desktop Widevine players."),
		},
	)
	return rows
}

// diagnosticsGroups is the Diagnostics struct as labelled rows with a verdict
// where one applies, in the page's order. The WebKitGTK rows (Rendering,
// the web inspector switch) don't exist natively. live is the CDM's status
// from cdm:status, newer than the report's copy while a download runs (nil:
// the report's).
func diagnosticsGroups(d services.Diagnostics, live *services.CDMStatus, now time.Time) []diagGroup {
	p := d.Playback
	cdm := p.CDM
	if live != nil {
		cdm = *live
	}
	playbackRows := []diagRow{{label: i18n.T("Playback"), value: i18n.T("Not available on this platform yet")}}
	// Engine "" means this platform has no playback engine, so no probe ran.
	if p.Engine != "" {
		playbackRows = cdmRows(p, cdm, now)
	}
	token := diagRow{label: i18n.T("Token storage"), value: keyringLabel(d.Keyring.Backend), status: statusOK}
	goos, _, _ := strings.Cut(d.App.Platform, "/") // "darwin/arm64": the notice names this OS's keyring
	if d.Keyring.Unencrypted {
		token = diagRow{
			label: i18n.T("Token storage"), value: i18n.T("File (unencrypted)"), status: statusWarning,
			detail: kit.KeyringWords(goos).TokenInFileDiag,
		}
	}
	c := d.Database.Counts
	contents := strings.Join([]string{
		i18n.T("{songs, plural, one {# song} other {# songs}}", "songs", c.Songs),
		i18n.T("{albums, plural, one {# album} other {# albums}}", "albums", c.Albums),
		i18n.T("{artists, plural, one {# artist} other {# artists}}", "artists", c.Artists),
		i18n.T("{playlists, plural, one {# playlist} other {# playlists}}", "playlists", c.Playlists),
	}, ", ")
	lastSync := i18n.T("Never")
	if d.Database.LastSync > 0 {
		lastSync = i18n.RelativeTime(stampTime(d.Database.LastSync), now)
	}
	startup := dash
	if d.App.StartupMs > 0 {
		startup = i18n.T("{startupMs} ms", "startupMs", d.App.StartupMs)
	}
	return []diagGroup{
		{id: "playback", title: i18n.T("Playback"), rows: playbackRows},
		{id: "credentials", title: i18n.T("Credentials"), rows: []diagRow{token}},
		{id: "database", title: i18n.T("Library database"), rows: []diagRow{
			{label: i18n.T("File"), value: d.Database.Path},
			{label: i18n.T("Size"), value: i18n.FormatBytes(d.Database.SizeBytes)},
			{label: i18n.T("Schema version"), value: strconv.FormatInt(d.Database.SchemaVersion, 10)},
			{label: i18n.T("Contents"), value: contents},
			{label: i18n.T("Last full sync"), value: lastSync},
			{label: i18n.T("Artwork cache"), value: i18n.FormatBytes(d.ArtworkCacheBytes)},
		}},
		{id: "sona", title: i18n.T("Sona"), rows: []diagRow{
			{label: i18n.T("Version"), value: orDash(d.App.Version)},
			{label: i18n.T("Go"), value: d.App.GoVersion},
			{label: i18n.T("Platform"), value: d.App.Platform},
			{label: i18n.T("Start-up to first paint"), value: startup},
			{label: i18n.T("Log file"), value: d.Paths.LogFile},
			{label: i18n.T("Settings"), value: d.Paths.Settings},
			{label: i18n.T("Data"), value: d.Paths.Data},
			{label: i18n.T("Cache"), value: d.Paths.Cache},
		}},
	}
}

// tally counts the problems and the warnings of the rows of every group.
func tally(groups []diagGroup) (problems, warnings int) {
	for _, g := range groups {
		for _, r := range g.rows {
			switch r.status {
			case statusError:
				problems++
			case statusWarning:
				warnings++
			}
		}
	}
	return problems, warnings
}
