package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// The settings export document: {"format": ExportFormat, "version":
// ExportVersion, "settings": {...}}. The settings object uses the
// settings.json names; a later version that renames or reshapes a key
// raises ExportVersion and migrates older documents in ParseExport.
const (
	ExportFormat  = "sona-settings"
	ExportVersion = 1
	// MaxExportBytes bounds a settings file read for import; an export
	// is a few kilobytes.
	MaxExportBytes = 1 << 20
)

// localKeys are the settings an export leaves out and an import ignores:
// they describe this computer or this session, not how the user likes
// Sona, so they never travel to another machine. Export drops them by
// name; Import restores the matching fields in keepLocal, because
// encoding/json matches keys case-insensitively and a filter on the raw
// key ("volume") would let "Volume" through.
var localKeys = []string{
	"volume",             // the player's live level
	"diagnostics",        // WebKit workarounds for this GPU and driver
	"scrobbling",         // account connections; their keys stay in this keyring
	"window",             // this screen's window size
	"desktop.autostart",  // mirrors this computer's autostart entry
	"desktop.lastRoute",  // where this session left off
	"audio.outputDevice", // a PipeWire sink on this computer
	"onboarding",         // whether this computer's first-run setup was done
}

// keepLocal copies the localKeys fields from src into dst.
func keepLocal(dst *Settings, src Settings) {
	dst.Volume = src.Volume
	dst.Diagnostics, dst.Scrobbling, dst.Window = src.Diagnostics, src.Scrobbling, src.Window
	dst.Desktop.Autostart, dst.Desktop.LastRoute = src.Desktop.Autostart, src.Desktop.LastRoute
	dst.Audio.OutputDevice = src.Audio.OutputDevice
	dst.Onboarding = src.Onboarding
}

// Import errors, worded for the person who picked the file.
var (
	ErrNotExport   = errors.New("this file isn't a Sona settings export")
	ErrNewerExport = errors.New("this file was exported by a newer version of Sona")
	ErrBadValue    = errors.New("this file has a value that isn't valid")
)

type exportDoc struct {
	Format   string          `json:"format"`
	Version  int             `json:"version"`
	Settings json.RawMessage `json:"settings"`
}

// Export returns s, normalised and without the local keys, as an export
// document.
func Export(s Settings) ([]byte, error) {
	m, err := settingsMap(s.Normalized())
	if err != nil {
		return nil, err
	}
	for _, k := range localKeys {
		deletePath(m, k)
	}
	body, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("config: export settings: %w", err)
	}
	b, err := json.MarshalIndent(exportDoc{Format: ExportFormat, Version: ExportVersion, Settings: body}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("config: export settings: %w", err)
	}
	return append(b, '\n'), nil
}

// Import is a parsed export document, ready to apply to the current
// settings.
type Import struct {
	// settings is the document's settings object, as written. Apply
	// decodes it and then puts the local fields back (keepLocal).
	settings []byte
	// shortcuts: the document lists the shortcut overrides, which then
	// replace the current ones instead of merging with them.
	shortcuts bool
}

// ParseExport reads an export document. Unknown keys are ignored, local
// settings are never imported, and a value of the wrong type rejects the
// whole file; out-of-range values are left to [Settings.Normalized] when
// the import is applied through [Store.Update].
func ParseExport(data []byte) (Import, error) {
	var doc exportDoc
	if err := json.Unmarshal(data, &doc); err != nil || doc.Format != ExportFormat {
		return Import{}, ErrNotExport
	}
	switch {
	case doc.Version > ExportVersion:
		return Import{}, ErrNewerExport
	case doc.Version < 1:
		return Import{}, ErrNotExport
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(doc.Settings, &m); err != nil || m == nil {
		return Import{}, ErrNotExport
	}
	probe := Defaults()
	if err := json.Unmarshal(doc.Settings, &probe); err != nil {
		if te, ok := errors.AsType[*json.UnmarshalTypeError](err); ok && te.Field != "" {
			return Import{}, fmt.Errorf("%w: %s", ErrBadValue, te.Field)
		}
		return Import{}, ErrNotExport
	}
	// encoding/json matches field names case-insensitively, so does this.
	shortcuts := false
	for k := range m {
		shortcuts = shortcuts || strings.EqualFold(k, "shortcuts")
	}
	return Import{settings: doc.Settings, shortcuts: shortcuts}, nil
}

// Apply writes the imported values over s; keys the document does not have
// keep their values, and the local settings always keep theirs. It never
// writes into s's existing maps or slices, so a copy of s taken before
// still holds the old values.
func (im Import) Apply(s *Settings) {
	next := s.clone()
	if im.shortcuts {
		next.Shortcuts = nil
	}
	// ParseExport decoded the same bytes into Settings without error.
	_ = json.Unmarshal(im.settings, &next)
	keepLocal(&next, *s)
	*s = next
}

// Changes lists the settings that differ between a and b, as dotted JSON
// paths ("theme", "desktop.closeAction"), sorted; empty, never nil, when
// they are the same. The shortcut map and lists compare as one value each.
func Changes(a, b Settings) []string {
	ma, errA := settingsMap(a)
	mb, errB := settingsMap(b)
	out := []string{}
	if errA != nil || errB != nil {
		return out
	}
	fa, fb := map[string]any{}, map[string]any{}
	flatten("", ma, fa)
	flatten("", mb, fb)
	for k, v := range fa {
		if w, ok := fb[k]; !ok || !reflect.DeepEqual(v, w) {
			out = append(out, k)
		}
	}
	for k := range fb {
		if _, ok := fa[k]; !ok {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

// settingsMap is s as its generic JSON object.
func settingsMap(s Settings) (map[string]any, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("config: encode settings: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("config: encode settings: %w", err)
	}
	return m, nil
}

// flatten collects m's leaves under dotted paths. The shortcut map is a
// leaf: its keys are ids that contain dots themselves.
func flatten(prefix string, m map[string]any, out map[string]any) {
	for k, v := range m {
		path := prefix + k
		if sub, ok := v.(map[string]any); ok && path != "shortcuts" {
			flatten(path+".", sub, out)
			continue
		}
		out[path] = v
	}
}

// deletePath removes the dotted path from m, if every step is an object.
func deletePath(m map[string]any, path string) {
	parent, key, nested := strings.Cut(path, ".")
	if !nested {
		delete(m, path)
		return
	}
	if sub, ok := m[parent].(map[string]any); ok {
		deletePath(sub, key)
	}
}
