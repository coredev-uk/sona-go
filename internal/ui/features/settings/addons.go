package settings

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/addons"
	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Settings → Add-ons (docs/addons.md): a note on what an add-on is, the
// folder to put one in, then each add-on under a heading of its own with its
// switch, permissions, settings rows and log. Sona draws every word of it:
// the manifest's labels are plain text, cleaned by the service and again
// here, always beside the add-on's name.

// addonsAPI is what the section asks of AddonsService.
type addonsAPI interface {
	List() []services.AddonInfo
	Contributions(locale string) []services.AddonContribution
	Enable(id string, approve []string) error
	Disable(id string) error
	Revoke(id, permission string) error
	Log(id string) ([]string, error)
	SetValue(id, row, value string) error
	SetSecret(id, row, value string) error
	OpenFolder() error
}

// Query keys of the add-ons: the list (kept by addons:changed) and the rows
// with their values (read again when it changes or a value is saved).
var (
	keyAddons    = store.Key{"addons", "list"}
	keyAddonRows = store.Key{"addons", "rows"}
)

// addonsStale is how long the list counts as fresh when the page is built
// again: a folder put in the add-ons directory shows when the person comes
// back to it, without an event (Sona doesn't watch the folder).
const addonsStale = 2 * time.Second

// addonList is the add-ons as Go lists them.
func (s *settings) addonList() store.Result[[]services.AddonInfo] {
	return store.Use(s.env.Queries, keyAddons, func(context.Context) ([]services.AddonInfo, error) {
		if s.addons == nil {
			return nil, nil
		}
		return s.addons.List(), nil
	}, store.Options{Stale: addonsStale})
}

// addonRows is every valid add-on's settings rows, in this language.
func (s *settings) addonRows() store.Result[[]services.AddonContribution] {
	return store.Use(s.env.Queries, keyAddonRows, func(context.Context) ([]services.AddonContribution, error) {
		if s.addons == nil {
			return nil, nil
		}
		return s.addons.Contributions(i18n.Locale()), nil
	}, store.Options{Stale: addonsStale})
}

// addonsUI is the section's state that outlives a frame.
type addonsUI struct {
	// approving is the add-on whose approval panel is open.
	approving string
	// busy is the add-ons with a call in flight; errs the last failure of
	// each, in words.
	busy map[string]bool
	errs map[string]string
	// logOpen and logs are the logs shown and what they last read.
	logOpen map[string]bool
	logs    map[string]*addonLog
	// drafts are the text of a row's field (or the value a switch or select
	// was just set to), by "<add-on>/<row>"; seen is the value the row had
	// when the draft was taken, so a value that changed elsewhere replaces it.
	drafts map[string]*string
	seen   map[string]string
	// focus puts the focus on Cancel when the approval panel opens, so Escape
	// and Enter act on the panel and the switch stays where it was.
	focus kit.FocusStep
	// folderBusy: Open add-ons folder is in flight.
	folderBusy bool
}

type addonLog struct {
	lines   []string
	loading bool
	err     bool
}

func (u *addonsUI) init() {
	if u.busy == nil {
		u.busy, u.errs, u.logOpen = map[string]bool{}, map[string]string{}, map[string]bool{}
		u.logs, u.drafts, u.seen = map[string]*addonLog{}, map[string]*string{}, map[string]string{}
	}
}

// draft is the text of a row's field: what the row holds, until the person
// types over it.
func (u *addonsUI) draft(key, value string) *string {
	if d, ok := u.drafts[key]; ok && u.seen[key] == value {
		return d
	}
	d := new(string)
	*d = value
	u.drafts[key], u.seen[key] = d, value
	return d
}

