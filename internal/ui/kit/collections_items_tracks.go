package kit

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Geometry of the track shelf (v0.2.0's TrackShelf), in DIPs.
const (
	trackMin  = 280
	trackGapX = 24
	trackRowH = 56
	trackArt  = 40
)

// TrackShelfState is the state of a TrackShelf: the width its columns follow.
type TrackShelfState struct {
	width WidthTracker
	// Sel is the selected rows: a click or Space selects, as a TrackList row
	// does (Shift+click and Ctrl+click too, Ctrl+Space toggles).
	Sel Selection
}

// TrackShelfOpts are the options of TrackShelf.
type TrackShelfOpts struct {
	// Title heads the shelf, names its list and labels the queue the songs
	// play as.
	Title string
	// First leaves out the 40 DIPs above: the first shelf of a page.
	First bool
	// More is the section's "See all" page, as a route path; "" for none.
	More string
	// Rows is the rows per column (default 4).
	Rows int
}

// TrackShelf is songs as a compact block of rows that fills the width in
// columns of Rows songs, reading down each column (v0.2.0's TrackShelf). A click
// or Space selects the row, as in a TrackList. Double
// click, Enter or the cover's play button plays from that song with the shelf's
// songs as the queue, labelled by the title. A song that cannot play (or is
// withheld from a restricted account) is dimmed and does nothing. The song
// playing (ctx.Current) shows the equaliser over its cover.
func TrackShelf(c *ui.Context, st *TrackShelfState, items []services.Item, ctx ItemsCtx, o TrackShelfOpts) {
	rows := o.Rows
	if rows <= 0 {
		rows = 4
	}
	so := ShelfOpts{Title: o.Title, First: o.First}
	if o.More != "" {
		so.More = func() { ctx.nav(o.More) }
	}
	Shelf(c, so, func() {
		grid := ui.Grid(c.Key("kit.tracks")).GapX(trackGapX).Role(ui.RoleList).Label(o.Title)
		width := st.width.Track(c, grid)
		cols := 1
		if width > 0 {
			cols, _ = CoverColumns(width, trackMin)
			grid.Columns(cols)
		}
		grid.Children(func() {
			if width <= 0 {
				ui.Box(c).Height(1) // the first frame measures; the next builds the rows
				return
			}
			songs := make([]*services.Song, 0, len(items))
			for i := range items {
				if items[i].Song != nil {
					songs = append(songs, items[i].Song)
				}
			}
			shown := songs[:min(len(songs), cols*rows)]
			down := min(rows, len(shown))
			st.Sel.Hold = collectionsHold(ctx.Menus, nil)
			st.Sel.Rows(KeysOf(len(shown), func(i int) string { return shown[i].ID }), nil)
			for i, song := range shown {
				// Filled down each column, so Tab follows the eye.
				play := func() {
					all := make([]services.Song, len(songs))
					for j, s := range songs {
						all[j] = *s
					}
					ctx.Players.PlaySongList(all, i, o.Title)
				}
				trackRow(c, &st.Sel, *song, i, play, ctx, i/down, i%down)
			}
			if ctx.Menus != nil && !ctx.NoPopup {
				ctx.Menus.Popup(c)
			}
		})
		st.Sel.OutsidePress(grid)
	})
}

// trackRow is one song: a 40 DIP cover, the title with its explicit mark, the
// artist and the song's menu. play starts the queue from it.
func trackRow(c *ui.Context, sel *Selection, song services.Song, i int, play func(), ctx ItemsCtx, col, rowAt int) {
	tk := theme.Current()
	current := SongIsCurrent(&song, ctx.Current)
	withheld := ctx.Restricted && song.Explicit
	disabled := !song.Playable || withheld

	name := song.Name
	if song.IsUpload() {
		name = UploadedLabel(name)
	}
	row := ui.Row(c.Key(strconv.Itoa(i) + ":" + song.ID)).Height(trackRowH).MinWidth(0).Gap(12).PaddingX(8).
		Radius(tk.Radius["md"]).Focusable().Role(ui.RoleListItem).Label(name).ColumnStart(col + 1).RowStart(rowAt + 1)
	hot := HoverRow(row)
	if run := sel.Run(i); run != RunNone {
		row.Draw(func(p *ui.Painter, r ui.Rect) { p.Fill(r, theme.C("ink").Alpha(0.12), tk.Radius["md"]) })
		row.Checked(true)
	}
	if disabled {
		row.Opacity(0.5)
	}
	if withheld {
		row.Tooltip(i18n.T("Explicit content is restricted on this account"))
	}
	// Only the row itself takes the keys: the cover's play button and the
	// artist link are not tab stops.
	if row.Clicked() {
		sel.Click(i, ModsOf(row.ClickModifiers()))
	}
	sel.Keys(row, i)
	if !disabled && (row.DoubleClicked() || row.Shortcut(0, ui.KeyEnter)) {
		play()
	}

	row.Children(func() {
		ui.Box(c.Key("art")).Size(trackArt, trackArt).Shrink(0).Children(func() {
			itemsArtwork(c, ctx.Art, song.Art.Small, trackArt, ArtOpts{Edge: true})
			wash := ArtworkWash(false)
			switch {
			case current:
				ui.Row(c.Key("now")).Absolute().Left(0).Top(0).Size(trackArt, trackArt).Center().Radius(tk.Radius["sm"]).
					Background(wash).IgnoreInput(true).Children(func() { EqualiserBars(c, ctx.Playing, ctx.Motion) })
			case !disabled:
				btn := ui.Row(c.Key("play")).Absolute().Left(0).Top(0).Size(trackArt, trackArt).Center().Radius(tk.Radius["sm"]).
					Background(wash).TextColor(theme.C("ink")).Role(ui.RoleButton).Label(i18n.T("Play {name}", "name", name)).
					Cursor(ui.CursorPointer)
				// Hover or keyboard focus shows it, over the press duration:
				// the button stays in the tree at rest.
				Fade(btn, row.Hovered() || row.FocusWithin())
				btn.Children(func() { PlayGlyph(c, 16, theme.GlyphOpts{Color: theme.C("ink"), Inherit: true}) })
				if btn.Clicked() {
					play()
				}
			}
		})

		ui.Column(c.Key("text")).Grow(1).MinWidth(0).Children(func() {
			sub := tk.Type["sub"]
			ink := theme.C("ink")
			if current {
				ink = theme.C("play-ink")
			}
			ui.Row(c.Key("name")).Gap(6).AlignItems(ui.Center).MinWidth(0).Children(func() {
				theme.StyleText(ui.Text(c, song.Name), "label").
					TextColor(ink).SingleLine().Shrink(1).MinWidth(0)
				if song.Explicit {
					ExplicitMark(c.Key("explicit"))
				}
				if song.IsUpload() {
					UploadedBadge(c.Key("uploaded"))
				}
			})
			var open func(id string)
			if ctx.HasRoute == nil || ctx.HasRoute("/artist/x") {
				open = func(id string) { ctx.nav(itemsPage("/artist/", id)) }
			}
			ArtistLinks(c, ctx.Artists.OfSong(song), song.Artist, theme.C("ink-muted"), ArtistLinksOpts{
				Open: open, Static: true, Hover: theme.C("ink"),
				Style: func(e ui.Element) {
					e.FontSize(sub.Size).FixedLineHeight(sub.LineHeight).SingleLine().TextAlign(ui.Start)
				},
			})
		})

		if ctx.Menus != nil {
			ctx.Menus.Trigger(c.Key("more"), row, ActionTarget{Item: SongItem(song), Name: song.Name}, TriggerOpts{Visible: hot})
		}
	})
}
