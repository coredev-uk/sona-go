package search

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

const (
	// hintDelay is the pause after typing before the suggestions follow it.
	hintDelay = 150 * time.Millisecond
	// hintStale is how long suggestions stay fresh.
	hintStale = 5 * time.Minute
	// maxRecent is how many recent entries the empty field lists.
	maxRecent = 6
	// maxLibraryHints is how many library matches the field suggests.
	maxLibraryHints = 6
	// fieldMaxWidth is the field's widest (max-w-[520px]).
	fieldMaxWidth = 520
)

// keyHints is the catalog suggestions of a partial term (v0.2.0's
// keys.catalogSearchHints).
func keyHints(term string) store.Key { return store.Key{"catalog", "search-hints", term} }

type optionKind uint8

const (
	optTerm   optionKind = iota // a completion of what is typed
	optItem                     // a top result that opens directly
	optRecent                   // a remembered term or opened item
)

// option is one row of the suggestion list.
type option struct {
	kind    optionKind
	key     string
	term    string // optTerm: the term searched
	display string // optTerm: the completion as shown
	item    services.Item
	entry   services.RecentSearch
}

// hintData is what the suggestions list: completions then items.
type hintData struct {
	terms []services.SearchTerm
	items []services.Item
}

// field is the top bar's search field (v0.2.0's SearchField.tsx): suggestions
// as you type (completions and top results that open directly), recent
// searches when empty, Enter to search. The search.focus shortcut focuses it
// from anywhere; Up and Down move through the list, Escape closes it, and
// Down with the list closed moves into the results.
type field struct {
	s *search

	// text is the draft; it belongs to the location and scope it was typed
	// at (at): any navigation hands the field back to the route's query.
	text    string
	at      string
	started bool
	routeQ  string // the route's q at the last build

	open   bool
	active int // the option the keys or the pointer are on; -1: the field
	// focus is FocusBind's field: 1 wants the input focused, 0 not (Escape
	// and picking a result blur it).
	focus      int
	wasFocused bool
	// selectReq selects the field's text once the focus the shortcut asked
	// for has arrived.
	selectReq bool
	// blur counts the builds until the field gives up the focus: picking a
	// result blurs it, but the list's overlay hands the focus back to the
	// field as it goes, so the blur waits for the overlay to be gone (a
	// frame later when the pick was a press: MyGo ends the press first).
	blur int

	// term is the typed text after the pause; pending the text waiting out
	// the pause, due at pendingAt.
	term      string
	pending   string
	pendingAt time.Time

	// last are the suggestions of the last answer, kept while the next load
	// (v0.2.0's placeholderData: (prev) => prev).
	last      hintData
	lastScope string

	// moved: the pointer moved over the list since the last frame (a
	// pointer resting under a list that appears must not take the choice
	// from the keys). reveal scrolls the active row into view, once, after
	// the keys moved it.
	moved, reveal bool
	// resultsReq: Down was pressed with the list closed on /search, this
	// frame: the page moves the focus into its first result.
	resultsReq bool
}

func newField(s *search) *field { return &field{s: s, active: -1} }

// focusAndSelect is the search.focus shortcut: focus the field and select
// its text (field.ts's focusSearchField).
func (f *field) focusAndSelect() {
	f.focus = 1
	f.selectReq = true
}

// takeResultsFocus reports, in the frame Down was pressed with the field's
// list closed on /search, that the focus should move into the page's first
// result (a result row, tile or list item); the page calls it while
// building, after its first result, and focuses that element on true.
func (f *field) takeResultsFocus() bool {
	r := f.resultsReq
	f.resultsReq = false
	return r
}

func (f *field) close() { f.open, f.active = false, -1 }

