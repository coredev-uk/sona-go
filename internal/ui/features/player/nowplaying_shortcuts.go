package player

import (
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
	return []module.Shortcut{
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
			Label:    func() string { return i18n.T("Full-screen lyrics") },
			Keywords: func() string { return i18n.T("lyrics room karaoke big large tv") },
			When:     func() bool { return lyricsync.SongID(p.env.Play.State().Current) != "" },
			Run:      p.toggleRoom,
		},
	}
}
