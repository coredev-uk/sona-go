package playlist

import (
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The editor's geometry (PlaylistEditor.tsx), in DIPs: the rows are the
// track list's, so long playlists virtualise exactly like it.
const (
	editorRow = kit.TrackRowHeight
	// editorOverscan is how many rows beyond the ones showing are built.
	editorOverscan = 12
	// editorHandleW and editorRemoveW are the drag handle's and the
	// remove cross's columns.
	editorHandleW = 24
	editorNumberW = 32
	editorTimeW   = 56
	editorRemoveW = 32
)

// editor is the state of the edit mode's list of songs: a roving focus (one
// Tab stop, the focused row), the drop slot of a drag in progress and where
// the list sits in its pane.
type editor struct {
	nav kit.GridNav
	// slot is the gap (0…n) a drag hovers; slotOn is false when none.
	slot   int
	slotOn bool
	// top is the list's offset in its pane's content; drawTop is that
	// offset as the last paint saw it (one frame's position and scroll
	// offset together), drawn says there was a paint.
	top     float32
	drawTop float32
	drawn   bool
}

// window works out the rows of a long list to build: those in view and
// editorOverscan beyond.
func (ed *editor) window(p *kit.Pane, count int) []int {
	pane := p.Bounds()
	y := p.Pos.Y - ed.top
	h := pane.H
	if h <= 0 {
		h = editorRow * 20
	}
	first := max(0, int(y/editorRow))
	last := min(count-1, int((y+h)/editorRow))
	from, to := max(0, first-editorOverscan), min(count-1, last+editorOverscan)
	out := make([]int, 0, max(0, to-from+1))
	for i := from; i <= to; i++ {
		out = append(out, i)
	}
	return out
}

// scrollTo scrolls the pane as little as shows row i, so it gets built.
func (ed *editor) scrollTo(p *kit.Pane, i int) {
	pane := p.Bounds()
	top := ed.top + float32(i)*editorRow
	bottom := top + editorRow
	switch {
	case top < p.Pos.Y:
		p.Pos.Y = max(0, top)
	case pane.H > 0 && bottom > p.Pos.Y+pane.H:
		p.Pos.Y = bottom - pane.H
	}
}

// slotAt is the gap a pointer at y (in the list's box) is nearest.
func slotAt(y float32, n int) int {
	return max(0, min(n, int(math.Round(float64(y/editorRow)))))
}

// editorList is PlaylistEditor: a playlist's tracks in edit mode. Drag a row
// by its handle (or press Alt+↑/↓ on a focused row) to move it; the cross
// (or Delete) removes it. Every change is optimistic and goes through
// moveTrack and removeTracks. pane is the page's scroll container: a list
// of more than VirtualFrom rows in one is built as it shows.
func (p *playlists) editorList(c *ui.Context, id string, d services.PlaylistDetail, pane *kit.Pane) {
	ed := &p.page.editor
	tracks := d.Tracks
	n := len(tracks)
	name := d.Playlist.Name
	listID := "playlist-editor:" + id
	virtual := pane != nil && n > kit.VirtualFrom

	ed.nav.Count, ed.nav.Columns = n, 1
	_, wh := c.Size()
	ed.nav.Page = max(1, int(wh/editorRow)-2)
	ed.nav.Ends = true
	ed.nav.ScrollTo = nil
	if n > 0 && ed.nav.Focus() >= n {
		ed.nav.SetFocus(n - 1)
	}

	move := func(from, to int) {
		// from may be older than the list: a drag that began before a refresh
		// shrank it.
		if from < 0 || from >= n || to < 0 || to >= n || from == to {
			return
		}
		p.moveTrack(id, from, to, tracks[from])
		c.Announce(i18n.T("Moved {songName} to position {position} of {count}", "songName", tracks[from].Name, "position", to+1, "count", n))
		c.Invalidate()
	}
	remove := func(i int) {
		p.removeTracks(id, []int{i}, []services.Song{tracks[i]})
		c.Announce(i18n.T("Removed {songName}", "songName", tracks[i].Name))
		c.Invalidate()
	}

	root := ui.Column(c.Key("editor")).FillWidth().Role(ui.RoleTable).
		Label(i18n.T("Edit the songs in {name}", "name", name)).
		Description(i18n.T("Drag the handle, or press Alt+↑ and Alt+↓ on a song, to change the order. Delete removes the focused song."))
	root.Children(func() {
		body := ui.Column(c.Key("body")).FillWidth()
		if pane != nil {
			body.Draw(func(_ *ui.Painter, r ui.Rect) {
				// Paint runs after this frame's layout: the box and the
				// scroll offset belong to the same frame.
				top := r.Y - pane.Bounds().Y + pane.Pos.Y
				if !ed.drawn || top != ed.drawTop {
					ed.drawTop, ed.drawn = top, true
					c.Invalidate()
				}
			})
			if ed.drawn {
				ed.top = ed.drawTop
			} else {
				ed.top = body.Bounds().Y - pane.Bounds().Y + pane.Pos.Y
			}
		}
		// Reordering is the shared drag: the handle starts it, the list is
		// its drop target and only a change of drop slot matters.
		kit.DropOn(body, listID, kit.DropTarget{
			Accepts:  func(dp *kit.DragPayload) bool { return dp.Origin != nil && dp.Origin.List == listID },
			Describe: func(*kit.DragPayload) string { return i18n.T("Move here") },
			Hover: func(_ *kit.DragPayload, at *kit.DragPoint) {
				if at == nil {
					ed.slotOn = false
					return
				}
				ed.slot, ed.slotOn = slotAt(at.LocalY, n), true
			},
			Drop: func(dp *kit.DragPayload, at kit.DragPoint) {
				from := dp.Origin.Index
				to := kit.DropIndex(from, slotAt(at.LocalY, n))
				move(from, to)
				ed.nav.SetFocus(max(0, min(to, n-1)))
				ed.slotOn = false
			},
		})
		body.Children(func() {
			var built []int
			if virtual {
				built = ed.window(pane, n)
				ed.nav.ScrollTo = func(i int) { ed.scrollTo(pane, i) }
			} else {
				built = make([]int, n)
				for i := range built {
					built[i] = i
				}
			}
			dragged, dragging := kit.DragPinned(ed)
			if dragging && dragged < n && !slices.Contains(built, dragged) {
				built = append(built, dragged)
				slices.Sort(built)
			}
			prev := -1
			for _, i := range built {
				if gap := i - prev - 1; gap > 0 {
					ui.Box(c.Key("gap-" + strconv.Itoa(prev))).Height(float32(gap) * editorRow)
				}
				prev = i
				p.editorRow(c, ed, name, i, tracks[i], listID, dragging && dragged == i, move, remove)
			}
			if gap := n - 1 - prev; virtual && gap > 0 {
				ui.Box(c.Key("gap-end")).Height(float32(gap) * editorRow)
			}
			// The drop slot: a 2 DIP line between rows, not at the song's
			// own position.
			if dragging && ed.slotOn && ed.slot != dragged && ed.slot != dragged+1 {
				ui.Box(c.Key("slot")).Absolute().Left(4).Right(4).Top(float32(ed.slot)*editorRow - 1).Height(2).Radius(1).
					Background(theme.C("ink")).IgnoreInput(true).Role(ui.RoleNone)
			}
		})
	})
	ed.nav.Tick(c)
}

// editorRow is one editable row: drag handle, position, 32 DIP art with the
// name and artist, album, duration ("—" when unknown) and the remove cross.
func (p *playlists) editorRow(c *ui.Context, ed *editor, playlist string, i int, song services.Song, listID string, dragged bool, move func(from, to int), remove func(i int)) {
	tk := theme.Current()
	ink, muted, faint := theme.C("ink"), theme.C("ink-muted"), theme.C("ink-faint")
	row := ui.Row(c.Key(i)).FillWidth().Height(editorRow).Shrink(0).AlignItems(ui.Center).Gap(12).PaddingX(4).
		Radius(tk.Radius["md"]).Role(ui.RoleRow).Label(song.Name)
	if i == ed.nav.Focus() {
		row.Focusable()
	}
	switch {
	case dragged:
		row.Background(theme.C("active")).Opacity(0.6)
	case row.Hovered() || row.Focused():
		row.Background(theme.C("hover"))
	}
	row.Children(func() {
		// The handle starts the drag; it is no Tab stop (the row is).
		kit.DragItem(c, kit.DragOpts{
			Key:   "grab:" + strconv.Itoa(i),
			Scope: ed,
			Index: i,
			Art:   p.env.Art,
			Row:   true,
			Payload: func() *kit.DragPayload {
				ed.nav.SetFocus(i)
				return &kit.DragPayload{Origin: &kit.DragOrigin{List: listID, Index: i}, Label: song.Name, Art: song.Art.Small}
			},
			Outer: func(w ui.Element) { w.Size(editorHandleW, 32).Shrink(0).Role(ui.RoleCell) },
			Source: func(s ui.Element) {
				s.Role(ui.RoleButton).Label(i18n.T("Drag {songName} to reorder", "songName", song.Name)).Center().
					Radius(tk.Radius["sm"]).Cursor(ui.CursorGrab)
			},
		}, func(_, s ui.Element) {
			col := faint
			if s.Hovered() {
				col = ink
			}
			ui.Icon(c, theme.Icon("grip-vertical")).FontSize(16).TextColor(col)
		})

		numT := tk.Type["sub"]
		ui.Row(c.Key("num")).Width(editorNumberW).Shrink(0).Justify(ui.End).Role(ui.RoleCell).Children(func() {
			ui.Text(c, strconv.Itoa(i+1)).FontSize(numT.Size).FixedLineHeight(numT.LineHeight).TextColor(faint).FontFeatures("tnum").SingleLine()
		})

		ui.Row(c.Key("title")).Grow(2).Basis(0).MinWidth(0).AlignItems(ui.Center).Gap(12).Role(ui.RoleCell).Children(func() {
			kit.Artwork(c, p.env.Art, song.Art.Small, tk.Metric["track-art"], kit.ArtOpts{})
			ui.Column(c).MinWidth(0).Grow(1).Justify(ui.Center).Children(func() {
				uiT, subT := tk.Type["ui"], tk.Type["sub"]
				ui.Text(c, song.Name).FontSize(uiT.Size).FixedLineHeight(uiT.LineHeight).TextColor(ink).SingleLine()
				ui.Text(c, song.Artist).FontSize(subT.Size).FixedLineHeight(subT.LineHeight).TextColor(muted).SingleLine()
			})
		})

		ui.Row(c.Key("album")).Grow(1).Basis(0).MinWidth(0).Role(ui.RoleCell).Children(func() {
			subT := tk.Type["sub"]
			ui.Text(c, song.Album).FontSize(subT.Size).FixedLineHeight(subT.LineHeight).TextColor(muted).SingleLine()
		})

		dur := "—"
		if song.DurationMs > 0 {
			dur = i18n.FormatDuration(time.Duration(song.DurationMs) * time.Millisecond)
		}
		ui.Row(c.Key("dur")).Width(editorTimeW).Shrink(0).Justify(ui.End).Role(ui.RoleCell).Children(func() {
			subT := tk.Type["sub"]
			ui.Text(c, dur).FontSize(subT.Size).FixedLineHeight(subT.LineHeight).TextColor(faint).FontFeatures("tnum").SingleLine()
		})

		cell := ui.Row(c.Key("remove-cell")).Width(editorRemoveW).Shrink(0).Justify(ui.End).Role(ui.RoleCell)
		cell.Children(func() {
			cross := ui.Row(c.Key("remove")).Size(28, 28).Shrink(0).Center().Radius(tk.Radius["md"]).Role(ui.RoleButton).
				Label(i18n.T("Remove {songName} from {playlist}", "songName", song.Name, "playlist", playlist)).Cursor(ui.CursorPointer)
			col := faint
			if cross.Hovered() {
				col = ink
				cross.Background(theme.C("active"))
			}
			cross.TextColor(col).Children(func() { ui.Icon(c, theme.Icon("x")).FontSize(16) })
			if cross.Clicked() {
				remove(i)
			}
		})
	})

	ed.nav.Claim(row, i)
	if row.Clicked() {
		ed.nav.SetFocus(i)
		if !row.Focused() {
			ed.nav.Want(i)
		}
	}
	if !row.Focused() {
		return
	}
	switch {
	case row.Shortcut(ui.Alt, ui.KeyUp):
		move(i, i-1)
		ed.nav.Want(i - 1)
	case row.Shortcut(ui.Alt, ui.KeyDown):
		move(i, i+1)
		ed.nav.Want(i + 1)
	case row.Shortcut(0, ui.KeyDelete), row.Shortcut(0, ui.KeyBackspace):
		remove(i)
		ed.nav.Want(min(i, ed.nav.Count-2))
	default:
		ed.nav.Keys(row, i, nil)
	}
}
