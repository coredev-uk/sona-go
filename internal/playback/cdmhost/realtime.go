//go:build linux || (cgo && (darwin || windows))

package cdmhost

import (
	"fmt"
	"slices"
	"strings"

	"github.com/coredev-uk/sona-go/internal/playback"
)

// Realtime is the scheduling the child's audio threads got. The child
// cannot ask for realtime priority itself: its sandbox has no D-Bus, so
// it reports the threads (threadReport) once its output plays, and the
// parent asks RTKit or the Realtime portal on its behalf
// (realtime_<os>.go). Failure is not fatal: the audio then plays at
// normal priority and may skip under heavy CPU load.
type Realtime struct {
	// State is one of playback.Realtime*, or "" while no audio thread has
	// been reported (the output has not played yet).
	State string
	// Via names who answered: "RTKit" or "portal".
	Via string
	// Priority is the SCHED_RR priority (RealtimeGranted) or the nice
	// level (RealtimeRaised).
	Priority int
	// Threads counts the child's threads that got it.
	Threads int
	// Reason says why realtime was refused or is unavailable, in the
	// backend's words (for the log).
	Reason string
	// Refusal is why realtime was refused (RealtimeRaised or
	// RealtimeDenied); "" otherwise.
	Refusal playback.RealtimeRefusal
}

// threadReport is a child→parent frame: the child's audio threads, by
// thread ID, that should run with realtime priority. The child has no PID
// namespace of its own, so its thread IDs are the parent's.
type threadReport struct {
	TIDs []int
}

// maxRealtimeThreads bounds how many threads one child gets raised over
// its life, whatever it reports. The output's PipeWire threads are one
// or two per output; a rebuilt output may bring new ones.
const maxRealtimeThreads = 8

// clientPriority is the realtime priority asked for: PipeWire's client
// default (module-rt's rt.prio), below its server's, capped by the
// broker's MaxRealtimePriority (RTKit's default is 20).
const clientPriority = 83

// rtLimits are the broker's limits, read from its properties.
type rtLimits struct {
	maxPriority int   // MaxRealtimePriority; below 1, no realtime
	minNice     int   // MinNiceLevel; 0, no raised nice level
	rttimeMax   int64 // RTTimeUSecMax: the RLIMIT_RTTIME it requires, µs
}

// rtBroker raises another process's thread: RTKit on the system bus, or
// the Realtime portal (org.freedesktop.portal.Realtime), which forwards to
// RTKit and maps process IDs out of a Flatpak sandbox.
type rtBroker interface {
	name() string
	limits() rtLimits
	makeRealtime(pid, tid, priority int) error
	makeHighPriority(pid, tid, nice int) error
	close()
}

// rtEnv is what a realtime request needs from the OS (realtime_<os>.go);
// tests replace it.
type rtEnv struct {
	// dial returns the first broker that answers, or why none did.
	dial func() (rtBroker, error)
	// limitRTTime caps process pid's RLIMIT_RTTIME at usec µs, which RTKit
	// requires before it grants realtime scheduling.
	limitRTTime func(pid int, usec int64) error
	// owns reports whether tid is a thread of process pid.
	owns func(pid, tid int) bool
}

// raise asks env's broker for realtime scheduling for the threads tids of
// process pid, and for a raised nice level for those it refuses. Threads
// pid does not own are skipped; ok is false when none was left to ask
// for.
func (env rtEnv) raise(pid int, tids []int) (rt Realtime, ok bool) {
	var own []int
	for _, tid := range tids {
		if env.owns(pid, tid) {
			own = append(own, tid)
		}
	}
	if len(own) == 0 {
		return Realtime{}, false
	}
	b, err := env.dial()
	if err != nil {
		return Realtime{State: playback.RealtimeUnavailable, Reason: err.Error()}, true
	}
	defer b.close()
	lim := b.limits()
	rt.Via = b.name()
	priority := min(clientPriority, lim.maxPriority)
	var rtErr error
	switch {
	case priority < 1:
		rtErr, rt.Refusal = fmt.Errorf("%s allows no realtime priority", b.name()), playback.RealtimeNoPriority
	case lim.rttimeMax > 0:
		if err := env.limitRTTime(pid, lim.rttimeMax); err != nil {
			rtErr, rt.Refusal = fmt.Errorf("limit the host's realtime CPU time: %w", err), playback.RealtimeCPULimit
		}
	}
	var realtime, raised int
	var errs []string
	for _, tid := range own {
		if rtErr == nil {
			err := b.makeRealtime(pid, tid, priority)
			if err == nil {
				realtime++
				continue
			}
			rtErr, rt.Refusal = fmt.Errorf("%s: %w", b.name(), err), playback.RealtimeRefused
		}
		if lim.minNice >= 0 {
			continue
		}
		if err := b.makeHighPriority(pid, tid, lim.minNice); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", b.name(), err))
			continue
		}
		raised++
	}
	switch {
	case raised > 0:
		rt.State, rt.Priority = playback.RealtimeRaised, lim.minNice
	case realtime > 0:
		rt.State, rt.Priority = playback.RealtimeGranted, priority
	default:
		rt.State = playback.RealtimeDenied
	}
	rt.Threads = realtime + raised
	if rtErr != nil {
		errs = append([]string{rtErr.Error()}, errs...)
	}
	// Each refusal once: RTKit often refuses both calls for the same reason.
	rt.Reason = strings.Join(slices.Compact(errs), "; ")
	return rt, true
}

// merge folds the outcome of a later request into rt: threads add up, and
// a request that raised nothing leaves threads raised before as they are.
func (rt Realtime) merge(next Realtime) Realtime {
	if next.Threads == 0 && rt.Threads > 0 {
		return rt
	}
	next.Threads += rt.Threads
	return next
}

// audioThreads takes the child's report of its audio threads: those not
// asked for before, up to maxRealtimeThreads per child, are raised off
// the read loop, one request at a time.
func (h *Host) audioThreads(tids []int) {
	h.rtMu.Lock()
	var fresh []int
	for _, tid := range tids {
		if len(h.rtAsked) >= maxRealtimeThreads {
			break
		}
		if tid > 0 && !h.rtAsked[tid] {
			h.rtAsked[tid] = true
			fresh = append(fresh, tid)
		}
	}
	h.rtMu.Unlock()
	if len(fresh) == 0 || h.rtEnv.dial == nil {
		return
	}
	go func() {
		h.rtReq.Lock()
		defer h.rtReq.Unlock()
		rt, ok := h.rtEnv.raise(h.cmd.Process.Pid, fresh)
		if !ok {
			return
		}
		h.rtMu.Lock()
		h.rt = h.rt.merge(rt)
		h.rtMu.Unlock()
		attrs := []any{"state", rt.State, "via", rt.Via, "priority", rt.Priority, "threads", fresh}
		switch rt.State {
		case playback.RealtimeGranted, playback.RealtimeRaised:
			h.log.Info("cdmhost: audio thread priority raised", append(attrs, "reason", rt.Reason)...)
		default:
			h.log.Warn("cdmhost: audio threads keep normal priority", append(attrs, "reason", rt.Reason)...)
		}
	}()
}

// realtime is the scheduling the child's audio threads got so far.
func (h *Host) realtime() Realtime {
	h.rtMu.Lock()
	defer h.rtMu.Unlock()
	return h.rt
}
