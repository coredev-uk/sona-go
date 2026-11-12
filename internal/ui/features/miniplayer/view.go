package miniplayer

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
	// pad is the margin of the text block from the window's other edges, and
	// coverSize the cover's side: the cover takes the window's whole height
	// (132 DIPs) and its leading edge, with no margin.
	pad       = 12
	coverSize = 132
	// hitH is the height of the seek line's hit area at the window's foot;
	// the line itself is 2px (4 while hovered or focused).
	hitH = 12
)

// content is the mini player window's whole content (Slots.Mini): the cover
// fills the window's height at the leading edge, the song, the line being
// sung and the transport sit beside it over the cover's wash, and the
// progress is a hairline along the whole foot. The background is the
// window's handle: dragging it moves the window; the controls take their own
// presses.
//
//	[cover | Title                         [Open Sona][x]
//	[cover | Artist
//	[cover | ♪ the line being sung
//	[cover | −2:31                    ( prev PLAY next )
//	━━━━━━━━━━━━━━━━o─────────────────────────────────────
func (m *mini) content(c *ui.Context) {
	theme.Apply(c)
	s := m.env.Play.State()
	m.announce.Announce(c, s)
	cur := s.Current
	backdrop := ""
	if cur != nil {
		backdrop = cur.Backdrop
	}
	root := kit.Backdrop(c.Key("mini"), m.env.Art, kit.BackdropOpts{Src: backdrop}, func() {
		ui.Row(c.Key("body")).Fill().AlignItems(ui.Stretch).Children(func() {
			src := ""
			if cur != nil {
				src = cur.Art.Medium
			}
			// A radius that rounds nothing: the cover meets the window's
			// corners, which are the window's to round.
			cover := kit.Artwork(c.Key("cover"), m.env.Art, src, coverSize, kit.ArtOpts{Radius: 0.01}).Shrink(0)
			m.playingRow(cover, cur)
			ui.Column(c.Key("col")).Grow(1).Basis(0).MinWidth(0).FillHeight().Padding(pad, pad, pad, pad+2).Children(func() {
				m.head(c, s)
				m.third(c, s)
				m.controls(c, s)
			})
			m.menus.PopupFor(c, cur != nil)
		})
		m.seekLine(c, s)
	}).Fill().Background(theme.C("surface")).DragWindow().Role(ui.RoleGroup).Label(i18n.T("Sona mini player"))
	m.playingKeys(root, cur)
}

// leading reports the window's own buttons on the start side: macOS's inset
// title bar, or a desktop that puts Close first.
func (m *mini) leading() bool {
	r := store.UseCapabilities(m.env.Queries, m.env.Core)
	if !r.HasData {
		return false
	}
	w := r.Data.WindowControls
	if w.Native == "inset" {
		return true
	}
	for _, b := range w.Start {
		if b == "close" {
			return true
		}
	}
	return false
}

// head is the song's title and artist, and the window's own buttons, Close
// outermost on the side the platform puts it.
func (m *mini) head(c *ui.Context, s services.PlaybackState) {
	cur := s.Current
	leading := m.leading()
	ui.Row(c.Key("head")).FillWidth().Gap(8).AlignItems(ui.Start).Children(func() {
		if leading {
			m.chrome(c, leading)
		}
		song := ui.Column(c.Key("song")).Grow(1).Basis(0).MinWidth(0)
		song.Children(func() {
			if cur == nil {
				text(c.Key("title"), i18n.T("Not playing"), "heading").TextColor(theme.C("ink-muted"))
				// translators: Line of the small player when nothing is playing.
				text(c.Key("artist"), i18n.T("Choose music in the main window"), "sub").TextColor(theme.C("ink-faint"))
				return
			}
			text(c.Key("title"), cur.Name, "heading").TextColor(theme.C("ink")).Tooltip(cur.Name)
			text(c.Key("artist"), cur.Artist, "sub").TextColor(theme.C("ink-muted")).Tooltip(cur.Artist)
		})
		m.playingRow(song, cur)
		if !leading {
			m.chrome(c, leading)
		}
	})
}

