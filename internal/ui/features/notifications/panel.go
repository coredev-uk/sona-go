package notifications

import (
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// entriesMax is the most the panel's list grows to before it scrolls.
const entriesMax = 392

// panel is the centre: its title and Clear all, the live rows, the entries
// and the modules' foot lines. With nothing live and no entries it says so
// in one quiet block instead of an empty list.
func (f *feature) panel(c *ui.Context, acts []module.Activity, statuses []module.ActivityStatus, list services.Notifications) {
	live := false
	for i, a := range acts {
		live = live || (statuses[i].Shown && a.Build != nil)
	}
	ui.Column(c.Key("notices:panel")).FillWidth().Role(ui.RoleGroup).Label(i18n.T("Notifications")).Children(func() {
		ui.Row(c.Key("notices:head")).FillWidth().Height(32).Padding(0, 4, 0, 8).Margin(0, 0, 8, 0).AlignItems(ui.Center).Children(func() {
			theme.Text(c, i18n.T("Notifications"), "heading").TextColor(theme.C("ink")).Grow(1).Role(ui.RoleHeading)
			if len(list.Items) > 0 {
				if kit.Button(c.Key("notices:clear"), i18n.T("Clear all"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).Clicked() {
					f.clear()
				}
			}
		})
		if live {
			ui.Column(c.Key("notices:live")).FillWidth().Gap(4).Margin(0, 0, 8, 0).Children(func() {
				for i, a := range acts {
					if statuses[i].Shown && a.Build != nil {
						a.Build(c)
					}
				}
			})
		}
		switch {
		case len(list.Items) > 0:
			ui.Scroll(c.Key("notices:scroll")).FillWidth().MaxHeight(entriesMax).Children(func() {
				ui.Column(c.Key("notices:list")).FillWidth().Gap(2).Role(ui.RoleList).Label(i18n.T("Notifications")).Children(func() {
					now := f.now()
					for _, n := range list.Items {
						f.entry(c, n, now)
					}
				})
			})
		case !live:
			ui.Column(c.Key("notices:empty")).FillWidth().Padding(20, 16, 24).Gap(4).AlignItems(ui.Center).Children(func() {
				theme.Text(c, i18n.T("Nothing new"), "label").TextColor(theme.C("ink"))
				theme.Text(c, i18n.T("Releases you asked to hear about, and anything that needs you, show up here."), "sub").
					TextColor(theme.C("ink-muted")).TextAlign(ui.Center).FillWidth()
			})
		}
		for _, a := range acts {
			if a.Foot != nil {
				a.Foot(c)
			}
		}
	})
}

// entry is one entry: its picture (the cover, or the kind's glyph on a
// --hover tile), the words, and at the trailing end when it happened, which
// the Dismiss button replaces while the row is under the pointer or holds
// the focus. Choosing the words opens what the entry is about. A new entry
// has its title at 600 and a dot after its time.
func (f *feature) entry(c *ui.Context, n services.Notification, now time.Time) {
	w := words(n)
	tk := theme.Current()
	key := "notice:" + strconv.FormatInt(n.ID, 10)
	fresh := f.fresh[n.ID] || !n.Read
	row := ui.Row(c.Key(key)).FillWidth().Gap(4).Radius(tk.Radius["md"]).AlignItems(ui.Center).Role(ui.RoleListItem)
	hot := row.Hovered() || row.FocusWithin()
	if hot {
		row.Background(theme.C("hover"))
	}
	row.Children(func() {
		open := ui.ButtonBase(c.Key("open")).Grow(1).Shrink(1).MinWidth(0).Padding(8).Gap(12).Radius(tk.Radius["md"]).
			AlignItems(ui.Start).Justify(ui.Start).Label(w.title).Description(w.detail)
		if n.Target.Kind != "" {
			open.Cursor(ui.CursorPointer)
		}
		open.Children(func() {
			f.picture(c, n, w)
			ui.Column(c.Key("words")).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
				t := theme.Text(c, w.title, "body").TextColor(theme.C("ink")).FillWidth().MaxLines(2)
				if fresh {
					t.FontWeight(600)
				} else {
					t.FontWeight(500)
				}
				if w.detail != "" {
					theme.Text(c, w.detail, "sub").TextColor(theme.C("ink-muted")).FillWidth().MaxLines(2)
				}
			})
		})
		if open.Clicked() {
			f.follow(n.Target)
		}
		// The time sets the column's width; Remove sits over it at the end.
		ui.Box(c.Key("end")).Shrink(0).MinWidth(32).AlignSelf(ui.Stretch).Padding(10, 8, 0, 4).Children(func() {
			when := ui.Row(c.Key("when")).Gap(6).AlignItems(ui.Center).Children(func() {
				theme.Text(c, i18n.RelativeTime(time.UnixMilli(n.CreatedAt), now), "meta").FontFeatures("tnum").
					TextColor(theme.C("ink-faint")).SingleLine()
				if fresh {
					ui.Box(c.Key("new")).Size(6, 6).Radius(tk.Radius["full"]).Background(theme.C("ink")).Shrink(0)
				}
			})
			kit.Fade(when, !hot)
			// Named by the entry, so no two Remove buttons share a name.
			x := kit.IconButton(c.Key("dismiss"), "x", i18n.T("Remove “{title}”", "title", w.title), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM})
			x.Absolute().Top(6).Right(4)
			kit.Fade(x, hot)
			if x.Clicked() {
				f.dismiss(n.ID)
			}
		})
	})
}