// permissionText is what a permission is called and what it allows.
func permissionText(p string) (name, allows string) {
	switch p {
	case cli.PermPlaybackRead:
		return i18n.T("Playback state"), i18n.T("Sees which song plays, its progress and whether it is paused.")
	case cli.PermPlaybackControl:
		return i18n.T("Playback controls"), i18n.T("Can play, pause, skip and go back.")
	case cli.PermQueueRead:
		return i18n.T("The queue"), i18n.T("Can read the songs that are queued.")
	case cli.PermLyricsRead:
		return i18n.T("Lyrics"), i18n.T("Can read the lyrics of the song that plays.")
	case cli.PermSearch:
		return i18n.T("Search"), i18n.T("Can search Apple Music's catalogue.")
	case cli.PermOpen:
		return i18n.T("Open Apple Music links"), i18n.T("Can ask Sona to open an Apple Music page.")
	case cli.PermVolume:
		return i18n.T("Volume"), i18n.T("Can read and change the volume.")
	case cli.PermLike:
		return i18n.T("Likes"), i18n.T("Can love or unlove the song that plays.")
	case cli.PermPrivateRead:
		return i18n.T("Private listening, read only"), i18n.T("Can see whether private listening is on. It never hears a private listen and can't change the setting.")
	case cli.PermNetwork:
		return i18n.T("Reach the internet"), i18n.T("Can connect to any address on the network: web sites, your home network and this computer's own services.")
	}
	return addons.Sanitize(p, 40), ""
}

// addonProblemText says why Sona won't run an add-on's folder.
func addonProblemText(code string) string {
	switch code {
	case addons.ProblemUnreadable:
		return i18n.T("Sona can't read the folder or its addon.json.")
	case addons.ProblemTooLarge:
		return i18n.T("Its addon.json is larger than 64 KiB.")
	case addons.ProblemManifest:
		return i18n.T("Its addon.json isn't valid: it isn't JSON, has a field Sona doesn't know or one of the wrong type.")
	case addons.ProblemID, addons.ProblemIDMismatch:
		return i18n.T("Its ID isn't the folder's name, or isn't a valid ID.")
	case addons.ProblemReservedID:
		return i18n.T("Its ID is kept for Sona's own add-ons.")
	case addons.ProblemName:
		return i18n.T("Its name isn't plain text of 1 to 60 characters.")
	case addons.ProblemVersion:
		return i18n.T("Its version isn't a semantic version such as 1.2.0.")
	case addons.ProblemCommand:
		return i18n.T("Its command isn't a plain path inside the folder.")
	case addons.ProblemPermission:
		return i18n.T("It asks for a permission Sona doesn't have.")
	case addons.ProblemContribution:
		return i18n.T("What it adds to Sona's menus, palette or settings isn't valid.")
	case addons.ProblemStrings:
		return i18n.T("Its translated labels don't match what it adds.")
	case addons.ProblemNoExecutable:
		return i18n.T("Its command isn't a program Sona can run: it is missing, not a regular file or not executable.")
	case addons.ProblemEscapes:
		return i18n.T("Its command leads out of its folder.")
	case addons.ProblemUnsafe:
		return i18n.T("The folder or the program isn't yours alone: it belongs to someone else or others may change it.")
	case addons.ProblemTooMany:
		return i18n.T("Sona reads at most 100 add-on folders.")
	}
	return i18n.T("Sona won't run it.")
}

// addonStatus is one sentence on where an add-on is.
func addonStatus(a services.AddonInfo) string {
	switch a.State {
	case services.AddonRunning:
		return i18n.T("Running.")
	case services.AddonStarting:
		return i18n.T("Starting.")
	case services.AddonRestarting:
		return i18n.T("It stopped on its own. Sona starts it again in a moment.")
	case services.AddonCrashed:
		return i18n.T("Switched off: it stopped on its own too many times in a row. Turn it on to try again.")
	case services.AddonNeedsApproval:
		return i18n.T("Waiting for you: it asks for a permission you haven't approved. It doesn't run until you do.")
	case services.AddonInvalid:
		return addonProblemText(a.Problem)
	}
	return i18n.T("Off. It isn't running.")
}

