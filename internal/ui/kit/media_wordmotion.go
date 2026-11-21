package kit

import (
	"math"
	"strings"
	"time"
	"unicode"
)

// The motion of a word-timed lyric line: every word lifts a little as it is
// sung, and a long held word swells letter by letter with a glow, the motion
// Apple Music has and the players modelled on it share. The numbers come from
// Apple Music-like Lyrics (AMLL, AGPL-3.0, github.com/amll-dev/
// applemusic-like-lyrics, commit ccca63ce, packages/core/src/lyric-player:
// dom/animation/float/index.ts, dom/animation/emphasize/index.ts,
// base/line.ts shouldEmphasize), which replicates the iPad's lyrics; Spicy
// Lyrics (AGPL-3.0, github.com/Spikerko/spicy-lyrics, commit 1c02f47c,
// src/utils/Lyrics/Animator/Lyrics/LyricsAnimator.ts) does the same with
// springs. This file is the arithmetic, in time and em, with no painting.

const (
	// liftEm is how far a word rises as it is sung (-0.05em; background
	// vocals 0.10em): the float animation's end, held while the line is sung.
	liftEm = 0.05
	// liftMin is the shortest a word's rise takes, however short the word:
	// max(1 s, the word's own length), eased out from the word's begin.
	liftMin = time.Second
	// empMin is how long a word is held to swell, and empMaxChars the longest
	// (CJK: no longest) it may be, trimmed.
	empMin      = time.Second
	empMinChars = 2
	empMaxChars = 7
	// swellStep is the steps the swell's scale is quantised to, and
	// swellSteps how many: the letters are shaped at each of them, so that a
	// swollen letter is drawn at its size and stays sharp (MyGo v0.9.0
	// rasterises text before it transforms it, and a scaled glyph blurs).
	swellStep  = 0.02
	swellSteps = 6
)

// bezier is a CSS cubic-bezier(x1, y1, x2, y2) easing: the value at progress x.
type bezier struct{ x1, y1, x2, y2 float64 }

func (b bezier) at(x float32) float32 {
	switch {
	case x <= 0:
		return 0
	case x >= 1:
		return 1
	}
	// The curve parameter whose x is the progress, by bisection (x is
	// monotonic for the control points here), then its y.
	coord := func(t, a, c float64) float64 {
		u := 1 - t
		return 3*u*u*t*a + 3*u*t*t*c + t*t*t
	}
	lo, hi := 0.0, 1.0
	for range 24 {
		mid := (lo + hi) / 2
		if coord(mid, b.x1, b.x2) < float64(x) {
			lo = mid
		} else {
			hi = mid
		}
	}
	return float32(coord((lo+hi)/2, b.y1, b.y2))
}

var (
	easeOut = bezier{0, 0, 0.58, 1}     // CSS ease-out: the rise
	empIn   = bezier{0.2, 0.4, 0.58, 1} // the swell's way up
	empOut  = bezier{0.3, 0, 0.58, 1}   // and, reversed, down
)

// empEasing is the swell's envelope over progress x of its length: up to 1
// at half way by empIn, then back down to 0 by 1 − empOut.
func empEasing(x float32) float32 {
	if x < 0.5 {
		return empIn.at(x / 0.5)
	}
	return 1 - empOut.at((x-0.5)/0.5)
}

// emphasizes reports whether a word of text sung for dur gets the swell
// instead of the plain lift: held for a second or more and, trimmed, two to
// seven characters long; CJK (Han, kana, Hangul and their punctuation) at a
// second whatever its length (AMLL's shouldEmphasize).
func emphasizes(text string, dur time.Duration) bool {
	if dur < empMin {
		return false
	}
	text = strings.TrimSpace(text)
	n := 0
	cjk := text != ""
	for _, r := range text {
		n++
		cjk = cjk && isCJK(r)
	}
	return cjk || n >= empMinChars && n <= empMaxChars
}

func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) ||
		r >= 0x3000 && r <= 0x303F || r >= 0xFF00 && r <= 0xFFEF
}

// swell is the arithmetic of a held word's swell: how strong it is, how long
// each letter's takes and how the letters follow one another.
type swell struct {
	amount, blur float32       // the scale, offset and glow's strengths
	du           time.Duration // one letter's swell
	n            int           // letters
	bg           bool
}

// newSwell is the swell of a word held for dur (a second at least), of n
// letters; last says it ends the line, which takes it further (×1.6 the
// swell, ×1.5 the glow, ×1.2 as long), and bg a background voice, which
// lifts twice as far.
func newSwell(dur time.Duration, n int, last, bg bool) swell {
	du := float64(max(dur, empMin)) / float64(time.Millisecond)
	amount, blur := du/2000, du/3000
	for _, v := range []*float64{&amount, &blur} {
		if *v > 1 {
			*v = math.Sqrt(*v)
		} else {
			*v = *v * *v * *v
		}
	}
	amount *= 0.6
	blur *= 0.5
	if last {
		amount *= 1.6
		blur *= 1.5
		du *= 1.2
	}
	return swell{
		amount: float32(min(amount, 1.2)), blur: float32(min(blur, 0.8)),
		du: time.Duration(du * float64(time.Millisecond)), n: max(n, 1), bg: bg,
	}
}

// delay is when letter i starts after the word does: du / 2.5 / n apart.
func (w swell) delay(i int) time.Duration {
	return time.Duration(float64(w.du) / 2.5 / float64(w.n) * float64(i))
}

// total is how long the whole word's swell runs: the last letter's delay and
// its float, which lasts 1.4 du.
func (w swell) total() time.Duration {
	return w.delay(w.n-1) + time.Duration(float64(w.du)*1.4)
}

// glowEm is the radius of the glow, in em: min(0.3, 0.3 blur).
func (w swell) glowEm() float32 { return min(0.3, w.blur*0.3) }

// letterPose is where letter i stands s into the word: its scale about its
// own centre, its offset in em (dy negative is up) and the glow's alpha.
type letterPose struct {
	scale, dx, dy, glow float32
}

func (w swell) pose(i int, s time.Duration) letterPose {
	local := s - w.delay(i)
	if local <= 0 {
		return letterPose{scale: 1}
	}
	e := empEasing(clamp01(float32(local) / float32(w.du)))
	// The float: a rise of 0.05em at its end, with a bump of 0.05em on the
	// way (twice both for a background voice) over 1.4 du.
	x := float64(clamp01(float32(local) / (1.4 * float32(w.du))))
	k := float32(1)
	if w.bg {
		k = 2
	}
	float := k * float32(math.Sin(math.Pi*x)*liftEm+x*liftEm)
	return letterPose{
		scale: 1 + e*0.1*w.amount,
		dx:    -e * 0.03 * w.amount * (float32(w.n)/2 - float32(i)),
		dy:    -e*0.025*w.amount - float,
		glow:  e * w.blur,
	}
}

func clamp01(x float32) float32 { return min(max(x, 0), 1) }

// wordClock is how far into a word's motion the voice is at t: t − begin
// while the line is sung (until is when it stops), and once it has stopped
// the motion plays back the way it came, from where it had got (no further
// than total, its end) at the speed it went: AMLL plays the float with
// playbackRate −1 as a line is deactivated. Zero or less is not begun.
func wordClock(t, begin, until, total time.Duration) time.Duration {
	if t <= until {
		return t - begin
	}
	return min(until-begin, total) - (t - until)
}
