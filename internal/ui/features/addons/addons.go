// Package addons is what the running add-ons (docs/addons.md) add to Sona's
// own surfaces: their item actions in the menus of a song, album, artist or
// playlist, and their commands in the command palette. Sona draws all of it,
// as plain text under the add-on's name; choosing one invokes the add-on off
// the main thread and shows its answer as a toast.
//
// Settings → Add-ons, with each add-on's permissions, switch, log and rows,
// is the settings feature's (it never imports this one).
package addons

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/addons"
	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// How long an invocation may take and how often one add-on may show a toast
// (the limits docs/addons.md names).
const (
	invokeTimeout = 10 * time.Second
	toastEvery    = time.Second
	toastLong     = 8 * time.Second // a toast with a link to open
	maxHost       = 80
)

// api is what the feature asks of AddonsService.
type api interface {
	List() []services.AddonInfo
	Contributions(locale string) []services.AddonContribution
	Invoke(ctx context.Context, id, action string, target *services.AddonTarget) (services.AddonResult, error)
	AnswerDev(token string, run bool)
}

// feature is the module's state; everything but the calls it hands to
// goroutines is the main thread's.
type feature struct {
	env *module.Env
	api api
	now func() time.Time
	// locale is the language the contributions were last asked in.
	locale string

	// commands are the palette entries of the running add-ons.
	commands []module.Shortcut
	// loading: a Contributions call is in flight; again: ask once more when
	// it lands (the add-ons changed meanwhile).
	loading, again bool
	// busy are the invocations in flight, by add-on and action: choosing the
	// same one again meanwhile does nothing.
	busy map[string]bool
	// lastToast is when each add-on last showed a toast.
	lastToast map[string]time.Time
	// asking is the token of the `sona addons dev` question on screen, "" when
	// none is.
	asking string
	// infos is the add-ons and where each stands, for the notification
	// centre's rows (main thread).
	infos []services.AddonInfo
}

// New is the add-ons module.
func New(env *module.Env) module.Module {
	f := &feature{env: env, now: time.Now, busy: map[string]bool{}, lastToast: map[string]time.Time{}}
	if env.Core != nil {
		f.api = services.Service[services.AddonsService](env.Core)
	}
	env.AddonCommands = func() []module.Shortcut { return f.commands }
	return module.Module{
		ID:    "addons",
		Slots: module.Slots{Activity: &module.Activity{Status: f.status, Build: f.rows}},
		Init: func() {
			if f.api == nil {
				return
			}
			store.On(env.Bus, services.EventAddonsChanged, func(list []services.AddonInfo) {
				f.infos = list
				f.refresh()
			})
			store.On(env.Bus, services.EventAddonsDevAsk, f.devAsk)
			store.On(env.Bus, services.EventAddonsDevAskEnd, f.devAskEnd)
			store.On(env.Bus, services.EventSettingsChanged, func(config.Settings) {
				if i18n.Locale() != f.locale {
					f.refresh()
				}
			})
			f.infos = f.api.List()
			f.refresh()
		},
	}
}

// refresh reads the contributions off the main thread and swaps them in: one
// read at a time, and one more when something changed during it.
func (f *feature) refresh() {
	if f.api == nil {
		return
	}
	if f.loading {
		f.again = true
		return
	}
	f.loading = true
	locale := i18n.Locale()
	f.locale = locale
	go func() {
		list := f.api.Contributions(locale)
		f.env.Post(func() {
			f.loading = false
			f.apply(list)
			if f.again {
				f.again = false
				f.refresh()
			}
		})
	}()
}

// apply replaces the registered actions and the palette's commands with those
// of the running add-ons in list. It must run on the main thread.
func (f *feature) apply(list []services.AddonContribution) {
	var defs []kit.ActionDef
	var cmds []module.Shortcut
	for _, c := range list {
		if !c.Running {
			continue
		}
		name := clean(c.Name, 60)
		if name == "" {
			continue
		}
		// Whatever the service says, nothing outside addon.<id>.* is registered:
		// an add-on never takes a Sona action's or shortcut's ID.
		own := "addon." + c.ID + "."
		for _, a := range c.Actions {
			if strings.HasPrefix(a.ID, own) {
				defs = append(defs, f.actionDefs(c.ID, name, a)...)
			}
		}
		for _, cm := range c.Commands {
			if strings.HasPrefix(cm.ID, own) {
				cmds = append(cmds, f.command(c.ID, name, cm))
			}
		}
	}
	if f.env.Actions != nil {
		f.env.Actions.ReplacePrefix("addon.", defs...)
	}
	f.commands = cmds
}

