package commands

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

const (
	// libraryDelay is the pause after typing before the library is asked.
	libraryDelay = 150 * time.Millisecond
	// libraryPerKind is how many of each kind the lookup asks for.
	libraryPerKind = 4
	// maxLibrary is how many library entries show.
	maxLibrary = 8
	// minTerm is the shortest term the library is asked about.
	minTerm = 2
)

// follow moves the debounced term to the typed one once it has rested for
// libraryDelay.
func (m *commands) follow(c *ui.Context) {
	q := strings.TrimSpace(m.pal.Query())
	now := c.Now()
	if q != m.typed {
		m.typed, m.typedAt = q, now
	}
	if m.term == m.typed {
		return
	}
	if wait := libraryDelay - now.Sub(m.typedAt); wait > 0 {
		c.After(wait)
		return
	}
	m.term = m.typed
}

// available are the commands the palette offers: every shortcut but the
// ones kept out of it, whose When fails or that say they are unavailable.
func (m *commands) available() []module.BoundShortcut {
	if m.env.Shortcuts == nil {
		return nil
	}
	return slices.DeleteFunc(m.env.Shortcuts(), func(s module.BoundShortcut) bool {
		return s.NoPalette || s.Run == nil || (s.When != nil && !s.When()) || (s.Unavailable != nil && s.Unavailable() != "")
	})
}

// tileSize is the quiet tile an entry's icon sits in: the size of a library
// item's cover, so the leading column of a list that mixes both is one width.
const tileSize = 32

// iconTile draws an icon in the quiet tile.
func iconTile(icon string) func(c *ui.Context) {
	return func(c *ui.Context) {
		ui.Row(c.Key("tile")).Size(tileSize, tileSize).Shrink(0).Center().Radius(theme.Current().Radius["md"]).
			Background(theme.C("surface")).Role(ui.RoleNone).Children(func() {
			theme.Glyph(c, icon, 16, theme.GlyphOpts{Color: theme.C("ink-muted")})
		})
	}
}

// commandEntry is a command: its icon in a quiet tile, its label, its keys.
// detail is the second line, the group it belongs to, for a ranked list
// where no group heads it.
func commandEntry(s module.BoundShortcut, detail string) kit.PaletteEntry {
	icon := s.Icon
	if icon == "" {
		icon = "command" // the sidebar's go-tos and the like name none
	}
	return kit.PaletteEntry{Key: "c:" + s.ID, Label: s.Label(), Detail: detail, Keys: s.Keys, Run: s.Run, Leading: iconTile(icon)}
}

// paletteGroups are the groups in the order the palette opens on them: the
// places to go first, then what the player does, so Enter on opening goes
// somewhere rather than pausing the music.
var paletteGroups = []module.ShortcutGroup{"pages", "playback", "view", "navigation", "general"}

// sections is the palette's source: with nothing typed, every available
// command by group; with a query, the best matching commands, then library
// items from the local index, then a search of Apple Music for the text.
func (m *commands) sections(c *ui.Context) kit.PaletteSource {
	return func(query string) []kit.PaletteSection {
		cmds := m.available()
		q := strings.TrimSpace(query)
		if q == "" {
			var out []kit.PaletteSection
			for _, g := range paletteGroups {
				var entries []kit.PaletteEntry
				for _, s := range cmds {
					if s.Group == g {
						entries = append(entries, commandEntry(s, ""))
					}
				}
				if len(entries) > 0 {
					out = append(out, kit.PaletteSection{Title: groupTitle(g), Entries: entries})
				}
			}
			return out
		}
		var out []kit.PaletteSection
		ranked := kit.RankCommands(q, cmds,
			func(s module.BoundShortcut) string { return s.Label() },
			func(s module.BoundShortcut) string {
				if s.Keywords == nil {
					return ""
				}
				return s.Keywords()
			})
		if len(ranked) > 0 {
			entries := make([]kit.PaletteEntry, len(ranked))
			for i, s := range ranked {
				entries[i] = commandEntry(s, groupTitle(s.Group))
			}
			out = append(out, kit.PaletteSection{Title: i18n.T("Commands"), Entries: entries})
		}
		if items := m.libraryEntries(c, q); len(items) > 0 {
			out = append(out, kit.PaletteSection{Title: i18n.T("In your library"), Entries: items})
		}
		if m.env.Routed("/search") {
			out = append(out, kit.PaletteSection{Title: i18n.T("Apple Music"), Entries: []kit.PaletteEntry{{
				Key: "search", Label: i18n.T("Search Apple Music for “{term}”", "term", q), Leading: iconTile("search"),
				Run: func() { m.env.Nav.Push("/search?q=" + url.QueryEscape(q)) },
			}}})
		}
		return out
	}
}

