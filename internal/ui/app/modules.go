package app

import (
	"github.com/coredev-uk/sona-go/internal/ui/features/library"
	"github.com/coredev-uk/sona-go/internal/ui/features/player"
	"github.com/coredev-uk/sona-go/internal/ui/module"
)

// factories is the native modules.ts: one line per ported feature, in
// v0.2.0's order (routes, event handlers, shortcuts and shell slots follow
// it; nav items sort by their own Order). Features not ported yet are
// absent: there are no placeholders.
var factories = []module.Factory{
	library.New, // Albums only
	player.New,  // the player bar only
}
