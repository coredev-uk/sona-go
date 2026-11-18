package player

import (
	"time"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/lyricsync"
	"github.com/coredev-uk/sona-go/internal/ui/module"
)

// nowPlayingShortcuts are the keys for Now Playing, the lyrics and the video
// view, with v0.2.0's IDs and keys (features/player/shortcuts.ts).
func (p *player) nowPlayingShortcuts() []module.Shortcut {
	nudge := func(dir int) func() {
		return func() {
			if id := lyricsync.SongID(p.env.Play.State().Current); id != "" {
				lyricsync.Session().Nudge(id, dir)
			}
		}
	}
	list := []module.Shortcut{
		{
			ID: "player.lyrics", Group: "view", Keys: []string{"L"}, Icon: "mic-vocal",
			Label:    func() string { return i18n.T("Show or hide lyrics") },
			Keywords: func() string { return i18n.T("words sing") },
			When:     func() bool { return !p.videoPlaying() },
			Run:      p.toggleLyrics,
		},
		{
			ID: "player.now-playing", Group: "view", Keys: []string{"N"}, Icon: "disc-3",
			Label:    func() string { return i18n.T("Show or hide now playing") },
			Keywords: func() string { return i18n.T("cover artwork full") },
			Run:      p.toggleNowPlaying,
		},
		{
			ID: "player.video", Group: "view", Keys: []string{"V"}, Icon: "clapperboard",
			Label:    func() string { return i18n.T("Show or hide the video") },
			Keywords: func() string { return i18n.T("music video picture watch") },
			When:     p.videoPlaying,
			Run:      p.toggleVideo,
		},
		{
			ID: "player.close-overlay", Group: "view", Keys: []string{"Escape"}, Icon: "x", Scope: "now-playing", NoPalette: true,
			Label: func() string { return i18n.T("Close now playing") },
			When:  func() bool { return p.ui.overlay != overlayNone },
			Run:   func() { p.ui.closeOverlay() },
		},
		{
			ID: "player.lyrics-earlier", Group: "view", Keys: []string{"["}, Icon: "minus", Repeat: true,
			Label:    func() string { return i18n.T("Show lyrics half a second earlier") },
			Keywords: func() string { return i18n.T("lyrics timing offset sync delay") },
			When:     p.lyricsShown,
			Run:      nudge(-1),
		},
		{
			ID: "player.lyrics-later", Group: "view", Keys: []string{"]"}, Icon: "plus", Repeat: true,
			Label:    func() string { return i18n.T("Show lyrics half a second later") },
			Keywords: func() string { return i18n.T("lyrics timing offset sync delay") },
			When:     p.lyricsShown,
			Run:      nudge(1),
		},
		{
			ID: "player.lyrics-reset", Group: "view", Keys: []string{"\\"}, Icon: "timer-reset",
			Label:    func() string { return i18n.T("Put the lyrics back on their original timing") },
			Keywords: func() string { return i18n.T("lyrics timing offset sync reset") },
			When:     p.lyricsShown,
			Run: func() {
				if id := lyricsync.SongID(p.env.Play.State().Current); id != "" {
					lyricsync.Session().Reset(id)
				}
			},
		},
		{
			ID: "player.lyrics-room", Group: "view", Keys: []string{"Shift+L"}, Icon: "maximize-2",
			Label:    func() string { return i18n.T("Full-Screen Lyrics") },
			Keywords: func() string { return i18n.T("lyrics room karaoke big large tv") },
			When:     func() bool { return lyricsync.SongID(p.env.Play.State().Current) != "" },
			Run:      p.toggleRoom,
		},
	}
	return append(list, p.stageShortcuts()...)
}

// stageShortcuts are the keys of Full-Screen Lyrics, in a context of their
// own (they go to the view only while it is open; Space, Escape and Shift+L
// are the player's). Up and Down go to the previous or next line, which for
// lyrics with no timing moves the line being read.
func (p *player) stageShortcuts() []module.Shortcut {
	seek := func(d time.Duration) func() {
		return func() { p.seekBy(d) }
	}
	return []module.Shortcut{
		{
			ID: "player.lyrics-line-previous", Group: "playback", Keys: []string{"ArrowUp"}, Icon: "chevron-up", Scope: stageScope, NoPalette: true, Repeat: true,
			Label: func() string { return i18n.T("Full-Screen Lyrics: previous line") },
			When:  p.roomOpen,
			Run:   func() { p.stageStep(-1) },
		},
		{
			ID: "player.lyrics-line-next", Group: "playback", Keys: []string{"ArrowDown"}, Icon: "chevron-down", Scope: stageScope, NoPalette: true, Repeat: true,
			Label: func() string { return i18n.T("Full-Screen Lyrics: next line") },
			When:  p.roomOpen,
			Run:   func() { p.stageStep(1) },
		},
		{
			ID: "player.lyrics-seek-back", Group: "playback", Keys: []string{"ArrowLeft"}, Icon: "rewind", Scope: stageScope, NoPalette: true, Repeat: true,
			Label:       func() string { return i18n.T("Full-Screen Lyrics: back 5 seconds") },
			When:        p.roomOpen,
			Unavailable: func() string { return unavailable(p.env.Play.State(), ctlSeek) },
			Run:         seek(-stageSeek),
		},
		{
			ID: "player.lyrics-seek-forward", Group: "playback", Keys: []string{"ArrowRight"}, Icon: "fast-forward", Scope: stageScope, NoPalette: true, Repeat: true,
			Label:       func() string { return i18n.T("Full-Screen Lyrics: ahead 5 seconds") },
			When:        p.roomOpen,
			Unavailable: func() string { return unavailable(p.env.Play.State(), ctlSeek) },
			Run:         seek(stageSeek),
		},
		{
			ID: "player.lyrics-start", Group: "playback", Keys: []string{"Home"}, Icon: "skip-back", Scope: stageScope, NoPalette: true,
			Label:       func() string { return i18n.T("Full-Screen Lyrics: start of the song") },
			When:        p.roomOpen,
			Unavailable: func() string { return unavailable(p.env.Play.State(), ctlSeek) },
			Run:         func() { p.tr.SeekTo(0, p.now()) },
		},
	}
}
