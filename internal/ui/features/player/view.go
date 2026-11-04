package player

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/lyricsync"
)

// overlayKind is the panel the player has open over the page (v0.2.0's
// ui.ts Overlay): the now-playing view or its full-screen lyrics ("room").
// The video view is the video module's own layer (Env.Video).
type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayNowPlaying
	overlayRoom
)

// paneKind is what the now-playing view shows beside the cover.
type paneKind string

const (
	paneLyrics paneKind = "lyrics"
	paneQueue  paneKind = "queue"
)

// view is the player's panels' state (v0.2.0's useUI store): the queue
// beside the page, the overlay and the pane the now-playing view shows.
// Main thread only.
type view struct {
	// queueOpen is the queue beside the page (while no overlay covers it).
	queueOpen bool
	overlay   overlayKind
	// pane is the now-playing view's pane, kept while it is closed so it
	// reopens where it was.
	pane paneKind
	// fromRoom: now playing was reopened by leaving full-screen lyrics, so
	// focus goes back to its Full screen button.
	fromRoom bool
	// opening is the bar control (its handle) whose click is opening a panel
	// now; opener the one that opened the panel showing, which the focus goes
	// back to when the panel closes (v0.2.0's useReturnFocus). nil: a key
	// opened it, so the focus is left with the page. restoring is the opener
	// a return was asked of and that is not answered yet.
	opening, opener, restoring *ui.Handle
}

func newView() *view { return &view{pane: paneLyrics} }

func (v *view) toggleQueue() { v.queueOpen = !v.queueOpen }

// openNowPlaying opens the view on pane, or on the one it showed last when
// pane is "".
func (v *view) openNowPlaying(pane paneKind) {
	if v.overlay == overlayNone {
		v.opener = v.opening
	}
	v.overlay, v.fromRoom = overlayNowPlaying, false
	if pane != "" {
		v.pane = pane
	}
}

// openRoom is full-screen lyrics.
func (v *view) openRoom() {
	if v.overlay == overlayNone {
		v.opener = v.opening
	}
	v.overlay, v.pane = overlayRoom, paneLyrics
}

// closeOverlay steps back one level: full-screen lyrics → now playing → the
// page.
func (v *view) closeOverlay() {
	v.fromRoom = v.overlay == overlayRoom
	if v.overlay == overlayRoom {
		v.overlay = overlayNowPlaying
		return
	}
	v.overlay = overlayNone
}

func (v *view) setPane(pane paneKind) { v.pane = pane }

// shownPane is the pane the view shows for pane and the playing item: a
// station, a music video or a song outside the catalogue has no lyrics, so
// it shows Up next.
func shownPane(pane paneKind, cur *services.QueueItem) paneKind {
	if lyricsync.SongID(cur) != "" && !cur.Live {
		return pane
	}
	return paneQueue
}

// queueShown reports whether the queue is on screen: the view's Up next
// pane while it is open, else the queue beside the page.
func (p *player) queueShown() bool {
	v := p.ui
	if v.overlay == overlayNowPlaying {
		return shownPane(v.pane, p.env.Play.State().Current) == paneQueue
	}
	return v.overlay == overlayNone && v.queueOpen
}

// toggleQueueView is the bar's Queue toggle and its shortcut: with the
// now-playing view open it turns to Up next, or closes the view when Up
// next is showing; otherwise it shows or hides the queue beside the page.
func (p *player) toggleQueueView() {
	v := p.ui
	switch {
	case v.overlay != overlayNowPlaying:
		v.toggleQueue()
	case shownPane(v.pane, p.env.Play.State().Current) == paneQueue:
		v.closeOverlay()
	default:
		v.setPane(paneQueue)
	}
}
