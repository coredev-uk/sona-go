package player

import (
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// actions are the queue actions (features/player/actions.ts): a song or music
// video queues next or last; a selection of songs, music videos, albums and
// playlists queues them all in order (a collection as its songs). A live
// station has no queue to add to.
func (p *player) actions() []kit.ActionDef {
	noQueue := func(*kit.ActionContext) string { return unavailable(p.env.Play.State(), ctlQueue) }
	playNext := func(*kit.ActionContext) string { return i18n.T("Play next") }
	playLast := func(*kit.ActionContext) string { return i18n.T("Play last") }
	songPlayable := func(a *kit.ActionContext) bool { return a.Item().Song().Playable }
	// Selected songs can be queued when one of them plays; collections are
	// checked when loaded.
	somePlayable := func(a *kit.ActionContext) bool {
		for _, it := range a.Items {
			if it.Kind != kit.KindSong || it.Song().Playable {
				return true
			}
		}
		return false
	}
	run := func(next bool) func(*kit.ActionContext) {
		return func(a *kit.ActionContext) { p.cmds.QueueItems(a.Items, next, !next, 0) }
	}
	def := func(id string, target kit.ItemKind, label func(*kit.ActionContext) string, icon string, order int, next bool, enabled func(*kit.ActionContext) bool) kit.ActionDef {
		return kit.ActionDef{
			ID: id, Target: target, Label: label, Icon: icon, Group: kit.GroupQueue, Order: order,
			DisabledReason: noQueue, Enabled: enabled, Run: run(next),
		}
	}
	many := func(id string, label func(*kit.ActionContext) string, icon string, order int, next bool) kit.ActionDef {
		d := def(id, kit.KindSelection, label, icon, order, next, somePlayable)
		d.Kinds = []kit.ItemKind{kit.KindSong, kit.KindMusicVideo, kit.KindAlbum, kit.KindPlaylist}
		return d
	}
	return []kit.ActionDef{
		def("player.play-next", kit.KindSong, playNext, "list-start", 10, true, songPlayable),
		def("player.add-to-queue", kit.KindSong, playLast, "list-end", 20, false, songPlayable),
		def("player.play-next.video", kit.KindMusicVideo, playNext, "list-start", 10, true, nil),
		def("player.add-to-queue.video", kit.KindMusicVideo, playLast, "list-end", 20, false, nil),
		many("player.play-next.many", playNext, "list-start", 10, true),
		many("player.add-to-queue.many", playLast, "list-end", 20, false),
	}
}
