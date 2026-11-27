package song

import (
	"context"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// creditTitle is a credit group's heading: Sona's title for the kinds it
// knows (Apple sends upper-case titles), else Apple's in sentence case with
// "&" as "and" (v0.2.0's credits.ts).
func creditTitle(g services.CreditGroup) string {
	switch g.Kind {
	case "performer":
		return i18n.T("Performers")
	case "composer-and-lyrics":
		return i18n.T("Composition and Lyrics")
	case "production-and-engineering":
		return i18n.T("Production and Engineering")
	}
	s := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(g.Title)), "&", "and")
	if s == "" {
		return i18n.T("Credits")
	}
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}

const (
	// creditBusyAfter is how long a credited name's lookup runs before the
	// name shows it: a lookup that answers sooner (an artist of the song, a
	// cached search, a quick network) shows nothing.
	creditBusyAfter = 300 * time.Millisecond
	// creditLookupMax bounds a lookup: past it the name opens the search.
	creditLookupMax = 10 * time.Second
)

// creditLinked reports whether a credited name can open anything: both the
// artist page and the artists' search it falls back to are pages here.
// Otherwise the names are plain text, as the byline's artist is without
// its page.
func (s *song) creditLinked() bool {
	return s.env.Routed("/artist/x") && s.env.Routed("/search/artist")
}

// creditArtistPath is the artist's page by ID.
func creditArtistPath(id string) string { return "/artist/" + url.PathEscape(id) }

// creditSearchPath is the search for an artist by name (the Artists group of
// the search: Apple's "see all" page of artists), where the user picks.
func creditSearchPath(name string) string {
	return "/search/artist?" + url.Values{"q": {name}}.Encode()
}

// creditName is a credited person's name: a link that opens their artist
// page when it is a link at all (creditLinked), else plain text. While a
// lookup for the name runs past creditBusyAfter the name shows it, a small
// spinner after it, and says so to assistive technology.
func (s *song) creditName(c *ui.Context, page services.CatalogSong, p services.Credit) {
	body := theme.Current().Type["body"]
	style := func(e ui.Element) ui.Element {
		return e.FontSize(body.Size).FixedLineHeight(body.LineHeight).TextColor(theme.C("ink")).SingleLine()
	}
	if !s.creditLinked() {
		style(ui.Text(c, p.Name))
		return
	}
	busy := s.creditBusy(c, p.Name)
	ui.Row(c.Key("name")).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
		// Outside a page ui.Link would hand its URL to the browser: it has
		// none, and the click opens the page.
		l := ui.Link(c.Key("link"), p.Name, "").Cursor(ui.CursorPointer).MinWidth(0).Shrink(1).
			Label(i18n.T("{name}, artist page", "name", p.Name))
		style(l)
		if l.Hovered() {
			l.Underline()
		}
		if kit.ClickOnce(l) {
			s.openCredit(page, p.Name)
		}
		if busy {
			sp := ui.Spinner(c.Key("busy")).Size(12, 12).Shrink(0).Role(ui.RoleNone)
			kit.AnnounceMounted(c, sp, i18n.T("Finding {name}…", "name", p.Name))
		}
	})
}

// creditBusy reports whether name's lookup has run past creditBusyAfter; if
// it has not, it asks for a frame at that time.
func (s *song) creditBusy(c *ui.Context, name string) bool {
	began, ok := s.page.asking[name]
	if !ok {
		return false
	}
	if left := s.busyAfter - time.Since(began); left > 0 {
		c.After(left)
		return false
	}
	return true
}

// openCredit opens what a click on a credited name opens. An artist of the
// song opens at once, a name looked up before on this page opens where it did,
// and any other is looked up (CatalogService.CreditedArtist) on a goroutine:
// the artist's page when the catalogue knows exactly one artist by that name,
// else, or when the lookup fails, the search for the name. Only the name
// clicked last opens, and only if the page is still the one showing.
func (s *song) openCredit(page services.CatalogSong, name string) {
	pg := &s.page
	if a, ok := services.OwnArtist(name, page.Artists); ok {
		s.env.Nav.Push(creditArtistPath(a.ID))
		return
	}
	pg.want = name
	if path, ok := pg.opens[name]; ok {
		s.env.Nav.Push(path)
		return
	}
	if _, ok := pg.asking[name]; ok {
		return
	}
	if pg.asking == nil {
		pg.asking = map[string]time.Time{}
	}
	pg.asking[name] = time.Now()
	id, own, here := pg.id, page.Artists, "/song/"+pg.id
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), creditLookupMax)
		defer cancel()
		artist, err := s.catalog.CreditedArtist(ctx, name, own)
		s.env.Post(func() {
			if pg.id != id {
				return
			}
			delete(pg.asking, name)
			path := creditSearchPath(name)
			if artist != "" {
				path = creditArtistPath(artist)
			}
			if err == nil {
				// A failure is not kept: the next click asks again.
				if pg.opens == nil {
					pg.opens = map[string]string{}
				}
				pg.opens[name] = path
			}
			if pg.want == name && s.env.Nav.Path() == here {
				s.env.Nav.Push(path)
			}
		})
	}()
}
