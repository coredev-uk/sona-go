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

// Settings → Add-ons (docs/addons.md, docs/design.md "Settings"): a roster.
// Sona's own add-ons come first, as status only (their accounts and switches
// are in Connections); then the installed ones, one ledger row each with its
// state in words and its switch. A row opens its permissions, settings and
// log under it, one add-on at a time. Sona draws every word: the manifest's
// labels are plain text, cleaned by the service and again here.

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
	// open is the add-on whose details are open under its row ("" none);
	// approving the one whose approval panel is open.
	open      string
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
		return i18n.T("Playback state"), i18n.T("Sees which song plays, its progress, whether it is paused, and whether it is a favourite or in your library.")
	case cli.PermPlaybackControl:
		return i18n.T("Playback controls"), i18n.T("Can play, pause, skip, go back and seek, play an Apple Music link, and change shuffle, repeat and crossfade.")
	case cli.PermQueueRead:
		return i18n.T("The queue"), i18n.T("Can read the songs that are queued.")
	case cli.PermQueueControl:
		return i18n.T("Queue controls"), i18n.T("Can jump to a song in the queue and clear the queue.")
	case cli.PermLyricsRead:
		return i18n.T("Lyrics"), i18n.T("Can read the lyrics of the song that plays.")
	case cli.PermSearch:
		return i18n.T("Search"), i18n.T("Can search Apple Music's catalogue.")
	case cli.PermOpen:
		return i18n.T("Open Apple Music links"), i18n.T("Can ask Sona to open an Apple Music page.")
	case cli.PermVolume:
		return i18n.T("Volume"), i18n.T("Can read and change the volume.")
	case cli.PermLike:
		return i18n.T("Likes"), i18n.T("Can love or unlove the song that plays, or rate it Suggest Less.")
	case cli.PermLibrary:
		return i18n.T("Your library"), i18n.T("Can add the song that plays to your library or remove it, and list your library's playlists and albums.")
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
		return i18n.T("Its command isn't a plain path inside the folder (a .wasm file, for a WebAssembly add-on).")
	case addons.ProblemRuntime:
		return i18n.T("Its runtime isn't \"native\" or \"wasm\".")
	case addons.ProblemPermission:
		return i18n.T("It asks for a permission Sona doesn't have.")
	case addons.ProblemContribution:
		return i18n.T("What it adds to Sona's menus, palette or settings isn't valid.")
	case addons.ProblemStrings:
		return i18n.T("Its translated labels don't match what it adds.")
	case addons.ProblemNoExecutable:
		return i18n.T("Its command isn't a program Sona can run: it is missing, not a regular file or not executable (for a WebAssembly add-on, not a WebAssembly module).")
	case addons.ProblemEscapes:
		return i18n.T("Its command leads out of its folder.")
	case addons.ProblemUnsafe:
		return i18n.T("The folder or the program isn't yours alone: it belongs to someone else or others may change it.")
	case addons.ProblemTooMany:
		return i18n.T("Sona reads at most 100 add-on folders.")
	}
	return i18n.T("Sona won't run it.")
}

// sandboxText says why the OS sandbox can't be set up, so that the add-on does
// not run. Sona never runs one without it.
func sandboxText(code string) string {
	switch code {
	case addons.SandboxUserNamespaceRestricted:
		return i18n.T("It doesn't run: this computer's security settings (AppArmor) stop Sona from creating the isolated space its sandbox needs. Sona never runs an add-on without the sandbox.")
	case addons.SandboxNoUserNamespace:
		return i18n.T("It doesn't run: this system doesn't let Sona create the isolated space (a user namespace) its sandbox needs. A Flatpak, a container or a kernel setting can cause that. Sona never runs an add-on without the sandbox.")
	case addons.SandboxNoSeccomp:
		return i18n.T("It doesn't run: the sandbox's system-call filter can't be set up on this computer. Sona never runs an add-on without the sandbox.")
	case addons.SandboxNoSeatbelt:
		return i18n.T("It doesn't run: macOS's sandbox tool is missing or refused Sona's rules. Sona never runs an add-on without the sandbox.")
	case addons.SandboxNoAppContainer:
		return i18n.T("It doesn't run: Windows couldn't create the AppContainer that sandboxes it. Sona never runs an add-on without the sandbox.")
	case addons.SandboxUnsupported:
		return i18n.T("It doesn't run: this system has no sandbox for add-ons, and Sona never runs an add-on without it.")
	}
	return i18n.T("It doesn't run: Sona couldn't set up its sandbox, and never runs an add-on without it. The add-on's log has the details.")
}