// chrome is the window's buttons: Open Sona and Close.
func (m *mini) chrome(c *ui.Context, leading bool) {
	ui.Row(c.Key("window")).Gap(2).Shrink(0).Margin(-2, 0, 0, 0).AlignItems(ui.Center).Role(ui.RoleGroup).Label(i18n.T("Window")).Children(func() {
		open := func() {
			if kit.Button(c.Key("expand"), "", kit.ButtonOpts{
				// translators: Button of the small player: brings the main window back.
				Variant: kit.Ghost, Size: kit.SizeXS, Icon: "app-window", Muted: true, A11yLabel: i18n.T("Open Sona"),
			}).Clicked() {
				m.expand()
			}
		}
		closeBtn := func() {
			if kit.Button(c.Key("close"), "", kit.ButtonOpts{
				// translators: Button of the small player: closes it. Sentence case, unlike the tray's "Close Mini Player".
				Variant: kit.Ghost, Size: kit.SizeXS, Icon: "x", Muted: true, A11yLabel: i18n.T("Close mini player"),
			}).Clicked() {
				m.close()
			}
		}
		if leading {
			closeBtn()
			open()
			return
		}
		open()
		closeBtn()
	})
}

// third is the line under the song, which takes the room left: a failed
// command's message for a few seconds, else the line being sung, else what
// needs the user or what plays (the album, Live radio…).
func (m *mini) third(c *ui.Context, s services.PlaybackState) {
	cur := s.Current
	// A music video's picture shows in the main window's video view, not here.
	line := ""
	if cur != nil {
		line = cur.Album
		if cur.Video {
			line = i18n.T("Music video")
		}
	}
	switch {
	case s.AuthRequired:
		// translators: Line under the title of the small player when playback stopped because Apple Music needs a new sign-in.
		line = i18n.T("Sign in again in Sona to keep playing")
	case s.NoCDM:
		// translators: Line under the title of the small player while the Widevine decryption module (needed to play Apple Music) downloads.
		line = i18n.T("Waiting for Widevine: see Settings → Playback in Sona")
	case s.State == store.StateError && s.Error != "" && cur != nil && cur.Video:
		line = i18n.T("Couldn't play this video.")
	case s.State == store.StateError && s.Error != "":
		line = i18n.T("Couldn't play this song. Press play to try again.")
	case cur != nil && cur.Live:
		line = i18n.T("Live radio")
	}
	ui.Column(c.Key("third")).FillWidth().Grow(1).Basis(0).MinHeight(18).Margin(2, 0, 0, 0).Children(func() {
		switch msg := m.noticeText(c); {
		case msg != "":
			ui.Row(c.Key("notice")).FillWidth().Gap(6).AlignItems(ui.Center).Tooltip(msg).Children(func() {
				ui.Icon(c.Key("alert"), theme.Icon("circle-alert")).FontSize(14).TextColor(theme.C("danger")).Shrink(0)
				quiet(c.Key("msg"), msg).TextColor(theme.C("ink"))
			})
		case cur != nil && !cur.Live && !s.AuthRequired && !s.NoCDM && s.State != store.StateError:
			m.sungLine(c, cur, s.State == store.StatePlaying, line)
		default:
			quiet(c.Key("line"), line).TextColor(theme.C("ink-faint"))
		}
	})
}

// controls is the foot of the text block: the time left (a live station has
// its live marker) at the start, the transport at the trailing side.
func (m *mini) controls(c *ui.Context, s services.PlaybackState) {
	idle := s.Current == nil
	playing := s.State == store.StatePlaying
	loading := s.State == store.StateLoading
	ui.Row(c.Key("controls")).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		if s.Current != nil && s.Current.Live {
			kit.LiveMarker(c.Key("live"), playing).Shrink(0)
		} else {
			m.timeLeft(c, s, idle)
		}
		ui.Box(c.Key("gap")).Grow(1).Basis(0)
		ui.Row(c.Key("transport")).Gap(4).AlignItems(ui.Center).Shrink(0).Children(func() {
			prev := kit.ControlUnavailable(s, s.Controls.Previous)
			if m.stepButton(c, "prev", "skip-back", i18n.T("Previous"), idle || (!s.Controls.Previous && prev == ""), prev) {
				m.tr.Prev()
			}

			icon, name := kit.PlayPauseIcon(s.State)
			if kit.Button(c.Key("play"), "", kit.ButtonOpts{Variant: kit.Play, Size: kit.SizeSM, Icon: icon, Spin: loading, Disabled: idle, A11yLabel: name}).Clicked() {
				m.tr.Toggle()
			}

			next := kit.ControlUnavailable(s, s.Controls.Next)
			if m.stepButton(c, "next", "skip-forward", i18n.T("Next"), idle || (!s.Controls.Next && next == ""), next) {
				m.tr.Next()
			}
		})
	})
}

