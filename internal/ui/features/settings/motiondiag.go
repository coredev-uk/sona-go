package settings

import (
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// motionState is what decides whether motion artwork plays, as Diagnostics
// reads it: the native policy's report (env.Motion.Report) and Go's power
// state (MotionService.Status). v0.2.0's MotionState.
type motionState struct {
	allowed, setting, reduceEffects, reducedMotion bool
	onBattery                                      bool
	// power is Go's power decision; nil until MotionService.Status answers.
	power *services.PowerState
	// h264 and hevc: the decoder can play Apple's renditions in that codec.
	h264, hevc bool
}

// motionStateOf is the state from the policy's report and Go's status (nil
// until it answers).
func motionStateOf(r services.MotionReport, st *services.MotionStatus) motionState {
	m := motionState{
		allowed: r.Allowed, setting: r.Setting, reduceEffects: r.ReduceEffects, reducedMotion: r.ReducedMotion,
		onBattery: r.OnBattery, h264: r.H264 != "", hevc: r.HEVC != "",
	}
	if st != nil {
		p := st.Power
		m.power = &p
	}
	return m
}

func onOff(on bool) string {
	if on {
		return i18n.T("On")
	}
	return i18n.T("Off")
}

// motionVerdict is why motion is on or off, from the first input that stops it.
func motionVerdict(s motionState) diagRow {
	label := i18n.T("Motion artwork")
	if s.allowed {
		return diagRow{label: label, value: i18n.T("Plays where Apple has it"), status: statusOK}
	}
	var why string
	switch {
	case !s.setting:
		why = i18n.T("Off in Settings → Appearance")
	case s.reduceEffects:
		why = i18n.T("Off while Reduce effects is on")
	case s.reducedMotion:
		why = i18n.T("Off: your system asks apps to reduce motion")
	case s.onBattery:
		why = i18n.T("Paused on battery")
	default:
		why = i18n.T("Off: Sona can't decode Apple's videos")
	}
	row := diagRow{label: label, value: why}
	if !s.h264 && !s.hevc {
		row.status = statusError
	}
	return row
}

// powerRow is the power source Go decided, and why.
func powerRow(p *services.PowerState) diagRow {
	label := i18n.T("Power")
	if p == nil {
		return diagRow{label: label, value: i18n.T("Checking…")}
	}
	if !p.Followed {
		if p.Unreadable {
			return diagRow{
				label: label, value: i18n.T("Unknown: counted as mains power"), status: statusWarning,
				detail: i18n.T("UPower didn't answer. Sona's log says why."),
			}
		}
		return diagRow{label: label, value: i18n.T("Not followed on this platform: counted as mains power")}
	}
	var notes []string
	if p.Ignored > 0 {
		// translators: {0} is the number of peripheral batteries the motion report left out.
		notes = append(notes, i18n.T("{0, plural, one {# peripheral battery (a mouse, keyboard or headset) is ignored.} other {# peripheral batteries (mice, keyboards, headsets) are ignored.}}", "0", p.Ignored))
	}
	if p.UPower != p.OnBattery {
		if p.UPower {
			notes = append(notes, i18n.T("UPower itself reports this computer on battery; Sona decides from the power devices instead."))
		} else {
			notes = append(notes, i18n.T("UPower itself reports mains power; Sona decides from the power devices instead."))
		}
	}
	detail := strings.Join(notes, " ")
	switch p.Source {
	case "ac":
		return diagRow{label: label, value: i18n.T("Mains power (AC adapter connected)"), status: statusOK, detail: detail}
	case "battery":
		return diagRow{label: label, value: i18n.T("On battery: motion artwork pauses"), status: statusWarning, detail: detail}
	case "ups":
		return diagRow{label: label, value: i18n.T("On UPS battery: motion artwork pauses"), status: statusWarning, detail: detail}
	}
	return diagRow{label: label, value: i18n.T("Mains power (no battery powers this computer)"), status: statusOK, detail: detail}
}

// lastVideo is the newest motion video, from the policy's outcome string
// ("playing", "media-error: <err>", "download-failed: <err>"; "" before any)
// and when it was recorded (Unix milliseconds).
func lastVideo(last string, at int64, now time.Time) diagRow {
	label := i18n.T("Last video")
	if last == "" {
		return diagRow{label: label, value: i18n.T("None since Sona started")}
	}
	when := i18n.RelativeTime(stampTime(at), now)
	kind, detail, _ := strings.Cut(last, ": ")
	switch kind {
	case "loading":
		return diagRow{label: label, value: i18n.T("Loading · {when}", "when", when)}
	case "playing":
		return diagRow{label: label, value: i18n.T("Played · {when}", "when", when), status: statusOK}
	case "download-failed":
		return diagRow{label: label, value: i18n.T("Couldn't download · {when}", "when", when), status: statusWarning, detail: detail}
	case "media-error":
		return diagRow{label: label, value: i18n.T("Sona couldn't play it · {when}", "when", when), status: statusWarning, detail: detail}
	}
	return diagRow{label: label, value: last, status: statusWarning}
}

// lastDownload is the newest rendition Go downloaded from Apple.
func lastDownload(st *services.MotionStatus, now time.Time) diagRow {
	label := i18n.T("Last download")
	if st == nil {
		return diagRow{label: label, value: i18n.T("Checking…")}
	}
	l := st.Last
	if l.At == 0 {
		return diagRow{label: label, value: i18n.T("None since Sona started")}
	}
	when := i18n.RelativeTime(stampTime(l.At), now)
	if l.Error != "" {
		detail := i18n.ErrorText(l.Error)
		if l.Rendition != "" {
			detail = l.Rendition + ": " + detail
		}
		return diagRow{label: label, value: i18n.T("Failed · {when}", "when", when), status: statusWarning, detail: detail}
	}
	return diagRow{label: label, value: l.Rendition + " · " + when, status: statusOK}
}

// motionGroup is Settings → Diagnostics → Motion artwork: every input that
// decides whether motion plays, what the decoder can play, and how the
// newest video and download went (MotionDiagnostics.ts motionGroup). visible
// is whether the window shows.
func motionGroup(r services.MotionReport, st *services.MotionStatus, now time.Time) diagGroup {
	s := motionStateOf(r, st)
	decoderHelp := i18n.T("Install gst-libav (avdec_h264) and gst-plugins-bad (h264parse) for GStreamer, then restart Sona.")
	h264 := diagRow{label: i18n.T("H.264 decoding"), value: i18n.T("Supported"), status: statusOK}
	if !s.h264 {
		h264 = diagRow{label: i18n.T("H.264 decoding"), value: i18n.T("Not supported"), status: statusError, detail: decoderHelp}
		if s.hevc {
			h264.status = statusWarning
			h264.detail = i18n.T("HEVC is used instead. {decoderHelp}", "decoderHelp", decoderHelp)
		}
	}
	reduced := diagRow{label: i18n.T("System reduced motion"), value: i18n.T("Not requested")}
	if s.reducedMotion {
		reduced.value = i18n.T("Requested")
		reduced.detail = i18n.T("Your desktop asks apps to reduce motion (GTK's gtk-enable-animations is off).")
	}
	window := i18n.T("Visible")
	if !r.Visible {
		window = i18n.T("Hidden: videos pause")
	}
	hevc := i18n.T("Not supported")
	if s.hevc {
		hevc = i18n.T("Supported")
	}
	rows := []diagRow{
		motionVerdict(s),
		{label: i18n.T("Setting"), value: onOff(s.setting)},
		{label: i18n.T("Reduce effects"), value: onOff(s.reduceEffects)},
		reduced,
		powerRow(s.power),
		h264,
		{label: i18n.T("HEVC decoding"), value: hevc},
		{label: i18n.T("Window"), value: window},
		lastVideo(r.Last, r.LastAt, now),
		lastDownload(st, now),
	}
	if st != nil {
		// translators: {0} is the number of videos in the cache.
		videos := i18n.T("{0, plural, one {# video} other {# videos}}", "0", st.CacheFiles)
		rows = append(rows, diagRow{label: i18n.T("Motion cache"), value: videos + " · " + i18n.FormatBytes(st.CacheBytes)})
	}
	return diagGroup{id: "motion", title: i18n.T("Motion artwork"), rows: rows}
}