// unisolatedText is the muted line under one of Sona's own add-ons that runs
// without the sandbox, with the reason in plain words.
func unisolatedText(code string) string {
	switch code {
	case addons.SandboxUserNamespaceRestricted:
		return i18n.T("Running without isolation: this computer's security settings (AppArmor) stop Sona from creating the isolated space its sandbox needs. This add-on is part of Sona and has the same access Sona has.")
	case addons.SandboxNoUserNamespace:
		return i18n.T("Running without isolation: this system doesn't let Sona create the isolated space (a user namespace) its sandbox needs. A Flatpak, a container or a kernel setting can cause that. This add-on is part of Sona and has the same access Sona has.")
	case addons.SandboxNoSeccomp:
		return i18n.T("Running without isolation: the sandbox's system-call filter can't be set up on this computer. This add-on is part of Sona and has the same access Sona has.")
	case addons.SandboxNoSeatbelt:
		return i18n.T("Running without isolation: macOS's sandbox tool is missing or refused Sona's rules. This add-on is part of Sona and has the same access Sona has.")
	case addons.SandboxNoAppContainer:
		return i18n.T("Running without isolation: Windows couldn't create the AppContainer that sandboxes it. This add-on is part of Sona and has the same access Sona has.")
	case addons.SandboxUnsupported:
		return i18n.T("Running without isolation: this system has no sandbox for add-ons. This add-on is part of Sona and has the same access Sona has.")
	}
	return i18n.T("Running without isolation: Sona couldn't set up its sandbox. This add-on is part of Sona and has the same access Sona has. The add-on's log has the details.")
}

// tone is the colour of a row's state dot; the state is always also said in
// words beside it.
type tone int

const (
	toneOff tone = iota
	toneOK
	toneAttention
)

// addonState is where an add-on is: a word or two for its row, the tone of
// its dot, and, when the word alone doesn't say what to do, a sentence under
// it.
func addonState(a services.AddonInfo) (word string, t tone, reason string) {
	switch a.State {
	case services.AddonRunning:
		if a.Unsandboxed != "" {
			return i18n.T("Running"), toneOK, unisolatedText(a.Unsandboxed)
		}
		return i18n.T("Running"), toneOK, ""
	case services.AddonStarting:
		return i18n.T("Starting"), toneOK, ""
	case services.AddonIdle:
		return i18n.T("On"), toneOK, i18n.T("It starts when Sona has something for it to do.")
	case services.AddonRestarting:
		return i18n.T("Restarting"), toneAttention, i18n.T("It stopped on its own. Sona starts it again in a moment.")
	case services.AddonCrashed:
		return i18n.T("Switched off"), toneAttention, i18n.T("It stopped on its own too many times in a row. Turn it on to try again.")
	case services.AddonNeedsApproval:
		return i18n.T("Needs your approval"), toneAttention, i18n.T("It asks for a permission you haven't approved. It doesn't run until you do.")
	case services.AddonInvalid:
		return i18n.T("Won't run"), toneAttention, addonProblemText(a.Problem)
	case services.AddonNoSandbox:
		return i18n.T("Can't run"), toneAttention, sandboxText(a.Sandbox)
	}
	if a.Bundled {
		return i18n.T("Not in use"), toneOff, ""
	}
	return i18n.T("Off"), toneOff, ""
}

// needsYou reports an add-on whose row asks for the person: one that can't
// run, was switched off, or waits for an approval.
func needsYou(a services.AddonInfo) bool {
	_, t, _ := addonState(a)
	return t == toneAttention
}

