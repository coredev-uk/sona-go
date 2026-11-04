package player

import (
	"fmt"
	"net/url"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// bar is NowPlayingBar: the song across the left, the transport and the
// seek bar in the middle, the sleep timer, lyrics, queue and volume at the
// right, 72 high with a line above it, over a wash of the cover.
//
//	[cover] Title ☆               shuffle prev PLAY next repeat          [moon][mic][queue][mute] ──o──
//	        Artist · Album        0:42 ━━━━━━o────────────── 3:58
func (p *player) bar(c *ui.Context) {
	p.toasts(c)
	s := p.env.Play.State()
	p.announceTrack(c, s)
	tk := theme.Current()
	backdrop := ""
	if s.Current != nil {
		backdrop = s.Current.Backdrop
	}
	ui.Column(c).Height(tk.Metric["player-bar"]).FillWidth().Background(theme.C("surface")).Label(i18n.T("Player")).Role(ui.RoleGroup).Children(func() {
		ui.Box(c).Height(1).FillWidth().Background(theme.C("line"))
		kit.Backdrop(c, p.env.Art, kit.BackdropOpts{Src: backdrop}, func() {
			ui.Row(c).Fill().Padding(0, 16).Gap(24).AlignItems(ui.Center).Children(func() {
				p.song(c, s)
				p.transport(c, s)
				p.volumeControls(c, s)
			})
		}).Grow(1).Basis(0).MinHeight(0).FillWidth()
	})
}

// song is the left column: the cover, the title and the line under it.
func (p *player) song(c *ui.Context, s services.PlaybackState) {
	cur := s.Current
	ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
		p.cover(c, s)
		ui.Column(c).Shrink(1).MinWidth(0).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).MinWidth(0).Children(func() {
				if cur == nil {
					label(c, i18n.T("Not playing"), "label").TextColor(theme.C("ink-muted"))
					return
				}
				p.link(c, "title", cur.Name, p.titlePath(cur), "label", theme.C("ink"))
				if s.Preview {
					// Apple's 30-second clip instead of the song (SONA_PREVIEW=1).
					tag(c, i18n.TC("a song playing as its 30-second preview", "Preview"), i18n.T("Apple Music's 30-second preview, not the full song"))
				}
			})
			p.subline(c, s)
		})
		p.ratingButtons(c, cur, true)
	})
}

// cover is the cover in a button that opens or closes Now Playing; what a
// local broadcaster says is on air wins over the station's art.
func (p *player) cover(c *ui.Context, s services.PlaybackState) {
	cur := s.Current
	src := ""
	if cur != nil {
		src = cur.Art.Small
		if s.OnAir != nil && s.OnAir.Art.Small != "" {
			src = s.OnAir.Art.Small
		}
	}
	open := p.ui.overlay != overlayNone
	name := i18n.T("Open now playing")
	switch {
	case cur == nil:
		name = i18n.T("Nothing playing")
	case open:
		name = i18n.T("Close now playing")
	}
	b := ui.ButtonBase(c.Key("cover")).Label(name).Shrink(0).Radius(theme.Current().Radius["sm"]).Padding(0)
	b.Background(ui.Transparent)
	if cur == nil {
		b.Disabled(true)
	}
	b.Children(func() { kit.Artwork(c, p.env.Art, src, 48, kit.ArtOpts{}).Shrink(0) })
	if b.Clicked() && cur != nil {
		if open {
			p.ui.closeOverlay()
		} else {
			p.ui.openNowPlaying("")
		}
	}
}

// titlePath is the page the playing item's name links to ("" for none): a
// live station its page (a local broadcaster has none), a music video its
// page, a song its album with the song marked.
func (p *player) titlePath(cur *services.QueueItem) string {
	switch {
	case cur.Live && cur.Broadcast:
		return ""
	case cur.Live:
		return "/station/" + url.PathEscape(cur.ID)
	case cur.Video:
		id := cur.CatalogID
		if id == "" {
			id = cur.ID
		}
		return "/music-video/" + url.PathEscape(id)
	case cur.AlbumID != "":
		return "/album/" + url.PathEscape(cur.AlbumID) + "?song=" + url.QueryEscape(cur.ID)
	}
	return ""
}

