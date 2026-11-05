package playlist

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The cover picker's geometry, in DIPs: four tiles across the dialog's
// content (480 less its padding and border), in as many rows as the options
// need; the rows scroll when the window is too short for them.
const (
	coverTileW   = 92
	coverFrame   = 4 // the ring a chosen tile draws around its cover, and the room for it
	coverGapPx   = 16
	coverColumns = 4
	// coverChrome is what the dialog holds besides the grid: header, upload
	// row and limits, footer and padding.
	coverChrome = 330
	// coverRing is the room the scroll area leaves round the grid for the focus ring.
	coverRing = 3
	// coverDialogW is the dialog's widest.
	coverDialogW = 480
)

// keyCovers is a library playlist's cover picker; it sits under the
// playlist's own key, so whatever refreshes the playlist (a track edit moves
// its colours) refreshes it too.
func keyCovers(id string) store.Key { return append(keyDetail(id), "covers") }

// covers is a library playlist's cover options, from SQLite and the artwork
// cache.
func (p *playlists) covers(id string) store.Result[services.PlaylistCovers] {
	return store.Use(p.env.Queries, keyCovers(id), func(ctx context.Context) (services.PlaylistCovers, error) {
		return p.api.CoverOptions(ctx, id)
	}, store.Options{Stale: store.Forever})
}

// changeCover opens the cover picker of library playlist id.
func (p *playlists) changeCover(id, name string) {
	p.dialogs.show(dialogState{kind: dialogCover, id: id, name: name})
}

// coverLabel is the words for a cover option.
func coverLabel(o services.CoverOption) string {
	switch o.Kind {
	case "auto":
		// translators: A choice of playlist cover: the one Sona picks by itself (Apple's artwork, or the playlist's albums as a mosaic).
		return i18n.TC("playlist cover", "Automatic")
	case "none":
		// translators: A choice of playlist cover: no picture, the plain placeholder.
		return i18n.TC("playlist cover", "None")
	case "upload":
		// translators: A choice of playlist cover: the picture the user chose from a file.
		return i18n.T("Your image")
	}
	switch o.Style {
	case "classic":
		// translators: Name of a generated playlist cover: a diagonal gradient with the playlist's name, like the cover Apple Music makes for a new playlist.
		return i18n.T("Classic")
	case "aurora":
		// translators: Name of a generated playlist cover: soft glows of the playlist's colours on a dark ground.
		return i18n.T("Aurora")
	case "dusk":
		// translators: Name of a generated playlist cover: a slanted gradient from light to dark.
		return i18n.T("Dusk")
	case "horizon":
		// translators: Name of a generated playlist cover: a sky over three ridges of hills.
		return i18n.T("Horizon")
	case "moon":
		// translators: Name of a generated playlist cover: a large disc over a night sky.
		return i18n.T("Moon")
	case "glow":
		// translators: Name of a generated playlist cover: a bright centre fading to a dark edge.
		return i18n.T("Glow")
	case "duo":
		// translators: Name of a generated playlist cover: two tones cut by a diagonal.
		return i18n.T("Duo")
	}
	return i18n.T("Cover")
}

// coverHint is the line under a cover choice for assistive technology.
func coverHint(o services.CoverOption) string {
	switch o.Kind {
	case "auto":
		return i18n.T("Apple Music's cover, or the playlist's albums as a mosaic")
	case "none":
		return i18n.T("No picture")
	case "upload":
		return i18n.T("The image you chose")
	}
	return i18n.T("A cover drawn from the playlist's colours")
}

// coverDialog is the picker of a library playlist's cover: automatic, none,
// the user's image and six covers drawn from the playlist's colours, then
// the choice of a new image. A choice applies at once, on every surface that
// shows the playlist, so the dialog has no Save: Done closes it.
func (p *playlists) coverDialog(c *ui.Context) {
	d := &p.dialogs
	s := d.s
	q := p.covers(s.id)
	title := i18n.T("Playlist cover")
	// Where the cover lives is part of the choice: said where it is made.
	description := i18n.T("Apple Music can't keep a playlist cover from here, so your choice stays on this computer and shows in Sona only.")
	kit.Dialog(c.Key("playlist-dialog"), &d.open, kit.DialogOpts{
		Title: title, Description: description, HideHeader: true, MaxWidth: coverDialogW, Padding: 24, Gap: 20,
	}, func() {
		dialogHeader(c, title, description, false)
		switch {
		case q.HasData:
			p.coverChoices(c, q.Data)
			p.coverUpload(c, q.Data)
		case q.Err != nil:
			ui.Column(c.Key("failed")).FillWidth().Gap(12).AlignItems(ui.Start).Children(func() {
				dialogText(c.Key("message"), i18n.T("Couldn't load the covers. {reason}", "reason", failText(q.Err)), "secondary").
					TextColor(theme.C("ink-muted")).Role(ui.RoleStatus)
				if kit.Button(c.Key("retry"), i18n.T("Try again"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM}).Clicked() {
					p.env.Queries.Invalidate(keyCovers(s.id))
				}
			})
		default:
			dialogText(c.Key("loading"), i18n.T("Loading covers…"), "secondary").TextColor(theme.C("ink-muted")).Role(ui.RoleStatus)
		}
		kit.DialogFooter(c, &d.open, false, func() {
			if kit.Button(c.Key("done"), i18n.T("Done"), kit.ButtonOpts{}).Clicked() {
				d.open = false
			}
		})
		if d.coverSaid != "" {
			c.Announce(d.coverSaid)
			d.coverSaid = ""
		}
	})
}

