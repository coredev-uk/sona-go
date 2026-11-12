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

// The suggestions' geometry, in DIPs: rows of the track list's height and
// padding, so the B-side lines up under the songs above it.
const (
	suggestionRow   = kit.TrackRowHeight
	suggestionArt   = 32
	suggestionLoadN = 6
	suggestionPadX  = 8
	suggestionGap   = 16
	suggestionTimeW = 56
	// suggestionNumW and suggestionActW are the track list's number and
	// actions columns, left empty so the columns line up with it.
	suggestionNumW = 40
	suggestionActW = 32
)

// suggestState is the shelf's state: the songs that show "Added".
type suggestState struct {
	added map[string]bool
}

// suggestionsShelf is Suggested songs: the songs Apple suggests for a
// library playlist, listed under the playlist as its next side, each
// playing on click and joining the playlist with its add button. It is
// hidden when the suggestions failed or came back empty.
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
		count := len(res.Data)
		if loading {
			count = suggestionLoadN
		}
		ui.Column(c.Key("suggestions")).FillWidth().Role(ui.RoleList).Label(title).Children(func() {
			for i := range count {
				if loading {
					p.suggestionSkeleton(c, i)
					continue
				}
				p.suggestionRow(c, id, playlist, res.Data[i])
			}
		})
	})
}

// suggestionSkeleton is a loading row: flat bars at the real sizes.
func (p *playlists) suggestionSkeleton(c *ui.Context, i int) {
	hover := theme.C("hover")
	r := theme.Current().Radius["sm"]
	ui.Row(c.Key("loading-" + strconv.Itoa(i))).FillWidth().Height(suggestionRow).AlignItems(ui.Center).Gap(suggestionGap).
		PaddingX(suggestionPadX).Role(ui.RoleNone).Children(func() {
		ui.Box(c).Width(suggestionNumW).Shrink(0).Role(ui.RoleNone)
		ui.Box(c).Size(suggestionArt, suggestionArt).Shrink(0).Radius(r).Background(hover)
		ui.Box(c).WidthPercent(24).Height(14).Radius(r).Background(hover)
	})
}

// suggestionRow is one suggested song: the art plays it, the add button puts
// it in the playlist. Title, artist, length and the button sit in the
// columns of the track list.
func (p *playlists) suggestionRow(c *ui.Context, id, playlist string, s services.Song) {
	tk := theme.Current()
	st := &p.page.suggest
	row := ui.Row(c.Key(s.ID)).FillWidth().Height(suggestionRow).AlignItems(ui.Center).Gap(suggestionGap).PaddingX(suggestionPadX).
		Radius(tk.Radius["md"]).Role(ui.RoleListItem)
	if row.Hovered() {
		row.Background(theme.C("hover"))
	}
	uiT, subT := tk.Type["ui"], tk.Type["sub"]
	row.Children(func() {
		ui.Box(c.Key("num")).Width(suggestionNumW).Shrink(0).Role(ui.RoleNone)
		var play ui.Element
		ui.Row(c.Key("song")).Grow(2).Basis(0).MinWidth(0).AlignItems(ui.Center).Gap(12).Children(func() {
			play = ui.ButtonBase(c.Key("play")).Shrink(0).Radius(tk.Radius["sm"]).Label(i18n.T("Play {songName}", "songName", s.Name))
			play.Children(func() {
				kit.Artwork(c, p.env.Art, s.Art.Small, suggestionArt, kit.ArtOpts{})
			})
			if play.Clicked() {
				p.playSongs([]services.Song{s}, 0, services.PlayFrom{ID: id}, false)
			}
			ui.Text(c, s.Name).FontSize(uiT.Size).FixedLineHeight(uiT.LineHeight).TextColor(theme.C("ink")).SingleLine().Shrink(1).MinWidth(0)
		})
		ui.Row(c.Key("artist")).Grow(1).Basis(0).MinWidth(0).Children(func() {
			ui.Text(c, s.Artist).FontSize(subT.Size).FixedLineHeight(subT.LineHeight).TextColor(theme.C("ink-muted")).SingleLine()
		})
		ui.Row(c.Key("add-cell")).Grow(1).Basis(0).MinWidth(0).Justify(ui.Start).Children(func() {
			if st.added[s.ID] {
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
				// The button is replaced by "Added": say so, and keep the focus
				// in the row, on the song's play button.
				c.Announce(i18n.T("Added {label} to “{name}”", "label", "“"+s.Name+"”", "name", playlist))
				play.Focus()
			}
		})
		dur := ""
		if s.DurationMs > 0 {
			dur = i18n.FormatDuration(time.Duration(s.DurationMs) * time.Millisecond)
		}
		ui.Row(c.Key("dur")).Width(suggestionTimeW).Shrink(0).Justify(ui.End).Children(func() {
			ui.Text(c, dur).FontSize(subT.Size).FixedLineHeight(subT.LineHeight).TextColor(theme.C("ink-faint")).FontFeatures("tnum").SingleLine()
		})

		ui.Box(c.Key("act")).Width(suggestionActW).Shrink(0).Role(ui.RoleNone)
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