// link is text that goes to path in the router when that page is ported;
// else plain text. Outside a page ui.Link would hand its URL to the
// browser, so it has an empty URL and the click pushes the path.
func (p *player) link(c *ui.Context, key, text, path, style string, color ui.Color) {
	if path == "" || p.env.HasRoute == nil || !p.env.HasRoute(path) {
		label(c, text, style).TextColor(color)
		return
	}
	t := theme.Current().Type[style]
	l := ui.Link(c.Key(key), text, "").FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).
		LetterSpacing(t.Tracking).SingleLine().MinWidth(0).Shrink(1).TextColor(color).Cursor(ui.CursorPointer)
	if l.Hovered() {
		l.Underline()
	}
	if l.Clicked() {
		p.env.Nav.Push(path)
	}
}

// subline is the line under the title: what needs the user, what failed,
// or the artist and album.
func (p *player) subline(c *ui.Context, s services.PlaybackState) {
	cur := s.Current
	switch {
	case cur == nil:
		label(c, i18n.T("Choose a song, album or playlist to start"), "sub").TextColor(theme.C("ink-faint"))
	case s.AuthRequired:
		// The account dialog belongs to the account module: until the bar can
		// open it (module.Env.Surfaces), the line is plain text.
		label(c, i18n.T("Sign in again to keep playing"), "sub").TextColor(theme.C("ink"))
	case s.NoCDM:
		p.env.PlayerNotice.Build(c) // features/cdm's notice
	case s.State == "error" && s.Error != "":
		// The line says what happened, the tooltip why (Go's error code, worded).
		ui.Row(c).Gap(6).AlignItems(ui.Center).MinWidth(0).Tooltip(i18n.ErrorText(s.Error)).Children(func() {
			ui.Icon(c, theme.Icon("circle-alert")).FontSize(14).TextColor(theme.C("danger"))
			msg := i18n.T("Couldn't play this song. Press play to try again.")
			if cur.Video {
				msg = i18n.T("Couldn't play this video.")
			}
			label(c, msg, "sub").TextColor(theme.C("ink"))
		})
	case cur.Artist == "" && cur.Album == "":
		// A broadcaster names no provider: its name stands alone.
	case cur.ArtistID != "" && p.env.HasRoute != nil && p.env.HasRoute("/artist/"+url.PathEscape(cur.ArtistID)):
		ui.Row(c).MinWidth(0).Shrink(1).AlignItems(ui.Center).Children(func() {
			p.link(c, "artist", cur.Artist, "/artist/"+url.PathEscape(cur.ArtistID), "sub", theme.C("ink-muted"))
			if cur.Album != "" {
				label(c, " · "+cur.Album, "sub").TextColor(theme.C("ink-muted"))
			}
		})
	default:
		line := cur.Artist
		if cur.Album != "" {
			line += " · " + cur.Album
		}
		label(c, line, "sub").TextColor(theme.C("ink-muted"))
	}
}

// transport is the middle column: the buttons, then the seek bar (a live
// station has a live marker instead).
func (p *player) transport(c *ui.Context, s services.PlaybackState) {
	idle := s.Current == nil
	playing := s.State == store.StatePlaying
	loading := s.State == store.StateLoading
	ui.Column(c).Basis(560).Grow(0).Shrink(1).MinWidth(280).Gap(4).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			shuffle := unavailable(s, ctlShuffle)
			on := s.Shuffle && shuffle == ""
			name := i18n.T("Shuffle off")
			if on {
				name = i18n.T("Shuffle on")
			}
			if p.iconButton(c, "shuffle", "shuffle", name, idle, shuffle, &on) {
				p.toggleShuffle()
			}
			if p.iconButton(c, "prev", "skip-back", i18n.T("Previous"), idle || (!s.Controls.Previous && unavailable(s, ctlPrevious) == ""), unavailable(s, ctlPrevious), nil) {
				p.previous()
			}

			icon, name := "play", i18n.T("Play")
			switch {
			case loading:
				icon = "loader-circle"
			case playing:
				icon, name = "pause", i18n.T("Pause")
			}
			if kit.Button(c.Key("play"), "", kit.ButtonOpts{Variant: kit.Play, Icon: icon, Disabled: idle, A11yLabel: name}).Clicked() {
				p.toggle()
			}

			if p.iconButton(c, "next", "skip-forward", i18n.T("Next"), idle || (!s.Controls.Next && unavailable(s, ctlNext) == ""), unavailable(s, ctlNext), nil) {
				p.next()
			}
			repeat := unavailable(s, ctlRepeat)
			on = s.Repeat != "off" && repeat == ""
			rname, ricon := i18n.T("Repeat off"), "repeat"
			switch {
			case repeat != "" || s.Repeat == "off":
			case s.Repeat == "all":
				rname = i18n.T("Repeat all")
			default:
				rname, ricon = i18n.T("Repeat one"), "repeat-1"
			}
			if p.iconButton(c, "repeat", ricon, rname, idle, repeat, &on) {
				p.cycleRepeat()
			}
		})
		if s.Current != nil && s.Current.Live {
			p.live(c, s)
			return
		}
		p.seekBar(c, s, idle)
	})
}

