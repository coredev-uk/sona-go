package playlist

import (
	"context"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// dialogKind is which playlist dialog is open (dialogs.ts's PlaylistDialog).
type dialogKind int

const (
	// dialogAdd picks a playlist to add items to; label names what is added.
	dialogAdd dialogKind = iota
	// dialogCreate makes a playlist holding items, in folder folderID ("" is
	// the top level).
	dialogCreate
	// dialogDetails edits the name, description and visibility of library
	// playlist id.
	dialogDetails
	// dialogFolder makes a folder, or renames folder.
	dialogFolder
	// dialogMove moves library playlist id (named name) from folder folderID
	// to another.
	dialogMove
	// dialogCover picks the cover of library playlist id (named name).
	dialogCover
)

// dialogState is the dialog to show and what it was opened with.
type dialogState struct {
	kind  dialogKind
	items []services.PlaylistItem
	label string
	// folderID is the folder a new playlist is created in ("" the top
	// level); for dialogMove, the playlist's current folder.
	folderID    string
	id, name    string
	description string
	isPublic    bool
	// folder is the folder being renamed (nil: a new one).
	folder *services.PlaylistFolder
}

// dialogs is the playlist dialogs: one at a time, the form of the open one
// reset each time one is shown (PlaylistDialogs.tsx). The zero value is
// closed and ready. Main thread only, like every view.
type dialogs struct {
	s    dialogState
	open bool
	// gen counts the dialogs shown: a callback that comes back after its
	// dialog went (a create that took a while) knows by it.
	gen int
	// age counts the frames the open dialog was built: the field takes
	// the focus on the second, after MyGo noted what to give it back to.
	age int

	// The forms: the name, description and visibility of a playlist, or the
	// name of a folder; busy while a request is out.
	name, description string
	public            bool
	busy              bool
	// query filters the add picker's playlists.
	query string

	// The cover picker: the option chosen and not yet confirmed ("" none),
	// what a failed upload says, what to announce when a change is done, and
	// whether the first stop took the focus.
	coverPick, coverErr, coverSaid string
	focused                        bool
	// nav is the keyboard navigation of a picker's options.
	nav kit.GridNav

	// seen is the tree nodes was built from: the pickers rebuild only when
	// the query hands them another.
	seen  services.PlaylistTree
	nodes []*treeNode
}

// show opens dialog s, replacing the one that is open.
func (d *dialogs) show(s dialogState) {
	*d = dialogs{
		s: s, open: true, gen: d.gen + 1,
		nav: kit.GridNav{Count: 1, Columns: 1, Ends: true},
	}
	switch s.kind {
	case dialogDetails:
		d.name, d.description, d.public = s.name, s.description, s.isPublic
	case dialogFolder:
		if s.folder != nil {
			d.name = s.folder.Name
		}
	}
}

// closeGen closes the open dialog, if dialog gen is still the one shown (a
// request that finishes after the user moved on leaves the next one alone).
func (d *dialogs) closeGen(gen int) {
	if d.open && d.gen == gen {
		d.open = false
	}
}

// release ends dialog gen's request: its form is usable again.
func (d *dialogs) release(gen int) {
	if d.gen == gen {
		d.busy = false
	}
}

// treeNodes is the folder tree of t, as the pickers list it (no playlists
// left out: v0.2.0 passed no hidden set), built again only when the query
// gave another tree.
func (d *dialogs) treeNodes(t services.PlaylistTree) []*treeNode {
	if d.nodes == nil || !sameTree(d.seen, t) {
		d.seen, d.nodes = t, buildTree(t, nil)
	}
	return d.nodes
}

// sameTree reports whether a and b are the same data (the query hands out
// one slice until it fetches again).
func sameTree(a, b services.PlaylistTree) bool {
	if len(a.Folders) != len(b.Folders) || len(a.Playlists) != len(b.Playlists) {
		return false
	}
	if len(a.Folders) > 0 && &a.Folders[0] != &b.Folders[0] {
		return false
	}
	return len(a.Playlists) == 0 || &a.Playlists[0] == &b.Playlists[0]
}

// Maximum lengths of the fields (v0.2.0's maxLength).
const (
	maxNameRunes        = 200
	maxDescriptionRunes = 1000
)

func limitRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// dialogHost builds the open dialog (the Aside slot: an overlay, it takes no
// room in the layout, and builds nothing while closed).
func (p *playlists) dialogHost(c *ui.Context) {
	d := &p.dialogs
	if !d.open {
		return
	}
	d.age++
	if d.age == 1 {
		c.Invalidate()
	}
	switch d.s.kind {
	case dialogAdd:
		p.addDialog(c)
	case dialogCreate, dialogDetails:
		p.playlistDialog(c)
	case dialogFolder:
		p.folderDialog(c)
	case dialogMove:
		p.moveDialog(c)
	case dialogCover:
		p.coverDialog(c)
	}
}

// dialogText is a Text in one of the theme's type styles.
func dialogText(c *ui.Context, s, style string) ui.Element {
	t := theme.Current().Type[style]
	return ui.Text(c, s).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).LetterSpacing(t.Tracking)
}

