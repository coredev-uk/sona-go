package library

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// errorBlock is ErrorBlock: what failed, Go's reason in the product's
// words, and a Try again button, inside a dashed block.
func (l *library) errorBlock(c *ui.Context, err error) {
	tk := theme.Current()
	ui.Row(c).Gap(12).Padding(16, 20).Radius(tk.Radius["lg"]).Border(1, theme.C("control-line")).Role(ui.RoleStatus).Children(func() {
		ui.Icon(c, theme.Icon("circle-alert")).FontSize(16).TextColor(theme.C("danger")).Margin(2, 0, 0, 0)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, i18n.T("Couldn't read your albums from this computer")).FontSize(tk.Type["body"].Size).
				FixedLineHeight(tk.Type["body"].LineHeight).FontWeight(500).TextColor(theme.C("ink"))
			ui.Text(c, i18n.ErrorText(services.CodeOf(err))).FontSize(tk.Type["secondary"].Size).
				FixedLineHeight(tk.Type["secondary"].LineHeight).TextColor(theme.C("ink-muted")).Margin(2, 0, 0, 0).Wrap()
		})
		if kit.Button(c.Key("retry"), i18n.T("Try again"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).Clicked() {
			l.env.Queries.Invalidate(store.Key{"library", "albums"})
		}
	})
}

// empty is what the page shows in place of the grid when the list has no
// albums (useLibraryGate and the filtered-empty state): one row of empty
// cover slots, a title, one sentence. A type with nothing in it keeps the
// page and says so, with Show all albums as its one action.
func (l *library) empty(c *ui.Context, q albumsQuery, allAlbums int, gutter float32) {
	var title, body, action string
	switch {
	case q.kind != "" && allAlbums > 0:
		title = i18n.T("No albums besides singles and EPs")
		body = i18n.T("Everything in your library is a single or an EP.")
		if q.kind == "singles" {
			title = i18n.T("No singles or EPs in your library")
			body = i18n.T("Singles and EPs you add in Apple Music appear here after the next sync.")
		}
		action = i18n.T("Show all albums")
	default:
		auth := store.Use(l.env.Queries, keyAuth, func(context.Context) (services.AuthStatus, error) { return l.auth.Status(), nil })
		status := store.Use(l.env.Queries, keySync, l.lib.SyncStatus)
		if !auth.HasData || auth.Data.State == "restoring" {
			return
		}
		running := status.Data.Running
		if l.events > 0 {
			running = l.syncing
		}
		switch {
		case signedOut(auth.Data.State):
			title = i18n.T("Sign in to see your albums")
			body = i18n.T("Sona copies your Apple Music library to this computer after you sign in, so it opens instantly and works offline.")
		case running || status.Data.LastSuccess == 0:
			title = i18n.T("Your library is syncing")
			body = i18n.T("Your albums appear here as Sona copies them from Apple Music. Large libraries take a minute or two the first time.")
		default:
			title = i18n.T("No albums in your library yet")
			body = i18n.T("Albums you add in Apple Music appear here after the next sync, newest first.")
		}
	}

	tk := theme.Current()
	ui.Column(c).Gap(4).AlignItems(ui.Start).Children(func() {
		// One row of empty slots at the grid's real tile size.
		pg := &l.albums
		gapX, minTile := tk.Metric["tile-gap-x"], tk.Metric["tile-min"]
		inner := pg.width - 2*gutter - tk.Metric["scrollbar"]
		cols := max(1, int((inner+gapX)/(minTile+gapX)))
		tile := (inner - gapX*float32(cols-1)) / float32(cols)
		ui.Row(c).Gap(gapX).Margin(0, 0, 20, 0).Children(func() {
			for i := range cols {
				ui.Box(c.Key(i)).Size(tile, tile).Radius(tk.Radius["sm"]).Border(1, theme.C("control-line"))
			}
		})
		ui.Text(c, title).FontSize(tk.Type["heading"].Size).FixedLineHeight(tk.Type["heading"].LineHeight).
			FontWeight(int(tk.Type["heading"].Weight)).TextColor(theme.C("ink"))
		ui.Text(c, body).FontSize(tk.Type["body"].Size).FixedLineHeight(tk.Type["body"].LineHeight).
			TextColor(theme.C("ink-muted")).MaxWidth(60 * 8).Wrap()
		if action != "" {
			if kit.Button(c.Key("all"), action, kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM}).Margin(16, 0, 0, 0).Clicked() {
				next := q
				next.kind = ""
				l.env.Nav.Replace(next.location())
			}
		}
	})
}

// signedOut is isSignedOut: no account, or a sign-in still running; not
// while Go reads the sign-in from the keyring, nor when Apple only asks to
// sign in again (the library is still here).
func signedOut(state string) bool {
	switch state {
	case "signed_out", "signing_in", "error":
		return true
	}
	return false
}
