package player

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/lyricsync"
	"github.com/coredev-uk/sona-go/internal/ui/module"
)

// nowPlayingState is Now Playing's and the full-screen lyrics' own state (the
// panels' shared state, which the bar and the queue read too, is view.go's).
// The zero value works. Main thread only.
type nowPlayingState struct {
	// prefs are the Translation and Pronunciation choices (the settings', set
	// up on first use unless prefsInit says a test did).
	prefs     lyricsPrefs
	prefsInit bool
	// lyrics and motion stand in for the services (a test's); nil: the core's.
	lyrics lyricsSource
	motion motionSource

	// sheet is the lyrics pane's sheet and room the full-screen one; each
	// belongs to a song and starts again with the next.
	sheet, room *lyricsSheet

	// shown: the view has been built since it opened (focus goes to its first
	// stop once); roomShown is the same for full-screen lyrics.
	shown, roomShown bool
	// hadFocus: the focus was inside the view when it was last built, so
	// closing it returns the focus to the control that opened it.
	hadFocus bool
	// roomFull: full-screen lyrics put the window in full screen, so leaving
	// takes it out again.
	roomFull, roomEntered bool

	// queueView builds Up next's body: the queue list, then its foot strip (the
	// summary, which calls leave when the user leaves the view, and the queue
	// tools). The queue code sets it; nil (no queue panel) leaves the pane empty.
	queueView func(c *ui.Context, leave func())
	// rating builds Favourite and Suggest Less for the playing song; nil: none.
	rating func(c *ui.Context, cur *services.QueueItem)

	root, closeBtn, roomToggle, roomExit ui.Handle
	tabs                                 [2]ui.Handle
}

// nowPlayingLayout is how the view is laid out in its box.
type nowPlayingLayout struct {
	stacked bool
	cover   float32
}

func itoa(i int) string { return strconv.Itoa(i) }

// prefsFor returns the lyrics view's choices, set up on first use.
func (p *player) prefsFor() *lyricsPrefs {
	if !p.np.prefsInit {
		p.np.prefsInit = true
		p.np.prefs.read, p.np.prefs.write = p.settingsPrefs()
	}
	return &p.np.prefs
}

// overlay is the Now Playing view for the shell's Overlay slot: open while it
// is the shown panel (full-screen lyrics are the Layer's).
func (p *player) overlaySlot() *module.Overlay {
	return &module.Overlay{
		Open: func() bool {
			open := p.ui.overlay == overlayNowPlaying
			if !open {
				p.np.shown = false
			}
			return open
		},
		Build: p.nowPlaying,
	}
}

// toggleLyrics is the bar's Lyrics button and the L key: Now Playing opens on
// Lyrics, or closes when Lyrics is already showing.
func (p *player) toggleLyrics() {
	if p.ui.overlay == overlayNowPlaying && p.ui.pane == paneLyrics {
		p.ui.closeOverlay()
		return
	}
	if p.env.Play.State().Current != nil {
		p.ui.openNowPlaying(paneLyrics)
	}
}

// toggleNowPlaying is the N key: Now Playing opens on its last pane, or
// closes.
func (p *player) toggleNowPlaying() {
	if p.ui.overlay == overlayNowPlaying {
		p.ui.closeOverlay()
		return
	}
	if p.env.Play.State().Current != nil {
		p.ui.openNowPlaying("")
	}
}

// toggleVideo is the V key: the music video view, or hides it (the bar's
// button opens it from itself, so the focus returns to it).
func (p *player) toggleVideo() { p.env.Video.Toggle() }

// toggleRoom is Shift+L: full-screen lyrics, or back out of them.
func (p *player) toggleRoom() {
	if p.ui.overlay == overlayRoom {
		p.ui.closeOverlay()
		return
	}
	p.ui.openRoom()
}

// videoPlaying reports a music video as the playing item.
func (p *player) videoPlaying() bool {
	cur := p.env.Play.State().Current
	return cur != nil && cur.Video
}

// lyricsSurface reports whether the bar offers Lyrics: always, as the view
// shows Up next for what has none (a music video has its video button instead).
func (p *player) lyricsSurface() bool { return !p.videoSurface() }

// videoSurface reports whether the bar offers the video: a music video plays.
func (p *player) videoSurface() bool { return p.videoPlaying() }

// lyricsShown reports lyrics on screen: Now Playing's lyrics pane or
// full-screen lyrics, for a song that has some.
func (p *player) lyricsShown() bool {
	if lyricsync.SongID(p.env.Play.State().Current) == "" {
		return false
	}
	switch p.ui.overlay {
	case overlayRoom:
		return true
	case overlayNowPlaying:
		return p.ui.pane == paneLyrics
	}
	return false
}

// nowPlayingSurface reports whether the Now Playing view exists: the bar's
// cover opens it.
func (p *player) nowPlayingSurface() bool { return true }
