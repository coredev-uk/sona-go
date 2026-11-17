package services

import (
	"fmt"
	"maps"
	"slices"

	"github.com/coredev-uk/sona-go/internal/addons"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/config"
)

// addonOwn is what one add-on's session serves of its own data (cli.AddonOwn):
// the values of its settings rows and its secrets. A key is looked up under
// this add-on's ID only, so an add-on reads no one else's.
type addonOwn struct {
	s        *AddonsService
	id       string
	manifest addons.Manifest
	hub      *cli.Hub
	// dev is an add-on run by `addons dev`: it gets the defaults of its rows
	// and none of the user's saved values or secrets, whatever its ID.
	dev bool
}

func newAddonOwn(s *AddonsService, m addons.Manifest, dev bool) *addonOwn {
	o := &addonOwn{s: s, id: m.ID, manifest: m, hub: cli.NewHub(), dev: dev}
	o.publish()
	return o
}

// values are the non-secret rows' values: the saved ones over the defaults.
func (o *addonOwn) values(saved map[string]string) cli.SettingsValues {
	out := map[string]string{}
	for _, r := range o.manifest.Contributions.Settings {
		if r.Kind == addons.SettingSecret {
			continue
		}
		v, ok := saved[r.ID]
		if !ok || r.ValidValue(v) != nil {
			v = r.SettingDefault()
		}
		out[r.ID] = v
	}
	return cli.SettingsValues{Values: out}
}

// publish sends the current values to the add-on's subscribers (the hub
// drops a value that is the last one).
func (o *addonOwn) publish() {
	var saved map[string]string
	if !o.dev {
		saved = o.s.e.Settings.Get().Addons[o.id].Values
	}
	o.hub.Publish(cli.TopicSettings, o.values(saved))
}

// WatchSettings implements cli.AddonOwn.
func (o *addonOwn) WatchSettings() (<-chan any, func()) { return o.hub.Watch(cli.TopicSettings) }

// Secret implements cli.AddonOwn: a secret row of the manifest, or for one of
// Sona's own add-ons a key of its sign-in (addons.BundledSecret).
func (o *addonOwn) Secret(key string) (string, bool) {
	declared := slices.ContainsFunc(o.manifest.Contributions.Settings, func(r addons.Setting) bool {
		return r.ID == key && r.Kind == addons.SettingSecret
	})
	if o.dev || (!declared && !addons.BundledSecret(o.id, key)) {
		return "", false
	}
	m, err := o.s.e.addonSecrets().all(o.id)
	if err != nil {
		o.s.e.Logger.Warn("addons: read secrets", "addon", o.id, "err", err)
		return "", false
	}
	v, ok := m[key]
	return v, ok && v != ""
}

// row finds the manifest's settings row of a valid add-on.
func (s *AddonsService) row(id, rowID string) (addons.Entry, addons.Setting, error) {
	en, err := s.find(id)
	if err != nil {
		return addons.Entry{}, addons.Setting{}, err
	}
	for _, r := range en.Manifest.Contributions.Settings {
		if r.ID == rowID {
			return en, r, nil
		}
	}
	return addons.Entry{}, addons.Setting{}, fmt.Errorf("%w: add-on %q has no setting %q", ErrNotFoundAddon, id, rowID)
}

// SetValue saves a toggle, text or choice row of an add-on, and tells the add-on
// when it runs. A value the row can't hold is refused. A secret row is set
// with SetSecret: settings.json never holds one.
func (s *AddonsService) SetValue(id, row, value string) error {
	en, r, err := s.row(id, row)
	if err != nil {
		return err
	}
	if r.Kind == addons.SettingSecret {
		return fmt.Errorf("%w: setting %q is a secret", ErrInvalidArgument, row)
	}
	if err := r.ValidValue(value); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}
	if _, err := s.e.updateSettings(func(st *config.Settings) {
		if st.Addons == nil {
			st.Addons = config.Addons{}
		}
		a := st.Addons[en.ID]
		if a.Values == nil {
			a.Values = map[string]string{}
		}
		a.Values[row] = value
		st.Addons[en.ID] = a
	}); err != nil {
		return err
	}
	s.changed()
	return nil
}

