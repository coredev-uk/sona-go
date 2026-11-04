package search

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// scopeSwitch is v0.2.0's ScopeSwitch: where the search looks, the Apple
// Music catalogue or the library on this computer. Changing it replaces
// the page's history entry, keeping the term and writing scope=library only
// for the library.
func (s *search) scopeSwitch(c *ui.Context, q, scope string) {
	v := scope
	opts := []kit.SegmentOption[string]{
		{Value: scopeCatalog, Label: i18n.T("Apple Music")},
		{Value: scopeLibrary, Label: i18n.T("Library")},
	}
	if kit.Segmented(c, &v, opts, i18n.T("Search in")).Changed() {
		s.env.Nav.Replace(searchLocation(q, v))
	}
}
