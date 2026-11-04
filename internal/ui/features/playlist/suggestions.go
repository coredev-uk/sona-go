package playlist

import (
	"context"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The suggestions' geometry (PlaylistPage.tsx's grid of
// repeat(auto-fill, minmax(300px, 1fr)) with 24 between columns), in DIPs.
const (
	suggestionMin    = 300
	suggestionGap    = 24
	suggestionRow    = 56
	suggestionArt    = 40
	suggestionLoadN  = 6
	suggestionPadX   = 8
	suggestionGapRow = 12
)

// suggestState is the shelf's state: the songs that show "Added" and the
// width of the last frame, which the columns follow.
type suggestState struct {
	added map[string]bool
	width float32
}

// suggestionsShelf is Suggested songs: the songs Apple suggests for a
// library playlist, each playing on click and joining the playlist with its
// add button. It is hidden when the suggestions failed or came back empty.
func (p *playlists) suggestionsShelf(c *ui.Context, id, playlist string) {
	res := p.suggestions(id)
	loading := !res.HasData && res.Err == nil
	if res.Err != nil || (!loading && len(res.Data) == 0) {
		return
	}
	st := &p.page.suggest
	if st.added == nil {
		st.added = map[string]bool{}
	}
	title := i18n.T("Suggested songs")
	kit.Shelf(c, kit.ShelfOpts{Title: title}, func() {
		grid := ui.Column(c.Key("suggestions")).FillWidth().Gap(0).Role(ui.RoleList).Label(title)
		grid.Draw(func(_ *ui.Painter, r ui.Rect) {
			if r.W != st.width {
				st.width = r.W
				c.Invalidate() // the columns follow the width
			}
		})
		if w := grid.Bounds().W; w > 0 && st.width == 0 {
			st.width = w
		}
		cols, itemW := 1, float32(suggestionMin)
		if st.width > 0 {
			cols = max(1, int((st.width+suggestionGap)/(suggestionMin+suggestionGap)))
			itemW = (st.width - suggestionGap*float32(cols-1)) / float32(cols)
		}
		count := len(res.Data)
		if loading {
			count = suggestionLoadN
		}
		grid.Children(func() {
			for first := 0; first < count; first += cols {
				ui.Row(c.Key("row-" + strconv.Itoa(first/cols))).Gap(suggestionGap).Children(func() {
					for i := first; i < min(count, first+cols); i++ {
						if loading {
							p.suggestionSkeleton(c, i, itemW)
							continue
						}
						p.suggestionRow(c, id, playlist, res.Data[i], itemW)
					}
				})
			}
		})
	})
}

// suggestionSkeleton is a loading row: flat bars at the real sizes.
func (p *playlists) suggestionSkeleton(c *ui.Context, i int, w float32) {
	hover := theme.C("hover")
	r := theme.Current().Radius["sm"]
	ui.Row(c.Key("loading-" + strconv.Itoa(i))).Width(w).Height(suggestionRow).AlignItems(ui.Center).Gap(suggestionGapRow).
		PaddingX(suggestionPadX).Role(ui.RoleNone).Children(func() {
		ui.Box(c).Size(suggestionArt, suggestionArt).Shrink(0).Radius(r).Background(hover)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Box(c).WidthPercent(50).Height(14).Radius(r).Background(hover)
			ui.Box(c).WidthPercent(33).Height(12).Margin(6, 0, 0, 0).Radius(r).Background(hover)
		})
	})
}

// suggestionRow is one suggested song: the art plays it, the add button puts
// it in the playlist.
func (p *playlists) suggestionRow(c *ui.Context, id, playlist string, s services.Song, w float32) {
	tk := theme.Current()
	st := &p.page.suggest
	row := ui.Row(c.Key(s.ID)).Width(w).Height(suggestionRow).AlignItems(ui.Center).Gap(suggestionGapRow).PaddingX(suggestionPadX).
		Radius(tk.Radius["md"]).Role(ui.RoleListItem)
	if row.Hovered() {
		row.Background(theme.C("hover"))
	}
	row.Children(func() {
		play := ui.ButtonBase(c.Key("play")).Shrink(0).Radius(tk.Radius["sm"]).Label(i18n.T("Play {songName}", "songName", s.Name))
		play.Children(func() {
			kit.Artwork(c, p.env.Art, s.Art.Small, suggestionArt, kit.ArtOpts{})
		})
		if play.Clicked() {
			go p.playSongs(context.Background(), []services.Song{s}, 0, services.PlayFrom{ID: id}, false)
		}

		ui.Column(c.Key("text")).Grow(1).Basis(0).MinWidth(0).Children(func() {
			uiT, subT := tk.Type["ui"], tk.Type["sub"]
			ui.Text(c, s.Name).FontSize(uiT.Size).FixedLineHeight(uiT.LineHeight).TextColor(theme.C("ink")).SingleLine()
			line := s.Artist
			if s.DurationMs > 0 {
				line += " · " + i18n.FormatDuration(time.Duration(s.DurationMs)*time.Millisecond)
			}
			ui.Text(c, line).FontSize(subT.Size).FixedLineHeight(subT.LineHeight).TextColor(theme.C("ink-muted")).SingleLine()
		})

		if st.added[s.ID] {
			subT := tk.Type["sub"]
			ui.Row(c.Key("added")).Height(32).PaddingX(8).Gap(4).AlignItems(ui.Center).Shrink(0).Children(func() {
				ui.Icon(c, theme.Icon("check")).FontSize(16).TextColor(theme.C("ink-muted"))
				ui.Text(c, i18n.T("Added")).FontSize(subT.Size).FixedLineHeight(subT.LineHeight).TextColor(theme.C("ink-muted")).SingleLine()
			})
			return
		}
		add := kit.Button(c.Key("add"), i18n.T("Add"), kit.ButtonOpts{
			Variant: kit.Ghost, Size: kit.SizeSM, Icon: "plus", Muted: true,
			A11yLabel: i18n.T("Add {songName} to {playlistName}", "songName", s.Name, "playlistName", playlist),
		})
		if add.Clicked() {
			p.addSuggestion(id, playlist, s)
		}
	})
}

// addSuggestion puts a suggested song in the playlist: it shows "Added" at
// once, the add is queued with the playlist's other edits, and a failure
// puts the button back with a toast.
func (p *playlists) addSuggestion(id, playlist string, s services.Song) {
	added := p.page.suggest.added
	added[s.ID] = true
	items := []services.PlaylistItem{{Kind: "song", ID: s.ID, Source: s.Source}}
	p.edits.serial(id, func() (services.PlaylistDetail, error) {
		return p.api.Add(context.Background(), id, items)
	}, func(d services.PlaylistDetail, err error, _ bool) {
		p.env.Post(func() {
			if err != nil {
				delete(added, s.ID)
				kit.ToastErrorf(i18n.T("Couldn't add “{songName}” to “{playlistName}”", "songName", s.Name, "playlistName", playlist), failText(err))
				// An earlier add that did succeed was never shown in the
				// list: SQLite has it.
				if !p.edits.editing(id) {
					p.env.Queries.Invalidate(keyDetail(id))
				}
				return
			}
			// Later edits are still on their way: their optimistic state
			// stays, and their answer holds this song too.
			if !p.edits.editing(id) {
				store.Set(p.env.Queries, keyDetail(id), d)
			}
		})
	})
}