// dialogHeader is a dialog's title (16 semibold) and the muted line under
// it (v0.2.0's gap-1 DialogHeader with text-heading and text-sub); oneLine
// keeps a long description (a playlist's name) to one line.
func dialogHeader(c *ui.Context, title, description string, oneLine bool) {
	ui.Column(c.Key("header")).Gap(4).FillWidth().MinWidth(0).Children(func() {
		dialogText(c.Key("title"), title, "heading").TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(2)
		d := dialogText(c.Key("description"), description, "secondary").TextColor(theme.C("ink-muted"))
		if oneLine {
			d.SingleLine()
		}
	})
}

// dialogField is a labelled control: the label 14 medium, 6 above it; a
// click on the label focuses the control (v0.2.0's <label>).
func dialogField(c *ui.Context, key, label string, control func() ui.Element) {
	ui.Column(c.Key(key)).Gap(6).FillWidth().Children(func() {
		l := ui.Row(c.Key("label"))
		l.Children(func() {
			dialogText(c, label, "label").TextColor(theme.C("ink"))
		})
		if field := control(); l.Clicked() {
			field.Focus()
		}
	})
}

// listMaxHeight is how tall a picker's list may be in a dialog of at most
// maxH (min(maxH, 85% of the window)), less its header and field.
func listMaxHeight(c *ui.Context, maxH, chrome float32) float32 {
	_, wh := c.Size()
	return max(min(maxH, wh*0.85)-chrome, 96)
}

// option is one row of a picker: an icon or a cover, a title, a line under
// it, and the check of the current choice.
type option struct {
	key, title, meta string
	// icon is a Lucide name for a row with no cover (art is used when it is
	// empty).
	icon, art string
	current   bool
	// focus gives the row the focus when it is built.
	focus bool
	pick  func()
}

