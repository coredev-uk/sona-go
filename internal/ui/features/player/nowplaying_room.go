package player

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/lyricsync"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// roomOpen reports full-screen lyrics showing: the shell's Layer is open, and
// the window behind it inert.
func (p *player) roomOpen() bool {
	return p.ui.overlay == overlayRoom && lyricsync.SongID(p.env.Play.State().Current) != ""
}

// roomLayer is RoomLyrics for the shell's Layer slot: the whole window given
// to the words at room size, the sung line in amber, the song and the timing
// nudge in a quiet strip at the foot. Escape returns to Now Playing. The
// window goes full screen while it is open (and back when it closes, if it
// was not already).
func (p *player) roomLayer(c *ui.Context) {
	cur := p.env.Play.State().Current
	songID := lyricsync.SongID(cur)
	if p.ui.overlay == overlayRoom && songID == "" {
		// Nothing to read once the queue is cleared or the song has no lyrics.
		p.ui.closeOverlay()
		c.Invalidate()
	}
	open := p.roomOpen()
	p.roomFullscreen(open)
	if !open {
		p.np.roomShown = false
		return
	}
	first := !p.np.roomShown
	p.np.roomShown = true
	if first || p.np.room == nil || p.np.room.songID != songID {
		p.np.room = newLyricsSheet(songID)
	}
	w, h := c.Size()

	// The wash is the root itself, so the lines draw over it (an Absolute
	// sibling would paint above them).
	root := kit.Backdrop(c.Key("room"), p.env.Art, kit.BackdropOpts{Src: cur.Backdrop, Tone: kit.ToneGround}, func() {
		p.roomLines(c, songID, roomLook(w, h))
		ui.Row(c.Key("room-footer")).FillWidth().Shrink(0).Gap(16).Padding(12, 0.07*w, 28).AlignItems(ui.Center).Children(func() {
			kit.Artwork(c.Key("art"), p.env.Art, cur.Art.Small, 44, kit.ArtOpts{})
			ui.Column(c.Key("song")).Grow(1).MinWidth(0).Children(func() {
				lineText(c.Key("name"), cur.Name, 14, 20, 500, theme.C("ink")).SingleLine()
				lineText(c.Key("artist"), cur.Artist, 13, 18, 400, theme.C("ink-muted")).SingleLine()
			})
			p.lyricsTiming(c, songID)
			if kit.Button(c.Key("exit"), i18n.T("Exit full screen"), kit.ButtonOpts{
				Variant: kit.Ghost, Size: kit.SizeSM, Icon: "minimize-2", Tip: i18n.T("Exit full screen"), Keys: p.env.KeysOf("player.close-overlay"),
			}).
				Bind(&p.np.roomExit).Clicked() {
				p.ui.closeOverlay()
			}
		})
	}).FillWidth().FillHeight().Grow(1).Basis(0).MinHeight(0)
	root.Background(theme.C("ground")).Role(ui.RoleGroup).Label(i18n.T("Lyrics, full screen"))
	if !theme.ReducedMotion() {
		root.Transition(ui.ElementTransition{Enter: &ui.Motion{}, Duration: 200 * time.Millisecond})
	}
	if first {
		p.np.roomExit.Focus(c)
	}
}

// roomFullscreen puts the window in full screen while the lyrics are open,
// and back on leaving if the window was not full screen before.
func (p *player) roomFullscreen(open bool) {
	w := p.env.Window
	if w == nil {
		return
	}
	switch {
	case open && !p.np.roomEntered:
		p.np.roomEntered = true
		if !w.FullScreen() {
			w.SetFullScreen(true)
			p.np.roomFull = true
		}
	case !open && p.np.roomEntered:
		p.np.roomEntered = false
		if p.np.roomFull {
			p.np.roomFull = false
			w.SetFullScreen(false)
		}
	}
}

// roomLines is the room's sheet, or its stand-in while the lyrics load, failed
// or do not exist.
func (p *player) roomLines(c *ui.Context, songID string, look lyricLook) {
	r := p.lyricsFor(songID)
	switch {
	case !r.HasData && r.Err == nil:
		ui.Box(c.Key("loading")).Grow(1).Role(ui.RoleStatus).Label(i18n.T("Loading lyrics"))
	case lyricsMissing(r):
		ui.Row(c.Key("none")).Grow(1).Basis(0).Justify(ui.Center).AlignItems(ui.Center).Padding(0, look.sidePad).Children(func() {
			lineText(c, i18n.T("No lyrics for this song."), look.size, look.lineH, look.weight, theme.C("ink-muted")).MaxWidth(look.columnMax)
		})
	case !r.HasData:
		ui.Row(c.Key("error")).Grow(1).Basis(0).Justify(ui.Center).AlignItems(ui.Center).Padding(0, look.sidePad).Children(func() {
			kit.ErrorBlock(c, i18n.T("Couldn't load the lyrics"), r.Err, kit.ErrorBlockOpts{
				OnRetry: func() { p.retryLyrics(songID) }, Retrying: r.Fetching,
			}).MaxWidth(576)
		})
	default:
		// The sheet's fades are siblings of its scroller: a box of its own keeps
		// them at the sheet's edges, above the footer.
		ui.Column(c.Key("sheet-box")).Grow(1).Basis(0).MinHeight(0).FillWidth().FillHeight().Children(func() {
			p.lyricsScroller(c, p.np.room, &r.Data, sheetIn{look: look, focusable: true})
		})
	}
}
