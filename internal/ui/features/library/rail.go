package library

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The letter rail is a static column at a list page's right edge: A to Z and
// "#", the letters that start something in the list as buttons, the others
// faint, each jumping to the first item of its letter. It exists while the
// list is ordered A to Z by title or artist.

const (
	railWidth    = 24
	railLetterH  = 16
	railSlots    = 27
	railOthers   = 26 // "#": digits, symbols and other scripts
	railSlotRuns = "ABCDEFGHIJKLMNOPQRSTUVWXYZ#"
)

// letterJumps is, for each slot of the rail, the index of the first item
// that starts with its letter, -1 when none does.
type letterJumps [railSlots]int

// noLetters is a rail with nothing to jump to.
func noLetters() letterJumps {
	var j letterJumps
	for i := range j {
		j[i] = -1
	}
	return j
}

// lower is the first byte of s as the library's NOCASE order sees it: ASCII
// folded to lower case, every other byte as it is.
func lower(s string) byte {
	if s == "" {
		return 0
	}
	b := s[0]
	if b >= 'A' && b <= 'Z' {
		b += 'a' - 'A'
	}
	return b
}

// slotOf is the rail slot a first byte files under.
func slotOf(b byte) int {
	if b >= 'a' && b <= 'z' {
		return int(b - 'a')
	}
	return railOthers
}

// lettersOf is the jumps of a list held whole in memory: key(i) is the sort
// key of item i.
func lettersOf(n int, key func(i int) string) letterJumps {
	j := noLetters()
	for i := range n {
		if s := slotOf(lower(key(i))); j[s] < 0 {
			j[s] = i
		}
	}
	return j
}

// searchLetters finds the jumps of a list of n items that is too long to
// hold, in the order the library sorts it (ascending, NOCASE): each letter's
// first item by bisection over single-row reads, starting from where the
// letter before it ended, so a whole rail costs a few hundred reads on a
// worker, once per sort until a sync changes the library. at reads the sort
// key of item i.
func searchLetters(ctx context.Context, n int, at func(ctx context.Context, i int) (string, error)) (letterJumps, error) {
	j := noLetters()
	seen := map[int]byte{}
	first := func(i int) (byte, error) {
		if b, ok := seen[i]; ok {
			return b, nil
		}
		s, err := at(ctx, i)
		if err != nil {
			return 0, err
		}
		b := lower(s)
		seen[i] = b
		return b, nil
	}
	if n == 0 {
		return j, nil
	}
	b0, err := first(0)
	if err != nil {
		return j, err
	}
	if slotOf(b0) == railOthers {
		j[railOthers] = 0
	}
	lo := 0
	for s := range railOthers {
		target := byte('a' + s)
		l, h := lo, n
		for l < h {
			m := (l + h) / 2
			b, err := first(m)
			if err != nil {
				return j, err
			}
			if b >= target {
				h = m
			} else {
				l = m + 1
			}
		}
		lo = l
		if l < n {
			b, err := first(l)
			if err != nil {
				return j, err
			}
			if b == target {
				j[s] = l
			}
		}
	}
	return j, nil
}

// railShown: the list is ordered A to Z by a name, so the rail means something.
func railShown(sort string, desc bool, total int) bool {
	if desc || total == 0 {
		return false
	}
	switch sort {
	case "title", "artist", "album":
		return true
	}
	return false
}

// jumpsOf is the rail's letters of a paged list: read off the page held
// when the whole list is that page (loaded), else searched on a worker (key: the
// query key, which carries the sort and the size). ready is false until
// they are known; the rail then shows every letter faint.
func jumpsOf(l *library, total int, loaded bool, held func(i int) string, key store.Key, at func(ctx context.Context, i int) (string, error)) (j letterJumps, ready bool) {
	if total <= pageSize {
		if !loaded {
			return noLetters(), false
		}
		return lettersOf(total, held), true
	}
	res := store.Use(l.env.Queries, key, func(ctx context.Context) (letterJumps, error) {
		return searchLetters(ctx, total, at)
	}, store.Options{Stale: store.Forever})
	if !res.HasData {
		return noLetters(), false
	}
	return res.Data, true
}

// letterRail builds the rail: one Tab stop, Up and Down moving among the
// letters that can be jumped to, Home and End to the first and the last.
func (l *library) letterRail(c *ui.Context, j letterJumps, ready bool, jump func(slot int)) {
	ui.Column(c.Key("rail")).Width(railWidth).Shrink(0).AlignItems(ui.Center).
		FocusGroup(ui.Vertical).Role(ui.RoleToolbar).Label(i18n.T("Jump to letter")).Children(func() {
		for slot := range railSlots {
			letter := string(railSlotRuns[slot])
			if !ready || j[slot] < 0 {
				styled(c.Key(slot), letter, "meta").FontWeight(600).TextColor(theme.C("control-line")).
					Height(railLetterH).Role(ui.RoleNone)
				continue
			}
			label := i18n.T("Jump to {letter}", "letter", letter)
			if slot == railOthers {
				label = i18n.T("Jump to numbers and symbols")
			}
			b := ui.ButtonBase(c.Key(slot)).Width(railWidth).Height(railLetterH).Radius(theme.Current().Radius["sm"]).
				AlignItems(ui.Center).Justify(ui.Center).Label(label)
			fg := theme.C("ink-muted")
			if b.Hovered() {
				b.Background(theme.C("hover"))
				fg = theme.C("ink")
			}
			b.Children(func() {
				styled(c, letter, "meta").FontWeight(600).TextColor(fg).Role(ui.RoleNone)
			})
			if b.Clicked() {
				jump(slot)
			}
		}
	})
}