// addons is Settings → Add-ons.
func (s *settings) addonsSection(c *ui.Context) {
	_, caps, ok := s.ready()
	if !ok {
		s.loading(c, 3)
		return
	}
	if !caps.Addons {
		s.stack(c, func() {
			s.group(c, "addons-about", i18n.T("Add-ons"), "", func() {
				s.unavailableRow(c, "addons-folder", i18n.T("Add-ons folder"), "")
			})
		})
		return
	}
	u := stateOf[addonsUI](s, "addons")
	u.init()
	list := s.addonList()
	rows := s.addonRows()
	s.stack(c, func() {
		s.group(c, "addons-about", i18n.T("Add-ons"), i18n.T("Programs that run beside Sona and add small things to it."), func() {
			s.addonsTrustNote(c)
			s.addonsFolderRow(c, u)
		})
		if !list.HasData {
			s.loading(c, 2)
			return
		}
		if list.Err != nil {
			kit.AlertLine(c.Key("addons-error"), i18n.T("Couldn't read the add-ons: {statusError}", "statusError", i18n.ErrorMessage(list.Err)))
		}
		for _, a := range list.Data {
			s.addonGroup(c, u, a, rowsOf(rows.Data, a.ID))
		}
		if len(list.Data) == 0 {
			s.group(c, "addons-none", i18n.T("Installed"), "", func() {
				theme.Text(c, i18n.T("No add-ons are installed. Open the folder, put an add-on's folder in it and it appears here."), "secondary").
					TextColor(theme.C("ink-muted")).Padding(rowPadY, 0).MaxWidth(rowTextWidth)
			})
		}
	})
}

// rowsOf is the settings rows of one add-on.
func rowsOf(list []services.AddonContribution, id string) []services.AddonRow {
	for _, c := range list {
		if c.ID == id {
			return c.Rows
		}
	}
	return nil
}

// addonsTrustNote says, before anything else, what an add-on is: a program
// run with your rights. Permissions are advisory until the OS sandbox (#304).
func (s *settings) addonsTrustNote(c *ui.Context) {
	c = c.Key("trust")
	msg := i18n.T("An add-on is a program that runs with your rights. Permissions limit what Sona gives it, not what the program can do: it can read your files and use the network. Turn on only add-ons you trust.")
	ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Start).Padding(12, 16).Margin(rowPadY, 0, 0, 0).Radius(theme.Current().Radius["lg"]).
		Background(theme.C("raised")).Border(1, theme.C("control-line")).Role(ui.RoleGroup).Label(msg).Children(func() {
		ui.Icon(c.Key("i"), theme.Icon("shield-alert")).FontSize(16).TextColor(theme.C("danger")).Margin(2, 0, 0, 0).Shrink(0)
		theme.Text(c, msg, "secondary").TextColor(theme.C("ink")).Shrink(1).MinWidth(0)
	})
}

// addonsFolderRow is Open add-ons folder: how an add-on is installed.
func (s *settings) addonsFolderRow(c *ui.Context, u *addonsUI) {
	s.row(c, rowOpts{
		ID: "addons-folder", Title: i18n.T("Add-ons folder"),
		Desc: i18n.T("Put an add-on's folder here to install it. Sona doesn't run it until you turn it on."),
	}, func() ui.Element {
		b := kit.Button(c.Key("open"), i18n.T("Open add-ons folder"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Icon: "folder-open", Disabled: u.folderBusy})
		if kit.Button(c.Key("again"), i18n.T("Look again"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, A11yLabel: i18n.T("Look in the add-ons folder again")}).Clicked() {
			s.env.Queries.Invalidate(keyAddons, keyAddonRows)
		}
		if b.Clicked() {
			u.folderBusy = true
			calls := s.addons
			go func() {
				err := calls.OpenFolder()
				s.env.Post(func() {
					u.folderBusy = false
					if err != nil {
						kit.ToastFailure(i18n.T("Couldn't open the add-ons folder"), err)
					}
				})
			}()
		}
		return b
	})
}

// addonGroup is one add-on under its own heading.
func (s *settings) addonGroup(c *ui.Context, u *addonsUI, a services.AddonInfo, rows []services.AddonRow) {
	name := clean(a.Name, 60)
	if name == "" {
		name = clean(a.ID, 60)
	}
	desc := clean(a.Description, 300)
	if a.Version != "" {
		desc = i18n.Meta(i18n.T("Version {version}", "version", clean(a.Version, 40)), desc)
	}
	s.group(c, "addon:"+a.ID, name, desc, func() {
		if a.State == services.AddonInvalid {
			s.addonProblem(c, a)
			return
		}
		s.addonSwitch(c, u, a, name)
		if u.approving == a.ID {
			s.addonApproval(c, u, a, name)
		}
		if msg := u.errs[a.ID]; msg != "" {
			ui.Column(c.Key("err:"+a.ID)).FillWidth().Padding(0, 0, rowPadY, 0).Children(func() { kit.AlertLine(c, msg) })
		}
		s.addonPermissions(c, u, a, name)
		for _, r := range rows {
			s.addonRow(c, u, a, name, r)
		}
		s.addonLogRow(c, u, a, name)
	})
}

