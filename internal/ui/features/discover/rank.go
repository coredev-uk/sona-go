package discover

import (
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Geometry of the ranked rows, in DIPs.
const (
	rankRowH = 56
	rankCell = 48 // the rank's cell: three digits of the numeral, or the play button
	rankArt  = 40
	rankDur  = 56
	// rankAlbumAt is the narrowest page (the scroll pane) that shows the album
	// column.
	rankAlbumAt = 900
)

// rankState is a ranked list's keyboard navigation: one tab stop, the arrows
// move it.
type rankState struct{ nav kit.GridNav }

// rankOpts are what a ranked list takes of the page that shows it.
type rankOpts struct {
	// label names the list: "Top songs".
	label string
	// album shows the album column.
	album bool
	// onPlay plays from row i with the whole list as the queue.
	onPlay func(i int)
	// onRange reports the rows built (all of them: the list is not virtual).
	onRange func(start, end int)
	// count is the rows, songs the song of row i (nil: not loaded yet).
	count int
	songs func(i int) *services.Song

	menus                 *kit.ActionMenus
	art                   *kit.ArtLoader
	motion                *kit.MotionPolicy
	current               func(*services.Song) bool
	playing, restricted   bool
	openArtist, openAlbum func(id string)
}

// rankList is a chart's songs, ranked: the rank leads each row as a large
// numeral (the order is the information), then the cover, the title with its
// artist, the album and the length. The song playing shows the equaliser in
// the numeral's place, and hovering or focusing a row shows its play button
// there. Double click or Enter plays from the row with the list as the queue;
// the arrows move the one tab stop; the item menu opens on a right click, the
// menu key or the more button. A row not loaded yet (o.songs gives nil) is a
// placeholder keeping its rank. The list is built whole, not by row: a chart
// has at most a few hundred songs.
func (d *discover) rankList(c *ui.Context, st *rankState, o rankOpts) {
	count := o.count
	st.nav.Count, st.nav.Columns = count, 1
	if f := st.nav.Focus(); count > 0 && f >= count {
		st.nav.SetFocus(count - 1)
	}
	st.nav.Page, st.nav.Ends, st.nav.ScrollTo = 8, true, nil
	ui.Column(c.Key("ranks")).FillWidth().Margin(0, -8, 0, -8).Role(ui.RoleList).Label(o.label).Children(func() {
		for i := range count {
			if s := o.songs(i); s != nil {
				d.rankRow(c, st, i, s, o)
			} else {
				rankPlaceholder(c, i, o.album)
			}
		}
		st.nav.Tick(c)
	})
	if o.onRange != nil {
		o.onRange(0, count)
	}
}

// rankNumeral is a rank's numeral: 24/650 in tabular figures, faint.
func rankNumeral(c *ui.Context, i int, col ui.Color) ui.Element {
	return text(c, strconv.Itoa(i+1), "title").TextColor(col).FontFeatures("tnum").SingleLine()
}

func (d *discover) rankRow(c *ui.Context, st *rankState, i int, s *services.Song, o rankOpts) {
	tk := theme.Current()
	withheld := o.restricted && s.Explicit
	disabled := !s.Playable || withheld
	current := o.current != nil && o.current(s)
	target := kit.ActionTarget{Item: kit.SongItem(*s), Name: s.Name, Key: "rank:" + strconv.Itoa(i)}
	play := func() {
		if o.onPlay != nil && !disabled {
			o.onPlay(i)
		}
	}

	row := ui.Row(c.Key(strconv.Itoa(i) + ":" + s.ID)).Height(rankRowH).Shrink(0).PaddingX(8).Gap(12).AlignItems(ui.Center).
		Radius(tk.Radius["md"]).Role(ui.RoleListItem).Label(s.Name)
	hovered := row.Hovered()
	hot := hovered || row.FocusWithin()
	if hot {
		row.Background(theme.C("hover"))
	}
	if disabled {
		row.Opacity(0.5)
	}
	if withheld {
		row.AccessDisabled(true).Description(i18n.T("Explicit content is restricted on this account")).
			Tooltip(i18n.T("Explicit content is restricted on this account"))
	}
	if i == st.nav.Focus() {
		row.Focusable()
	}
	if !theme.ReducedMotion() {
		m := tk.Motion["feedback"]
		row.Transition(ui.ElementTransition{Colors: true, Duration: m.Duration, Ease: m.Ease})
	}
	faint, ink := theme.C("ink-faint"), theme.C("ink")
	row.Children(func() {
		ui.Row(c.Key("rank")).Width(rankCell).Shrink(0).AlignItems(ui.Center).Children(func() {
			if current {
				kit.EqualiserBars(c, o.playing, o.motion)
				return
			}
			// The play button is built at rest too, over the numeral and
			// invisible, so that assistive technology finds and presses it
			// (the pointer and Tab ignore it until the row is hovered).
			shown := hovered && !disabled
			if !disabled {
				b := ui.ButtonBase(c.Key("play")).Size(32, 32).Radius(tk.Radius["md"]).Center().Label(i18n.T("Play {name}", "name", s.Name))
				if !shown {
					b.Absolute().Left(0).IgnoreInput(true).Opacity(0)
				}
				b.Children(func() { theme.Glyph(c, "play", 16, theme.GlyphOpts{Color: ink}) })
				if b.Clicked() {
					play()
				}
			}
			if !shown {
				rankNumeral(c, i, faint)
			}
		})
		kit.Artwork(c, o.art, s.Art.Small, rankArt, kit.ArtOpts{Edge: true})
		ui.Column(c.Key("text")).Grow(3).Basis(0).MinWidth(0).Justify(ui.Center).Children(func() {
			label, sub := tk.Type["label"], tk.Type["secondary"]
			title := ink
			if current {
				title = theme.C("play-ink")
			}
			ui.Row(c.Key("name")).Gap(6).AlignItems(ui.Center).MinWidth(0).Children(func() {
				ui.Text(c, s.Name).FontSize(label.Size).FixedLineHeight(label.LineHeight).FontWeight(int(label.Weight)).
					TextColor(title).SingleLine().Shrink(1).MinWidth(0)
				if s.Explicit {
					kit.ExplicitMark(c.Key("explicit"))
				}
			})
			rankLink(c, s.Artist, s.ArtistID, sub, o.openArtist)
		})
		if o.album {
			ui.Row(c.Key("album")).Grow(2).Basis(0).MinWidth(0).AlignItems(ui.Center).Children(func() {
				rankLink(c, s.Album, s.AlbumID, tk.Type["secondary"], o.openAlbum)
			})
		}
		dur := "—"
		if s.DurationMs > 0 {
			dur = i18n.FormatDuration(time.Duration(s.DurationMs) * time.Millisecond)
		}
		ui.Row(c.Key("dur")).Width(rankDur).Shrink(0).Justify(ui.End).Children(func() {
			text(c, dur, "secondary").TextColor(faint).FontFeatures("tnum").SingleLine()
		})
		if o.menus != nil {
			o.menus.Trigger(c.Key("more"), target, kit.TriggerOpts{Visible: hot})
		}
	})

	st.nav.Claim(row, i)
	if row.Clicked() {
		st.nav.SetFocus(i)
		if !row.Focused() {
			st.nav.Want(i)
		}
	}
	if row.DoubleClicked() {
		play()
	}
	if row.Focused() && !st.nav.Keys(row, i, nil) && row.Shortcut(0, ui.KeyEnter) {
		play()
	}
	if o.menus != nil {
		o.menus.Row(row, target)
	}
}

// rankLink is a song's artist or album: a link to its page when the song names
// one and the page exists (underlined while hovered), plain text otherwise.
func rankLink(c *ui.Context, name, id string, style theme.TypeStyle, open func(id string)) {
	t := ui.Text(c, name).FontSize(style.Size).FixedLineHeight(style.LineHeight).TextColor(theme.C("ink-muted")).
		SingleLine().MinWidth(0).TextAlign(ui.Start)
	if id == "" || open == nil {
		return
	}
	t.Role(ui.RoleLink)
	if t.Hovered() {
		t.Underline().TextColor(theme.C("ink")).Cursor(ui.CursorPointer)
	}
	if t.Clicked() {
		open(id)
	}
}

// rankPlaceholder is a row whose page is still loading: its rank and flat bars
// where the text goes.
func rankPlaceholder(c *ui.Context, i int, album bool) {
	tk := theme.Current()
	ui.Row(c.Key("loading-" + strconv.Itoa(i))).Height(rankRowH).Shrink(0).PaddingX(8).Gap(12).AlignItems(ui.Center).
		Role(ui.RoleListItem).Label(i18n.T("Loading")).Children(func() {
		ui.Row(c).Width(rankCell).Shrink(0).Children(func() { rankNumeral(c, i, theme.C("ink-faint")) })
		ui.Box(c).Size(rankArt, rankArt).Shrink(0).Radius(tk.Radius["sm"]).Background(theme.C("hover"))
		ui.Column(c).Grow(3).Basis(0).MinWidth(0).Children(func() {
			ui.Box(c).WidthPercent(45).Height(14).Radius(tk.Radius["sm"]).Background(theme.C("hover"))
			ui.Box(c).WidthPercent(30).Height(12).Margin(6, 0, 0, 0).Radius(tk.Radius["sm"]).Background(theme.C("hover"))
		})
		if album {
			ui.Column(c).Grow(2).Basis(0).MinWidth(0).Children(func() {
				ui.Box(c).WidthPercent(50).Height(12).Radius(tk.Radius["sm"]).Background(theme.C("hover"))
			})
		}
		ui.Box(c).Size(rankDur, 1).Shrink(0)
	})
}
