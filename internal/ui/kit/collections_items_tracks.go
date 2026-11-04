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
type TrackShelfState struct{ width itemsWidth }

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
// columns of Rows songs, reading down each column (v0.2.0's TrackShelf). Double
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
		width := st.width.track(c, grid)
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
			for i, song := range shown {
				// Filled down each column, so Tab follows the eye.
				play := func() {
					all := make([]services.Song, len(songs))
					for j, s := range songs {
						all[j] = *s
					}
					ctx.Players.PlaySongList(all, i, o.Title)
				}
				trackRow(c, *song, i, play, ctx, i/down, i%down)
			}
			if ctx.Menus != nil && !ctx.NoPopup {
				ctx.Menus.Popup(c)
			}
		})
	})
}

// trackRow is one song: a 40 DIP cover, the title with its explicit mark, the
// artist and the song's menu. play starts the queue from it.
func trackRow(c *ui.Context, song services.Song, i int, play func(), ctx ItemsCtx, col, rowAt int) {
	tk := theme.Current()
	current := ctx.Current != nil && ctx.Current(song)
	withheld := ctx.Restricted && song.Explicit
	disabled := !song.Playable || withheld

	row := ui.Row(c.Key(strconv.Itoa(i) + ":" + song.ID)).Height(trackRowH).MinWidth(0).Gap(12).PaddingX(8).
		Radius(tk.Radius["md"]).Focusable().Role(ui.RoleListItem).Label(song.Name).ColumnStart(col + 1).RowStart(rowAt + 1)
	hot := row.Hovered() || row.FocusWithin()
	if hot {
		row.Background(theme.C("hover"))
	}
	if disabled {
		row.Opacity(0.5)
	}
	if withheld {
		row.Tooltip(i18n.T("Explicit content is restricted on this account"))
	}
	if !theme.ReducedMotion() {
		m := tk.Motion["feedback"]
		row.Transition(ui.ElementTransition{Colors: true, Duration: m.Duration, Ease: m.Ease})
	}
	// Only the row itself takes the keys: the cover's play button and the
	// artist link are not tab stops.
	if !disabled && (row.DoubleClicked() || row.Shortcut(0, ui.KeyEnter)) {
		play()
	}

	row.Children(func() {
		ui.Box(c.Key("art")).Size(trackArt, trackArt).Shrink(0).Children(func() {
			itemsArtwork(c, ctx.Art, song.Art.Small, trackArt, ArtOpts{Edge: true})
			wash := theme.C("ground").Alpha(0.7)
			switch {
			case current:
				ui.Row(c.Key("now")).Absolute().Left(0).Top(0).Size(trackArt, trackArt).Center().Radius(tk.Radius["sm"]).
					Background(wash).IgnoreInput(true).Children(func() { EqualiserBars(c, ctx.Playing, ctx.Motion) })
			case !disabled:
				btn := ui.Row(c.Key("play")).Absolute().Left(0).Top(0).Size(trackArt, trackArt).Center().Radius(tk.Radius["sm"]).
					Background(wash).TextColor(theme.C("ink")).Role(ui.RoleButton).Label(i18n.T("Play {name}", "name", song.Name)).
					Cursor(ui.CursorPointer)
				if !row.Hovered() {
					btn.Opacity(0)
				}
				btn.Children(func() { ui.Icon(c, theme.Icon("play")).FontSize(16) })
				if btn.Clicked() {
					play()
				}
			}
		})

		ui.Column(c.Key("text")).Grow(1).MinWidth(0).Children(func() {
			label, sub := tk.Type["label"], tk.Type["sub"]
			ink := theme.C("ink")
			if current {
				ink = theme.C("play-ink")
			}
			ui.Row(c.Key("name")).Gap(6).AlignItems(ui.Center).MinWidth(0).Children(func() {
				ui.Text(c, song.Name).FontSize(label.Size).FixedLineHeight(label.LineHeight).FontWeight(int(label.Weight)).
					TextColor(ink).SingleLine().Shrink(1).MinWidth(0)
				if song.Explicit {
					ExplicitMark(c.Key("explicit"))
				}
			})
			artist := ui.Text(c, song.Artist).FontSize(sub.Size).FixedLineHeight(sub.LineHeight).
				TextColor(theme.C("ink-muted")).SingleLine().TextAlign(ui.Start)
			if song.ArtistID != "" {
				artist.Role(ui.RoleLink).Cursor(ui.CursorPointer)
				if artist.Hovered() {
					artist.Underline().TextColor(theme.C("ink"))
				}
				if artist.Clicked() {
					ctx.nav(itemsPage("/artist/", song.ArtistID))
				}
			}
		})

		if ctx.Menus != nil {
			ctx.Menus.Trigger(c.Key("more"), ActionTarget{Item: SongItem(song), Name: song.Name}, TriggerOpts{Visible: hot})
		}
	})
}