// optionRow builds picker row i: 44 high, hover and keyboard focus tint it,
// and Up, Down, Home and End move among the rows (d.nav).
func (p *playlists) optionRow(c *ui.Context, i int, o option) {
	d := &p.dialogs
	tk := theme.Current()
	s := ui.ButtonBase(c.Key(o.key)).Row().AlignItems(ui.Center).FillWidth().Height(44).Padding(0, 8).Gap(12).
		Radius(tk.Radius["md"]).Cursor(ui.CursorPointer).Label(o.title)
	// The row is one piece for assistive technology: the line under the
	// title and the check of the current choice ride in its description.
	switch {
	case o.meta != "" && o.current:
		s.Description(o.meta + " · " + i18n.T("Current"))
	case o.current:
		s.Description(i18n.T("Current"))
	case o.meta != "":
		s.Description(o.meta)
	}
	if o.focus && d.age == 2 {
		s.Focus()
	}
	if s.Hovered() || s.FocusVisible() {
		s.Background(theme.C("hover"))
	}
	s.Children(func() {
		if o.icon != "" {
			ui.Box(c.Key("icon")).Size(32, 32).Shrink(0).Center().Radius(tk.Radius["sm"]).Background(theme.C("hover")).Children(func() {
				theme.Glyph(c, o.icon, 16, theme.GlyphOpts{Color: theme.C("ink-muted")})
			})
		} else {
			kit.Artwork(c.Key("art"), p.env.Art, o.art, 32, kit.ArtOpts{Edge: true}).Shrink(0)
		}
		ui.Column(c.Key("text")).Grow(1).MinWidth(0).Children(func() {
			dialogText(c.Key("title"), o.title, "body").TextColor(theme.C("ink")).SingleLine()
			if o.meta != "" {
				dialogText(c.Key("meta"), o.meta, "meta").TextColor(theme.C("ink-faint")).SingleLine()
			}
		})
		if o.current {
			// Inside the row, which is one piece for assistive technology:
			// the description above says it; the label finds the mark.
			ui.Icon(c.Key("current"), theme.Icon("check")).FontSize(16).TextColor(theme.C("ink-muted")).Shrink(0).Label(i18n.T("Current"))
		}
	})
	d.nav.Claim(s, i)
	if s.Clicked() {
		o.pick()
		c.Invalidate()
	}
	d.nav.Keys(s, i, nil)
}

// groupHeading is the small muted heading over a group of options: a
// folder's path with its icon, or a section's name.
func groupHeading(c *ui.Context, key, text string, folder bool) {
	ui.Row(c.Key(key)).Padding(16, 8, 4, 8).Gap(6).AlignItems(ui.Center).Children(func() {
		if folder {
			ui.Icon(c.Key("icon"), theme.Icon("folder-closed")).FontSize(12).TextColor(theme.C("ink-faint"))
		}
		dialogText(c.Key("text"), text, "meta").TextColor(theme.C("ink-faint")).Role(ui.RoleHeading).Level(3)
	})
}

// songCount is "N songs".
func songCount(n int) string {
	return i18n.T("{n, plural, one {# song} other {# songs}}", "n", n)
}

// addDialog is the "Add to playlist" picker: a search field, a "New playlist"
// row, Apple's recent targets (when not filtering), then the playlists the
// user can edit, grouped by folder (PlaylistDialogs.tsx's AddToPlaylist).
func (p *playlists) addDialog(c *ui.Context) {
	d := &p.dialogs
	s := d.s
	const chrome = 120 // the header and the search field
	tree, recent := p.tree(), p.recentTargets()
	pending := !tree.HasData && tree.Err == nil
	var nodes []*treeNode
	if tree.HasData {
		nodes = d.treeNodes(tree.Data)
	}
	kit.Dialog(c.Key("playlist-dialog"), &d.open, kit.DialogOpts{
		Title: i18n.T("Add to playlist"), Description: s.label, HideHeader: true, Bare: true, MaxWidth: 448,
	}, func() {
		ui.Column(c.Key("head")).FillWidth().Padding(20, 20, 12, 20).Children(func() {
			dialogHeader(c, i18n.T("Add to playlist"), s.label, true)
		})
		var find ui.Element
		ui.Box(c.Key("find")).FillWidth().Padding(0, 20, 12, 20).Children(func() {
			find = kit.SearchField(c.Key("query"), &d.query, kit.SearchFieldOpts{Placeholder: i18n.T("Find a playlist")})
			if d.age == 2 {
				find.Focus()
			}
		})
		if find.Shortcut(0, ui.KeyDown) {
			d.nav.Want(0)
		}
		recents, groups := addTargets(nodes, recent.Data, d.query)
		d.nav.Count = 1
		for _, g := range groups {
			d.nav.Count += len(g.Items)
		}
		d.nav.Count += len(recents)

		add := func(tp services.TreePlaylist) func() {
			return func() {
				d.open = false
				p.addToPlaylistNow(tp.Playlist.ID, tp.Playlist.Name, s.items, s.label)
			}
		}
		row := func(prefix string, i int, tp services.TreePlaylist) {
			p.optionRow(c, i, option{
				key: prefix + tp.Playlist.ID, title: tp.Playlist.Name, art: tp.Playlist.Art.Small,
				meta: songCount(tp.Playlist.TrackCount), pick: add(tp),
			})
		}
		ui.Scroll(c.Key("list")).FillWidth().MaxHeight(listMaxHeight(c, 640, chrome)).Children(func() {
			ui.Column(c.Key("options")).FillWidth().Padding(0, 12, 12, 12).Role(ui.RoleGroup).Children(func() {
				i := 0
				p.optionRow(c, i, option{
					key: "new", title: i18n.T("New playlist"), icon: "plus",
					pick: func() {
						d.show(dialogState{kind: dialogCreate, items: s.items, label: s.label})
					},
				})
				i++
				if len(recents) > 0 {
					groupHeading(c, "recent", i18n.T("Recent"), false)
					for _, tp := range recents {
						row("recent:", i, tp)
						i++
					}
				}
				for _, g := range groups {
					title, folder := g.Path, g.Path != ""
					if !folder {
						title = i18n.T("Playlists")
					}
					groupHeading(c, "group:"+g.Path, title, folder)
					for _, tp := range g.Items {
						row("in:", i, tp)
						i++
					}
				}
				if len(recents) == 0 && len(groups) == 0 {
					var msg string
					switch q := strings.TrimSpace(d.query); {
					case q != "":
						msg = i18n.T("No playlist you can edit is called “{trimmed}”.", "trimmed", q)
					case pending:
						msg = i18n.T("Loading your playlists…")
					default:
						msg = i18n.T("You have no playlists you can edit yet.")
					}
					// A status, read as the filter or the load changes what
					// it says; the note turning up is announced here, as a
					// status is not read when it first appears. The focus
					// stays in the field.
					note := dialogText(c.Key("empty"), msg, "secondary").TextColor(theme.C("ink-muted")).Padding(16, 8, 0, 8).Role(ui.RoleStatus)
					kit.AnnounceMounted(c, note, msg)
				}
			})
		})
		d.nav.Tick(c)
	})
}