// libraryEntries are the library items matching the term the typing has
// settled on: the first maxLibrary of artists, albums, playlists and songs.
// The last answer stays until the next one arrives.
func (m *commands) libraryEntries(c *ui.Context, q string) []kit.PaletteEntry {
	if m.lib == nil || utf8.RuneCountInString(m.term) < minTerm || q == "" {
		m.shown = nil
		return nil
	}
	term := m.term
	r := store.Use(m.env.Queries, store.LibrarySearch(term, libraryPerKind),
		func(ctx context.Context) (services.LibrarySearch, error) {
			return m.lib.Library(ctx, term, libraryPerKind)
		})
	if !r.HasData {
		return m.shown
	}
	var out []kit.PaletteEntry
	add := func(e kit.PaletteEntry) { out = append(out, e) }
	for _, a := range r.Data.Artists {
		add(m.itemEntry(c, "artist", a.ID, a.Name, a.Genre, a.Art, true, "/artist/"+url.PathEscape(a.ID), nil))
	}
	for _, a := range r.Data.Albums {
		add(m.itemEntry(c, "album", a.ID, a.Name, a.Artist, a.Art, false, "/album/"+url.PathEscape(a.ID), nil))
	}
	for _, p := range r.Data.Playlists {
		add(m.itemEntry(c, "playlist", p.ID, p.Name, p.Curator, p.Art, false, "/playlist/"+url.PathEscape(p.ID), nil))
	}
	for _, s := range r.Data.Songs {
		add(m.itemEntry(c, "song", s.ID, s.Name, s.Artist, s.Art, false, "", &s))
	}
	// A page another feature has not ported has no entry: nothing pretends.
	out = slices.DeleteFunc(out, func(e kit.PaletteEntry) bool { return e.Run == nil })
	if len(out) > maxLibrary {
		out = out[:maxLibrary]
	}
	m.shown = out
	return out
}

// itemEntry is one library item: cover, name and "Kind · subtitle". Picking
// opens its page (when a module registered it), or, for a song, plays it. A
// nil Run says there is nothing to do.
func (m *commands) itemEntry(c *ui.Context, kind, id, name, subtitle string, art services.Art, round bool, path string, song *services.Song) kit.PaletteEntry {
	e := kit.PaletteEntry{
		Key:    "i:" + kind + ":" + id,
		Label:  name,
		Detail: itemMeta(kind, subtitle),
		Leading: func(c *ui.Context) {
			kit.Artwork(c.Key("art"), m.env.Art, art.Small, 32, kit.ArtOpts{Round: round, Edge: true})
		},
	}
	switch {
	case song != nil && m.env.Queue != nil:
		s := *song
		e.Run = func() { m.playSong(s) }
	case path != "" && m.env.Routed(path):
		e.Run = func() { m.env.Nav.Push(path) }
	}
	return e
}

// itemMeta is "Song · Artist": the kind, then the item's own subtitle.
func itemMeta(kind, subtitle string) string {
	var label string
	switch kind {
	case "song":
		label = i18n.T("Song")
	case "album":
		label = i18n.T("Album")
	case "artist":
		label = i18n.T("Artist")
	case "playlist":
		label = i18n.T("Playlist")
	}
	return strings.Join(slices.DeleteFunc([]string{label, subtitle}, func(s string) bool { return s == "" }), " · ")
}

// playSong plays one song alone as the queue (v0.2.0's playSongs) through
// the shared queue commands: they say when it cannot be streamed, and ask
// before replacing a long queue the user built.
func (m *commands) playSong(s services.Song) {
	m.env.Queue.PlaySongs([]services.Song{s}, 0, services.PlayFrom{ID: s.Name})
}