// kinds maps an action's kind to the menu's.
var kinds = map[string]kit.ItemKind{
	"song": kit.KindSong, "album": kit.KindAlbum, "artist": kit.KindArtist, "playlist": kit.KindPlaylist,
}

// actionDefs is one action as a menu entry per kind it is for. Menu entries
// need an ID each, so it is the action's with "/<kind>"; the invocation
// carries the action's own.
func (f *feature) actionDefs(addon, name string, a services.AddonAction) []kit.ActionDef {
	label := entry(a.Label, name)
	var out []kit.ActionDef
	for i, k := range a.Kinds {
		kind, ok := kinds[k]
		if !ok || label == "" {
			continue
		}
		out = append(out, kit.ActionDef{
			ID: a.ID + "/" + k, Target: kind, Icon: iconOf(a.Icon), Group: kit.GroupAddon, Order: i,
			Label: func(*kit.ActionContext) string { return label },
			Run: func(ac *kit.ActionContext) {
				if t, ok := services.AddonTargetOf(ac.Item().Target); ok {
					f.invoke(addon, name, a.ID, &t)
				}
			},
		})
	}
	return out
}

// command is a palette entry, in the palette's Add-ons group, with no keys.
func (f *feature) command(addon, name string, c services.AddonCommand) module.Shortcut {
	label := entry(c.Label, name)
	keywords := clean(c.Keywords, 200) + " " + name
	return module.Shortcut{
		ID: c.ID, Group: module.AddonsGroup, Icon: iconOf(c.Icon), NoPalette: label == "",
		Label:    func() string { return label },
		Keywords: func() string { return keywords },
		Run:      func() { f.invoke(addon, name, c.ID, nil) },
	}
}

// entry is a contribution's text as it is shown: the add-on's label, cleaned
// and cut, then the add-on's name, so no contribution is anonymous.
func entry(label, name string) string {
	label = clean(label, 60)
	if label == "" {
		return ""
	}
	return i18n.T("{label} · {addon}", "label", label, "addon", name)
}

// clean is plain text of at most n bytes: control, bidirectional and other
// format characters become spaces, ends trimmed. The service cleans what it
// hands over; this does not rely on it.
func clean(s string, n int) string { return strings.TrimSpace(addons.Sanitize(s, n)) }

// defaultIcon is the icon of a contribution that names none, or one Sona
// doesn't have.
const defaultIcon = "square-terminal"

// iconOf is the icon an add-on named, one of Sona's own; a name Sona doesn't
// have gets a plain one.
func iconOf(name string) string {
	if canon, ok := theme.IconName(name); ok {
		return canon
	}
	return defaultIcon
}

// invoke asks the add-on to run an action or command. The call runs off the
// main thread and its answer comes back through Post; the same one chosen
// again before it answers does nothing.
func (f *feature) invoke(addon, name, action string, target *services.AddonTarget) {
	if f.api == nil {
		return
	}
	key := addon + "\x00" + action
	if f.busy[key] {
		return
	}
	f.busy[key] = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), invokeTimeout)
		defer cancel()
		res, err := f.api.Invoke(ctx, addon, action, target)
		f.env.Post(func() {
			delete(f.busy, key)
			f.answer(addon, name, res, err)
		})
	}()
}

// answer shows what an add-on said, or why it didn't: a toast under the
// add-on's name, at most one a second per add-on (the later answer is
// dropped, never stacked). Main thread.
func (f *feature) answer(addon, name string, res services.AddonResult, err error) {
	id := "addon-toast:" + addon
	if err != nil {
		var why string
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			why = i18n.T("It didn't answer in time.")
		case errors.Is(err, services.ErrAddonUnavailable):
			why = i18n.T("It isn't running.")
		default:
			why = i18n.ErrorText(services.CodeOf(err))
		}
		kit.ShowToast(kit.ToastSpec{ID: id, Title: name, Description: why, Kind: kit.ToastError})
		return
	}
	now := f.now()
	if last, ok := f.lastToast[addon]; ok && now.Sub(last) < toastEvery {
		return
	}
	f.lastToast[addon] = now
	text := clean(res.Toast, cli.MaxToast)
	if text == "" {
		text = i18n.T("Done.")
	}
	spec := kit.ToastSpec{ID: id, Title: name, Description: text, Kind: kit.ToastSuccess}
	if link, host, ok := safeLink(res.URL); ok {
		// The link is offered, never followed: it opens when its button is
		// clicked, and the button says where.
		spec.Action = i18n.T("Open {host}", "host", host)
		spec.Duration = toastLong
		spec.OnAction = func() { f.open(link) }
	}
	kit.ShowToast(spec)
}

