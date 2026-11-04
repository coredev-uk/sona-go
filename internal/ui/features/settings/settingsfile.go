package settings

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// rowTitle is the Settings row a changed setting belongs to, by Go's dotted
// settings name (SystemService.ImportSettings' Changed), and whether there
// is one. A name matches its longest listed prefix (settings-file.ts), so
// every field of a group (the equaliser) reads as one row.
func rowTitle(name string) (string, bool) {
	for key := name; key != ""; {
		if t := listedRow(key); t != "" {
			return t, true
		}
		i := strings.LastIndex(key, ".")
		if i < 0 {
			break
		}
		key = key[:i]
	}
	return name, false
}

// listedRow is the title of the row a settings name is listed for, "" when
// it isn't. Each title is a literal msgid, so it is looked up here, not in a
// table of strings.
func listedRow(key string) string {
	switch key {
	case "theme":
		return i18n.T("Theme")
	case "reduceEffects":
		return i18n.T("Reduce effects")
	case "motionArtwork.enabled":
		return i18n.T("Motion artwork")
	case "audioQuality":
		return i18n.T("Audio quality")
	case "engineIdleMinutes":
		return i18n.T("Free memory when idle")
	case "autoPlay":
		return i18n.T("AutoPlay")
	case "audio.soundCheck":
		return i18n.T("Sound Check")
	case "audio.equalizer":
		return i18n.T("Equaliser")
	case "crossfade":
		return i18n.T("Crossfade")
	case "queue.replaceConfirm":
		return i18n.T("Replacing your queue")
	case "lyrics":
		return i18n.T("Lyrics")
	case "desktop.closeAction":
		return i18n.T("When the window is closed")
	case "desktop.startHidden":
		return i18n.T("Start in the tray")
	case "desktop.startPage", "desktop.resumeLastRoute":
		return i18n.T("Open on")
	case "integrations.notifications":
		return i18n.T("Track notifications")
	case "integrations.inhibitSleep":
		return i18n.T("Stay awake while playing")
	case "integrations.discordPresence":
		return i18n.T("Discord status")
	case "shortcuts":
		return i18n.T("Keyboard shortcuts")
	case "storage.artCacheMiB":
		return i18n.T("Artwork cache limit")
	case "updates.check":
		return i18n.T("Check for updates")
	}
	return ""
}

// changedRows is the titles of the Settings rows the changed names belong
// to, once each, in order (settings-file.ts changedRows).
func changedRows(changed []string) []string {
	rows := make([]string, 0, len(changed))
	seen := make(map[string]bool, len(changed))
	for _, name := range changed {
		t, _ := rowTitle(name)
		if !seen[t] {
			seen[t] = true
			rows = append(rows, t)
		}
	}
	return rows
}

// fileAPI is what the row asks of SystemService: the settings file through
// the desktop's file chooser.
type fileAPI interface {
	ExportSettings(ctx context.Context) (string, error)
	ImportSettings(ctx context.Context) (services.SettingsImport, error)
}

// fileState is the row's calls in flight.
type fileState struct{ exporting, importing bool }

// settingsFile is the Settings file row (SettingsFileRows.tsx): save the
// settings to a file and load one, through the desktop's file chooser. Go
// leaves out what belongs to this computer and applies an import like any
// other change, so settings:changed refreshes every row. Without a file
// chooser the row says so.
func (s *settings) settingsFile(c *ui.Context) {
	caps := s.caps()
	if !caps.HasData {
		return
	}
	title := i18n.T("Settings file")
	api, ok := s.sys.(fileAPI)
	if !caps.Data.FileChooser || !ok {
		s.unavailableRow(c, "settings-file", title, "")
		return
	}
	st := stateOf[fileState](s, "settings-file")
	busy := st.exporting || st.importing
	s.row(c, rowOpts{
		ID: "settings-file", Title: title,
		Desc: i18n.T("Save your settings to a file, or load one exported from Sona on another computer. The window size, audio output, rendering fixes and connected accounts stay with this computer."),
	}, func() ui.Element {
		var first ui.Element
		ui.Row(c.Key("files")).Gap(8).Role(ui.RoleGroup).Label(title).Description("").Children(func() {
			label := i18n.T("Export…")
			if st.exporting {
				label = i18n.T("Exporting…")
			}
			b := kit.Button(c.Key("export"), label, kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Disabled: busy})
			first = b
			if b.Clicked() {
				s.exportSettings(api, st)
			}
			label = i18n.T("Import…")
			if st.importing {
				label = i18n.T("Importing…")
			}
			if kit.Button(c.Key("import"), label, kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Disabled: busy}).Clicked() {
				s.importSettings(api, st)
			}
		})
		return first
	})
}

func (s *settings) exportSettings(api fileAPI, st *fileState) {
	st.exporting = true
	go func() {
		p, err := api.ExportSettings(context.Background())
		s.env.Post(func() {
			st.exporting = false
			switch {
			case err != nil:
				kit.ToastErrorf(i18n.T("Couldn't export your settings"), i18n.ErrorMessage(err))
			case p != "": // "" when the chooser was cancelled
				kit.ShowToast(kit.ToastSpec{
					Title: i18n.T("Settings exported"), Description: i18n.T("Saved as {file}.", "file", filepath.Base(p)),
				})
			}
		})
	}()
}

func (s *settings) importSettings(api fileAPI, st *fileState) {
	st.importing = true
	go func() {
		r, err := api.ImportSettings(context.Background())
		s.env.Post(func() {
			st.importing = false
			switch {
			case err != nil:
				kit.ToastErrorf(i18n.T("Couldn't import the settings"), i18n.ErrorMessage(err))
			case r.Path == "": // the chooser was cancelled
			default:
				rows := changedRows(r.Changed)
				if len(rows) == 0 {
					kit.ShowToast(kit.ToastSpec{
						Title:       i18n.T("Nothing to change"),
						Description: i18n.T("The settings in {file} already match yours.", "file", filepath.Base(r.Path)),
					})
					return
				}
				kit.ShowToast(kit.ToastSpec{
					Title:       i18n.T("{count, plural, one {Imported # setting} other {Imported # settings}}", "count", len(rows)),
					Description: strings.Join(rows, ", "),
				})
			}
		})
	}()
}
