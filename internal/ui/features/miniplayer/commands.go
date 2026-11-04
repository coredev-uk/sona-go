package miniplayer

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// noticeFor is how long a failed command's message replaces the third line.
const noticeFor = 4 * time.Second

// notice is a failed command, said on the third line: the mini player has no
// room for toasts (v0.2.0's useCommand).
type notice struct {
	text  string
	id    int       // tells a repeated failure from the one showing
	until time.Time // zero until a frame shows it
}

// run starts a command off the main thread; its failure is said on the
// third line as "what: why".
func (m *mini) run(what string, fn func(ctx context.Context) error) {
	go func() {
		if err := fn(context.Background()); err != nil {
			m.env.Post(func() { m.fail(what, services.CodeOf(err)) })
		}
	}()
}

// fail says a command failed. Signing in again is the one thing the user can
// do about two of the codes, and says it in the app's own words.
func (m *mini) fail(what string, code services.ErrorCode) {
	why := i18n.ErrorText(code)
	if code == services.CodeNotSignedIn || code == services.CodeMUTRejected {
		why = i18n.T("sign in to Apple Music in Sona")
	}
	m.noticeID++
	m.notice = &notice{text: what + ": " + why, id: m.noticeID}
}

// noticeText is the message to show on the third line, "" for none: the
// first frame to show it starts its four seconds and announces it once.
func (m *mini) noticeText(c *ui.Context) string {
	n := m.notice
	if n == nil {
		return ""
	}
	now := c.Now()
	if n.until.IsZero() {
		n.until = now.Add(noticeFor)
		c.Announce(n.text)
	}
	if !now.Before(n.until) {
		m.notice = nil
		return ""
	}
	c.After(n.until.Sub(now))
	return n.text
}

// The commands. Each says what it was doing (the notice's start) as a
// literal, so the catalogue's extraction finds it.

func (m *mini) toggleCmd() {
	m.run(i18n.T("Couldn't play or pause"), m.svc.Toggle)
}

func (m *mini) nextCmd() {
	m.run(i18n.T("Couldn't skip"), m.svc.Next)
}

func (m *mini) previousCmd() {
	m.run(i18n.T("Couldn't go back"), m.svc.Prev)
}

// seekTo seeks to a position in the playing song: the mirror shows it at
// once, until Go's next snapshot.
func (m *mini) seekTo(pos time.Duration) {
	ms := pos.Milliseconds()
	now := m.now().UnixMilli()
	m.env.Play.Patch(func(s *services.PlaybackState) { s.PositionMs, s.At = ms, now })
	m.run(i18n.T("Couldn't seek"), func(ctx context.Context) error { return m.svc.Seek(ctx, ms) })
}

// expand brings the main window back (and closes the mini player). The
// window calls run off the main thread: the service waits for the main one.
func (m *mini) expand() {
	go m.desk.ShowMainWindow()
}

// close is the window's own close button: Sona keeps playing in the
// background or quits, as closing the main window does.
func (m *mini) close() {
	go m.desk.CloseMiniPlayer()
}
