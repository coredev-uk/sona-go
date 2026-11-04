// Package miniplayer is the mini player: a small always-on-top window with
// the cover, the song, the line being sung, the transport and a seek bar,
// for keeping music in reach while the main window is out of the way
// (v0.2.0's features/miniplayer). The shell owns the window (Slots.Mini is
// its whole content); this package builds that content, the player bar's
// Mini player button (Env.PlayerControls) and the shortcut that opens it.
//
// The mini player mirrors store.Playback like the player bar and sends the
// same commands through services.PlaybackService off the main thread. It has
// no room for toasts, so a command that fails is said on the line under the
// song for a few seconds, and announced once.
package miniplayer

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/lyricsync"
	"github.com/coredev-uk/sona-go/internal/ui/module"
)

// controlOrder is the Mini player button's place among the player bar's
// other modules' controls (private listening is 10).
const controlOrder = 30

// transport is what the mini player asks of the playback service
// (*services.PlaybackService).
type transport interface {
	Toggle(ctx context.Context) error
	Next(ctx context.Context) error
	Prev(ctx context.Context) error
	Seek(ctx context.Context, positionMs int64) error
}

// desktop is what it asks of the desktop service
// (*services.DesktopService): the windows.
type desktop interface {
	ShowMainWindow()
	CloseMiniPlayer()
	ToggleMiniPlayer()
}

// lyricsSource is what the sung line asks of the lyrics service.
type lyricsSource interface {
	Lyrics(ctx context.Context, songID string) (services.Lyrics, error)
}

// mini is the feature's state.
type mini struct {
	env  *module.Env
	svc  transport
	desk desktop
	lyr  lyricsSource
	now  func() time.Time
	sung sungState

	// seek is the seek bar while the user drags it: the position shown,
	// committed when the pointer lets go.
	seek struct {
		active bool
		secs   float64
	}
	announce kit.TrackAnnouncer
	notice   *notice
	noticeID int
}

// New is the mini player's module.
func New(env *module.Env) module.Module {
	m := newMini(env)
	if env.Core != nil {
		m.svc = services.Service[services.PlaybackService](env.Core)
		m.desk = services.Service[services.DesktopService](env.Core)
		m.lyr = services.Service[services.LyricsService](env.Core)
	}
	env.PlayerControls.Add(controlOrder, m.button)
	return module.Module{
		ID: "miniplayer",
		Shortcuts: []module.Shortcut{{
			ID:       "miniplayer.toggle",
			Label:    func() string { return i18n.T("Show the mini player") },
			Group:    "view",
			Keys:     []string{"Ctrl+Shift+M"},
			Keywords: func() string { return i18n.T("compact small window") },
			Icon:     "picture-in-picture-2",
			Run:      m.toggle,
		}},
		Slots: module.Slots{Mini: m.content},
		Init:  m.init,
	}
}

func newMini(env *module.Env) *mini {
	return &mini{env: env, now: time.Now}
}

// init follows the per-song timing offset the main window changes, so the
// sung line moves with it even while the song is paused.
func (m *mini) init() {
	lyricsync.Session().OnChange(func() { m.env.Post(func() {}) })
}

// button is the player bar's Mini player button, among its controls.
func (m *mini) button(c *ui.Context) {
	if kit.Button(c.Key("mini"), "", kit.ButtonOpts{
		Variant: kit.Ghost, Size: kit.SizeSM, Icon: "picture-in-picture-2", A11yLabel: i18n.T("Mini player"),
	}).Clicked() {
		m.toggle()
	}
}

// toggle opens the mini player in place of the main window, or returns from
// it.
func (m *mini) toggle() { go m.desk.ToggleMiniPlayer() }
