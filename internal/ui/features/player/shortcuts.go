package player

import (
	"time"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/module"
)

const (
	seekStep   = 10 * time.Second
	volumeStep = 0.05
)

// shortcuts are the keys for what the bar does, with v0.2.0's IDs and keys
// (features/player/shortcuts.ts). Space and the arrows leave a focused
// button or slider alone: MyGo gives them to it first.
func (p *player) shortcuts() []module.Shortcut {
	volume := func() bool { return p.volume() }
	cur := func(ctl control) func() string {
		return func() string { return unavailable(p.env.Play.State(), ctl) }
	}
	list := []module.Shortcut{
		{
			ID: "player.toggle", Group: "playback", Keys: []string{"Space"}, Icon: "pause",
			Label:    func() string { return i18n.T("Play or pause") },
			Keywords: func() string { return i18n.T("resume stop") },
			Run:      p.toggle,
		},
		{
			ID: "player.next", Group: "playback", Keys: []string{"Ctrl+ArrowRight"}, Icon: "skip-forward",
			Label:       func() string { return i18n.T("Next track") },
			Keywords:    func() string { return i18n.T("skip forward") },
			Unavailable: cur(ctlNext),
			Run:         p.next,
		},
		{
			ID: "player.previous", Group: "playback", Keys: []string{"Ctrl+ArrowLeft"}, Icon: "skip-back",
			Label:       func() string { return i18n.T("Previous track") },
			Keywords:    func() string { return i18n.T("back restart") },
			Unavailable: cur(ctlPrevious),
			Run:         p.previous,
		},
		{
			ID: "player.seek-forward", Group: "playback", Keys: []string{"Shift+ArrowRight"}, Icon: "fast-forward", Repeat: true,
			Label:       func() string { return i18n.T("Skip ahead 10 seconds") },
			Keywords:    func() string { return i18n.T("seek forward") },
			Unavailable: cur(ctlSeek),
			Run:         func() { p.seekBy(seekStep) },
		},
		{
			ID: "player.seek-back", Group: "playback", Keys: []string{"Shift+ArrowLeft"}, Icon: "rewind", Repeat: true,
			Label:       func() string { return i18n.T("Go back 10 seconds") },
			Keywords:    func() string { return i18n.T("seek rewind") },
			Unavailable: cur(ctlSeek),
			Run:         func() { p.seekBy(-seekStep) },
		},
		{
			ID: "player.volume-up", Group: "playback", Keys: []string{"Ctrl+ArrowUp"}, Icon: "volume-2", Repeat: true,
			Label:    func() string { return i18n.T("Volume up") },
			Keywords: func() string { return i18n.T("louder") },
			When:     volume,
			Run:      func() { p.setVolume(p.env.Play.State().Volume + volumeStep) },
		},
		{
			ID: "player.volume-down", Group: "playback", Keys: []string{"Ctrl+ArrowDown"}, Icon: "volume-1", Repeat: true,
			Label:    func() string { return i18n.T("Volume down") },
			Keywords: func() string { return i18n.T("quieter") },
			When:     volume,
			Run:      func() { p.setVolume(p.env.Play.State().Volume - volumeStep) },
		},
		{
			ID: "player.mute", Group: "playback", Keys: []string{"M"}, Icon: "volume-x",
			Label:    func() string { return i18n.T("Mute or unmute") },
			Keywords: func() string { return i18n.T("silence sound") },
			When:     volume,
			Run:      p.toggleMute,
		},
		{
			ID: "player.shuffle", Group: "playback", Keys: []string{"S"}, Icon: "shuffle",
			Label:       func() string { return i18n.T("Shuffle on or off") },
			Keywords:    func() string { return i18n.T("random") },
			Unavailable: cur(ctlShuffle),
			Run:         p.toggleShuffle,
		},
		{
			ID: "player.repeat", Group: "playback", Keys: []string{"R"}, Icon: "repeat",
			Label:       func() string { return i18n.T("Repeat off, all or one") },
			Keywords:    func() string { return i18n.T("loop") },
			Unavailable: cur(ctlRepeat),
			Run:         p.cycleRepeat,
		},
		{
			ID: "player.favourite", Group: "playback", Keys: []string{"F"}, Icon: "star",
			Label:    func() string { return i18n.T("Favourite the playing song") },
			Keywords: func() string { return i18n.T("like love star favorite") },
			// Not while a music video plays (v0.2.0's !videoPlaying()).
			When: func() bool { cur := p.env.Play.State().Current; return cur == nil || !cur.Video },
			Run:  p.toggleFavourite,
		},
		{
			ID: "player.queue", Group: "view", Keys: []string{"Q"}, Icon: "list-music",
			Label:    func() string { return i18n.T("Show or hide the queue") },
			Keywords: func() string { return i18n.T("up next") },
			Run:      p.toggleQueueView,
		},
	}
	return append(list, p.sleepShortcuts()...)
}

// sleepShortcuts are the sleep timer's commands for the palette (v0.2.0's
// player.sleep-*, no default keys): each one while something plays, and the
// one that turns it off while a timer runs.
func (p *player) sleepShortcuts() []module.Shortcut {
	playing := func() bool { return p.env.Play.State().Current != nil }
	timer := func(id string, label func() string, mode string, minutes int) module.Shortcut {
		return module.Shortcut{
			ID: id, Group: "playback", Icon: "moon", Label: label, When: playing,
			Keywords: func() string { return i18n.T("sleep timer bedtime fade") },
			Run:      func() { p.setSleep(mode, minutes) },
		}
	}
	return []module.Shortcut{
		timer("player.sleep-30", func() string { return i18n.T("Sleep timer: stop in 30 minutes") }, "after", 30),
		timer("player.sleep-60", func() string { return i18n.T("Sleep timer: stop in 1 hour") }, "after", 60),
		timer("player.sleep-track", func() string { return i18n.T("Sleep timer: stop after this song") }, "track", 0),
		timer("player.sleep-queue", func() string { return i18n.T("Sleep timer: stop after the queue") }, "queue", 0),
		{
			ID: "player.sleep-off", Group: "playback", Icon: "moon",
			Label:    func() string { return i18n.T("Turn off the sleep timer") },
			Keywords: func() string { return i18n.T("sleep timer cancel") },
			When:     func() bool { return p.env.Play.State().Sleep != nil },
			Run:      func() { p.setSleep("", 0) },
		},
	}
}
