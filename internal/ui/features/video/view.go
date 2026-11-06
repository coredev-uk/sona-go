package video

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

const (
	// seekStep is Left and Right in the view: a shorter step than the
	// player's Shift+arrows.
	seekStep = 5 * time.Second
	// idleAfter is how long the pointer and the keys stay still before the
	// controls fade, while playing.
	idleAfter = 3 * time.Second
	// fadeFor is how long the controls take to fade.
	fadeFor = 300 * time.Millisecond
	// fadeIn is how long the view takes to appear.
	fadeIn = 200 * time.Millisecond
	// openWait is how long the view waits for Go's state to name the video
	// that opened it.
	openWait = 2 * time.Second
	// stallAfter is how long a playing video may show no new picture before
	// the view says it is buffering (the frame loop's one-second poll).
	stallAfter = time.Second
	// posterWidth is the poster's request: 16:9, the picture box's usual size.
	posterWidth = 960
)

// shapeVideo is the picture's shape before the first one: a video frame.
const shapeVideo = 16.0 / 9

// viewState is what the view keeps from frame to frame; it is dropped when the
// view closes.
type viewState struct {
	open     bool      // the layer has been built since it opened
	openedAt time.Time // when it opened
	wasVideo bool      // a video was playing when it opened
	shownAt  time.Time // when it first had a video to show
	seen     bool      // the current item has been a video since it opened

	started bool   // the picture feed runs
	uid     uint64 // the queue entry the feed was started for

	// Idle: the controls fade after idleAfter without the pointer or a key.
	wake    time.Time
	ptrX    float32
	ptrY    float32
	focused bool // a control had the keyboard focus visibly, last frame

	// The fade: how opaque the controls are, and when that was set.
	alpha   float32
	alphaAt time.Time

	// The seek bar while the pointer drags it: the position shown, committed
	// when the pointer lets go.
	seek struct {
		active bool
		secs   float64
	}

	// Pictures that stopped coming while playing say "Buffering…".
	pics  uint64
	picAt time.Time

	stage ui.Rect // the picture area as laid out this frame's predecessor
	in    float32 // how far the view has faded in (0 to 1)

	// own: a slider has the focus this frame, and keeps the arrows.
	own bool

	status string // what the status region said last, announced once
}

// layerOpen is the shell's: the view covers the window while it is open and a
// music video plays (until Go's state names the video that opened it, the
// page behind stays usable).
func (v *video) layerOpen() bool {
	return v.env.Video.IsOpen() && v.playingVideo()
}

func (v *video) playingVideo() bool {
	cur := v.env.Play.State().Current
	return cur != nil && cur.Video
}

// closed drops what the view held: its pictures and its state.
func (v *video) closed() {
	if !v.view.open {
		return
	}
	if v.feed != nil {
		v.feed.Stop()
	}
	v.view = viewState{}
}

// layer is the shell's Layer slot: the video view over the whole window while
// it is open. It closes the view when the queue moves on from the video, and
// when the play that opened it never reaches the playback mirror as a video
// within two seconds.
func (v *video) layer(c *ui.Context) {
	st := &v.view
	if !v.env.Video.IsOpen() {
		v.closed()
		// However it closed (Escape, Close video, the toggle key, the bar's
		// Show video), the focus goes back to what opened it.
		v.env.Video.ReturnFocus(c)
		return
	}
	now := c.Now()
	if !st.open {
		*st = viewState{open: true, openedAt: now, wake: now, alpha: 1, alphaAt: now}
		st.wasVideo = v.playingVideo()
	}
	s := v.env.Play.State()
	if s.Current == nil || !s.Current.Video {
		// Opened by a play before Go's state named the video: wait a moment.
		// A video that was playing and moved on closes the view at once.
		if st.seen || now.Sub(st.openedAt) >= openWait {
			v.env.Video.Close()
			v.closed()
			c.Invalidate()
			return
		}
		c.After(openWait - now.Sub(st.openedAt))
		return
	}
	// A video that named itself after the view opened (a play opened it
	// first): any picture on show is the item before's.
	fresh := !st.seen && !st.wasVideo
	if !st.seen {
		st.shownAt = now
	}
	st.seen = true
	v.screen(c, s, fresh)
}

