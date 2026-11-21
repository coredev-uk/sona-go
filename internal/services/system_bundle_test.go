package services

import (
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/diag"
)

// The bundle is pasted into public issues: the Settings section must not
// say which city the person chose or which devices the computer has.
func TestDiagnosticsBundleSettingsLeaveOutLocation(t *testing.T) {
	sys, _, _ := systemEnv(t, func(s *config.Settings) {
		s.Theme = config.ThemeDark
		s.Concerts.City = "gb-manchester"
		s.Sidebar.OpenFolders = []string{"p.folder"}
		s.Audio.OutputDevice = "alsa_output.usb-Vendor_DAC_SERIAL1234-00.analog-stereo"
	})
	text := diag.Bundle(diag.BundleOptions{Sections: []diag.Section{sys.settingsSection()}})
	for _, leaked := range []string{"manchester", "p.folder", "SERIAL1234", "usb-Vendor"} {
		if strings.Contains(text, leaked) {
			t.Errorf("the bundle carries %q:\n%s", leaked, text)
		}
	}
	for _, want := range []string{`"theme": "dark"`, "concerts.city", "audio.outputDevice"} {
		if !strings.Contains(text, want) {
			t.Errorf("the bundle lacks %s:\n%s", want, text)
		}
	}
}
