package services

import (
	"maps"
	"strings"
	"testing"
)

func TestSystemSectionPerOS(t *testing.T) {
	env := map[string]string{"XDG_CURRENT_DESKTOP": "GNOME", "XDG_SESSION_TYPE": "wayland"}
	getenv := func(k string) string { return env[k] }
	cases := []struct {
		goos, os string
		want     map[string]string
	}{
		// Linux keeps its three keys, empty ones included.
		{"linux", "Fedora Linux 43 (Workstation Edition)", map[string]string{
			"os": "Fedora Linux 43 (Workstation Edition)", "desktop": "GNOME", "session": "wayland",
		}},
		{"freebsd", "FreeBSD 15.0-RELEASE", map[string]string{"os": "FreeBSD 15.0-RELEASE", "desktop": "GNOME", "session": "wayland"}},
		// macOS and Windows have no XDG desktop or session: only the OS,
		// even when such variables happen to be set.
		{"darwin", "macOS 27.2 (26B123)", map[string]string{"os": "macOS 27.2 (26B123)"}},
		{"windows", "Windows 11 Pro 24H2 (build 26100.2605)", map[string]string{"os": "Windows 11 Pro 24H2 (build 26100.2605)"}},
	}
	for _, c := range cases {
		if got := systemSection(c.goos, c.os, getenv); !maps.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.goos, got, c.want)
		}
	}
	if got := systemSection("linux", "", func(string) string { return "" }); !maps.Equal(got, map[string]string{"os": "", "desktop": "", "session": ""}) {
		t.Errorf("linux, nothing known: %v", got)
	}
}

func TestOSReleaseName(t *testing.T) {
	for in, want := range map[string]string{
		"NAME=NixOS\nPRETTY_NAME=\"NixOS 26.05 (Yarara)\"\nID=nixos\n": "NixOS 26.05 (Yarara)",
		"PRETTY_NAME='Arch Linux'\n":                                   "Arch Linux",
		"PRETTY_NAME=Debian\n":                                         "Debian",
		"NAME=Unnamed\n":                                               "",
		"":                                                             "",
	} {
		if got := osReleaseName(strings.NewReader(in)); got != want {
			t.Errorf("osReleaseName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMacOSName(t *testing.T) {
	for _, c := range []struct{ version, build, want string }{
		{"27.2", "26B123", "macOS 27.2 (26B123)"},
		{"15.7.1", "24G231", "macOS 15.7.1 (24G231)"},
		{"27.2", "", "macOS 27.2"},
		{"", "", "macOS"},
	} {
		if got := macOSName(c.version, c.build); got != c.want {
			t.Errorf("macOSName(%q, %q) = %q, want %q", c.version, c.build, got, c.want)
		}
	}
}

func TestWindowsName(t *testing.T) {
	for _, c := range []struct {
		product, display, build string
		ubr                     uint64
		want                    string
	}{
		// Windows 11 still says "Windows 10" in ProductName.
		{"Windows 10 Pro", "24H2", "26100", 2605, "Windows 11 Pro 24H2 (build 26100.2605)"},
		{"Windows 10 Home", "21H2", "22000", 0, "Windows 11 Home 21H2 (build 22000)"},
		// Windows 10 itself is left alone, up to the last build before 11.
		{"Windows 10 Enterprise", "22H2", "19045", 6456, "Windows 10 Enterprise 22H2 (build 19045.6456)"},
		{"Windows 10 Pro", "21H2", "21999", 1, "Windows 10 Pro 21H2 (build 21999.1)"},
		// A product name that is already right stays so.
		{"Windows 11 Pro", "25H2", "26200", 7019, "Windows 11 Pro 25H2 (build 26200.7019)"},
		{"Windows Server 2025 Datacenter", "24H2", "26100", 1, "Windows Server 2025 Datacenter 24H2 (build 26100.1)"},
		// Missing values are left out.
		{"Windows 10 Pro", "", "19041", 0, "Windows 10 Pro (build 19041)"},
		{"", "", "", 0, "Windows"},
		{"", "", "notanumber", 0, "Windows (build notanumber)"},
	} {
		if got := windowsName(c.product, c.display, c.build, c.ubr); got != c.want {
			t.Errorf("windowsName(%q, %q, %q, %d) = %q, want %q", c.product, c.display, c.build, c.ubr, got, c.want)
		}
	}
}