// startFeed starts the picture feed for the queue entry uid; fresh takes only
// pictures shown from now on.
func (v *video) startFeed(uid uint64, fresh bool) {
	st := &v.view
	st.uid = uid
	if v.feed == nil || st.started {
		return
	}
	st.started = true
	if fresh {
		v.feed.Restart()
	} else {
		v.feed.Start()
	}
}

// hasPicture reports whether a picture of the current video is on show.
func (v *video) hasPicture() bool { return v.feed != nil && v.feed.HasPicture() }

// problem is why there is no picture, in the picture's place.
type problem struct {
	title, detail string
	retry         bool
}

// screen is the view's content for the playing music video: its picture,
// letterboxed on black at the picture's shape, between the title strip and the
// transport. The controls fade while the video plays and nothing moves, and
// come back with the pointer, any key, a pause, or focus.
func (v *video) screen(c *ui.Context, s services.PlaybackState, fresh bool) {
	st := &v.view
	cur := s.Current
	now := c.Now()
	if !st.started {
		v.startFeed(cur.UID, fresh)
	} else if st.uid != cur.UID {
		// A picture of an earlier queue entry is never shown for this one.
		st.started = false
		v.startFeed(cur.UID, true)
	}

	caps, known := v.caps()
	playing, loading := s.State == store.StatePlaying, s.State == store.StateLoading

	var pr *problem
	switch {
	case known && !caps.Video:
		pr = &problem{
			title:  i18n.T("Music videos don't play here"),
			detail: i18n.T("This platform or build of Sona can't play music videos. You can watch it in Apple Music."),
		}
	case s.State == store.StateError && s.Error != "":
		pr = &problem{
			title:  i18n.T("Couldn't play this video"),
			detail: i18n.ErrorText(s.Error),
			// Pressing play again can't change what Apple offers or refuses.
			retry: s.Error != services.CodeVideoHDOnly && s.Error != services.CodeVideoLicence,
		}
	}

	// Pictures that stop coming while playing are buffering.
	if v.feed != nil {
		if n := v.feed.Pictures(); n != st.pics {
			st.pics, st.picAt = n, now
		}
	}
	stalled := false
	if playing && v.hasPicture() {
		if since := now.Sub(st.picAt); since >= stallAfter {
			stalled = true
		} else {
			c.After(stallAfter - since)
		}
	}
	status := ""
	if pr == nil && (loading || playing) {
		switch {
		case !v.hasPicture():
			status = i18n.T("Loading the video…")
		case loading || stalled:
			status = i18n.T("Buffering…")
		}
	}
	if status != st.status {
		st.status = status
		if status != "" {
			c.Announce(status)
		}
	}

	// The controls: shown while paused, loading, on a problem, or while one
	// has the keyboard focus (Tab never lands on an unseen control); else
	// they fade once nothing has moved for idleAfter.
	idle := now.Sub(st.wake) >= idleAfter
	if playing && !idle {
		c.After(idleAfter - now.Sub(st.wake))
	}
	shown := !playing || !idle || st.focused || status != "" || pr != nil
	v.fade(c, shown)

	help := i18n.T("Space or K plays and pauses, Left and Right go back or ahead 5 seconds, F switches full screen, Escape closes the video.")
	if caps.Volume {
		help = i18n.T("Space or K plays and pauses, Left and Right go back or ahead 5 seconds, F switches full screen, M mutes, Escape closes the video.")
	}
	root := ui.Column(c.Key("video-view")).Fill().Background(theme.C("ground")).Role(ui.RoleGroup).
		Label(i18n.T("Music video")).Description(help).TabStop(false).FocusRing(false).AutoFocus()
	if !shown {
		root.Cursor(ui.CursorNone)
	}
	// The view fades in over 200 ms.
	st.in = 1
	if d := now.Sub(st.shownAt); !theme.ReducedMotion() && d < fadeIn {
		st.in = float32(d) / float32(fadeIn)
		root.Opacity(st.in)
		c.After(16 * time.Millisecond)
	}
	// Any pointer move or press, and any key that reaches the view, brings
	// the controls back.
	if x, y, over := root.PointerPosition(); over && (x != st.ptrX || y != st.ptrY) {
		st.ptrX, st.ptrY = x, y
		st.wake = now
	}
	root.HandleInput(func(ev ui.InputEvent) bool {
		switch ev.Kind {
		case ui.InputKeyDown, ui.InputPointerDown, ui.InputPointerMove:
			st.wake = ev.Time
		}
		return false
	})
	focusVisible := false
	st.own = false
	root.Children(func() {
		v.header(c, cur, pr, status, &focusVisible)
		v.stage(c, cur, pr)
		v.footer(c, s, caps, playing, loading, &focusVisible)
	})
	if focusVisible != st.focused {
		st.focused = focusVisible
		c.Invalidate()
	}
	v.keys(c, root, caps.Volume)
}

