//go:build linux || (cgo && (darwin || windows))

package gst

import "math"

// The mixer joins the decoded tracks of the lanes into the one stream the
// output plays: sample-exact one after the other (gapless), or overlapped
// with an equal-power crossfade. It knows nothing of GStreamer; lanes are
// pcmReaders and the output is the dst slices fill writes, so its timeline
// arithmetic is testable on its own.

// Rate and Channels are the format of every lane's decoded audio and of
// the output: interleaved stereo float at 44.1 kHz (Apple's AAC rate).
const (
	Rate     = 44100
	Channels = 2
)

// pcmReader is a lane's decoded audio for one track.
type pcmReader interface {
	// peek returns up to max frames that follow what was consumed,
	// waiting briefly for decoded audio. eos reports the track's end (with
	// no frames); no frames and no eos means none arrived in time.
	peek(max int) (frames []float32, eos bool)
	// advance consumes n frames of the last peek.
	advance(n int)
}

// track is one track in a lane, as the mixer plays it.
type track struct {
	gen    uint64
	r      pcmReader
	gain   float32 // Sound Check gain (linear), 1 = unity
	start  int64   // track position of its first frame
	length int64   // frames from start to the track's end; 0 if unknown
	read   int64   // frames consumed
	eos    bool
	// applied is the gain last applied; a change ramps over one block.
	applied float32
}

// mark says that output frame frame plays frame pos of track gen.
type mark struct {
	frame int64
	gen   uint64
	pos   int64
}

// mixer is the timeline: what plays now, what follows and how.
type mixer struct {
	cur, next *track
	fade      int64 // crossfade length into next in frames; 0 joins gaplessly
	// fading: frames into the crossfade (fadeLen frames long) once started.
	fading  bool
	fadePos int64
	fadeLen int64

	trackGain bool  // apply tracks' Sound Check gain
	out       int64 // frames written since the timeline started
	marks     []mark
	drained   bool // cur ended with nothing to follow
}

// load starts a new timeline with t.
func (m *mixer) load(t *track) {
	*m = mixer{trackGain: m.trackGain, cur: t, marks: []mark{{0, t.gen, t.start}}}
	t.applied = m.gainOf(t)
}

// queue makes t follow the current track, crossfading over fade frames.
// It reports false when the current track has already ended.
func (m *mixer) queue(t *track, fade int64) bool {
	if m.cur == nil || m.drained || m.fading {
		return false
	}
	m.next, m.fade = t, max(fade, 0)
	return true
}

func (m *mixer) unqueue() {
	if !m.fading {
		m.next, m.fade = nil, 0
	}
}

func (m *mixer) gainOf(t *track) float32 {
	if m.trackGain && t.gain > 0 {
		return t.gain
	}
	return 1
}

// fadeStart is the frame of cur at which the crossfade into next begins,
// or -1 when they join without one.
func (m *mixer) fadeStart() int64 {
	if m.next == nil || m.fade == 0 || m.cur.length <= 0 {
		return -1
	}
	return max(m.cur.length-m.fade, 0)
}

// promote makes next the current track.
func (m *mixer) promote() {
	n := m.next
	m.cur, m.next, m.fade = n, nil, 0
	m.fading, m.fadePos, m.fadeLen = false, 0, 0
	n.applied = m.gainOf(n)
}

// switchTo makes next current at output frame at, without a crossfade.
func (m *mixer) switchTo(at int64) {
	m.marks = append(m.marks, mark{at, m.next.gen, m.next.start + m.next.read})
	m.promote()
}

