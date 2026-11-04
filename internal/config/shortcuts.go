package config

import "strings"

// Shortcuts maps a UI shortcut id ("player.toggle") to its binding:
// space-separated chords ("Ctrl+K", "Shift+ArrowRight"), or "none" for no
// key. An id without an entry keeps its default keys. Go stores the strings
// only; the UI parses and validates them.
type Shortcuts map[string]string

// normalized drops entries with a blank id or binding (a blank binding means
// the default) and never returns nil, so the UI always gets an object.
func (s Shortcuts) normalized() Shortcuts {
	out := make(Shortcuts, len(s))
	for id, binding := range s {
		id, binding = strings.TrimSpace(id), strings.TrimSpace(binding)
		if id != "" && binding != "" {
			out[id] = binding
		}
	}
	return out
}
