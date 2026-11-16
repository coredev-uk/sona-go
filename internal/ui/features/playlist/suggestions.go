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

// suggestState is the shelf's state, for as long as the page shows one
// playlist.
type suggestState struct {
	// open is the disclosure: the shelf starts closed and asks Apple for
	// nothing until it is opened. It is not remembered beyond the page.
	open bool
	// hidden are the songs added from the shelf: they leave the list at
	// once, and come back only if their add fails.
	hidden map[string]bool
	// seen are the songs a Refresh replaced; a later set leaves them out.
	seen map[string]bool
	// refreshing is a Refresh waiting for Apple's answer; fresh are the songs
	// it put in seen, to take back if the fetch fails.
	refreshing bool
	fresh      []string
	// focus is the song whose play button takes the focus on the next frame
	// (the row after an added one); focusRefresh moves it to Refresh instead
	// (the list is empty).
	focus        string
	focusRefresh bool
}

// suggestionsShelf is Suggested songs: the songs Apple suggests for a
// library playlist, listed under the playlist as its next side, each
// playing on click and leaving the list when its add button puts it in the
// playlist. The shelf starts closed and fetches only when opened; Refresh
// asks Apple for another set (every call to the suggestions endpoint draws
// a new one). tracks are the playlist's songs, which never come back as
// suggestions.
func (p *playlists) suggestionsShelf(c *ui.Context, id, playlist string, tracks []services.Song) {
	st := &p.page.suggest
	title := i18n.T("Suggested Songs")
	var res store.Result[[]services.Song]
	if st.open {
		res = p.suggestions(id)
		p.finishRefresh(c, st, res)
	}
	var refresh ui.Element
	kit.Shelf(c, kit.ShelfOpts{Title: title, Actions: func() {
		label := i18n.T("Show suggestions")
		if st.open {
			label = i18n.T("Hide suggestions")
			refresh = kit.Button(c.Key("refresh"), i18n.T("Refresh"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Icon: "refresh-cw", Muted: true})
			if refresh.Clicked() && !st.refreshing {
				p.refreshSuggestions(id, res)
			}
			if st.focusRefresh {
				st.focusRefresh = false
				refresh.Focus()
			}
		}
		if kit.ShelfDisclosure(c, label, st.open).Clicked() {
			st.open = !st.open
		}
	}}, func() {
		if !st.open {
			return
		}
		loading := (!res.HasData && res.Err == nil) || (st.refreshing && res.Fetching)
		var songs []services.Song
		if !loading {
			songs = p.visibleSuggestions(res.Data, tracks)
		}
		switch {
		case loading:
			ui.Column(c.Key("suggestions")).FillWidth().Role(ui.RoleList).Label(title).Children(func() {
				for i := range suggestionLoadN {
					p.suggestionSkeleton(c, i)
				}
			})
		case len(songs) == 0:
			heading, text := i18n.T("No suggestions right now"), i18n.T("Refresh for another set.")
			if !res.HasData {
				heading, text = i18n.T("Couldn't load suggestions"), i18n.T("Refresh to try again.")
			}
			emptyBlock(c, heading, text)
		default:
			ui.Column(c.Key("suggestions")).FillWidth().Role(ui.RoleList).Label(title).Children(func() {
				for i, s := range songs {
					// The focus goes to the row after the added one, or the
					// one before it.
					next := ""
					switch {
					case i+1 < len(songs):
						next = songs[i+1].ID
					case i > 0:
						next = songs[i-1].ID
					}
					p.suggestionRow(c, id, playlist, s, next)
				}
			})
		}
	})
}