// build is the TopBarSearch slot.
func (f *field) build(c *ui.Context) {
	env := f.s.env
	f.resultsReq = false
	if f.blur > 0 {
		if f.blur--; f.blur == 0 {
			f.focus = 0
		}
		c.AnimationFrame()
	}
	path := env.Nav.Path()
	q, scope := searchParams(env.Nav.Location())
	f.routeQ = q
	if at := path + "\n" + q + "\n" + scope; at != f.at {
		f.at, f.text, f.active = at, q, -1
	}
	if !f.started {
		f.started = true
		f.term, f.pending = strings.TrimSpace(f.text), strings.TrimSpace(f.text)
	}

	placeholder := fieldPlaceholder(scope)
	var in ui.Element
	box := ui.Row(c.Key("search")).Role(ui.RoleSearch).FillWidth().MaxWidth(fieldMaxWidth).Height(32).
		Padding(0, 6, 0, 12).Gap(8).AlignItems(ui.Center).Cursor(ui.CursorText)
	box.Children(func() {
		theme.Glyph(c.Key("icon"), "search", 16, theme.GlyphOpts{Color: theme.C("ink-faint")}).Shrink(0)
		t := theme.Current().Type["body"]
		in = ui.TextInputBase(c.Key("input"), &f.text).Role(ui.RoleComboBox).Grow(1).MinWidth(0).
			FontSize(t.Size).FixedLineHeight(t.LineHeight).
			Placeholder(placeholder).PlaceholderColor(theme.C("ink-faint")).TextColor(theme.C("ink")).
			Label(i18n.T("Search")).FocusRing(false).FocusBind(&f.focus, 1).
			OnTextKey(func(k ui.TextKey) bool {
				if k.Key == ui.KeyTab {
					f.close() // Tab closes the list and the focus moves on
				}
				return false
			})
		if in.Changed() {
			f.open, f.active = true, -1
		}
		f.trailing(c, in)
	})

	focused := in.Focused()
	if focused && !f.wasFocused {
		f.open = true
	}
	if !focused && f.wasFocused {
		f.close() // blur out of the form closes the list
	}
	f.wasFocused = focused
	if in.Clicked() {
		f.open = true
	}
	if f.selectReq && focused {
		in.SetTextSelection(0, utf8.RuneCountInString(f.text))
		f.selectReq = false
	}

	typed := strings.TrimSpace(f.text)
	f.debounce(c, typed)
	hints := f.hints(f.term, scope, f.open)
	recent := f.s.recent()
	opts := f.options(typed, scope, hints, recent.Data)
	if f.active >= len(opts) {
		f.active = -1
	}

	down := in.Shortcut(0, ui.KeyDown)
	up := !down && in.Shortcut(0, ui.KeyUp)
	shown := f.open && len(opts) > 0
	switch {
	case down && !shown && path == searchPath:
		f.resultsReq = true // the list is closed: Down moves into the results
	case down || up:
		delta := 1
		if up {
			delta = -1
		}
		f.open = true
		f.active = moveActive(f.active, delta, len(opts))
		f.reveal = true
	}
	if in.Shortcut(0, ui.KeyEscape) {
		if shown {
			f.close()
		} else {
			f.focus = 0
		}
	}
	if in.Submitted() {
		if f.active >= 0 && f.active < len(opts) {
			f.pick(opts[f.active], scope)
		} else {
			f.search(f.text, scope)
		}
	}
	if box.Clicked() {
		in.Focus()
	}

	focusedFace := focused
	border, bg := ui.Transparent, theme.C("hover")
	if focusedFace {
		border, bg = theme.C("control-line"), theme.C("raised")
	}
	box.Radius(theme.Current().Radius["md"]).Background(bg).Border(1, border)
	if !theme.ReducedMotion() {
		m := theme.Current().Motion["feedback"]
		box.Transition(ui.ElementTransition{Colors: true, Duration: m.Duration, Ease: m.Ease})
	}

	shown = f.open && len(opts) > 0
	in.Expanded(shown)
	if shown {
		if row := f.popup(c, box, opts, typed, scope, hints); row.Valid() && f.active >= 0 {
			in.ActiveDescendant(row)
		}
	}
}

// fieldPlaceholder says where the field searches.
func fieldPlaceholder(scope string) string {
	if scope == scopeLibrary {
		return i18n.T("Search in library")
	}
	return i18n.T("Search in Apple Music")
}

// trailing is the field's right edge: the Clear button while it holds
// text, else the search.focus chord's hint.
func (f *field) trailing(c *ui.Context, in ui.Element) {
	if f.text != "" {
		clear := ui.ButtonBase(c.Key("clear")).Size(24, 24).Shrink(0).Radius(theme.Current().Radius["sm"]).
			Center().KeepFocus().Cursor(ui.CursorPointer).Label(i18n.T("Clear search"))
		ink := theme.C("ink-faint")
		if clear.Hovered() {
			clear.Background(theme.C("active"))
			ink = theme.C("ink")
		}
		clear.Children(func() {
			theme.Glyph(c.Key("x"), "x", 14, theme.GlyphOpts{Color: ink})
		})
		if clear.Clicked() {
			f.text, f.active = "", -1
			in.Focus()
		}
		return
	}
	// The first key search.focus answers to now; none (the user unbound it)
	// hints at nothing.
	keys := f.s.env.KeysOf("search.focus")
	if len(keys) == 0 {
		return
	}
	// The chord is for the eye only: it takes no pointer and no focus.
	ui.Row(c.Key("chord")).Shrink(0).Gap(2).AlignItems(ui.Center).Role(ui.RoleNone).IgnoreInput(true).Margin(0, 2, 0, 0).Children(func() {
		for i, part := range kit.ChordParts(keys[0]) {
			ui.Row(c.Key("cap:"+string(rune('0'+i)))).Height(20).MinWidth(20).PaddingX(4).Center().Role(ui.RoleNone).
				Radius(theme.Current().Radius["sm"]).Border(1, theme.C("control-line")).Children(func() {
				fieldText(c, part, "meta").FontWeight(500).TextColor(theme.C("ink-muted")).Role(ui.RoleNone)
			})
		}
	})
}

