package app

import (
	"github.com/coredev-uk/sona-go/internal/ui/features/album"
	"github.com/coredev-uk/sona-go/internal/ui/features/artist"
	"github.com/coredev-uk/sona-go/internal/ui/features/auth"
	"github.com/coredev-uk/sona-go/internal/ui/features/cdm"
	"github.com/coredev-uk/sona-go/internal/ui/features/commands"
	"github.com/coredev-uk/sona-go/internal/ui/features/equalizer"
	"github.com/coredev-uk/sona-go/internal/ui/features/concerts"
	"github.com/coredev-uk/sona-go/internal/ui/features/library"
	"github.com/coredev-uk/sona-go/internal/ui/features/onboarding"
	"github.com/coredev-uk/sona-go/internal/ui/features/player"
	"github.com/coredev-uk/sona-go/internal/ui/features/playlist"
	"github.com/coredev-uk/sona-go/internal/ui/features/private"
	"github.com/coredev-uk/sona-go/internal/ui/features/reminders"
	"github.com/coredev-uk/sona-go/internal/ui/features/settings"
	"github.com/coredev-uk/sona-go/internal/ui/features/song"
	"github.com/coredev-uk/sona-go/internal/ui/features/search"
	"github.com/coredev-uk/sona-go/internal/ui/features/updates"
	"github.com/coredev-uk/sona-go/internal/ui/module"
)

// factories is the native modules.ts: one line per ported feature, in
// v0.2.0's order (routes, event handlers, shortcuts and shell slots follow
// it; nav items sort by their own Order). Features not ported yet are
// absent: there are no placeholders.
var factories = []module.Factory{
	library.New, // Albums only
	album.New,   // the album page
	artist.New,  // the artist page and its see-all views
	song.New,    // the song page
	library.New,
	player.New,  // the player bar only
	auth.New,    // sign-in state, the account menu and dialog
	updates.New, // the sidebar's update notice
	library.New,  // Albums only
	playlist.New, // the playlist page, the sidebar tree and pins, the dialogs
	player.New,   // the player bar only
	auth.New,     // sign-in state, the account menu and dialog
	search.New,   // search field, results, group and profile pages
	concerts.New, // concerts page and location dialog
	updates.New,  // the sidebar's update notice
	// motion has no module: the policy is kit.MotionPolicy (Env.Motion), kept by the shell
	private.New,    // the player bar's private listening pill and its command
	cdm.New,        // the Widevine mirror and the player bar's notice
	reminders.New,  // release reminders' album action
	onboarding.New, // the first-run setup layer
	settings.New,   // the settings page
	equalizer.New,
	commands.New,
}