// open opens a link the user clicked, if it is still one Sona offers.
func (f *feature) open(link string) {
	if _, _, ok := safeLink(link); !ok || f.env.OpenURL == nil {
		return
	}
	if err := f.env.OpenURL(link); err != nil {
		kit.ToastFailure(i18n.T("Couldn't open the link"), err)
	}
}

// safeLink is the link an add-on's answer may offer and the host to show on
// its button (cli.SafeLink: http or https, no credentials). Sona checks it
// again here, when it is offered and when it is clicked.
func safeLink(raw string) (link, host string, ok bool) {
	host, ok = cli.SafeLink(raw)
	if !ok {
		return "", "", false
	}
	if len(host) > maxHost {
		host = host[:maxHost] + "…"
	}
	return raw, host, true
}

// waiting is the add-ons the notification centre lists: one that waits
// for the person's approval, or can't run at all. One that crashed and was
// switched off is an entry of the centre instead (Go posts it).
func (f *feature) waiting() []services.AddonInfo {
	var out []services.AddonInfo
	for _, a := range f.infos {
		switch a.State {
		case services.AddonNeedsApproval, services.AddonNoSandbox, services.AddonInvalid:
			out = append(out, a)
		}
	}
	return out
}

// status is the Activity's Status: news while an add-on waits for an
// approval, an alert while one can't run.
func (f *feature) status(*ui.Context) module.ActivityStatus {
	var s module.ActivityStatus
	for _, a := range f.waiting() {
		s.Shown = true
		if a.State == services.AddonNeedsApproval {
			s.News = true
		} else {
			s.Alert = true
		}
	}
	return s
}

// rows is the Activity's rows: one per add-on that waits, each with Review,
// which opens Settings → Add-ons where the approval and the log are.
func (f *feature) rows(c *ui.Context) {
	for _, a := range f.waiting() {
		name := clean(a.Name, 60)
		if name == "" {
			name = clean(a.ID, 60)
		}
		spec := kit.ActivitySpec{Action: i18n.T("Review"), OnAction: f.review}
		if a.State == services.AddonNeedsApproval {
			spec.Icon = "shield-alert"
			spec.Title = i18n.T("{name} needs your approval", "name", name)
			spec.Detail = i18n.T("It asks for a permission you haven't approved. It doesn't run until you do.")
		} else {
			spec.Icon, spec.Alert = "circle-alert", true
			spec.Title = i18n.T("{name} can't run", "name", name)
			spec.Detail = i18n.T("Settings → Add-ons says why.")
		}
		kit.ActivityRow(c, "addon:"+a.ID, spec)
	}
}

// review opens Settings → Add-ons.
func (f *feature) review() {
	if f.env.Nav != nil {
		f.env.Nav.Push("/settings?section=addons")
	}
}

// devAsk puts `sona addons dev`'s question to the person: this folder, these
// permissions, for this run. Nothing runs unless they say yes here, in Sona's
// own window. A question of the person's own on screen is never replaced: the
// answer is no, and the command may be tried again.
func (f *feature) devAsk(q services.AddonDevAsk) {
	if kit.ConfirmPending() {
		go f.api.AnswerDev(q.Token, false)
		return
	}
	perms := i18n.T("none")
	if len(q.Permissions) > 0 {
		perms = clean(strings.Join(q.Permissions, ", "), 300)
	}
	f.asking = q.Token
	kit.Confirm(kit.ConfirmOptions{
		Title: i18n.T("Run {name}?", "name", clean(q.Name, 60)),
		Body: i18n.T("It runs in the sandbox from the folder {dir} and asks for these permissions: {permissions}. It runs until you stop it in the terminal or quit Sona, and nothing is saved.",
			"dir", clean(q.Dir, 300), "permissions", perms),
		ConfirmLabel: i18n.T("Run"),
		CancelLabel:  i18n.T("Don't run"),
	}, func(ok bool) {
		if f.asking != q.Token {
			return
		}
		f.asking = ""
		go f.api.AnswerDev(q.Token, ok)
	})
}

// devAskEnd closes the question when its time ran out or it was answered
// elsewhere, if it is still the one on screen.
func (f *feature) devAskEnd(token string) {
	if f.asking == token {
		f.asking = ""
		kit.ConfirmCancel()
	}
}
