package album

import (
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/theme/palette"
)

// colours is the album's colours from its cover (v0.2.0's albumColors, in
// theme/palette) as the page uses them: the sleeve's surface and the colour
// tokens its content is built with, and the accent's tokens, which take
// amber's place on the whole page. It is built once per cover, theme and
// layout (palKey), never per frame.
type colours struct {
	sleeve    palette.Sleeve
	surface   ui.Color
	sleeveTok map[string]ui.Color
	accent    bool
	accentTok map[string]ui.Color
}

// palKey is what colours depend on: when none of it changed the last result
// stands.
type palKey struct {
	apple palette.Cover
	dark  bool
	// rev is the theme's tokens (theme.Revision): the palette is checked
	// against the theme's colours, spot colour included.
	rev uint64
}

// palCache is the last colours built, kept on the page.
type palCache struct {
	key      palKey
	dominant []string
	built    bool
	colours  *colours // nil: the cover gives no colour
}

// colours returns the album's colours for the cover and the theme now, or
// nil when the cover gives none (a library-only album with no dominant
// colours: the sleeve keeps its plain wash).
func (a *album) colours(m *model) *colours {
	key := palKey{dark: theme.Dark(), rev: theme.Revision()}
	if m.page != nil {
		key.apple = palette.Cover(m.page.ArtworkColors)
	}
	pc := &a.pal
	if pc.built && pc.key == key && slices.Equal(pc.dominant, m.header.Colors) {
		return pc.colours
	}
	th := palette.Light
	if key.dark {
		th = palette.Dark
	}
	*pc = palCache{key: key, dominant: slices.Clone(m.header.Colors), built: true}
	pal, ok := palette.ForAlbum(&key.apple, m.header.Colors, th, true)
	if !ok {
		return nil
	}
	col := &colours{sleeve: pal.Sleeve, surface: ui.Hex(pal.Sleeve.Surface), sleeveTok: pal.Sleeve.Overrides()}
	if pal.Accent != nil {
		col.accent, col.accentTok = true, pal.Accent.Overrides()
	}
	pc.colours = col
	return col
}

// accentScope builds children with the cover's accent in amber's place (the
// play control, the playing row, the artist's link); without an accent it
// just builds them.
func (k *colours) accentScope(children func()) {
	if k == nil || !k.accent {
		children()
		return
	}
	theme.Scope(k.accentTok, children)
}
