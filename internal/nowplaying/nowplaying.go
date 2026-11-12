// Package nowplaying adapts Sona's playback state to the system's Now
// Playing and media keys on macOS (MPNowPlayingInfoCenter) and Windows
// (SystemMediaTransportControls). The system side is MyGo's
// mygo.NowPlaying, reached through the Backend seam so this package, and
// its tests, never link it; cmd/sona wires the real one on macOS and
// Windows only. Linux's only player is Go's MPRIS (internal/mpris).
package nowplaying

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/diag"
	"github.com/coredev-uk/sona-go/internal/mpris"
)

// Command is a media key or system control.
type Command int

const (
	Play Command = 1 << iota
	Pause
	Stop
	Next
	Previous
	Seek
)

// State is whether what is shown plays.
type State int

const (
	Playing State = iota
	Paused
	Stopped
)

// Info is what the system shows.
type Info struct {
	Title, Artist, Album string
	Artwork              []byte // PNG or JPEG
	Duration, Position   time.Duration
	State                State
	Commands             Command
}

// Backend is the system's Now Playing (mygo.NowPlaying).
type Backend interface {
	Set(Info)
	SetPlayback(State, time.Duration)
	Clear()
	// OnCommand registers fn for commands, called on the main thread;
	// position is where Seek moves to. It returns the unregistration.
	OnCommand(fn func(cmd Command, position time.Duration)) (off func())
}

// callTimeout bounds one command's call into the player.
const callTimeout = 10 * time.Second

// driftTolerance is how far the reported position may stray from the
// extrapolated one before the system is told (a seek).
const driftTolerance = time.Second

// Adapter publishes snapshots to a Backend and applies its commands to a
// mpris.Player.
type Adapter struct {
	be     Backend
	player mpris.Player
	log    *slog.Logger
	off    func()
	cmds   chan func()
	done   chan struct{}
	once   sync.Once

	mu      sync.Mutex // Update and the command side (track id)
	showing bool
	shown   shown
	trackID string
	art     []byte
	state   State
	pos     time.Duration
	at      time.Time
}

// shown is what the last Set carried, to diff the next snapshot against.
type shown struct {
	title, artist, album string
	duration             time.Duration
	commands             Command
	artFile              string
}

// New registers for b's commands, which act on player.
func New(b Backend, player mpris.Player, log *slog.Logger) *Adapter {
	a := &Adapter{be: b, player: player, log: log, cmds: make(chan func(), 16), done: make(chan struct{})}
	diag.Go(log, "nowplaying commands", a.worker)
	a.off = b.OnCommand(a.onCommand)
	return a
}

// worker runs commands in order, off the main thread.
func (a *Adapter) worker() {
	for {
		select {
		case <-a.done:
			return
		case fn := <-a.cmds:
			fn()
		}
	}
}

// onCommand runs on the main thread: it only queues.
func (a *Adapter) onCommand(cmd Command, pos time.Duration) {
	a.mu.Lock()
	id := a.trackID
	a.mu.Unlock()
	select {
	case a.cmds <- func() { a.run(cmd, pos, id) }:
	default:
		a.log.Warn("nowplaying: command dropped", "cmd", cmd)
	}
}

func (a *Adapter) run(cmd Command, pos time.Duration, id string) {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	var err error
	switch cmd {
	case Play:
		err = a.player.Play(ctx)
	case Pause:
		err = a.player.Pause(ctx)
	case Stop:
		err = a.player.Stop(ctx)
	case Next:
		err = a.player.Next(ctx)
	case Previous:
		err = a.player.Previous(ctx)
	case Seek:
		err = a.player.SetPosition(ctx, id, pos)
	}
	if err != nil {
		a.log.Debug("nowplaying: command", "cmd", cmd, "err", err)
	}
}

// Update publishes s: Set when the track, its artwork or the offered
// commands changed, SetPlayback when only the state changed or the
// position jumped, nothing otherwise.
func (a *Adapter) Update(s mpris.Snapshot) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if s.Track == nil {
		a.trackID = ""
		if a.showing {
			a.showing, a.shown, a.art = false, shown{}, nil
			a.be.Clear()
		}
		return
	}
	a.trackID = s.Track.ID
	state := stateOf(s.Status)
	now := time.Now()
	at := s.UpdatedAt
	if at.IsZero() {
		at = now
	}
	pos := s.Position
	if s.Status == mpris.StatusPlaying {
		pos += now.Sub(at)
	}
	cur := shown{
		title: s.Track.Title, artist: s.Track.Artist, album: s.Track.Album,
		duration: s.Track.Length, commands: commandsOf(s),
		artFile: artPath(s.Track.ArtURL),
	}
	if !a.showing || cur != a.shown {
		if cur.artFile != a.shown.artFile || !a.showing {
			a.art = nil
			if cur.artFile != "" {
				// A small cached file; this runs on the media goroutine,
				// not the main thread.
				a.art, _ = os.ReadFile(cur.artFile)
			}
		}
		a.be.Set(Info{
			Title: cur.title, Artist: cur.artist, Album: cur.album, Artwork: a.art,
			Duration: cur.duration, Position: pos, State: state, Commands: cur.commands,
		})
		a.showing, a.shown, a.state, a.pos, a.at = true, cur, state, pos, now
		return
	}
	expected := a.pos
	if a.state == Playing {
		expected += now.Sub(a.at)
	}
	if state == a.state && (pos-expected).Abs() <= driftTolerance {
		return
	}
	a.be.SetPlayback(state, pos)
	a.state, a.pos, a.at = state, pos, now
}

// Close removes what is shown and stops taking commands.
func (a *Adapter) Close() error {
	a.once.Do(func() {
		a.off()
		close(a.done)
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.showing {
			a.showing = false
			a.be.Clear()
		}
	})
	return nil
}

func stateOf(s mpris.PlaybackStatus) State {
	switch s {
	case mpris.StatusPlaying:
		return Playing
	case mpris.StatusPaused:
		return Paused
	}
	return Stopped
}

func commandsOf(s mpris.Snapshot) Command {
	if !s.CanControl {
		return 0
	}
	var c Command
	if s.CanPlay {
		c |= Play
	}
	if s.CanPause {
		c |= Pause | Stop
	}
	if s.CanGoNext {
		c |= Next
	}
	if s.CanGoPrevious {
		c |= Previous
	}
	if s.CanSeek {
		c |= Seek
	}
	return c
}

// artPath is the file behind a file:// artwork URL.
func artPath(u string) string {
	return strings.TrimPrefix(u, "file://")
}
