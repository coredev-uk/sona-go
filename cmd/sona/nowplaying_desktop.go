//go:build darwin || windows

package main

import (
	"log/slog"
	"time"

	"github.com/egoist/mygo"

	"github.com/coredev-uk/sona-go/internal/mpris"
	"github.com/coredev-uk/sona-go/internal/nowplaying"
	"github.com/coredev-uk/sona-go/internal/services"
)

// mediaControls is the system's Now Playing and media keys (MPNowPlayingInfoCenter
// on macOS, SystemMediaTransportControls on Windows) through mygo.NowPlaying.
func mediaControls(log *slog.Logger) func(mpris.Player) (services.MediaControls, error) {
	return func(player mpris.Player) (services.MediaControls, error) {
		return nowplaying.New(mygoBackend{}, player, log), nil
	}
}

// mygoBackend is mygo.NowPlaying as a nowplaying.Backend.
type mygoBackend struct{}

var states = [...]mygo.PlaybackState{nowplaying.Playing: mygo.PlaybackPlaying, nowplaying.Paused: mygo.PlaybackPaused, nowplaying.Stopped: mygo.PlaybackStopped}

func (mygoBackend) Set(i nowplaying.Info) {
	mygo.NowPlaying.Set(mygo.NowPlayingInfo{
		Title: i.Title, Artist: i.Artist, Album: i.Album, Artwork: i.Artwork,
		Duration: i.Duration, Position: i.Position, State: states[i.State],
		Commands: toMygoCommands(i.Commands),
	})
}

func (mygoBackend) SetPlayback(s nowplaying.State, pos time.Duration) {
	mygo.NowPlaying.SetPlayback(states[s], pos)
}

func (mygoBackend) Clear() { mygo.NowPlaying.Clear() }

func (mygoBackend) OnCommand(fn func(nowplaying.Command, time.Duration)) func() {
	return mygo.NowPlaying.OnCommand(func(c mygo.MediaCommand, pos time.Duration) {
		switch c {
		case mygo.MediaPlay:
			fn(nowplaying.Play, pos)
		case mygo.MediaPause:
			fn(nowplaying.Pause, pos)
		case mygo.MediaStop:
			fn(nowplaying.Stop, pos)
		case mygo.MediaNext:
			fn(nowplaying.Next, pos)
		case mygo.MediaPrevious:
			fn(nowplaying.Previous, pos)
		case mygo.MediaSeek:
			fn(nowplaying.Seek, pos)
		}
	})
}

func toMygoCommands(c nowplaying.Command) mygo.MediaCommand {
	var out mygo.MediaCommand
	for _, m := range [...]struct {
		from nowplaying.Command
		to   mygo.MediaCommand
	}{
		{nowplaying.Play, mygo.MediaPlay}, {nowplaying.Pause, mygo.MediaPause}, {nowplaying.Stop, mygo.MediaStop},
		{nowplaying.Next, mygo.MediaNext}, {nowplaying.Previous, mygo.MediaPrevious}, {nowplaying.Seek, mygo.MediaSeek},
	} {
		if c&m.from != 0 {
			out |= m.to
		}
	}
	return out
}