// debounce keeps term at the typed text after it has rested for hintDelay.
func (f *field) debounce(c *ui.Context, typed string) {
	now := c.Now()
	if typed != f.pending {
		f.pending, f.pendingAt = typed, now.Add(hintDelay)
	}
	if f.term == f.pending {
		return
	}
	if now.Before(f.pendingAt) {
		c.After(f.pendingAt.Sub(now))
		return
	}
	f.term = f.pending
}

// hints are the suggestions for term (v0.2.0's useHints): the catalog's
// completions and top results, or the library's matches (up to six, artists,
// albums, playlists, songs), cached five minutes, the previous answer kept
// while the next loads, no retry. enabled is whether the list is open.
func (f *field) hints(term, scope string, enabled bool) hintData {
	if scope != f.lastScope {
		f.last, f.lastScope = hintData{}, scope
	}
	if !enabled || term == "" {
		return f.last
	}
	api, queries := f.s.api, f.s.env.Queries
	if scope == scopeLibrary {
		r := store.Use(queries, keyLibrarySearch(term, 3), func(ctx context.Context) (services.LibrarySearch, error) {
			return api.Library(ctx, term, 3)
		}, store.Options{Stale: hintStale})
		if r.HasData {
			artists, albums, playlists, songs := libraryCards(r.Data)
			var items []services.Item
			for _, l := range [][]services.Item{artists, albums, playlists, songs} {
				items = append(items, l...)
			}
			f.last = hintData{items: items[:min(len(items), maxLibraryHints)]}
		}
		return f.last
	}
	r := store.Use(queries, keyHints(term), func(ctx context.Context) (services.SearchHints, error) {
		return api.Hints(ctx, term)
	}, store.Options{Stale: hintStale})
	if r.HasData {
		f.last = hintData{terms: r.Data.Terms, items: r.Data.Items}
	}
	return f.last
}

// options are the list's rows: completions then items for typed text, else
// the recent entries (terms of this scope, and opened items), up to six.
func (f *field) options(typed, scope string, h hintData, recent []services.RecentSearch) []option {
	var out []option
	if typed != "" {
		for _, t := range h.terms {
			out = append(out, option{kind: optTerm, key: "t:" + t.Term, term: t.Term, display: t.Display})
		}
		for _, it := range h.items {
			out = append(out, option{kind: optItem, key: "i:" + it.Kind + ":" + it.ID, item: it})
		}
		return out
	}
	for _, r := range recent {
		if r.Kind == services.RecentTerm && cleanScope(r.Scope) != scope {
			continue
		}
		out = append(out, option{kind: optRecent, key: "r:" + r.Key, entry: r})
		if len(out) == maxRecent {
			break
		}
	}
	return out
}

// search runs a search for q in scope (v0.2.0's search): the term is
// remembered (best effort, none when blank) and /search?q= opens, replacing
// the history entry when already on /search without a query.
func (f *field) search(q, scope string) {
	f.close()
	t := cleanTerm(q)
	f.s.rememberTerm(t, scope)
	f.text = t
	nav := f.s.env.Nav
	if nav.Path() == searchPath && f.routeQ == "" {
		nav.Replace(searchLocation(t, scope))
		return
	}
	nav.Push(searchLocation(t, scope))
}

// visit opens a page, when it is ported (the router has it).
func (f *field) visit(path string) {
	env := f.s.env
	if path != "" && env.HasRoute != nil && env.HasRoute(path) {
		env.Nav.Push(path)
	}
}