// playlistDialog is the form of a new playlist (dialogCreate) and of a
// library playlist's details (dialogDetails): name, description, and
// whether it shows on the user's profile (PlaylistDialogs.tsx's
// PlaylistForm).
func (p *playlists) playlistDialog(c *ui.Context) {
	d := &p.dialogs
	s, gen := d.s, d.gen
	editing := s.kind == dialogDetails
	title, description := i18n.T("New playlist"), i18n.T("Add songs from any album, playlist or search result.")
	switch {
	case editing:
		title, description = i18n.T("Edit playlist"), i18n.T("Changes appear in Apple Music on all your devices.")
	case len(s.items) > 0:
		description = i18n.T("With {withLabel}.", "withLabel", s.label)
	}
	kit.Dialog(c.Key("playlist-dialog"), &d.open, kit.DialogOpts{
		Title: title, Description: description, HideHeader: true, MaxWidth: 448, Padding: 24, Gap: 20,
	}, func() {
		dialogHeader(c, title, description, false)
		var name ui.Element
		dialogField(c, "name", i18n.T("Name"), func() ui.Element {
			name = kit.Input(c.Key("name-"+strconv.Itoa(gen)), &d.name, kit.InputOpts{A11yLabel: i18n.T("Name")}).FillWidth()
			if d.age == 2 {
				name.Focus()
			}
			d.name = limitRunes(d.name, maxNameRunes)
			return name
		})
		dialogField(c, "description", i18n.T("Description"), func() ui.Element {
			area := kit.TextArea(c.Key("description-"+strconv.Itoa(gen)), &d.description, kit.TextAreaOpts{
				Placeholder: i18n.T("Optional"), A11yLabel: i18n.T("Description"), Rows: 3,
			})
			d.description = limitRunes(d.description, maxDescriptionRunes)
			return area
		})
		ui.Row(c.Key("public")).FillWidth().AlignItems(ui.Center).Gap(16).Children(func() {
			text := ui.Column(c.Key("text")).Grow(1).MinWidth(0)
			text.Children(func() {
				dialogText(c.Key("title"), i18n.T("Show on my profile"), "label").TextColor(theme.C("ink"))
				dialogText(c.Key("hint"), i18n.T("People who follow you can find and play it."), "meta").TextColor(theme.C("ink-faint"))
			})
			if text.Clicked() {
				d.public = !d.public
			}
			kit.Switch(c.Key("public-switch-"+strconv.Itoa(gen)), &d.public, kit.SwitchOpts{
				A11yLabel: i18n.T("Show on my profile"),
			}).Shrink(0)
		})
		label := i18n.T("Create playlist")
		switch {
		case editing && d.busy:
			label = i18n.T("Saving…")
		case editing:
			label = i18n.T("Save")
		case d.busy:
			label = i18n.T("Creating…")
		}
		blocked := strings.TrimSpace(d.name) == "" || d.busy
		submit := false
		kit.DialogFooter(c, &d.open, false, func() {
			if kit.Button(c.Key("cancel"), i18n.T("Cancel"), kit.ButtonOpts{Variant: kit.Ghost}).Clicked() {
				d.open = false
			}
			if kit.Button(c.Key("submit"), label, kit.ButtonOpts{Disabled: blocked}).Clicked() {
				submit = true
			}
		})
		if name.Submitted() {
			submit = true
		}
		if submit && !blocked {
			p.submitPlaylist()
			c.Invalidate()
		}
	})
}