// SetSecret saves a secret row in the keyring under `addon.<id>`; an empty
// value clears it. The value is sent to no one but the add-on that asks for
// it.
func (s *AddonsService) SetSecret(id, row, value string) error {
	_, r, err := s.row(id, row)
	if err != nil {
		return err
	}
	if r.Kind != addons.SettingSecret {
		return fmt.Errorf("%w: setting %q is not a secret", ErrInvalidArgument, row)
	}
	if err := r.ValidValue(value); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}
	err = s.e.addonSecrets().update(id, func(m map[string]string) {
		if value == "" {
			delete(m, row)
		} else {
			m[row] = value
		}
	})
	if err != nil {
		return fmt.Errorf("%w: %v", auth.ErrKeyring, err)
	}
	s.changed()
	return nil
}

// Contributions returns what the valid add-ons add to Sona, with their
// labels in locale ("de", "pt-BR"; the manifest's English where it has no
// translation). It reads the add-ons folder: call it off the UI's way (it
// is cheap, but it is I/O) and again when "addons:changed" arrives.
func (s *AddonsService) Contributions(locale string) []AddonContribution {
	set := s.e.Settings.Get().Addons
	s.mu.Lock()
	runners := make(map[string]*addonRunner, len(s.runners))
	for id, r := range s.runners {
		runners[id] = r
	}
	s.mu.Unlock()
	out := []AddonContribution{}
	// build is one add-on's contribution; rows are the add-on's settings rows
	// (an installed one's: a development run has none to show).
	build := func(m addons.Manifest, running, rows bool) {
		st := set[m.ID]
		c := AddonContribution{
			ID: m.ID, Name: m.Name, Running: running,
			Actions: []AddonAction{}, Commands: []AddonCommand{}, Rows: []AddonRow{},
		}
		text := func(key, fallback string) string {
			return addons.Sanitize(m.Text(locale, key, fallback), addons.MaxDetailRunes*4)
		}
		if running {
			for _, a := range m.Contributions.Actions {
				c.Actions = append(c.Actions, AddonAction{
					ID: addons.ActionID(m.ID, a.ID), Label: text("action."+a.ID, a.Label), Kinds: slices.Clone(a.Kinds), Icon: a.Icon,
				})
			}
			for _, a := range m.Contributions.Commands {
				c.Commands = append(c.Commands, AddonCommand{
					ID: addons.ActionID(m.ID, a.ID), Label: text("command."+a.ID, a.Label), Keywords: text("command."+a.ID+".keywords", a.Keywords), Icon: a.Icon,
				})
			}
		}
		var secrets map[string]string
		for _, r := range m.Contributions.Settings {
			if !rows {
				break
			}
			row := AddonRow{
				ID: r.ID, Kind: r.Kind, Label: text("setting."+r.ID, r.Label), Description: text("setting."+r.ID+".description", r.Description),
				Options: []AddonOption{},
			}
			for _, o := range r.Options {
				row.Options = append(row.Options, AddonOption{Value: o.Value, Label: text("option."+r.ID+"."+o.Value, o.Label)})
			}
			if r.Kind == addons.SettingSecret {
				if secrets == nil {
					secrets, _ = s.e.addonSecrets().all(m.ID)
				}
				row.Saved = secrets[r.ID] != ""
			} else if v, ok := st.Values[r.ID]; ok && r.ValidValue(v) == nil {
				row.Value = v
			} else {
				row.Value = r.SettingDefault()
			}
			c.Rows = append(c.Rows, row)
		}
		out = append(out, c)
	}
	for _, en := range s.scan() {
		if en.Problem == nil {
			_, running := runnerState(runners[en.ID])
			build(en.Manifest, running, true)
		}
	}
	// A development run (`addons dev`) is not installed: its actions and
	// commands are offered while it runs, and it has no settings rows.
	for _, id := range slices.Sorted(maps.Keys(runners)) {
		if r := runners[id]; r.dev != nil {
			if sess, m := r.current(); sess != nil {
				build(m, true, false)
			}
		}
	}
	return out
}

// runnerState is the state of a runner and whether its add-on is up.
func runnerState(r *addonRunner) (AddonState, bool) {
	if r == nil {
		return AddonOff, false
	}
	st, _ := r.snapshot()
	return st, st == AddonRunning
}
