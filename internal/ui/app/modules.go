package app

import (
	"github.com/coredev-uk/sona-go/internal/ui/features/album"
	"github.com/coredev-uk/sona-go/internal/ui/features/artist"
	"github.com/coredev-uk/sona-go/internal/ui/features/auth"
	"github.com/coredev-uk/sona-go/internal/ui/features/cdm"
	"github.com/coredev-uk/sona-go/internal/ui/features/commands"
	"github.com/coredev-uk/sona-go/internal/ui/features/concerts"
	"github.com/coredev-uk/sona-go/internal/ui/features/discover"
	"github.com/coredev-uk/sona-go/internal/ui/features/equalizer"
	"github.com/coredev-uk/sona-go/internal/ui/features/home"
	"github.com/coredev-uk/sona-go/internal/ui/features/library"
	"github.com/coredev-uk/sona-go/internal/ui/features/onboarding"
	"github.com/coredev-uk/sona-go/internal/ui/features/player"
	"github.com/coredev-uk/sona-go/internal/ui/features/playlist"
	"github.com/coredev-uk/sona-go/internal/ui/features/private"
	"github.com/coredev-uk/sona-go/internal/ui/features/radio"
	"github.com/coredev-uk/sona-go/internal/ui/features/reminders"
	"github.com/coredev-uk/sona-go/internal/ui/features/search"
	"github.com/coredev-uk/sona-go/internal/ui/features/settings"
	"github.com/coredev-uk/sona-go/internal/ui/features/song"
	"github.com/coredev-uk/sona-go/internal/ui/features/updates"
	"github.com/coredev-uk/sona-go/internal/ui/features/video"
	"github.com/coredev-uk/sona-go/internal/ui/module"
)

// factories is the native modules.ts: one line per ported feature, in
// v0.2.0's order (routes, event handlers, shortcuts and shell slots follow
// it; nav items sort by their own Order). Features not ported yet are
// absent: there are no placeholders.
var factories = []module.Factory{
	home.New,      // For you, and one shelf in full
	discover.New,  // Discover, Charts, rooms, curators, groupings; the item players
	search.New,    // search field, results, group and profile pages
	concerts.New,  // concerts page and location dialog
	library.New,   // the library pages, the sync indicator and the item actions
	album.New,     // the album page
	artist.New,    // the artist page and its see-all views
	song.New,      // the song page
	video.New,     // the music video page and view
	playlist.New,  // the playlist page, the sidebar tree and pins, the dialogs
	radio.New,     // Radio, stations, and the station actions
	player.New,    // the player bar, the queue, Now Playing and lyrics
	equalizer.New, // the equaliser page
	settings.New,  // the settings page
	auth.New,      // sign-in state, the account menu and dialog
	commands.New,  // the command palette and the shortcut help
	updates.New,   // the sidebar's update notice
	// motion has no module: the policy is kit.MotionPolicy (Env.Motion), kept by the shell
	private.New,    // the player bar's private listening pill and its command
	cdm.New,        // the Widevine mirror and the player bar's notice
	reminders.New,  // release reminders' album action
	onboarding.New, // the first-run setup layer
}