// fill writes up to len(dst)/Channels frames of the timeline into dst and
// returns how many it wrote. It returns early when a lane has no audio in
// time; done reports that the timeline has ended (cur played out with
// nothing queued).
func (m *mixer) fill(dst []float32) (n int, done bool) {
	want := len(dst) / Channels
	for n < want && m.cur != nil && !m.drained {
		cur := m.cur
		if m.fading {
			k, ok := m.mixFade(dst[n*Channels:want*Channels], m.out+int64(n))
			n += k
			if !ok {
				return n, false
			}
			continue
		}
		limit := want - n
		if fs := m.fadeStart(); fs >= 0 {
			if cur.read >= fs {
				// Start the crossfade once the next track has audio.
				f, eosNext := m.next.r.peek(1)
				switch {
				case eosNext:
					m.next, m.fade = nil, 0 // nothing to fade into
					continue
				case len(f) == 0:
					if cur.eos {
						return n, false // wait for it: the next one is due
					}
				default:
					m.fading, m.fadePos = true, 0
					m.fadeLen = min(m.fade, max(cur.length-cur.read, 1))
					m.marks = append(m.marks, mark{m.out + int64(n), m.next.gen, m.next.start})
					continue
				}
			} else {
				limit = int(min(int64(limit), fs-cur.read))
			}
		}
		if cur.eos {
			if m.next == nil {
				m.drained = true
				return n, true
			}
			if f, eos := m.next.r.peek(1); len(f) == 0 && !eos {
				return n, false // the next track has no audio yet
			}
			m.switchTo(m.out + int64(n))
			continue
		}
		f, eos := cur.r.peek(limit)
		if eos {
			cur.eos = true
			continue
		}
		if len(f) == 0 {
			return n, false
		}
		k := len(f) / Channels
		out := dst[n*Channels : (n+k)*Channels]
		g0, g1 := cur.applied, m.gainOf(cur)
		scale(out, f, g0, g1)
		cur.applied = g1
		cur.r.advance(k)
		cur.read += int64(k)
		n += k
	}
	return n, m.drained
}

// mixFade writes the crossfade into dst starting at output frame at. ok is
// false when it stopped for want of audio.
func (m *mixer) mixFade(dst []float32, at int64) (n int, ok bool) {
	a, b := m.cur, m.next
	want := len(dst) / Channels
	for n < want {
		if m.fadePos >= m.fadeLen {
			// The crossfade is over; what remains of the old track is
			// dropped (its lane is flushed when reused).
			m.promote()
			return n, true
		}
		limit := int(min(int64(want-n), m.fadeLen-m.fadePos))
		fb, eosB := b.r.peek(limit)
		if eosB {
			// The new track is shorter than the crossfade.
			b.eos = true
			m.promote()
			return n, true
		}
		if len(fb) == 0 {
			return n, false
		}
		k := len(fb) / Channels
		var fa []float32
		if !a.eos {
			var eosA bool
			fa, eosA = a.r.peek(k)
			if eosA {
				a.eos = true
			} else if len(fa) == 0 {
				return n, false
			} else {
				k = len(fa) / Channels
			}
		}
		out := dst[n*Channels : (n+k)*Channels]
		ga, gb := m.gainOf(a), m.gainOf(b)
		for i := range k {
			x := (float64(m.fadePos) + float64(i)) / float64(m.fadeLen)
			in := float32(math.Sin(x*math.Pi/2)) * gb
			o0, o1 := fb[i*Channels]*in, fb[i*Channels+1]*in
			if fa != nil {
				gout := float32(math.Cos(x*math.Pi/2)) * ga
				o0 += fa[i*Channels] * gout
				o1 += fa[i*Channels+1] * gout
			}
			out[i*Channels], out[i*Channels+1] = o0, o1
		}
		b.r.advance(k)
		b.read += int64(k)
		if fa != nil {
			a.r.advance(k)
			a.read += int64(k)
		}
		m.fadePos += int64(k)
		n += k
	}
	return n, true
}

// scale copies src into dst multiplied by a gain ramping from g0 to g1.
func scale(dst, src []float32, g0, g1 float32) {
	if g0 == 1 && g1 == 1 {
		copy(dst, src)
		return
	}
	frames := len(src) / Channels
	step := (g1 - g0) / float32(max(frames, 1))
	g := g0
	for i := range frames {
		g += step
		dst[i*Channels] = src[i*Channels] * g
		dst[i*Channels+1] = src[i*Channels+1] * g
	}
}

// commit advances the timeline by n written frames.
func (m *mixer) commit(n int) { m.out += int64(n) }

// at maps output frame f to the track playing it and its position there.
func (m *mixer) at(f int64) (gen uint64, pos int64, ok bool) {
	for i := len(m.marks) - 1; i >= 0; i-- {
		if mk := m.marks[i]; mk.frame <= f {
			return mk.gen, mk.pos + f - mk.frame, true
		}
	}
	return 0, 0, false
}

// prune forgets marks before output frame f, keeping the one in force.
func (m *mixer) prune(f int64) {
	i := 0
	for i+1 < len(m.marks) && m.marks[i+1].frame <= f {
		i++
	}
	if i > 0 {
		m.marks = append(m.marks[:0], m.marks[i:]...)
	}
}