// clean is plain text of at most n bytes; the service cleans it already.
func clean(text string, n int) string { return strings.TrimSpace(addons.Sanitize(text, n)) }

// addonProblem is what a refused add-on shows: the reason and nothing to
// turn on.
func (s *settings) addonProblem(c *ui.Context, a services.AddonInfo) {
	id := "addon:" + a.ID + ":problem"
	why := addonProblemText(a.Problem)
	s.seen[id] = true
	ui.Column(c.Key(id)).FillWidth().Children(func() {
		s.line(c)
		ui.Row(c.Key("in")).FillWidth().Gap(8).AlignItems(ui.Start).Padding(rowPadY, 0).Role(ui.RoleGroup).Label(why).Children(func() {
			ui.Icon(c.Key("i"), theme.Icon("circle-alert")).FontSize(16).TextColor(theme.C("danger")).Margin(2, 0, 0, 0).Shrink(0)
			ui.Column(c.Key("t")).Shrink(1).MinWidth(0).Children(func() {
				theme.Text(c, i18n.T("Sona won't run this add-on"), "label").TextColor(theme.C("ink"))
				theme.Text(c, why, "secondary").TextColor(theme.C("ink-muted")).MaxWidth(rowTextWidth).Margin(2, 0, 0, 0)
			})
		})
	})
}

// addonSwitch is the on/off switch, with where the add-on is in words.
// Turning it on asks first, in a panel under the row, naming each permission;
// turning it off stops the add-on at once.
func (s *settings) addonSwitch(c *ui.Context, u *addonsUI, a services.AddonInfo, name string) {
	status := addonStatus(a)
	if a.Restarts > 0 && a.State != services.AddonCrashed {
		status += " " + i18n.T("{n, plural, one {It has stopped on its own once.} other {It has stopped on its own # times.}}", "n", a.Restarts)
	}
	id := "addon:" + a.ID + ":on"
	s.tell(c, id, status)
	busy := u.busy[a.ID]
	s.row(c, rowOpts{ID: id, Title: i18n.T("Turned on"), Desc: status}, func() ui.Element {
		on := a.Enabled && a.State != services.AddonNeedsApproval || u.approving == a.ID
		sw := kit.Switch(c, &on, kit.SwitchOpts{A11yLabel: i18n.T("Turn on {name}", "name", name), Disabled: busy})
		if sw.Changed() {
			switch {
			case on:
				// Never on without the person seeing what it asks for.
				u.approving, u.focus = a.ID, 0
				u.errs[a.ID] = ""
			case u.approving == a.ID:
				u.approving = ""
			default:
				s.addonCall(u, a.ID, func() error { return s.addons.Disable(a.ID) })
			}
		}
		return sw
	})
}

// addonApproval is the panel under the switch: what the add-on asks for and
// what that is worth, then Turn on or Cancel (Escape cancels). The permissions
// approved are the ones shown.
func (s *settings) addonApproval(c *ui.Context, u *addonsUI, a services.AddonInfo, name string) {
	c = c.Key("approve:" + a.ID)
	title := i18n.T("Turn on {name}?", "name", name)
	panel := ui.Column(c).FillWidth().Gap(10).Padding(14, 16).Margin(0, 0, rowPadY, 0).Radius(theme.Current().Radius["lg"]).
		Background(theme.C("raised")).Border(1, theme.C("control-line")).Role(ui.RoleGroup).Label(title)
	kit.AnnounceMounted(c, panel, title)
	panel.Children(func() {
		theme.Text(c, title, "label").TextColor(theme.C("ink"))
		if len(a.Permissions) == 0 {
			theme.Text(c, i18n.T("It asks for no permissions."), "secondary").TextColor(theme.C("ink-muted"))
		} else {
			theme.Text(c, i18n.T("It asks for these permissions:"), "secondary").TextColor(theme.C("ink-muted"))
			s.permissionList(c, name, a.Permissions, nil)
		}
		theme.Text(c, i18n.T("It runs with your rights: permissions limit what Sona gives it, not what it can do."), "secondary").
			TextColor(theme.C("ink-muted")).MaxWidth(rowTextWidth)
		ui.Row(c.Key("buttons")).Gap(8).Children(func() {
			ok := kit.Button(c.Key("ok"), i18n.T("Turn on"), kit.ButtonOpts{Size: kit.SizeSM, Disabled: u.busy[a.ID], A11yLabel: i18n.T("Turn on {name}", "name", name)})
			if ok.Clicked() {
				approve := slices.Clone(a.Permissions)
				u.approving = ""
				s.addonCall(u, a.ID, func() error { return s.addons.Enable(a.ID, approve) })
			}
			cancel := kit.Button(c.Key("cancel"), i18n.T("Cancel"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM})
			kit.FocusWhenShown(c, cancel, &u.focus)
			if cancel.Clicked() {
				u.approving = ""
			}
		})
	})
	if panel.Shortcut(0, ui.KeyEscape) {
		u.approving = ""
	}
}