// fade moves the controls' opacity toward shown over fadeFor (at once when
// motion is reduced).
func (v *video) fade(c *ui.Context, shown bool) {
	st := &v.view
	now := c.Now()
	want := float32(0)
	if shown {
		want = 1
	}
	if theme.ReducedMotion() || st.alpha == want {
		st.alpha, st.alphaAt = want, now
		return
	}
	step := float32(now.Sub(st.alphaAt)) / float32(fadeFor)
	st.alphaAt = now
	switch {
	case st.alpha < want:
		st.alpha = min(st.alpha+step, want)
	default:
		st.alpha = max(st.alpha-step, want)
	}
	if st.alpha != want {
		c.After(16 * time.Millisecond)
	}
}

// keys are the view's keys, which go to it while the focus is in it (an
// element around the focus takes a key before the window's shortcuts do, as
// v0.2.0's onKeyDown did): Space or K play and pause, Left and Right seek 5 s,
// F is full screen, M mutes, Escape leaves full screen, then closes the view.
// A focused button keeps Space and a focused slider its arrows; Shift+arrows
// are not the view's, they go on to the player's 10 s seek.
func (v *video) keys(c *ui.Context, root ui.Element, volume bool) {
	st := &v.view
	now := c.Now()
	pressed := func(mods ui.Modifiers, key ui.Key) bool {
		if !root.Shortcut(mods, key) {
			return false
		}
		st.wake = now
		return true
	}
	// A button or link with the focus keeps Space (the view only asks for it
	// while the focus is on the view itself); a slider does nothing with it
	// and keeps its arrows, so the view takes Space there and lets it go.
	onView := root.Focused() || !root.FocusWithin()
	if (onView || st.own) && pressed(0, ui.KeySpace) && onView {
		v.toggle()
	}
	if pressed(0, ui.KeyK) || pressed(ui.Shift, ui.KeyK) {
		v.toggle()
	}
	if !st.own && pressed(0, ui.KeyLeft) {
		v.seekBy(-seekStep, now)
	}
	if !st.own && pressed(0, ui.KeyRight) {
		v.seekBy(seekStep, now)
	}
	if pressed(0, ui.KeyF) || pressed(ui.Shift, ui.KeyF) {
		v.env.Video.SetFullscreen(!v.env.Video.Fullscreen())
	}
	if pressed(0, ui.KeyM) || pressed(ui.Shift, ui.KeyM) {
		if volume {
			v.toggleMute()
		}
	}
	if pressed(0, ui.KeyEscape) {
		if v.env.Video.Fullscreen() {
			v.env.Video.SetFullscreen(false)
		} else {
			v.env.Video.Close()
			c.Invalidate()
		}
	}
}

// text is a single line in one of the theme's type styles.
func text(c *ui.Context, s, style string) ui.Element {
	t := theme.Current().Type[style]
	return ui.Text(c, s).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).
		LetterSpacing(t.Tracking).SingleLine().MinWidth(0)
}

// control notes a control's keyboard focus for the idle rule and returns it.
func control(e ui.Element, focusVisible *bool) ui.Element {
	if e.FocusVisible() {
		*focusVisible = true
	}
	return e
}