// coverChoices is the radio group of options: a grid of covers, each with
// its name under it and a ring and a check on the chosen one (more than the
// ring's colour says it).
func (p *playlists) coverChoices(c *ui.Context, data services.PlaylistCovers) {
	d := &p.dialogs
	tk := theme.Current()
	current := data.Current
	if d.coverPick != "" {
		current = d.coverPick
	}
	d.nav.Count, d.nav.Columns = len(data.Options), coverColumnsIn(c)
	ui.Scroll(c.Key("scroll")).FillWidth().MaxHeight(listMaxHeight(c, 720, coverChrome)).Children(func() {
		ui.Row(c.Key("covers")).FillWidth().Wrap().Gap(coverGapPx).Padding(coverRing).Role(ui.RoleRadioGroup).Label(i18n.T("Cover")).Children(func() {
			for i, o := range data.Options {
				chosen := o.ID == current
				label := coverLabel(o)
				tile := ui.ButtonBase(c.Key("option:" + o.ID)).Column().Width(coverTileW).Gap(6).AlignItems(ui.Center).
					Radius(tk.Radius["md"]).Cursor(ui.CursorPointer).Role(ui.RoleRadio).Checked(chosen).Label(label).Description(coverHint(o))
				// The first stop is the chosen cover, once the dialog is in.
				if chosen && !d.focused && d.age >= 2 {
					d.focused = true
					tile.Focus()
				}
				tile.Children(func() {
					ring := ui.Box(c.Key("frame")).Size(coverTileW, coverTileW).Padding(coverFrame-2).Radius(tk.Radius["lg"]).
						Border(2, ui.Transparent)
					if chosen {
						ring.Border(2, theme.C("ink"))
					} else if tile.Hovered() {
						ring.Border(2, theme.C("control-line"))
					}
					ring.Children(func() {
						kit.Artwork(c.Key("art"), p.env.Art, o.Art.Medium, coverTileW-2*coverFrame, kit.ArtOpts{Edge: true})
					})
					ui.Row(c.Key("name")).Gap(4).AlignItems(ui.Center).MaxWidth(coverTileW).Children(func() {
						t := tk.Type["secondary"]
						if chosen {
							ui.Icon(c.Key("check"), theme.Icon("check")).FontSize(14).TextColor(theme.C("ink")).Shrink(0)
						}
						lbl := ui.Text(c.Key("label"), label).FontSize(t.Size).FixedLineHeight(t.LineHeight).SingleLine()
						if chosen {
							lbl.FontWeight(600).TextColor(theme.C("ink"))
						} else {
							lbl.FontWeight(int(t.Weight)).TextColor(theme.C("ink-muted"))
						}
					})
				})
				d.nav.Claim(tile, i)
				if tile.Clicked() && !d.busy {
					p.chooseCover(o, label)
					c.Invalidate()
				}
				d.nav.Keys(tile, i, nil)
			}
		})
	})
	d.nav.Tick(c)
}

// coverColumnsIn is how many tiles fit across the dialog's content in the
// window c draws: four unless the window is narrower than the dialog.
func coverColumnsIn(c *ui.Context) int {
	w, _ := c.Size()
	inner := min(coverDialogW, max(w-32, 0)) - 2*24 - 2*coverRing - 2
	return max(1, min(coverColumns, int((inner+coverGapPx)/(coverTileW+coverGapPx))))
}

