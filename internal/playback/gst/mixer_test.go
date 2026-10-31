//go:build linux

package gst

import (
	"math"
	"testing"
)

// fakeLane serves frames in uneven blocks, like decoder output.
type fakeLane struct {
	data  []float32 // interleaved
	pos   int       // frames consumed
	block int
	stall int // peeks that return nothing before data flows
}

func ramp(start, n int) *fakeLane {
	d := make([]float32, 2*n)
	for i := range n {
		d[2*i], d[2*i+1] = float32(start+i), -float32(start+i)
	}
	return &fakeLane{data: d, block: 333}
}

func constant(v float32, n int) *fakeLane {
	d := make([]float32, 2*n)
	for i := range d {
		d[i] = v
	}
	return &fakeLane{data: d, block: 500}
}

func (f *fakeLane) peek(max int) ([]float32, bool) {
	if f.stall > 0 {
		f.stall--
		return nil, false
	}
	left := len(f.data)/2 - f.pos
	if left == 0 {
		return nil, true
	}
	n := min(max, left, f.block-f.pos%f.block)
	return f.data[2*f.pos : 2*(f.pos+n)], false
}

func (f *fakeLane) advance(n int) { f.pos += n }

// drain runs the mixer to the end in chunks of chunk frames.
func drain(t *testing.T, m *mixer, chunk int) []float32 {
	t.Helper()
	var out []float32
	buf := make([]float32, 2*chunk)
	for range 10000 {
		n, done := m.fill(buf)
		out = append(out, buf[:2*n]...)
		m.commit(n)
		if done {
			return out
		}
	}
	t.Fatal("mixer never finished")
	return nil
}

func TestGaplessJoinIsContinuous(t *testing.T) {
	a, b := ramp(0, 5000), ramp(5000, 3000)
	b.stall = 3 // the next track's audio is late: the mixer waits, it does not skip
	var m mixer
	m.load(&track{gen: 1, r: a, length: 5000})
	if !m.queue(&track{gen: 2, r: b, length: 3000}, 0) {
		t.Fatal("queue refused")
	}
	out := drain(t, &m, 1024)
	if len(out) != 2*8000 {
		t.Fatalf("%d frames, want 8000", len(out)/2)
	}
	for i := range 8000 {
		if out[2*i] != float32(i) || out[2*i+1] != -float32(i) {
			t.Fatalf("frame %d = %v,%v: the join is not continuous", i, out[2*i], out[2*i+1])
		}
	}
	for f, want := range map[int64]struct {
		gen uint64
		pos int64
	}{0: {1, 0}, 4999: {1, 4999}, 5000: {2, 0}, 7999: {2, 2999}} {
		if gen, pos, ok := m.at(f); !ok || gen != want.gen || pos != want.pos {
			t.Errorf("at(%d) = %d, %d, %v; want track %d at %d", f, gen, pos, ok, want.gen, want.pos)
		}
	}
}

func TestLoadStartsTimelineAtTrackPosition(t *testing.T) {
	var m mixer
	m.load(&track{gen: 7, r: ramp(0, 100), start: 44100})
	drain(t, &m, 64)
	if gen, pos, _ := m.at(10); gen != 7 || pos != 44110 {
		t.Fatalf("at(10) = %d, %d; want track 7 at frame 44110", gen, pos)
	}
}

func TestCrossfadeIsEqualPower(t *testing.T) {
	const la, lb, fade = 4000, 3000, 1000
	var m mixer
	m.load(&track{gen: 1, r: constant(1, la), length: la})
	m.queue(&track{gen: 2, r: constant(1, lb), length: lb}, fade)
	out := drain(t, &m, 512)
	if len(out)/2 != la+lb-fade {
		t.Fatalf("%d frames, want %d (overlap of %d)", len(out)/2, la+lb-fade, fade)
	}
	for i := range len(out) / 2 {
		want := 1.0
		if i >= la-fade && i < la {
			x := float64(i-(la-fade)) / fade
			want = math.Cos(x*math.Pi/2) + math.Sin(x*math.Pi/2)
		}
		if math.Abs(float64(out[2*i])-want) > 1e-4 {
			t.Fatalf("frame %d = %v, want %v", i, out[2*i], want)
		}
	}
	if gen, pos, _ := m.at(la - fade); gen != 2 || pos != 0 {
		t.Errorf("the next track is reported from the crossfade's start: at(%d) = %d, %d", la-fade, gen, pos)
	}
}

func TestCrossfadeOutlivesShortTrack(t *testing.T) {
	// The current track ends earlier than its expected length: the new one
	// keeps ramping up alone, without a jump.
	var m mixer
	m.load(&track{gen: 1, r: constant(0, 3500), length: 4000})
	m.queue(&track{gen: 2, r: constant(1, 3000), length: 3000}, 1000)
	out := drain(t, &m, 256)
	for i := 1; i < len(out)/2; i++ {
		if d := out[2*i] - out[2*i-2]; d > 0.01 || d < -0.01 {
			t.Fatalf("jump of %v at frame %d", d, i)
		}
	}
}

func TestTrackGainRampsAndSwitchesAtBoundary(t *testing.T) {
	var m mixer
	m.trackGain = true
	m.load(&track{gen: 1, r: constant(1, 2048), gain: 0.5, length: 2048})
	m.queue(&track{gen: 2, r: constant(1, 1024), gain: 0.25, length: 1024}, 0)
	buf := make([]float32, 2*1024)
	n, _ := m.fill(buf)
	m.commit(n)
	if buf[0] != 0.5 || buf[2*(n-1)] != 0.5 {
		t.Fatalf("track 1 gain: %v … %v, want 0.5", buf[0], buf[2*(n-1)])
	}
	m.trackGain = false // turned off mid-track: ramps to unity over the next block
	n, _ = m.fill(buf)
	m.commit(n)
	if buf[0] <= 0.5 || buf[0] >= 0.51 || buf[2*(n-1)] != 1 {
		t.Fatalf("ramp to unity: %v … %v", buf[0], buf[2*(n-1)])
	}
	m.trackGain = true
	out := drain(t, &m, 1024)
	if out[0] != 0.25 {
		t.Fatalf("track 2 starts at gain %v, want its own 0.25 at once", out[0])
	}
}

func TestQueueRefusedAfterDrain(t *testing.T) {
	var m mixer
	m.load(&track{gen: 1, r: ramp(0, 10)})
	drain(t, &m, 64)
	if m.queue(&track{gen: 2, r: ramp(0, 10)}, 0) {
		t.Fatal("queue after the timeline ended was accepted")
	}
}