// submitPlaylist saves the details or creates the playlist the open form
// describes, off the main thread.
func (p *playlists) submitPlaylist() {
	d := &p.dialogs
	s, gen := d.s, d.gen
	in := services.PlaylistInput{Name: d.name, Description: d.description, IsPublic: d.public}
	d.busy = true
	if s.kind == dialogDetails {
		p.run(i18n.T("Couldn't save the playlist"),
			func(ctx context.Context) error { return p.api.Update(ctx, s.id, in) },
			func(ok bool) {
				d.release(gen)
				if ok {
					p.env.Queries.Invalidate(keyDetail(s.id))
					d.closeGen(gen)
				}
			})
		return
	}
	go func() {
		created, err := p.api.Create(context.Background(), in, s.items, s.folderID)
		p.env.Post(func() {
			if err != nil {
				p.fail(i18n.T("Couldn't create the playlist"), err)
				d.release(gen)
				return
			}
			d.closeGen(gen)
			if len(s.items) == 0 {
				p.openPlaylist(created.ID)
				return
			}
			kit.ShowToast(kit.ToastSpec{
				Title:    i18n.T("Added {label} to “{pname}”", "label", s.label, "pname", created.Name),
				Action:   i18n.T("Open"),
				OnAction: func() { p.openPlaylist(created.ID) },
			})
		})
	}()
}

// folderDialog is the form of a new folder, or of renaming one
// (PlaylistDialogs.tsx's FolderForm).
func (p *playlists) folderDialog(c *ui.Context) {
	d := &p.dialogs
	s, gen := d.s, d.gen
	renaming := s.folder != nil
	title, submitLabel := i18n.T("New folder"), i18n.T("Create folder")
	if renaming {
		title, submitLabel = i18n.T("Rename folder"), i18n.T("Rename")
	}
	description := i18n.T("Folders group playlists in the sidebar on all your devices.")
	kit.Dialog(c.Key("playlist-dialog"), &d.open, kit.DialogOpts{
		Title: title, Description: description, HideHeader: true, MaxWidth: 384, Padding: 24, Gap: 20,
	}, func() {
		dialogHeader(c, title, description, false)
		var name ui.Element
		dialogField(c, "name", i18n.T("Name"), func() ui.Element {
			name = kit.Input(c.Key("name-"+strconv.Itoa(gen)), &d.name, kit.InputOpts{A11yLabel: i18n.T("Name")}).FillWidth()
			if d.age == 2 {
				name.Focus()
			}
			d.name = limitRunes(d.name, maxNameRunes)
			return name
		})
		blocked := strings.TrimSpace(d.name) == "" || d.busy
		submit := false
		kit.DialogFooter(c, &d.open, false, func() {
			if kit.Button(c.Key("cancel"), i18n.T("Cancel"), kit.ButtonOpts{Variant: kit.Ghost}).Clicked() {
				d.open = false
			}
			if kit.Button(c.Key("submit"), submitLabel, kit.ButtonOpts{Disabled: blocked}).Clicked() {
				submit = true
			}
		})
		if name.Submitted() {
			submit = true
		}
		if submit && !blocked {
			p.submitFolder()
			c.Invalidate()
		}
	})
}