// iconButton is a quiet icon button of the bar. disabled is for what can't
// be pressed now (nothing playing, the end of the queue); reason is for a
// control that doesn't apply to what plays (a live station) and says why in
// its tooltip. It reports a click.
func (p *player) iconButton(c *ui.Context, key, icon, name string, disabled bool, reason string, pressed *bool) bool {
	a11y := name
	if reason != "" {
		a11y = name + " · " + reason
		disabled = true
	}
	btn := kit.Button(c.Key(key), "", kit.ButtonOpts{
		Variant: kit.Ghost, Size: kit.SizeSM, Icon: icon, Pressed: pressed, Disabled: disabled, A11yLabel: a11y,
	})
	if key == "queue-toggle" {
		btn.Bind(&p.queue.toggle) // the queue's Escape returns the focus here
	}
	return btn.Clicked()
}

// live is a live station's line in place of the seek bar: a marker, and
// what the station says is on air.
func (p *player) live(c *ui.Context, s services.PlaybackState) {
	ui.Row(c).Height(16).FillWidth().Gap(8).Justify(ui.Center).AlignItems(ui.Center).MinWidth(0).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			dot := theme.C("ink-faint")
			if s.State == store.StatePlaying {
				dot = theme.C("play")
			}
			ui.Box(c).Size(6, 6).Radius(3).Background(dot)
			label(c, i18n.T("Live"), "meta").FontWeight(500).TextColor(theme.C("ink"))
		})
		if on := s.OnAir; on != nil {
			line := on.Title
			if on.Artist != "" {
				line = on.Artist + " – " + on.Title
			}
			label(c, line, "meta").TextColor(theme.C("ink-muted"))
		}
	})
}

// seekBar is the elapsed time, the slider and the length. Dragging shows
// where it will go and seeks once, when the pointer lets go (or a key moved
// it).
func (p *player) seekBar(c *ui.Context, s services.PlaybackState, idle bool) {
	dur := time.Duration(s.DurationMs) * time.Millisecond
	pos := p.env.Play.Position()
	if p.seek.active {
		pos = time.Duration(p.seek.secs * float64(time.Second))
	}
	if idle {
		pos = 0
	}
	can := !idle && dur > 0 && s.Controls.Seek
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
		elapsed := "–:––"
		if !idle {
			elapsed = clock(pos)
		}
		label(c, elapsed, "meta").Width(40).TextAlign(ui.End).TextColor(theme.C("ink-faint")).FontFeatures("tnum")

		v := pos.Seconds()
		sl := slider(c.Key("seek"), &v, 0, max(dur.Seconds(), 1), theme.C("amber"), !can)
		sl.Label(i18n.T("Seek")).Description(clock(pos) + " / " + clock(dur))
		if sl.Changed() {
			p.seek.active, p.seek.secs = true, v
		}
		if p.seek.active && !sl.Pressed() {
			p.seek.active = false
			p.seekTo(time.Duration(p.seek.secs * float64(time.Second)))
		}

		total := "–:––"
		if !idle {
			total = clock(dur)
		}
		label(c, total, "meta").Width(40).TextColor(theme.C("ink-faint")).FontFeatures("tnum")
	})
	if s.State == store.StatePlaying && !p.seek.active {
		c.After(250 * time.Millisecond) // the position ticks (v0.2.0's useLivePosition)
	}
}

