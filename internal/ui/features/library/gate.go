package library

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// Loading placeholders at the lists' real sizes: 16 rows, 12 tiles.
const (
	skeletonRows  = 16
	skeletonTiles = 12
)

// libraryKind is what a library page lists, which words its gate.
type libraryKind int

const (
	kindSongs libraryKind = iota
	kindAlbums
	kindArtists
	kindPlaylists
	kindRecent
)

// gateCopy is one full sentence per kind, so translations never splice a
// noun into a sentence.
type gateCopy struct{ err, signIn, syncing, none string }

func (k libraryKind) copy() gateCopy {
	switch k {
	case kindSongs:
		return gateCopy{
			err:     i18n.T("Couldn't read your songs from this computer"),
			signIn:  i18n.T("Sign in to see your songs"),
			syncing: i18n.T("Your songs appear here as Sona copies them from Apple Music. Large libraries take a minute or two the first time."),
			none:    i18n.T("No songs in your library yet"),
		}
	case kindAlbums:
		return gateCopy{
			err:     i18n.T("Couldn't read your albums from this computer"),
			signIn:  i18n.T("Sign in to see your albums"),
			syncing: i18n.T("Your albums appear here as Sona copies them from Apple Music. Large libraries take a minute or two the first time."),
			none:    i18n.T("No albums in your library yet"),
		}
	case kindArtists:
		return gateCopy{
			err:     i18n.T("Couldn't read your artists from this computer"),
			signIn:  i18n.T("Sign in to see your artists"),
			syncing: i18n.T("Your artists appear here as Sona copies them from Apple Music. Large libraries take a minute or two the first time."),
			none:    i18n.T("No artists in your library yet"),
		}
	case kindPlaylists:
		return gateCopy{
			err:     i18n.T("Couldn't read your playlists from this computer"),
			signIn:  i18n.T("Sign in to see your playlists"),
			syncing: i18n.T("Your playlists appear here as Sona copies them from Apple Music. Large libraries take a minute or two the first time."),
			none:    i18n.T("No playlists in your library yet"),
		}
	default:
		return gateCopy{
			err:     i18n.T("Couldn't read your recent additions from this computer"),
			signIn:  i18n.T("Sign in to see your recent additions"),
			syncing: i18n.T("Your recent additions appear here as Sona copies them from Apple Music. Large libraries take a minute or two the first time."),
			none:    i18n.T("No recent additions in your library yet"),
		}
	}
}

// gateOpts are what useLibraryGate took.
type gateOpts struct {
	total    int
	pending  bool
	err      error
	retry    func()
	retrying bool
	kind     libraryKind
	// empty is the hint of a synced, empty list.
	empty string
	frame kit.EmptyFrame
}

// signIn is the Sign in action of the empty states: it opens the account
// dialog; none while the shell has no dialog to open.
func (l *library) signIn() *kit.EmptyAction {
	if l.openAccount == nil {
		return nil
	}
	return &kit.EmptyAction{Label: i18n.T("Sign in"), Run: l.openAccount}
}

// gated reports whether gate takes the place of the list: an error, or an
// empty list of a known account that is not restoring.
func (l *library) gated(o gateOpts) bool {
	if o.err != nil {
		return true
	}
	auth := l.authStatus()
	return !o.pending && o.total == 0 && auth.HasData && auth.Data.State != "restoring"
}

// gate is what a library page shows instead of its list: an error, a
// sign-in prompt, a first sync in progress, or an empty library. It builds
// that and reports true; false means show the list (or its skeleton while
// the first page loads). Nothing shows while the first page is pending,
// when there is data, when the account is not known yet or is restoring.
func (l *library) gate(c *ui.Context, o gateOpts) bool {
	if !l.gated(o) {
		return false
	}
	words := o.kind.copy()
	auth := l.authStatus()
	status := l.syncStatus()
	if o.err != nil {
		kit.ErrorBlock(c, words.err, o.err, kit.ErrorBlockOpts{OnRetry: o.retry, Retrying: o.retrying})
		return true
	}
	running, _, _, _ := l.sync.state()
	switch {
	case signedOut(auth.Data.State):
		kit.EmptyState(c, words.signIn,
			i18n.T("Sona copies your Apple Music library to this computer after you sign in, so it opens instantly and works offline."),
			kit.EmptyOpts{Frame: o.frame, Action: l.signIn()})
	case running || status.Data.LastSuccess == 0:
		kit.EmptyState(c, i18n.T("Your library is syncing"), words.syncing, kit.EmptyOpts{Frame: o.frame})
	default:
		kit.EmptyState(c, words.none, o.empty, kit.EmptyOpts{Frame: o.frame})
	}
	return true
}