// stepButton is Previous or Next. disabled is for what can't be pressed now
// (nothing playing, the end of the queue); reason is for a control that
// doesn't apply to what plays (a live station): it stays enabled, keeps its
// place in the Tab order and says why as its description, and the press does
// nothing.
func (m *mini) stepButton(c *ui.Context, key, icon, name string, disabled bool, reason string) bool {
	if disabled {
		reason = ""
	}
	btn := kit.Button(c.Key(key), "", kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Icon: icon, Disabled: disabled, A11yLabel: name, Reason: reason})
	return btn.Clicked() && reason == ""
}

// position is where the song is: the dragged position while the user drags
// the seek line, else the mirror's, kept inside the song.
func (m *mini) position(s services.PlaybackState, idle bool) (pos, dur time.Duration) {
	dur = time.Duration(s.DurationMs) * time.Millisecond
	pos = m.seek.Pos(m.env.Play.Position())
	if idle {
		pos = 0
	}
	return min(pos, max(dur, 0)), dur
}

// timeLeft is the time left of the song in the meta style, tabular so the
// figures do not shift as it counts down.
func (m *mini) timeLeft(c *ui.Context, s services.PlaybackState, idle bool) {
	pos, dur := m.position(s, idle)
	left := "–:––"
	if !idle && dur > 0 {
		left = "−" + i18n.FormatDuration(max(dur-pos, 0))
	}
	text(c.Key("left"), left, "meta").Shrink(0).TextColor(theme.C("ink-muted")).FontFeatures("tnum")
}

// seekLine is the progress and the seek bar in one: a hairline along the
// window's whole foot, over the cover too, drawn by a slider whose hit area
// is hitH high. Dragging shows where it will go and seeks once, when the
// pointer lets go (or a key moved it). It grows to 4px under the pointer and
// while focused, which also draws its thumb. A live station has no seek bar
// (its marker is in the controls); the line is then not drawn. It is the
// only part that ticks.
func (m *mini) seekLine(c *ui.Context, s services.PlaybackState) {
	if s.Current != nil && s.Current.Live {
		return
	}
	idle := s.Current == nil
	pos, dur := m.position(s, idle)
	can := !idle && dur > 0 && s.Controls.Seek
	v := pos.Seconds()
	hi := max(dur.Seconds(), 1)
	sl := ui.SliderBase(c.Key("seek"), &v, 0, hi).Step(1).FocusRing(false).Cursor(ui.CursorPointer).
		Absolute().Left(0).Right(0).Bottom(0).Height(hitH).Label(i18n.T("Seek"))
	if can {
		sl.Description(i18n.T("{at} of {total}", "at", i18n.FormatDuration(pos), "total", i18n.FormatDuration(dur)))
	} else {
		sl.Disabled(true)
	}
	if to, ok := m.seek.Update(sl, v); ok {
		m.tr.SeekTo(to, m.now())
	}
	at := float32(0)
	if !idle && dur > 0 {
		at = float32(min(max(v/hi, 0), 1))
	}
	track, rng, ink, ring := theme.C("ink").Alpha(0.12), theme.C("amber"), theme.C("ink"), theme.C("ring")
	rtl := c.Direction() == ui.RightToLeft
	focus := sl.FocusVisible()
	_, _, held := sl.Dragged()
	grown := can && (focus || held || sl.Hovered())
	sl.Draw(func(p *ui.Painter, r ui.Rect) {
		h := float32(2)
		if grown {
			h = 4
		}
		y := r.Y + r.H - h
		p.Fill(ui.Rect{X: r.X, Y: y, W: r.W, H: h}, track, 0)
		edge := r.X + r.W*at // where the filled part ends
		if rtl {
			edge = r.X + r.W*(1-at)
		}
		if at > 0 {
			fill := ui.Rect{X: r.X, Y: y, W: edge - r.X, H: h}
			if rtl {
				fill = ui.Rect{X: edge, Y: y, W: r.X + r.W - edge, H: h}
			}
			p.Fill(fill, rng, 0)
		}
		if grown {
			const d = 10
			k := ui.Rect{X: min(max(edge-d/2, r.X), r.X+r.W-d), Y: y + h/2 - d/2, W: d, H: d}
			if focus {
				p.Fill(ui.Rect{X: k.X - 3, Y: k.Y - 3, W: d + 6, H: d + 6}, ring, d/2+3)
			}
			p.Fill(k, ink, d/2)
		}
	})
	m.seek.Tick(c, s.State == store.StatePlaying)
}

// text is a single line of text in one of the theme's type styles.
func text(c *ui.Context, s, style string) ui.Element {
	return theme.Text(c, s, style).SingleLine().MinWidth(0).Shrink(1)
}

// quiet is text of the line under the song's size.
func quiet(c *ui.Context, s string) ui.Element { return text(c, s, "sub").Tooltip(s) }
