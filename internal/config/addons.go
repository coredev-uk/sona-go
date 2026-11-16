package config

import "slices"

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
}

// Limits of what the file may hold: add-ons are a handful, and a file that
// lists more is not the user's.
const (
	MaxAddons          = 100
	maxAddonIDLength   = 40
	maxAddonGrants     = 32
	maxAddonPermLength = 40
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
		out[id] = s
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
		out[id] = s
	}
	return out
}