// permissionList is the permissions as a list: each one's name and what it
// allows, and, with granted given, whether it is approved and a Revoke button.
func (s *settings) permissionList(c *ui.Context, addon string, asked []string, granted func(string) (bool, func())) {
	ui.Column(c.Key("perms")).FillWidth().Gap(8).Role(ui.RoleList).Label(i18n.T("Permissions")).Children(func() {
		for _, p := range asked {
			name, allows := permissionText(p)
			k := c.Key("perm:" + p)
			ui.Row(k).FillWidth().Gap(12).AlignItems(ui.Start).Role(ui.RoleListItem).Children(func() {
				ui.Column(k.Key("t")).Grow(1).Shrink(1).MinWidth(0).MaxWidth(rowTextWidth).Children(func() {
					theme.Text(k, name, "label").TextColor(theme.C("ink"))
					if allows != "" {
						theme.Text(k, allows, "secondary").TextColor(theme.C("ink-muted"))
					}
				})
				if granted != nil {
					ok, revoke := granted(p)
					if !ok {
						theme.Text(k.Key("no"), i18n.T("Not approved"), "meta").TextColor(theme.C("ink-muted")).Shrink(0)
					} else if revoke != nil {
						if kit.Button(k.Key("revoke"), i18n.T("Revoke"), kit.ButtonOpts{
							Variant: kit.Ghost, Size: kit.SizeSM, A11yLabel: i18n.T("Revoke {permission}", "permission", name+" · "+addon),
						}).Clicked() {
							revoke()
						}
					}
				}
			})
		}
	})
}

// addonPermissions is the permissions row: each one asked for, whether it is
// approved, and Revoke on the approved ones (it takes effect on the add-on's
// very next call).
func (s *settings) addonPermissions(c *ui.Context, u *addonsUI, a services.AddonInfo, name string) {
	id := "addon:" + a.ID + ":permissions"
	desc := i18n.T("What Sona lets it do. It runs with your rights either way.")
	s.row(c, rowOpts{ID: id, Title: i18n.T("Permissions"), Desc: desc, Stacked: true}, func() ui.Element {
		if len(a.Permissions) == 0 {
			theme.Text(c, i18n.T("It asks for no permissions."), "secondary").TextColor(theme.C("ink-muted"))
			return ui.Element{}
		}
		s.permissionList(c, name, a.Permissions, func(p string) (bool, func()) {
			if !slices.Contains(a.Granted, p) {
				return false, nil
			}
			return true, func() { s.addonCall(u, a.ID, func() error { return s.addons.Revoke(a.ID, p) }) }
		})
		return ui.Element{}
	})
}

// addonCall runs one call off the main thread, then reads the add-ons and
// their rows again. A failure is a sentence under the add-on's switch.
func (s *settings) addonCall(u *addonsUI, id string, call func() error) {
	if u.busy[id] {
		return
	}
	u.busy[id], u.errs[id] = true, ""
	calls := s.addons
	go func() {
		err := call()
		list := calls.List()
		s.env.Post(func() {
			u.busy[id] = false
			if err != nil {
				u.errs[id] = addonErrorText(err)
			}
			store.Set(s.env.Queries, keyAddons, list)
			s.env.Queries.Invalidate(keyAddonRows)
		})
	}()
}