// openItem opens a top result: the field loses the focus, the item is
// remembered, and its page opens; a song, or anything without a page,
// plays or opens as it does elsewhere.
func (f *field) openItem(it services.Item) {
	f.close()
	f.blur = 3
	f.s.rememberItem(it, itemSource(it))
	switch p := itemPath(it); {
	case it.Kind == services.ItemSong:
		f.s.playSong(it)
	case p != "":
		f.visit(p)
	default:
		kit.OpenWithoutPage(it, f.s.env.ItemsCtx(nil))
	}
}

// pick chooses a row (v0.2.0's pick).
func (f *field) pick(o option, scope string) {
	switch o.kind {
	case optTerm:
		f.search(o.term, scope)
	case optItem:
		f.openItem(o.item)
	default:
		r := o.entry
		if r.Kind == services.RecentTerm {
			f.search(r.Term, cleanScope(r.Scope))
			return
		}
		f.close()
		path := recentPath(r)
		if path == "" {
			return // a library song has no page
		}
		f.s.rememberItem(recentItem(r), r.Source)
		f.visit(path)
	}
}

// popup draws the suggestion list below the field and returns the active
// row (the field keeps the focus, so the row carries it for assistive
// technology).
func (f *field) popup(c *ui.Context, box ui.Element, opts []option, typed, scope string, h hintData) ui.Element {
	title := i18n.T("Recently searched")
	if typed != "" {
		title = i18n.T("Suggestions")
	}
	hasTerms := len(h.terms) > 0
	firstItem := -1
	for i, o := range opts {
		if o.kind == optItem {
			firstItem = i
			break
		}
	}
	var activeRow ui.Element
	hovered := -1
	pick := -1
	w := box.Bounds().W
	_, wh := c.Size()
	ui.Overlay(c, func() {
		sh := theme.Current().Shadow["float"]
		panel := ui.Column(c.Key("suggestions")).AttachTo(box, ui.AnchorBottomStart, ui.AnchorTopStart).
			Margin(6, 0, 6, 0).Padding(6).Radius(theme.Current().Radius["lg"]).
			Background(theme.C("raised")).Border(1, theme.C("line")).
			Shadow(sh.X, sh.Y, sh.Blur, sh.Spread, sh.Color).KeepFocus()
		if w > 0 {
			panel.Width(w)
		}
		panel.Children(func() {
			list := ui.Scroll(c.Key("list")).Role(ui.RoleListBox).Label(title).FillWidth().MaxHeight(min(wh*0.7, 520) - 14)
			list.HandleInput(func(ev ui.InputEvent) bool {
				if ev.Kind == ui.InputPointerMove {
					f.moved = true
					return true // a frame follows, which reads the row under the pointer
				}
				return false
			})
			list.Children(func() {
				if typed == "" {
					f.groupLabel(c, "recent", "clock", title)
				}
				for i, o := range opts {
					if i == 0 && o.kind == optTerm {
						f.groupLabel(c, "terms", "search", title)
					}
					if i == firstItem {
						label, icon := i18n.T("Results"), "star"
						switch {
						case scope == scopeLibrary:
							label, icon = i18n.T("In your library"), "folder-closed"
						case hasTerms:
							label = i18n.T("Top results")
						}
						f.groupLabel(c, "items", icon, label)
					}
					row := f.row(c, o, i == f.active)
					if row.Hovered() {
						hovered = i
					}
					if row.Clicked() {
						pick = i
					}
					if i == f.active {
						activeRow = row
						if f.reveal {
							row.ScrollIntoView()
						}
					}
				}
			})
		})
	})
	f.reveal = false
	if f.moved && hovered >= 0 {
		f.active = hovered // the pointer moving over a row makes it active
	}
	f.moved = false
	if pick >= 0 {
		f.pick(opts[pick], scope)
	}
	return activeRow
}

// groupLabel is the heading of a run of rows: its icon and its name, muted,
// with room above to part it from the run before.
func (f *field) groupLabel(c *ui.Context, id, icon, text string) {
	ui.Row(c.Key("label:"+id)).AlignItems(ui.Center).Gap(6).Padding(10, 8, 4).Role(ui.RoleNone).Children(func() {
		theme.Glyph(c.Key("icon"), icon, 14, theme.GlyphOpts{Color: theme.C("ink-faint")}).Shrink(0)
		fieldText(c.Key("text"), text, "meta").FontWeight(600).TextColor(theme.C("ink-muted")).SingleLine().Role(ui.RoleNone)
	})
}