// volumeControls is the right column: the sleep timer, lyrics, the queue,
// then mute and the volume slider, where Sona sets the playback volume.
func (p *player) volumeControls(c *ui.Context, s services.PlaybackState) {
	idle := s.Current == nil
	ui.Row(c).Grow(1).Basis(0).Justify(ui.End).AlignItems(ui.Center).Gap(4).Children(func() {
		p.env.PlayerControls.Build(c) // the other features' controls: private listening…
		p.sleepButton(c)
		if p.lyricsSurface() {
			on := p.ui.overlay == overlayNowPlaying && shownPane(p.ui.pane, s.Current) == paneLyrics
			if p.iconButton(c, "lyrics", "mic-vocal", i18n.T("Lyrics"), idle, "", &on) {
				p.toggleLyrics()
			}
		}
		queue := p.queueShown()
		if p.iconButton(c, "queue-toggle", "list-music", i18n.T("Queue"), false, "", &queue) {
			p.toggleQueueView()
		}
		if !p.volume() {
			return
		}
		icon, name := "volume-2", i18n.T("Mute")
		switch {
		case s.Volume == 0:
			icon, name = "volume-x", i18n.T("Unmute")
		case s.Volume < 0.5:
			icon = "volume-1"
		}
		if p.iconButton(c, "mute", icon, name, false, "", nil) {
			p.toggleMute()
		}
		v := s.Volume * 100
		sl := slider(c.Key("volume"), &v, 0, 100, theme.C("ink-muted"), false).Width(96).Label(i18n.T("Volume"))
		sl.Description(fmt.Sprintf("%d%%", int(v+0.5)))
		if sl.Changed() {
			p.setVolume(v / 100)
		}
	})
}

// slider is a slider in the design's look: a 4px track at 40% of the
// faint ink, the range in the given colour, a 12px ink thumb (shared/ui/
// slider.tsx).
func slider(c *ui.Context, v *float64, lo, hi float64, rangeColor ui.Color, disabled bool) ui.Element {
	const thumb = 12
	s := ui.SliderBase(c, v, lo, hi).Height(16).PaddingX(thumb / 2).Cursor(ui.CursorPointer)
	if disabled {
		s.Disabled(true).Opacity(0.5)
	}
	at := float32(0)
	if hi > lo {
		at = float32((*v - lo) / (hi - lo))
	}
	track, ink := theme.C("ink-faint").Alpha(0.4), theme.C("ink")
	s.Draw(func(p *ui.Painter, r ui.Rect) {
		x0, w := r.X+thumb/2, r.W-thumb
		y := r.Y + (r.H-4)/2
		p.Fill(ui.Rect{X: x0, Y: y, W: w, H: 4}, track, 2)
		if fill := w * min(max(at, 0), 1); fill > 0 {
			p.Fill(ui.Rect{X: x0, Y: y, W: fill, H: 4}, rangeColor, 2)
		}
		p.Fill(ui.Rect{X: x0 + w*min(max(at, 0), 1) - thumb/2, Y: r.Y + (r.H-thumb)/2, W: thumb, H: thumb}, ink, thumb/2)
	})
	return s.Grow(1)
}

// tag is a small bordered label (shared/components/Badges.tsx's Tag).
func tag(c *ui.Context, s, tip string) {
	ui.Row(c).Height(20).PaddingX(6).Shrink(0).AlignItems(ui.Center).Radius(4).Border(1, theme.C("control-line")).Tooltip(tip).Children(func() {
		ui.Text(c, s).FontSize(11).FontWeight(500).TextColor(theme.C("ink-muted")).SingleLine()
	})
}

// label is a single line of text in one of the theme's type styles.
func label(c *ui.Context, s, style string) ui.Element {
	t := theme.Current().Type[style]
	return ui.Text(c, s).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).
		LetterSpacing(t.Tracking).SingleLine().MinWidth(0).Shrink(1)
}

// clock is a time as m:ss, or h:mm:ss from an hour (v0.2.0's formatTime).
func clock(d time.Duration) string {
	total := int(max(d, 0) / time.Second)
	h, m, s := total/3600, total%3600/60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
