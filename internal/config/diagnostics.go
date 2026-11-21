package config

// diagnosticsOmitted are the settings a diagnostics bundle leaves out. The
// bundle is meant for public issues, and these name where the person is or
// what hardware this computer has: the Concerts city, which folders of the
// library are open, and the audio output device (a PipeWire node name can
// carry a USB serial number). The rest of the settings stay, because a bug
// report needs them (theme, quality, equaliser, crossfade, shortcuts).
var diagnosticsOmitted = []string{
	"concerts.city",
	"sidebar.openFolders",
	"audio.outputDevice",
}

// DiagnosticsWithheldKey is the key under which [ForDiagnostics] lists what
// it left out, so a reader of the bundle knows a key is absent on purpose.
const DiagnosticsWithheldKey = "_withheld"

// ForDiagnostics is s as the generic JSON object the diagnostics bundle
// shows: the settings without the values that describe this computer or its
// owner (see diagnosticsOmitted, and every add-on's own text settings, which
// the user typed). It adds [DiagnosticsWithheldKey], the dotted paths it
// removed.
func ForDiagnostics(s Settings) map[string]any {
	m, err := settingsMap(s.Normalized())
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	withheld := make([]string, 0, len(diagnosticsOmitted)+1)
	for _, k := range diagnosticsOmitted {
		deletePath(m, k)
		withheld = append(withheld, k)
	}
	if addons, ok := m["addons"].(map[string]any); ok {
		for _, a := range addons {
			if one, ok := a.(map[string]any); ok {
				delete(one, "values")
			}
		}
		withheld = append(withheld, "addons.*.values")
	}
	m[DiagnosticsWithheldKey] = withheld
	return m
}