// addonErrorText is what a failed call says.
func addonErrorText(err error) string {
	switch services.CodeOf(err) {
	case services.CodeNotFound:
		return i18n.T("That add-on isn't in the folder any more.")
	case services.CodeInvalidArgument:
		return i18n.T("Sona couldn't do that: the add-on changed since you looked. Check what it asks for and try again.")
	}
	return i18n.T("Sona couldn't do that: {why}", "why", i18n.ErrorMessage(err))
}

// addonRow is one settings row of an add-on, under its heading: a switch, a
// field, a select, or a secret. Values save at once (a field with Save or
// Enter); a secret is kept in the keyring and never shown again.
func (s *settings) addonRow(c *ui.Context, u *addonsUI, a services.AddonInfo, name string, r services.AddonRow) {
	label := clean(r.Label, 60)
	desc := clean(r.Description, 200)
	if label == "" {
		return
	}
	id := "addon:" + a.ID + ":row:" + r.ID
	key := a.ID + "/" + r.ID
	busy := u.busy[a.ID]
	save := func(value string) {
		s.addonCall(u, a.ID, func() error { return s.addons.SetValue(a.ID, r.ID, value) })
	}
	switch r.Kind {
	case "toggle":
		d := u.draft(key, r.Value)
		s.row(c, rowOpts{ID: id, Title: label, Desc: desc, Labels: true}, func() ui.Element {
			on := *d == "true"
			sw := kit.Switch(c, &on, kit.SwitchOpts{Disabled: busy})
			if sw.Changed() {
				*d = map[bool]string{true: "true", false: "false"}[on]
				save(*d)
			}
			return sw
		})
	case "choice":
		d := u.draft(key, r.Value)
		items := make([]kit.SelectItem, 0, len(r.Options))
		for _, o := range r.Options {
			items = append(items, kit.SelectItem{Value: o.Value, Label: clean(o.Label, 60)})
		}
		s.row(c, rowOpts{ID: id, Title: label, Desc: desc, Labels: true}, func() ui.Element {
			v := *d
			sel := kit.Select(c, &v, items, kit.SelectOpts{Small: true, Disabled: busy, Width: selectWidthFor(items, v)})
			if sel.Changed() && v != *d {
				*d = v
				save(v)
			}
			return sel
		})
	case "text":
		d := u.draft(key, r.Value)
		s.row(c, rowOpts{ID: id, Title: label, Desc: desc, Stacked: true}, func() ui.Element {
			var first ui.Element
			ui.Row(c.Key("field")).FillWidth().Gap(8).Children(func() {
				in := kit.Input(c, d, kit.InputOpts{A11yLabel: label, Disabled: busy})
				in.Grow(1).Shrink(1).MinWidth(0).Height(32)
				first = in
				changed := *d != r.Value
				b := kit.Button(c.Key("save"), i18n.T("Save"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Disabled: busy || !changed, A11yLabel: i18n.T("Save {label} · {name}", "label", label, "name", name)})
				if (b.Clicked() || in.Submitted()) && changed && !busy {
					save(*d)
				}
			})
			return first
		})
	case "secret":
		d := u.draft(key, "")
		// A secret is never sent to the window: only that one is stored.
		note := i18n.T("Nothing is saved.")
		if r.Saved {
			note = i18n.T("Saved in your system keyring. It isn't shown again.")
		}
		s.row(c, rowOpts{ID: id, Title: label, Desc: strings.TrimSpace(desc + " " + note), Stacked: true}, func() ui.Element {
			var first ui.Element
			ui.Row(c.Key("field")).FillWidth().Gap(8).Children(func() {
				ph := ""
				if r.Saved {
					ph = i18n.T("Saved")
				}
				in := kit.Input(c, d, kit.InputOpts{A11yLabel: label, Password: true, Placeholder: ph, Disabled: busy})
				in.Grow(1).Shrink(1).MinWidth(0).Height(32)
				first = in
				ready := strings.TrimSpace(*d) != ""
				b := kit.Button(c.Key("save"), i18n.T("Save"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Disabled: busy || !ready, A11yLabel: i18n.T("Save {label} · {name}", "label", label, "name", name)})
				if (b.Clicked() || in.Submitted()) && ready && !busy {
					value := *d
					*d = ""
					s.addonCall(u, a.ID, func() error { return s.addons.SetSecret(a.ID, r.ID, value) })
				}
				if r.Saved {
					if kit.Button(c.Key("remove"), i18n.T("Remove"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Disabled: busy, A11yLabel: i18n.T("Remove {label} · {name}", "label", label, "name", name)}).Clicked() {
						s.addonCall(u, a.ID, func() error { return s.addons.SetSecret(a.ID, r.ID, "") })
					}
				}
			})
			return first
		})
	}
}