// picture is an entry's 32px leading picture: the cover where it has one,
// else the kind's glyph on a --hover tile.
func (f *feature) picture(c *ui.Context, n services.Notification, w entryWords) {
	if n.Art.Small != "" {
		kit.Artwork(c.Key("art"), f.env.Art, n.Art.Small, 32, kit.ArtOpts{Edge: true})
		return
	}
	ui.Box(c.Key("glyph")).Size(32, 32).Shrink(0).Radius(theme.Current().Radius["md"]).Background(theme.C("hover")).Center().Children(func() {
		ui.Icon(c.Key("icon"), theme.Icon(w.icon)).FontSize(16).TextColor(theme.C("ink-muted"))
	})
}

// entryWords is how an entry reads.
type entryWords struct {
	title, detail, icon string
}

// words words an entry in the interface's language. Go keeps only the kind
// and the names.
func words(n services.Notification) entryWords {
	switch n.Kind {
	case services.NoticeReleaseOut:
		return entryWords{
			// translators: Notification: an album the person asked to be reminded of has come out.
			title:  i18n.T("{album} is out", "album", n.Subject),
			detail: n.Artist,
			icon:   "disc-3",
		}
	case services.NoticeNewReleases:
		if n.Count == 1 {
			return entryWords{title: n.Subject, detail: newReleaseLine(n.Variant, n.Artist), icon: "disc-3"}
		}
		return entryWords{
			title:  i18n.T("{n, plural, one {# new release from your artists} other {# new releases from your artists}}", "n", n.Count),
			detail: i18n.T("They're in New Releases."),
			icon:   "disc-3",
		}
	case services.NoticeAddonOff:
		return entryWords{
			title:  i18n.T("{name} was switched off", "name", n.Subject),
			detail: i18n.T("It stopped several times in a row. Settings → Add-ons has its log."),
			icon:   "puzzle",
		}
	}
	return entryWords{title: n.Subject, detail: n.Artist, icon: "bell"}
}

// newReleaseLine is the line under a new release: what it is and whose.
func newReleaseLine(kind, artist string) string {
	switch kind {
	case services.ReleaseSingle:
		return i18n.T("New single from {artist}", "artist", artist)
	case services.ReleaseEP:
		return i18n.T("New EP from {artist}", "artist", artist)
	}
	return i18n.T("New album from {artist}", "artist", artist)
}