// addonsSection is Settings → Add-ons.
func (s *settings) addonsSection(c *ui.Context) {
	_, caps, ok := s.ready()
	if !ok {
		s.loading(c, 3)
		return
	}
	if !caps.Addons {
		s.stack(c, func() {
			s.group(c, "addons-installed", i18n.T("Installed"), "", func() {
				s.unavailableRow(c, "addons-folder", i18n.T("Add-ons folder"), "")
			})
		})
		return
	}
	u := stateOf[addonsUI](s, "addons")
	u.init()
	list := s.addonList()
	rows := s.addonRows()
	var own, installed []services.AddonInfo
	for _, a := range list.Data {
		if a.Bundled {
			own = append(own, a)
		} else {
			installed = append(installed, a)
		}
	}
	s.stack(c, func() {
		if len(own) > 0 {
			s.group(c, "addons-own", i18n.T("Sona's Own"), i18n.T("Part of Sona. Their accounts are in Connections."), func() {
				for _, a := range own {
					s.addonEntry(c, u, a, nil)
				}
			})
		}
		s.group(c, "addons-installed", i18n.T("Installed"),
			i18n.T("Programs you add. Each runs in a sandbox with only what you approve, so turn on only add-ons you trust."), func() {
				switch {
				case !list.HasData:
					s.addonsLoading(c)
				case list.Err != nil:
					ui.Column(c.Key("addons-error")).FillWidth().Padding(rowPadY, 0).Children(func() {
						kit.AlertLine(c, i18n.T("Couldn't read the add-ons: {statusError}", "statusError", i18n.ErrorMessage(list.Err)))
					})
				case len(installed) == 0:
					ui.Column(c.Key("addons-none")).FillWidth().Padding(rowPadY, 0).Children(func() {
						kit.EmptyState(c, i18n.T("No add-ons installed"),
							i18n.T("Put an add-on's folder in the add-ons folder, then look again."), kit.EmptyOpts{})
					})
				}
				for _, a := range installed {
					s.addonEntry(c, u, a, rowsOf(rows.Data, a.ID))
				}
				s.addonsFolderRow(c, u)
			})
	})
}

