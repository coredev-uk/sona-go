package services

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/config"
)

// settingsFilters are the file chooser's types for a settings export.
var settingsFilters = []app.FileFilter{{Name: "Sona settings (JSON)", Patterns: []string{"*.json"}}}

// ExportSettings asks where to save (the desktop's file chooser,
// Platform.Files) and writes the settings there as an export document
// (config.Export): without the values that belong to this computer, and
// never with a secret, since settings hold none. It returns the path, or
// "" when the user cancels.
func (s *SystemService) ExportSettings(ctx context.Context) (string, error) {
	if s.o.Platform.Files == nil {
		return "", unsupported("the file chooser")
	}
	path, err := s.o.Platform.Files.SaveFile(ctx, app.SaveFileOptions{
		Title:       "Export Sona settings",
		AcceptLabel: "Export",
		Name:        "sona-settings.json",
		Filters:     settingsFilters,
	})
	if err != nil || path == "" {
		return "", err
	}
	return path, s.exportSettings(path)
}

func (s *SystemService) exportSettings(path string) error {
	b, err := config.Export(s.o.Settings.Get())
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("couldn't write the settings file: %w", err)
	}
	s.o.Logger.Info("settings exported", "path", path)
	return nil
}

// ImportSettings asks for an export document (the desktop's file chooser)
// and applies it through the settings store, so every feature's OnChange
// observer sees the change. Keys the file lacks keep their values, unknown
// keys and this computer's settings are ignored, and out-of-range values
// are normalised; a file that is not an export, or has a value of the wrong
// type, changes nothing. It returns what changed; Path is "" when the user
// cancels.
func (s *SystemService) ImportSettings(ctx context.Context) (SettingsImport, error) {
	if s.o.Platform.Files == nil {
		return SettingsImport{}, unsupported("the file chooser")
	}
	path, err := s.o.Platform.Files.OpenFile(ctx, app.OpenFileOptions{
		Title:       "Import Sona settings",
		AcceptLabel: "Import",
		Filters:     settingsFilters,
	})
	if err != nil || path == "" {
		return SettingsImport{}, err
	}
	return s.importSettings(path)
}

func (s *SystemService) importSettings(path string) (SettingsImport, error) {
	f, err := os.Open(path)
	if err != nil {
		return SettingsImport{}, fmt.Errorf("couldn't read the settings file: %w", err)
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, config.MaxExportBytes+1))
	if err != nil {
		return SettingsImport{}, fmt.Errorf("couldn't read the settings file: %w", err)
	}
	if len(b) > config.MaxExportBytes {
		return SettingsImport{}, config.ErrNotExport
	}
	im, err := config.ParseExport(b)
	if err != nil {
		return SettingsImport{}, err
	}
	var prev config.Settings
	saved, err := s.o.Settings.Update(func(st *config.Settings) {
		prev = *st
		im.Apply(st)
	})
	if err != nil {
		return SettingsImport{}, err
	}
	out := SettingsImport{Path: path, Changed: config.Changes(prev, saved)}
	if len(out.Changed) > 0 {
		s.o.Emit.Emit(EventSettingsChanged, saved)
	}
	s.o.Logger.Info("settings imported", "path", path, "changed", out.Changed)
	return out, nil
}
