package services

import "golang.org/x/sys/unix"

// osName is the macOS version and build from sysctl, as `sw_vers` reports
// them: "macOS 27.2 (26B123)".
func osName() string {
	version, _ := unix.Sysctl("kern.osproductversion")
	build, _ := unix.Sysctl("kern.osversion")
	return macOSName(version, build)
}
