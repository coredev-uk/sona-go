package app

import (
	"github.com/egoist/mygo"
	"golang.org/x/sys/windows"
)

// miniCornerRadius is 0 off macOS: the mini player keeps the desktop's
// corners.
const miniCornerRadius = 0

// micaBuild is the first Windows build whose DWM draws Mica as a system
// backdrop (DWMWA_SYSTEMBACKDROP_TYPE): Windows 11 22H2.
const micaBuild = 22621

// windowMaterial is the main window's material, shown behind the sidebar
// alone (docs/ui-architecture.md, "Window materials"): Mica from Windows 11
// 22H2, none before it. DWM draws it, never Sona.
func windowMaterial() mygo.Vibrancy {
	if windows.RtlGetVersion().BuildNumber >= micaBuild {
		return mygo.VibrancyMica
	}
	return mygo.VibrancyNone
}