// kindIcon is the Lucide icon of a result's kind.
func kindIcon(it services.Item) string {
	switch it.Kind {
	case services.ItemSong:
		return "eighth-notes"
	case services.ItemAlbum:
		return "disc-3"
	case services.ItemArtist, services.ItemCurator, services.ItemProfile:
		return "user"
	case services.ItemPlaylist:
		return "list-music"
	case services.ItemStation:
		return "radio"
	case services.ItemMusicVideo:
		return "clapperboard"
	}
	return "star"
}

// row draws one option: min 36 tall, radius 6, the active one on the active
// tone (with the ring the palette's rows carry).
func (f *field) row(c *ui.Context, o option, active bool) ui.Element {
	var label, detail string
	switch {
	case o.kind == optTerm:
		label = o.display
	case o.kind == optRecent && o.entry.Kind == services.RecentTerm:
		label = o.entry.Term
	case o.kind == optRecent:
		it := recentItem(o.entry)
		label, detail = it.Name, itemMeta(it)
	default:
		label, detail = o.item.Name, itemMeta(o.item)
	}
	row := ui.Row(c.Key("opt:"+o.key)).MinHeight(36).FillWidth().Padding(4, 8).Gap(12).AlignItems(ui.Center).
		Radius(theme.Current().Radius["md"]).Role(ui.RoleOption).Label(label).Selected(active).Cursor(ui.CursorPointer)
	if detail != "" {
		row.Description(detail)
	}
	ring := ui.Transparent
	if active {
		row.Background(theme.C("active"))
		ring = theme.C("ring")
	}
	row.Border(2, ring)
	row.Children(func() {
		switch {
		case o.kind == optTerm:
			f.icon(c, "search")
			f.termText(c, o.display)
		case o.kind == optRecent && o.entry.Kind == services.RecentTerm:
			f.icon(c, "clock")
			fieldText(c.Key("txt"), o.entry.Term, "body").TextColor(theme.C("ink")).SingleLine().Grow(1).MinWidth(0)
		default:
			it := o.item
			if o.kind == optRecent {
				it = recentItem(o.entry)
			}
			kit.Artwork(c.Key("art"), f.s.env.Art, it.Art.Small, 32, kit.ArtOpts{Round: kit.ItemRound(it)})
			ui.Column(c.Key("txt")).Grow(1).MinWidth(0).Children(func() {
				fieldText(c.Key("name"), it.Name, "body").TextColor(theme.C("ink")).SingleLine()
				fieldText(c.Key("meta"), itemMeta(it), "secondary").TextColor(theme.C("ink-muted")).SingleLine()
			})
			f.kindMark(c, kindIcon(it), active)
		}
		if active && (o.kind == optTerm || (o.kind == optRecent && o.entry.Kind == services.RecentTerm)) {
			f.kindMark(c, "", true)
		}
	})
	return row
}

// kindMark is the row's trailing glyph: its kind's icon, faint, and on the
// active row the chevron that says Enter opens it. An empty icon with the
// row not active builds nothing.
func (f *field) kindMark(c *ui.Context, icon string, active bool) {
	switch {
	case active:
		theme.Glyph(c.Key("go"), "chevron-right", 16, theme.GlyphOpts{Color: theme.C("ink-muted")}).Shrink(0).Margin(0, 4, 0, 0)
	case icon != "":
		theme.Glyph(c.Key("kind"), icon, 14, theme.GlyphOpts{Color: theme.C("ink-faint")}).Shrink(0).Margin(0, 4, 0, 0)
	}
}

func (f *field) icon(c *ui.Context, name string) {
	theme.Glyph(c.Key("icon"), name, 16, theme.GlyphOpts{Color: theme.C("ink-faint")}).Shrink(0)
}

// termText is a completion with what was typed muted and the rest of it
// bold (splitMatch).
func (f *field) termText(c *ui.Context, display string) {
	before, match, after := splitMatch(display, strings.TrimSpace(f.text))
	ui.Row(c.Key("txt")).Grow(1).MinWidth(0).Clip().Children(func() {
		if before+match != "" {
			fieldText(c.Key("typed"), before+match, "body").TextColor(theme.C("ink-muted")).SingleLine().Shrink(0)
		}
		if after != "" {
			fieldText(c.Key("rest"), after, "body").FontWeight(600).TextColor(theme.C("ink")).SingleLine()
		}
	})
}

// fieldText is a text in one of the theme's type styles.
func fieldText(c *ui.Context, s, style string) ui.Element {
	t := theme.Current().Type[style]
	return ui.Text(c, s).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).LetterSpacing(t.Tracking)
}
