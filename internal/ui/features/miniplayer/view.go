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
	// pad is the window's inner margin and coverSize the cover's side: the
	// window is 132 DIPs high, so the cover takes what the margins leave.
	pad       = 12
	coverSize = 108
	// timeW is the width of the time left, so the seek bar does not move as it
	// counts down.
	timeW = 40
)

// content is the mini player window's whole content (Slots.Mini): the cover
// beside the song, the line being sung and the transport, over the cover's
// wash. The background is the window's handle: dragging it moves the window;
// the controls take their own presses.
//
//	[cover]  Title                              [Open Sona][x]
//	         Artist
//	         ♪ the line being sung
//	         ( prev PLAY next )  ───o───────  −2:31
func (m *mini) content(c *ui.Context) {
	theme.Apply(c)
	s := m.env.Play.State()
	m.announce.Announce(c, s)
	cur := s.Current
	backdrop := ""
	if cur != nil {
		backdrop = cur.Backdrop
	}
	kit.Backdrop(c.Key("mini"), m.env.Art, kit.BackdropOpts{Src: backdrop}, func() {
		ui.Row(c.Key("body")).Fill().Padding(pad).Gap(14).AlignItems(ui.Center).Children(func() {
			src := ""
			if cur != nil {
				src = cur.Art.Medium
			}
			kit.Artwork(c.Key("cover"), m.env.Art, src, coverSize, kit.ArtOpts{}).Shrink(0)
			ui.Column(c.Key("col")).Grow(1).Basis(0).MinWidth(0).FillHeight().Children(func() {
				m.head(c, s)
				m.third(c, s)
				m.controls(c, s)
			})
		})
	}).Fill().Background(theme.C("surface")).DragWindow().Role(ui.RoleGroup).Label(i18n.T("Sona mini player"))
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
		ui.Column(c.Key("song")).Grow(1).Basis(0).MinWidth(0).Children(func() {
			if cur == nil {
				text(c.Key("title"), i18n.T("Not playing"), "body").FontWeight(600).TextColor(theme.C("ink-muted"))
				// translators: Line of the small player when nothing is playing.
				text(c.Key("artist"), i18n.T("Choose music in the main window"), "sub").TextColor(theme.C("ink-faint"))
				return
			}
			text(c.Key("title"), cur.Name, "body").FontWeight(600).TextColor(theme.C("ink")).Tooltip(cur.Name)
			text(c.Key("artist"), cur.Artist, "sub").TextColor(theme.C("ink-muted")).Tooltip(cur.Artist)
		})
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

// controls is the transport and the seek bar (a live station has a live
// marker instead).
func (m *mini) controls(c *ui.Context, s services.PlaybackState) {
	idle := s.Current == nil
	playing := s.State == store.StatePlaying
	loading := s.State == store.StateLoading
	ui.Row(c.Key("controls")).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Row(c.Key("transport")).Gap(4).AlignItems(ui.Center).Shrink(0).Children(func() {
			prev := kit.ControlUnavailable(s, s.Controls.Previous)
			if m.stepButton(c, "prev", "skip-back", i18n.T("Previous"), idle || (!s.Controls.Previous && prev == ""), prev) {
				m.previousCmd()
			}

			icon, name := "play", i18n.T("Play")
			switch {
			case loading:
				icon = "loader-circle"
			case playing:
				icon, name = "pause", i18n.T("Pause")
			}
			if kit.Button(c.Key("play"), "", kit.ButtonOpts{Variant: kit.Play, Size: kit.SizeSM, Icon: icon, Disabled: idle, A11yLabel: name}).Clicked() {
				m.toggleCmd()
			}

			next := kit.ControlUnavailable(s, s.Controls.Next)
			if m.stepButton(c, "next", "skip-forward", i18n.T("Next"), idle || (!s.Controls.Next && next == ""), next) {
				m.nextCmd()
			}
		})
		if s.Current != nil && s.Current.Live {
			m.live(c, playing)
			return
		}
		m.seekBar(c, s, idle)
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

// live is a live station's marker in place of the seek bar.
func (m *mini) live(c *ui.Context, playing bool) {
	ui.Row(c.Key("live")).Gap(6).AlignItems(ui.Center).Children(func() {
		dot := theme.C("ink-faint")
		if playing {
			dot = theme.C("play")
		}
		ui.Box(c.Key("dot")).Size(6, 6).Radius(3).Background(dot).Shrink(0)
		text(c.Key("word"), i18n.T("Live"), "meta").FontWeight(500).TextColor(theme.C("ink"))
	})
}

// seekBar is the slider and the time left. Dragging shows where it will go
// and seeks once, when the pointer lets go (or a key moved it): the only part
// that ticks.
func (m *mini) seekBar(c *ui.Context, s services.PlaybackState, idle bool) {
	dur := time.Duration(s.DurationMs) * time.Millisecond
	pos := m.env.Play.Position()
	if m.seek.active {
		pos = time.Duration(m.seek.secs * float64(time.Second))
	}
	if idle {
		pos = 0
	}
	pos = min(pos, max(dur, 0))
	can := !idle && dur > 0 && s.Controls.Seek
	ui.Row(c.Key("seek-row")).Grow(1).Basis(0).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
		v := pos.Seconds()
		o := kit.SliderOpts{A11yLabel: i18n.T("Seek"), Disabled: !can, Step: 1, Range: theme.C("amber")}
		if can {
			o.ValueText = i18n.T("{at} of {total}", "at", i18n.FormatDuration(pos), "total", i18n.FormatDuration(dur))
		}
		sl := kit.Slider(c.Key("seek"), &v, 0, max(dur.Seconds(), 1), o)
		if sl.Changed() {
			m.seek.active, m.seek.secs = true, v
		}
		if kit.SliderCommitted(sl) && m.seek.active {
			m.seek.active = false
			m.seekTo(time.Duration(m.seek.secs * float64(time.Second)))
		}
		left := "–:––"
		if can || (!idle && dur > 0) {
			left = "−" + i18n.FormatDuration(max(dur-pos, 0))
		}
		text(c.Key("left"), left, "meta").Width(timeW).Shrink(0).TextAlign(ui.End).TextColor(theme.C("ink-faint")).FontFeatures("tnum")
	})
	if s.State == store.StatePlaying && !m.seek.active {
		c.After(250 * time.Millisecond) // the position ticks (v0.2.0's useLivePosition)
	}
}

// text is a single line of text in one of the theme's type styles.
func text(c *ui.Context, s, style string) ui.Element {
	t := theme.Current().Type[style]
	return ui.Text(c, s).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).
		LetterSpacing(t.Tracking).SingleLine().MinWidth(0).Shrink(1)
}

// quiet is text of the line under the song's size.
func quiet(c *ui.Context, s string) ui.Element { return text(c, s, "sub").Tooltip(s) }

func msDur(ms int64) time.Duration { return time.Duration(ms) * time.Millisecond }
