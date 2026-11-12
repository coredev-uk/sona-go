package discover

import (
	"context"
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// listState is the state of a room's or a chart's list: its further pages, the
// songs of a track list and the lists' own state.
type listState struct {
	pager  pager
	tracks kit.TrackListState
	grid   kit.ItemGridState
	// songs are the songs of the items loaded, rebuilt when more arrive.
	songs      []services.Song
	songsCount int
	songsKey   string
	// songsFrom are the pages the songs were made of (their first items): a
	// refetch of the same size with other songs makes them again.
	songsFrom []*services.Item
}

// sources are where the loaded items live: the first page and each further
// one, by the address of their first item.
func (p *itemPages) sources() []*services.Item {
	out := make([]*services.Item, 0, 1+len(p.more))
	first := func(items []services.Item) *services.Item {
		if len(items) == 0 {
			return nil
		}
		return &items[0]
	}
	out = append(out, first(p.first))
	for k := range p.more {
		out = append(out, first(p.more[k].Items))
	}
	return out
}

// songList is the songs the loaded items hold, in order.
func (st *listState) songList(key string, pages *itemPages) []services.Song {
	from := pages.sources()
	if st.songsKey == key && st.songsCount == pages.count && slices.Equal(st.songsFrom, from) {
		return st.songs
	}
	st.songs, st.songsKey, st.songsCount, st.songsFrom = st.songs[:0], key, pages.count, from
	for i := range pages.count {
		if it := pages.at(i); it != nil && it.Song != nil {
			st.songs = append(st.songs, *it.Song)
		}
	}
	return st.songs
}

// roomPage is the state of the room page.
type roomPage struct {
	pg    pageView
	menus *kit.ActionMenus
	list  listState
	skel  measure
	// sort is the value of the Order select.
	sort string
}

// sortLabels are the room's orders in Sona's words; another order is shown as
// Apple names it.
func sortLabel(sort string) string {
	switch sort {
	case "featured":
		return i18n.T("Editors' order")
	case "alphabet":
		return i18n.T("A to Z")
	case "releaseDate":
		return i18n.T("Newest first")
	}
	return sort
}

// roomPage is an editorial section in full ("See all"), in pages of 100.
func (d *discover) roomPage(c *ui.Context, r *ui.Route) {
	st := &d.room
	id, sort := r.Param("id"), roomSearch(r)
	q := d.roomFirst(id, sort)
	key := "room/" + id + "|" + sort
	st.list.pager.reset(key)
	st.pg.body(c, key, func(pane *kit.Pane, _ float32) {
		switch {
		case q.HasData:
			d.roomContent(c, r, st, pane, id, sort, q.Data)
		case q.Err != nil:
			d.pageError(c, i18n.T("Couldn't load this list"), q.Err, q.Fetching, d.retry(keyRoom(id, sort)))
		default:
			d.masthead(c, "", "", true, nil)
			d.shelvesSkeleton(c, &st.skel, false)
		}
	})
}

// roomContent is a loaded room: its title and Order, and its items as a track
// list or a grid.
func (d *discover) roomContent(c *ui.Context, r *ui.Route, st *roomPage, pane *kit.Pane, id, sort string, room services.RoomPage) {
	r.Title(room.Title)
	ctx := d.env.ItemsCtx(st.menus)
	pages := d.morePages(&st.list.pager, keyRoom(id, sort), room.Items, room.Next, func(ctx context.Context, offset int) (services.ItemPage, error) {
		return d.api.RoomItems(ctx, id, sort, offset)
	})
	order := func() {
		if len(room.Sorts) < 2 {
			return
		}
		st.sort = room.Sort
		items := make([]kit.SelectItem, len(room.Sorts))
		for i, s := range room.Sorts {
			items[i] = kit.SelectOption(sortToken(s), sortLabel(s))
		}
		value := sortToken(st.sort)
		sel := kit.Select(c.Key("order"), &value, items, kit.SelectOpts{Small: true, A11yLabel: i18n.T("Order"), Width: 176})
		if sel.Changed() && value != sortToken(st.sort) {
			d.env.Nav.Replace(roomURL(id, sortFromToken(value)))
		}
	}
	songs := room.Kind == services.SectionTracks
	var list []services.Song
	if songs {
		list = st.list.songList(id+"|"+sort, &pages)
	}
	if songs {
		d.masthead(c, room.Title, "", false, func() {
			d.playButtons(c, room.Title, list)
			order()
		})
	} else {
		d.masthead(c, room.Title, "", false, order)
	}
	d.itemList(c, &st.list, pane, &pages, list, ctx, listOpts{title: room.Title, songs: songs, placeholders: 12, near: 24, min: 160, menus: st.menus})
}

// sortToken maps Apple's default order ("") to a value a select can hold.
func sortToken(sort string) string {
	if sort == "" {
		return "_"
	}
	return sort
}

func sortFromToken(v string) string {
	if v == "_" {
		return ""
	}
	return v
}

// playButtons are Play and Shuffle for a list of songs: disabled when none is
// playable; Play plays them from the first, Shuffle from a random one, with the
// title as the queue's source.
func (d *discover) playButtons(c *ui.Context, title string, songs []services.Song) {
	disabled := true
	for _, s := range songs {
		if s.Playable {
			disabled = false
			break
		}
	}
	play, shuffle := kit.PlayButtons(c, kit.PlayButtonsOpts{Label: title, Shuffle: true, Disabled: disabled})
	if play {
		d.env.Queue.PlaySongs(songs, 0, services.PlayFrom{ID: title})
	}
	if shuffle {
		d.env.Queue.ShuffleSongs(songs, services.PlayFrom{ID: title})
	}
}

// listOpts are what a list of items needs of the page that shows it.
type listOpts struct {
	title string
	// songs: the items are songs, listed as a track table.
	songs bool
	// placeholders are the loading tiles a grid shows while more remain, and
	// near how close to the loaded end the list asks for the next page.
	placeholders, near int
	min                float32
	menus              *kit.ActionMenus
}

// itemList is a room's or a chart's items: a track list with the row numbers,
// artwork, artist and album for songs, else the item grid; a placeholder row
// or tiles while more pages are to come, and the next page asked for as the
// list scrolls near its end.
func (d *discover) itemList(c *ui.Context, st *listState, pane *kit.Pane, pages *itemPages, songs []services.Song, ctx kit.ItemsCtx, o listOpts) {
	onRange := func(_, end int) {
		before := st.pager.want
		st.pager.ask(pages, end, o.near)
		if st.pager.want != before {
			c.Invalidate()
		}
	}
	if !o.songs {
		count := pages.count
		if pages.hasMore {
			count += o.placeholders
		}
		kit.ItemGrid(c, &st.grid, count, func(i int) *services.Item { return pages.at(i) }, ctx,
			kit.ItemGridOpts{Label: o.title, Min: o.min, Pane: pane, OnRange: onRange})
		return
	}
	count := len(songs)
	if pages.hasMore {
		count++
	}
	// The ranges are in rows of songs; the pages ask in items.
	songRange := func(_, end int) { onRange(0, end+pages.count-len(songs)) }
	to := d.pageOpener
	kit.TrackList(c, &st.tracks, count, func(i int) *services.Song {
		if i < len(songs) {
			return &songs[i]
		}
		return nil
	}, kit.TrackListOpts{
		Label:   o.title,
		Columns: kit.TrackColumns{RowNumbers: true, Artwork: true, Artist: true, Album: true},
		OnPlay:  func(i int) { d.env.Queue.PlaySongs(songs, i, services.PlayFrom{ID: o.title}) },
		OnRange: songRange,
		Pane:    pane, Menus: o.menus, Art: ctx.Art, Motion: ctx.Motion,
		Current: ctx.Current, Playing: ctx.Playing, Restricted: ctx.Restricted,
		OpenArtist: to("/artist/"), OpenAlbum: to("/album/"),
	})
}

// seeAllLink is a shelf heading's "See all" control, as kit.Shelf draws it, for
// the headings this feature builds itself.
func seeAllLink(c *ui.Context, key string, open func()) {
	tk := theme.Current()
	b := ui.ButtonBase(c.Key(key)).Role(ui.RoleLink).Height(28).PaddingX(6).MarginEnd(-4).Gap(2).Radius(tk.Radius["md"]).Center()
	bg, fg := ui.Transparent, theme.C("ink-muted")
	if b.Hovered() {
		bg, fg = theme.C("hover"), theme.C("ink")
	}
	b.Background(bg).TextColor(fg)
	b.Children(func() {
		theme.Text(c, i18n.T("See all"), "sub").SingleLine()
		theme.Glyph(c, "chevron-right", 14, theme.GlyphOpts{Color: fg, Inherit: true})
	})
	if b.Clicked() {
		open()
	}
}