// owns notes that a slider has the focus, which keeps the arrows from the
// view.
func (v *video) owns(e ui.Element) ui.Element {
	if e.Focused() {
		v.view.own = true
	}
	return e
}

// header is the strip over the picture: the title and the artist, which link
// to their pages and close the view, the "SD" mark, the status, and the Full
// screen and Close buttons.
func (v *video) header(c *ui.Context, cur *services.QueueItem, pr *problem, status string, focusVisible *bool) {
	st := &v.view
	id := cur.CatalogID
	if id == "" {
		id = cur.ID
	}
	// Over a hidden title bar (zero in full screen) the picture's top strip
	// is the window's: the buttons it has stay clear of the window's, and the
	// strip drags the window.
	bar := c.TitleBar()
	lead, trail := bar.Left, bar.Right
	if c.Direction() == ui.RightToLeft {
		lead, trail = trail, lead
	}
	head := ui.Row(c.Key("header")).Height(64).Shrink(0).PaddingStart(max(32, lead+8)).PaddingEnd(max(32, trail+8)).
		Gap(16).AlignItems(ui.Center).Opacity(st.alpha)
	if bar.Height > 0 {
		head.DragWindow()
	}
	head.Children(func() {
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
				h := theme.Current().Type["heading"]
				if path := "/music-video/" + id; v.hasRoute(path) {
					l := ui.Link(c.Key("title"), "", "").Shrink(1).MinWidth(0)
					l.Children(func() {
						ui.Text(c, cur.Name).FontSize(h.Size).FixedLineHeight(h.LineHeight).FontWeight(int(h.Weight)).
							TextColor(theme.C("ink")).SingleLine()
					})
					control(l, focusVisible)
					if l.Clicked() {
						v.env.Video.Close()
						v.env.Nav.Push(path)
					}
				} else {
					text(c, cur.Name, "heading").TextColor(theme.C("ink"))
				}
				if pr == nil {
					kit.SDMark(c)
				}
				// A new paragraph per state, so each is announced.
				ui.Row(c.Key("status")).Shrink(0).AlignItems(ui.Center).Role(ui.RoleStatus).Children(func() {
					if status == "" {
						return
					}
					ui.Row(c.Key(status)).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Spinner(c).Size(14, 14).Role(ui.RoleNone) // the status names it
						text(c, status, "meta").TextColor(theme.C("ink-muted"))
					})
				})
			})
			if path := "/artist/" + cur.ArtistID; cur.ArtistID != "" && v.hasRoute(path) {
				l := ui.Link(c.Key("artist"), "", "").MinWidth(0)
				l.Children(func() { text(c, cur.Artist, "sub").TextColor(theme.C("ink-muted")) })
				control(l, focusVisible)
				if l.Clicked() {
					v.env.Video.Close()
					v.env.Nav.Push(path)
				}
			} else {
				text(c, cur.Artist, "sub").TextColor(theme.C("ink-muted"))
			}
		})
		icon, name := "maximize", i18n.T("Full screen")
		full := v.env.Video.Fullscreen()
		if full {
			icon, name = "minimize", i18n.T("Exit full screen")
		}
		if control(kit.IconButton(c.Key("full"), icon, name, kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}), focusVisible).Clicked() {
			st.wake = c.Now()
			v.env.Video.SetFullscreen(!full)
		}
		if control(kit.IconButton(c.Key("close"), "x", i18n.T("Close video"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}), focusVisible).Clicked() {
			v.env.Video.Close()
			c.Invalidate()
		}
	})
}

// fitPicture is the largest box of the picture's shape that fits box: the
// picture is letterboxed, never cropped or stretched. Empty before either is
// measured.
func fitPicture(pw, ph, bw, bh float32) (w, h float32) {
	if pw <= 0 || ph <= 0 || bw <= 0 || bh <= 0 {
		return 0, 0
	}
	scale := min(bw/pw, bh/ph)
	return float32(int(pw * scale)), float32(int(ph * scale))
}

