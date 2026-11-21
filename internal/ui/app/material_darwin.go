package app

import (
	"strconv"
	"strings"
	"syscall"

	"github.com/egoist/mygo"
)

// miniCornerRadius rounds the frameless mini player window, as macOS rounds
// its floating panels.
const miniCornerRadius = 20

// windowMaterial is the main window's material, shown behind the sidebar
// alone (docs/ui-architecture.md, "Window materials"): Liquid Glass from
// macOS 26, the sidebar material before it. The OS draws it, never Sona.
func windowMaterial() mygo.Vibrancy {
	if macOSMajor() >= 26 {
		return mygo.VibrancyGlass
	}
	return mygo.VibrancySidebar
}

// macOSMajor is the macOS major version (26 for Tahoe), 0 if unknown.
func macOSMajor() int {
	v, err := syscall.Sysctl("kern.osproductversion")
	if err != nil {
		return 0
	}
	major, _, _ := strings.Cut(v, ".")
	n, _ := strconv.Atoi(major)
	return n
}
