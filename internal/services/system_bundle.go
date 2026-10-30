package services

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/coredev-uk/sona-go/internal/diag"
)

// DiagnosticsBundle builds the bug-report bundle (Settings → Diagnostics →
// Copy diagnostics): the Diagnostics report (versions, CDM probe, playback
// host, paths), the OS and desktop session, the settings, crash output and
// the last log lines, with tokens, e-mail addresses and the home directory
// scrubbed. It is also saved to a 0600 file in the state directory.
func (s *SystemService) DiagnosticsBundle(ctx context.Context) (DiagBundle, error) {
	d, err := s.Diagnostics(ctx)
	sections := []diag.Section{
		{Title: "System", Value: map[string]string{
			"os":      osRelease(),
			"desktop": os.Getenv("XDG_CURRENT_DESKTOP"),
			"session": os.Getenv("XDG_SESSION_TYPE"),
		}},
		{Title: "Diagnostics", Value: d},
	}
	if err != nil {
		sections = append(sections, diag.Section{Title: "Diagnostics errors", Value: err.Error()})
	}
	sections = append(sections,
		diag.Section{Title: "Account", Value: map[string]string{"state": string(s.o.Auth.Status().State)}},
		diag.Section{Title: "Settings", Value: s.o.Settings.Get()},
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

// osRelease is /etc/os-release's PRETTY_NAME ("" if unknown).
func osRelease() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "PRETTY_NAME="); ok {
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}
