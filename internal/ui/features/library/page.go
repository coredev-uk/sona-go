package library

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// sortSelect is the Sort select of a list's header: it reports the new sort
// (and its natural order) when the user chose another.
func (l *library) sortSelect(c *ui.Context, spec listSpec, q listQuery) {
	items := make([]kit.SelectItem, len(spec.sorts))
	for i, o := range spec.sorts {
		items[i] = kit.SelectOption(o.key, o.label())
	}
	value := q.sort
	if kit.Select(c, &value, items, kit.SelectOpts{Small: true, Width: 160, A11yLabel: i18n.T("Sort")}).Changed() && value != q.sort {
		next := q
		next.sort = value
		next.desc = startsDesc(value)
		l.env.Nav.Replace(spec.location(next))
	}
}