// stage is the area between the strip and the transport: the picture at its
// own shape on black (16:9 before the first picture, with the video's cover
// as a poster until it comes), or the problem in its place. The picture box
// stays built under a problem, so the feed keeps running for a retry.
func (v *video) stage(c *ui.Context, cur *services.QueueItem, pr *problem) {
	st := &v.view
	stage := ui.Column(c.Key("stage")).Grow(1).Basis(0).MinHeight(0).MarginX(32).Center().Clip()
	pw, ph := float32(16), float32(9)
	if w, h := v.pictureSize(); w > 0 {
		pw, ph = float32(w), float32(h)
	}
	fw, fh := fitPicture(pw, ph, st.stage.W, st.stage.H)
	stage.Children(func() {
		if pr != nil {
			v.problemBox(c, cur, pr)
		}
		ui.Box(c.Key("picture")).Size(fw, fh).Background(ui.RGB(0, 0, 0)).Clip().Hidden(pr != nil).Children(func() {
			if v.feed != nil {
				kit.VideoSurface(c.Key("surface"), v.feed, kit.VideoOpts{})
			}
			if !v.hasPicture() && cur.Art.Large != "" {
				ui.Box(c.Key("poster")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Children(func() {
					kit.FluidArtwork(c, v.env.Art, cur.Art.Large, posterWidth, kit.ArtOpts{Aspect: shapeVideo, Radius: 0.01})
				})
			}
		})
	})
	if b := stage.Bounds(); b.W != st.stage.W || b.H != st.stage.H {
		st.stage = b
		c.Invalidate()
	}
}

func (v *video) pictureSize() (w, h int) {
	if v.feed == nil {
		return 0, 0
	}
	return v.feed.PictureSize()
}

// problemBox says why there is no picture, in the picture's place, with Apple
// Music as the way to watch.
func (v *video) problemBox(c *ui.Context, cur *services.QueueItem, pr *problem) {
	e := ui.Row(c.Key("problem")).MaxWidth(520).Gap(12).AlignItems(ui.Start).Padding(16, 20).
		Radius(theme.Current().Radius["lg"]).Border(1, theme.C("control-line")).BorderStyle(ui.BorderDashed).
		Role(ui.RoleStatus).Label(pr.title)
	announced := ui.Local(e, "announced", func() string { return "" })
	if *announced != pr.title+pr.detail {
		*announced = pr.title + pr.detail
		c.Announce(pr.title + " " + pr.detail)
	}
	id := cur.CatalogID
	if id == "" {
		id = cur.ID
	}
	e.Children(func() {
		ui.Icon(c, theme.Icon("circle-alert")).FontSize(16).TextColor(theme.C("danger")).Margin(2, 0, 0, 0).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			t := theme.Current().Type["label"]
			ui.Text(c, pr.title).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).TextColor(theme.C("ink"))
			ui.Text(c, pr.detail).TextColor(theme.C("ink-muted")).FontSize(theme.Current().Type["sub"].Size).Margin(2, 0, 0, 0)
			ui.Row(c).Margin(12, 0, 0, 0).Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
				v.watchInAppleMusic(c, id)
				if pr.retry {
					if kit.Button(c.Key("retry"), i18n.T("Try again"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).Clicked() {
						v.toggle()
					}
				}
			})
		})
	})
}

// watchInAppleMusic is the "Watch in Apple Music" button, once the video's
// page (with its link) has loaded.
func (v *video) watchInAppleMusic(c *ui.Context, id string) {
	q := v.videoQuery(id)
	if !q.HasData || q.Data.Video.URL == "" {
		return
	}
	if kit.Button(c.Key("watch"), i18n.T("Watch in Apple Music"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Icon: "external-link"}).Clicked() {
		v.openInAppleMusic(q.Data.Video.URL)
	}
}

