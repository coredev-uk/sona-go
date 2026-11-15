package config

import (
	"maps"
	"slices"
)

// Addons is what the user decided about each add-on, by add-on ID: it is
// the one place a permission is granted (docs/addons.md). It describes this
// computer, so settings exports leave it out and an import never touches
// it: code is approved where it runs, and an imported file must not be able
// to approve any.
type Addons map[string]AddonSetting

// AddonSetting is one add-on's state.
type AddonSetting struct {
	// Enabled: the add-on runs when Sona starts.
	Enabled bool `json:"enabled"`
	// Granted are the permissions the user approved, as shown to them when
	// they enabled it. An add-on whose manifest asks for one that isn't here
	// does not run until the user approves again. Revoking one removes it
	// here, and the next call that needs it is refused.
	Granted []string `json:"granted"`
	// Values are the add-on's non-secret settings rows, by row ID: a toggle
	// "true" or "false", a text, a choice's value. A row the user never set
	// has none here. Secrets never are: they live in the keyring under
	// `addon.<id>`.
	Values map[string]string `json:"values,omitempty"`
}

// Limits of what the file may hold: add-ons are a handful, and a file that
// lists more is not the user's.
const (
	MaxAddons          = 100
	maxAddonIDLength   = 40
	maxAddonGrants     = 32
	maxAddonPermLength = 40
	maxAddonValues     = 32
	maxAddonValueKey   = 40
	maxAddonValueBytes = 1024
)

// normalized drops entries with an unreasonable ID, keeps at most MaxAddons
// of them (the first by ID) and tidies each grant list; an empty set is nil,
// as in the defaults.
func (a Addons) normalized() Addons {
	if len(a) == 0 {
		return nil
	}
	ids := make([]string, 0, len(a))
	for id := range a {
		if id != "" && len(id) <= maxAddonIDLength {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	ids = ids[:min(len(ids), MaxAddons)]
	if len(ids) == 0 {
		return nil
	}
	out := make(Addons, len(ids))
	for _, id := range ids {
		s := a[id]
		g := tidyIDs(s.Granted)
		g = slices.DeleteFunc(g, func(p string) bool { return len(p) > maxAddonPermLength })
		s.Granted = g[:min(len(g), maxAddonGrants)]
		if len(s.Granted) == 0 {
			s.Granted = nil
		}
		s.Values = normalizedValues(s.Values)
		out[id] = s
	}
	return out
}

// normalizedValues keeps at most maxAddonValues rows (the first by ID) whose
// ID and value are of a sane length; none is nil.
func normalizedValues(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for k, v := range in {
		if k != "" && len(k) <= maxAddonValueKey && len(v) <= maxAddonValueBytes {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	keys = keys[:min(len(keys), maxAddonValues)]
	if len(keys) == 0 {
		return nil
	}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = in[k]
	}
	return out
}

// clone returns a copy that shares nothing with a: the Store hands out
// copies and an Update writes into its own.
func (a Addons) clone() Addons {
	if a == nil {
		return nil
	}
	out := make(Addons, len(a))
	for id, s := range a {
		s.Granted = slices.Clone(s.Granted)
		s.Values = maps.Clone(s.Values)
		out[id] = s
	}
	return out
}
