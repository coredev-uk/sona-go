package player

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// openRowMenu opens the menu of the entry uid at a point in window DIPs
// (right-click, the row's corner for Shift+F10, below the row's "more"
// button). keyboard starts it on its first item.
func (p *player) openRowMenu(uid uint64, x, y float32, keyboard bool) {
	p.queue.menuUID = uid
	p.queue.menu.OpenAt(x, y, keyboard)
}

// rowMenu draws the row menu (QueueRowMenu.tsx) while it is open: Play,
// Play next and Move to end (each only where it changes something), then
// Remove. It follows its song by uid if the queue moves under it, and goes
// when the song is no longer upcoming. When it closes the focus returns to
// the row, not to where the pointer was.
func (p *player) rowMenu(c *ui.Context, st *queueList, items []services.QueueItem, index int) {
	m := &p.queue.menu
	uid := p.queue.menuUID
	qi := indexOfUID(items, uid)
	if m.IsOpen() && qi <= index {
		m.Close()
	}
	var rows []kit.MenuItem
	name := ""
	if m.IsOpen() {
		it := items[qi]
		name = it.Name
		g := &st.g
		next, end := menuMoves(qi, index, g.userEnd, it.Auto)
		// The entry's place is read when a row is chosen: the queue may have
		// moved since the menu opened.
		at := func() (int, services.QueueItem, bool) {
			cur := p.env.Play.Queue().Items
			i := indexOfUID(cur, uid)
			if i < 0 {
				return 0, services.QueueItem{}, false
			}
			return i, cur[i], true
		}
		rows = append(rows, kit.MenuItem{Key: "play", Label: i18n.T("Play"), Icon: "play", Run: func() {
			if i, _, ok := at(); ok {
				p.jumpTo(i)
			}
		}})
		if next {
			rows = append(rows, kit.MenuItem{Key: "next", Label: i18n.T("Play next"), Icon: "list-start", Run: func() {
				if i, cur, ok := at(); ok {
					p.askMove(cur, i, true, func() error { return p.svc.MoveNext(i) })
				}
			}})
		}
		if end {
			rows = append(rows, kit.MenuItem{Key: "end", Label: i18n.T("Move to end"), Icon: "list-end", Run: func() {
				if i, cur, ok := at(); ok {
					p.askMove(cur, i, true, func() error { return p.svc.MoveToEnd(i) })
				}
			}})
		}
		rows = append(rows,
			kit.MenuItem{Key: "sep", Kind: kit.MenuSeparator},
			kit.MenuItem{Key: "remove", Label: i18n.T("Remove"), Icon: "x", Run: func() {
				if i, _, ok := at(); ok {
					p.removeAt(i)
				}
			}})
	}
	m.Show(c, kit.MenuOpts{ID: "queue-row-menu", Label: i18n.T("More for {name}", "name", name), Width: 192}, ui.Element{}, rows)

	open := m.IsOpen()
	if st.menuWas && !open {
		p.focusRow(st, items, indexOfUID(items, uid))
	}
	st.menuWas = open
}