// footer is the transport: the seek bar, then previous, play or pause and
// next in the middle and the volume at the right.
func (v *video) footer(c *ui.Context, s services.PlaybackState, caps services.PlaybackCapabilities, playing, loading bool, focusVisible *bool) {
	st := &v.view
	ui.Column(c.Key("footer")).Shrink(0).Padding(12, 32, 24).Opacity(st.alpha).Children(func() {
		v.seekBar(c, s, playing, focusVisible)
		ui.Row(c).FillWidth().Margin(8, 0, 0, 0).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Grow(1).Basis(0).Height(1)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if control(kit.IconButton(c.Key("prev"), "skip-back", i18n.T("Previous"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Filled: true, Disabled: !s.Controls.Previous}), focusVisible).Clicked() {
					v.previous()
				}
				icon, name := "play", i18n.T("Play")
				switch {
				case loading:
					icon = "loader-circle"
				case playing:
					icon, name = "pause", i18n.T("Pause")
				}
				disc := kit.Button(c.Key("play"), "", kit.ButtonOpts{Variant: kit.Play, Size: kit.SizeLG, Icon: icon, Spin: loading, Filled: !loading, A11yLabel: name}).Size(44, 44)
				if control(disc, focusVisible).Clicked() {
					v.toggle()
				}
				if control(kit.IconButton(c.Key("next"), "skip-forward", i18n.T("Next"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Filled: true, Disabled: !s.Controls.Next}), focusVisible).Clicked() {
					v.next()
				}
			})
			ui.Row(c).Grow(1).Basis(0).Justify(ui.End).AlignItems(ui.Center).Gap(4).Children(func() {
				if caps.Volume {
					v.volume(c, s, focusVisible)
				}
			})
		})
	})
}

// seekBar is the elapsed time, the slider and what is left, live from the
// playback mirror. Dragging shows where it will go and seeks once, when the
// pointer lets go (or a key moved it).
func (v *video) seekBar(c *ui.Context, s services.PlaybackState, playing bool, focusVisible *bool) {
	st := &v.view
	dur := time.Duration(s.DurationMs) * time.Millisecond
	pos := v.env.Play.Position()
	if st.seek.active {
		pos = time.Duration(st.seek.secs * float64(time.Second))
	}
	pos = min(pos, dur)
	if dur <= 0 {
		pos = 0
	}
	ui.Row(c.Key("seek")).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
		text(c, i18n.FormatDuration(pos), "meta").Width(48).TextAlign(ui.End).Shrink(0).TextColor(theme.C("ink-muted")).FontFeatures("tnum")
		secs := pos.Seconds()
		at, total := i18n.FormatDuration(pos), i18n.FormatDuration(dur)
		sl := kit.Slider(c.Key("seek-slider"), &secs, 0, max(dur.Seconds(), 1), kit.SliderOpts{
			A11yLabel: i18n.T("Seek"),
			ValueText: i18n.T("{at} of {total}", "at", at, "total", total),
			Disabled:  dur <= 0,
			Step:      1,
			Range:     theme.C("amber"),
		})
		control(v.owns(sl), focusVisible)
		if sl.Changed() {
			st.seek.active, st.seek.secs = true, secs
		}
		if kit.SliderCommitted(sl) && st.seek.active {
			st.seek.active = false
			v.seekTo(time.Duration(st.seek.secs*float64(time.Second)), c.Now())
		}
		left := i18n.FormatDuration(max(dur-pos, 0))
		text(c, "−"+left, "meta").Width(48).Shrink(0).TextColor(theme.C("ink-muted")).FontFeatures("tnum").
			Label(i18n.T("{left} left", "left", left))
	})
	if playing && !st.seek.active {
		c.After(250 * time.Millisecond) // the position ticks
	}
}

// volume is the mute button and the volume slider.
func (v *video) volume(c *ui.Context, s services.PlaybackState, focusVisible *bool) {
	icon, name := "volume-2", i18n.T("Mute")
	switch {
	case s.Volume == 0:
		icon, name = "volume-x", i18n.T("Unmute")
	case s.Volume < 0.5:
		icon = "volume-1"
	}
	if control(kit.IconButton(c.Key("mute"), icon, name, kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}), focusVisible).Clicked() {
		v.toggleMute()
	}
	pct := s.Volume * 100
	sl := kit.Slider(c.Key("volume"), &pct, 0, 100, kit.SliderOpts{
		A11yLabel: i18n.T("Volume"), Width: 96, Range: theme.C("ink-muted"), // 0 to 100: no value text, which would say the number a third time
	})
	control(v.owns(sl), focusVisible)
	if sl.Changed() {
		v.setVolume(pct / 100)
	}
}
