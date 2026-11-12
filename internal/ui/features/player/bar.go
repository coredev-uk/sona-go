package player

import (
	"math"
	"net/url"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
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
	p.announce.Announce(c, s)
	p.announce.AnnounceFailure(c, s)
	p.returnFocus(c)
	tk := theme.Current()
	backdrop := ""
	if s.Current != nil {
		backdrop = s.Current.Backdrop
	}
	ui.Column(c).Height(tk.Metric["player-bar"]).FillWidth().Background(theme.C("surface")).Label(i18n.T("Player")).Role(ui.RoleRegion).Children(func() {
		theme.Hairline(c)
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
	// The song keeps room for its title at the window's minimum width: the
	// transport shrinks towards its 280 first (v0.2.0's grid let it fall to
	// nothing once the right group grew).
	row := ui.Row(c.Key("song")).Grow(1).Basis(0).MinWidth(160).Gap(12).AlignItems(ui.Center)
	row.Children(func() {
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
	p.playingMenu(c, row, cur, &p.menus.bar)
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
	case p.videoPlaying():
		name = i18n.T("Show video")
	case open:
		name = i18n.T("Close now playing")
	}
	b := ui.ButtonBase(c.Key("cover")).Label(name).Shrink(0).Radius(theme.Current().Radius["sm"]).Padding(0).Bind(&p.coverBtn)
	b.Background(ui.Transparent)
	if cur == nil {
		b.Disabled(true)
	}
	b.Children(func() { kit.Artwork(c, p.env.Art, src, 48, kit.ArtOpts{}).Shrink(0) })
	// The button takes the pointer from the row: it opens the menu itself.
	p.playingRow(b, cur, &p.menus.bar)
	if b.Clicked() && cur != nil {
		if p.videoPlaying() && p.env.Video != nil {
			p.env.Video.OpenFrom(&p.coverBtn)
		} else if open {
			p.ui.closeOverlay()
		} else {
			p.opened(&p.coverBtn, func() { p.ui.openNowPlaying("") })
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
	if path == "" || !p.env.Routed(path) {
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
	// The link takes the pointer from the song around it: it opens the menu itself.
	p.playingRow(l, p.env.Play.State().Current, &p.menus.bar)
}

// subline is the line under the title: what needs the user, what failed,
// or the artist and album.
func (p *player) subline(c *ui.Context, s services.PlaybackState) {
	cur := s.Current
	switch {
	case cur == nil:
		label(c, i18n.T("Choose a song, album or playlist to start"), "sub").TextColor(theme.C("ink-faint"))
	case s.AuthRequired:
		msg := i18n.T("Sign in again to keep playing")
		if p.env.Surfaces == nil {
			label(c, msg, "sub").TextColor(theme.C("ink"))
			break
		}
		// A button that opens the account dialog: it looks as the plain line
		// does (ink, underlined, the line's own height), is a Tab stop and is
		// on the accessibility tree under the line's words.
		b := ui.ButtonBase(c.Key("sign-in")).Padding(0).MinWidth(0).Shrink(1).Background(ui.Transparent).
			Radius(theme.Current().Radius["sm"]).Cursor(ui.CursorPointer)
		b.Children(func() { label(c, msg, "sub").TextColor(theme.C("ink")).Underline() })
		if b.Clicked() {
			p.env.Surfaces.OpenAccount()
		}
	case s.NoCDM:
		p.env.PlayerNotice.Build(c) // features/cdm's notice
	case s.State == "error" && s.Error != "":
		// The line says what happened, the tooltip why (Go's error code, worded).
		// It is a status, and said once (TrackAnnouncer.AnnounceFailure).
		msg := kit.FailureLine(cur)
		ui.Row(c).Gap(6).AlignItems(ui.Center).MinWidth(0).Role(ui.RoleStatus).Label(msg).Tooltip(i18n.ErrorText(s.Error)).Children(func() {
			ui.Icon(c, theme.Icon("circle-alert")).FontSize(14).TextColor(theme.C("danger"))
			label(c, msg, "sub").TextColor(theme.C("ink"))
		})
	case cur.Artist == "" && cur.Album == "":
		// A broadcaster names no provider: its name stands alone.
	case cur.ArtistID != "" && p.env.Routed("/artist/"+url.PathEscape(cur.ArtistID)):
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
				p.tr.Prev()
			}

			icon, name := kit.PlayPauseIcon(s.State)
			if kit.Button(c.Key("play"), "", kit.ButtonOpts{Variant: kit.Play, Icon: icon, Spin: loading, Disabled: idle, A11yLabel: name, Keys: p.env.KeysOf("player.toggle")}).Clicked() {
				p.tr.Toggle()
			}
			if loading {
				// The spinner is only seen: this is what a screen reader finds of
				// it, beside the button (no size, so the row does not move).
				ui.Box(c.Key("loading")).Absolute().Size(0, 0).Role(ui.RoleStatus).Label(i18n.T("Loading"))
			}

			if p.iconButton(c, "next", "skip-forward", i18n.T("Next"), idle || (!s.Controls.Next && unavailable(s, ctlNext) == ""), unavailable(s, ctlNext), nil) {
				p.tr.Next()
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

// barShortcuts is the shortcut each of the bar's icon buttons stands for,
// whose keys its tooltip shows.
var barShortcuts = map[string]string{
	"shuffle": "player.shuffle", "prev": "player.previous", "next": "player.next", "repeat": "player.repeat",
	"mute": "player.mute", "queue-toggle": "player.queue", "lyrics": "player.lyrics", "video": "player.video",
}

// iconButton is a quiet icon button of the bar. disabled is for what can't
// be pressed now (nothing playing, the end of the queue); reason is for a
// control that doesn't apply to what plays (a live station): it is
// unavailable to assistive technology but stays enabled (focus reaches it,
// and the tooltip and the description say why) while a press does nothing.
// It reports a click.
func (p *player) iconButton(c *ui.Context, key, icon, name string, disabled bool, reason string, pressed *bool) bool {
	if disabled {
		reason = ""
	}
	btn := kit.Button(c.Key(key), "", kit.ButtonOpts{
		Variant: kit.Ghost, Size: kit.SizeSM, Icon: icon, Pressed: pressed, Disabled: disabled, A11yLabel: name, Reason: reason,
		Keys: p.env.KeysOf(barShortcuts[key]),
	})
	switch key {
	case "queue-toggle":
		btn.Bind(&p.queue.toggle) // the queue's Escape returns the focus here
	case "lyrics":
		btn.Bind(&p.lyricsBtn) // Now Playing's close returns it here
	case "video":
		btn.Bind(&p.videoBtn) // so does the video view's
	}
	return btn.Clicked() && reason == ""
}

// live is a live station's line in place of the seek bar: a marker, and
// what the station says is on air.
func (p *player) live(c *ui.Context, s services.PlaybackState) {
	ui.Row(c).Height(16).FillWidth().Gap(8).Justify(ui.Center).AlignItems(ui.Center).MinWidth(0).Children(func() {
		kit.LiveMarker(c, s.State == store.StatePlaying)
		if on := s.OnAir; on != nil {
			line := on.Title
			if on.Artist != "" {
				line = on.Artist + " – " + on.Title
			}
			label(c, line, "meta").TextColor(theme.C("ink-muted"))
		} else if ev := p.onAirProgramme(c, s.Current.ID); ev != nil {
			// What the station's schedule says is on air, while it names no song.
			label(c, programmeLine(ev), "meta").TextColor(theme.C("ink-muted"))
		}
	})
}

// seekBar is the elapsed time, the slider and the length. Dragging shows
// where it will go and seeks once, when the pointer lets go (or a key moved
// it).
func (p *player) seekBar(c *ui.Context, s services.PlaybackState, idle bool) {
	dur := time.Duration(s.DurationMs) * time.Millisecond
	pos := p.seek.Pos(p.env.Play.Position())
	if idle {
		pos = 0
	}
	can := !idle && dur > 0 && s.Controls.Seek
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
		elapsed := "–:––"
		if !idle {
			elapsed = i18n.FormatDuration(pos)
		}
		label(c, elapsed, "meta").Width(40).TextAlign(ui.End).TextColor(theme.C("ink-faint")).FontFeatures("tnum")

		v := pos.Seconds()
		sl := kit.Slider(c.Key("seek"), &v, 0, max(dur.Seconds(), 1), kit.SliderOpts{A11yLabel: i18n.T("Seek"), Disabled: !can, Range: theme.C("amber")}).Grow(1)
		told := pos // what assistive technology reads of the position
		if can && sl.Focused() && !p.seek.Active() {
			if !p.holding {
				p.holding, p.held = true, math.Floor(pos.Seconds())
			}
			told = time.Duration(p.held * float64(time.Second))
			sl.Range(0, max(dur.Seconds(), 1), p.held)
		} else {
			p.holding = false
		}
		if can {
			// v0.2.0's aria-valuetext, there only while the slider is on.
			sl.Description(i18n.T("{at} of {total}", "at", i18n.FormatDuration(told), "total", i18n.FormatDuration(dur)))
		}
		if to, ok := p.seek.Update(sl, v); ok {
			p.tr.SeekTo(to, p.now())
		}

		total := "–:––"
		if !idle {
			total = i18n.FormatDuration(dur)
		}
		label(c, total, "meta").Width(40).TextColor(theme.C("ink-faint")).FontFeatures("tnum")
	})
	p.seek.Tick(c, s.State == store.StatePlaying)
}

// volumeWidth is the volume slider's width, v0.2.0's `w-24` in the bar.
const volumeWidth = 96

// volumeControls is the right column: the sleep timer, lyrics, the queue,
// then mute and the volume slider, where Sona sets the playback volume.
func (p *player) volumeControls(c *ui.Context, s services.PlaybackState) {
	idle := s.Current == nil
	// The column never shrinks below its controls (v0.2.0's grid track was
	// minmax(max-content, 1fr)): the transport gives way first. The group's
	// own width, read from the last frame, is the column's minimum.
	var group ui.Element
	ui.Row(c).Grow(1).Basis(0).MinWidth(p.rightW).Justify(ui.End).AlignItems(ui.Center).Children(func() {
		group = ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(4)
		group.Children(func() {
			p.env.PlayerControls.BuildRange(c, 0, module.ControlsAfterQueue) // the other features' controls: private listening…
			p.sleepButton(c)
			if p.videoPlaying() {
				on := p.env.Video.IsOpen()
				if p.iconButton(c, "video", "clapperboard", i18n.T("Show video"), idle, "", &on) {
					p.env.Video.ToggleFrom(&p.videoBtn)
				}
			}
			if !p.videoPlaying() {
				on := p.ui.overlay == overlayNowPlaying && shownPane(p.ui.pane, s.Current) == paneLyrics
				if p.iconButton(c, "lyrics", "mic-vocal", i18n.T("Lyrics"), idle, "", &on) {
					p.opened(&p.lyricsBtn, p.toggleLyrics)
				}
			}
			queue := p.queueShown()
			if p.iconButton(c, "queue-toggle", "list-music", i18n.T("Queue"), false, "", &queue) {
				p.toggleQueueView()
			}
			p.env.PlayerControls.BuildRange(c, module.ControlsAfterQueue, 1<<30) // …and after the queue: Mini player, Equaliser
			if !p.volume() {
				return
			}
			icon, name := kit.VolumeIcon(s.Volume)
			if p.iconButton(c, "mute", icon, name, false, "", nil) {
				p.tr.ToggleMute()
			}
			v := s.Volume * 100
			// v0.2.0's w-24 shrink-0.
			sl := kit.Slider(c.Key("volume"), &v, 0, 100, kit.SliderOpts{A11yLabel: i18n.T("Volume"), Range: theme.C("ink-muted"), Width: volumeWidth})
			// No description: its range is 0 to 100, so a screen reader already
			// says the value and the percentage ("80, 80 percent"); "80%" again
			// would be a third time.
			if sl.Changed() {
				p.tr.SetVolume(v / 100)
			}
		})
	})
	if w := group.Bounds().W; w != p.rightW {
		p.rightW = w
		c.Invalidate()
	}
}

// tag is a small bordered label (shared/components/Badges.tsx's Tag).
func tag(c *ui.Context, s, tip string) {
	ui.Row(c).Height(20).PaddingX(6).Shrink(0).AlignItems(ui.Center).Radius(4).Border(1, theme.C("control-line")).Tooltip(tip).Children(func() {
		ui.Text(c, s).FontSize(11).FontWeight(500).TextColor(theme.C("ink-muted")).SingleLine()
	})
}

// label is a single line of text in one of the theme's type styles.
func label(c *ui.Context, s, style string) ui.Element {
	return theme.Text(c, s, style).SingleLine().MinWidth(0).Shrink(1)
}
