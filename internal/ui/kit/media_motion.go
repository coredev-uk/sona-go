package kit

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// tileDwell is how long a tile must stay hovered or focused before its
// motion loads (React's TILE_DWELL_MS), so a pointer sweeping across a shelf
// starts none.
const tileDwell = 400 * time.Millisecond

// tileCSS is the width tile motion is requested at, in DIPs: the 360 px
// rendition at 2x (React's TILE_VIDEO_CSS).
const tileCSS = 180

// motionFade is how long a video takes to fade in over its still.
const motionFade = 300 * time.Millisecond

// motionPx is the device pixels a motion rendition is requested at for a
// cover drawn size DIPs wide at scale device pixels per DIP: scale clamped to
// 1–2 as artwork's density, the result to the 16–2048 the cache serves.
func motionPx(size, scale float32) int {
	scale = min(2, max(1, scale))
	return int(min(2048, max(16, size*scale+0.5)))
}

type slotState uint8

const (
	slotIdle    slotState = iota // nothing wanted or opened
	slotLoading                  // a goroutine is fetching and opening the file
	slotReady                    // player is set
	slotFailed                   // it did not load or decode: the still stays
)

// motionSlot is one cover's or tile's motion: its decoder from open to
// close. It lives in the element's Local state; the policy's registry lets
// the sweep close it when its view is no longer built.
type motionSlot struct {
	p     *MotionPolicy
	built atomic.Int64 // when a view last built it (unix ns), for the sweep

	url     string
	px      int
	state   slotState
	player  *ui.VideoPlayer
	cancel  context.CancelFunc
	attempt int  // tells a late answer from the current one
	gen     int  // counts players: each fades in
	live    bool // Play was the last call
	err     error

	hotSince time.Time // tiles: when the dwell began
}

func newMotionSlot(p *MotionPolicy) *motionSlot { return &motionSlot{p: p} }

// touch marks the slot as built now.
func (s *motionSlot) touch() {
	first := s.built.Load() == 0
	s.built.Store(s.p.now().UnixNano())
	if first {
		s.p.register(s)
	}
}

// release closes the decoder and forgets the file: nothing wants this
// motion now (the view is gone, motion is not allowed, the tile went cold).
func (s *motionSlot) release() {
	s.attempt++
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.player != nil {
		s.player.Close()
		s.player = nil
	}
	s.live, s.state, s.err, s.url, s.px = false, slotIdle, nil, "", 0
	s.hotSince = time.Time{}
	if s.p.owner == s {
		s.p.owner = nil
	}
	s.p.unregister(s)
	s.built.Store(0)
}

// sync brings the slot to what the view wants: url at px when want, nothing
// otherwise. It starts the load (off the main thread: the download, the
// decoder and its first frame) and reads its outcome.
func (s *motionSlot) sync(url string, px int, want bool) {
	if !want || url == "" {
		if s.state != slotIdle {
			s.release()
		}
		return
	}
	s.touch()
	if s.url != url || s.px != px {
		if s.state != slotIdle {
			s.release()
			s.touch()
		}
		s.url, s.px = url, px
	}
	switch s.state {
	case slotIdle:
		s.state = slotLoading
		s.attempt++
		attempt := s.attempt
		ctx, cancel := context.WithCancel(context.Background())
		s.cancel = cancel
		open := s.p.open
		go func() {
			player, err := open(ctx, url, px)
			s.p.post(func() { s.arrived(attempt, player, err) })
		}()
	case slotReady:
		if err := s.player.Err(); err != nil {
			s.p.outcome("media-error", err)
			s.err = err
			s.player.Close()
			s.player, s.live, s.state = nil, false, slotFailed
		}
	}
}

// arrived takes the outcome of a load, on the main thread.
func (s *motionSlot) arrived(attempt int, player *ui.VideoPlayer, err error) {
	if attempt != s.attempt || s.state != slotLoading {
		if player != nil {
			player.Close() // nobody wants it any more
		}
		return
	}
	s.cancel = nil
	if err != nil {
		s.state, s.err = slotFailed, err
		s.p.outcome("download-failed", err)
		return
	}
	s.player, s.state = player, slotReady
	s.gen++
	s.p.outcome("playing", nil)
}

// play plays or pauses the player (while ready) as the view says: a video
// plays only while on screen, the window shows and the page is not covered.
func (s *motionSlot) play(on bool) {
	if s.state != slotReady || s.live == on {
		return
	}
	s.live = on
	if on {
		s.player.Play()
	} else {
		s.player.Pause()
	}
}

