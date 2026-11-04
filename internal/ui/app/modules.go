package app

import (
	"github.com/coredev-uk/sona-go/internal/ui/features/auth"
	"github.com/coredev-uk/sona-go/internal/ui/features/cdm"
	"github.com/coredev-uk/sona-go/internal/ui/features/discover"
	"github.com/coredev-uk/sona-go/internal/ui/features/home"
	"github.com/coredev-uk/sona-go/internal/ui/features/library"
	"github.com/coredev-uk/sona-go/internal/ui/features/onboarding"
	"github.com/coredev-uk/sona-go/internal/ui/features/player"
	"github.com/coredev-uk/sona-go/internal/ui/features/private"
	"github.com/coredev-uk/sona-go/internal/ui/features/radio"
	"github.com/coredev-uk/sona-go/internal/ui/features/reminders"
	"github.com/coredev-uk/sona-go/internal/ui/features/updates"
	"github.com/coredev-uk/sona-go/internal/ui/module"
)

// factories is the native modules.ts: one line per ported feature, in
// v0.2.0's order (routes, event handlers, shortcuts and shell slots follow
// it; nav items sort by their own Order). Features not ported yet are
// absent: there are no placeholders.
var factories = []module.Factory{
	home.New,     // For you, and one shelf in full
	discover.New, // Discover, Charts, rooms, curators, groupings; the item players
	library.New,  // Albums only
	radio.New,    // Radio, stations, and the station actions
	player.New,   // the player bar only
	auth.New,     // sign-in state, the account menu and dialog
	updates.New,  // the sidebar's update notice
	// motion has no module: the policy is kit.MotionPolicy (Env.Motion), kept by the shell
	private.New,    // the player bar's private listening pill and its command
	cdm.New,        // the Widevine mirror and the player bar's notice
	reminders.New,  // release reminders' album action
	onboarding.New, // the first-run setup layer
}