// addonsLoading is the roster's rows in their shape while the list loads.
func (s *settings) addonsLoading(c *ui.Context) {
	ui.Column(c.Key("addons-loading")).FillWidth().Role(ui.RoleStatus).Label(i18n.T("Loading")).Children(func() {
		for i := range 2 {
			k := c.Key(i)
			ui.Column(k).FillWidth().Children(func() {
				s.line(k)
				ui.Column(k.Key("r")).Gap(8).Padding(rowPadY, 0).Children(func() {
					kit.Bar(k.Key("t"), 160, 14)
					kit.Bar(k.Key("d"), 220, 12)
				})
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

// addonName is what the add-on is called on its row.
func addonName(a services.AddonInfo) string {
	if name := clean(a.Name, 60); name != "" {
		return name
	}
	return clean(a.ID, 60)
}

// addonEntry is one add-on's row in the roster: a disclosure with its name
// and its state in words, and at the right its switch (an installed one) or
// the way to Connections (one of Sona's own). Under it, while open, the
// approval panel, a failed call, and its details.
func (s *settings) addonEntry(c *ui.Context, u *addonsUI, a services.AddonInfo, rows []services.AddonRow) {
	c = c.Key("addon:" + a.ID)
	name := addonName(a)
	open := u.open == a.ID
	ui.Column(c).FillWidth().Children(func() {
		s.line(c)
		// The control sits level with the name line (6 DIP of padding, a
		// 20 DIP line), however many lines the state takes under it.
		ui.Row(c.Key("head")).FillWidth().AlignItems(ui.Start).Gap(rowGap).Padding(rowPadY-6, 0).Children(func() {
			s.addonDisclosure(c, u, a, name, open)
			switch {
			case a.Bundled:
				s.connectionsLink(c, name)
			case a.State != services.AddonInvalid:
				s.addonSwitch(c, u, a, name)
			}
		})
		if u.approving == a.ID {
			s.addonApproval(c, u, a, name)
		}
		if msg := u.errs[a.ID]; msg != "" {
			ui.Column(c.Key("err")).FillWidth().Padding(0, 0, rowPadY, 0).Children(func() { kit.AlertLine(c, msg) })
		}
		if open {
			s.addonDetails(c, u, a, name, rows)
		}
	})
}

// addonDisclosure is the row's text as one button: the chevron and the name,
// then the state dot, the state in words and how it runs, and the sentence
// under it when there is one. It opens and closes the add-on's details.
func (s *settings) addonDisclosure(c *ui.Context, u *addonsUI, a services.AddonInfo, name string, open bool) {
	word, t, reason := addonState(a)
	line := word
	if a.Restarts > 0 && a.State != services.AddonCrashed {
		line = i18n.Meta(line, i18n.T("{n, plural, one {stopped once} other {stopped # times}}", "n", a.Restarts))
	}
	if how := addonRunsAs(a); how != "" {
		line = i18n.Meta(line, how)
	}
	tk := theme.Current()
	b := ui.ButtonBase(c.Key("open")).Grow(1).Shrink(1).MinWidth(0).Margin(0, 0, 0, -rowWashInset).Padding(6, rowWashInset).
		Radius(tk.Radius["md"]).Role(ui.RoleDisclosure).Expanded(open).Label(name).Description(strings.TrimSpace(line + ". " + reason))
	if b.Hovered() {
		b.Background(theme.C("hover"))
	}
	theme.Fade(b, "feedback")
	b.Children(func() {
		ui.Column(c.Key("text")).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Start).Children(func() {
			ui.Row(c.Key("name")).AlignItems(ui.Center).Gap(6).Children(func() {
				ic := theme.Glyph(c, "chevron-right", 14, theme.GlyphOpts{Color: theme.C("ink-muted")})
				turn := float32(0)
				if open {
					turn = 90
				}
				if theme.ReducedMotion() {
					ic.Rotate(turn)
				} else {
					ic.Rotate(ic.Animate("turn", turn, tk.Motion["press"].Duration))
				}
				theme.Text(c, name, "label").TextColor(theme.C("ink")).Shrink(1).MinWidth(0)
			})
			ui.Row(c.Key("state")).AlignItems(ui.Center).Gap(8).Margin(2, 0, 0, 20).Children(func() {
				dot := theme.C("ink-faint")
				switch t {
				case toneOK:
					dot = theme.C("positive")
				case toneAttention:
					dot = theme.C("danger")
				}
				ui.Box(c.Key("dot")).Width(7).Height(7).Shrink(0).Radius(tk.Radius["full"]).Background(dot)
				theme.Text(c, line, "secondary").TextColor(theme.C("ink-muted")).Shrink(1).MinWidth(0)
			})
			if reason != "" {
				theme.Text(c.Key("why"), reason, "secondary").TextColor(theme.C("ink-muted")).MaxWidth(rowTextWidth).Margin(2, 0, 0, 35)
			}
		})
	})
	if b.Clicked() {
		if open {
			u.open = ""
		} else {
			u.open = a.ID
		}
	}
}

// addonRunsAs is how an add-on runs, in a word or two: inside Sona as
// WebAssembly, or in its sandbox, with the internet when that is approved.
// Sona's own say nothing: they are part of Sona.
func addonRunsAs(a services.AddonInfo) string {
	if a.Bundled || a.State == services.AddonInvalid {
		return ""
	}
	if a.Runtime == addons.RuntimeWasm {
		return i18n.T("WebAssembly")
	}
	if slices.Contains(a.Granted, cli.PermNetwork) {
		return i18n.Meta(i18n.T("Sandboxed"), i18n.T("Internet"))
	}
	return i18n.T("Sandboxed")
}

// connectionsLink is the way from one of Sona's own add-ons to where its
// account and switch are.
func (s *settings) connectionsLink(c *ui.Context, name string) {
	tk := theme.Current()
	e := ui.ButtonBase(c.Key("connections")).Role(ui.RoleLink).Label(i18n.T("Manage {name} in Connections", "name", name)).
		Height(28).PaddingX(8).Gap(2).Radius(tk.Radius["md"]).Center().Shrink(0).Margin(2, 0, 0, 0)
	ink := theme.C("ink-muted")
	if e.Hovered() {
		e.Background(theme.C("hover"))
		ink = theme.C("ink")
	}
	theme.Fade(e, "feedback")
	e.TextColor(ink).Children(func() {
		theme.Text(c, i18n.T("Connections"), "secondary").SingleLine()
		theme.Glyph(c, "chevron-right", 14, theme.GlyphOpts{Color: ink, Inherit: true})
	})
	if e.Clicked() {
		s.env.Nav.Replace(settingsPath + "?section=" + secConnections)
	}
}

// clean is plain text of at most n bytes; the service cleans it already.
func clean(text string, n int) string { return strings.TrimSpace(addons.Sanitize(text, n)) }

// addonSwitch is the on/off switch at the right of an installed add-on's row.
// Turning it on asks first, in a panel under the row, naming each permission;
// turning it off stops the add-on at once.
func (s *settings) addonSwitch(c *ui.Context, u *addonsUI, a services.AddonInfo, name string) {
	on := a.Enabled && a.State != services.AddonNeedsApproval || u.approving == a.ID
	sw := kit.Switch(c.Key("on"), &on, kit.SwitchOpts{A11yLabel: i18n.T("Turn on {name}", "name", name), Disabled: u.busy[a.ID]})
	sw.Shrink(0).Margin(7, 0, 0, 0)
	if !sw.Changed() {
		return
	}
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

// addonApproval is the panel under the row: what the add-on asks for and
// what that is worth, then Turn on or Cancel (Escape cancels). The permissions
// approved are the ones shown.
func (s *settings) addonApproval(c *ui.Context, u *addonsUI, a services.AddonInfo, name string) {
	c = c.Key("approve")
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
		note := i18n.T("It runs in a sandbox: it reads only its own folder, writes only its own data folder and reaches the internet only if you approve that.")
		if a.Runtime == addons.RuntimeWasm {
			note = i18n.T("It runs inside Sona as a WebAssembly module. It can use only its own data folder and can't reach the network: permissions limit what Sona gives it.")
		}
		theme.Text(c, note, "secondary").
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

// facetWidth is the column an add-on's details name their parts in.
const facetWidth = 104

// addonDetails is an add-on's opened row: its version and description, then
// its permissions, its settings rows and its log, each named in a column of
// its own, on the hover wash so the block reads as the row's own.
func (s *settings) addonDetails(c *ui.Context, u *addonsUI, a services.AddonInfo, name string, rows []services.AddonRow) {
	c = c.Key("details")
	about := clean(a.Description, 300)
	if a.Version != "" && !a.Bundled {
		about = i18n.Meta(i18n.T("Version {version}", "version", clean(a.Version, 40)), about)
	}
	ui.Column(c).FillWidth().Margin(0, 0, rowPadY, 0).Padding(6, 20).Radius(theme.Current().Radius["lg"]).
		Background(theme.C("hover")).Role(ui.RoleGroup).Label(name).Children(func() {
		first := true
		facet := func(key, title string, body func()) {
			k := c.Key("facet:" + key)
			if !first {
				ui.Box(c.Key("line:" + key)).FillWidth().Height(1).Shrink(0).Background(theme.C("line"))
			}
			first = false
			ui.Row(k).FillWidth().Gap(16).AlignItems(ui.Start).Padding(12, 0).Children(func() {
				theme.Text(k, title, "secondary").FontWeight(600).TextColor(theme.C("ink")).Width(facetWidth).Shrink(0).Margin(1, 0, 0, 0)
				ui.Column(k.Key("body")).Grow(1).Shrink(1).MinWidth(0).Gap(12).Children(body)
			})
		}
		if about != "" {
			facet("about", i18n.T("About"), func() {
				theme.Text(c, about, "secondary").TextColor(theme.C("ink-muted")).MaxWidth(rowTextWidth)
			})
		}
		if a.State == services.AddonInvalid {
			return // nothing runs: no permissions to show, no rows, no log
		}
		facet("permissions", i18n.T("Permissions"), func() { s.addonPermissions(c, u, a, name) })
		if len(rows) > 0 {
			facet("settings", i18n.T("Settings"), func() {
				for _, r := range rows {
					s.addonRow(c, u, a, name, r)
				}
			})
		}
		facet("log", i18n.T("Log"), func() { s.addonLog(c, u, a, name) })
	})
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

// addonPermissions is what an add-on asks for, whether each is approved, and
// Revoke on the approved ones (it takes effect on the add-on's very next
// call). Sona's own have nothing to revoke: they are part of Sona.
func (s *settings) addonPermissions(c *ui.Context, u *addonsUI, a services.AddonInfo, name string) {
	if len(a.Permissions) == 0 {
		theme.Text(c, i18n.T("It asks for no permissions."), "secondary").TextColor(theme.C("ink-muted"))
		return
	}
	s.permissionList(c, name, a.Permissions, func(p string) (bool, func()) {
		if !slices.Contains(a.Granted, p) {
			return false, nil
		}
		if a.Bundled {
			return true, nil
		}
		return true, func() { s.addonCall(u, a.ID, func() error { return s.addons.Revoke(a.ID, p) }) }
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

// panelRow is one line of an opened add-on's settings: its label and what it
// does, and its control at the right, or under it (wide), full width.
func panelRow(c *ui.Context, key, title, desc string, below bool, control func()) (label ui.Element) {
	c = c.Key("prow:" + key)
	text := func() {
		ui.Column(c.Key("t")).Grow(1).Shrink(1).MinWidth(0).MaxWidth(rowTextWidth).Children(func() {
			label = theme.Text(c, title, "label").TextColor(theme.C("ink"))
			if desc != "" {
				theme.Text(c, desc, "secondary").TextColor(theme.C("ink-muted"))
			}
		})
	}
	if below {
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			text()
			control()
		})
		return label
	}
	ui.Row(c).FillWidth().Gap(rowGap).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
		text()
		ui.Row(c.Key("c")).Shrink(0).AlignItems(ui.Center).Gap(8).Children(control)
	})
	return label
}

// addonRow is one settings row of an add-on, in its Settings: a switch, a
// field, a select, or a secret. Values save at once (a field with Save or
// Enter); a secret is kept in the keyring and never shown again.
func (s *settings) addonRow(c *ui.Context, u *addonsUI, a services.AddonInfo, name string, r services.AddonRow) {
	label := clean(r.Label, 60)
	desc := clean(r.Description, 200)
	if label == "" {
		return
	}
	c = c.Key("row:" + r.ID)
	key := a.ID + "/" + r.ID
	busy := u.busy[a.ID]
	save := func(value string) {
		s.addonCall(u, a.ID, func() error { return s.addons.SetValue(a.ID, r.ID, value) })
	}
	switch r.Kind {
	case "toggle":
		d := u.draft(key, r.Value)
		var sw ui.Element
		title := panelRow(c, r.ID, label, desc, false, func() {
			on := *d == "true"
			sw = kit.Switch(c.Key("sw:"+r.ID), &on, kit.SwitchOpts{A11yLabel: label, Disabled: busy})
			if sw.Changed() {
				*d = map[bool]string{true: "true", false: "false"}[on]
				save(*d)
			}
		})
		// The title is the switch's label: a click on it flips the switch,
		// as a Settings row's does.
		if title.Clicked() && !busy {
			*d = map[bool]string{true: "false", false: "true"}[*d == "true"]
			save(*d)
			sw.Focus()
		}
	case "choice":
		d := u.draft(key, r.Value)
		items := make([]kit.SelectItem, 0, len(r.Options))
		for _, o := range r.Options {
			items = append(items, kit.SelectItem{Value: o.Value, Label: clean(o.Label, 60)})
		}
		var sel ui.Element
		title := panelRow(c, r.ID, label, desc, false, func() {
			v := *d
			sel = kit.Select(c.Key("sel:"+r.ID), &v, items, kit.SelectOpts{Small: true, A11yLabel: label, Disabled: busy, Width: selectWidthFor(items, v)})
			if sel.Changed() && v != *d {
				*d = v
				save(v)
			}
		})
		if title.Clicked() {
			sel.Focus()
		}
	case "text":
		d := u.draft(key, r.Value)
		var in ui.Element
		title := panelRow(c, r.ID, label, desc, true, func() {
			ui.Row(c.Key("field:" + r.ID)).FillWidth().Gap(8).Children(func() {
				in = kit.Input(c, d, kit.InputOpts{A11yLabel: label, Disabled: busy})
				in.Grow(1).Shrink(1).MinWidth(0).Height(32)
				changed := *d != r.Value
				b := kit.Button(c.Key("save"), i18n.T("Save"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Disabled: busy || !changed, A11yLabel: i18n.T("Save {label} · {name}", "label", label, "name", name)})
				if (b.Clicked() || in.Submitted()) && changed && !busy {
					save(*d)
				}
			})
		})
		if title.Clicked() {
			in.Focus()
		}
	case "secret":
		d := u.draft(key, "")
		// A secret is never sent to the window: only that one is stored.
		note := i18n.T("Nothing is saved.")
		if r.Saved {
			note = i18n.T("Saved in your system keyring. It isn't shown again.")
		}
		var in ui.Element
		title := panelRow(c, r.ID, label, strings.TrimSpace(desc+" "+note), true, func() {
			ui.Row(c.Key("field:" + r.ID)).FillWidth().Gap(8).Children(func() {
				ph := ""
				if r.Saved {
					ph = i18n.T("Saved")
				}
				in = kit.Input(c, d, kit.InputOpts{A11yLabel: label, Password: true, Placeholder: ph, Disabled: busy})
				in.Grow(1).Shrink(1).MinWidth(0).Height(32)
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
		})
		if title.Clicked() {
			in.Focus()
		}
	}
}

// addonLog is an opened add-on's Log: its last lines, read when the person
// asks (Show log) and again with Refresh. The lines are the add-on's own
// words, plain text, selectable as a block.
func (s *settings) addonLog(c *ui.Context, u *addonsUI, a services.AddonInfo, name string) {
	open := u.logOpen[a.ID]
	l := u.logs[a.ID]
	panelRow(c, "log", i18n.T("What it wrote to its standard error"), i18n.T("The last 200 lines."), false, func() {
		label := i18n.T("Show log")
		if open {
			label = i18n.T("Hide log")
		}
		if open {
			if kit.Button(c.Key("refresh"), i18n.T("Refresh"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Disabled: l != nil && l.loading, A11yLabel: i18n.T("Refresh the log of {name}", "name", name)}).Clicked() {
				s.readLog(u, a.ID)
			}
		}
		if kit.Button(c.Key("toggle"), label, kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, A11yLabel: label + " · " + name}).Clicked() {
			u.logOpen[a.ID] = !open
			if !open {
				s.readLog(u, a.ID)
			}
		}
	})
	if open {
		s.logLines(c, l)
	}
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

// addonsSummary is the section's line in the list: how many add-ons are
// installed, and how many need the person (or how many are on).
func (s *settings) addonsSummary() string {
	list := s.addonList()
	if !list.HasData {
		return i18n.T("None installed")
	}
	installed, on, attention := 0, 0, 0
	for _, a := range list.Data {
		if needsYou(a) {
			attention++
		}
		if a.Bundled {
			continue
		}
		installed++
		if a.Enabled {
			on++
		}
	}
	first := i18n.T("None installed")
	if installed > 0 {
		first = i18n.T("{n, plural, one {# installed} other {# installed}}", "n", installed)
	}
	switch {
	case attention > 0:
		return i18n.Meta(first, i18n.T("{n, plural, one {# needs you} other {# need you}}", "n", attention))
	case installed > 0:
		return i18n.Meta(first, i18n.T("{n, plural, one {# on} other {# on}}", "n", on))
	}
	return first
}