// submitFolder creates or renames the folder the open form describes, off the
// main thread.
func (p *playlists) submitFolder() {
	d := &p.dialogs
	s, gen, name := d.s, d.gen, d.name
	d.busy = true
	what, call := i18n.T("Couldn't create the folder"), func(ctx context.Context) error {
		_, err := p.api.CreateFolder(ctx, name)
		return err
	}
	if s.folder != nil {
		id := s.folder.ID
		what, call = i18n.T("Couldn't rename the folder"), func(ctx context.Context) error {
			return p.api.RenameFolder(ctx, id, name)
		}
	}
	p.run(what, call, func(ok bool) {
		d.release(gen)
		if ok {
			d.closeGen(gen)
		}
	})
}

// moveDialog lists "Top level" and every folder by path, the current one
// checked (PlaylistDialogs.tsx's MoveToFolder).
func (p *playlists) moveDialog(c *ui.Context) {
	d := &p.dialogs
	s := d.s
	const chrome = 76 // the header
	tree := p.tree()
	var folders []folderPath
	if tree.HasData {
		folders = folderPaths(d.treeNodes(tree.Data), "")
	}
	settled := tree.HasData || tree.Err != nil
	move := func(folderID, folder string) {
		d.open = false
		if folderID == s.folderID {
			return
		}
		p.run(i18n.T("Couldn't move “{name}”", "name", s.name),
			func(ctx context.Context) error { return p.api.MoveToFolder(ctx, s.id, folderID) },
			func(ok bool) {
				switch {
				case !ok:
				case folderID == "":
					kit.ShowToast(kit.ToastSpec{Title: i18n.T("Moved “{name}” to the top level", "name", s.name)})
				default:
					kit.ShowToast(kit.ToastSpec{Title: i18n.T("Moved “{name}” to “{folder}”", "name", s.name, "folder", folder)})
				}
			})
	}
	kit.Dialog(c.Key("playlist-dialog"), &d.open, kit.DialogOpts{
		Title: i18n.T("Move to folder"), Description: s.name, HideHeader: true, Bare: true, MaxWidth: 384,
	}, func() {
		ui.Column(c.Key("head")).FillWidth().Padding(20, 20, 12, 20).Children(func() {
			dialogHeader(c, i18n.T("Move to folder"), s.name, true)
		})
		d.nav.Count = 1 + len(folders)
		ui.Scroll(c.Key("list")).FillWidth().MaxHeight(listMaxHeight(c, 560, chrome)).Children(func() {
			ui.Column(c.Key("options")).FillWidth().Padding(0, 12, 12, 12).Role(ui.RoleGroup).Children(func() {
				p.optionRow(c, 0, option{
					key: "top", title: i18n.T("Top level"), icon: "list-music", current: s.folderID == "", focus: true,
					pick: func() { move("", "") },
				})
				for i, f := range folders {
					p.optionRow(c, i+1, option{
						key: "folder:" + f.Folder.ID, title: f.Path, icon: "folder-closed", current: f.Folder.ID == s.folderID,
						pick: func() { move(f.Folder.ID, f.Folder.Name) },
					})
				}
				if len(folders) == 0 && settled {
					dialogText(c.Key("empty"), i18n.T("You have no folders yet. Create one from the sidebar’s Playlists menu."), "secondary").
						TextColor(theme.C("ink-muted")).Padding(12, 8, 0, 8)
				}
			})
		})
		d.nav.Tick(c)
	})
}