// visibleSuggestions are the songs of the set that may show: not added from
// the shelf, not in an earlier set, not already in the playlist.
func (p *playlists) visibleSuggestions(set, tracks []services.Song) []services.Song {
	st := &p.page.suggest
	if len(set) == 0 {
		return nil
	}
	have := make(map[string]bool, 2*len(tracks))
	for i := range tracks {
		have[tracks[i].ID] = true
		if tracks[i].CatalogID != "" {
			have[tracks[i].CatalogID] = true
		}
	}
	out := make([]services.Song, 0, len(set))
	for _, s := range set {
		if st.hidden[s.ID] || st.seen[s.ID] || have[s.ID] || (s.CatalogID != "" && have[s.CatalogID]) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// refreshSuggestions asks Apple for another set: the songs of this one are
// not shown again, and the list is skeleton rows until the answer.
func (p *playlists) refreshSuggestions(id string, res store.Result[[]services.Song]) {
	st := &p.page.suggest
	if st.seen == nil {
		st.seen = map[string]bool{}
	}
	st.fresh = st.fresh[:0]
	for _, s := range res.Data {
		if !st.seen[s.ID] {
			st.seen[s.ID] = true
			st.fresh = append(st.fresh, s.ID)
		}
	}
	st.refreshing = true
	p.env.Queries.Invalidate(keySuggestions(id))
}

// finishRefresh ends a Refresh once its fetch has settled: the new set is
// announced, or a failure toasts and the old set comes back.
func (p *playlists) finishRefresh(c *ui.Context, st *suggestState, res store.Result[[]services.Song]) {
	if !st.refreshing || res.Fetching {
		return
	}
	st.refreshing = false
	if res.Err != nil {
		for _, id := range st.fresh {
			delete(st.seen, id)
		}
		kit.ToastFailure(i18n.T("Couldn't refresh the suggestions"), res.Err)
		return
	}
	c.Announce(i18n.T("Suggestions refreshed"))
}

// suggestionSkeleton is a loading row: flat bars at the real sizes.
func (p *playlists) suggestionSkeleton(c *ui.Context, i int) {
	ui.Row(c.Key("loading-" + strconv.Itoa(i))).FillWidth().Height(suggestionRow).AlignItems(ui.Center).Gap(suggestionGap).
		PaddingX(suggestionPadX).Role(ui.RoleNone).Children(func() {
		ui.Box(c).Width(suggestionNumW).Shrink(0).Role(ui.RoleNone)
		kit.Bar(c, suggestionArt, suggestionArt)
		kit.Bar(c, 0, 14).WidthPercent(24)
	})
}

// suggestionRow is one suggested song: the art plays it, the add button puts
// it in the playlist. Title, artist, length and the button sit in the
// columns of the track list.
func (p *playlists) suggestionRow(c *ui.Context, id, playlist string, s services.Song, next string) {
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
			add := kit.Button(c.Key("add"), i18n.T("Add"), kit.ButtonOpts{
				Variant: kit.Ghost, Size: kit.SizeSM, Icon: "plus", Muted: true,
				A11yLabel: i18n.T("Add {songName} to {playlistName}", "songName", s.Name, "playlistName", playlist),
			})
			if add.Clicked() {
				p.addSuggestion(id, playlist, s)
				// The row leaves the list: say so, and move the focus to the
				// next row's play button, or Refresh when none is left.
				c.Announce(i18n.T("Added {label} to “{name}”", "label", "“"+s.Name+"”", "name", playlist))
				if next == "" {
					st.focusRefresh = true
				} else {
					st.focus = next
				}
			}
		})
		if st.focus == s.ID {
			st.focus = ""
			play.Focus()
		}
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

// addSuggestion puts a suggested song in the playlist: the row leaves the
// list at once, the add is queued with the playlist's other edits, and a
// failure puts the row back in its place with a toast.
func (p *playlists) addSuggestion(id, playlist string, s services.Song) {
	st := &p.page.suggest
	if st.hidden == nil {
		st.hidden = map[string]bool{}
	}
	hidden := st.hidden
	hidden[s.ID] = true
	items := []services.PlaylistItem{{Kind: "song", ID: s.ID, Source: s.Source}}
	p.edits.serial(id, func() (services.PlaylistDetail, error) {
		return p.api.Add(context.Background(), id, items)
	}, func(d services.PlaylistDetail, err error, _ bool) {
		p.env.Post(func() {
			if err != nil {
				delete(hidden, s.ID)
				kit.ToastFailure(i18n.T("Couldn't add “{songName}” to “{playlistName}”", "songName", s.Name, "playlistName", playlist), err)
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