// coverUpload is the choice of an image from a file, with the limits it
// has to meet beside it, and the way to delete the one kept. Where the
// platform has no file chooser it says so instead of showing a dead button.
func (p *playlists) coverUpload(c *ui.Context, data services.PlaylistCovers) {
	d := &p.dialogs
	hasUpload := false
	for _, o := range data.Options {
		hasUpload = hasUpload || o.Kind == "upload"
	}
	limits := i18n.T("JPEG or PNG, up to {maxMiB} MB and {maxMpx} megapixels, at least {minEdge} px wide and tall. Sona crops it to a square from the centre.",
		"maxMiB", data.Limits.MaxBytes>>20, "maxMpx", data.Limits.MaxMpx, "minEdge", data.Limits.MinEdge)
	ui.Column(c.Key("upload")).FillWidth().Gap(10).AlignItems(ui.Start).Children(func() {
		if !data.CanUpload {
			dialogText(c.Key("unavailable"), i18n.T("Choosing an image needs a file chooser, and this system has none."), "secondary").
				TextColor(theme.C("ink-muted"))
			p.coverError(c)
			return
		}
		ui.Row(c.Key("buttons")).Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
			label := i18n.T("Choose an image…")
			if d.busy {
				label = i18n.T("Preparing your image…")
			}
			choose := kit.Button(c.Key("choose"), label, kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Icon: "upload", Disabled: d.busy})
			choose.Description(limits)
			if choose.Clicked() {
				p.uploadCover()
				c.Invalidate()
			}
			if hasUpload {
				remove := kit.Button(c.Key("remove"), i18n.T("Remove my image"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Icon: "trash", Disabled: d.busy})
				if remove.Clicked() {
					p.removeCoverImage()
					c.Invalidate()
				}
			}
		})
		// The limits stay in view beside the button, not behind a failure.
		dialogText(c.Key("limits"), limits, "meta").TextColor(theme.C("ink-faint"))
		p.coverError(c)
	})
}

// coverError is what the last change could not do, a status beside the
// controls (where the limits it may have broken are), announced once.
func (p *playlists) coverError(c *ui.Context) {
	d := &p.dialogs
	if d.coverErr == "" {
		return
	}
	errText := dialogText(c.Key("error"), d.coverErr, "secondary").TextColor(theme.C("ink")).Role(ui.RoleStatus)
	if said := ui.Local(errText, "said", func() string { return "" }); *said != d.coverErr {
		*said = d.coverErr
		c.Announce(d.coverErr)
	}
}

// chooseCover applies option o as the open dialog's playlist's cover: the
// choice shows at once and is taken back, with the reason, when it fails.
func (p *playlists) chooseCover(o services.CoverOption, label string) {
	d := &p.dialogs
	id, gen := d.s.id, d.gen
	d.coverPick, d.coverErr = o.ID, ""
	d.busy = true
	go func() {
		got, err := p.api.SetCover(context.Background(), id, o.ID)
		p.env.Post(func() {
			p.coverDone(gen, id, got, err, i18n.T("Couldn't change the cover"), i18n.T("Cover: {name}", "name", label))
		})
	}()
}

// uploadCover asks for an image (the desktop's file chooser) and makes it
// the open dialog's playlist's cover.
func (p *playlists) uploadCover() {
	d := &p.dialogs
	id, gen := d.s.id, d.gen
	prev, prevArt := "", ""
	if q := p.covers(id); q.HasData {
		prev, prevArt = q.Data.Current, uploadArt(q.Data)
	}
	d.coverErr = ""
	d.busy = true
	go func() {
		got, err := p.api.UploadCover(context.Background(), id)
		p.env.Post(func() {
			said := ""
			// A cancelled chooser changes neither; a replaced image changes the
			// picture of the same option.
			if err == nil && (got.Current != prev || uploadArt(got) != prevArt) {
				said = i18n.T("Your image is now the cover")
			}
			p.coverDone(gen, id, got, err, "", said)
		})
	}()
}

// uploadArt is the picture of the user's image in covers ("" without one).
func uploadArt(c services.PlaylistCovers) string {
	for _, o := range c.Options {
		if o.Kind == "upload" {
			return o.Art.Large
		}
	}
	return ""
}

// removeCoverImage deletes the open dialog's playlist's kept image.
func (p *playlists) removeCoverImage() {
	d := &p.dialogs
	id, gen := d.s.id, d.gen
	d.coverErr = ""
	d.busy = true
	go func() {
		got, err := p.api.RemoveCoverUpload(context.Background(), id)
		p.env.Post(func() {
			p.coverDone(gen, id, got, err, "", i18n.T("Your image was removed"))
		})
	}()
}

// coverDone takes the answer of a cover call on the main thread: the new
// options replace the cached ones, or the reason shows beside the controls
// (in a toast when the dialog has gone and what names the step).
func (p *playlists) coverDone(gen int, id string, got services.PlaylistCovers, err error, what, said string) {
	d := &p.dialogs
	if err == nil {
		store.Set(p.env.Queries, keyCovers(id), got)
	} else {
		p.env.Queries.Invalidate(keyCovers(id))
	}
	if d.gen != gen {
		if err != nil && what != "" {
			p.fail(what, err)
		}
		return
	}
	d.busy = false
	d.coverPick = ""
	switch {
	case err != nil && d.open:
		d.coverErr = failText(err)
	case err != nil && what != "":
		p.fail(what, err)
	case err == nil:
		d.coverSaid = said
	}
}