// fill builds the muted loop into host once the file is open and decoding,
// fading in over 300 ms, with the edge outline of artwork. round makes it a
// circle. host must be the element the slot lives in or one built each frame.
func (s *motionSlot) fill(c *ui.Context, host ui.Element, size float32, round bool) ui.Element {
	if s.state != slotReady {
		return host
	}
	radius := theme.Current().Radius["sm"]
	if round {
		radius = size / 2
	}
	fade := host.Keyframes(s.gen, []ui.Keyframe{{At: 0, Value: 0}, {At: motionFade, Value: 1, Ease: theme.Current().Motion["backdrop"].Ease}})
	player := s.player
	host.Radius(radius).ClipX().ClipY()
	host.Children(func() {
		ui.Video(c, player).Size(size, size).Fit(ui.Cover).Opacity(fade)
	})
	edge := theme.C("edge")
	host.DrawOver(func(p *ui.Painter, r ui.Rect) { p.Stroke(r, edge.Alpha(fade), radius, 1) })
	return host
}

// MotionOpts are the options of MotionCover.
type MotionOpts struct {
	// VideoSize is the size the video is requested for when the cover
	// changes size in place (the now-playing view), so a resize does not
	// reload it; 0 is the cover's size.
	VideoSize float32
	// Round is a round picture (artists).
	Round bool
	// Overlay is for a cover that belongs to what covers the page (Now
	// Playing's own): it keeps playing while MotionPolicy.Covered.
	Overlay bool
	// A11yLabel names the cover (the still's alt text); decoration without.
	A11yLabel string
}

// MotionCover is a cover that moves: Apple's looping motion artwork over its
// still, when there is one (motion is the DTO's "/motion/<key>/<n>.mp4", ""
// for none) and the policy allows motion. The still shows until the first
// frame is decoded, the video then fades in over 300 ms, and the still stays
// if the video fails. The file comes from the motion cache directly (no
// HTTP), the decoder is GStreamer's, requested at VideoSize or size × the
// window's scale (at most 2x). The video plays only while on screen, the
// window shows and the page is not covered, and is dropped, decoder
// included, when the policy stops allowing motion or the cover stops being
// built.
func MotionCover(c *ui.Context, p *MotionPolicy, l *ArtLoader, art, motion string, size float32, o MotionOpts) ui.Element {
	root := ui.Box(c).Size(size, size)
	slot := *ui.Local(root, "motion", func() *motionSlot { return newMotionSlot(p) })
	root.Draw(func(pt *ui.Painter, _ ui.Rect) { l.noteScale(pt.Scale()) })
	root.Children(func() {
		Artwork(c.Key("still"), l, art, size, ArtOpts{Round: o.Round, Edge: true, A11yLabel: o.A11yLabel})
		// Not before the window's scale is known: a first request at the
		// wrong size would be dropped and the video opened again.
		want := p.Allowed() && motion != "" && l.scaleKnown()
		px := motionPx(cmpOr(o.VideoSize, size), l.scale)
		slot.sync(motion, px, want)
		on := want && p.Visible() && (o.Overlay || !p.Covered()) && root.InView()
		slot.play(on)
		slot.fill(c, ui.Box(c.Key("video")).Absolute().Top(0).Left(0).Size(size, size).IgnoreInput(true), size, o.Round)
	})
	return root
}

func cmpOr(v, fallback float32) float32 {
	if v != 0 {
		return v
	}
	return fallback
}

// TileMotionOpts are the options of TileMotion.
type TileMotionOpts struct {
	// Round is a round picture (artists).
	Round bool
	// Hot is whether the tile is hovered or has keyboard focus (read it from
	// the tile's own element: Hovered() || FocusWithin()).
	Hot bool
}

// TileMotion is a cover tile's motion artwork, played only after the tile
// has been hot (hovered or focused) for 400 ms and only one tile at a time:
// the video loads then, at the 180 DIP rendition, and is dropped as the tile
// goes cold, so a page of tiles at rest decodes nothing. Build it inside
// the tile's art box (size × size DIPs): it draws over the cover, takes no
// input and draws nothing at rest. motion is the DTO's motion URL; "" (Apple
// has none), a nil policy or motion that is not allowed make it do nothing at
// all.
func TileMotion(c *ui.Context, p *MotionPolicy, l *ArtLoader, motion string, size float32, o TileMotionOpts) ui.Element {
	host := ui.Box(c).Absolute().Top(0).Left(0).Size(size, size).IgnoreInput(true)
	slot := *ui.Local(host, "tile-motion", func() *motionSlot { return newMotionSlot(p) })
	host.Draw(func(pt *ui.Painter, _ ui.Rect) { l.noteScale(pt.Scale()) })
	hot := p.Allowed() && motion != "" && o.Hot
	if !hot {
		slot.hotSince = time.Time{}
		slot.sync("", 0, false) // cold: the decoder goes
		return host
	}
	if !l.scaleKnown() {
		return host
	}
	now := c.Now()
	if slot.hotSince.IsZero() {
		slot.hotSince = now
	}
	if wait := tileDwell - now.Sub(slot.hotSince); wait > 0 {
		c.After(wait)
		return host
	}
	if p.owner != nil && p.owner != slot {
		c.After(50 * time.Millisecond) // another tile's motion is still playing
		return host
	}
	p.owner = slot
	slot.sync(motion, motionPx(tileCSS, l.scale), true)
	slot.play(p.Visible() && !p.Covered() && host.InView())
	return slot.fill(c, host, size, o.Round)
}
