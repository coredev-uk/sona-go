package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/diag"
)

// DiagnosticsBundle builds the bug-report bundle (Settings → Diagnostics →
// Copy diagnostics): the OS (and, on XDG desktops, the desktop and session
// type), the Diagnostics report (versions, CDM probe, playback host,
// paths), the Memory section (the app's and the playback host's memory as
// the OS counts it, the Go runtime's), the features' sections (Env.OnDiagnostics: motion artwork), the
// settings (without the city, the open sidebar folders, the audio output
// device and the add-ons' own text settings: see [config.ForDiagnostics]), crash output and the last log lines, with tokens, e-mail
// addresses and the home directory scrubbed. It is also saved to a 0600
// file in the state directory.
func (s *SystemService) DiagnosticsBundle(ctx context.Context) (DiagBundle, error) {
	d, err := s.Diagnostics(ctx)
	sections := []diag.Section{
		{Title: "System", Value: systemInfo()},
		{Title: "Diagnostics", Value: d},
	}
	if err != nil {
		sections = append(sections, diag.Section{Title: "Diagnostics errors", Value: err.Error()})
	}
	sections = append(sections, diag.Section{Title: "Memory", Value: memoryReport(d.App.PID, d.Playback.HostPID)})
	sections = append(sections, s.o.diagnosticSections()...)
	sections = append(sections,
		diag.Section{Title: "Account", Value: map[string]string{"state": string(s.o.Auth.Status().State)}},
		s.settingsSection(),
	)
	home, _ := os.UserHomeDir()
	now := s.o.Now()
	text := diag.Bundle(diag.BundleOptions{
		Now:      now,
		Sections: sections,
		LogFile:  s.o.Paths.LogFile(),
		StateDir: s.o.Paths.State,
		Home:     home,
	})
	out := DiagBundle{Text: text, Path: filepath.Join(s.o.Paths.State, "sona-diagnostics-"+now.Format("20060102-150405")+".txt")}
	if werr := os.WriteFile(out.Path, []byte(text), 0o600); werr != nil {
		out.Path = ""
		return out, errors.Join(err, fmt.Errorf("save diagnostics: %w", werr))
	}
	s.o.Logger.Info("diagnostics bundle saved", "path", out.Path)
	return out, nil
}

// settingsSection is the bundle's Settings section. The bundle is pasted
// into public issues, so it leaves out what says where the person is or
// which hardware the computer has, and lists what it left out.
func (s *SystemService) settingsSection() diag.Section {
	return diag.Section{Title: "Settings", Value: config.ForDiagnostics(s.o.Settings.Get())}
}
