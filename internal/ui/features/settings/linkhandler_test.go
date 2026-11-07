package settings

import (
	"errors"
	"fmt"
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
)

// linkRig is General on a Mac whose system says who opens Apple Music links
// (link, or linkErr), asked again once it does.
func linkRig(t *testing.T, caps services.Capabilities, link services.LinkHandlerStatus, linkErr error) *generalRig {
	t.Helper()
	g := newGeneralRig(t, caps, nil)
	g.dt.mu.Lock()
	g.dt.link, g.dt.linkErr = link, linkErr
	g.dt.mu.Unlock()
	g.h.Env.Queries.Invalidate(keyLinkHandler)
	return g
}

func linkCaps() services.Capabilities {
	c := macCaps()
	c.LinkHandler = true
	return c
}

// While Music.app (or any other app) opens the links, the row says which and
// the button asks the system to use Sona; once it does the row says so.
func TestLinkHandlerRowMakesSonaTheDefault(t *testing.T) {
	g := linkRig(t, linkCaps(), services.LinkHandlerStatus{Other: "Music"}, nil)
	g.until("who opens them", g.tt.Shows("Open Apple Music links in Sona", "They open in Music now.", "Make Sona the default"))
	g.click("Make Sona the default")
	g.until("Sona opens them", g.tt.Shows("Sona opens them"))
	if g.tt.HasText("Make Sona the default") || g.tt.HasText("They open in Music now.") {
		t.Errorf("the row still offers the change: %q", g.tt.Texts())
	}
	if n := g.dt.linkSets(); n != 1 {
		t.Errorf("the system was asked %d times, want once", n)
	}
}

func TestLinkHandlerRowOffersNothingWhenSonaOpensThem(t *testing.T) {
	g := linkRig(t, linkCaps(), services.LinkHandlerStatus{Sona: true}, nil)
	g.until("the state", g.tt.Shows("Open Apple Music links in Sona", "Sona opens them"))
	if g.tt.HasText("Make Sona the default") {
		t.Errorf("a button to change what is already so: %q", g.tt.Texts())
	}
}

// A system that keeps its own choice (the user said no to the prompt) is
// said, and the row still offers Sona.
func TestLinkHandlerRowSaysWhenTheSystemKeepsItsChoice(t *testing.T) {
	g := linkRig(t, linkCaps(), services.LinkHandlerStatus{Other: "Music"}, nil)
	g.dt.mu.Lock()
	g.dt.keeps = true
	g.dt.mu.Unlock()
	g.until("the button", g.tt.Shows("Make Sona the default"))
	g.click("Make Sona the default")
	g.until("the toast", g.tt.Shows("Sona isn't the default yet", "Your system kept Music."))
	if !g.tt.HasText("Make Sona the default") || g.tt.HasText("Sona opens them") {
		t.Errorf("the row claims a change that didn't happen: %q", g.tt.Texts())
	}
}

// A build that isn't an installed app can't be chosen: the row says what
// would make it possible, with no button.
func TestLinkHandlerRowExplainsABuildThatIsNotInstalled(t *testing.T) {
	g := linkRig(t, linkCaps(), services.LinkHandlerStatus{}, fmt.Errorf("a development bundle: %w", errors.ErrUnsupported))
	g.until("the reason", g.tt.Shows("Only the installed Sona.app can be chosen for them, not a source build or a development bundle."))
	if g.tt.HasText("Make Sona the default") {
		t.Errorf("a button that can't work: %q", g.tt.Texts())
	}
}

// A run outside Sona.app says it needs the bundle.
func TestLinkHandlerRowOutsideTheAppBundleSaysItNeedsIt(t *testing.T) {
	caps := linkCaps()
	caps.LinkHandler, caps.NeedsAppBundle = false, true
	g := newGeneralRig(t, caps, nil)
	g.until("the row", g.tt.Shows("Open Apple Music links in Sona", "Only when Sona runs from Sona.app, not from a source build."))
}

// Windows' installer registers the links for the user: no row.
func TestLinkHandlerRowIsAbsentWhereThePlatformHasNone(t *testing.T) {
	caps := linuxCaps()
	caps.OS = "windows"
	g := newGeneralRig(t, caps, nil)
	if g.tt.HasText("Open Apple Music links in Sona") {
		t.Errorf("Windows shows the row: %q", g.tt.Texts())
	}
}
