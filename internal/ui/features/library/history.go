package library

import (
	"context"
	"reflect"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// historyPageDays is the days loaded at first, and added by "Show earlier
// days".
const historyPageDays = 30

// historyDay is the view state of one day's list.
type historyDay struct {
	list  kit.TrackListState
	menus *kit.ActionMenus
}

// historyPage is the History page's view state.
type historyPage struct {
	pageView
	// days is how many days are asked for (0 until the first frame).
	days int
	// data is the last result of any days value: it keeps showing while
	// the next key loads. has: there is one.
	data services.ListeningHistory
	has  bool
	// id tells a new result from the one shown (a fetch makes a new slice).
	id uintptr
	// arrived is when data arrived: day labels are relative to it.
	arrived time.Time
	groups  []historyGroup
	lists   map[int64]*historyDay
	// skeleton is the loading rows' list state.
	skeleton kit.TrackListState
}

// historyKey is `["library", "recent", "history", days]`: under the recent
// prefix, so a new song refreshes it.
func historyKey(days int) store.Key {
	return append(append(store.Key(nil), keyRecent...), "history", days)
}

// historyPage is HistoryPage: what the account played, newest first and
// grouped by day: plays on this computer with Apple's recently played from
// the account's other devices merged in by Go (LibraryService.History).
func (l *library) historyPage(c *ui.Context, r *ui.Route) {
	pg := &l.history
	if pg.days == 0 {
		pg.days = historyPageDays
	}
	key := historyKey(pg.days)
	days := pg.days
	res := store.Use(l.env.Queries, key, func(ctx context.Context) (services.ListeningHistory, error) {
		return l.lib.History(ctx, days)
	}, store.Options{Stale: store.Forever}) // refreshed by ["library","recent"] when a new song plays
	if res.HasData {
		if id := reflect.ValueOf(res.Data.Days).Pointer(); !pg.has || id != pg.id || len(res.Data.Days) != len(pg.data.Days) {
			pg.data, pg.has, pg.id, pg.arrived = res.Data, true, id, time.Now()
			pg.groups = historyGroups(res.Data)
		}
	}
	auth := l.authStatus()
	out := auth.HasData && signedOut(auth.Data.State)

	subline := ""
	if pg.has {
		switch {
		case pg.data.Apple:
			subline = i18n.T("On this computer and your other devices")
		case out:
			subline = i18n.T("On this computer")
		default:
			subline = i18n.T("On this computer only: Apple Music couldn't be reached for your other devices")
		}
	}

	l.scrollPage(c, &pg.pageView, "history", func(*kit.Pane) {
		kit.PageHeader(c, i18n.T("History"), subline, nil)
		switch {
		case res.Err != nil && !pg.has:
			kit.ErrorBlock(c, i18n.T("Couldn't read your listening history"), res.Err, kit.ErrorBlockOpts{
				OnRetry:  func() { l.env.Queries.Invalidate(key) },
				Retrying: res.Fetching,
			})
		case !pg.has:
			l.historySkeleton(c)
		case len(pg.groups) == 0:
			if out {
				kit.EmptyState(c, i18n.T("Nothing played on this computer yet"),
					i18n.T("Songs you play in Sona are listed here by day. Sign in to add what you play on your other devices."),
					kit.EmptyOpts{Action: l.signIn()})
			} else {
				kit.EmptyState(c, i18n.T("Nothing played yet"),
					i18n.T("Songs you play here or in any Apple Music app are listed here by day."), kit.EmptyOpts{})
			}
		default:
			for i := range pg.groups {
				l.historySection(c, pg, &pg.groups[i])
			}
			l.historyFooter(c, pg, res, key)
		}
	})
}

// historySkeleton is the loading placeholder: a day heading and 16 rows.
func (l *library) historySkeleton(c *ui.Context) {
	ui.Column(c.Key("history-loading")).Role(ui.RoleGroup).Label(i18n.T("Loading your listening history")).Children(func() {
		ui.Row(c).Height(22).AlignItems(ui.Center).Padding(0, 8).Margin(0, 0, 8, 0).Children(func() {
			kit.Skeleton(c, 96, 16)
		})
		kit.TrackList(c, &l.history.skeleton, skeletonRows, func(int) *services.Song { return nil }, kit.TrackListOpts{
			Label: i18n.T("History"), Columns: historyColumns,
		})
	})
}

var historyColumns = kit.TrackColumns{RowNumbers: true, Artwork: true, Artist: true, Album: true}

// historySubline is "12 songs, 15 plays · 3 from other devices": the play
// count only when songs repeated.
func historySubline(g *historyGroup) string {
	songs := len(g.songs)
	text := i18n.T("{songs, plural, one {# song} other {# songs}}", "songs", songs)
	if g.plays > songs {
		text = i18n.T("{songs, plural, one {# song} other {# songs}}, {plays, plural, one {# play} other {# plays}}", "songs", songs, "plays", g.plays)
	}
	if g.day != 0 && g.elsewhere > 0 {
		text += " · " + i18n.T("{elsewhere} from other devices", "elsewhere", i18n.FormatNumber(float64(g.elsewhere)))
	}
	return text
}

// historySection is one day: its heading with the counts, and its songs.
func (l *library) historySection(c *ui.Context, pg *historyPage, g *historyGroup) {
	label := dayLabel(g.day, pg.arrived, time.Local)
	st := pg.lists[g.day]
	if st == nil {
		if pg.lists == nil {
			pg.lists = map[int64]*historyDay{}
		}
		st = &historyDay{}
		pg.lists[g.day] = st
	}
	tk := theme.Current()
	h, meta := tk.Type["heading"], tk.Type["meta"]
	ui.Column(c.Key(g.day)).Role(ui.RoleGroup).Label(label).Margin(0, 0, 40, 0).Children(func() {
		ui.Row(c).Wrap().AlignItems(ui.Baseline).Gap(8).Padding(0, 8).Margin(0, 0, 8, 0).Children(func() {
			ui.Text(c, label).FontSize(h.Size).FixedLineHeight(h.LineHeight).FontWeight(int(h.Weight)).
				TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(2)
			ui.Text(c, historySubline(g)).FontSize(meta.Size).FixedLineHeight(meta.LineHeight).TextColor(theme.C("ink-faint"))
		})
		songs := g.songs
		opts := kit.TrackListOpts{
			Label:      i18n.T("Played {label}", "label", label),
			Columns:    historyColumns,
			OnPlay:     func(i int) { l.queue.PlaySongs(songs, i, services.PlayFrom{ID: i18n.T("History")}) },
			Art:        l.env.Art,
			Motion:     l.env.Motion,
			Restricted: l.restricted(),
			Menus:      l.itemMenus(&st.menus, &st.list.Sel),
		}
		if l.env.HasRoute != nil && l.env.HasRoute("/artist/x") {
			opts.OpenArtist = func(id string) { l.env.Nav.Push("/artist/" + id) }
		}
		if l.env.HasRoute != nil && l.env.HasRoute("/album/x") {
			opts.OpenAlbum = func(id string) { l.env.Nav.Push("/album/" + id) }
		}
		kit.TrackList(c, &st.list, len(songs), func(i int) *services.Song { return &songs[i] }, opts)
	})
}

// historyFooter is "Show earlier days", or the retention note once Go has
// nothing older.
func (l *library) historyFooter(c *ui.Context, pg *historyPage, res store.Result[services.ListeningHistory], key store.Key) {
	tk := theme.Current()
	ui.Row(c.Key("history-footer")).AlignItems(ui.Center).Gap(16).Padding(0, 8).Margin(32, 0, 0, 0).Children(func() {
		if !pg.data.More {
			meta := tk.Type["meta"]
			ui.Text(c, i18n.T("Plays on this computer are kept for a year.")).FontSize(meta.Size).FixedLineHeight(meta.LineHeight).TextColor(theme.C("ink-faint"))
			return
		}
		// A new key has no data until it arrives: the page keeps the last
		// result and this button says so.
		loading := res.Fetching
		label := i18n.T("Show earlier days")
		if loading {
			label = i18n.T("Loading…")
		}
		if kit.Button(c.Key("history-more"), label, kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Disabled: loading}).Clicked() {
			if res.Err != nil && !res.HasData {
				l.env.Queries.Invalidate(key) // the longer list failed: ask again
			} else {
				pg.days += historyPageDays
			}
		}
	})
}