// addonLogRow is the add-on's log: its last lines, read when the person asks
// (Show log) and again with Refresh. The lines are the add-on's own words,
// plain text, selectable as a block.
func (s *settings) addonLogRow(c *ui.Context, u *addonsUI, a services.AddonInfo, name string) {
	id := "addon:" + a.ID + ":log"
	open := u.logOpen[a.ID]
	l := u.logs[a.ID]
	s.row(c, rowOpts{ID: id, Title: i18n.T("Log"), Desc: i18n.T("What it wrote to its standard error, the last 200 lines."), Stacked: open}, func() ui.Element {
		var first ui.Element
		ui.Row(c.Key("buttons")).Gap(8).Children(func() {
			label := i18n.T("Show log")
			if open {
				label = i18n.T("Hide log")
			}
			b := kit.Button(c.Key("toggle"), label, kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, A11yLabel: label + " · " + name})
			first = b
			if b.Clicked() {
				u.logOpen[a.ID] = !open
				if !open {
					s.readLog(u, a.ID)
				}
			}
			if open {
				if kit.Button(c.Key("refresh"), i18n.T("Refresh"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Disabled: l != nil && l.loading, A11yLabel: i18n.T("Refresh the log of {name}", "name", name)}).Clicked() {
					s.readLog(u, a.ID)
				}
			}
		})
		if open {
			s.logLines(c, l)
		}
		return first
	})
}

// readLog reads the log off the main thread.
func (s *settings) readLog(u *addonsUI, id string) {
	l := u.logs[id]
	if l == nil {
		l = &addonLog{}
		u.logs[id] = l
	}
	if l.loading {
		return
	}
	l.loading = true
	calls := s.addons
	go func() {
		lines, err := calls.Log(id)
		s.env.Post(func() {
			l.loading, l.err = false, err != nil
			l.lines = lines
		})
	}()
}

// logLines draws the lines in a scroll box, newest last.
func (s *settings) logLines(c *ui.Context, l *addonLog) {
	c = c.Key("lines")
	msg := ""
	switch {
	case l == nil || (l.loading && len(l.lines) == 0):
		msg = i18n.T("Reading the log…")
	case l.err:
		msg = i18n.T("Couldn't read the log.")
	case len(l.lines) == 0:
		msg = i18n.T("Nothing logged yet.")
	}
	box := ui.Column(c).FillWidth().Padding(10, 12).Radius(theme.Current().Radius["md"]).Background(theme.C("raised")).
		Border(1, theme.C("control-line")).Role(ui.RoleLog).Label(i18n.T("Log"))
	box.Children(func() {
		if msg != "" {
			theme.Text(c, msg, "secondary").TextColor(theme.C("ink-muted"))
			return
		}
		ui.Scroll(c.Key("scroll")).FillWidth().MaxHeight(240).Children(func() {
			ui.Column(c.Key("text")).FillWidth().Selectable().Children(func() {
				for i, line := range l.lines {
					theme.Text(c.Key(i), line, "secondary").TextColor(theme.C("ink"))
				}
			})
		})
	})
}

// addonsSummary is the section's line in the list: how many add-ons run.
func (s *settings) addonsSummary() string {
	list := s.addonList()
	if !list.HasData || len(list.Data) == 0 {
		return i18n.T("None installed")
	}
	on := 0
	for _, a := range list.Data {
		if a.Enabled {
			on++
		}
	}
	return i18n.Meta(
		i18n.T("{n, plural, one {# add-on} other {# add-ons}}", "n", len(list.Data)),
		i18n.T("{n, plural, one {# on} other {# on}}", "n", on))
}
